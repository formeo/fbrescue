package record

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/formeo/fbrescue/internal/fdb"
)

func TestUnpack(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   []byte
		long bool
		want []byte
	}{
		{"literal", []byte{3, 'a', 'b', 'c'}, false, []byte("abc")},
		{"run", []byte{0xFD, 'x'}, false, []byte("xxx")},
		{"empty literal pads a fragment", []byte{2, 'a', 'b', 0}, false, []byte("ab")},
		{"classic -1 repeats once", []byte{0xFF, 'z'}, false, []byte("z")},
		{"ODS 13.1 long run", []byte{1, 'a', 0xFF, 0x2C, 0x01, ' '}, true, append([]byte("a"), bytes.Repeat([]byte(" "), 300)...)},
		{"ODS 13.1 short run", []byte{0xFD, 'q'}, true, []byte("qqq")},
	} {
		got, err := Unpack(tc.in, tc.long)
		if err != nil || !bytes.Equal(got, tc.want) {
			t.Errorf("%s: got %q, %v; want %q", tc.name, got, err, tc.want)
		}
	}
	for _, tc := range []struct {
		name string
		in   []byte
		long bool
	}{
		{"literal past the end", []byte{5, 'a'}, false},
		{"run without its byte", []byte{0xFD}, false},
		{"long run truncated", []byte{0xFF, 0x10}, true},
		{"-2 in ODS 13.1", []byte{0xFE, 1, 2, 3, 4, 'x'}, true},
	} {
		if _, err := Unpack(tc.in, tc.long); err == nil {
			t.Errorf("%s: no error", tc.name)
		}
	}
}

func TestBlobID(t *testing.T) {
	id := ParseBlobID([]byte{0x81, 0, 2, 0, 0x10, 0, 0, 0})
	if id.Relation != 129 || id.Number != 2<<32|16 {
		t.Errorf("got %+v", id)
	}
	if MaxRecordsPerPage(4096) != 239 || MaxRecordsPerPage(8192) != 480 {
		t.Errorf("records per page: %d, %d", MaxRecordsPerPage(4096), MaxRecordsPerPage(8192))
	}
}

func TestReadLongTx(t *testing.T) {
	for _, fragmented := range []bool{false, true} {
		t.Run(map[bool]string{false: "single", true: "fragmented"}[fragmented], func(t *testing.T) {
			const pageSize = 1024
			contents := make([]byte, 3*pageSize)
			contents[0] = fdb.TypeHeader
			le.PutUint16(contents[0x10:], pageSize)
			le.PutUint16(contents[0x12:], 13|0x8000)

			putRecord := func(page []byte, flags uint16, body []byte, nextPage uint32) {
				page[0] = fdb.TypeData
				le.PutUint16(page[0x16:], 1)
				n := longTxHeaderSize
				if flags&FlagIncomplete != 0 {
					n = fragmentHeaderSize
				}
				off := len(page) - n - len(body)
				le.PutUint16(page[0x18:], uint16(off))
				le.PutUint16(page[0x1a:], uint16(n+len(body)))
				rec := page[off:]
				le.PutUint32(rec[0:], 0x89abcdef)
				le.PutUint16(rec[10:], flags)
				rec[12] = 7
				le.PutUint16(rec[14:], 0x1234)
				if flags&FlagIncomplete != 0 {
					le.PutUint32(rec[16:], nextPage)
				}
				copy(rec[n:], body)
			}

			flags := uint16(FlagLongTx)
			body := []byte{3, 'a', 'b', 'c'}
			if fragmented {
				flags |= FlagIncomplete
				body = []byte{1, 'a'}
			}
			putRecord(contents[pageSize:2*pageSize], flags, body, 2)
			le.PutUint32(contents[pageSize+12:], 1)
			if fragmented {
				putRecord(contents[2*pageSize:], FlagLongTx|FlagFragment, []byte{2, 'b', 'c'}, 0)
				le.PutUint32(contents[2*pageSize+12:], 2)
			}

			path := filepath.Join(t.TempDir(), "longtx.fdb")
			if err := os.WriteFile(path, contents, 0o600); err != nil {
				t.Fatal(err)
			}
			db, err := fdb.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = db.Close() }()
			h, data, err := NewReader(db).Read(1, 0)
			if err != nil {
				t.Fatal(err)
			}
			if want := uint64(0x123489abcdef); h.Tx != want {
				t.Errorf("transaction = %#x, want %#x", h.Tx, want)
			}
			if !bytes.Equal(data, []byte("abc")) {
				t.Errorf("data = %q, want abc", data)
			}
		})
	}
}

func TestLongTxHeaderTruncated(t *testing.T) {
	for _, tc := range []struct {
		name  string
		flags uint16
		n     int
	}{
		{name: "record", flags: FlagLongTx, n: longTxHeaderSize - 1},
		{name: "fragment", flags: FlagLongTx | FlagIncomplete, n: fragmentHeaderSize - 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page := make([]byte, 1024)
			le.PutUint16(page[0x16:], 1)
			le.PutUint16(page[0x18:], 100)
			le.PutUint16(page[0x1a:], uint16(tc.n))
			le.PutUint16(page[110:], tc.flags)
			if _, err := slot(page, 0); err == nil {
				t.Fatal("truncated long transaction header was accepted")
			}
		})
	}
}
