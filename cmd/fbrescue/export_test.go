package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/formeo/fbrescue/internal/corpus"
)

// fbTool runs a tool of the engine that created a corpus database
// ("fb25" ... "fb50"), through tools/mkcorpus/fb.sh. Any output on stderr
// fails the test.
func fbTool(t *testing.T, engine, stdin string, args ...string) string {
	t.Helper()
	fb, err := filepath.Abs("../../tools/mkcorpus/fb.sh")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(fb, append([]string{strings.TrimPrefix(engine, "fb")}, args...)...)
	cmd.Stdin = strings.NewReader(stdin)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil || errOut.Len() > 0 {
		t.Fatalf("%v: %v\n%s", args, err, errOut.String())
	}
	return out.String()
}

func copyTo(t *testing.T, src, dir string) string {
	t.Helper()
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, filepath.Base(src))
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return dst
}

type exportedTable struct {
	name string
	cols []string
}

func exportedTables(t *testing.T, path string) []exportedTable {
	t.Helper()
	var out, errOut bytes.Buffer
	if code := run([]string{"tables", "--json", path}, &out, &errOut); code != 0 {
		t.Fatalf("tables: exit %d: %s", code, errOut.String())
	}
	var r struct {
		Tables []struct {
			Name    string
			Columns []struct {
				Name     string
				Exported bool
			}
		}
	}
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	var tables []exportedTable
	for _, tb := range r.Tables {
		et := exportedTable{name: tb.Name}
		for _, c := range tb.Columns {
			if c.Exported {
				et.cols = append(et.cols, `"`+c.Name+`"`)
			}
		}
		tables = append(tables, et)
	}
	return tables
}

// TestExportRoundTrip is the main check of export: the SQL export of each
// database is loaded into a new database by the same engine, and SELECT on
// the new database prints exactly what SELECT on the original prints, value
// for value, in the engine's own formatting.
func TestExportRoundTrip(t *testing.T) {
	roundTrip := map[string]bool{"base": true, "deleted": true, "readonly": true, "types": true, "layout": true, "manytx": true}
	for _, db := range corpus.List(t) {
		if !roundTrip[db.Name] {
			continue
		}
		t.Run(db.Engine+"/"+db.Name, func(t *testing.T) {
			dir := t.TempDir()
			out := filepath.Join(dir, "out")
			var stdout, stderr bytes.Buffer
			code := run([]string{"export", "--format", "sql", "--out", out, db.Path}, &stdout, &stderr)
			// layout has rows changed by a rolled-back transaction (RB), a BLOB
			// column (BLOBS) and WIN1251 bytes in a NONE column (CS).
			wantCode := 0
			if db.Name == "layout" {
				wantCode = 2
			}
			if code != wantCode {
				t.Fatalf("export: exit %d, want %d\n%s%s", code, wantCode, stdout.String(), stderr.String())
			}

			newDB := filepath.Join(dir, "new.fdb")
			fbTool(t, db.Engine, "create database '"+newDB+"' default character set utf8; quit;", "isql", "-q")
			fbTool(t, db.Engine, "", "isql", "-q", "-b", "-ch", "UTF8", "-i", filepath.Join(out, "schema.sql"), newDB)
			orig := copyTo(t, db.Path, dir) // SELECT may garbage-collect: never on the corpus file

			tables := exportedTables(t, db.Path)
			if len(tables) == 0 {
				t.Fatal("no tables")
			}
			for _, tb := range tables {
				if tb.name == "CS" { // raw WIN1251 bytes are not valid in a UTF-8 script; see TestExportCharset
					continue
				}
				fbTool(t, db.Engine, "", "isql", "-q", "-b", "-ch", "UTF8", "-i", filepath.Join(out, tb.name+".sql"), newDB)
				where := ""
				if tb.name == "RB" { // rows 1 and 2 need the older committed version: v0.3
					where = " where id not in (1, 2)"
				}
				q := "set list on; select " + strings.Join(tb.cols, ", ") + ` from "` + tb.name + `"` + where + " order by 1; quit;"
				want := fbTool(t, db.Engine, q, "isql", "-q", "-ch", "UTF8", orig)
				got := fbTool(t, db.Engine, q, "isql", "-q", "-ch", "UTF8", newDB)
				if got != want {
					t.Errorf("table %s differs after the round trip:\n%s", tb.name, firstDiff(want, got))
				}
				if strings.Count(want, "\n") < 2 && tb.name != "WIDE" {
					t.Errorf("table %s: the reference query returned nothing", tb.name)
				}
			}
		})
	}
}

