---
title: "Firebird: I/O error during \"read\" operation for file"
description: "Firebird \"I/O error during read operation\": a truncated database file or a failing disk. What it means and how to save the data that is still there."
lang: en
alt: /ru/errors/io-error-during-read/
---

# `I/O error during "read" operation for file`

On Windows the operation is called `"ReadFile"`; the next line is usually `Error while trying to read from file`, sometimes with `Reached the end of the file` or an operating system error.

## What it means

The engine asked the operating system for a page and did not get it. Either the page is beyond the end of the file — the file is shorter than the database thinks: a copy that stopped half way, a full disk, a download cut short — or the disk could not read it.

If the disk is failing, **stop using it**: every read can make it worse. Copy the file to another disk first, with a tool that skips unreadable sectors if needed (`ddrescue` on Linux), and work on the copy.

If the file is truncated, the pages past the end are gone, and so are the rows on them. Everything before the cut can be saved.

{% include steps-en.md %}

`gbak` often stops at the first page it cannot read, even when most of the database is intact.

{% include fbrescue-en.md %}

For a truncated file, `fbrescue scan` reports the pages that are referenced but no longer in the file, and which tables they belong to. Export reads every page that is still there and lists the rows and values it could not read; blobs whose pages are gone are written as NULL, the rest of their row is kept.

## Related

- [`database file appears corrupt`](../database-file-appears-corrupt/)
- [All errors](../)
