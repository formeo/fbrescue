package main

import (
	"fmt"
	"io"

	"github.com/formeo/fbrescue/internal/export"
	"github.com/formeo/fbrescue/internal/fdb"
	"github.com/formeo/fbrescue/internal/meta"
	"github.com/formeo/fbrescue/internal/value"
)

func openCatalog(cmd, path string, stderr io.Writer) (*fdb.File, *meta.Catalog, bool) {
	db, err := fdb.Open(path)
	if err != nil {
		fmt.Fprintf(stderr, "fbrescue %s: %v\n", cmd, err)
		return nil, nil, false
	}
	cat, err := meta.Load(db)
	if err != nil {
		db.Close()
		fmt.Fprintf(stderr, "fbrescue %s: %v\n", cmd, err)
		return nil, nil, false
	}
	return db, cat, true
}

type columnJSON struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	NotNull  bool   `json:"not_null"`
	Exported bool   `json:"exported"`
	Note     string `json:"note,omitempty"`
}

type tableJSON struct {
	ID      uint16       `json:"id"`
	Name    string       `json:"name"`
	Formats int          `json:"formats"`
	Columns []columnJSON `json:"columns"`
}

func runTables(a cliArgs, stdout, stderr io.Writer) int {
	db, cat, ok := openCatalog("tables", a.path, stderr)
	if !ok {
		return 1
	}
	defer db.Close()
	if a.json {
		out := struct {
			Tables   []tableJSON `json:"tables"`
			Warnings []string    `json:"warnings"`
		}{Tables: []tableJSON{}, Warnings: append(warnings(db), cat.Warnings...)}
		for _, t := range cat.Tables {
			tj := tableJSON{ID: t.ID, Name: t.Name, Formats: len(t.Formats), Columns: []columnJSON{}}
			for _, c := range t.Columns {
				ok, why := export.Exported(c)
				tj.Columns = append(tj.Columns, columnJSON{c.Name, c.Field.SQLType(), c.NotNull, ok, why})
			}
			out.Tables = append(out.Tables, tj)
		}
		writeJSON(stdout, out)
		return 0
	}
	if err := export.WriteSchema(stdout, cat.Tables); err != nil {
		fmt.Fprintf(stderr, "fbrescue tables: %v\n", err)
		return 1
	}
	for _, w := range append(warnings(db), cat.Warnings...) {
		fmt.Fprintf(stderr, "⚠ %s\n", w)
	}
	return 0
}

func runExport(a cliArgs, stdout, stderr io.Writer) int {
	opt := export.Options{Format: a.format, Dir: a.out, Tables: a.tables, IncludeDeleted: a.includeDeleted}
	if a.charset != "" {
		id, ok := value.CharsetByName(a.charset)
		if !ok || !value.Decodable(id) {
			fmt.Fprintf(stderr, "fbrescue export: unknown or unsupported character set %q\n", a.charset)
			return 1
		}
		opt.Value.NoneCharset = id
	}
	db, cat, ok := openCatalog("export", a.path, stderr)
	if !ok {
		return 1
	}
	defer db.Close()

	progress := func(t *export.TableResult) {
		if a.json {
			return
		}
		mark := "✓"
		if t.Skipped > 0 || len(t.Omitted) > 0 || t.Warned {
			mark = "⚠"
		}
		fmt.Fprintf(stdout, "%-30s %10d rows  %s", t.Name, t.Rows, mark)
		if t.Skipped > 0 {
			fmt.Fprintf(stdout, " %d skipped", t.Skipped)
		}
		if len(t.Omitted) > 0 {
			fmt.Fprintf(stdout, " %d columns left out", len(t.Omitted))
		}
		if t.Deleted > 0 {
			fmt.Fprintf(stdout, " + %d deleted rows in %s", t.Deleted, t.DeletedFile)
		}
		fmt.Fprintln(stdout)
	}
	res, err := export.Run(cat, opt, progress)
	if err != nil {
		fmt.Fprintf(stderr, "fbrescue export: %v\n", err)
		return 1
	}
	res.Warnings = append(warnings(db), res.Warnings...)
	status := 0
	if res.Partial() {
		status = 2
	}
	if a.json {
		writeJSON(stdout, res)
		return status
	}
	fmt.Fprintf(stdout, "Wrote %s: schema.sql and %d %s files\n", a.out, len(res.Tables), a.format)
	if res.Errors > 0 {
		fmt.Fprintf(stdout, "%d rows or pages could not be exported: details in _errors.jsonl\n", res.Errors)
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(stdout, "⚠ %s\n", w)
	}
	for _, n := range res.Notes {
		fmt.Fprintf(stdout, "  %s\n", n)
	}
	return status
}
