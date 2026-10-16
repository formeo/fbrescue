package main

import (
	"bytes"
	"encoding/binary"
	"encoding/csv"
	"encoding/json"
	"math/rand/v2"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/formeo/fbrescue/internal/corpus"
	"github.com/formeo/fbrescue/internal/damage"
)

// TestReportHasNoData: on every corpus database the report is valid JSON
// and contains none of the table names, column names, file name, or text
// values of the database (taken from its export), and counts the rows the
// export wrote.
func TestReportHasNoData(t *testing.T) {
	dbs := corpus.List(t)
	// Damaged copies of the shop database, whose messages name tables and columns.
	for _, db := range dbs {
		if db.Name != "shop" {
			continue
		}
		orig, _ := os.ReadFile(db.Path)
		ps := int(binary.LittleEndian.Uint16(orig[0x10:]))
		for ki, k := range damage.Kinds {
			b, ok := k.Apply(bytes.Clone(orig), ps, rand.New(rand.NewPCG(1, uint64(ki))))
			if !ok {
				continue
			}
			path := filepath.Join(t.TempDir(), "shop.fdb")
			os.WriteFile(path, b, 0o644)
			dbs = append(dbs, corpus.DB{Engine: db.Engine, Name: "shop, " + k.Name, Path: path})
		}
	}
	for _, db := range dbs {
		t.Run(db.Engine+"/"+db.Name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if code := run([]string{"report", db.Path}, &out, &errOut); code != 0 {
				t.Fatalf("exit %d: %s", code, errOut.String())
			}
			var r report
			if err := json.Unmarshal(out.Bytes(), &r); err != nil {
				t.Fatal(err)
			}
			text := out.String()

			secrets := []string{filepath.Base(db.Path), db.Path}
			dir := t.TempDir()
			code := run([]string{"export", "--out", dir, db.Path}, &bytes.Buffer{}, &bytes.Buffer{})
			if code == 1 {
				return // hiwords: nothing exported, nothing to leak
			}
			files, _ := filepath.Glob(filepath.Join(dir, "*.csv"))
			rows := 0
			for _, f := range files {
				secrets = append(secrets, strings.TrimSuffix(filepath.Base(f), ".csv")) // table names
				b, _ := os.ReadFile(f)
				recs, err := csv.NewReader(bytes.NewReader(b)).ReadAll()
				if err != nil || len(recs) == 0 {
					t.Fatalf("%s: %v", f, err)
				}
				rows += len(recs) - 1
				secrets = append(secrets, recs[0]...) // column names
				for _, rec := range recs[1:min(len(recs), 50)] {
					for _, v := range rec {
						if len(v) >= 4 && regexp.MustCompile(`[^0-9.:\- ]`).MatchString(v) {
							secrets = append(secrets, v)
						}
					}
				}
			}
			for _, s := range secrets {
				// Names of one or two letters ("T", "ID") stand for nothing
				// in particular and would match the report's own words.
				if len(s) < 3 || !utf8.ValidString(s) {
					continue
				}
				if regexp.MustCompile(`(^|[^\pL\pN_$])` + regexp.QuoteMeta(s) + `([^\pL\pN_$]|$)`).MatchString(text) {
					t.Errorf("the report contains %q", s)
				}
			}
			if !strings.HasPrefix(db.Name, "shop, ") && (r.Export == nil || r.Export.Rows != rows) {
				t.Errorf("report counts %+v rows, the export wrote %d", r.Export, rows)
			}
		})
	}
}
