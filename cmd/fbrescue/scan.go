package main

import (
	"cmp"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/formeo/fbrescue/internal/fdb"
	"github.com/formeo/fbrescue/internal/meta"
	"github.com/formeo/fbrescue/internal/scan"
)

const maxProblemsShown = 20

type scanJSON struct {
	Warnings []string `json:"warnings"`
	*scan.Report
}

func runScan(path string, asJSON bool, stdout, stderr io.Writer) int {
	db, err := fdb.Open(path)
	if err != nil {
		fmt.Fprintf(stderr, "fbrescue scan: %v\n", err)
		return 1
	}
	defer db.Close()

	r := scan.Run(db, progressReporter(stderr))
	warns := warnings(db)
	status := 0
	if len(r.Problems) > 0 || len(db.Warnings) > 0 {
		status = 2
	}
	if asJSON {
		writeJSON(stdout, scanJSON{warns, r})
		return status
	}

	fmt.Fprintf(stdout, "Pages         %d total · %d in use · %d free\n", r.Pages, r.UsedPages, r.FreePages)
	types := make([]string, 0, len(r.PageTypes))
	for name := range r.PageTypes {
		types = append(types, name)
	}
	slices.SortFunc(types, func(a, b string) int {
		return cmp.Or(cmp.Compare(r.PageTypes[b], r.PageTypes[a]), cmp.Compare(a, b))
	})
	parts := make([]string, len(types))
	for i, name := range types {
		parts[i] = fmt.Sprintf("%s %d", name, r.PageTypes[name])
	}
	fmt.Fprintf(stdout, "              %s\n", strings.Join(parts, " · "))
	if tx := r.Transactions; tx != nil {
		fmt.Fprintf(stdout, "Transactions  1-%d: committed %d · active %d · dead %d · limbo %d\n",
			tx.Next, tx.Committed, tx.Active, tx.Dead, tx.Limbo)
	} else {
		fmt.Fprintln(stdout, "Transactions  unknown (header page or transaction inventory damaged)")
	}

	// Names come from the system tables; on a database too damaged for
	// that, the relation ids are shown instead.
	names := map[uint16]string{}
	if cat, err := meta.Load(db); err == nil {
		for _, t := range cat.Tables {
			names[t.ID] = t.Name
		}
	}
	var system, systemRecords int
	fmt.Fprintf(stdout, "\n%-24s %13s %10s %10s %9s %9s\n", "Table", "Pointer pages", "Data pages", "Records", "Deleted", "Versions")
	for _, rel := range r.Relations {
		if rel.ID < 128 { // system tables
			system++
			systemRecords += rel.Records
			continue
		}
		name := names[rel.ID]
		if name == "" {
			name = fmt.Sprintf("#%d", rel.ID)
		}
		fmt.Fprintf(stdout, "%-24s %13d %10d %10d %9d %9d\n", name, rel.PointerPages, rel.DataPages, rel.Records, rel.Deleted, rel.Versions)
	}
	fmt.Fprintf(stdout, "(%d system tables, %d records)\n\n", system, systemRecords)

	for _, w := range warns {
		fmt.Fprintf(stdout, "⚠ %s\n", w)
	}
	if len(r.Problems) == 0 {
		fmt.Fprintln(stdout, "No problems found.")
		return status
	}
	fmt.Fprintf(stdout, "%d problems:\n", len(r.Problems))
	for i, p := range r.Problems {
		if i == maxProblemsShown {
			fmt.Fprintf(stdout, "… and %d more (all of them with --json)\n", len(r.Problems)-i)
			break
		}
		fmt.Fprintf(stdout, "  page %-9d %-18s %s\n", p.Page, p.Kind, p.Message)
	}
	return status
}

// progressReporter prints a percentage to the terminal; nothing when stderr
// is redirected.
func progressReporter(w io.Writer) func(done, total uint32) {
	f, ok := w.(*os.File)
	if !ok {
		return nil
	}
	if st, err := f.Stat(); err != nil || st.Mode()&os.ModeCharDevice == 0 {
		return nil
	}
	return func(done, total uint32) {
		if done == total {
			fmt.Fprint(w, "\r              \r")
			return
		}
		fmt.Fprintf(w, "\rscanning %3d%%", uint64(done)*100/uint64(max(total, 1)))
	}
}
