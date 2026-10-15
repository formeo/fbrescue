# fbrescue

Read-only data recovery for Firebird 2.5–5.0 databases.

`fbrescue` reads `.fdb` / `.gdb` files directly, without the Firebird engine, and gets data out of databases that `gbak` cannot back up. **It never modifies the source file.**

> **Status: v0.3.** `info` and `scan` analyze a database; `tables` and `export` get the schema and the data of intact databases out without the engine, including BLOBs, Firebird 4 types and the committed version of rows changed by transactions that did not commit; `page` shows a single page. Not exported yet: arrays and records with 48-bit transaction numbers — reported, never silently dropped. Damaged databases are the subject of v0.4; for real recoveries try `gfix` and `gbak` first.

## Usage

```
$ go install github.com/formeo/fbrescue/cmd/fbrescue@latest

$ fbrescue info shop.fdb
File        shop.fdb · 2.4 MB · 307 pages × 8 KB
Engine      ODS 13.1 (Firebird 5.0) · dialect 3
Created     2026-10-14 08:48:59 (UTC)
Tx          OIT 21 · OAT 22 · OST 22 · next 22
Attributes  force write

$ fbrescue scan shop.fdb
Pages         307 total · 287 in use · 20 free
              data 141 · index 61 · index root 40 · pointer 40 · generator 1 · header 1 · page inventory 1 · transaction inventory 1 · type 10 1
Transactions  1-22: committed 22 · active 0 · dead 0 · limbo 0

Table                    Pointer pages Data pages    Records   Deleted  Versions
T                                    1         40       2700         0       386
T2                                   1         16       1000       250       250
(38 system tables, 3332 records)

No problems found.

$ fbrescue export shop.fdb --out rescued --format sql
T                                    2700 rows  ✓
T2                                    750 rows  ✓
Wrote rescued: schema.sql and 2 sql files
```

| Command | What it does |
|---|---|
| `info <db>` | header, ODS version, flags, warnings (nbackup lock, damaged header, truncated file) |
| `scan <db>` | page map, damage, per-table summary. Exit status 2 when it finds problems: pages holding another page's contents, pages in use with an invalid type, broken pointer page chains, data pages no table references, missing transaction inventory pages, truncated files |
| `tables <db>` | `CREATE TABLE` statements for the user tables, from the stored record formats |
| `export <db> --out <dir>` | the rows of every user table, plus `schema.sql`. `--format csv` (default), `sql` (INSERTs for isql) or `jsonl`; `--table NAME` to pick tables; `--charset WIN1251` to decode text stored in CHARACTER SET NONE; `--include-deleted` to also write deleted rows whose old version is still on disk to `<table>.deleted.<format>`. Exit status 2 when something was left out: the reasons are printed, and rows that could not be read are listed in `_errors.jsonl` |
| `page <db> <N>` | one page decoded: type and header fields; for data pages every record with its flags, transaction state, version chain and values; pointer pages list their data pages |
| `report <db>` | planned: anonymized diagnostic report for bug reports |

All commands take `--json`. A single static binary for Windows, Linux and macOS: no fbclient, no cgo. Multi-file databases are supported: secondary files are looked up next to the main file first. A damaged header page is not fatal for `info` and `scan`: the page size is detected from the other pages.

What export handles: every column type of Firebird 2.5–5.0 except arrays — BLOBs of every level, segmented and stream; INT128, DECFLOAT, TIME/TIMESTAMP WITH TIME ZONE with region zones; text in UTF8, UNICODE_FSS and the single-byte character sets (WIN125x, KOI8R/U, ISO8859_x, DOSxxx; not CYRL and NEXT); tables altered many times (each row is read with the format it was written with, columns added later get the default the engine would show); records larger than a page; the record encodings of Firebird 2.5 to 5.0. CSV keeps NULL (empty field) apart from an empty string (`""`). Each row is exported as SELECT would see it after all transactions finished: the newest committed version, read through the chain of older versions, including versions stored as differences. In tests a 140 MB database exports to CSV in under a second.

BLOBs in SQL exports: values up to 1 MB are written inline as joined literals; larger ones go to `<table>.blobs/<row>.<column>.txt|bin`, with NULL and a comment naming the file in the INSERT. CSV and JSON Lines always hold the whole value. Firebird 2.5's isql reads statements of at most 64 KB, so SQL files with large inline values load only with isql 3.0 or later.

## Roadmap

| Version | Scope |
|---|---|
| v0.1 | `info` and `scan`: correct analysis of Firebird 2.5–5.0, damaged files open |
| v0.2 | `export` of intact databases: core types, CSV and SQL, `tables` |
| v0.3 | BLOBs, record versions, deleted rows, Firebird 4 types, `page` |
| v0.4 | damaged-database corpus, published recovery rate, export with a user-supplied DDL |
| v1.0 | load into a new database, stable JSON reports, package managers |

## How correctness is checked

Every assumption about the on-disk format is tested against databases created by real Firebird engines; [docs/format.md](docs/format.md) lists each one and how it was verified. `tools/mkcorpus/mkcorpus.sh` downloads Firebird 2.5, 3.0, 4.0 and 5.0 and creates 75 test databases in embedded mode: plain data, deleted rows, each header flag fbrescue names, multi-file, three TIP pages, more than one page inventory range, every exported column type with edge values, tables altered seven times, records larger than a page, rolled-back changes and record versions left on disk, BLOBs of every level, Firebird 4 types with edge values. Each engine's own `gstat -h` and `gstat -d -r -s` output is saved as the reference.

The main export test is a round trip through the engine: the SQL export is loaded into a new database by the same Firebird version, and `SELECT` on it must print exactly what `SELECT` on the original prints — every value of every row, in the engine's own formatting. CI runs all tests against the corpus, including damaged copies (zeroed, misplaced and truncated pages, broken chains) and a check that no command ever changes the database file.

```sh
# Ubuntu: sudo apt-get install libtommath1 libncurses6
tools/mkcorpus/mkcorpus.sh corpus
FBRESCUE_CORPUS=$PWD/corpus go test ./...
```

## Background

fbrescue replaces [FirebirdRepairBase](https://github.com/formeo/FirebirdRepairBase), whose problems are listed in [FirebirdRepairBase#10](https://github.com/formeo/FirebirdRepairBase/issues/10).

## License

MIT