func firstDiff(a, b string) string {
	al, bl := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := 0; i < len(al) && i < len(bl); i++ {
		if al[i] != bl[i] {
			return "line " + itoa(i+1) + ":\n  original: " + al[i] + "\n  exported: " + bl[i]
		}
	}
	return "lengths differ: " + itoa(len(al)) + " vs " + itoa(len(bl)) + " lines"
}

func itoa(i int) string { b, _ := json.Marshal(i); return string(b) }

// TestExportFormats checks CSV and JSON Lines on the types table: quoting,
// NULL versus empty string, extreme values and non-ASCII text.
func TestExportFormats(t *testing.T) {
	for _, db := range corpus.List(t) {
		if db.Name != "types" {
			continue
		}
		t.Run(db.Engine, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range []string{"csv", "jsonl"} {
				var out, errOut bytes.Buffer
				if code := run([]string{"export", "--format", f, "--out", dir, db.Path}, &out, &errOut); code != 0 {
					t.Fatalf("%s: exit %d: %s%s", f, code, out.String(), errOut.String())
				}
			}

			raw, _ := os.ReadFile(filepath.Join(dir, "TYPES.csv"))
			recs, err := csv.NewReader(bytes.NewReader(raw)).ReadAll()
			if err != nil || len(recs) != 2001 {
				t.Fatalf("csv: %v, %d records", err, len(recs))
			}
			col := map[string]int{}
			for i, n := range recs[0] {
				col[n] = i
			}
			row := map[string][]string{}
			for _, r := range recs[1:] {
				row[r[col["ID"]]] = r
			}
			for name, want := range map[string]string{
				"BI": "-9223372036854775808", "N184": "-99999999999999.9999", "D": "0001-01-01", "T": "23:59:59.9999",
				"TS": "0001-01-01 00:00:00.0001", "VC": `quote ' double " comma , semi ; pipe | back \ end`,
				"CU": "ÀéЖ  ", "VU": "Привет, мир 😀", "VW": "Привет", "VF": "ΩЖ", "OCT": "deadbeef", "DP": "-1.7976931348623157e+308",
			} {
				if got := row["2"][col[name]]; got != want {
					t.Errorf("csv row 2 %s = %q, want %q", name, got, want)
				}
			}
			lines := strings.Split(string(raw), "\r\n")
			if !strings.HasPrefix(lines[4], "4,,,") || strings.Contains(lines[4], `""`) {
				t.Errorf("csv: NULLs should be empty unquoted fields: %q", lines[4])
			}
			if !strings.Contains(lines[1], `,"",`) {
				t.Errorf("csv: an empty string should be \"\": %q", lines[1])
			}

			jl, _ := os.ReadFile(filepath.Join(dir, "TYPES.jsonl"))
			var n int
			for _, l := range strings.Split(strings.TrimSpace(string(jl)), "\n") {
				d := json.NewDecoder(strings.NewReader(l))
				d.UseNumber()
				var m map[string]any
				if err := d.Decode(&m); err != nil {
					t.Fatalf("jsonl line %d: %v", n+1, err)
				}
				n++
				switch m["ID"].(json.Number).String() {
				case "2":
					if m["BI"].(json.Number).String() != "-9223372036854775808" || m["VU"] != "Привет, мир 😀" || m["OCT"] != "deadbeef" {
						t.Errorf("jsonl row 2: %v", m)
					}
				case "4":
					if m["SM"] != nil || m["VC"] != nil {
						t.Errorf("jsonl row 4 should be NULLs: %v", m)
					}
				}
			}
			if n != 2000 {
				t.Errorf("jsonl: %d rows", n)
			}
		})
	}
}

