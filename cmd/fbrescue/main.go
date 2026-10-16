// Command fbrescue reads Firebird database files without the Firebird engine
// and recovers data from them. It never modifies the source file.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

var version = "dev"

const usage = `fbrescue - read-only data recovery for Firebird 2.5-5.0 databases.
The source database file is never modified.

Usage:
  fbrescue <command> [options] <db>

Commands:
  info    <db>                header, ODS version, flags, warnings
  scan    <db>                page map, damage, per-table summary
  tables  <db>                schema of the user tables (CREATE TABLE)
  export  <db> --out <dir>    export the user tables

Options:
  --json                      machine-readable output (info, scan, tables, export summary)
  --format csv|sql|jsonl      export: file format (default csv)
  --table NAME                export: only this table; repeat for more
  --charset NAME              export: decode CHARACTER SET NONE text as NAME, e.g. WIN1251
  --include-deleted           export: also write deleted rows still on disk, to <table>.deleted.<format>
  --versions all              export: also write every record version still on disk, with its
                              transaction and state, to <table>.versions.<format>
  --ddl FILE                  export, tables: CREATE TABLE statements for tables whose definition
                              is lost from the database (e.g. a schema.sql of an earlier export)

  page    <db> <N>            decode one page: header, pointer slots, records and their values

  report  <db>                diagnostic report to attach to a bug report, as JSON: the
                              database structure and the problems found, no data, names
                              replaced by numbers

  version                     print the version

Exit status: 0 ok, 1 error, 2 problems found (scan) or data left out (export).
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 1
	}
	cmd, rest := args[0], args[1:]
	switch {
	case cmd == "-h" || cmd == "--help" || cmd == "help":
		fmt.Fprint(stdout, usage)
		return 0
	case cmd == "version":
		fmt.Fprintln(stdout, "fbrescue", version)
		return 0
	case cmd == "info" || cmd == "scan" || cmd == "tables" || cmd == "export" || cmd == "page" || cmd == "report":
		a, err := parseArgs(cmd, rest)
		if err != nil {
			fmt.Fprintf(stderr, "fbrescue %s: %v\n", cmd, err)
			return 1
		}
		switch cmd {
		case "info":
			return runInfo(a.path, a.json, stdout, stderr)
		case "scan":
			return runScan(a.path, a.json, stdout, stderr)
		case "tables":
			return runTables(a, stdout, stderr)
		case "page":
			return runPage(a, stdout, stderr)
		case "report":
			return runReport(a, stdout, stderr)
		}
		return runExport(a, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "fbrescue: unknown command %q\n\n%s", cmd, usage)
		return 1
	}
}

type cliArgs struct {
	path           string
	json           bool
	includeDeleted bool
	out            string
	format         string
	charset        string
	ddl            string
	versions       string
	tables         []string
	pageNo         string // page: the page number
}

// parseArgs accepts one database path and options anywhere on the line,
// as --name value or --name=value.
func parseArgs(cmd string, in []string) (cliArgs, error) {
	a := cliArgs{format: "csv"}
	valued := map[string]*string{"out": &a.out, "format": &a.format, "charset": &a.charset, "ddl": &a.ddl, "versions": &a.versions, "table": nil}
	var paths []string
	for i := 0; i < len(in); i++ {
		arg := in[i]
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			paths = append(paths, arg)
			continue
		}
		name, val, hasVal := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		if name == "json" && !hasVal {
			a.json = true
			continue
		}
		if name == "include-deleted" && !hasVal && cmd == "export" {
			a.includeDeleted = true
			continue
		}
		dst, ok := valued[name]
		if !ok || cmd != "export" && !(cmd == "tables" && name == "ddl") {
			return a, fmt.Errorf("unknown option %s", arg)
		}
		if !hasVal {
			if i+1 >= len(in) {
				return a, fmt.Errorf("option %s needs a value", arg)
			}
			i++
			val = in[i]
		}
		if dst == nil {
			a.tables = append(a.tables, val)
		} else {
			*dst = val
		}
	}
	if cmd == "page" {
		if len(paths) != 2 {
			return a, fmt.Errorf("expected a database file and a page number")
		}
		a.pageNo, paths = paths[1], paths[:1]
	}
	if len(paths) != 1 {
		return a, fmt.Errorf("expected one database file, got %d arguments", len(paths))
	}
	a.path = paths[0]
	if cmd == "export" {
		if a.out == "" {
			return a, fmt.Errorf("--out <directory> is required")
		}
		if a.format != "csv" && a.format != "sql" && a.format != "jsonl" {
			return a, fmt.Errorf("unknown format %q: use csv, sql or jsonl", a.format)
		}
		if a.versions != "" && a.versions != "all" && a.versions != "visible" {
			return a, fmt.Errorf("unknown --versions %q: use all or visible", a.versions)
		}
	}
	return a, nil
}

func writeJSON(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
