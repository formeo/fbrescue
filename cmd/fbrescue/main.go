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
  fbrescue <command> [--json] <db>

Commands:
  info    <db>                header, ODS version, flags, warnings
  scan    <db>                page map, damage, per-table summary

Planned:
  tables  <db>                recovered schema (DDL)
  export  <db> --out <dir>    export data as csv, sql or jsonl
  page    <db> <N>            decode a single page
  report  <db>                anonymized diagnostic report for bug reports

  version                     print the version

Exit status: 0 ok, 1 error, 2 the database has problems (scan).
`

var planned = map[string]bool{"tables": true, "export": true, "page": true, "report": true}

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
	case cmd == "info" || cmd == "scan":
		path, asJSON, err := parseArgs(rest)
		if err != nil {
			fmt.Fprintf(stderr, "fbrescue %s: %v\n", cmd, err)
			return 1
		}
		if cmd == "info" {
			return runInfo(path, asJSON, stdout, stderr)
		}
		return runScan(path, asJSON, stdout, stderr)
	case planned[cmd]:
		fmt.Fprintf(stderr, "fbrescue %s: not implemented yet\n", cmd)
		return 1
	default:
		fmt.Fprintf(stderr, "fbrescue: unknown command %q\n\n%s", cmd, usage)
		return 1
	}
}

// parseArgs accepts exactly one database path and an optional --json
// anywhere on the line.
func parseArgs(args []string) (path string, asJSON bool, err error) {
	var paths []string
	for _, a := range args {
		switch {
		case a == "--json" || a == "-json":
			asJSON = true
		case strings.HasPrefix(a, "-") && a != "-":
			return "", false, fmt.Errorf("unknown option %s", a)
		default:
			paths = append(paths, a)
		}
	}
	if len(paths) != 1 {
		return "", false, fmt.Errorf("expected one database file, got %d arguments", len(paths))
	}
	return paths[0], asJSON, nil
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
