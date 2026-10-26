---
title: Move data from Firebird to PostgreSQL
description: "Move Firebird tables to PostgreSQL: export the .fdb to CSV without a Firebird server, map the types, load with \\copy. MySQL and SQL Server too."
lang: en
alt: /ru/firebird-to-postgresql/
---

# Move data from Firebird to PostgreSQL

The data part of a migration: get every table of a Firebird 2.5–5.0 database into PostgreSQL. [fbrescue](https://github.com/formeo/fbrescue), a free, open-source (MIT) tool, reads the `.fdb` file directly, so the old server does not have to run, and its CSV is what PostgreSQL's `COPY` expects.

## 1. Export

```
fbrescue export old.fdb --out data
```

You get `data/schema.sql` with the table definitions and one CSV per table. If the database uses `CHARACTER SET NONE` for text written by a Windows program, add `--charset WIN1251` (or the code page it used).

## 2. Create the tables

`schema.sql` is Firebird SQL. Create the same tables in PostgreSQL with these types:

| Firebird (`schema.sql`) | PostgreSQL |
|---|---|
| `SMALLINT`, `INTEGER`, `BIGINT`, `BOOLEAN`, `DATE`, `TIME`, `TIMESTAMP`, `DOUBLE PRECISION` | the same |
| `NUMERIC(p,s)`, `DECIMAL(p,s)`, `INT128`, `DECFLOAT` | `numeric(p,s)` / `numeric` |
| `FLOAT` | `real` |
| `CHAR(n)` / `VARCHAR(n) CHARACTER SET …` | `char(n)` / `varchar(n)` (a UTF-8 database) |
| `BLOB SUB_TYPE TEXT` | `text` |
| `BLOB SUB_TYPE BINARY` (or `0`) | `bytea` — the CSV has hex: load into `text`, then `decode(col, 'hex')` |
| `TIMESTAMP WITH TIME ZONE` | `timestamptz` — the CSV shows the local time and the zone name |

Names in `schema.sql` are quoted upper case (`"CUSTOMERS"`). Either keep them quoted in PostgreSQL, or lower-case them in the schema and in the commands below. `NUMERIC` precision in `schema.sql` is the maximum for the storage size (`NUMERIC(18,2)` for a `NUMERIC(12,2)` column); use the original precision if you know it.

## 3. Load

```
for f in data/*.csv; do
  t=$(basename "$f" .csv)
  psql -d target -c "\copy \"$t\" FROM '$f' WITH (FORMAT csv, HEADER true)"
done
```

PostgreSQL's CSV format reads an empty field as NULL and `""` as an empty string — exactly how fbrescue writes them.

## 4. What to recreate

fbrescue moves tables and rows. Primary and foreign keys, indexes, generators (sequences), triggers, procedures and views are not exported: create them in PostgreSQL, and set each sequence past the largest key, for example `SELECT setval('customers_id_seq', (SELECT max(id) FROM customers));`.

## MySQL, SQL Server, SQLite

Same CSV, same steps: `LOAD DATA INFILE … FIELDS TERMINATED BY ',' OPTIONALLY ENCLOSED BY '"' IGNORE 1 LINES` in MySQL, `BULK INSERT … WITH (FORMAT = 'CSV', FIRSTROW = 2)` in SQL Server, `.import --csv` in SQLite. Check how each one reads empty fields: not all of them load them as NULL.

More about the CSV files: [Export a Firebird database to CSV]({{ '/firebird-to-csv/' | relative_url }}).
