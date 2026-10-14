package fdb

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/formeo/fbrescue/internal/corpus"
)

func TestHeaderMatchesGstat(t *testing.T) {
	for _, db := range corpus.List(t) {
		t.Run(db.Engine+"/"+db.Name, func(t *testing.T) {
			f, err := Open(db.Path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			h := f.Header
			if h == nil {
				t.Fatalf("header not parsed; warnings: %v", f.Warnings)
			}
			check := func(key, got string) {
				t.Helper()
				if want := db.Gstat(key); got != want {
					t.Errorf("%s = %q, gstat says %q", key, got, want)
				}
			}
			check("Page size", fmt.Sprint(h.PageSize))
			check("ODS version", fmt.Sprintf("%d.%d", h.ODSMajor, h.ODSMinor))
			check("Oldest transaction", fmt.Sprint(h.OIT))
			check("Oldest active", fmt.Sprint(h.OAT))
			check("Oldest snapshot", fmt.Sprint(h.OST))
			check("Next transaction", fmt.Sprint(h.NextTx))
			check("Next attachment ID", fmt.Sprint(h.NextAttachment))
			check("Page buffers", fmt.Sprint(h.PageBuffers))
			check("Database dialect", fmt.Sprint(h.Dialect()))
			// gstat prints the hour without a leading zero: compare as times.
			if want, err := time.Parse("Jan 2, 2006 15:04:05", db.Gstat("Creation date")); err != nil || !h.Created.Truncate(time.Second).Equal(want) {
				t.Errorf("Creation date = %v, gstat says %q (%v)", h.Created, db.Gstat("Creation date"), err)
			}

			names, unknown := h.Attributes()
			want := strings.Split(db.Gstat("Attributes"), ", ")
			if want[0] == "" {
				want = nil
			}
			slices.Sort(names)
			slices.Sort(want)
			if !slices.Equal(names, want) {
				t.Errorf("attributes = %q, gstat says %q", names, want)
			}
			if unknown != 0 {
				t.Errorf("unknown flag bits %#04x", unknown)
			}
			if len(f.Warnings) != 0 {
				t.Errorf("warnings on a healthy file: %v", f.Warnings)
			}
		})
	}
}

func TestMultiFile(t *testing.T) {
	for _, db := range corpus.List(t) {
		if db.Name != "multifile" {
			continue
		}
		t.Run(db.Engine, func(t *testing.T) {
			// Copy both files to a new directory: the copy next to the main
			// file must win over the absolute path stored in the header.
			dir := t.TempDir()
			main := copyFile(t, db.Path, dir)
			second := copyFile(t, strings.TrimSuffix(db.Path, ".fdb")+".f2", dir)

			f, err := Open(main)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			if len(f.Files) != 2 || f.Files[1].Path != second {
				t.Fatalf("files = %+v", f.Files)
			}
			if f.Files[0].Pages != 300 || f.Files[1].FirstPage != 300 {
				t.Errorf("main file holds %d pages, second starts at %d; want 300, 300", f.Files[0].Pages, f.Files[1].FirstPage)
			}
			buf := make([]byte, f.PageSize)
			for _, n := range []uint32{299, 300, f.Pages - 1} {
				if err := f.ReadPage(n, buf); err != nil {
					t.Fatal(err)
				}
				if f.Family == ODS12Plus && buf[0] != 0 && PageNumber(buf) != n {
					t.Errorf("page %d holds page number %d", n, PageNumber(buf))
				}
			}
		})
	}
}

func TestDamagedHeaderDetectsPageSize(t *testing.T) {
	for _, db := range corpus.List(t) {
		if db.Name != "base" && db.Name != "manytx" {
			continue
		}
		t.Run(db.Engine+"/"+db.Name, func(t *testing.T) {
			path := copyFile(t, db.Path, t.TempDir())
			overwrite(t, path, 0, make([]byte, 1024))

			f, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			if want := db.Gstat("Page size"); fmt.Sprint(f.PageSize) != want {
				t.Errorf("detected page size %d, want %s", f.PageSize, want)
			}
			wantFamily := ODS11
			if !strings.HasPrefix(db.Gstat("ODS version"), "11.") {
				wantFamily = ODS12Plus
			}
			if f.Family != wantFamily || f.Header != nil || len(f.Warnings) != 1 {
				t.Errorf("family %d header %v warnings %q", f.Family, f.Header, f.Warnings)
			}
		})
	}
}

func TestTruncatedFileOpens(t *testing.T) {
	for _, db := range corpus.List(t) {
		if db.Name != "base" {
			continue
		}
		t.Run(db.Engine, func(t *testing.T) {
			path := copyFile(t, db.Path, t.TempDir())
			st, _ := os.Stat(path)
			if err := os.Truncate(path, st.Size()-100); err != nil {
				t.Fatal(err)
			}
			f, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			if want := uint32(st.Size()/int64(f.PageSize)) - 1; f.Pages != want {
				t.Errorf("pages = %d, want %d", f.Pages, want)
			}
			if len(f.Warnings) != 1 || !strings.Contains(f.Warnings[0], "incomplete page") {
				t.Errorf("warnings = %q", f.Warnings)
			}
		})
	}
}

func copyFile(t *testing.T, src, dir string) string {
	t.Helper()
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, filepath.Base(src))
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return dst
}

func overwrite(t *testing.T, path string, off int64, b []byte) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteAt(b, off); err != nil {
		t.Fatal(err)
	}
}
