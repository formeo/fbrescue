package meta

import (
	"testing"

	"github.com/formeo/fbrescue/internal/corpus"
	"github.com/formeo/fbrescue/internal/fdb"
	"github.com/formeo/fbrescue/internal/record"
)

// TestCatalogMatchesGstat: on every corpus database the catalog loads
// without warnings, finds the same user tables as gstat -r (relation ids and
// names), and every primary record of every table, system tables included,
// reads and unpacks to at least the length of its format.
func TestCatalogMatchesGstat(t *testing.T) {
	for _, db := range corpus.List(t) {
		t.Run(db.Engine+"/"+db.Name, func(t *testing.T) {
			f, err := fdb.Open(db.Path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			if db.Name == "hiwords" {
				return // header patched to transaction numbers the inventory never saw
			}
			c, err := Load(f)
			if err != nil {
				t.Fatal(err)
			}
			for _, w := range c.Warnings {
				t.Errorf("warning: %s", w)
			}

			if want, ok := db.Relations(t); ok {
				got := map[uint16]string{}
				for _, tb := range c.Tables {
					got[tb.ID] = tb.Name
				}
				for id, r := range want {
					if id >= 128 && got[id] != r.Name {
						t.Errorf("table %d: got %q, gstat says %q", id, got[id], r.Name)
					}
				}
				for id, name := range got {
					if _, ok := want[id]; !ok {
						t.Errorf("table %d %s is not in gstat", id, name)
					}
				}
			}

			rels := []uint16{relPages, relRelationFields, relRelations, relFormats}
			for _, tb := range c.Tables {
				rels = append(rels, tb.ID)
			}
			for _, rel := range rels {
				var formats map[uint16]*Format
				for _, tb := range c.Tables {
					if tb.ID == rel {
						formats = tb.Formats
					}
				}
				err := c.Records(rel, func(v Version, rec []byte, err error) error {
					if err != nil {
						t.Errorf("relation %d page %d line %d: %v", rel, v.Page, v.Line, err)
						return nil
					}
					if formats == nil || v.Header.Flags&record.FlagDeleted != 0 {
						return nil
					}
					fm, ok := formats[uint16(v.Header.Format)]
					if !ok {
						t.Errorf("relation %d page %d line %d: no format %d", rel, v.Page, v.Line, v.Header.Format)
						return nil
					}
					end := 0
					for _, fl := range fm.Fields {
						if fl.Stored() {
							end = max(end, int(fl.Offset)+int(fl.Length))
						}
					}
					if len(rec) < end {
						t.Errorf("relation %d page %d line %d: %d bytes, format %d needs %d", rel, v.Page, v.Line, len(rec), v.Header.Format, end)
					}
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

// TestRolledBackVersions: the layout database keeps the record versions
// of a rolled-back transaction (see tools/mkcorpus/sql/layout.sql).
func TestRolledBackVersions(t *testing.T) {
	for _, db := range corpus.List(t) {
		if db.Name != "layout" {
			continue
		}
		t.Run(db.Engine, func(t *testing.T) {
			f, err := fdb.Open(db.Path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			c, err := Load(f)
			if err != nil {
				t.Fatal(err)
			}
			var rb *Table
			for _, tb := range c.Tables {
				if tb.Name == "RB" {
					rb = tb
				}
			}
			var dead, deadWithOlder int
			c.Records(rb.ID, func(v Version, rec []byte, err error) error {
				if ok, _ := c.Committed(v.Header.Tx); !ok {
					dead++
					if v.Header.BackPage != 0 {
						deadWithOlder++
					}
				}
				return nil
			})
			// insert of 100 (no older version), update of 1 and delete of 2
			if dead != 3 || deadWithOlder != 2 {
				t.Errorf("uncommitted versions: %d, with an older version: %d; want 3 and 2", dead, deadWithOlder)
			}
		})
	}
}
