// Package record reads records from data pages: the record header, the
// run-length encoding and records split into fragments across pages.
package record

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/formeo/fbrescue/internal/fdb"
)

var le = binary.LittleEndian

// Record header flags (rhd_flags).
const (
	FlagDeleted    = 0x0001 // delete stub
	FlagChain      = 0x0002 // back version of another record
	FlagFragment   = 0x0004 // continuation of a record split across pages
	FlagIncomplete = 0x0008 // more fragments follow
	FlagBlob       = 0x0010 // a blob stored on the data page, not a record
	FlagDelta      = 0x0020 // the back version is stored as differences
	FlagLarge      = 0x0040
	FlagDamaged    = 0x0080 // marked damaged by the engine
	FlagLongTx     = 0x0400 // 48-bit transaction number (ODS 12+)
	FlagNotPacked  = 0x0800 // stored without run-length encoding (ODS 13.1)
)

const (
	headerSize         = 13 // transaction 4, back page 4, back line 2, flags 2, format 1
	fragmentHeaderSize = 22 // header, padding, next fragment page 4 at 16, line 2 at 20
	dataPageHeader     = 0x18
)

// Header is the fixed part of a record version.
type Header struct {
	Tx       uint32
	BackPage uint32
	BackLine uint16
	Flags    uint16
	Format   uint8
}

// Primary reports whether this is the newest version of a row (or a delete
// stub), as opposed to a back version, a fragment or a blob.
func (h Header) Primary() bool {
	return h.Flags&(FlagChain|FlagFragment|FlagBlob) == 0
}

// MaxRecordsPerPage is the record number multiplier in record numbers and
// blob ids: record number = data page sequence * this + line.
func MaxRecordsPerPage(pageSize int) int { return (pageSize - dataPageHeader) / (4 + headerSize) }

// Reader reads records of one database.
type Reader struct {
	db        *fdb.File
	packed131 bool // ODS 13.1: each fragment packed on its own, long runs
	page      []byte
	pageNo    uint32
	frag      []byte // separate buffer for fragment pages
}

// NewReader returns a reader for db. db.Header must be known: the record
// encoding depends on the ODS version.
func NewReader(db *fdb.File) *Reader {
	h := db.Header
	return &Reader{
		db:        db,
		packed131: h != nil && (h.ODSMajor > 13 || h.ODSMajor == 13 && h.ODSMinor >= 1),
		page:      make([]byte, db.PageSize),
		pageNo:    ^uint32(0),
		frag:      make([]byte, db.PageSize),
	}
}

// Slot is one entry of a data page's slot array.
type Slot struct {
	Line   int
	Header Header
}

// DataPage reads a data page and returns its relation, sequence and the
// headers of its non-empty slots.
func (r *Reader) DataPage(n uint32) (rel uint16, seq uint32, slots []Slot, err error) {
	if err := r.load(n); err != nil {
		return 0, 0, nil, err
	}
	b := r.page
	if b[0] != fdb.TypeData {
		return 0, 0, nil, fmt.Errorf("page %d is not a data page (type %d)", n, b[0])
	}
	seq, rel = le.Uint32(b[0x10:]), le.Uint16(b[0x14:])
	count := int(le.Uint16(b[0x16:]))
	if dataPageHeader+4*count > len(b) {
		return rel, seq, nil, fmt.Errorf("page %d: %d slots do not fit in the page", n, count)
	}
	for i := 0; i < count; i++ {
		frag, err := slot(b, i)
		if err != nil {
			return rel, seq, slots, fmt.Errorf("page %d: %w", n, err)
		}
		if frag == nil {
			continue
		}
		slots = append(slots, Slot{Line: i, Header: parseHeader(frag)})
	}
	return rel, seq, slots, nil
}

func (r *Reader) load(n uint32) error {
	if n == r.pageNo {
		return nil
	}
	r.pageNo = ^uint32(0)
	if err := r.db.ReadPage(n, r.page); err != nil {
		return err
	}
	r.pageNo = n
	return nil
}

// slot returns the bytes of slot i, nil for an empty slot.
func slot(b []byte, i int) ([]byte, error) {
	count := int(le.Uint16(b[0x16:]))
	if i >= count {
		return nil, fmt.Errorf("line %d does not exist (%d slots)", i, count)
	}
	off, n := int(le.Uint16(b[dataPageHeader+4*i:])), int(le.Uint16(b[dataPageHeader+4*i+2:]))
	if n == 0 {
		return nil, nil
	}
	if off < dataPageHeader+4*count || off+n > len(b) || n < headerSize {
		return nil, fmt.Errorf("line %d points outside the page (offset %d, length %d)", i, off, n)
	}
	return b[off : off+n], nil
}

func parseHeader(b []byte) Header {
	return Header{
		Tx:       le.Uint32(b[0:]),
		BackPage: le.Uint32(b[4:]),
		BackLine: le.Uint16(b[8:]),
		Flags:    le.Uint16(b[10:]),
		Format:   b[12],
	}
}

