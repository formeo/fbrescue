package corpus

import (
	"encoding/binary"
	"os"
	"strconv"
	"strings"
	"testing"
)

// TestCorpusMatchesGstat checks that every corpus file starts with a header
// page whose page size and ODS major version agree with the engine's gstat.
// The offsets used here (type at 0x00, page size at 0x10, ODS at 0x12) are
// the same in ODS 11, 12 and 13.
func TestCorpusMatchesGstat(t *testing.T) {
	for _, db := range List(t) {
		t.Run(db.Engine+"/"+db.Name, func(t *testing.T) {
			f, err := os.Open(db.Path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			hdr := make([]byte, 0x14)
			if _, err := f.ReadAt(hdr, 0); err != nil {
				t.Fatal(err)
			}

			if hdr[0] != 1 {
				t.Errorf("page 0 type = %d, want 1 (header)", hdr[0])
			}
			if got, want := strconv.Itoa(int(binary.LittleEndian.Uint16(hdr[0x10:]))), db.Gstat("Page size"); got != want {
				t.Errorf("page size = %s, gstat says %q", got, want)
			}
			major := binary.LittleEndian.Uint16(hdr[0x12:]) & 0x7FFF
			wantMajor, _, _ := strings.Cut(db.Gstat("ODS version"), ".")
			if got := strconv.Itoa(int(major)); got != wantMajor {
				t.Errorf("ODS major = %s, gstat says %q", got, db.Gstat("ODS version"))
			}
		})
	}
}
