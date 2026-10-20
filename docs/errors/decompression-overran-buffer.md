---
title: "Firebird error: decompression overran buffer (179)"
description: What the Firebird error "decompression overran buffer (179)" means — a damaged record — and how to get the rest of the table back when gfix and gbak fail.
lang: en
alt: /ru/errors/decompression-overran-buffer/
---

# `internal Firebird consistency check (decompression overran buffer (179))`

The full message usually reads `internal Firebird consistency check (decompression overran buffer (179), file: sqz.cpp line: ...)`, in a query, in `gbak` or in `gfix -v`.

## What it means

Firebird stores each record compressed with a simple run-length encoding. When it unpacks a record and the result does not fit the record's layout, the stored bytes are damaged: by a failing disk, a bad copy, a crash in the middle of a write, or a page overwritten with something else. The engine stops the statement that touched the record, so a `SELECT` or a backup of that table fails at the same place every time. The other records of the table are usually fine.

{% include steps-en.md %}

`gfix -mend` is the engine's answer to exactly this error: it marks the damaged record so the next backup skips it.

{% include fbrescue-en.md %}

For this error fbrescue reads every record of the table, lists the ones that do not unpack in `_errors.jsonl` with their page and line, and exports the rest. A record that continues on another page (records larger than a page) is lost only if one of its pieces is damaged.

## Related

- [`wrong record length (183)`](../wrong-record-length/) — the same kind of damage, found later in reading
- [`gbak` backup fails](../gbak-backup-fails/)
- [All errors](../)
