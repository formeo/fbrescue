// Package meta reads the system tables a data export needs: RDB$PAGES
// (where every table's pointer pages and the transaction inventory are),
// RDB$FORMATS (the physical layout of each table's records), RDB$RELATIONS
// and RDB$RELATION_FIELDS (names and column order).
package meta

import (
	"cmp"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/formeo/fbrescue/internal/fdb"
	"github.com/formeo/fbrescue/internal/record"
	"github.com/formeo/fbrescue/internal/scan"
	"github.com/formeo/fbrescue/internal/value"
)

var le = binary.LittleEndian

// Catalog is what export needs to know about a database.
type Catalog struct {
	Tables   []*Table // persistent user tables, by relation id
	Warnings []string

	db       *fdb.File
	rd       *record.Reader
	pointers map[uint16][]pointerPage // relation -> pointer pages by sequence
	tips     []uint32                 // transaction inventory pages by sequence
	tipCache map[uint32][]byte
	seqPages map[uint16]map[uint32]uint32 // relation -> data page sequence -> page

	// One pass over all pages, for what the structure no longer tells.
	pageScan  *scan.Report
	pageSeq   map[uint32]relSeq // data page -> its relation and sequence number
	dataPages map[uint16][]uint32
	dataErr   map[uint16]error
}

// Table is a user table.
type Table struct {
	ID      uint16
	Name    string
	Columns []*Column // in the order of SELECT *
	Formats map[uint16]*Format
}

// Column is a column of a table as of its current format.
type Column struct {
	Name     string
	Position int
	NotNull  bool
	Field    value.Field // physical type in the current format
	Computed bool        // COMPUTED BY: no stored value

	// Default is the value of this column in records written before it
	// was added, if the engine reads one there (NOT NULL with a default).
	HasDefault bool
	Default    any
	DefaultErr error // the default exists but could not be evaluated
}

// Format is one version of a table's record layout.
type Format struct {
	Fields   []value.Field
	Defaults map[int]any // ODS 12+: values for fields missing from older records
}

type relSeq struct {
	rel uint16
	seq uint32
}

type pointerPage struct {
	seq, page uint32
}

// Load reads the catalog. Damage to the page structure (pointer pages,
// RDB$PAGES, the transaction inventory, the header page) is worked around
// where the pages themselves tell enough, with a warning.
func Load(db *fdb.File) (*Catalog, error) {
	rep := scan.Run(db, nil)
	moved := relocate(db, rep)
	if moved > 0 {
		rep = scan.Run(db, nil) // with the pages in their places
	}
	var c *Catalog
	var err error
	if db.Header == nil {
		c, err = guessHeader(db, rep)
	} else {
		c, err = load(db, rep)
	}
	if err == nil && moved > 0 {
		c.Warnings = append([]string{fmt.Sprintf("%d pages are stored at the location of another page; they are read from there", moved)}, c.Warnings...)
	}
	return c, err
}

func load(db *fdb.File, rep *scan.Report) (*Catalog, error) {
	c := &Catalog{
		db: db, rd: record.NewReader(db),
		pointers: map[uint16][]pointerPage{}, tipCache: map[uint32][]byte{},
		seqPages: map[uint16]map[uint32]uint32{},
		pageScan: rep, pageSeq: map[uint32]relSeq{}, dataPages: map[uint16][]uint32{}, dataErr: map[uint16]error{},
	}
	for rel, dps := range rep.DataPages {
		for _, d := range dps {
			c.pageSeq[d.Page] = relSeq{rel, d.Seq}
		}
	}
	if err := c.loadPages(); err != nil {
		return nil, fmt.Errorf("RDB$PAGES: %w", err)
	}
	formats, err := c.loadFormats()
	if err != nil {
		return nil, fmt.Errorf("RDB$FORMATS: %w", err)
	}
	if err := c.loadTables(formats); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Catalog) warn(format string, args ...any) {
	c.Warnings = append(c.Warnings, fmt.Sprintf(format, args...))
}

