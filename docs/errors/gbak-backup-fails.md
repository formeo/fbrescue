---
title: "gbak cannot back up a damaged Firebird database"
description: "gbak: ERROR ... Exiting before completion due to errors" on a damaged Firebird database — the gbak options that get further, and how to get the data out when gbak still fails."
lang: en
alt: /ru/errors/gbak-backup-fails/
---

# `gbak: ERROR` … `gbak:Exiting before completion due to errors`

## What it means

`gbak` backs up through the Firebird engine, table by table and row by row. The first record or page the engine cannot read stops the whole backup, and the message before `Exiting before completion` names the reason: often an [`internal Firebird consistency check`](../decompression-overran-buffer/), a [wrong page type](../database-file-appears-corrupt/) or an [I/O error](../io-error-during-read/). Everything after that point is missing from the backup, even if it is intact in the file.

## Options that get further

- `-g` (no garbage collection): the backup does not try to clean up old record versions, which can touch damaged records it would not otherwise read.
- `-ig` (ignore checksums): skips the page checksums older versions write.
- `gfix -mend -full -ignore` before the backup: marks the damaged records so `gbak` skips them.
- `-v` shows which table was being read when it stopped.

The full order of steps:

{% include steps-en.md %}

{% include fbrescue-en.md %}

fbrescue reads without the engine, so a damaged record or page costs that record or page, not the rest of the backup. Each table is exported to its own file, and `_errors.jsonl` says what was left out and why.

## Related

- [All errors](../)
- [Recovery steps]({{ '/' | relative_url }})
