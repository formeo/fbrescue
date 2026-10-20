---
title: "fbrescue FAQ: data from a corrupted Firebird database"
description: "Short answers on recovering a damaged Firebird database with fbrescue: without the server, without changing the file, versions, price, limits."
lang: en
alt: /ru/faq/
faq:
  - q: Can I get the data out of a Firebird database without the Firebird server?
    a: >-
      Yes. fbrescue reads the `.fdb` file directly, without the engine and without `fbclient`, and exports
      every table to CSV, SQL (INSERT statements) or JSON Lines. It is a single binary for Windows, Linux
      and macOS. It also works when the database no longer opens and `gbak` cannot back it up. Step by step:
      [open a .fdb file without Firebird](../open-fdb-without-firebird/).
  - q: Does fbrescue change or repair my database file?
    a: >-
      No. It opens the file read-only and never writes to it; a test in CI runs every command on read-only
      copies of real databases and checks that nothing changed. fbrescue gets the data out into files you
      load into a new database; it does not repair the original. Still work on a copy: other tools you try
      may write to it.
  - q: Which Firebird versions does it support?
    a: >-
      Firebird 2.5, 3.0, 4.0 and 5.0 (ODS 11.2 to 13.1), including Firebird 4 types (INT128, DECFLOAT,
      TIME and TIMESTAMP WITH TIME ZONE), BLOBs and multi-file databases. InterBase and
      Firebird 1.x, 2.0 and 2.1 are not supported.
  - q: Is it free?
    a: >-
      Yes. fbrescue is open source under the MIT license: no license per database, no limit on the number
      of rows or databases.
  - q: Should I try gfix and gbak first?
    a: >-
      Yes. On a copy of the file run `gfix -v -full -n`, then `gfix -mend -full -ignore` and
      `gbak -b -ig -g`. If the backup completes and restores, you get the whole database back, with indexes,
      triggers and procedures. Use fbrescue when gbak still stops.
  - q: How is fbrescue different from IBSurgeon FirstAID?
    a: >-
      FirstAID is a commercial Windows tool that can repair the database file in place or extract data,
      and supports InterBase and older Firebird versions. fbrescue is free, read-only, runs on Windows,
      Linux and macOS, and only extracts data, from Firebird 2.5 to 5.0. See the [comparison](../compare/).
  - q: What does fbrescue not recover?
    a: >-
      Indexes, triggers, stored procedures, views, users, generator values and constraints: it exports table
      definitions (schema.sql) and rows. Array columns are not exported yet. Rows on pages that are zeroed
      or cut off from the file are lost; every row it could not read is listed in `_errors.jsonl`.
  - q: Can it recover deleted or overwritten rows?
    a: >-
      Often, if the old record versions are still on disk (not yet garbage-collected): `--include-deleted`
      writes deleted rows to a separate file, and `--versions all` writes every version of every row with
      its transaction and that transaction's state.
  - q: What if the system tables are damaged?
    a: >-
      Tables whose definitions are partly lost are exported as `TABLE_<id>` with columns `FIELD_<n>`. If
      the record formats themselves are lost, pass the CREATE TABLE statements of the database with
      `--ddl schema.sql` (the creation script, `isql -x` output, or an earlier fbrescue export), and
      fbrescue computes the record layout from them.
  - q: How much data does it recover?
    a: >-
      It depends on the damage. On copies of a test database damaged in 13 different ways on Firebird 2.5
      to 5.0, it recovers 100% of the rows when pointer, index, transaction inventory or header pages are
      zeroed, and everything but the damaged page's rows when data pages are lost. The full table is in the
      [README](https://github.com/formeo/fbrescue#recovery-rate).
  - q: How can I get help without sending my data?
    a: >-
      Run `fbrescue report copy.fdb` and attach the output to a
      [GitHub issue](https://github.com/formeo/fbrescue/issues). It describes the damage without any data,
      with table and column names replaced by numbers.
---

# fbrescue FAQ

{% for item in page.faq %}
## {{ item.q }}

{{ item.a }}
{% endfor %}

More: [recovery steps]({{ '/' | relative_url }}), [error messages]({{ '/errors/' | relative_url }}), [download](https://github.com/formeo/fbrescue/releases/latest).
