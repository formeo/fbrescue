# fbrescue

Read-only data recovery for Firebird 2.5–5.0 databases.

`fbrescue` reads `.fdb` / `.gdb` files directly, without the Firebird engine, and gets data out of databases that `gbak` cannot back up. **It never modifies the source file.**

> **Status: pre-alpha.** Nothing works yet — this is the project skeleton. Until a release says otherwise, use `gfix` and `gbak` for real recoveries.

## Planned commands

```
fbrescue info   <db>                header, ODS version, flags, warnings
fbrescue scan   <db>                page map, damage, per-table summary
fbrescue tables <db>                recovered schema (DDL)
fbrescue export <db> --out <dir>    export data as csv, sql or jsonl
fbrescue page   <db> <N>            decode a single page
fbrescue report <db>                anonymized diagnostic report for bug reports
```

A single static binary for Windows, Linux and macOS: no fbclient, no cgo.

## Roadmap

| Version | Scope |
|---|---|
| v0.1 | `info` and `scan`: correct analysis of Firebird 2.5–5.0, damaged files open |
| v0.2 | `export` of intact databases: core types, CSV and SQL, `tables` |
| v0.3 | BLOBs, record versions, deleted rows, Firebird 4 types, `page` |
| v0.4 | damaged-database corpus, published recovery rate, export with a user-supplied DDL |
| v1.0 | load into a new database, stable JSON reports, package managers |

## How correctness is checked

Every assumption about the on-disk format is tested against databases created by real Firebird engines. `tools/mkcorpus/mkcorpus.sh` downloads Firebird 2.5, 3.0, 4.0 and 5.0, creates test databases in embedded mode (plain data, deleted rows, read-only, shutdown, dialect 1, nbackup lock) and saves each engine's `gstat -h` output as the reference. CI runs the tests against that corpus.

```sh
# Ubuntu: sudo apt-get install libtommath1 libncurses6
tools/mkcorpus/mkcorpus.sh corpus
FBRESCUE_CORPUS=$PWD/corpus go test ./...
```

## Background

fbrescue replaces [FirebirdRepairBase](https://github.com/formeo/FirebirdRepairBase), whose problems are listed in [FirebirdRepairBase#10](https://github.com/formeo/FirebirdRepairBase/issues/10).

## License

MIT
