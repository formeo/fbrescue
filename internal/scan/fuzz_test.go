package scan

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/formeo/fbrescue/internal/corpus"
	"github.com/formeo/fbrescue/internal/fdb"
)

const (
	fuzzPageSize  = 1024
	fuzzDataPages = 3
)

func FuzzRun(f *testing.F) {
	seed := make([]byte, fuzzDataPages*fuzzPageSize)
	seed[0] = fdb.TypePIP
	seed[fuzzPageSize] = fdb.TypeTIP
	seed[2*fuzzPageSize] = fdb.TypePointer
	f.Add(seed)
	f.Add(seed[:100])
	for _, page := range corpus.FuzzPages(f, 6) {
		f.Add(page[:min(len(page), len(seed))])
	}

	path := filepath.Join(f.TempDir(), "fuzz.fdb")
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > fuzzDataPages*fuzzPageSize {
			return
		}
		contents := make([]byte, (fuzzDataPages+1)*fuzzPageSize)
		contents[0] = fdb.TypeHeader
		le.PutUint16(contents[0x10:], fuzzPageSize)
		le.PutUint16(contents[0x12:], 11|0x8000)
		copy(contents[fuzzPageSize:], data)
		if err := os.WriteFile(path, contents, 0o600); err != nil {
			t.Fatal(err)
		}

		db, err := fdb.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		_ = Run(db, nil)
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}

		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, contents) {
			t.Fatal("scan modified the source file")
		}
	})
}
