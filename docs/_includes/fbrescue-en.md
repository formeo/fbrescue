## If gbak still fails: get the data out with fbrescue

[fbrescue](https://github.com/formeo/fbrescue) reads the database file directly, without the Firebird engine, and exports every row it can read. It does not stop at the first damaged record or page, and it never writes to the file. Download the single binary for your system from [Releases](https://github.com/formeo/fbrescue/releases/latest) (Windows, Linux, macOS; nothing to install).

```
fbrescue scan copy.fdb                               # what is damaged, by table
fbrescue export copy.fdb --out rescued --format sql  # schema.sql + INSERTs per table
```

`--format csv` and `--format jsonl` are there too. Rows that could not be read are listed in `rescued/_errors.jsonl` with the page and the reason, and the exit status is 2 when anything was left out. To load the result into a new database:

```
isql -ch UTF8 -i rescued/schema.sql new.fdb
isql -ch UTF8 -i rescued/CUSTOMERS.sql new.fdb  # and so on for each table
```

If you would like help, open an [issue](https://github.com/formeo/fbrescue/issues) and attach the output of `fbrescue report copy.fdb`: it describes the damage without any of your data, and with table and column names replaced by numbers.
