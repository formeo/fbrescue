// Package scan walks every page of a database once, builds a map of the
// page structure (PIP, pointer pages, data pages, TIP) and reports anything
// that does not fit together.
package scan

import (
	"cmp"
	"encoding/binary"
	"fmt"
	"slices"

	"github.com/formeo/fbrescue/internal/fdb"
)

var le = binary.LittleEndian

// Report is the result of a scan.
type Report struct {
	Pages        uint32            `json:"pages"`
	UsedPages    uint32            `json:"used_pages"`
	FreePages    uint32            `json:"free_pages"`
	PageTypes    map[string]uint32 `json:"page_types"` // used pages by type
	Transactions *Transactions     `json:"transactions,omitempty"`
	Relations    []Relation        `json:"relations"`
	Problems     []Problem         `json:"problems"`

	// For export when the page structure is damaged: data pages in use by
	// relation, wherever they are listed, and the TIP pages in chain order.
	DataPages map[uint16][]DataPage `json:"-"`
	TIPPages  []uint32              `json:"-"`
	Misplaced map[uint32][]uint32   `json:"-"` // ODS 12+: page number -> locations holding that page
}

// DataPage is a data page in use, as found by reading it.
type DataPage struct {
	Page, Seq uint32
	Unlisted  bool // holds only fragments of large records; never in a pointer page
}

// Transactions counts transaction states recorded in the TIP for
// transactions 1..Next, Next being the last one started.
type Transactions struct {
	Next      uint64 `json:"next"`
	Committed uint64 `json:"committed"`
	Active    uint64 `json:"active"` // includes transactions whose process died
	Dead      uint64 `json:"dead"`   // rolled back
	Limbo     uint64 `json:"limbo"`  // prepared two-phase transactions
}

// Relation summarizes one table. As in gstat, only data pages reachable
// from the table's pointer pages are counted.
type Relation struct {
	ID               uint16 `json:"id"`
	FirstPointerPage uint32 `json:"first_pointer_page"`
	PointerPages     int    `json:"pointer_pages"`
	DataPages        int    `json:"data_pages"`
	Records          int    `json:"records"`  // primary record versions, deleted ones included
	Deleted          int    `json:"deleted"`  // primary versions marked deleted
	Versions         int    `json:"versions"` // back versions
}

