package meta

import (
	"errors"
	"fmt"

	"github.com/formeo/fbrescue/internal/record"
)

// Row is a version of a row chosen from its version chain.
type Row struct {
	Page    uint32
	Line    int
	Header  record.Header
	Data    []byte // unpacked record; nil for a delete stub
	Deleted bool   // the version is a delete stub: the row was deleted
	Older   bool   // newer versions exist but did not commit
	Limbo   bool   // a newer version belongs to a limbo (prepared) transaction
}

const maxVersions = 1 << 16

// Current returns the newest committed version of the row whose primary
// version is v (as passed to the Records callback): what a new transaction
// would read. ok is false when no version committed, which is an insert
// that never happened.
func (c *Catalog) Current(v Version, rec []byte) (Row, bool, error) {
	r := Row{Page: v.Page, Line: v.Line, Header: v.Header, Data: rec, Deleted: v.Header.Flags&record.FlagDeleted != 0}
	return c.committedFrom(r)
}

// BeforeDelete returns the version of a deleted row as it was before the
// delete, if it is still on disk.
func (c *Catalog) BeforeDelete(stub Row) (Row, bool, error) {
	if stub.Header.BackPage == 0 {
		return Row{}, false, nil
	}
	r, err := c.back(stub)
	if err != nil {
		return Row{}, false, err
	}
	return c.committedFrom(r)
}

// committedFrom walks the chain from r to the first committed version.
func (c *Catalog) committedFrom(r Row) (Row, bool, error) {
	var older, limbo bool
	for n := 0; ; n++ {
		if r.Header.Flags&record.FlagLongTx != 0 {
			return Row{}, false, errors.New("48-bit transaction number: not supported yet")
		}
		if r.Header.Tx != 0 {
			s, err := c.TxState(r.Header.Tx)
			if err != nil {
				return Row{}, false, err
			}
			if s == TxCommitted {
				r.Older, r.Limbo = older, limbo
				return r, true, nil
			}
			limbo = limbo || s == TxLimbo
		} else {
			r.Older, r.Limbo = older, limbo
			return r, true, nil
		}
		if r.Header.BackPage == 0 {
			return Row{}, false, nil
		}
		if n >= maxVersions {
			return Row{}, false, errors.New("version chain does not end")
		}
		next, err := c.back(r)
		if err != nil {
			return Row{}, false, err
		}
		r, older = next, true
	}
}

// back reads the version r's back pointer leads to, rebuilding it from
// r when it is stored as differences.
func (c *Catalog) back(r Row) (Row, error) {
	p, l := r.Header.BackPage, int(r.Header.BackLine)
	h, data, err := c.rd.Read(p, l)
	if err != nil {
		return Row{}, fmt.Errorf("older version at page %d line %d: %w", p, l, err)
	}
	if h.Flags&record.FlagChain == 0 {
		return Row{}, fmt.Errorf("page %d line %d should hold an older version of the row (flags %#x)", p, l, h.Flags)
	}
	if r.Header.Flags&record.FlagDelta != 0 {
		if r.Data == nil {
			return Row{}, fmt.Errorf("older version at page %d line %d is stored as differences from a delete stub", p, l)
		}
		if data, err = record.ApplyDelta(r.Data, data); err != nil {
			return Row{}, fmt.Errorf("older version at page %d line %d: %w", p, l, err)
		}
	}
	deleted := h.Flags&record.FlagDeleted != 0
	if deleted {
		data = nil
	}
	return Row{Page: p, Line: l, Header: h, Data: data, Deleted: deleted}, nil
}
