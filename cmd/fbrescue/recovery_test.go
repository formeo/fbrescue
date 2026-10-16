package main

import (
	"bytes"
	"encoding/binary"
	"encoding/csv"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"testing"

	"github.com/formeo/fbrescue/internal/corpus"
	"github.com/formeo/fbrescue/internal/damage"
)

// recoverySeeds is the number of damaged copies per kind of damage and engine.
const recoverySeeds = 10

// tally counts rows over all trials of one kind of damage. A changed row
// is counted as reported when export logged a value it could not read
// (and wrote as NULL) in that trial; the rest are silent.
type tally struct {
	trials, failed, panics                  int
	rows, correct, missing, extra           int
	changedReported, changedSilent, colErrs int
	ddlCorrect, ddlWrong                    int // the same with --ddl (wrong: silently changed or extra)
}

// Recovery the tests insist on, per kind of damage: the share of rows
// recovered, and whether wrong rows (silently changed or extra) are
// allowed. Bit flips and random bytes inside a record cannot be told from
// real values: Firebird pages carry no checksum.
var recoveryFloor = map[string]struct {
	minPercent float64
	wrongOK    bool
}{
	"zeroed data page": {99, false}, "zeroed index page": {100, false}, "zeroed blob page": {99.9, false},
	"zeroed pointer page": {100, false}, "zeroed system table page": {90, false},
	"zeroed transaction inventory page": {100, false}, "zeroed page inventory page": {100, false},
	"zeroed header page": {100, false}, "page of random bytes": {99, true}, "16 flipped bits": {99, true},
	"zeroed 64 KB block": {85, false}, "two pages swapped": {99.9, false}, "truncated file": {60, false},
}

