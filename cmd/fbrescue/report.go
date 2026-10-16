package main

import (
	"bufio"
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/formeo/fbrescue/internal/export"
	"github.com/formeo/fbrescue/internal/fdb"
	"github.com/formeo/fbrescue/internal/meta"
	"github.com/formeo/fbrescue/internal/scan"
)

// report is a diagnostic report to attach to a bug report: the structure
// of the database and what reading it ran into, without data. Table and
// column names become T<relation id> and C<n>, file names "db", and
// numbers in error messages N; values never appear.
type report struct {
	Tool     string        `json:"tool"`
	File     reportFile    `json:"file"`
	Header   *reportHeader `json:"header"` // null if the header page is damaged
	Warnings []string      `json:"warnings"`
	Scan     *scan.Report  `json:"scan"`
	Catalog  reportCatalog `json:"catalog"`
	Export   *reportExport `json:"export,omitempty"`
	Tables   []reportTable `json:"tables"`
}

type reportFile struct {
	Size     int64  `json:"size"`
	Files    int    `json:"files"`
	PageSize int    `json:"page_size"`
	Pages    uint32 `json:"pages"`
}

type reportHeader struct {
	ODS        string   `json:"ods"`
	Dialect    int      `json:"dialect"`
	Attributes []string `json:"attributes"`
	OIT        uint64   `json:"oldest_transaction"`
	OAT        uint64   `json:"oldest_active"`
	OST        uint64   `json:"oldest_snapshot"`
	Next       uint64   `json:"next_transaction"`
}

type reportCatalog struct {
	Error    string   `json:"error,omitempty"`
	Warnings []string `json:"warnings"`
}

type reportExport struct {
	Rows     int            `json:"rows"`
	Skipped  int            `json:"skipped"`
	Errors   int            `json:"errors"`
	Reasons  map[string]int `json:"reasons"` // _errors.jsonl reasons, numbers replaced
	Warnings []string       `json:"warnings"`
	Notes    []string       `json:"notes"`
}

type reportTable struct {
	Name    string   `json:"name"` // T<relation id>
	Formats int      `json:"formats"`
	Columns []string `json:"columns"` // types
	Rows    int      `json:"rows"`
	Skipped int      `json:"skipped"`
	Deleted int      `json:"deleted_on_disk"`
}

func runReport(a cliArgs, stdout, stderr io.Writer) int {
	db, err := fdb.Open(a.path)
	if err != nil {
		fmt.Fprintf(stderr, "fbrescue report: %v\n", err)
		return 1
	}
	defer db.Close()

	r := report{Tool: "fbrescue " + version, Tables: []reportTable{}, Catalog: reportCatalog{Warnings: []string{}}}
	r.File = reportFile{Files: len(db.Files), PageSize: db.PageSize, Pages: db.Pages}
	names := []string{}
	for _, p := range db.Files {
		if st, err := os.Stat(p.Path); err == nil {
			r.File.Size += st.Size()
		}
		names = append(names, filepath.Base(p.Path))
	}
	an := newAnonymizer()
	for _, n := range names {
		an.add(n, "db")
	}
	if h := db.Header; h != nil {
		attrs, _ := h.Attributes()
		r.Header = &reportHeader{ODS: fmt.Sprintf("%d.%d", h.ODSMajor, h.ODSMinor), Dialect: h.Dialect(), Attributes: attrs,
			OIT: h.OIT, OAT: h.OAT, OST: h.OST, Next: h.NextTx}
		if r.Header.Attributes == nil {
			r.Header.Attributes = []string{}
		}
	}
	r.Warnings = warnings(db)
	r.Scan = scan.Run(db, nil)

	cat, err := meta.Load(db)
	if err != nil {
		r.Catalog.Error = err.Error()
	} else {
		for _, t := range cat.Tables {
			an.add(t.Name, fmt.Sprintf("T%d", t.ID))
		}
		// Column names repeat between tables: each distinct name is C<n>.
		cols := map[string]bool{}
		for _, t := range cat.Tables {
			for _, c := range t.Columns {
				if !cols[c.Name] {
					cols[c.Name] = true
					an.add(c.Name, fmt.Sprintf("C%d", len(cols)))
				}
			}
		}
		r.Export, r.Tables, err = dryExport(cat)
		if err != nil {
			r.Catalog.Error = "export: " + err.Error()
		}
		// Export repeats the catalog's warnings, which reading the tables
		// may have added to: they are listed once, under catalog.
		r.Catalog.Warnings = append(r.Catalog.Warnings, cat.Warnings...)
		if r.Export != nil {
			r.Export.Warnings = r.Export.Warnings[min(len(cat.Warnings), len(r.Export.Warnings)):]
		}
	}

	// Anonymize everything that came from messages.
	r.Warnings = an.all(r.Warnings)
	r.Catalog.Error = an.text(r.Catalog.Error)
	r.Catalog.Warnings = an.all(r.Catalog.Warnings)
	for i := range r.Scan.Problems {
		r.Scan.Problems[i].Message = an.text(r.Scan.Problems[i].Message)
	}
	if e := r.Export; e != nil {
		e.Warnings, e.Notes = an.all(e.Warnings), an.all(e.Notes)
		reasons := map[string]int{}
		for k, v := range e.Reasons {
			reasons[digits.ReplaceAllString(an.text(k), "N")] += v
		}
		e.Reasons = reasons
	}
	writeJSON(stdout, r)
	return 0
}

