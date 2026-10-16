package meta

import (
	"errors"
	"fmt"

	"github.com/formeo/fbrescue/internal/fdb"
	"github.com/formeo/fbrescue/internal/record"
)

// Version is the location and header of a primary record version.
type Version struct {
	Page   uint32
	Line   int // -1 when the error concerns the whole page
	Header record.Header
}

// DataPages returns the data pages of a relation: in sequence order as
// listed by its pointer pages, then any data pages of the relation that are
// in use but not listed, for sequence numbers the pointer pages do not
// cover (their pointer page is damaged or lost). The error reports damaged
// pointer pages; the pages returned are still usable.
func (c *Catalog) DataPages(rel uint16) ([]uint32, error) {
	if out, ok := c.dataPages[rel]; ok {
		return out, c.dataErr[rel]
	}
	var out []uint32
	var errs []error
	buf := make([]byte, c.db.PageSize)
	for _, pp := range c.pointers[rel] {
		if err := c.db.ReadPage(pp.page, buf); err != nil {
			errs = append(errs, err)
			continue
		}
		if buf[0] != fdb.TypePointer || le.Uint16(buf[0x1A:]) != rel {
			errs = append(errs, fmt.Errorf("page %d should be pointer page %d of relation %d", pp.page, pp.seq, rel))
			continue
		}
		n := int(le.Uint16(buf[0x18:]))
		if 0x20+4*n > len(buf) {
			errs = append(errs, fmt.Errorf("pointer page %d claims %d slots", pp.page, n))
			continue
		}
		for i := 0; i < n; i++ {
			if p := le.Uint32(buf[0x20+4*i:]); p != 0 {
				out = append(out, p)
			}
		}
	}

	listed, covered := map[uint32]bool{}, map[uint32]bool{}
	for _, p := range out {
		listed[p] = true
		if rs, ok := c.pageSeq[p]; ok && rs.rel == rel {
			covered[rs.seq] = true
		}
	}
	found := 0
	for _, d := range c.pageScan.DataPages[rel] {
		if !d.Unlisted && !listed[d.Page] && !covered[d.Seq] {
			out = append(out, d.Page)
			found++
		}
	}
	if found > 0 {
		c.warn("relation %d: %d data pages not reachable through its pointer pages were found by scanning the file", rel, found)
	}
	c.dataPages[rel], c.dataErr[rel] = out, errors.Join(errs...)
	return out, c.dataErr[rel]
}

// Records calls fn for every primary record version of relation rel (delete
// stubs included), in storage order. On a read error fn gets a nil record
// and the error; returning an error from fn stops the iteration.
func (c *Catalog) Records(rel uint16, fn func(v Version, rec []byte, err error) error) error {
	pages, err := c.DataPages(rel)
	if err != nil {
		if err := fn(Version{Line: -1}, nil, err); err != nil {
			return err
		}
	}
	for _, p := range pages {
		prel, _, slots, err := c.rd.DataPage(p)
		if err == nil && prel != rel {
			err = fmt.Errorf("data page %d belongs to relation %d", p, prel)
		}
		if err != nil {
			if err := fn(Version{Page: p, Line: -1}, nil, err); err != nil {
				return err
			}
			if slots == nil || prel != rel {
				continue
			}
		}
		for _, s := range slots {
			if !s.Header.Primary() {
				continue
			}
			v := Version{Page: p, Line: s.Line, Header: s.Header}
			var rec []byte
			var rerr error
			if s.Header.Flags&record.FlagDeleted == 0 {
				_, rec, rerr = c.rd.Read(p, s.Line)
			}
			if err := fn(v, rec, rerr); err != nil {
				return err
			}
		}
	}
	return nil
}

// visible calls fn for the current rows of a system table, warning about
// anything it has to skip.
func (c *Catalog) visible(rel uint16, fn func(v Version, rec []byte)) error {
	return c.Records(rel, func(v Version, rec []byte, err error) error {
		if err != nil {
			c.warn("system table %d: %v", rel, err)
			return nil
		}
		r, ok, err := c.Current(v, rec)
		switch {
		case err != nil:
			c.warn("system table %d, page %d line %d: %v", rel, v.Page, v.Line, err)
		case ok && !r.Deleted:
			fn(Version{Page: r.Page, Line: r.Line, Header: r.Header}, r.Data)
		}
		return nil
	})
}

// Transaction states in the transaction inventory.
const (
	TxActive    = 0 // still running, or its process died
	TxLimbo     = 1 // prepared two-phase transaction
	TxDead      = 2 // rolled back
	TxCommitted = 3
)

// TxState returns the state of a transaction from the transaction inventory.
func (c *Catalog) TxState(tx uint32) (int, error) {
	per := uint32(c.db.PageSize-0x14) * 4
	i := tx / per
	if int(i) >= len(c.tips) {
		return 0, fmt.Errorf("transaction %d is beyond the transaction inventory", tx)
	}
	page, ok := c.tipCache[c.tips[i]]
	if !ok {
		page = make([]byte, c.db.PageSize)
		if err := c.db.ReadPage(c.tips[i], page); err != nil {
			return 0, err
		}
		if page[0] != fdb.TypeTIP {
			return 0, fmt.Errorf("page %d should be a transaction inventory page", c.tips[i])
		}
		c.tipCache[c.tips[i]] = page
	}
	k := tx % per
	return int(page[0x14+k/4] >> (k % 4 * 2) & 3), nil
}

// Committed reports whether a transaction committed. Transaction 0 is the
// system transaction that creates the database.
func (c *Catalog) Committed(tx uint32) (bool, error) {
	if tx == 0 {
		return true, nil
	}
	s, err := c.TxState(tx)
	return s == TxCommitted, err
}

// Blob reads a blob stored in one of the tables.
func (c *Catalog) Blob(id record.BlobID) ([]byte, error) {
	m, ok := c.seqPages[id.Relation]
	if !ok {
		m = map[uint32]uint32{}
		// Damaged pointer pages are reported where the table's rows are
		// read; the pages found are still the ones to look in.
		pages, _ := c.DataPages(id.Relation)
		for _, p := range pages {
			rel, seq, _, err := c.rd.DataPage(p)
			if err != nil || rel != id.Relation {
				continue
			}
			m[seq] = p
		}
		c.seqPages[id.Relation] = m
	}
	return c.rd.Blob(id, func(seq uint32) (uint32, bool) { p, ok := m[seq]; return p, ok })
}

// Reader returns the record reader of the catalog's database.
func (c *Catalog) Reader() *record.Reader { return c.rd }
