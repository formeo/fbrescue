# fbrescue

Read-only data recovery for Firebird 2.5–5.0 databases.

`fbrescue` reads `.fdb` / `.gdb` files directly, without the Firebird engine, and gets data out of databases that `gbak` cannot back up. **It never modifies the source file.**

> **Status: v0.1 in progress.** `info` and `scan` analyze a database; data export arrives in v0.2. Until then, use `gfix` and `gbak` for real recoveries.

## Usage

```
$ go install github.com/formeo/fbrescue/cmd/fbrescue@latest

$ fbrescue info shop.fdb
File        shop.fdb · 2.4 MB · 307 pages × 8 KB
Engine      ODS 13.1 (Firebird 5.0) · dialect 3
Created     2026-10-12 12:00:38 (UTC)
Tx          OIT 21 · OAT 22 · OST 22 · next 22
Attributes  force write

$ fbrescue scan shop.fdb
Pages         307 total · 287 in use · 20 free
              data 141 · index 61 · index root 40 · pointer 40 · generator 1 · header 1 · page inventory 1 · transaction inventory 1 · type 10 1
Transactions  1-22: committed 22 · active 0 · dead 0 · limbo 0

Table    Pointer pages Data pages    Records   Deleted  Versions
128                  1         40       2700         0       386
129                  1         16       1000       250       250
(38 system tables, 3332 records; table names arrive in v0.2)

No problems found.
```

Both commands take `--json`. `scan` exits with status 2 when it finds problems: pages holding another page's contents, pages in use with an invalid type, broken pointer page chains, data pages no table references, missing transaction inventory pages, truncated files. A damaged header page is not fatal: the page size is detected from the other pages.

| Command | Status |
|---|---|
| `info <db>` | header, ODS version, flags, warnings (nbackup lock, damaged header, truncated file) |
| `scan <db>` | page map, damage, per-table summary |
| `tables <db>` | planned: recovered schema (DDL) |
| `export <db> --out <dir>` | planned: data as csv, sql or jsonl |
| `page <db> <N>` | planned: decode a single page |
| `report <db>` | planned: anonymized diagnostic report for bug reports |

A single static binary for Windows, Linux and macOS: no fbclient, no cgo. Multi-file databases are supported: secondary files are looked up next to the main file first.

## Roadmap

| Version | Scope |
|---|---|
| v0.1 | `info` and `scan`: correct analysis of Firebird 2.5–5.0, damaged files open |
| v0.2 | `export` of intact databases: core types, CSV and SQL, `tables` |
| v0.3 | BLOBs, record versions, deleted rows, Firebird 4 types, `page` |
| v0.4 | damaged-database corpus, published recovery rate, export with a user-supplied DDL |
| v1.0 | load into a new database, stable JSON reports, package managers |

## How correctness is checked

Every assumption about the on-disk format is tested against databases created by real Firebird engines; [docs/format.md](docs/format.md) lists each one and how it was verified. `tools/mkcorpus/mkcorpus.sh` downloads Firebird 2.5, 3.0, 4.0 and 5.0 and creates about 60 test databases in embedded mode: plain data, deleted rows, each header flag fbrescue names, multi-file, three TIP pages, more than one page inventory range. Each engine's own `gstat -h` and `gstat -d -r -s` output is saved as the reference. CI runs the tests against that corpus, including damaged copies (zeroed, misplaced and truncated pages, broken chains) and a check that no command ever changes the database file.

```sh
# Ubuntu: sudo apt-get install libtommath1 libncurses6
tools/mkcorpus/mkcorpus.sh corpus
FBRESCUE_CORPUS=$PWD/corpus go test ./...
```

## Background

fbrescue replaces [FirebirdRepairBase](https://github.com/formeo/FirebirdRepairBase), whose problems are listed in [FirebirdRepairBase#10](https://github.com/formeo/FirebirdRepairBase/issues/10).

## License

MIT
