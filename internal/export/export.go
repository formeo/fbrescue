// Package export writes the rows of user tables to CSV, SQL or JSON Lines
// files, and their schema to schema.sql.
package export

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/formeo/fbrescue/internal/meta"
	"github.com/formeo/fbrescue/internal/record"
	"github.com/formeo/fbrescue/internal/value"
)

// Options for an export.
type Options struct {
	Format string   // "csv", "sql" or "jsonl"
	Dir    string   // output directory, created if missing
	Tables []string // export only these tables (exact names); all if empty
	Value  value.Options
}

// Result summarizes an export.
type Result struct {
	Tables   []*TableResult `json:"tables"`
	Warnings []string       `json:"warnings"` // data left out or possibly wrong
	Notes    []string       `json:"notes"`    // worth knowing, nothing lost
	Errors   int            `json:"errors"`   // lines in _errors.jsonl
}

// TableResult summarizes one table.
type TableResult struct {
	Name    string   `json:"name"`
	File    string   `json:"file"`
	Rows    int      `json:"rows"`
	Skipped int      `json:"skipped"` // rows that exist but could not be exported
	Warned  bool     `json:"warned"`  // some values may be wrong: see the warnings
	Omitted []string `json:"omitted_columns,omitempty"`
}

// Partial reports whether some data could not be exported.
func (r *Result) Partial() bool {
	if r.Errors > 0 || len(r.Warnings) > 0 {
		return true
	}
	for _, t := range r.Tables {
		if t.Skipped > 0 || len(t.Omitted) > 0 {
			return true
		}
	}
	return false
}

// ErrorLine is one entry of _errors.jsonl.
type ErrorLine struct {
	Table  string `json:"table"`
	Page   uint32 `json:"page,omitempty"`
	Line   *int   `json:"line,omitempty"`
	Reason string `json:"reason"`
}

// Exported reports whether a column's values are written, and why not.
func Exported(c *meta.Column) (bool, string) {
	switch {
	case c.Computed:
		return false, "COMPUTED BY: the expression is not stored with the data"
	case !c.Field.Supported():
		return false, c.Field.SQLType() + " values are not exported yet"
	}
	return true, ""
}

// Run exports the tables of cat into opt.Dir. progress, if not nil, is
// called after each table.
func Run(cat *meta.Catalog, opt Options, progress func(*TableResult)) (*Result, error) {
	tables, err := selectTables(cat, opt.Tables)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(opt.Dir, 0o755); err != nil {
		return nil, err
	}
	res := &Result{Tables: []*TableResult{}, Warnings: append([]string{}, cat.Warnings...), Notes: []string{}}

	schema, err := os.Create(filepath.Join(opt.Dir, "schema.sql"))
	if err != nil {
		return nil, err
	}
	defer schema.Close()
	if err := WriteSchema(schema, tables); err != nil {
		return nil, err
	}

	errPath := filepath.Join(opt.Dir, "_errors.jsonl")
	os.Remove(errPath) // from an earlier run into the same directory
	var errFile *os.File
	logError := func(e ErrorLine) error {
		if errFile == nil {
			if errFile, err = os.Create(errPath); err != nil {
				return err
			}
		}
		res.Errors++
		b, _ := json.Marshal(e)
		_, err := errFile.Write(append(b, '\n'))
		return err
	}
	defer func() {
		if errFile != nil {
			errFile.Close()
		}
	}()

	names := fileNames(tables)
	for _, t := range tables {
		tr := &TableResult{Name: t.Name, File: names[t.ID] + "." + opt.Format}
		if err := exportTable(cat, t, opt, tr, res, logError); err != nil {
			return res, fmt.Errorf("table %s: %w", t.Name, err)
		}
		res.Tables = append(res.Tables, tr)
		if progress != nil {
			progress(tr)
		}
	}
	return res, schema.Close()
}