// loadPages reads RDB$PAGES, starting from its first pointer page named in
// the header, the way the engine finds everything else.
func (c *Catalog) loadPages() error {
	buf := make([]byte, c.db.PageSize)
	seen := map[uint32]bool{}
	var seq uint32
	// A break in the chain is not fatal: DataPages finds the rest of the
	// data pages of RDB$PAGES by scanning.
	for p := c.db.Header.PagesPointer; p != 0; seq++ {
		if seen[p] {
			c.warn("RDB$PAGES: pointer page chain loops at page %d", p)
			break
		}
		seen[p] = true
		if err := c.db.ReadPage(p, buf); err != nil {
			c.warn("RDB$PAGES: %v", err)
			break
		}
		if buf[0] != fdb.TypePointer || le.Uint16(buf[0x1A:]) != relPages {
			c.warn("RDB$PAGES: page %d should be a pointer page of RDB$PAGES", p)
			break
		}
		c.pointers[relPages] = append(c.pointers[relPages], pointerPage{seq, p})
		p = le.Uint32(buf[0x14:])
	}

	f := systemFormat(relPages, c.db)
	type tip struct{ seq, page uint32 }
	var tips []tip
	// RDB$PAGES is maintained by the system transaction: every primary
	// version is current.
	err := c.Records(relPages, func(v Version, rec []byte, err error) error {
		if err != nil {
			c.warn("RDB$PAGES: %v", err)
			return nil
		}
		if v.Header.Flags&record.FlagDeleted != 0 {
			return nil
		}
		page, rel, pseq, typ := intField(rec, f["PAGE_NUMBER"]), intField(rec, f["RELATION_ID"]),
			intField(rec, f["PAGE_SEQUENCE"]), intField(rec, f["PAGE_TYPE"])
		switch typ {
		case fdb.TypePointer:
			if rel != relPages {
				c.pointers[uint16(rel)] = append(c.pointers[uint16(rel)], pointerPage{uint32(pseq), uint32(page)})
			}
		case fdb.TypeTIP:
			tips = append(tips, tip{uint32(pseq), uint32(page)})
		}
		return nil
	})
	if err != nil {
		return err
	}
	for rel, pps := range c.pointers {
		slices.SortFunc(pps, func(a, b pointerPage) int { return cmp.Compare(a.seq, b.seq) })
		c.pointers[rel] = pps
	}
	slices.SortFunc(tips, func(a, b tip) int { return cmp.Compare(a.seq, b.seq) })
	for i, t := range tips {
		if t.seq != uint32(i) {
			break
		}
		c.tips = append(c.tips, t.page)
	}
	if len(c.tips) < len(tips) || len(tips) == 0 {
		if len(c.pageScan.TIPPages) > len(c.tips) {
			c.warn("RDB$PAGES lists %d of the transaction inventory pages; the chain of %d found by scanning is used", len(c.tips), len(c.pageScan.TIPPages))
			c.tips = c.pageScan.TIPPages
		} else {
			c.warn("transaction inventory page %d is missing", len(c.tips))
		}
	}
	return nil
}

// intField reads a SMALLINT or INTEGER system field; NULL reads as -1.
func intField(rec []byte, f value.Field) int64 {
	v, err := value.Decode(rec, f, value.Options{})
	if d, ok := v.(value.Decimal); ok && err == nil {
		return d.Int
	}
	return -1
}

func textField(rec []byte, f value.Field) string {
	v, err := value.Decode(rec, f, value.Options{})
	if t, ok := v.(value.Text); ok && err == nil {
		return strings.TrimRight(t.S, " ")
	}
	return ""
}

func blobField(rec []byte, f value.Field) (record.BlobID, bool) {
	if value.IsNull(rec, f) || int(f.Offset)+8 > len(rec) {
		return record.BlobID{}, false
	}
	return record.ParseBlobID(rec[f.Offset:]), true
}

type formatKey struct{ rel, format uint16 }

