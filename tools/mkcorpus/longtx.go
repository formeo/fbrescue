// longtx simulates Firebird 4+ records with 48-bit transaction numbers.
// It rewrites a copy of an ODS 13 database without changing its file size.
package main

import (
	"encoding/binary"
	"fmt"
	"os"
)

const (
	dataPage       = 5
	blobFlag       = 0x0010
	incompleteFlag = 0x0008
	longTxFlag     = 0x0400
)

func main() {
	if len(os.Args) != 3 {
		_, _ = fmt.Fprintf(os.Stderr, "usage: go run longtx.go <input.fdb> <output.fdb>\n")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if len(b) < 0x80 {
		return fmt.Errorf("%s: not an ODS 13 database", src)
	}
	ps := int(binary.LittleEndian.Uint16(b[0x10:]))
	ods := binary.LittleEndian.Uint16(b[0x12:])
	if ps < 1024 || ps&(ps-1) != 0 || ods&^0x8000 != 13 {
		return fmt.Errorf("%s: not an ODS 13 database", src)
	}
	for off := 0; off+ps <= len(b); off += ps {
		page := b[off : off+ps]
		if page[0] == dataPage {
			if err := rewritePage(page); err != nil {
				return fmt.Errorf("page %d: %w", off/ps, err)
			}
		}
	}

	// Make every rewritten transaction older than OIT, hence committed without
	// requiring billions of TIP entries. Next/OIT/OAT/OST share high word 1.
	next := binary.LittleEndian.Uint32(b[0x24:])
	binary.LittleEndian.PutUint32(b[0x24:], next+1)
	binary.LittleEndian.PutUint32(b[0x1c:], next+1)
	binary.LittleEndian.PutUint32(b[0x20:], next+1)
	binary.LittleEndian.PutUint32(b[0x48:], next+1)
	for _, off := range []int{0x78, 0x7a, 0x7c, 0x7e} {
		binary.LittleEndian.PutUint16(b[off:], 1)
	}
	return os.WriteFile(dst, b, 0o644)
}

func rewritePage(page []byte) error {
	count := int(binary.LittleEndian.Uint16(page[0x16:]))
	headerEnd := 0x18 + 4*count
	if headerEnd > len(page) {
		return fmt.Errorf("slot array is outside the page")
	}
	type record struct {
		line int
		data []byte
	}
	var records []record
	for line := 0; line < count; line++ {
		off := int(binary.LittleEndian.Uint16(page[0x18+4*line:]))
		n := int(binary.LittleEndian.Uint16(page[0x1a+4*line:]))
		if n == 0 {
			continue
		}
		if off < headerEnd || off+n > len(page) || n < 13 {
			return fmt.Errorf("line %d is outside the page", line)
		}
		rec := append([]byte(nil), page[off:off+n]...)
		flags := binary.LittleEndian.Uint16(rec[10:])
		if flags&blobFlag == 0 {
			if flags&longTxFlag != 0 {
				return fmt.Errorf("line %d already has a long transaction", line)
			}
			flags |= longTxFlag
			binary.LittleEndian.PutUint16(rec[10:], flags)
			if flags&incompleteFlag != 0 {
				// rhdf keeps the next fragment at 16/20: the high word
				// replaces two of the three padding bytes at 13-15.
				if len(rec) < 22 {
					return fmt.Errorf("line %d has a short fragment header", line)
				}
				rec[13] = 0
				binary.LittleEndian.PutUint16(rec[14:], 1)
			} else {
				// rhde adds padding at 13 and the high word at 14.
				long := make([]byte, len(rec)+3)
				copy(long, rec[:13])
				binary.LittleEndian.PutUint16(long[14:], 1)
				copy(long[16:], rec[13:])
				rec = long
			}
		}
		records = append(records, record{line: line, data: rec})
	}

	out := make([]byte, len(page))
	copy(out, page[:headerEnd])
	pos := len(out)
	for _, rec := range records {
		pos = (pos - len(rec.data)) &^ 3
		if pos < headerEnd {
			return fmt.Errorf("long transaction headers do not fit")
		}
		copy(out[pos:], rec.data)
		binary.LittleEndian.PutUint16(out[0x18+4*rec.line:], uint16(pos))
		binary.LittleEndian.PutUint16(out[0x1a+4*rec.line:], uint16(len(rec.data)))
	}
	copy(page, out)
	return nil
}
