---
title: "Firebird database recovery tools compared: gfix/gbak, IBSurgeon FirstAID, fbrescue"
description: Which tool to use on a corrupted Firebird database — the engine's gfix and gbak, IBSurgeon FirstAID, or the free open-source fbrescue — what each does, platforms, versions, price.
lang: en
alt: /ru/compare/
---

# Firebird recovery tools compared

Three kinds of tools get data back from a damaged Firebird database. They do different things, and they can be used one after another: start with the engine's own tools, on a copy of the file.

| | gfix + gbak | IBSurgeon FirstAID | fbrescue |
|---|---|---|---|
| What it does | validates and marks damaged records (`gfix -mend`), then backs up and restores through the engine | repairs the file in place ("direct fix"), or extracts data into a new database with the same structure | exports the data (CSV, SQL INSERTs, JSON Lines) and the table definitions, without the engine |
| Changes the file | `gfix -mend` does | direct fix does | never: read-only |
| What you get back | the whole database: indexes, triggers, procedures, users | the repaired database, or its data | tables and rows; indexes, triggers and procedures must be created again |
| When it stops | at the first record or page the engine cannot read | — | does not stop: skips and lists what it cannot read |
| Firebird versions | the server's own | Firebird 1.0–5.0, InterBase 4.0–2020 | Firebird 2.5–5.0 |
| Platforms | wherever Firebird runs | Windows 7 or later (copy files from Linux servers) | Windows, Linux, macOS; one binary, no Firebird needed |
| Interface | command line | graphical | command line, JSON output for scripts |
| Price | free, part of Firebird | free preview; a license per database for the repair (from $199 for 3 databases)¹ | free, MIT license |
| Source | open | closed | [open](https://github.com/formeo/fbrescue), with a [published recovery rate](https://github.com/formeo/fbrescue#recovery-rate) |

¹ From [IBSurgeon's FirstAID page](https://ib-aid.com/en/ibsurgeon-firstaid/), October 2026. IBSurgeon also offers a recovery service for heavy cases.

## Which one to use

- **Always first: gfix and gbak**, on a copy. If `gbak -b -ig -g` completes, the restored database is complete, with everything the engine knows. The steps are on the [recovery page]({{ '/' | relative_url }}).
- **fbrescue** when gbak stops, when you are on Linux or macOS, when you need the data from a database that does not open at all, or to see quickly what can be recovered: `fbrescue scan` reports the damage table by table, and `export` gets the rows out without touching the file. It is free, so there is nothing to lose by trying it on a copy.
- **IBSurgeon FirstAID** when you need the database itself repaired in place rather than the data exported, for InterBase or Firebird before 2.5, or when you want a commercial tool with support and a recovery service behind it.

The tools do not exclude each other: fbrescue never writes to the file, so it can be run on a copy before or after any other attempt.

## See also

- [Error messages and what they mean]({{ '/errors/' | relative_url }})
- [FAQ]({{ '/faq/' | relative_url }})
