// Package fdb opens Firebird database files read-only and reads their pages.
package fdb

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Page types (byte 0 of every page).
const (
	TypeHeader    = 1
	TypePIP       = 2
	TypeTIP       = 3
	TypePointer   = 4
	TypeData      = 5
	TypeIndexRoot = 6
	TypeBTree     = 7
	TypeBlob      = 8
	TypeGenerator = 9
	TypeMax       = 10 // highest page type seen in ODS 11-13
)

const (
	MinPageSize = 1024
	MaxPageSize = 32768
)

// ODS 11 writes this constant into the checksum field of every page.
const ods11Checksum = 12345

func validPageSize(ps int) bool {
	return ps >= MinPageSize && ps <= MaxPageSize && ps&(ps-1) == 0
}

// File is an open database: the main file plus any secondary files of a
// multi-file database. Files are only ever opened for reading.
type File struct {
	PageSize int
	Family   Family
	Header   *Header  // nil if the header page is damaged
	Pages    uint32   // logical pages that can be read
	Files    []Part   // the main file first
	Warnings []string // problems found while opening

	moved map[uint32]uint32 // logical page -> page whose location holds it (see Relocate)
}

// Part is one physical file of the database.
type Part struct {
	Path      string
	FirstPage uint32 // first logical page stored in this file
	Pages     uint32 // complete logical pages stored in this file
	f         *os.File
	skip      int64 // pages before FirstPage: secondary files start with their own header page
}

// Open opens a database file read-only.
func Open(path string) (*File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	db := &File{}
	if err := db.init(path, f); err != nil {
		f.Close()
		return nil, err
	}
	return db, nil
}

func (db *File) init(path string, f *os.File) error {
	size, err := fileSize(f)
	if err != nil {
		return err
	}
	first := make([]byte, min(size, MaxPageSize))
	if _, err := f.ReadAt(first, 0); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if len(first) < MinPageSize {
		return fmt.Errorf("%s: file is too small to be a database (%d bytes)", path, size)
	}

	h, err := parseHeader(first)
	switch {
	case err == nil:
		db.Header, db.PageSize, db.Family = h, h.PageSize, h.Family()
	case errors.Is(err, errNotHeader):
		ps, fam, ok := detectPageSize(f, size)
		if !ok {
			return fmt.Errorf("%s: header page is damaged and the page size could not be detected", path)
		}
		db.PageSize, db.Family = ps, fam
		db.warn("header page is damaged: page size %d detected from the other pages; header fields are unknown", ps)
	default:
		return fmt.Errorf("%s: %w", path, err)
	}

	db.addPart(path, f, size, 0, 0, h)
	for h != nil && h.Continuation != "" && len(db.Files) < 1000 { // bound guards against a cycle
		next := len(db.Files)
		cpath, cf, csize, err := openContinuation(path, h.Continuation)
		if err != nil {
			db.warn("secondary file %s not found: pages from %d on are unavailable", h.Continuation, db.Pages)
			break
		}
		buf := make([]byte, db.PageSize)
		if _, err := cf.ReadAt(buf, 0); err != nil {
			cf.Close()
			db.warn("secondary file %s: %v", cpath, err)
			break
		}
		h, err = parseHeader(buf)
		if err != nil {
			db.warn("secondary file %s: damaged header page, its continuation (if any) is unknown", cpath)
		}
		db.addPart(cpath, cf, csize, db.Pages, 1, h)
		if db.Files[next].Pages == 0 {
			break
		}
	}
	return nil
}

func (db *File) addPart(path string, f *os.File, size int64, first uint32, skip int64, h *Header) {
	ps := int64(db.PageSize)
	n := size/ps - skip
	if r := size % ps; r != 0 {
		db.warn("%s ends with an incomplete page (%d of %d bytes); it is ignored", filepath.Base(path), r, ps)
	}
	if h != nil && h.Continuation != "" && int64(h.LastPage)+1 >= int64(first) {
		if want := int64(h.LastPage) + 1 - int64(first); want < n {
			n = want
		} else if want > n {
			db.warn("%s is shorter than its header says: %d of %d pages", filepath.Base(path), n, want)
		}
	}
	if n < 0 {
		n = 0
	}
	db.Files = append(db.Files, Part{Path: path, FirstPage: first, Pages: uint32(n), f: f, skip: skip})
	db.Pages = first + uint32(n)
}

