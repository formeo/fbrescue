# Changelog

Each version's section is also its release notes on GitHub
(`tools/release/notes.sh`).

## v0.6.0 — 2026-11-13

Firebird 4+ records with 48-bit transaction numbers, and the CYRL and NEXT
character sets.

- **48-bit transaction numbers.** Firebird 4+ databases that have seen more
  than 2^32 transactions are now supported: records are exported with the
  correct transaction state, and `page` decodes and prints the full
  transaction number. Ordinary and fragmented records are covered by tests
  against real ODS 13 pages with a documented 48-bit patch
  ([#23](https://github.com/formeo/fbrescue/issues/23)).
- **CYRL and NEXT.** Text stored in these legacy Firebird character sets is
  now decoded correctly ([#22](https://github.com/formeo/fbrescue/issues/22)).
- **Parser hardening.** The header/page, record and scanner parsers now have
  fuzz tests with synthetic seeds and pages from the real Firebird corpus.
  CI runs each target for 60 seconds
  ([#19](https://github.com/formeo/fbrescue/issues/19)).
- **Unchanged:** array columns are still reported as unsupported.

## v0.5.0 — 2026-10-27

CSV for Excel, and the site in Portuguese.

- **`export --excel`**: CSV that Excel opens by double-click where the
  decimal separator is a comma (Brazil, Russia, most of Europe) — a UTF-8
  BOM, `;` between fields and a decimal comma in numbers; text, dates and
  times unchanged ([#15](https://github.com/formeo/fbrescue/issues/15)).
- **Site in three languages.** The recovery guide, the error pages, the FAQ
  and the comparison are now also in Brazilian Portuguese, at
  https://formeo.github.io/fbrescue/pt/.

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