func (c *Catalog) loadFormats() (map[formatKey]*Format, error) {
	f := systemFormat(relFormats, c.db)
	out := map[formatKey]*Format{}
	err := c.visible(relFormats, func(v Version, rec []byte) {
		rel, num := intField(rec, f["RELATION_ID"]), intField(rec, f["FORMAT"])
		id, ok := blobField(rec, f["DESCRIPTOR"])
		if rel < 0 || num < 0 || !ok {
			c.warn("RDB$FORMATS: incomplete row at page %d line %d", v.Page, v.Line)
			return
		}
		b, err := c.Blob(id)
		if err != nil {
			c.warn("RDB$FORMATS: format %d of table %d: %v", num, rel, err)
			return
		}
		fm, err := parseFormat(b, c.db.Header.ODSMajor)
		if err != nil {
			c.warn("RDB$FORMATS: format %d of table %d: %v", num, rel, err)
			return
		}
		out[formatKey{uint16(rel), uint16(num)}] = fm
	})
	return out, err
}

// parseFormat decodes an RDB$FORMATS descriptor: in ODS 11 an array of
// 12-byte field descriptors; from ODS 12 on a count first, then the array,
// then default values for fields added later.
func parseFormat(b []byte, ods int) (*Format, error) {
	fm := &Format{}
	if ods < 12 {
		if len(b)%value.DescriptorSize != 0 {
			return nil, fmt.Errorf("descriptor of %d bytes is not a whole number of fields", len(b))
		}
		for i := 0; i*value.DescriptorSize < len(b); i++ {
			fm.Fields = append(fm.Fields, value.ParseField(i, b[i*value.DescriptorSize:]))
		}
		return fm, nil
	}
	if len(b) < 2 {
		return nil, errors.New("descriptor is empty")
	}
	n, o := int(le.Uint16(b)), 2
	if o+n*value.DescriptorSize > len(b) {
		return nil, fmt.Errorf("descriptor claims %d fields, has room for %d", n, (len(b)-o)/value.DescriptorSize)
	}
	for i := 0; i < n; i++ {
		fm.Fields = append(fm.Fields, value.ParseField(i, b[o:]))
		o += value.DescriptorSize
	}
	if o+2 > len(b) {
		return fm, nil
	}
	nd := int(le.Uint16(b[o:]))
	o += 2
	for i := 0; i < nd; i++ {
		if o+2+value.DescriptorSize > len(b) {
			return nil, errors.New("default values are truncated")
		}
		id := int(le.Uint16(b[o:]))
		f := value.ParseField(id, b[o+2:])
		o += 2 + value.DescriptorSize
		if o+int(f.Length) > len(b) {
			return nil, errors.New("default value is truncated")
		}
		v, err := value.DecodeBytes(b[o:o+int(f.Length)], f, value.Options{})
		o += int(f.Length)
		if err != nil {
			return nil, fmt.Errorf("default value of field %d: %w", id, err)
		}
		if fm.Defaults == nil {
			fm.Defaults = map[int]any{}
		}
		fm.Defaults[id] = v
	}
	return fm, nil
}

type relation struct {
	id      uint16
	name    string
	format  uint16
	regular bool // persistent user table: not a view, external, virtual or temporary table
}

type relField struct {
	name     string
	position int
	id       int
	notNull  bool
	dflt     record.BlobID
	hasDflt  bool
}

