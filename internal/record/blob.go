package record

import (
	"errors"
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

// ErrBlobLevel2 is returned for blobs larger than one level of blob pages.
var ErrBlobLevel2 = errors.New("blobs with two levels of blob pages are not supported yet")

// Blob reads a segmented blob of level 0 or 1 and returns its data with the
// segment headers removed. seqPage maps a data page sequence of the blob's
// relation to its page number.
func (r *Reader) Blob(id BlobID, seqPage func(seq uint32) (uint32, bool)) ([]byte, error) {
	per := uint64(MaxRecordsPerPage(r.db.PageSize))
	seq, line := id.Number/per, int(id.Number%per)
	p, ok := seqPage(uint32(seq))
	if !ok {
		return nil, fmt.Errorf("blob %d:%d: data page %d of relation %d is unknown", id.Relation, id.Number, seq, id.Relation)
	}
	if err := r.load(p); err != nil {
		return nil, err
	}
	b, err := slot(r.page, line)
	if err == nil && b == nil {
		err = fmt.Errorf("line %d is empty", line)
	}
	if err != nil {
		return nil, fmt.Errorf("blob %d:%d at page %d: %w", id.Relation, id.Number, p, err)
	}
	flags := le.Uint16(b[10:])
	if flags&FlagBlob == 0 || len(b) < blhSize {
		return nil, fmt.Errorf("blob %d:%d: page %d line %d is not a blob", id.Relation, id.Number, p, line)
	}
	if flags&blobStream != 0 {
		return nil, fmt.Errorf("blob %d:%d: stream blobs are not supported yet", id.Relation, id.Number)
	}
	length := int(le.Uint32(b[blhLength:]))

	var data []byte
	switch b[blhLevel] {
	case 0:
		data = append(data, b[blhSize:]...)
	case 1:
		pages := make([]uint32, 0, (len(b)-blhSize)/4)
		for o := blhSize; o+4 <= len(b); o += 4 {
			pages = append(pages, le.Uint32(b[o:]))
		}
		buf := make([]byte, r.db.PageSize)
		for _, bp := range pages {
			if err := r.db.ReadPage(bp, buf); err != nil {
				return nil, fmt.Errorf("blob %d:%d: %w", id.Relation, id.Number, err)
			}
			if buf[0] != fdb.TypeBlob {
				return nil, fmt.Errorf("blob %d:%d: page %d is not a blob page", id.Relation, id.Number, bp)
			}
			n := int(le.Uint16(buf[blpLength:]))
			if blpDataOffset+n > len(buf) {
				return nil, fmt.Errorf("blob %d:%d: blob page %d claims %d bytes", id.Relation, id.Number, bp, n)
			}
			data = append(data, buf[blpDataOffset:blpDataOffset+n]...)
		}
	case 2:
		return nil, ErrBlobLevel2
	default:
		return nil, fmt.Errorf("blob %d:%d: unknown level %d", id.Relation, id.Number, b[blhLevel])
	}

	// Segments: 2-byte length, then the bytes.
	out := make([]byte, 0, length)
	for i := 0; i < len(data); {
		if i+2 > len(data) {
			return nil, fmt.Errorf("blob %d:%d: truncated segment header", id.Relation, id.Number)
		}
		n := int(le.Uint16(data[i:]))
		if i+2+n > len(data) {
			return nil, fmt.Errorf("blob %d:%d: segment of %d bytes runs past the data", id.Relation, id.Number, n)
		}
		out = append(out, data[i+2:i+2+n]...)
		i += 2 + n
	}
	if len(out) != length {
		return nil, fmt.Errorf("blob %d:%d: %d bytes read, header says %d", id.Relation, id.Number, len(out), length)
	}
	return out, nil
}
