---
title: Recover data from a corrupted Firebird database
description: What to do when a Firebird or InterBase database is corrupted and gbak cannot back it up — the engine's own tools first, then fbrescue, a free read-only tool that exports the data without the engine.
lang: en
alt: /ru/
---

# Recover data from a corrupted Firebird database

Your Firebird database stopped opening, a query fails with `internal Firebird consistency check`, or `gbak` cannot back it up. This page is the order of steps that saves the most data, from the least to the most drastic. The [error pages](errors/) explain what the common messages mean.

{% include steps-en.md %}

{% include fbrescue-en.md %}

## What fbrescue does with damage

- **Damaged records**: skipped and listed; the rest of the table is exported.
- **Damaged or zeroed pages**: the rows on them are lost, the rest of the database is not. Data pages that the table's pointer pages no longer reach are found by reading the whole file.
- **Pages in the wrong place** (Firebird 3 and later store each page's number on the page): read from where they are.
- **Damaged transaction inventory** (`cannot find tip page`): rows are still exported; where a transaction's state is lost, the newest version is taken, with a warning.
- **Damaged header page**: the page size and version come from the rest of the file.
- **Lost table definitions**: tables and columns come out under made-up names (`TABLE_131`, `FIELD_2`). If the record layouts themselves are lost, give fbrescue the CREATE TABLE statements of your database (`--ddl schema.sql`): the script that created it, `isql -x` output, or an earlier fbrescue export.

How much comes back from each kind of damage is measured on hundreds of damaged copies of a test database on Firebird 2.5, 3.0, 4.0 and 5.0: see [the recovery rate](https://github.com/formeo/fbrescue#recovery-rate).

fbrescue gets the data out; it does not repair the file in place. Indexes, triggers, procedures and users are not exported: create them again in the new database.

## Common errors

{% include errors-en.md %}