func (c *Catalog) loadTables(formats map[formatKey]*Format) error {
	fr := systemFormat(relRelations, c.db)
	var rels []relation
	err := c.visible(relRelations, func(v Version, rec []byte) {
		id, sys := intField(rec, fr["RELATION_ID"]), intField(rec, fr["SYSTEM_FLAG"])
		typ, fmtNo := intField(rec, fr["RELATION_TYPE"]), intField(rec, fr["FORMAT"])
		if id < 0 || sys > 0 {
			return
		}
		_, view := blobField(rec, fr["VIEW_BLR"])
		external := !value.IsNull(rec, fr["EXTERNAL_FILE"])
		rels = append(rels, relation{
			id: uint16(id), name: textField(rec, fr["RELATION_NAME"]), format: uint16(max(fmtNo, 0)),
			regular: !view && !external && typ <= 0,
		})
	})
	if err != nil {
		return fmt.Errorf("RDB$RELATIONS: %w", err)
	}

	ff := systemFormat(relRelationFields, c.db)
	fields := map[string][]relField{}
	err = c.visible(relRelationFields, func(v Version, rec []byte) {
		rel := textField(rec, ff["RELATION_NAME"])
		d, hasD := blobField(rec, ff["DEFAULT_VALUE"])
		fields[rel] = append(fields[rel], relField{
			name: textField(rec, ff["FIELD_NAME"]), position: int(intField(rec, ff["FIELD_POSITION"])),
			id: int(intField(rec, ff["FIELD_ID"])), notNull: intField(rec, ff["NULL_FLAG"]) == 1,
			dflt: d, hasDflt: hasD,
		})
	})
	if err != nil {
		return fmt.Errorf("RDB$RELATION_FIELDS: %w", err)
	}

	rels = append(rels, c.lostRelations(rels, formats)...)
	slices.SortFunc(rels, func(a, b relation) int { return cmp.Compare(a.id, b.id) })
	for _, r := range rels {
		if !r.regular {
			continue
		}
		t := &Table{ID: r.id, Name: r.name, Formats: map[uint16]*Format{}}
		for k, fm := range formats {
			if k.rel == r.id {
				t.Formats[k.format] = fm
			}
		}
		cur, ok := t.Formats[r.format]
		if !ok {
			c.warn("table %s: current format %d is missing from RDB$FORMATS; the table is skipped", r.name, r.format)
			continue
		}
		rfs := fields[r.name]
		slices.SortFunc(rfs, func(a, b relField) int { return cmp.Or(cmp.Compare(a.position, b.position), cmp.Compare(a.id, b.id)) })
		for _, rf := range rfs {
			if rf.id < 0 || rf.id >= len(cur.Fields) {
				c.warn("table %s: column %s has field id %d, not in the current format", r.name, rf.name, rf.id)
				continue
			}
			col := &Column{Name: rf.name, Position: rf.position, NotNull: rf.notNull, Field: cur.Fields[rf.id]}
			col.Computed = col.Field.DType != 0 && col.Field.Offset == 0
			c.setDefault(col, cur, rf)
			t.Columns = append(t.Columns, col)
		}
		c.lostColumns(t, cur)
		c.Tables = append(c.Tables, t)
	}
	return nil
}

// setDefault finds the value the engine reads for this column in records
// written before the column existed: ODS 12+ keeps it in the format; ODS 11
// evaluates the column's default for NOT NULL columns.
func (c *Catalog) setDefault(col *Column, cur *Format, rf relField) {
	if c.db.Header.ODSMajor >= 12 {
		if v, ok := cur.Defaults[col.Field.ID]; ok {
			col.HasDefault = true
			col.Default, col.DefaultErr = value.Convert(v, col.Field)
		}
		return
	}
	if !rf.notNull || !rf.hasDflt {
		return
	}
	col.HasDefault = true
	blr, err := c.Blob(rf.dflt)
	if err != nil {
		col.DefaultErr = err
		return
	}
	v, err := parseLiteral(blr)
	if err != nil {
		col.DefaultErr = err
		return
	}
	col.Default, col.DefaultErr = value.Convert(v, col.Field)
}

