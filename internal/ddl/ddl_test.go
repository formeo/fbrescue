package ddl_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/formeo/fbrescue/internal/corpus"
	"github.com/formeo/fbrescue/internal/ddl"
	"github.com/formeo/fbrescue/internal/export"
	"github.com/formeo/fbrescue/internal/fdb"
	"github.com/formeo/fbrescue/internal/meta"
	"github.com/formeo/fbrescue/internal/value"
)

// The scripts that created the corpus databases, and the databases'
// default character sets.
var scripts = map[string]struct {
	files []string
	utf8  bool
}{
	"base": {[]string{"base.sql"}, false}, "deleted": {[]string{"base.sql", "deleted.sql"}, false},
	"big": {[]string{"big.sql"}, false}, "multifile": {[]string{"multifile.sql"}, false},
	"layout": {[]string{"layout.sql"}, false}, "types": {[]string{"types.sql", "boolean.sql"}, true},
	"fb4types": {[]string{"fb4types.sql"}, true}, "blobs": {[]string{"blobs.sql"}, true},
	"versions": {[]string{"versions.sql"}, true}, "shop": {[]string{"shop.sql"}, true},
}

// TestLayoutMatchesFormats: the layout computed from DDL equals the record
// format the engine stored, field for field (type, length, scale, sub type,
// offset). The DDL comes from the scripts that created the corpus, and from
// fbrescue's own schema.sql for every table whose columns are still in
// creation order.
func TestLayoutMatchesFormats(t *testing.T) {
	checked := 0
	for _, db := range corpus.List(t) {
		t.Run(db.Engine+"/"+db.Name, func(t *testing.T) {
			f, err := fdb.Open(db.Path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			cat, err := meta.Load(f)
			if err != nil {
				t.Skip(err) // hiwords
			}
			var stored []*meta.Table
			for _, tb := range cat.Tables {
				if inCreationOrder(tb) {
					stored = append(stored, tb)
				}
			}
			var schema bytes.Buffer
			if err := export.WriteSchema(&schema, stored); err != nil {
				t.Fatal(err)
			}
			checked += compare(t, "schema.sql", schema.String(), cat, false)

			sc, ok := scripts[db.Name]
			if !ok {
				return
			}
			var src strings.Builder
			if sc.utf8 {
				src.WriteString("/* CREATE DATABASE 'x.fdb' PAGE_SIZE 4096 DEFAULT CHARACTER SET UTF8; */\n")
			}
			for _, name := range sc.files {
				if name == "boolean.sql" && db.Engine == "fb25" {
					continue
				}
				b, err := os.ReadFile(filepath.Join("../../tools/mkcorpus/sql", name))
				if err != nil {
					t.Fatal(err)
				}
				src.Write(b)
			}
			checked += compare(t, "the corpus scripts", src.String(), cat, true)
		})
	}
	if checked < 50 {
		t.Errorf("only %d tables compared", checked)
	}
}

// inCreationOrder: columns in field id order, no dropped fields.
func inCreationOrder(tb *meta.Table) bool {
	cur := tb.Formats[tb.CurrentFormat]
	if cur == nil || len(cur.Fields) != len(tb.Columns) {
		return false
	}
	for i, c := range tb.Columns {
		if c.Field.ID != i {
			return false
		}
	}
	return true
}

func compare(t *testing.T, what, script string, cat *meta.Catalog, all bool) int {
	t.Helper()
	tables, _, err := ddl.Parse(script)
	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
	n := 0
	for _, dt := range tables {
		var tb *meta.Table
		for _, x := range cat.Tables {
			if x.Name == dt.Name {
				tb = x
			}
		}
		if tb == nil {
			if all {
				t.Errorf("%s: table %s is not in the database", what, dt.Name)
			}
			continue
		}
		cur := tb.Formats[tb.CurrentFormat]
		got := dt.Fields()
		if len(got) != len(cur.Fields) {
			t.Errorf("%s: table %s: %d fields, the format has %d", what, dt.Name, len(got), len(cur.Fields))
			continue
		}
		for i, want := range cur.Fields {
			g := got[i]
			if !want.Stored() {
				if g.Offset != 0 || !dt.Columns[i].Computed {
					t.Errorf("%s: %s.%s: computed in the database, stored in the DDL", what, dt.Name, dt.Columns[i].Name)
				}
				continue
			}
			if g.DType != want.DType || g.Length != want.Length || g.Scale != want.Scale || g.SubType != want.SubType || g.Offset != want.Offset {
				t.Errorf("%s: %s.%s: DDL gives %s, the database has %s", what, dt.Name, dt.Columns[i].Name, desc(g), desc(want))
			}
		}
		n++
	}
	return n
}

func desc(f value.Field) string {
	return fmt.Sprintf("%s (type %d, length %d, scale %d, sub type %d, offset %d)", f.SQLType(), f.DType, f.Length, f.Scale, f.SubType, f.Offset)
}
