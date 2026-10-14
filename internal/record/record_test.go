package record

import (
	"bytes"
	"testing"
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
