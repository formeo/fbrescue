---
title: "internal Firebird consistency check (wrong record length (183))"
description: What the Firebird error "wrong record length (183)" means and how to save the data of the table when gfix and gbak cannot get past the damaged record.
lang: en
alt: /ru/errors/wrong-record-length/
---

# `internal Firebird consistency check (wrong record length (183))`

## What it means

Every record carries the number of the record format (the table's layout at the time it was written), and unpacks to the length of that format. `wrong record length` means a record unpacked to a different length: its bytes, its header or one of its fragments is damaged. Like [`decompression overran buffer`](../decompression-overran-buffer/), it is damage to individual records, and the statement that reads the record fails every time.

{% include steps-en.md %}

{% include fbrescue-en.md %}

For this error fbrescue reads each record with the format it names. A record that is too short for its format is listed in `_errors.jsonl` with its page and line; the other records of the table are exported. Tables altered many times are fine: each record is read with the layout it was written with.

## Related

- [`decompression overran buffer (179)`](../decompression-overran-buffer/)
- [`database file appears corrupt`](../database-file-appears-corrupt/)
- [All errors](../)
