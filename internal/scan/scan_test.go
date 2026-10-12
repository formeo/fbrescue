package scan

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/formeo/fbrescue/internal/corpus"
	"github.com/formeo/fbrescue/internal/fdb"
)

func scanFile(t *testing.T, path string) (*fdb.File, *Report) {
	t.Helper()
	f, err := fdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f, Run(f, nil)
}

// TestHealthyCorpusMatchesGstat: on every database created by a real
// engine there are no problems, and every table's pointer page, data page,
// record and version counts equal gstat's.
func TestHealthyCorpusMatchesGstat(t *testing.T) {
	for _, db := range corpus.List(t) {
		t.Run(db.Engine+"/"+db.Name, func(t *testing.T) {
			f, r := scanFile(t, db.Path)
			if db.Name == "hiwords" {
				// The patched header claims ~2^32 transactions the TIP never saw.
				if len(r.Problems) != 1 || r.Problems[0].Kind != TIPChain {
					t.Errorf("problems = %+v, want one %s", r.Problems, TIPChain)
				}
				return
			}
			for _, p := range r.Problems {
				t.Errorf("problem on a healthy database: %+v", p)
			}
			if r.UsedPages+r.FreePages != f.Pages {
				t.Errorf("used %d + free %d != %d pages", r.UsedPages, r.FreePages, f.Pages)
			}

			tx := r.Transactions
			if tx == nil {
				t.Fatal("no transaction summary")
			}
			if tx.Committed+tx.Active+tx.Dead+tx.Limbo != tx.Next {
				t.Errorf("transaction states %+v do not add up to %d", tx, tx.Next)
			}
			// Everything older than the oldest interesting transaction is committed.
			if tx.Committed < f.Header.OIT-1 {
				t.Errorf("committed %d < OIT-1 = %d", tx.Committed, f.Header.OIT-1)
			}

			want, ok := db.Relations(t)
			if !ok {
				return
			}
			got := map[uint16]Relation{}
			for _, rel := range r.Relations {
				got[rel.ID] = rel
			}
			for id, w := range want {
				g, found := got[id]
				if !found && w.DataPages == 0 {
					continue // gstat lists tables that never got a pointer page
				}
				if g.FirstPointerPage != w.PrimaryPointerPage || g.DataPages != w.DataPages ||
					g.Records != w.Records || g.Versions != w.Versions {
					t.Errorf("table %s (%d): got pointer %d, data pages %d, records %d, versions %d; gstat: %d, %d, %d, %d",
						w.Name, id, g.FirstPointerPage, g.DataPages, g.Records, g.Versions,
						w.PrimaryPointerPage, w.DataPages, w.Records, w.Versions)
				}
			}
		})
	}
}

func TestDeletedRecords(t *testing.T) {
	for _, db := range corpus.List(t) {
		if db.Name != "deleted" {
			continue
		}
		t.Run(db.Engine, func(t *testing.T) {
			_, r := scanFile(t, db.Path)
			for _, rel := range r.Relations {
				if rel.ID == 129 && (rel.Deleted != 250 || rel.Records != 1000) {
					t.Errorf("T2: %d records, %d deleted; want 1000 and 250", rel.Records, rel.Deleted)
				}
			}
		})
	}
}

func TestTIPChain(t *testing.T) {
	for _, db := range corpus.List(t) {
		if db.Name != "manytx" {
			continue
		}
		t.Run(db.Engine, func(t *testing.T) {
			_, r := scanFile(t, db.Path)
			if n := r.PageTypes["transaction inventory"]; n != 3 {
				t.Errorf("%d TIP pages, want 3", n)
			}
		})
	}
}

// Damage tests: corrupt a copy of a healthy database in one specific way and
// check that the scan reports exactly that.

func TestZeroedDataPage(t *testing.T) {
	for _, db := range damageInputs(t) {
		t.Run(db.Engine, func(t *testing.T) {
			path, ps := copyDB(t, db.Path)
			dp := findPage(t, path, ps, fdb.TypeData, 128)
			writeAt(t, path, int64(dp)*int64(ps), make([]byte, ps))

			_, r := scanFile(t, path)
			wantProblem(t, r, dp, InvalidType)
			wantProblem(t, r, findPage(t, path, ps, fdb.TypePointer, 128), PointerTarget)
		})
	}
}

func TestMisplacedPage(t *testing.T) {
	for _, db := range damageInputs(t) {
		t.Run(db.Engine, func(t *testing.T) {
			path, ps := copyDB(t, db.Path)
			dp := findPage(t, path, ps, fdb.TypeData, 128)
			// Put a copy of the data page somewhere else in place of another data page.
			other := findPageAfter(t, path, ps, fdb.TypeData, 128, dp)
			page := readAt(t, path, int64(dp)*int64(ps), ps)
			writeAt(t, path, int64(other)*int64(ps), page)

			_, r := scanFile(t, path)
			if strings.HasPrefix(db.Gstat("ODS version"), "11.") {
				// ODS 11 has no page number on the page: the duplicate is
				// caught through the data page sequence in the pointer page.
				wantKind(t, r, PointerTarget)
			} else {
				wantProblem(t, r, other, WrongPageNumber)
			}
		})
	}
}

