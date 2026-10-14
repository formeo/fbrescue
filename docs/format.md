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
Pointer pages list a table's data pages in order; export reads tables
this way, as the engine does.

## Data pages and record headers

0x10 sequence u32, 0x14 relation u16, 0x16 slot count u16, 0x18 slots of
(offset u16, length u16). Page flag 0x01 (byte 1): the page holds only
fragments of records larger than a page and is deliberately not listed in
a pointer page. Record header: transaction u32, back page u32, back line
u16, flags u16 (offset 10), format u8 — 13 bytes. Flags: 0x01 deleted,
0x02 back version, 0x04 fragment, 0x08 more fragments follow, 0x10 blob,
0x20 back version stored as differences, 0x40 large, 0x80 damaged, 0x400
48-bit transaction number (not read yet), 0x800 not packed (ODS 13.1).

Counting as gstat does — records: neither back version, fragment nor blob
(deleted included); versions: back versions — matches gstat for every
system and user table in the corpus. Test: `TestHealthyCorpusMatchesGstat`,
`TestDeletedRecords`.

A record number (in blob ids and DB_KEYs) is data page sequence ×
`(page size − 24) / 17` + line: 239 on 4K pages, 480 on 8K. Verified on
3000 blob ids per engine.

## Record encoding

Run-length encoding: control byte n > 0 is followed by n literal bytes;
n < 0 by one byte repeated −n times; 0 is an empty literal, which pads the
end of a fragment (seen in Firebird 3). ODS 13.1 adds −1: a 2-byte run
length, then the byte, and stores records that do not compress as they are
(flag 0x800).

A record that does not fit in a page is split. The first piece has flag
0x08 and a 22-byte header: the 13-byte header, three bytes of padding that
may hold garbage, next fragment page u32 at 16, line u16 at 20. Middle
pieces have 0x04|0x08 and the same header, the last one 0x04 and 13 bytes.
Up to ODS 13.0 the record is packed first and the packed bytes are split;
ODS 13.1 packs each piece on its own, and each piece has its own 0x800.
Test: `TestCatalogMatchesGstat` reads every record of every corpus
database; `TestExportRoundTrip` compares the values with SELECT, including
20 records of up to 20 000 bytes and 3000-byte runs.

## Record formats (RDB$FORMATS)

`RDB$DESCRIPTOR` is a blob of 12-byte field descriptors: dtype u8, scale
i8, length u16, sub_type i16 (text: character set in the low byte),
flags u16, offset u32. ODS 11: just the array. ODS 12+: count u16, the
array, then a count u16 of default values, each a field id u16, a
descriptor and the value bytes. A dropped column has dtype 0; a COMPUTED BY
column has offset 0.

Records start with a null bitmap of `((fields + 32) & ~31) / 8` bytes (bit
set = NULL). Records written before a column was added read NULL there,
except NOT NULL columns with a default: ODS 12+ stores that value in the
format; ODS 11 evaluates the column's default (`RDB$RELATION_FIELDS
.RDB$DEFAULT_VALUE`, BLR), of which fbrescue reads literals. Test:
`TestExportRoundTrip` on the ALT table (formats 1-7, a dropped column, a
type change, nullable and NOT NULL defaults).

Data types (dtype): 1 CHAR, 3 VARCHAR (u16 length first), 8 SMALLINT, 9
INTEGER, 19 BIGINT (with scale: NUMERIC/DECIMAL), 11 FLOAT, 12 DOUBLE, 14
DATE (days since 1858-11-17), 15 TIME (1/10000 s), 16 TIMESTAMP (date,
time), 17 BLOB (8-byte id), 21 BOOLEAN. Not decoded yet: 22/23 DECFLOAT, 24
INT128, 25/26 WITH TIME ZONE. CHAR in a multi-byte character set is padded
with spaces to its length in bytes; the value is padded to its length in
characters.

## System tables

Their formats are not in RDB$FORMATS. fbrescue builds the ones it reads
(RDB$PAGES, RDB$RELATIONS, RDB$RELATION_FIELDS, RDB$FORMATS) from the field
lists in `internal/meta/system.go` with the engine's layout rule: null
bitmap, then each field aligned for its type — CHAR 1, VARCHAR and
SMALLINT 2, INTEGER 4, BLOB id 8. Names are CHAR(31) UNICODE_FSS before
ODS 13, CHAR(63) UTF8 (252 bytes) from ODS 13 on. Verified by decoding
every row of these tables in every corpus database (`TestCatalogMatchesGstat`).
Firebird 2.0/2.1 (ODS 11.0/11.1) are not in the corpus: their rows are
shorter and should read as a prefix, but this is untested.

The header's field at 0x14 is the first pointer page of RDB$PAGES, whose
rows list every table's pointer pages (type 4) and the transaction
inventory pages (type 3) with their sequence numbers. RDB$PAGES rows are
written by the system transaction and are all current.

## Blobs

A blob id is relation u16, record number bits 32-39 u8, reserved u8,
record number bits 0-31 u32. The blob is a record with flag 0x10: lead
page u32, max sequence u32, max segment u16, flags u16 (0x20 stream blob),
level u8 at 12, segment count u32 at 16, length u32 at 20, sub type u16,
character set u8, then at 28 the data (level 0) or the blob page numbers
(level 1). Blob pages (type 8) keep the data length u16 at 24 and the
data at 28. Segmented blobs are segments of u16 length + bytes. Verified
on format descriptors up to 8 400 bytes (level 1). Level 2 and stream
blobs are not read yet.

## Visibility

A record version is current if its transaction committed (state 3 in the
transaction inventory) or is 0, the system transaction. Rows changed by a
transaction that rolled back or never finished keep the older committed
version as a back version; fbrescue does not read back versions yet and
reports such rows. A version without a back version from a transaction
that did not commit is an insert that never happened. Test: the RB table
(`TestRolledBackVersions`, `TestExportWarnings`), written with NO AUTO UNDO
so the rollback leaves its versions on disk.

## Character sets

Ids, names and bytes per character come from RDB$CHARACTER_SETS and are the
same in Firebird 2.5-5.0. Single-byte sets are decoded with tables
generated from Python's codecs (`tools/gencharsets`); UTF8, UNICODE_FSS
and ASCII are already UTF-8. Multi-byte Asian sets (SJIS, EUCJ, KSC_5601,
BIG_5, GB_2312, GBK, CP943C, GB18030) and NONE are written as they are,
with a warning if the bytes are not valid UTF-8.