// Problem is one inconsistency found by the scan.
type Problem struct {
	Page    uint32 `json:"page"`
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

// Problem kinds.
const (
	ReadError       = "read-error"
	MissingPIP      = "missing-pip"       // page inventory page absent or damaged
	InvalidType     = "invalid-type"      // page in use with an unknown type
	WrongPageNumber = "wrong-page-number" // ODS 12+: page holds another page's number
	BadSlots        = "bad-slots"         // slot array points outside the page
	PointerChain    = "pointer-chain"     // pointer pages of a table do not chain up
	PointerTarget   = "pointer-target"    // pointer page slot is not a data page of its table
	OrphanDataPage  = "orphan-data-page"  // data page not referenced by any pointer page
	TIPChain        = "tip-chain"         // transaction inventory incomplete
)

// Record header (rhd): transaction 4, back page 4, back line 2, flags 2, format 1.
const (
	rhdSize     = 13
	rhdFlags    = 10
	rhdDeleted  = 0x01
	rhdChain    = 0x02 // back version
	rhdFragment = 0x04
	rhdBlob     = 0x10
)

type dataPage struct {
	isData, referenced         bool
	unlisted                   bool // page flag dpg_orphan: deliberately not in a pointer page
	rel                        uint16
	seq                        uint32
	records, deleted, versions uint16
}

type pointerPage struct {
	page, seq, next uint32
	slots           []uint32
}

type scanner struct {
	db       *fdb.File
	r        *Report
	data     []dataPage // indexed by page number
	ptrs     map[uint16][]pointerPage
	tips     map[uint32]uint32 // TIP page -> next TIP page
	buf      []byte
	pipOff   int
	perPIP   uint32
	cur, nxt []byte // free-page bitmaps of the current and the next PIP range; nil if unknown
}

// Run scans the whole database. progress, if not nil, is called from time
// to time with the number of pages done.
func Run(db *fdb.File, progress func(done, total uint32)) *Report {
	s := &scanner{
		db: db,
		r: &Report{Pages: db.Pages, PageTypes: map[string]uint32{}, Relations: []Relation{}, Problems: []Problem{},
			DataPages: map[uint16][]DataPage{}, Misplaced: map[uint32][]uint32{}},
		data: make([]dataPage, db.Pages),
		ptrs: map[uint16][]pointerPage{},
		tips: map[uint32]uint32{},
		buf:  make([]byte, db.PageSize),
	}
	// A PIP is a bitmap of free pages: offset 0x14 in ODS 11, 0x1C in ODS 12+.
	s.pipOff = 0x14
	if db.Family == fdb.ODS12Plus {
		s.pipOff = 0x1C
	}
	s.perPIP = uint32(db.PageSize-s.pipOff) * 8

	for p := uint32(0); p < db.Pages; p++ {
		if progress != nil && p%8192 == 0 {
			progress(p, db.Pages)
		}
		s.page(p)
	}
	if progress != nil {
		progress(db.Pages, db.Pages)
	}
	s.link()
	s.transactions()
	return s.r
}

func (s *scanner) problem(page uint32, kind, format string, args ...any) {
	s.r.Problems = append(s.r.Problems, Problem{page, kind, fmt.Sprintf(format, args...)})
}

func (s *scanner) page(p uint32) {
	if p > 0 && p%s.perPIP == 0 {
		s.cur, s.nxt = s.nxt, nil
	}
	b := s.buf
	if err := s.db.ReadPage(p, b); err != nil {
		s.problem(p, ReadError, "%v", err)
		return
	}
	// The PIP of range 0 is page 1; the PIP of range k is the last page of range k-1.
	switch {
	case p == 1:
		s.cur = s.readPIP(p, b, 0)
	case (p+1)%s.perPIP == 0:
		s.nxt = s.readPIP(p, b, (p+1)/s.perPIP)
	}

	if !s.used(p, b) {
		s.r.FreePages++
		return
	}
	s.r.UsedPages++
	t := b[0]
	if p == 0 && t != fdb.TypeHeader {
		s.problem(p, InvalidType, "header page is damaged (page type %d)", t)
		return
	}
	if t == 0 || t > fdb.TypeMax {
		s.problem(p, InvalidType, "page is marked as used but has type %d", t)
		return
	}
	if s.db.Family == fdb.ODS12Plus && fdb.PageNumber(b) != p {
		s.problem(p, WrongPageNumber, "page holds the contents of page %d", fdb.PageNumber(b))
		s.r.Misplaced[fdb.PageNumber(b)] = append(s.r.Misplaced[fdb.PageNumber(b)], p)
		return
	}
	s.r.PageTypes[typeName(t)]++
	switch t {
	case fdb.TypeData:
		s.dataPage(p, b)
	case fdb.TypePointer:
		s.pointerPage(p, b)
	case fdb.TypeTIP:
		s.tips[p] = le.Uint32(b[0x10:])
	}
}

func (s *scanner) readPIP(p uint32, b []byte, k uint32) []byte {
	if b[0] != fdb.TypePIP {
		s.problem(p, MissingPIP, "expected the page inventory page for pages %d-%d, found page type %d; free pages in that range are guessed from page types",
			k*s.perPIP, (k+1)*s.perPIP-1, b[0])
		return nil
	}
	return slices.Clone(b[s.pipOff:])
}

func (s *scanner) used(p uint32, b []byte) bool {
	if p == 0 {
		return true
	}
	if s.cur == nil {
		return b[0] != 0
	}
	i := p % s.perPIP
	return s.cur[i/8]>>(i%8)&1 == 0
}

func (s *scanner) dataPage(p uint32, b []byte) {
	// Pages holding only fragments of records larger than a page carry the
	// flag 0x01 and are not listed in the table's pointer pages.
	d := dataPage{isData: true, seq: le.Uint32(b[0x10:]), rel: le.Uint16(b[0x14:]), unlisted: b[1]&0x01 != 0}
	n := int(le.Uint16(b[0x16:]))
	if 0x18+4*n > len(b) {
		s.problem(p, BadSlots, "data page claims %d slots, more than fit in the page", n)
		n = 0
	}
	bad := 0
	for i := 0; i < n; i++ {
		off, ln := int(le.Uint16(b[0x18+4*i:])), int(le.Uint16(b[0x1A+4*i:]))
		if ln == 0 {
			continue
		}
		if off < 0x18+4*n || off+ln > len(b) || ln < rhdSize {
			bad++
			continue
		}
		switch fl := le.Uint16(b[off+rhdFlags:]); {
		case fl&(rhdFragment|rhdBlob) != 0:
		case fl&rhdChain != 0:
			d.versions++
		default:
			d.records++
			if fl&rhdDeleted != 0 {
				d.deleted++
			}
		}
	}
	if bad > 0 {
		s.problem(p, BadSlots, "%d of %d record slots point outside the page", bad, n)
	}
	s.data[p] = d
}

func (s *scanner) pointerPage(p uint32, b []byte) {
	pp := pointerPage{page: p, seq: le.Uint32(b[0x10:]), next: le.Uint32(b[0x14:])}
	n, rel := int(le.Uint16(b[0x18:])), le.Uint16(b[0x1A:])
	if 0x20+4*n > len(b) {
		s.problem(p, BadSlots, "pointer page claims %d slots, more than fit in the page", n)
		n = 0
	}
	pp.slots = make([]uint32, n)
	for i := range pp.slots {
		pp.slots[i] = le.Uint32(b[0x20+4*i:])
	}
	s.ptrs[rel] = append(s.ptrs[rel], pp)
}

// link follows each table's pointer pages to its data pages.
func (s *scanner) link() {
	rels := make([]uint16, 0, len(s.ptrs))
	for rel := range s.ptrs {
		rels = append(rels, rel)
	}
	slices.Sort(rels)
	for _, rel := range rels {
		pps := s.ptrs[rel]
		slices.SortFunc(pps, func(a, b pointerPage) int { return cmp.Compare(a.seq, b.seq) })
		r := Relation{ID: rel, PointerPages: len(pps)}
		if pps[0].seq == 0 {
			r.FirstPointerPage = pps[0].page
		} else {
			s.problem(pps[0].page, PointerChain, "table %d: first pointer page found has sequence %d; earlier ones are missing", rel, pps[0].seq)
		}
		for i, pp := range pps {
			if i > 0 && pp.seq != pps[i-1].seq+1 {
				s.problem(pp.page, PointerChain, "table %d: pointer page sequence jumps from %d to %d", rel, pps[i-1].seq, pp.seq)
			}
			var want uint32
			if i+1 < len(pps) {
				want = pps[i+1].page
			}
			if pp.next != want {
				s.problem(pp.page, PointerChain, "table %d: pointer page links to page %d, expected %d", rel, pp.next, want)
			}
			s.linkSlots(&r, pp)
		}
		s.r.Relations = append(s.r.Relations, r)
	}
	for p := range s.data {
		d := s.data[p]
		if !d.isData {
			continue
		}
		s.r.DataPages[d.rel] = append(s.r.DataPages[d.rel], DataPage{Page: uint32(p), Seq: d.seq, Unlisted: d.unlisted})
		if !d.referenced && !d.unlisted {
			s.problem(uint32(p), OrphanDataPage, "data page of table %d is not referenced by any pointer page", d.rel)
		}
	}
}

func (s *scanner) linkSlots(r *Relation, pp pointerPage) {
	var base uint32
	haveBase := false
	for i, p := range pp.slots {
		if p == 0 {
			continue
		}
		if p >= uint32(len(s.data)) || !s.data[p].isData {
			s.problem(pp.page, PointerTarget, "table %d, slot %d: page %d is not a readable data page", r.ID, i, p)
			continue
		}
		d := &s.data[p]
		switch {
		case d.rel != r.ID:
			s.problem(pp.page, PointerTarget, "table %d, slot %d: page %d belongs to table %d", r.ID, i, p, d.rel)
			continue
		case d.referenced:
			s.problem(pp.page, PointerTarget, "table %d, slot %d: page %d is referenced twice", r.ID, i, p)
			continue
		}
		// Data page sequence = pointer page sequence * pages per pointer
		// page + slot; checked as a constant base within one pointer page.
		if b := d.seq - uint32(i); !haveBase {
			base, haveBase = b, true
		} else if b != base {
			s.problem(p, PointerTarget, "table %d: data page sequence %d does not match slot %d of pointer page %d", r.ID, d.seq, i, pp.page)
		}
		d.referenced = true
		r.DataPages++
		r.Records += int(d.records)
		r.Deleted += int(d.deleted)
		r.Versions += int(d.versions)
	}
}

// transactions follows the TIP chain and counts transaction states,
// two bits per transaction starting at offset 0x14.
func (s *scanner) transactions() {
	pointed := map[uint32]bool{}
	for _, next := range s.tips {
		pointed[next] = true
	}
	var heads []uint32
	for p := range s.tips {
		if !pointed[p] {
			heads = append(heads, p)
		}
	}
	slices.Sort(heads)
	if len(heads) != 1 {
		s.problem(0, TIPChain, "expected one chain of transaction inventory pages, found %d (starting at %v)", len(heads), heads)
		if len(heads) == 0 {
			return
		}
	}
	var chain []uint32
	seen := map[uint32]bool{}
	for p := heads[0]; p != 0 && !seen[p]; p = s.tips[p] {
		if _, ok := s.tips[p]; !ok {
			s.problem(p, TIPChain, "transaction inventory chain continues to page %d, which is not a TIP page", p)
			break
		}
		seen[p] = true
		chain = append(chain, p)
	}
	s.r.TIPPages = chain
	h := s.db.Header
	if h == nil {
		return
	}

	per := uint64(s.db.PageSize-0x14) * 4
	last := h.NextTx
	if covered := uint64(len(chain)) * per; last >= covered {
		s.problem(chain[len(chain)-1], TIPChain, "transaction inventory covers transactions up to %d, but transaction %d was started", covered-1, h.NextTx)
		last = covered - 1
	}
	t := &Transactions{Next: h.NextTx}
	for tx := uint64(1); tx <= last; tx++ {
		if tx == 1 || tx%per == 0 {
			if err := s.db.ReadPage(chain[tx/per], s.buf); err != nil {
				s.problem(chain[tx/per], ReadError, "%v", err)
				return
			}
		}
		i := tx % per
		switch s.buf[0x14+i/4] >> (i % 4 * 2) & 3 {
		case 0:
			t.Active++
		case 1:
			t.Limbo++
		case 2:
			t.Dead++
		case 3:
			t.Committed++
		}
	}
	s.r.Transactions = t
}

func typeName(t byte) string {
	switch t {
	case fdb.TypeHeader:
		return "header"
	case fdb.TypePIP:
		return "page inventory"
	case fdb.TypeTIP:
		return "transaction inventory"
	case fdb.TypePointer:
		return "pointer"
	case fdb.TypeData:
		return "data"
	case fdb.TypeIndexRoot:
		return "index root"
	case fdb.TypeBTree:
		return "index"
	case fdb.TypeBlob:
		return "blob"
	case fdb.TypeGenerator:
		return "generator"
	}
	return fmt.Sprintf("type %d", t)
}
