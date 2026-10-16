---
title: "internal Firebird consistency check (cannot find tip page (165))"
description: What "cannot find tip page (165)" means — the Firebird transaction inventory is damaged — and how to export the data when the database no longer opens.
lang: en
alt: /ru/errors/cannot-find-tip-page/
---

# `internal Firebird consistency check (cannot find tip page (165))`

## What it means

The transaction inventory (TIP) pages record, two bits per transaction, whether each transaction committed, rolled back or is still active. Every row version carries the number of the transaction that wrote it, and the engine looks that number up to decide which version a query sees. When a TIP page is missing, zeroed or overwritten, the engine cannot decide, and it refuses to go on: the database often does not open at all, and `gbak` cannot start.

The data itself is usually untouched: only the bookkeeping of transactions is gone.

{% include steps-en.md %}

{% include fbrescue-en.md %}

For this error fbrescue still exports every table. Transactions older than the oldest interesting transaction in the header are committed by definition. For newer ones whose state is lost, fbrescue takes the newest version of the row, which is right for every transaction that committed (nearly all of them), and the export says for how many rows it had to: those may include changes that were rolled back. When the inventory pages are only out of place, fbrescue finds them by scanning the file.

## Related

- [`database file appears corrupt`](../database-file-appears-corrupt/)
- [All errors](../)
