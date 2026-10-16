package record

import (
	"fmt"

	"github.com/formeo/fbrescue/internal/fdb"
)

// BlobID is the 8-byte blob id stored in a record field.
type BlobID struct {
	Relation uint16
	Number   uint64 // record number: data page sequence * MaxRecordsPerPage + line
}

// ParseBlobID decodes a blob id field. A zero id means an empty blob.
func ParseBlobID(b []byte) BlobID {
	return BlobID{Relation: le.Uint16(b), Number: uint64(b[2])<<32 | uint64(le.Uint32(b[4:]))}
}

// blob header (blh), stored as a record with FlagBlob
const (
	blhLevel      = 12
	blhLength     = 20
	blhSize       = 28 // level 0: data follows; level 1: page numbers follow
	blobStream    = 0x20
	blpLength     = 24 // blob page: data length
	blpDataOffset = 28
)

// Blob reads a blob (any level, segmented or stream) and returns its data
// with the segment headers removed. seqPage maps a data page sequence of the
// blob's relation to its page number.
func (r *Reader) Blob(id BlobID, seqPage func(seq uint32) (uint32, bool)) ([]byte, error) {
	fail := func(format string, args ...any) ([]byte, error) {
		return nil, fmt.Errorf("blob %d:%d: %s", id.Relation, id.Number, fmt.Sprintf(format, args...))
	}
	per := uint64(MaxRecordsPerPage(r.db.PageSize))
	seq, line := id.Number/per, int(id.Number%per)
	p, ok := seqPage(uint32(seq))
	if !ok {
		return fail("data page %d of relation %d is unknown", seq, id.Relation)
	}
	if err := r.load(p); err != nil {
		return nil, err
	}
	b, err := slot(r.page, line)
	if err == nil && b == nil {
		err = fmt.Errorf("line %d is empty", line)
	}
	if err != nil {
		return fail("page %d: %v", p, err)
	}
	flags := le.Uint16(b[10:])
	if flags&FlagBlob == 0 || len(b) < blhSize {
		return fail("page %d line %d is not a blob", p, line)
	}
	b = append([]byte(nil), b...) // r.page is reused below
	length := int(le.Uint32(b[blhLength:]))

	var data []byte
	buf := make([]byte, r.db.PageSize)
	// readPage returns the data area of a blob page.
	readPage := func(bp uint32) ([]byte, error) {
		if err := r.db.ReadPage(bp, buf); err != nil {
			return nil, err
		}
		if buf[0] != fdb.TypeBlob {
			return nil, fmt.Errorf("page %d is not a blob page", bp)
		}
		if err := r.db.CheckNumber(bp, buf); err != nil {
			return nil, err
		}
		n := int(le.Uint16(buf[blpLength:]))
		if blpDataOffset+n > len(buf) {
			return nil, fmt.Errorf("blob page %d claims %d bytes", bp, n)
		}
		return buf[blpDataOffset : blpDataOffset+n], nil
	}
	pageList := func(b []byte) []uint32 {
		var out []uint32
		for o := 0; o+4 <= len(b); o += 4 {
			out = append(out, le.Uint32(b[o:]))
		}
		return out
	}
	switch level := b[blhLevel]; level {
	case 0:
		data = b[blhSize:]
	case 1, 2:
		pages := pageList(b[blhSize:])
		if level == 2 {
			// The header lists pointer pages, which list the data pages.
			var all []uint32
			for _, pp := range pages {
				d, err := readPage(pp)
				if err != nil {
					return fail("%v", err)
				}
				all = append(all, pageList(d)...)
			}
			pages = all
		}
		for _, bp := range pages {
			d, err := readPage(bp)
			if err != nil {
				return fail("%v", err)
			}
			data = append(data, d...)
		}
	default:
		return fail("unknown level %d", level)
	}

	if flags&blobStream != 0 {
		if len(data) < length {
			return fail("%d bytes read, header says %d", len(data), length)
		}
		return data[:length], nil
	}
	// Segments: 2-byte length, then the bytes.
	out := make([]byte, 0, length)
	for i := 0; i < len(data); {
		if i+2 > len(data) {
			return fail("truncated segment header")
		}
		n := int(le.Uint16(data[i:]))
		if i+2+n > len(data) {
			return fail("segment of %d bytes runs past the data", n)
		}
		out = append(out, data[i+2:i+2+n]...)
		i += 2 + n
	}
	if len(out) != length {
		return fail("%d bytes read, header says %d", len(out), length)
	}
	return out, nil
}
