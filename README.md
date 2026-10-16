# fbrescue

Read-only data recovery for Firebird 2.5–5.0 databases.

`fbrescue` reads `.fdb` / `.gdb` files directly, without the Firebird engine, and gets data out of databases that `gbak` cannot back up. **It never modifies the source file.**

> **Status: v0.4.** `info` and `scan` analyze a database; `tables` and `export` get the schema and the data out without the engine — from intact databases, and from damaged ones as far as the pages allow ([recovery rate](#recovery-rate)); `page` shows a single page; `report` makes a diagnostic report without data. Not exported yet: arrays and records with 48-bit transaction numbers — reported, never silently dropped. On a damaged database, work on a copy and try `gfix` and `gbak` first: they are the engine's own tools.

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
| `export <db> --out <dir>` | the rows of every user table, plus `schema.sql`. `--format csv` (default), `sql` (INSERTs for isql) or `jsonl`; `--table NAME` to pick tables; `--charset WIN1251` to decode text stored in CHARACTER SET NONE; `--include-deleted` to also write deleted rows whose old version is still on disk to `<table>.deleted.<format>`; `--versions all` to write every version of every row still on disk, with its transaction and that transaction's state, to `<table>.versions.<format>`; `--ddl schema.sql` for tables whose definition is lost (see below). Exit status 2 when something was left out: the reasons are printed, and rows that could not be read are listed in `_errors.jsonl` |
| `page <db> <N>` | one page decoded: type and header fields; for data pages every record with its flags, transaction state, version chain and values; pointer pages list their data pages |
| `report <db>` | a JSON report to attach to a bug report: header, page map, problems found, what exporting each table ran into. No values; table and column names are replaced by numbers |

All commands take `--json`. A single static binary for Windows, Linux and macOS: no fbclient, no cgo. Multi-file databases are supported: secondary files are looked up next to the main file first. A damaged header page is not fatal for `info` and `scan`: the page size is detected from the other pages.

What export handles: every column type of Firebird 2.5–5.0 except arrays — BLOBs of every level, segmented and stream; INT128, DECFLOAT, TIME/TIMESTAMP WITH TIME ZONE with region zones; text in UTF8, UNICODE_FSS and the single-byte character sets (WIN125x, KOI8R/U, ISO8859_x, DOSxxx; not CYRL and NEXT); tables altered many times (each row is read with the format it was written with, columns added later get the default the engine would show); records larger than a page; the record encodings of Firebird 2.5 to 5.0. CSV keeps NULL (empty field) apart from an empty string (`""`). Each row is exported as SELECT would see it after all transactions finished: the newest committed version, read through the chain of older versions, including versions stored as differences. In tests a 140 MB database exports to CSV in under a second.

BLOBs in SQL exports: values up to 1 MB are written inline as joined literals; larger ones go to `<table>.blobs/<row>.<column>.txt|bin`, with NULL and a comment naming the file in the INSERT. CSV and JSON Lines always hold the whole value. Firebird 2.5's isql reads statements of at most 64 KB, so SQL files with large inline values load only with isql 3.0 or later.

## Damaged databases

What export does when the file is damaged, instead of stopping:

- **Pointer pages or RDB$PAGES damaged:** every data page records its table and its place in the table, so the pages the pointer pages no longer reach are found by reading the whole file. Only pages the page inventory marks as in use are taken, so freed pages with old rows do not come back.
- **Pages in the wrong place** (Firebird 3+ pages carry their own number): read from where they are, and never taken for the page whose place they occupy.
- **Transaction inventory damaged:** versions of transactions older than the oldest interesting one are committed by definition. For newer ones whose state is lost, the newest version is taken as committed, and the export says for how many rows.
- **Header page damaged:** the page size comes from the other pages, the ODS version from the system tables (the one under which they read their own names).
- **Table or column definitions damaged:** tables missing from RDB$RELATIONS come out as `TABLE_<id>`, columns missing from RDB$RELATION_FIELDS as `FIELD_<id>`. When the record formats themselves are lost, `--ddl` takes CREATE TABLE statements — the database's creation script, `isql -x` output, or the `schema.sql` of an earlier fbrescue export — and computes the record layout the engine would give those tables. A definition is matched to its table by name, or, when the name is lost too, by the length of the table's records. It works for tables whose columns were never dropped or changed.

Anything left out or uncertain is in the summary and in `_errors.jsonl`, and the exit status is 2.

### Recovery rate

`TestRecovery` damages copies of a small shop database (three tables, 21 230 rows, indexes, text blobs) in each way below, ten copies per kind on each of Firebird 2.5, 3.0, 4.0 and 5.0, exports them, and compares every row with the export of the intact database by primary key. *Changed, reported*: a value could not be read, was written as NULL and is listed in `_errors.jsonl`. *Changed silently* and *extra rows*: wrong values with no warning — bytes changed inside a record, which Firebird pages carry no checksum to detect. *With --ddl*: the same, given the script that created the database.

| Damage | Copies | Rows recovered | With --ddl | Changed, reported | Changed silently | Extra rows | Export failed |
|---|--:|--:|--:|--:|--:|--:|--:|
| zeroed data page | 40 | 99.8% | 99.8% | 2 | 0 | 0 | 0 |
| zeroed index page | 40 | 100.0% | 100.0% | 0 | 0 | 0 | 0 |
| zeroed blob page | 40 | 99.9% | 99.9% | 40 | 0 | 0 | 0 |
| zeroed pointer page | 40 | 100.0% | 100.0% | 0 | 0 | 0 | 0 |
| zeroed system table page | 40 | 95.0% | 100.0% | 0 | 0 | 0 | 0 |
| zeroed transaction inventory page | 40 | 100.0% | 100.0% | 0 | 0 | 0 | 0 |
| zeroed page inventory page | 40 | 100.0% | 100.0% | 0 | 0 | 0 | 0 |
| zeroed header page | 40 | 100.0% | 100.0% | 0 | 0 | 0 | 0 |
| page of random bytes | 40 | 99.8% | 99.8% | 58 | 0 | 0 | 0 |
| 16 flipped bits | 40 | 99.9% | 99.9% | 7 | 103 | 31 | 0 |
| zeroed 64 KB block | 40 | 90.3% | 97.8% | 379 | 0 | 0 | 0 |
| two pages swapped | 40 | 99.9% | 99.9% | 1 | 0 | 0 | 0 |
| truncated file | 40 | 68.3% | 68.3% | 160 | 0 | 0 | 0 |

Rows on a page that is zeroed or cut off are gone; the rest is recovered. The test fails if these numbers get worse.

## Roadmap

| Version | Scope |
|---|---|
| v0.1 | `info` and `scan`: correct analysis of Firebird 2.5–5.0, damaged files open |
| v0.2 | `export` of intact databases: core types, CSV and SQL, `tables` |
| v0.3 | BLOBs, record versions, deleted rows, Firebird 4 types, `page` |
| v0.4 | damaged databases: page structure rebuilt from the pages, published recovery rate, `--ddl`, `report`, `--versions all` |
| v1.0 | load into a new database, stable JSON reports, package managers |

## How correctness is checked

Every assumption about the on-disk format is tested against databases created by real Firebird engines; [docs/format.md](docs/format.md) lists each one and how it was verified. `tools/mkcorpus/mkcorpus.sh` downloads Firebird 2.5, 3.0, 4.0 and 5.0 and creates 79 test databases in embedded mode: plain data, deleted rows, each header flag fbrescue names, multi-file, three TIP pages, more than one page inventory range, every exported column type with edge values, tables altered seven times, records larger than a page, rolled-back changes and record versions left on disk, BLOBs of every level, Firebird 4 types with edge values, a shop database for the damage tests. Each engine's own `gstat -h` and `gstat -d -r -s` output is saved as the reference.

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