// openContinuation prefers a file with the same name next to the main file
// (a copy taken for recovery) over the absolute path stored in the header,
// which may point at the live production file.
func openContinuation(mainPath, stored string) (string, *os.File, int64, error) {
	candidates := []string{filepath.Join(filepath.Dir(mainPath), baseName(stored)), stored}
	var lastErr error
	for _, p := range candidates {
		f, err := os.Open(p)
		if err != nil {
			lastErr = err
			continue
		}
		size, err := fileSize(f)
		if err != nil {
			f.Close()
			lastErr = err
			continue
		}
		return p, f, size, nil
	}
	return "", nil, 0, lastErr
}

// baseName handles both Windows and Unix separators: the stored path comes
// from whatever server created the database.
func baseName(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == '\\' {
			return p[i+1:]
		}
	}
	return p
}

func fileSize(f *os.File) (int64, error) {
	st, err := f.Stat()
	if err != nil {
		return 0, err
	}
	return st.Size(), nil
}

func (db *File) warn(format string, args ...any) {
	db.Warnings = append(db.Warnings, fmt.Sprintf(format, args...))
}

// ReadPage reads logical page n into buf, which must be PageSize long.
func (db *File) ReadPage(n uint32, buf []byte) error {
	if at, ok := db.moved[n]; ok {
		n = at
	}
	for i := len(db.Files) - 1; i >= 0; i-- {
		p := &db.Files[i]
		if n < p.FirstPage {
			continue
		}
		if n-p.FirstPage >= p.Pages {
			break
		}
		_, err := p.f.ReadAt(buf[:db.PageSize], (int64(n-p.FirstPage)+p.skip)*int64(db.PageSize))
		return err
	}
	return fmt.Errorf("page %d is beyond the end of the database (%d pages)", n, db.Pages)
}

// Close closes all files of the database.
func (db *File) Close() error {
	var err error
	for _, p := range db.Files {
		err = errors.Join(err, p.f.Close())
	}
	return err
}

// PageNumber returns the page's own number, stored by ODS 12+ on every page.
func PageNumber(page []byte) uint32 { return binary.LittleEndian.Uint32(page[12:]) }

// CheckNumber reports a page read as page n that holds the contents of
// another page (ODS 12+ only: ODS 11 pages do not carry their number).
func (db *File) CheckNumber(n uint32, page []byte) error {
	if db.Family == ODS12Plus && page[0] != 0 && PageNumber(page) != n {
		return fmt.Errorf("page %d holds the contents of page %d", n, PageNumber(page))
	}
	return nil
}

// Relocate makes ReadPage(n) read the contents stored at the location of
// page at: for a page found in the wrong place (ODS 12+ pages carry their
// own number).
func (db *File) Relocate(n, at uint32) {
	if db.moved == nil {
		db.moved = map[uint32]uint32{}
	}
	db.moved[n] = at
}

// detectPageSize finds the page size from page signatures when the header
// is unreadable: ODS 12+ stores each page's own number at offset 12, ODS 11
// writes the constant 12345 into every page's checksum field.
func detectPageSize(f *os.File, size int64) (int, Family, bool) {
	const sample = 4096
	buf := make([]byte, 16)
	for ps := MinPageSize; ps <= MaxPageSize; ps *= 2 {
		n := size / int64(ps)
		if n < 2 {
			break
		}
		var typed, numbered, checksummed int
		for p := int64(1); p < n && p <= sample; p++ {
			if _, err := f.ReadAt(buf, p*int64(ps)); err != nil {
				break
			}
			if buf[0] == 0 {
				continue
			}
			typed++
			if PageNumber(buf) == uint32(p) {
				numbered++
			}
			if binary.LittleEndian.Uint16(buf[2:]) == ods11Checksum {
				checksummed++
			}
		}
		sampled := min(n-1, sample)
		switch {
		case typed > 0 && numbered*10 >= typed*9:
			return ps, ODS12Plus, true
		case int64(checksummed)*10 >= sampled*9:
			return ps, ODS11, true
		}
	}
	return 0, 0, false
}
