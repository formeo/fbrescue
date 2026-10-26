---
title: Export a Firebird database to CSV or Excel
description: Export every table of a Firebird database (.fdb) to CSV, JSON Lines or SQL — without a Firebird server — and open the result in Excel, LibreOffice or a script.
lang: en
---

# Export a Firebird database to CSV or Excel

[fbrescue](https://github.com/formeo/fbrescue), a free, open-source (MIT) tool, writes every table of a Firebird 2.5–5.0 database to its own CSV file, straight from the database file: no server, no ODBC driver, no password. Download the binary for your system from [Releases](https://github.com/formeo/fbrescue/releases/latest), then:

```
fbrescue export shop.fdb --out data
```

```
data/
  schema.sql        CREATE TABLE statements
  CUSTOMERS.csv     one file per table
  ORDERS.csv
  _errors.jsonl     only if some rows could not be read
```

Use `--table NAME` (repeatable) for some tables only. Make a copy of the file first if a server is using it: a file copied while the server writes to it can be inconsistent.

## What the CSV looks like

- RFC 4180: comma-separated, a header row with the column names, UTF-8, values quoted when needed.
- **NULL is an empty field, an empty string is `""`**, so the two stay apart.
- Numbers are exact: `NUMERIC(18,2)` comes out as `1234.50`, never as a float.
- Dates `2024-02-29`, times `13:45:00.0000`, timestamps `2024-02-29 13:45:00.0000`; booleans `true` / `false`.
- Text BLOBs are written in full; binary values as hexadecimal.
- Text in `CHARACTER SET NONE`: add `--charset WIN1251` (or the encoding the program used) to get readable UTF-8.

## Open in Excel

Double-clicking a CSV makes Excel guess the encoding and the separator, which breaks non-English text and, in some countries, the columns. Instead: **Data → From Text/CSV**, choose the file, set *File origin* to **65001: Unicode (UTF-8)** and *Delimiter* to **Comma**, then **Load**. LibreOffice Calc asks the same in its import dialog.

## Other formats

- `--format jsonl`: one JSON object per row, exact numbers as JSON numbers — easiest for Python, Node or `jq`.
- `--format sql`: INSERT statements and `schema.sql`, to load into another Firebird database with `isql`.

Moving to another database: [Firebird to PostgreSQL]({{ '/firebird-to-postgresql/' | relative_url }}). File will not open, or the export reports errors: [recovering a corrupted database]({{ '/' | relative_url }}).
