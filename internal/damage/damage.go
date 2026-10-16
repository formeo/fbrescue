// Package damage corrupts copies of database files in the ways disks,
// file systems and interrupted copies do. Each kind picks its target with
// a seeded random source, so a test run is repeatable.
package damage

import (
	"encoding/binary"
	"math/rand/v2"

	"github.com/formeo/fbrescue/internal/fdb"
)

// Kind is one way of damaging a database.
type Kind struct {
	Name string
	// Apply damages the file contents b (page size ps) and returns them; ok
	// is false when the file has nothing this kind can damage.
	Apply func(b []byte, ps int, r *rand.Rand) (out []byte, ok bool)
}

// Kinds lists the kinds of damage, roughly from the most to the least
// localized.
var Kinds = []Kind{
	{"zeroed data page", zeroPage(func(p []byte) bool { return p[0] == fdb.TypeData && rel(p) >= 128 })},
	{"zeroed index page", zeroPage(func(p []byte) bool { return p[0] == fdb.TypeBTree || p[0] == fdb.TypeIndexRoot })},
	{"zeroed blob page", zeroPage(func(p []byte) bool { return p[0] == fdb.TypeBlob })},
	{"zeroed pointer page", zeroPage(func(p []byte) bool { return p[0] == fdb.TypePointer && rel(p) >= 128 })},
	{"zeroed system table page", zeroPage(func(p []byte) bool {
		return (p[0] == fdb.TypeData || p[0] == fdb.TypePointer) && rel(p) < 128
	})},
	{"zeroed transaction inventory page", zeroPage(func(p []byte) bool { return p[0] == fdb.TypeTIP })},
	{"zeroed page inventory page", zeroPage(func(p []byte) bool { return p[0] == fdb.TypePIP })},
	{"zeroed header page", func(b []byte, ps int, r *rand.Rand) ([]byte, bool) {
		clear(b[:ps])
		return b, true
	}},
	{"page of random bytes", func(b []byte, ps int, r *rand.Rand) ([]byte, bool) {
		p, ok := pick(b, ps, r, func(p []byte) bool { return p[0] != 0 })
		if ok {
			fill(b[p*ps:(p+1)*ps], r)
		}
		return b, ok
	}},
	{"16 flipped bits", func(b []byte, ps int, r *rand.Rand) ([]byte, bool) {
		for range 16 {
			p, ok := pick(b, ps, r, func(p []byte) bool { return p[0] != 0 })
			if !ok {
				return b, false
			}
			i := p*ps + r.IntN(ps)
			b[i] ^= 1 << r.IntN(8)
		}
		return b, true
	}},
	{"zeroed 64 KB block", func(b []byte, ps int, r *rand.Rand) ([]byte, bool) {
		const n = 64 << 10
		if len(b) < 2*n {
			return b, false
		}
		at := r.IntN(len(b)/n-1)*n + n // a block-aligned block, not the first
		clear(b[at : at+n])
		return b, true
	}},
	{"two pages swapped", func(b []byte, ps int, r *rand.Rand) ([]byte, bool) {
		used := func(p []byte) bool { return p[0] != 0 && p[0] != fdb.TypeHeader }
		p, ok1 := pick(b, ps, r, used)
		q, ok2 := pick(b, ps, r, used)
		if !ok1 || !ok2 || p == q {
			return b, false
		}
		tmp := append([]byte(nil), b[p*ps:(p+1)*ps]...)
		copy(b[p*ps:], b[q*ps:(q+1)*ps])
		copy(b[q*ps:], tmp)
		return b, true
	}},
	{"truncated file", func(b []byte, ps int, r *rand.Rand) ([]byte, bool) {
		// Cut somewhere in the last half, not on a page boundary.
		n := len(b)/2 + r.IntN(len(b)/2)
		if n%ps == 0 {
			n -= ps / 2
		}
		return b[:n], true
	}},
}

func zeroPage(match func(page []byte) bool) func([]byte, int, *rand.Rand) ([]byte, bool) {
	return func(b []byte, ps int, r *rand.Rand) ([]byte, bool) {
		p, ok := pick(b, ps, r, match)
		if ok {
			clear(b[p*ps : (p+1)*ps])
		}
		return b, ok
	}
}

// pick returns a random page other than the header that matches.
func pick(b []byte, ps int, r *rand.Rand, match func(page []byte) bool) (int, bool) {
	var pages []int
	for p := 1; (p+1)*ps <= len(b); p++ {
		if match(b[p*ps : (p+1)*ps]) {
			pages = append(pages, p)
		}
	}
	if len(pages) == 0 {
		return 0, false
	}
	return pages[r.IntN(len(pages))], true
}

// rel is the relation of a data or pointer page.
func rel(p []byte) uint16 {
	if p[0] == fdb.TypePointer {
		return binary.LittleEndian.Uint16(p[0x1A:])
	}
	return binary.LittleEndian.Uint16(p[0x14:])
}

func fill(b []byte, r *rand.Rand) {
	for i := range b {
		b[i] = byte(r.Uint32())
	}
}
