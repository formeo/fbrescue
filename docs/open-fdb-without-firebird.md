---
title: Open a Firebird .fdb file without Firebird installed
description: "See the tables of a Firebird .fdb or .gdb file and export its data without a Firebird server installed, with fbrescue, a free tool."
lang: en
alt: /ru/open-fdb-without-firebird/
---

# Open a Firebird `.fdb` file without Firebird installed

You have a database file from an old program — `.fdb`, sometimes `.gdb` — and no Firebird server, no password, maybe not even the program any more. You need to see what is in it, or get the data into something you use today.

[fbrescue](https://github.com/formeo/fbrescue), a free, open-source (MIT) tool, reads the file directly. Nothing to install: download the single binary for Windows, Linux or macOS from [Releases](https://github.com/formeo/fbrescue/releases/latest). No server, no user name, no password, and it never writes to the file.

## See what is in the file

```
fbrescue info old.fdb      # Firebird version, page size, dialect
fbrescue tables old.fdb    # CREATE TABLE statements: tables, columns, types
fbrescue scan old.fdb      # pages, tables and row counts; damage, if any
```

## Get the data out

```
fbrescue export old.fdb --out data                        # every table to data/<TABLE>.csv
fbrescue export old.fdb --out data --table CUSTOMERS      # only some tables
fbrescue export old.fdb --out data --format sql           # INSERT statements for another Firebird
fbrescue export old.fdb --out data --format jsonl         # JSON Lines, for scripts
```

Every format comes with `schema.sql`, the table definitions. Text is written as UTF-8. Databases of Russian, Eastern European and other programs often store text in `CHARACTER SET NONE`; if the export warns that values are not valid UTF-8, name the real encoding: `--charset WIN1251` (or `WIN1250`, `WIN1252`, `KOI8R`, `DOS866` …).

Next steps for the CSV files: [open them in Excel or load them elsewhere]({{ '/firebird-to-csv/' | relative_url }}), or [move the data to PostgreSQL]({{ '/firebird-to-postgresql/' | relative_url }}).

## Which files it reads

Databases of **Firebird 2.5, 3.0, 4.0 and 5.0** — whatever the extension. Files of InterBase and of Firebird 1.x (most `.gdb` files from the early 2000s) use an older on-disk format and are not supported; `fbrescue info` says so instead of guessing.

If the file is damaged — the program stopped opening it, or it was copied from a failing disk — see [recovering a corrupted database]({{ '/' | relative_url }}).

Use fbrescue on databases you own or are allowed to access.