// parseLiteral decodes a default value stored as BLR: blr_version, then
// blr_literal with its type and value. Expressions are not evaluated.
func parseLiteral(b []byte) (any, error) {
	const (
		blrVersion4, blrVersion5, blrLiteral = 4, 5, 21
		blrText, blrText2, blrShort, blrLong = 14, 15, 7, 8
		blrInt64, blrFloat, blrDouble        = 16, 10, 27
		blrDate, blrTime, blrTimestamp       = 12, 13, 35
		blrBool                              = 23
	)
	bad := errors.New("default value is an expression, not a literal; not evaluated")
	if len(b) < 3 || (b[0] != blrVersion4 && b[0] != blrVersion5) || b[1] != blrLiteral {
		return nil, bad
	}
	b = b[2:]
	need := func(n int) error {
		if len(b) < n {
			return errors.New("default value is truncated")
		}
		return nil
	}
	f := value.Field{}
	var raw []byte
	switch t := b[0]; t {
	case blrShort, blrLong, blrInt64:
		size := map[byte]int{blrShort: 2, blrLong: 4, blrInt64: 8}[t]
		if err := need(2 + size); err != nil {
			return nil, err
		}
		f.DType = map[byte]uint8{blrShort: value.TypeShort, blrLong: value.TypeLong, blrInt64: value.TypeInt64}[t]
		f.Scale, f.Length, raw = int8(b[1]), uint16(size), b[2:2+size]
	case blrFloat, blrDouble, blrDate, blrTime, blrTimestamp, blrBool:
		size := map[byte]int{blrFloat: 4, blrDouble: 8, blrDate: 4, blrTime: 4, blrTimestamp: 8, blrBool: 1}[t]
		if err := need(1 + size); err != nil {
			return nil, err
		}
		f.DType = map[byte]uint8{blrFloat: value.TypeReal, blrDouble: value.TypeDouble, blrDate: value.TypeDate,
			blrTime: value.TypeTime, blrTimestamp: value.TypeTimestamp, blrBool: value.TypeBoolean}[t]
		f.Length, raw = uint16(size), b[1:1+size]
	case blrText, blrText2:
		o := 1
		if t == blrText2 {
			if err := need(3); err != nil {
				return nil, err
			}
			f.SubType = int16(le.Uint16(b[1:]))
			o = 3
		}
		if err := need(o + 2); err != nil {
			return nil, err
		}
		n := int(le.Uint16(b[o:]))
		if err := need(o + 2 + n); err != nil {
			return nil, err
		}
		f.DType, f.Length, raw = value.TypeText, uint16(n), b[o+2:o+2+n]
	default:
		return nil, bad
	}
	return value.DecodeBytes(raw, f, value.Options{})
}

// lostRelations makes up tables for relations that have record formats and
// data pages but no row in RDB$RELATIONS: lost to damage, since the engine
// deletes the formats and the data of a dropped table.
func (c *Catalog) lostRelations(rels []relation, formats map[formatKey]*Format) []relation {
	known := map[uint16]bool{}
	for _, r := range rels {
		known[r.id] = true
	}
	latest := map[uint16]uint16{}
	for k := range formats {
		if k.rel >= firstUserRelation && !known[k.rel] && k.format >= latest[k.rel] {
			latest[k.rel] = k.format
		}
	}
	var out []relation
	for id, f := range latest {
		if len(c.pageScan.DataPages[id]) == 0 && len(c.pointers[id]) == 0 {
			continue // a view or a temporary table: nothing to export
		}
		r := relation{id: id, name: fmt.Sprintf("TABLE_%d", id), format: f, regular: true}
		c.warn("table %d is missing from RDB$RELATIONS but has data: exported as %s, with its newest format (%d)", id, r.name, f)
		out = append(out, r)
	}
	return out
}

// firstUserRelation is the relation id of the first user table.
const firstUserRelation = 128

// lostColumns adds columns for stored fields of the current format that
// RDB$RELATION_FIELDS does not name, as FIELD_<id>.
func (c *Catalog) lostColumns(t *Table, cur *Format) {
	named := map[int]bool{}
	for _, col := range t.Columns {
		named[col.Field.ID] = true
	}
	var lost []string
	for _, f := range cur.Fields {
		if !f.Stored() || named[f.ID] {
			continue
		}
		col := &Column{Name: fmt.Sprintf("FIELD_%d", f.ID), Position: len(t.Columns), Field: f}
		t.Columns = append(t.Columns, col)
		lost = append(lost, col.Name)
	}
	if len(lost) > 0 {
		c.warn("table %s: %d columns are missing from RDB$RELATION_FIELDS and are exported as %s; their names, order and defaults are lost",
			t.Name, len(lost), strings.Join(lost, ", "))
	}
}