// Read returns the header and the unpacked data of the record at page n,
// line, following fragments to other pages.
func (r *Reader) Read(n uint32, line int) (Header, []byte, error) {
	if err := r.load(n); err != nil {
		return Header{}, nil, err
	}
	b, err := slot(r.page, line)
	if err != nil || b == nil {
		if err == nil {
			err = fmt.Errorf("line %d is empty", line)
		}
		return Header{}, nil, fmt.Errorf("page %d: %w", n, err)
	}
	h := parseHeader(b)
	var data []byte
	if r.packed131 || h.Flags&FlagIncomplete == 0 {
		// ODS 13.1 packs each fragment separately; a single fragment is
		// the same in every version.
		data, err = r.unpackFragment(b, h.Flags, nil)
	} else {
		data = append(data, b[fragmentHeaderSize:]...)
	}
	if err != nil {
		return h, nil, fmt.Errorf("page %d line %d: %w", n, line, err)
	}

	flags, cur := h.Flags, b
	for hops := 0; flags&FlagIncomplete != 0; hops++ {
		if len(cur) < fragmentHeaderSize || hops > 1<<16 {
			return h, nil, fmt.Errorf("page %d line %d: broken fragment chain", n, line)
		}
		fp, fl := le.Uint32(cur[16:]), int(le.Uint16(cur[20:]))
		if err := r.db.ReadPage(fp, r.frag); err != nil {
			return h, nil, fmt.Errorf("page %d line %d: fragment at page %d: %w", n, line, fp, err)
		}
		if r.frag[0] != fdb.TypeData {
			return h, nil, fmt.Errorf("page %d line %d: fragment page %d is not a data page", n, line, fp)
		}
		cur, err = slot(r.frag, fl)
		if err == nil && cur == nil {
			err = fmt.Errorf("line %d is empty", fl)
		}
		if err != nil {
			return h, nil, fmt.Errorf("page %d line %d: fragment at page %d: %w", n, line, fp, err)
		}
		flags = le.Uint16(cur[10:])
		if flags&FlagFragment == 0 {
			return h, nil, fmt.Errorf("page %d line %d: page %d line %d is not a fragment", n, line, fp, fl)
		}
		if r.packed131 {
			data, err = r.unpackFragment(cur, flags, data)
			if err != nil {
				return h, nil, fmt.Errorf("page %d line %d: fragment at page %d: %w", n, line, fp, err)
			}
		} else if flags&FlagIncomplete != 0 {
			data = append(data, cur[fragmentHeaderSize:]...)
		} else {
			data = append(data, cur[headerSize:]...)
		}
	}
	if !r.packed131 && h.Flags&FlagIncomplete != 0 {
		if data, err = Unpack(data, false); err != nil {
			return h, nil, fmt.Errorf("page %d line %d: %w", n, line, err)
		}
	}
	return h, data, nil
}

// unpackFragment appends the unpacked data of one stored record piece.
func (r *Reader) unpackFragment(b []byte, flags uint16, dst []byte) ([]byte, error) {
	body := b[headerSize:]
	if flags&FlagIncomplete != 0 {
		if len(b) < fragmentHeaderSize {
			return nil, errors.New("fragment header is truncated")
		}
		body = b[fragmentHeaderSize:]
	}
	if flags&FlagNotPacked != 0 {
		return append(dst, body...), nil
	}
	u, err := Unpack(body, r.packed131)
	return append(dst, u...), err
}

// Unpack decodes Firebird's run-length encoding. A positive control byte n
// is followed by n literal bytes; a negative one by a byte repeated -n
// times; 0 is an empty literal (it pads the end of a fragment). ODS 13.1
// adds -1: a 2-byte run length, then the byte.
func Unpack(b []byte, long bool) ([]byte, error) {
	out := make([]byte, 0, len(b)*2)
	for i := 0; i < len(b); {
		c := int(int8(b[i]))
		i++
		switch {
		case c == 0:
		case c > 0:
			if i+c > len(b) {
				return nil, fmt.Errorf("packed data: literal run of %d bytes at offset %d runs past the end", c, i-1)
			}
			out = append(out, b[i:i+c]...)
			i += c
		case c == -1 && long:
			if i+3 > len(b) {
				return nil, fmt.Errorf("packed data: long run at offset %d is truncated", i-1)
			}
			n, v := int(le.Uint16(b[i:])), b[i+2]
			for k := 0; k < n; k++ {
				out = append(out, v)
			}
			i += 3
		case c < 0 && (c < -2 || !long):
			if i >= len(b) {
				return nil, fmt.Errorf("packed data: run at offset %d is truncated", i-1)
			}
			for k := 0; k < -c; k++ {
				out = append(out, b[i])
			}
			i++
		default:
			return nil, fmt.Errorf("packed data: unsupported control byte %d at offset %d", c, i-1)
		}
	}
	return out, nil
}