// TestExportCharset: WIN1251 bytes in a CHARACTER SET NONE column are left
// as they are, with a warning, unless --charset says how to read them.
func TestExportCharset(t *testing.T) {
	for _, db := range corpus.List(t) {
		if db.Name != "layout" {
			continue
		}
		t.Run(db.Engine, func(t *testing.T) {
			dir := t.TempDir()
			var out, errOut bytes.Buffer
			code := run([]string{"export", "--table", "CS", "--out", dir, db.Path}, &out, &errOut)
			b, _ := os.ReadFile(filepath.Join(dir, "CS.csv"))
			if code != 2 || !strings.Contains(out.String(), "--charset") || !bytes.Contains(b, []byte{0xCF, 0xF0, 0xE8, 0xE2, 0xE5, 0xF2}) {
				t.Errorf("without --charset: exit %d, output %q, file %q", code, out.String(), b)
			}
			out.Reset()
			code = run([]string{"export", "--table", "CS", "--charset", "win1251", "--out", dir, db.Path}, &out, &errOut)
			b, _ = os.ReadFile(filepath.Join(dir, "CS.csv"))
			if code != 0 || string(b) != "ID,N\r\n1,Привет\r\n" {
				t.Errorf("with --charset: exit %d, file %q, output %s", code, b, out.String())
			}
		})
	}
}

// TestExportWarnings: rows the export cannot read correctly are counted and
// reported, never silently dropped or guessed.
func TestExportWarnings(t *testing.T) {
	for _, db := range corpus.List(t) {
		if db.Name != "layout" && db.Name != "nblock" && db.Name != "big" {
			continue
		}
		t.Run(db.Engine+"/"+db.Name, func(t *testing.T) {
			dir := t.TempDir()
			var out, errOut bytes.Buffer
			code := run([]string{"export", "--json", "--out", dir, db.Path}, &out, &errOut)
			var res struct {
				Tables []struct {
					Name          string
					Rows, Skipped int
					Omitted       []string `json:"omitted_columns"`
				}
				Warnings []string
				Errors   int
			}
			if err := json.Unmarshal(out.Bytes(), &res); err != nil {
				t.Fatalf("exit %d: %v: %s%s", code, err, out.String(), errOut.String())
			}
			tables := map[string]int{}
			for i, tb := range res.Tables {
				tables[tb.Name] = i
			}
			switch db.Name {
			case "layout":
				rb, blobs := res.Tables[tables["RB"]], res.Tables[tables["BLOBS"]]
				if code != 2 || rb.Rows != 18 || rb.Skipped != 2 || res.Errors != 2 || len(blobs.Omitted) != 1 || blobs.Rows != 3000 {
					t.Errorf("exit %d, RB %+v, BLOBS %+v, errors %d", code, rb, blobs, res.Errors)
				}
				errs, _ := os.ReadFile(filepath.Join(dir, "_errors.jsonl"))
				if strings.Count(string(errs), `"table":"RB"`) != 2 {
					t.Errorf("_errors.jsonl: %s", errs)
				}
			case "nblock":
				if code != 2 || !strings.Contains(strings.Join(res.Warnings, "\n"), "nbackup lock") {
					t.Errorf("exit %d, warnings %q", code, res.Warnings)
				}
			case "big":
				if code != 0 || res.Tables[tables["BIG"]].Rows != 34000 {
					t.Errorf("exit %d, tables %+v", code, res.Tables)
				}
			}
		})
	}
}
