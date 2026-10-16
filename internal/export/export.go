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
	Format         string   // "csv", "sql" or "jsonl"
	Dir            string   // output directory, created if missing
	Tables         []string // export only these tables (exact names); all if empty
	IncludeDeleted bool     // also write deleted rows still on disk, to <table>.deleted.<format>
	Value          value.Options
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
	Name        string   `json:"name"`
	File        string   `json:"file"`
	Rows        int      `json:"rows"`
	Skipped     int      `json:"skipped"` // rows that exist but could not be exported
	Warned      bool     `json:"warned"`  // some values may be wrong: see the warnings
	Omitted     []string `json:"omitted_columns,omitempty"`
	Deleted     int      `json:"deleted,omitempty"` // with IncludeDeleted
	DeletedFile string   `json:"deleted_file,omitempty"`
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

// ErrorLine is one entry of _errors.jsonl. With Column set, the row was
// exported with that column NULL; otherwise the row was left out.
type ErrorLine struct {
	Table  string `json:"table"`
	Page   uint32 `json:"page,omitempty"`
	Line   *int   `json:"line,omitempty"`
	Column string `json:"column,omitempty"`
	Reason string `json:"reason"`
}

// Exported reports whether a column's values are written, and why not.
func Exported(c *meta.Column) (bool, string) {
	switch {
	case c.Computed:
		return false, "COMPUTED BY: the expression is not stored with the data"
	case !c.Field.Supported():
		return false, fmt.Sprintf("%s (data type %d) values are not exported yet", c.Field.SQLType(), c.Field.DType)
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

// output is one file of rows: the table, or its deleted rows.
type output struct {
	f  *os.File
	bw *bufio.Writer
	w  rowWriter
}

func openOutput(path, format string, t *meta.Table, cols []*meta.Column, blobs blobSink) (*output, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	o := &output{f: f, bw: bufio.NewWriterSize(f, 1<<16)}
	switch format {
	case "csv":
		o.w, err = newCSV(o.bw, cols)
	case "sql":
		o.w, err = newSQL(o.bw, t, cols, blobs)
	case "jsonl":
		o.w, err = newJSONL(o.bw, cols)
	default:
		err = fmt.Errorf("unknown format %q", format)
	}
	if err != nil {
		f.Close()
		return nil, err
	}
	return o, nil
}

func (o *output) close() error {
	err := o.w.flush()
	if err == nil {
		err = o.bw.Flush()
	}
	if cerr := o.f.Close(); err == nil {
		err = cerr
	}
	return err
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

	base := strings.TrimSuffix(tr.File, "."+opt.Format)
	blobs := &blobFiles{dir: filepath.Join(opt.Dir, base+".blobs"), rel: base + ".blobs"}
	out, err := openOutput(filepath.Join(opt.Dir, tr.File), opt.Format, t, cols, blobs)
	if err != nil {
		return err
	}
	defer out.f.Close()
	var deleted *output
	defer func() {
		if deleted != nil {
			deleted.f.Close()
		}
	}()

	raw := map[string]int{}     // column -> values not valid UTF-8
	noDflt := map[string]int{}  // column -> rows whose default could not be evaluated
	colErrs := map[string]int{} // column -> values left NULL after an error
	var uncommitted, older, limbo, deletedGone, unknownTx int
	vals := make([]any, len(cols))
	logAt := func(page uint32, line int, column, reason string) error {
		e := ErrorLine{Table: t.Name, Page: page, Column: column, Reason: reason}
		if line >= 0 {
			e.Line = &line
		}
		return logError(e)
	}
	skip := func(page uint32, line int, reason string) error {
		tr.Skipped++
		return logAt(page, line, "", reason)
	}

	// values fills vals from a row version; a row-level problem is returned
	// as rowErr, column-level ones leave the column NULL and are logged.
	values := func(r meta.Row) (rowErr string, err error) {
		fm, ok := t.Formats[uint16(r.Header.Format)]
		switch {
		case !ok && t.FromDDL:
			return fmt.Sprintf("record format %d: the layout from the DDL is for format %d, the newest", r.Header.Format, t.CurrentFormat), nil
		case !ok:
			return fmt.Sprintf("record format %d is not in RDB$FORMATS", r.Header.Format), nil
		}
		for i, c := range cols {
			x, err := columnValue(r.Data, fm, c, opt.Value)
			switch {
			case err == errNoDefault:
				noDflt[c.Name]++
				err = nil
			case err != nil:
				return fmt.Sprintf("column %s: %v", c.Name, err), nil
			}
			x, cerr := resolve(cat, x, opt.Value)
			if cerr != nil {
				colErrs[c.Name]++
				if err := logAt(r.Page, r.Line, c.Name, cerr.Error()); err != nil {
					return "", err
				}
				x = nil
			}
			if s, ok := x.(value.Text); ok && s.Raw {
				raw[c.Name]++
			}
			vals[i] = x
		}
		return "", nil
	}

	err = cat.Records(t.ID, func(v meta.Version, rec []byte, rerr error) error {
		switch {
		case rerr != nil:
			return skip(v.Page, v.Line, rerr.Error())
		case v.Header.Flags&record.FlagDamaged != 0:
			return skip(v.Page, v.Line, "record is marked damaged by the engine")
		}
		r, found, err := cat.Current(v, rec)
		switch {
		case err != nil:
			return skip(v.Page, v.Line, err.Error())
		case !found:
			uncommitted++ // an insert that never committed: not a row
			return nil
		}
		if r.Older {
			older++
		}
		if r.Limbo {
			limbo++
		}
		if r.Unknown {
			unknownTx++
		}
		if r.Deleted {
			if !opt.IncludeDeleted {
				return nil
			}
			old, ok, err := cat.BeforeDelete(r)
			switch {
			case err != nil:
				return logAt(v.Page, v.Line, "", "deleted row: "+err.Error())
			case !ok:
				deletedGone++
				return nil
			}
			why, err := values(old)
			if err != nil || why != "" {
				if err == nil {
					err = logAt(v.Page, v.Line, "", "deleted row: "+why)
				}
				return err
			}
			if deleted == nil {
				tr.DeletedFile = base + ".deleted." + opt.Format
				dblobs := &blobFiles{dir: filepath.Join(opt.Dir, base+".deleted.blobs"), rel: base + ".deleted.blobs"}
				if deleted, err = openOutput(filepath.Join(opt.Dir, tr.DeletedFile), opt.Format, t, cols, dblobs); err != nil {
					return err
				}
			}
			tr.Deleted++
			return deleted.w.row(vals)
		}
		why, err := values(r)
		if err != nil {
			return err
		}
		if why != "" {
			return skip(v.Page, v.Line, why)
		}
		tr.Rows++
		return out.w.row(vals)
	})
	if err != nil {
		return err
	}
	tr.Warned = len(raw) > 0 || len(noDflt) > 0 || len(colErrs) > 0 || limbo > 0 || unknownTx > 0
	for c, n := range raw {
		res.Warnings = append(res.Warnings, fmt.Sprintf("%s.%s: %d values are not valid UTF-8 and were written unchanged; for CHARACTER SET NONE data try --charset", t.Name, c, n))
	}
	for c, n := range noDflt {
		res.Warnings = append(res.Warnings, fmt.Sprintf("%s.%s: %d rows predate the column and its default could not be evaluated; written as NULL", t.Name, c, n))
	}
	for c, n := range colErrs {
		res.Warnings = append(res.Warnings, fmt.Sprintf("%s.%s: %d values could not be read and were written as NULL; details in _errors.jsonl", t.Name, c, n))
	}
	if limbo > 0 {
		res.Warnings = append(res.Warnings, fmt.Sprintf("%s: %d rows have changes by limbo (prepared) transactions; their last committed version was written", t.Name, limbo))
	}
	if unknownTx > 0 {
		res.Warnings = append(res.Warnings, fmt.Sprintf("%s: for %d rows the transaction inventory is damaged and does not say whether the newest version committed; it was taken as committed", t.Name, unknownTx))
	}
	if blobs.n > 0 {
		res.Notes = append(res.Notes, fmt.Sprintf("%s: %d blobs over %d MB are in %s/ (NULL in the SQL file)", t.Name, blobs.n, blobInlineLimit>>20, blobs.rel))
	}
	if older > 0 {
		res.Notes = append(res.Notes, fmt.Sprintf("%s: %d rows were read from an older version, because the newest change did not commit", t.Name, older))
	}
	if uncommitted > 0 {
		res.Notes = append(res.Notes, fmt.Sprintf("%s: %d rows inserted by transactions that did not commit are not rows of the table and were left out", t.Name, uncommitted))
	}
	if opt.IncludeDeleted && deletedGone > 0 {
		res.Notes = append(res.Notes, fmt.Sprintf("%s: %d deleted rows have no older version left on disk", t.Name, deletedGone))
	}
	if deleted != nil {
		if err := deleted.close(); err != nil {
			return err
		}
		deleted = nil
	}
	return out.close()
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

// resolve reads blobs and checks values that need more than the record:
// an error here leaves the column NULL, the row is still exported.
func resolve(cat *meta.Catalog, x any, opt value.Options) (any, error) {
	switch v := x.(type) {
	case value.BlobRef:
		b, err := cat.Blob(record.BlobID{Relation: v.Relation, Number: v.Number})
		if err != nil {
			return nil, err
		}
		if v.SubType == 1 {
			return value.DecodeText(b, v.Charset, opt), nil
		}
		return value.Octets(b), nil
	case value.TimeTZ:
		_, err := v.Format()
		return x, err
	case value.TimestampTZ:
		_, err := v.Format()
		return x, err
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
				// The column keeps its place, so that its field id stays
				// that of the stored records (this file also works as --ddl).
				lines = append(lines, fmt.Sprintf("  %s COMPUTED BY (CAST(NULL AS %s)) -- the expression is not stored with the data", quote(c.Name), c.Field.SQLType()))
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
