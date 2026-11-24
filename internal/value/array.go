package value

import (
	"fmt"
	"strings"
)

// ArrayRef is the 8-byte array id stored in a record field. Arrays use the
// same record and page storage as stream blobs.
type ArrayRef struct {
	Relation uint16
	Number   uint64
}

// ArrayDimension is one inclusive range of array subscripts.
type ArrayDimension struct {
	Lower int32
	Upper int32
}

// ArrayType is the SQL declaration of an array column.
type ArrayType struct {
	Element    Field
	Dimensions []ArrayDimension
}

// SQLType returns the element type followed by its dimensions.
func (a ArrayType) SQLType() string {
	element, charset, hasCharset := strings.Cut(a.Element.SQLType(), " CHARACTER SET ")
	s := element + "["
	for i, d := range a.Dimensions {
		if i > 0 {
			s += ","
		}
		if d.Lower == 1 {
			s += fmt.Sprintf("%d", d.Upper)
		} else {
			s += fmt.Sprintf("%d:%d", d.Lower, d.Upper)
		}
	}
	s += "]"
	if hasCharset {
		s += " CHARACTER SET " + charset
	}
	return s
}

// Array is a decoded array. Values are in storage order: the last dimension
// changes fastest. Nested returns the corresponding nested slices.
type Array struct {
	Dimensions []ArrayDimension
	Values     []any
}

// InternalArrayDesc: 16-byte header, then 24 bytes per dimension. Only the
// first dimension's 12-byte Descriptor is reliable; later ones may contain
// uninitialized bytes (verified on Firebird 3).
const (
	arrayHeaderSize    = 16
	arrayDimensionSize = 24
)

// ParseArray decodes a stream blob holding an InternalArrayDesc and its
// elements. Firebird stores only the written prefix of a partial array; the
// engine reads the unwritten tail as zero values, and so does this parser.
func ParseArray(b []byte, opt Options) (Array, error) {
	var a Array
	if len(b) < arrayHeaderSize+arrayDimensionSize {
		return a, fmt.Errorf("array descriptor is truncated: %d bytes", len(b))
	}
	if b[0] != 1 {
		return a, fmt.Errorf("array descriptor version %d is not supported", b[0])
	}
	dims := int(b[1])
	if dims < 1 || dims > 16 {
		return a, fmt.Errorf("array descriptor has %d dimensions", dims)
	}
	descLen := int(le.Uint16(b[6:]))
	if descLen != arrayHeaderSize+dims*arrayDimensionSize || descLen > len(b) {
		return a, fmt.Errorf("array descriptor length %d does not match %d dimensions", descLen, dims)
	}
	elementLen := int(le.Uint16(b[4:]))
	if elementLen == 0 {
		return a, fmt.Errorf("array element length is zero")
	}
	f := ParseField(0, b[arrayHeaderSize:])
	if int(f.Length) != elementLen || !f.Supported() || f.DType == TypeArray || f.DType == TypeBlob {
		return a, fmt.Errorf("array element descriptor has type %d and length %d, header length %d", f.DType, f.Length, elementLen)
	}

	count := int64(1)
	a.Dimensions = make([]ArrayDimension, dims)
	for i := 0; i < dims; i++ {
		o := arrayHeaderSize + i*arrayDimensionSize
		d := ArrayDimension{Lower: int32(le.Uint32(b[o+16:])), Upper: int32(le.Uint32(b[o+20:]))}
		n := int64(d.Upper) - int64(d.Lower) + 1
		if n <= 0 || count > int64(^uint32(0))/n {
			return a, fmt.Errorf("array dimension %d has invalid bounds %d:%d", i, d.Lower, d.Upper)
		}
		count *= n
		a.Dimensions[i] = d
	}
	if int64(le.Uint32(b[8:])) != count {
		return a, fmt.Errorf("array descriptor count %d does not match dimensions (%d)", le.Uint32(b[8:]), count)
	}
	total := count * int64(elementLen)
	if total > int64(^uint32(0)) || int64(le.Uint32(b[12:])) != total {
		return a, fmt.Errorf("array descriptor data length %d does not match %d elements of %d bytes", le.Uint32(b[12:]), count, elementLen)
	}
	data := b[descLen:]
	if int64(len(data)) > total {
		return a, fmt.Errorf("array has %d data bytes, descriptor says %d", len(data), total)
	}
	if len(data) < int(total) {
		data = append(append([]byte(nil), data...), make([]byte, int(total)-len(data))...)
	}
	a.Values = make([]any, count)
	for i := range a.Values {
		v, err := DecodeBytes(data[i*elementLen:(i+1)*elementLen], f, opt)
		if err != nil {
			return Array{}, fmt.Errorf("array element %d: %w", i, err)
		}
		a.Values[i] = v
	}
	return a, nil
}

// Nested returns values grouped by dimensions, from the outermost to the
// innermost. Array values and their element types are preserved.
func (a Array) Nested() []any {
	at := 0
	var build func(int) []any
	build = func(dim int) []any {
		d := a.Dimensions[dim]
		out := make([]any, int(d.Upper)-int(d.Lower)+1)
		for i := range out {
			if dim == len(a.Dimensions)-1 {
				out[i] = a.Values[at]
				at++
			} else {
				out[i] = build(dim + 1)
			}
		}
		return out
	}
	if len(a.Dimensions) == 0 {
		return nil
	}
	return build(0)
}
