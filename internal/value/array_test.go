package value

import (
	"encoding/hex"
	"reflect"
	"strings"
	"testing"
)

func arrayBytes(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.Join(strings.Fields(s), ""))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseArray(t *testing.T) {
	// Captured from a Firebird 3 INTEGER[4] array. The last two elements of
	// the second copy are not stored and must read as zero, as in the engine.
	full := arrayBytes(t, `
		01 01 01 00 04 00 28 00 04 00 00 00 10 00 00 00
		09 00 04 00 00 00 00 00 00 00 00 00 01 00 00 00
		01 00 00 00 04 00 00 00 0a 00 00 00 ec ff ff ff
		00 00 00 00 ff ff ff 7f`)
	want := []any{Decimal{Int: 10}, Decimal{Int: -20}, Decimal{Int: 0}, Decimal{Int: 2147483647}}
	a, err := ParseArray(full, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.Values, want) || !reflect.DeepEqual(a.Nested(), want) {
		t.Fatalf("values %#v, nested %#v", a.Values, a.Nested())
	}
	if a.Dimensions[0] != (ArrayDimension{Lower: 1, Upper: 4}) {
		t.Fatalf("dimensions: %+v", a.Dimensions)
	}
	a, err = ParseArray(full[:48], Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.Values, []any{Decimal{Int: 10}, Decimal{Int: -20}, Decimal{}, Decimal{}}) {
		t.Fatalf("partial values: %#v", a.Values)
	}
}

func TestParseArrayDimensions(t *testing.T) {
	// INTEGER[0:1,2:4]. The descriptor in the second dimension deliberately
	// contains junk, as Firebird 3 leaves those bytes uninitialized.
	b := arrayBytes(t, `
		01 02 01 00 04 00 40 00 06 00 00 00 18 00 00 00
		09 00 04 00 00 00 00 00 00 00 00 00 03 00 00 00
		00 00 00 00 01 00 00 00 a2 d8 2d 95 e0 7f 00 00
		f8 14 ff 6a 01 00 00 00 02 00 00 00 04 00 00 00
		01 00 00 00 02 00 00 00 03 00 00 00 04 00 00 00
		05 00 00 00 06 00 00 00`)
	a, err := ParseArray(b, Options{})
	if err != nil {
		t.Fatal(err)
	}
	want := []any{[]any{Decimal{Int: 1}, Decimal{Int: 2}, Decimal{Int: 3}},
		[]any{Decimal{Int: 4}, Decimal{Int: 5}, Decimal{Int: 6}}}
	if !reflect.DeepEqual(a.Nested(), want) {
		t.Fatalf("nested: %#v", a.Nested())
	}
	if !reflect.DeepEqual(a.Dimensions, []ArrayDimension{{Lower: 0, Upper: 1}, {Lower: 2, Upper: 4}}) {
		t.Fatalf("dimensions: %+v", a.Dimensions)
	}
}

func TestParseArrayDamage(t *testing.T) {
	valid := arrayBytes(t, `
		01 01 01 00 04 00 28 00 01 00 00 00 04 00 00 00
		09 00 04 00 00 00 00 00 00 00 00 00 01 00 00 00
		01 00 00 00 01 00 00 00 2a 00 00 00`)
	for name, mutate := range map[string]func([]byte) []byte{
		"truncated": func(b []byte) []byte { return b[:20] },
		"version":   func(b []byte) []byte { b[0] = 2; return b },
		"count":     func(b []byte) []byte { b[8] = 2; return b },
		"data":      func(b []byte) []byte { return append(b, 0) },
	} {
		t.Run(name, func(t *testing.T) {
			b := append([]byte(nil), valid...)
			if _, err := ParseArray(mutate(b), Options{}); err == nil {
				t.Fatal("damaged array was accepted")
			}
		})
	}
}