// dryExport exports to a temporary directory, which is removed, to learn
// what reading every row runs into.
func dryExport(cat *meta.Catalog) (*reportExport, []reportTable, error) {
	dir, err := os.MkdirTemp("", "fbrescue-report-")
	if err != nil {
		return nil, nil, err
	}
	defer os.RemoveAll(dir)
	res, err := export.Run(cat, export.Options{Format: "csv", Dir: dir, IncludeDeleted: true}, nil)
	if err != nil {
		return nil, nil, err
	}
	e := &reportExport{Errors: res.Errors, Reasons: map[string]int{}, Warnings: res.Warnings, Notes: res.Notes}
	var tables []reportTable
	for i, tr := range res.Tables {
		t := cat.Tables[i] // all tables, in catalog order
		rt := reportTable{Name: fmt.Sprintf("T%d", t.ID), Formats: len(t.Formats), Columns: []string{},
			Rows: tr.Rows, Skipped: tr.Skipped, Deleted: tr.Deleted}
		for _, c := range t.Columns {
			typ := c.Field.SQLType()
			if c.Computed {
				typ = "COMPUTED BY"
			}
			rt.Columns = append(rt.Columns, typ)
		}
		tables = append(tables, rt)
		e.Rows += tr.Rows
		e.Skipped += tr.Skipped
	}
	if f, err := os.Open(filepath.Join(dir, "_errors.jsonl")); err == nil {
		sc := bufio.NewScanner(f)
		sc.Buffer(nil, 1<<20)
		for sc.Scan() {
			var l export.ErrorLine
			if json.Unmarshal(sc.Bytes(), &l) == nil {
				e.Reasons[l.Reason]++
			}
		}
		f.Close()
	}
	return e, tables, nil
}

var digits = regexp.MustCompile(`[0-9]+`)

// anonymizer replaces names, longest first, where they stand as words.
type anonymizer struct {
	names map[string]string
	re    *regexp.Regexp
}

func newAnonymizer() *anonymizer { return &anonymizer{names: map[string]string{}} }

// add maps a name, unless it already has a replacement.
func (a *anonymizer) add(name, replacement string) {
	if _, ok := a.names[name]; name != "" && !ok {
		a.names[name] = replacement
		a.re = nil
	}
}

func (a *anonymizer) text(s string) string {
	if s == "" || len(a.names) == 0 {
		return s
	}
	if a.re == nil {
		keys := make([]string, 0, len(a.names))
		for k := range a.names {
			keys = append(keys, regexp.QuoteMeta(k))
		}
		slices.SortFunc(keys, func(x, y string) int { return cmp.Compare(len(y), len(x)) })
		a.re = regexp.MustCompile(`(^|[^\pL\pN_$])(` + strings.Join(keys, "|") + `)([^\pL\pN_$]|$)`)
	}
	// Neighbouring matches share a boundary character, so one pass can
	// miss every second name of a list: a few passes.
	for range 4 {
		s = a.re.ReplaceAllStringFunc(s, func(m string) string {
			sub := a.re.FindStringSubmatch(m)
			return sub[1] + a.names[sub[2]] + sub[3]
		})
	}
	return s
}

func (a *anonymizer) all(list []string) []string {
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = a.text(s)
	}
	return out
}
