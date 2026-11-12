package record

import (
	"testing"

	"github.com/formeo/fbrescue/internal/corpus"
)

const (
	maxFuzzPage   = 1024
	maxFuzzPacked = 256
)

func FuzzDecode(f *testing.F) {
	f.Add([]byte{3, 'a', 'b', 'c'})
	f.Add([]byte{0xfd, 'x'})
	f.Add([]byte{0xff, 0x2c, 0x01, ' '})

	page := make([]byte, maxFuzzPage)
	le.PutUint16(page[0x16:], 1)
	le.PutUint16(page[0x18:], dataPageHeader+4)
	le.PutUint16(page[0x1a:], headerSize+1)
	f.Add(page)
	for _, seed := range corpus.FuzzPages(f, 6) {
		f.Add(seed[:min(len(seed), maxFuzzPage)])
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > maxFuzzPage {
			return
		}
		page := make([]byte, maxFuzzPage)
		copy(page, data)

		count := int(le.Uint16(page[0x16:]))
		if dataPageHeader+4*count <= len(page) {
			for i := 0; i < count; i++ {
				frag, err := slot(page, i)
				if err != nil {
					break
				}
				if frag != nil {
					_ = parseHeader(frag)
				}
			}
		}
		_ = ParseBlobID(page[:8])

		// Bound the worst-case expansion of an ODS 13.1 long run.
		packed := data[:min(len(data), maxFuzzPacked)]
		_, _ = Unpack(packed, false)
		_, _ = Unpack(packed, true)
		_, _ = ApplyDelta(packed, packed)
	})
}