func TestBrokenPointerChain(t *testing.T) {
	for _, db := range damageInputs(t) {
		t.Run(db.Engine, func(t *testing.T) {
			path, ps := copyDB(t, db.Path)
			pp := findPage(t, path, ps, fdb.TypePointer, 128)
			writeAt(t, path, int64(pp)*int64(ps)+0x14, []byte{7, 0, 0, 0}) // ppg_next

			_, r := scanFile(t, path)
			wantProblem(t, r, pp, PointerChain)
		})
	}
}

func TestOrphanDataPage(t *testing.T) {
	for _, db := range damageInputs(t) {
		t.Run(db.Engine, func(t *testing.T) {
			path, ps := copyDB(t, db.Path)
			pp := findPage(t, path, ps, fdb.TypePointer, 128)
			b := readAt(t, path, int64(pp)*int64(ps), ps)
			dp := binary.LittleEndian.Uint32(b[0x20:])
			writeAt(t, path, int64(pp)*int64(ps)+0x20, []byte{0, 0, 0, 0}) // clear slot 0

			_, r := scanFile(t, path)
			wantProblem(t, r, dp, OrphanDataPage)
		})
	}
}

func TestMissingTIP(t *testing.T) {
	for _, db := range damageInputs(t) {
		t.Run(db.Engine, func(t *testing.T) {
			path, ps := copyDB(t, db.Path)
			tip := findPage(t, path, ps, fdb.TypeTIP, -1)
			writeAt(t, path, int64(tip)*int64(ps), []byte{fdb.TypeData})

			_, r := scanFile(t, path)
			wantProblem(t, r, 0, TIPChain)
			if r.Transactions != nil {
				t.Errorf("transactions counted without a TIP: %+v", r.Transactions)
			}
		})
	}
}

func TestTruncatedFile(t *testing.T) {
	for _, db := range damageInputs(t) {
		t.Run(db.Engine, func(t *testing.T) {
			path, ps := copyDB(t, db.Path)
			last := findLastPage(t, path, ps, fdb.TypeData, 128)
			if err := os.Truncate(path, int64(last)*int64(ps)+100); err != nil {
				t.Fatal(err)
			}
			_, r := scanFile(t, path)
			wantProblem(t, r, findPage(t, path, ps, fdb.TypePointer, 128), PointerTarget)
		})
	}
}

// damageInputs are the base databases, one per engine.
func damageInputs(t *testing.T) []corpus.DB {
	var dbs []corpus.DB
	for _, db := range corpus.List(t) {
		if db.Name == "base" {
			dbs = append(dbs, db)
		}
	}
	return dbs
}

func copyDB(t *testing.T, src string) (string, int) {
	t.Helper()
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), filepath.Base(src))
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return dst, int(binary.LittleEndian.Uint16(b[0x10:]))
}

// findPage returns the first page of the given type; for data and pointer
// pages, of the given relation (rel -1 means any).
func findPage(t *testing.T, path string, ps int, typ byte, rel int) uint32 {
	return findPageAfter(t, path, ps, typ, rel, 0)
}

func findPageAfter(t *testing.T, path string, ps int, typ byte, rel int, after uint32) uint32 {
	t.Helper()
	b, _ := os.ReadFile(path)
	for p := after + 1; int(p+1)*ps <= len(b); p++ {
		if pageMatches(b[int(p)*ps:], typ, rel) {
			return p
		}
	}
	t.Fatalf("no page of type %d (relation %d) after %d", typ, rel, after)
	return 0
}

func findLastPage(t *testing.T, path string, ps int, typ byte, rel int) uint32 {
	t.Helper()
	b, _ := os.ReadFile(path)
	for p := len(b)/ps - 1; p > 0; p-- {
		if pageMatches(b[p*ps:], typ, rel) {
			return uint32(p)
		}
	}
	t.Fatalf("no page of type %d (relation %d)", typ, rel)
	return 0
}

func pageMatches(page []byte, typ byte, rel int) bool {
	if page[0] != typ {
		return false
	}
	switch {
	case rel < 0:
		return true
	case typ == fdb.TypeData:
		return int(binary.LittleEndian.Uint16(page[0x14:])) == rel
	case typ == fdb.TypePointer:
		return int(binary.LittleEndian.Uint16(page[0x1A:])) == rel
	}
	return true
}

func readAt(t *testing.T, path string, off int64, n int) []byte {
	t.Helper()
	b, _ := os.ReadFile(path)
	return b[off : off+int64(n)]
}

func writeAt(t *testing.T, path string, off int64, data []byte) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteAt(data, off); err != nil {
		t.Fatal(err)
	}
}

func wantProblem(t *testing.T, r *Report, page uint32, kind string) {
	t.Helper()
	for _, p := range r.Problems {
		if p.Page == page && p.Kind == kind {
			return
		}
	}
	t.Errorf("no %s problem on page %d; problems: %+v", kind, page, r.Problems)
}

func wantKind(t *testing.T, r *Report, kind string) {
	t.Helper()
	for _, p := range r.Problems {
		if p.Kind == kind {
			return
		}
	}
	t.Errorf("no %s problem; problems: %+v", kind, r.Problems)
}