func selectTables(cat *meta.Catalog, names []string) ([]*meta.Table, error) {
	if len(names) == 0 {
		return cat.Tables, nil
	}
	var out []*meta.Table
	for _, n := range names {
		var found *meta.Table
		for _, t := range cat.Tables {
			if t.Name == n {
				found = t
			}
		}
		if found == nil {
			for _, t := range cat.Tables {
				if strings.EqualFold(t.Name, n) {
					found = t
				}
			}
		}
		if found == nil {
			return nil, fmt.Errorf("no table %q", n)
		}
		out = append(out, found)
	}
	return out, nil
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9_$.-]`)

// fileNames maps tables to file names that are safe and unique on
// case-insensitive file systems.
func fileNames(tables []*meta.Table) map[uint16]string {
	out := map[uint16]string{}
	used := map[string]bool{}
	for _, t := range tables {
		n := unsafeName.ReplaceAllString(t.Name, "_")
		if n == "" || strings.HasPrefix(n, ".") || used[strings.ToLower(n)] {
			n = fmt.Sprintf("%s_%d", n, t.ID)
		}
		used[strings.ToLower(n)] = true
		out[t.ID] = n
	}
	return out
}

type rowWriter interface {
	row(vals []any) error
	flush() error
}

func exportTable(cat *meta.Catalog, t *meta.Table, opt Options, tr *TableResult, res *Result, logError func(ErrorLine) error) error {
	var cols []*meta.Column
	for _, c := range t.Columns {
		if ok, why := Exported(c); ok {
			cols = append(cols, c)
		} else if !c.Computed {
			tr.Omitted = append(tr.Omitted, c.Name)
			res.Warnings = append(res.Warnings, fmt.Sprintf("%s.%s: %s", t.Name, c.Name, why))
		}
	}

	f, err := os.Create(filepath.Join(opt.Dir, tr.File))
	if err != nil {
		return err
	}
	defer f.Close()
	bw := bufio.NewWriterSize(f, 1<<16)
	var w rowWriter
	switch opt.Format {
	case "csv":
		w, err = newCSV(bw, cols)
	case "sql":
		w, err = newSQL(bw, t, cols)
	case "jsonl":
		w, err = newJSONL(bw, cols)
	default:
		return fmt.Errorf("unknown format %q", opt.Format)
	}
	if err != nil {
		return err
	}

	raw := map[string]int{}    // column -> values not valid UTF-8
	noDflt := map[string]int{} // column -> rows whose default could not be evaluated
	uncommitted := 0           // inserts never committed: not rows
	vals := make([]any, len(cols))
	skip := func(v meta.Version, reason string) error {
		tr.Skipped++
		e := ErrorLine{Table: t.Name, Page: v.Page, Reason: reason}
		if v.Line >= 0 {
			e.Line = &v.Line
		}
		return logError(e)
	}

	err = cat.Records(t.ID, func(v meta.Version, rec []byte, rerr error) error {
		h := v.Header
		switch {
		case rerr != nil:
			return skip(v, rerr.Error())
		case h.Flags&record.FlagLongTx != 0:
			return skip(v, "48-bit transaction number: not supported yet")
		case h.Flags&record.FlagDamaged != 0:
			return skip(v, "record is marked damaged by the engine")
		}
		committed, err := cat.Committed(h.Tx)
		if err != nil {
			return skip(v, err.Error())
		}
		if !committed {
			if h.BackPage == 0 && h.Flags&record.FlagDeleted == 0 {
				uncommitted++ // an insert that never committed: not a row
				return nil
			}
			return skip(v, fmt.Sprintf("row changed by transaction %d, which did not commit; reading the older committed version arrives in v0.3", h.Tx))
		}
		if h.Flags&record.FlagDeleted != 0 {
			return nil
		}
		fm, ok := t.Formats[uint16(h.Format)]
		if !ok {
			return skip(v, fmt.Sprintf("record format %d is not in RDB$FORMATS", h.Format))
		}
		for i, c := range cols {
			x, err := columnValue(rec, fm, c, opt.Value)
			if err == errNoDefault {
				noDflt[c.Name]++
				err = nil
			}
			if err != nil {
				return skip(v, fmt.Sprintf("column %s: %v", c.Name, err))
			}
			if s, ok := x.(value.Text); ok && s.Raw {
				raw[c.Name]++
			}
			vals[i] = x
		}
		tr.Rows++
		return w.row(vals)
	})
	if err != nil {
		return err
	}
	tr.Warned = len(raw) > 0 || len(noDflt) > 0
	for c, n := range raw {
		res.Warnings = append(res.Warnings, fmt.Sprintf("%s.%s: %d values are not valid UTF-8 and were written unchanged; for CHARACTER SET NONE data try --charset", t.Name, c, n))
	}
	for c, n := range noDflt {
		res.Warnings = append(res.Warnings, fmt.Sprintf("%s.%s: %d rows predate the column and its default could not be evaluated; written as NULL", t.Name, c, n))
	}
	if uncommitted > 0 {
		res.Notes = append(res.Notes, fmt.Sprintf("%s: %d rows inserted by transactions that did not commit are not rows of the table and were left out", t.Name, uncommitted))
	}
	if err := w.flush(); err != nil {
		return err
	}
	if err := bw.Flush(); err != nil {
		return err
	}
	return f.Close()
}

var errNoDefault = fmt.Errorf("default not evaluated")

// columnValue reads a column from a record written with format fm,
// converting it to the column's current type.
func columnValue(rec []byte, fm *meta.Format, c *meta.Column, opt value.Options) (any, error) {
	id := c.Field.ID
	if id >= len(fm.Fields) || !fm.Fields[id].Stored() {
		// The record predates the column.
		switch {
		case !c.HasDefault:
			return nil, nil
		case c.DefaultErr != nil:
			return nil, errNoDefault
		}
		return c.Default, nil
	}
	f := fm.Fields[id]
	if !f.Supported() {
		return nil, fmt.Errorf("stored as data type %d, which is not supported yet", f.DType)
	}
	x, err := value.Decode(rec, f, opt)
	if err != nil || x == nil {
		return x, err
	}
	if f.DType != c.Field.DType || f.Scale != c.Field.Scale || f.Length != c.Field.Length {
		return value.Convert(x, c.Field)
	}
	return x, nil
}

// WriteSchema writes CREATE TABLE statements for the tables.
func WriteSchema(w io.Writer, tables []*meta.Table) error {
	bw := bufio.NewWriter(w)
	fmt.Fprintln(bw, "-- Tables recovered by fbrescue. Types come from the stored record format:")
	fmt.Fprintln(bw, "-- NUMERIC/DECIMAL precision is the maximum for the storage size. Constraints,")
	fmt.Fprintln(bw, "-- indexes, triggers and domains are not included.")
	fmt.Fprintln(bw, "-- Create the database with SQL dialect 3, then: isql -ch UTF8 -i schema.sql <database>")
	for _, t := range tables {
		fmt.Fprintf(bw, "\nCREATE TABLE %s (\n", quote(t.Name))
		var lines []string
		for _, c := range t.Columns {
			ok, why := Exported(c)
			switch {
			case c.Computed:
				lines = append(lines, fmt.Sprintf("  -- %s COMPUTED BY (...): expression not recovered", quote(c.Name)))
			case !ok:
				lines = append(lines, fmt.Sprintf("  %s %s -- %s", quote(c.Name), c.Field.SQLType(), why))
			default:
				l := fmt.Sprintf("  %s %s", quote(c.Name), c.Field.SQLType())
				if c.NotNull {
					l += " NOT NULL"
				}
				lines = append(lines, l)
			}
		}
		// Commas go after every real column but the last; comments carry none.
		last := -1
		for i, l := range lines {
			if !strings.HasPrefix(strings.TrimSpace(l), "--") {
				last = i
			}
		}
		for i, l := range lines {
			if i < last && !strings.HasPrefix(strings.TrimSpace(l), "--") {
				if k := strings.Index(l, " --"); k >= 0 {
					l = l[:k] + "," + l[k:]
				} else {
					l += ","
				}
			}
			fmt.Fprintln(bw, l)
		}
		fmt.Fprintln(bw, ");")
	}
	fmt.Fprintln(bw, "\nCOMMIT;")
	return bw.Flush()
}

func quote(name string) string { return `"` + strings.ReplaceAll(name, `"`, `""`) + `"` }
