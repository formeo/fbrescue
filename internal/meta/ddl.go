package meta

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/formeo/fbrescue/internal/ddl"
	"github.com/formeo/fbrescue/internal/record"
)

// UseDDL takes table layouts from user-supplied DDL for the tables whose
// own definition is lost, and names for tables exported as TABLE_<id>.
// A definition is matched to a table by name when RDB$RELATIONS still has
// it, otherwise by its layout: for a table whose record format is lost, the
// length of its records; for one exported as TABLE_<id>, the format itself.
// Tables the catalog describes are left as they are.
func (c *Catalog) UseDDL(defs []*ddl.Table) {
	byName := map[string]*Table{}
	for _, t := range c.Tables {
		if !t.Lost {
			byName[t.Name] = t
		}
	}
	var pending []*ddl.Table
	for _, d := range defs {
		if _, ok := byName[d.Name]; !ok {
			pending = append(pending, d)
		}
	}
	if len(pending) == 0 {
		return
	}

	// Tables whose format is lost: by name first.
	profiles := map[uint16]profile{}
	for _, id := range slices.Sorted(maps.Keys(c.noFormat)) {
		profiles[id] = c.profile(id)
	}
	used := map[*ddl.Table]bool{}
	for _, d := range pending {
		for id, name := range c.noFormat {
			if name == "" || name != d.Name {
				continue
			}
			used[d] = true
			if p := profiles[id]; !p.fits(d) {
				c.warn("--ddl: table %s: its records are %s bytes, the DDL gives %d; the table is skipped", d.Name, p.lengths(), d.Length())
				break
			}
			c.addDDLTable(id, d, profiles[id].format)
			delete(c.noFormat, id)
			break
		}
	}

	// The rest by layout: a definition and a table that only fit each other.
	var lost []*Table
	for _, t := range c.Tables {
		if t.Lost {
			lost = append(lost, t)
		}
	}
	fitting := func(d *ddl.Table) (ids []uint16, tables []*Table) {
		for id := range c.noFormat {
			if profiles[id].fits(d) {
				ids = append(ids, id)
			}
		}
		for _, t := range lost {
			if sameLayout(t.Formats[t.CurrentFormat], d) {
				tables = append(tables, t)
			}
		}
		return ids, tables
	}
	defsFitting := func(match func(d *ddl.Table) bool) int {
		n := 0
		for _, d := range pending {
			if !used[d] && match(d) {
				n++
			}
		}
		return n
	}
	for _, d := range pending {
		if used[d] {
			continue
		}
		ids, tables := fitting(d)
		switch {
		case len(ids)+len(tables) == 0:
			c.warn("--ddl: table %s matches none of the tables whose definition is lost", d.Name)
		case len(ids)+len(tables) > 1:
			c.warn("--ddl: table %s fits %d of the tables whose definition is lost; it is not used", d.Name, len(ids)+len(tables))
		case len(ids) == 1:
			p := profiles[ids[0]]
			if n := defsFitting(p.fits); n > 1 {
				c.warn("--ddl: table %d fits %d of the DDL tables; it is skipped", ids[0], n)
				continue
			}
			c.addDDLTable(ids[0], d, p.format)
			delete(c.noFormat, ids[0])
			used[d] = true
		default:
			t := tables[0]
			if n := defsFitting(func(x *ddl.Table) bool { return sameLayout(t.Formats[t.CurrentFormat], x) }); n > 1 {
				c.warn("--ddl: %s fits %d of the DDL tables; its name stays", t.Name, n)
				continue
			}
			c.warn("--ddl: %s has the layout of %s; exported under that name", t.Name, d.Name)
			t.Name, t.Lost = d.Name, false
			for _, col := range t.Columns {
				col.Name = d.Columns[col.Field.ID].Name
			}
			used[d] = true
		}
	}
	slices.SortFunc(c.Tables, func(a, b *Table) int { return int(a.ID) - int(b.ID) })
}

func (c *Catalog) addDDLTable(id uint16, d *ddl.Table, format uint16) {
	t := &Table{ID: id, Name: d.Name, Formats: map[uint16]*Format{format: {Fields: d.Fields()}}, CurrentFormat: format, FromDDL: true}
	for i, dc := range d.Columns {
		t.Columns = append(t.Columns, &Column{Name: dc.Name, Position: i, NotNull: dc.NotNull, Field: dc.Field, Computed: dc.Computed})
	}
	c.Tables = append(c.Tables, t)
	c.warn("--ddl: table %s (relation %d) is read with the layout from the DDL, for records of format %d; check the values", d.Name, id, format)
}

// sameLayout compares a stored format with a DDL layout.
func sameLayout(fm *Format, d *ddl.Table) bool {
	got := d.Fields()
	if fm == nil || len(fm.Fields) != len(got) {
		return false
	}
	for i, f := range fm.Fields {
		g := got[i]
		if f.Stored() != (g.Offset != 0) || f.Stored() && (f.DType != g.DType || f.Length != g.Length || f.Offset != g.Offset || f.Scale != g.Scale) {
			return false
		}
	}
	return true
}

// profile is what a sample of a table's records says about its layout.
type profile struct {
	format uint16       // the newest record format seen
	sizes  map[int]bool // lengths of the records of that format
}

const profileSample = 200

var errEnough = errors.New("enough")

func (c *Catalog) profile(rel uint16) profile {
	p := profile{sizes: map[int]bool{}}
	byFormat := map[uint16]map[int]bool{}
	n := 0
	c.Records(rel, func(v Version, rec []byte, err error) error {
		if err != nil || rec == nil || v.Header.Flags&record.FlagDeleted != 0 {
			return nil
		}
		f := uint16(v.Header.Format)
		if byFormat[f] == nil {
			byFormat[f] = map[int]bool{}
		}
		byFormat[f][len(rec)] = true
		if n++; n >= profileSample {
			return errEnough
		}
		return nil
	})
	for f, sizes := range byFormat {
		if f >= p.format {
			p.format, p.sizes = f, sizes
		}
	}
	return p
}

// fits: every record of the newest format is as long as the layout. Firebird
// 5 stores records it does not compress with one byte more.
func (p profile) fits(d *ddl.Table) bool {
	if len(p.sizes) == 0 {
		return false
	}
	l := d.Length()
	for n := range p.sizes {
		if n != l && n != l+1 {
			return false
		}
	}
	return true
}

func (p profile) lengths() string {
	var s []string
	for _, n := range slices.Sorted(maps.Keys(p.sizes)) {
		s = append(s, fmt.Sprint(n))
	}
	if len(s) == 0 {
		return "unknown (no readable records)"
	}
	return strings.Join(s, " or ")
}
