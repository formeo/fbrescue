---
title: "Firebird: database file appears corrupt, wrong page type"
description: "What \"database file appears corrupt\" and \"page is of wrong type\" mean in Firebird, and how to get the data out of the damaged database."
lang: en
alt: /ru/errors/database-file-appears-corrupt/
---

# `database file appears corrupt` / `page ... is of wrong type`

The two usually come together: `database file appears corrupt (...)`, then `wrong page type` and `page 1234 is of wrong type (expected 5, found 0)`.

## What it means

A Firebird database is a file of fixed-size pages, each with a type in its first byte: data, index, pointer, blob, transaction inventory and so on. The engine followed a reference to page 1234 expecting one type and found another. *found 0* means the page is zeroed — typical after a crash, a failing disk, a file system repair or a copy that went wrong. Another number usually means the page holds the contents of a different page.

Which data is lost depends on which page it is:

| Page type (expected) | What is lost |
|---|---|
| 5, data | the rows on that page |
| 4, pointer | for the engine, the table's data pages listed there; the pages themselves are intact |
| 7, index | nothing: indexes are rebuilt from the data |
| 8, blob | the BLOB values stored on that page |
| 3, transaction inventory | see [`cannot find tip page`](../cannot-find-tip-page/) |

{% include steps-en.md %}

{% include fbrescue-en.md %}

`fbrescue scan` lists every page that is not what it should be, and per table how many data pages and records it can reach. For export:

- data pages that the table's pointer pages no longer list are found by reading the whole file (each data page records its table and its place in it);
- a page found at the location of another page is read from there (Firebird 3 and later store each page's number on the page);
- damaged index pages do not matter: export reads only the data.

## Related

- [`I/O error during "read" operation`](../io-error-during-read/) — the file ends early
- [`cannot find tip page (165)`](../cannot-find-tip-page/)
- [All errors](../)
