# Changelog

Each version's section is also its release notes on GitHub
(`tools/release/notes.sh`).

## v0.4.0 — 2026-10-16

The first release with binaries. fbrescue now gets data out of damaged
databases, not only intact ones.

- **Damaged page structure.** Export no longer stops at damaged pointer
  pages, RDB$PAGES, the transaction inventory or the header page. Data
  pages are found by scanning the file, misplaced pages are read from
  where they are (Firebird 3+), and the ODS version of a database with a
  damaged header is worked out from its system tables.
- **Lost table definitions.** Tables missing from RDB$RELATIONS are
  exported as `TABLE_<id>`, and columns missing from RDB$RELATION_FIELDS
  as `FIELD_<id>`. When the record formats themselves are lost,
  `export --ddl schema.sql` reads the tables with layouts computed from
  CREATE TABLE statements. Those can be the creation script, `isql -x`
  output, or the `schema.sql` of an earlier export.
- **Recovery rate.** The README has a table of how many rows come back
  from each kind of damage, measured on 520 damaged copies of a test
  database. Corrupt bytes inside a record can't be detected and are
  counted as such.
- **`report`.** A JSON report to attach to a bug report: the database
  structure and the problems found. It contains no data, and table and
  column names are replaced by numbers.
- **`export --versions all`.** Writes every version of every row still
  on disk, with its transaction and that transaction's state.
- **Fix:** rows of another table's data page are no longer read as rows
  of the table whose pointer page lists it.
- **Unchanged:** records with 48-bit transaction numbers and array
  columns are still reported as unsupported.

## v0.3.0 — 2026-10-15 (not released as binaries)

- BLOBs of every level, segmented and stream.
- Record versions: the newest committed version of each row, including
  versions stored as differences.
- `--include-deleted`.
- Firebird 4 types: INT128, DECFLOAT, TIME/TIMESTAMP WITH TIME ZONE.
- The `page` command.

## v0.2.0 — 2026-10-14 (not released as binaries)

- `tables` and `export` of intact databases to CSV, SQL and JSON Lines,
  checked by a round trip through each engine.

## v0.1.0 — 2026-10-12 (not released as binaries)

- `info` and `scan`: header, page map and damage report for Firebird
  2.5–5.0, also with a damaged header page.