// TestRecovery damages copies of the shop database in each way of
// internal/damage, exports them, and compares the rows with the export of
// the intact database (which TestExportRoundTrip checks against the
// engine). Rows are matched by primary key: correct, changed (exported
// with different values), missing, or extra (keys the table never had).
// With FBRESCUE_RECOVERY_REPORT set, the table is written to that file.
func TestRecovery(t *testing.T) {
	results := map[string]*tally{}
	// The DDL a user would have: the script that created the database.
	script, err := os.ReadFile("../../tools/mkcorpus/sql/shop.sql")
	if err != nil {
		t.Fatal(err)
	}
	ddlPath := filepath.Join(t.TempDir(), "shop.sql")
	os.WriteFile(ddlPath, append([]byte("/* CREATE DATABASE 'shop.fdb' DEFAULT CHARACTER SET UTF8; */\n"), script...), 0o644)
	for _, db := range corpus.List(t) {
		if db.Name != "shop" {
			continue
		}
		orig, err := os.ReadFile(db.Path)
		if err != nil {
			t.Fatal(err)
		}
		ps := int(binary.LittleEndian.Uint16(orig[0x10:]))
		truth, _, _, err := exportRows(db.Path, t.TempDir())
		if err != nil || len(truth) != 3 {
			t.Fatalf("%s: intact export: %v, %d tables", db.Engine, err, len(truth))
		}
		for ki, k := range damage.Kinds {
			tl := results[k.Name]
			if tl == nil {
				tl = &tally{}
				results[k.Name] = tl
			}
			for seed := range uint64(recoverySeeds) {
				b, ok := k.Apply(bytes.Clone(orig), ps, rand.New(rand.NewPCG(seed, uint64(ki))))
				if !ok {
					continue
				}
				dir := t.TempDir()
				path := filepath.Join(dir, "shop.fdb")
				if err := os.WriteFile(path, b, 0o644); err != nil {
					t.Fatal(err)
				}
				got, colErrs, panicked, err := exportRows(path, filepath.Join(dir, "out"))
				withDDL, ddlColErrs, _, _ := exportRows(path, filepath.Join(dir, "ddl"), "--ddl", ddlPath)
				tl.trials++
				switch {
				case panicked != "":
					tl.panics++
					t.Errorf("%s, %s, seed %d: panic: %s", db.Engine, k.Name, seed, panicked)
				case err != nil:
					tl.failed++
					if testing.Verbose() {
						t.Logf("%s, %s, seed %d: %v", db.Engine, k.Name, seed, err)
					}
				}
				var tr tally
				compareRows(truth, got, &tr)
				reported := min(tr.changedSilent, colErrs)
				tl.rows, tl.correct, tl.missing, tl.extra = tl.rows+tr.rows, tl.correct+tr.correct, tl.missing+tr.missing, tl.extra+tr.extra
				tl.changedReported += reported
				tl.changedSilent += tr.changedSilent - reported
				var td tally
				compareRows(truth, withDDL, &td)
				tl.ddlCorrect += td.correct
				tl.ddlWrong += td.extra + td.changedSilent - min(td.changedSilent, ddlColErrs)
				if testing.Verbose() && tr.correct < tr.rows {
					t.Logf("%s, %s, seed %d: %d rows missing, %d changed (%d values reported unreadable), %d extra",
						db.Engine, k.Name, seed, tr.missing, tr.changedSilent, colErrs, tr.extra)
				}
			}
		}
	}
	if len(results) == 0 {
		t.Fatal("no shop database in the corpus")
	}

	var md strings.Builder
	fmt.Fprintf(&md, "| Damage | Copies | Rows recovered | With --ddl | Changed, reported | Changed silently | Extra rows | Export failed |\n|---|--:|--:|--:|--:|--:|--:|--:|\n")
	for _, k := range damage.Kinds {
		tl := results[k.Name]
		if tl.trials == 0 {
			t.Errorf("%s: never applied", k.Name)
			continue
		}
		pct := 100 * float64(tl.correct) / float64(tl.rows)
		pctDDL := 100 * float64(tl.ddlCorrect) / float64(tl.rows)
		fmt.Fprintf(&md, "| %s | %d | %.1f%% | %.1f%% | %d | %d | %d | %d |\n", k.Name, tl.trials, pct, pctDDL,
			tl.changedReported, tl.changedSilent, tl.extra, tl.failed)
		if tl.ddlCorrect < tl.correct || tl.ddlWrong > tl.changedSilent+tl.extra {
			t.Errorf("%s: with --ddl %d rows correct and %d wrong, without %d and %d", k.Name, tl.ddlCorrect, tl.ddlWrong, tl.correct, tl.changedSilent+tl.extra)
		}
		floor, ok := recoveryFloor[k.Name]
		switch {
		case !ok:
			t.Errorf("%s: no floor in recoveryFloor", k.Name)
		case pct < floor.minPercent:
			t.Errorf("%s: %.1f%% of rows recovered, want at least %.1f%%", k.Name, pct, floor.minPercent)
		case !floor.wrongOK && tl.changedSilent+tl.extra > 0:
			t.Errorf("%s: %d rows silently changed and %d extra rows, want none", k.Name, tl.changedSilent, tl.extra)
		}
	}
	t.Logf("recovery on the shop database (%d copies per kind and engine):\n%s", recoverySeeds, md.String())
	if p := os.Getenv("FBRESCUE_RECOVERY_REPORT"); p != "" {
		if err := os.WriteFile(p, []byte(md.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// exportRows exports a database to CSV and returns each table's rows by
// primary key. A panic is reported, not propagated.
func exportRows(path, dir string, options ...string) (rows map[string]map[string]string, colErrs int, panicked string, err error) {
	defer func() {
		if r := recover(); r != nil {
			panicked = fmt.Sprintf("%v\n%s", r, debug.Stack())
		}
	}()
	var out, errOut bytes.Buffer
	if code := run(append([]string{"export", "--out", dir, path}, options...), &out, &errOut); code == 1 {
		return nil, 0, "", fmt.Errorf("exit 1: %s", errOut.String())
	}
	if b, err := os.ReadFile(filepath.Join(dir, "_errors.jsonl")); err == nil {
		colErrs = bytes.Count(b, []byte(`"column":`))
	}
	keyCols := map[string]int{"CUSTOMERS": 1, "ORDERS": 1, "ITEMS": 2}
	rows = map[string]map[string]string{}
	for table, n := range keyCols {
		f, err := os.ReadFile(filepath.Join(dir, table+".csv"))
		if err != nil {
			continue // the table was not exported
		}
		recs, err := csv.NewReader(bytes.NewReader(f)).ReadAll()
		if err != nil || len(recs) == 0 {
			return nil, 0, "", fmt.Errorf("%s.csv: %v, %d lines; export said:\n%s%s", table, err, len(recs), out.String(), errOut.String())
		}
		m := map[string]string{}
		for i, r := range recs[1:] {
			key := strings.Join(r[:n], ",")
			if _, dup := m[key]; dup {
				key = fmt.Sprintf("%s#dup%d", key, i) // counts as an extra row
			}
			m[key] = strings.Join(r, "\x1f")
		}
		rows[table] = m
	}
	return rows, colErrs, "", nil
}

func compareRows(truth, got map[string]map[string]string, tl *tally) {
	tables := make([]string, 0, len(truth))
	for name := range truth {
		tables = append(tables, name)
	}
	sort.Strings(tables)
	for _, name := range tables {
		want, have := truth[name], got[name]
		for key, row := range want {
			tl.rows++
			switch g, ok := have[key]; {
			case !ok:
				tl.missing++
			case g == row:
				tl.correct++
			default:
				tl.changedSilent++
			}
		}
		for key := range have {
			if _, ok := want[key]; !ok {
				tl.extra++
			}
		}
	}
}
