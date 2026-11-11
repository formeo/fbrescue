package fdb

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/formeo/fbrescue/internal/corpus"
)

func FuzzOpen(f *testing.F) {
	f.Add([]byte(nil))
	for _, major := range []uint16{11, 12, 13} {
		header := make([]byte, MinPageSize)
		header[0] = TypeHeader
		binary.LittleEndian.PutUint16(header[0x10:], MinPageSize)
		binary.LittleEndian.PutUint16(header[0x12:], major|odsFirebirdFlag)
		f.Add(header)
		f.Add(header[:100])
	}
	for _, page := range corpus.FuzzPages(f, 1) {
		f.Add(page)
	}

	path := filepath.Join(f.TempDir(), "fuzz.fdb")
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > MaxPageSize {
			return
		}
		// Do not let a fuzzed header open an arbitrary continuation path.
		if len(data) >= MinPageSize {
			if h, err := parseHeader(data); err == nil && h.Continuation != "" {
				return
			}
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}

		db, err := Open(path)
		if err == nil {
			buf := make([]byte, db.PageSize)
			for n := uint32(0); n < db.Pages; n++ {
				if err := db.ReadPage(n, buf); err == nil {
					_ = db.CheckNumber(n, buf)
				}
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
		}

		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, data) {
			t.Fatal("Open or ReadPage modified the source file")
		}
	})
}
