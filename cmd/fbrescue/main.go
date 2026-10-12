// Command fbrescue reads Firebird database files without the Firebird engine
// and recovers data from them. It never modifies the source file.
package main

import (
	"fmt"
	"io"
	"os"
)

var version = "dev"

const usage = `fbrescue - read-only data recovery for Firebird 2.5-5.0 databases.
The source database file is never modified.

Usage:
  fbrescue <command> [arguments]

Commands (planned, not implemented yet):
  info    <db>                header, ODS version, flags, warnings
  scan    <db>                page map, damage, per-table summary
  tables  <db>                recovered schema (DDL)
  export  <db> --out <dir>    export data as csv, sql or jsonl
  page    <db> <N>            decode a single page
  report  <db>                anonymized diagnostic report for bug reports

  version                     print the version
`

var planned = map[string]bool{
	"info": true, "scan": true, "tables": true,
	"export": true, "page": true, "report": true,
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 1
	}
	switch cmd := args[0]; {
	case cmd == "-h" || cmd == "--help" || cmd == "help":
		fmt.Fprint(stdout, usage)
		return 0
	case cmd == "version":
		fmt.Fprintln(stdout, "fbrescue", version)
		return 0
	case planned[cmd]:
		fmt.Fprintf(stderr, "fbrescue %s: not implemented yet\n", cmd)
		return 1
	default:
		fmt.Fprintf(stderr, "fbrescue: unknown command %q\n\n%s", cmd, usage)
		return 1
	}
}
