# On-disk format notes

Everything fbrescue relies on, and how it was verified. "gstat" means the
engine's own `gstat` on the corpus built by `tools/mkcorpus` (Firebird 2.5.9,
3.0.12, 4.0.5, 5.0.1). All integers are little-endian.

## Families

| ODS | Firebird | Family |
|---|---|---|
| 11.0 / 11.1 / 11.2 | 2.0 / 2.1 / 2.5 | ODS 11 |
| 12.0 | 3.0 | ODS 12+ |
| 13.0 / 13.1 | 4.0 / 5.0 | ODS 12+ |

`hdr_ods_version` (0x12) carries the Firebird flag `0x8000`; the major
version is the rest.

## Page header (all pages)

| Offset | ODS 11 | ODS 12+ |
|---|---|---|
| 0x00 | page type (byte) | page type (byte) |
| 0x02 | checksum, always 12345 | — |
| 0x0C | — | the page's own number |

The last two are the page size detection signatures (`detectPageSize`); the
page number also catches misplaced pages. Verified on every page of the
corpus. Test: `TestDamagedHeaderDetectsPageSize`, `TestMisplacedPage`.

Page types: 1 header, 2 PIP, 3 TIP, 4 pointer, 5 data, 6 index root, 7 index
(B-tree), 8 blob, 9 generator, 10 — present in every version (page 2 in
ODS 11, one per page range in ODS 13), not interpreted.

## Header page (page 0)

| Field | ODS 11 | ODS 12 | ODS 13 |
|---|---|---|---|
| page size | 0x10 u16 | 0x10 | 0x10 |
| ODS version | 0x12 u16 | 0x12 | 0x12 |
| OIT / OAT / next tx | 0x1C / 0x20 / 0x24 u32 | same | same |
| flags | 0x2A u16 | 0x2A | 0x2A |
| creation date, time | 0x2C, 0x30 (days since 1858-11-17, 1/10000 s); server local time | same | same, UTC |
| next attachment | 0x34 u32 | 0x34 | 0x34 |
| ODS minor | 0x3E u16 | 0x40 | 0x40 |
| page buffers | 0x44 u32 | 0x44 | 0x44 |
| OST | 0x4C u32 | 0x48 | 0x48 |
| attachment high 32 bits | — | 0x78 u32 | 0x74 |
| transaction high 16 bits: next, OIT, OAT, OST | — | 0x7C 4×u16 | 0x78 |
| variable data (tag, length, value) | 0x60 | 0x84 | 0x80 |
| tag: continuation file / last logical page | 3 / 4 | 2 / 3 | 2 / 3 |

"Next transaction" is the number of the last started transaction (its TIP
state is set). The high words were verified by patching them in a copy and
reading them back with gstat (`hiwords` in the corpus). Test:
`TestHeaderMatchesGstat` compares every field above with gstat.

## Header flags

Only these bits are named; anything else is reported as unknown.

| Meaning (gstat wording) | ODS 11 | ODS 12+ |
|---|---|---|
| force write | 0x0002 | 0x0002 |
| no reserve | 0x0020 | 0x0008 |
| SQL dialect 3 | 0x0100 | 0x0010 |
| read only | 0x0200 | 0x0020 |
| backup lock (nbackup) | 0x0400 | 0x0400 |
| multi-user maintenance / full shutdown / single-user maintenance (mask 0x1080) | 0x0080 / 0x1000 / 0x1080 | same |
| read-only / read-write replica (mask 0x6000) | — | 0x2000 / 0x4000 (ODS 13) |

Each one has its own corpus database. Test: `TestHeaderMatchesGstat`.

## Multi-file databases

The main file holds logical pages 0..last-page; each secondary file starts
with its own header page, then logical pages from the previous file's last
page + 1. fbrescue looks for a secondary file next to the main file first,
then at the stored absolute path. Test: `TestMultiFile`.

## Page inventory (PIP)

Bitmap of free pages (bit set = free), at 0x14 in ODS 11 and 0x1C in ODS 12+.
One PIP covers `(page size - offset) * 8` pages. The first PIP is page 1; the
PIP for range k is the last page of range k-1 (`k * per_pip - 1`). In
ODS 12+ unused pages at the end of the file have type 0 and are free.
Verified on 4K-page databases larger than one range (`big`).

## Transaction inventory (TIP)

0x10 next TIP page (0 in the last), 0x14 two bits per transaction:
0 active, 1 limbo, 2 dead, 3 committed. `(page size - 0x14) * 4`
transactions per page; the chain starts at the TIP no other TIP points to.
All transactions below OIT are committed. Test: `TestTIPChain` (three TIPs),
`TestHealthyCorpusMatchesGstat`.

## Pointer pages

0x10 sequence u32, 0x14 next pointer page, 0x18 slot count u16, 0x1A
relation u16, 0x20 data page numbers (u32 each). A data page's sequence is
pointer page sequence × pages-per-pointer-page + slot; fbrescue only checks
that this base is constant within a pointer page (the per-page count is 956
for ODS 11 and 808 for ODS 12+ on 4K pages, but its formula is not needed).

## Data pages and record headers

0x10 sequence u32, 0x14 relation u16, 0x16 slot count u16, 0x18 slots of
(offset u16, length u16). Record header: transaction u32, back page u32,
back line u16, flags u16 (offset 10), format u8 — 13 bytes. Flags: 0x01
deleted, 0x02 back version, 0x04 fragment, 0x10 blob.

Counting as gstat does — records: neither back version, fragment nor blob
(deleted included); versions: back versions — matches gstat for every
system and user table in the corpus. Test: `TestHealthyCorpusMatchesGstat`,
`TestDeletedRecords`.
