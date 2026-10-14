// Package value decodes field values from unpacked records, using the field
// descriptors stored in RDB$FORMATS, and formats them for output.
package value

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var le = binary.LittleEndian

// Physical data types (dsc_dtype).
const (
	TypeText        = 1
	TypeVarying     = 3
	TypeShort       = 8
	TypeLong        = 9
	TypeQuad        = 10
	TypeReal        = 11
	TypeDouble      = 12
	TypeDate        = 14
	TypeTime        = 15
	TypeTimestamp   = 16
	TypeBlob        = 17
	TypeArray       = 18
	TypeInt64       = 19
	TypeBoolean     = 21
	TypeDec64       = 22
	TypeDec128      = 23
	TypeInt128      = 24
	TypeTimeTZ      = 25
	TypeTimestampTZ = 26
)

// Field is one entry of a record format (Ods::Descriptor): 12 bytes on disk.
type Field struct {
	ID      int // position in the format; also the bit in the null bitmap
	DType   uint8
	Scale   int8
	Length  uint16
	SubType int16 // text: character set (low byte) and collation; numbers: 1 NUMERIC, 2 DECIMAL
	Flags   uint16
	Offset  uint32 // 0: not stored (computed field)
}

// DescriptorSize is the on-disk size of a Field.
const DescriptorSize = 12

// ParseField decodes a 12-byte descriptor.
func ParseField(id int, b []byte) Field {
	return Field{
		ID: id, DType: b[0], Scale: int8(b[1]), Length: le.Uint16(b[2:]),
		SubType: int16(le.Uint16(b[4:])), Flags: le.Uint16(b[6:]), Offset: le.Uint32(b[8:]),
	}
}

// Stored reports whether the field has bytes in the record: dropped columns
// have type 0 and computed ones offset 0.
func (f Field) Stored() bool { return f.DType != 0 && f.Offset != 0 }

// Supported reports whether values of this type can be decoded.
func (f Field) Supported() bool {
	switch f.DType {
	case TypeText, TypeVarying, TypeShort, TypeLong, TypeInt64, TypeReal, TypeDouble,
		TypeDate, TypeTime, TypeTimestamp, TypeBoolean:
		return true
	}
	return false
}

// Charset is the character set id of a text field.
func (f Field) Charset() int { return int(uint16(f.SubType) & 0xFF) }

// CharLength is the declared length in characters of a text field.
func (f Field) CharLength() int {
	n := int(f.Length)
	if f.DType == TypeVarying {
		n -= 2
	}
	return n / max(charsetInfo(f.Charset()).bytesPerChar, 1)
}

// Decimal is an exact number: Int * 10^Scale, Scale <= 0.
type Decimal struct {
	Int   int64
	Scale int8
}

// Date is days since 1858-11-17.
type Date int32

// Time is 1/10000 seconds since midnight.
type Time uint32

// Timestamp is a date and a time.
type Timestamp struct {
	Date Date
	Time Time
}

// Text is a decoded string. Raw is set when the bytes could not be decoded
// to UTF-8 (unknown character set, or NONE with non-UTF-8 bytes); the
// string then holds the original bytes.
type Text struct {
	S   string
	Raw bool
}

// Octets is binary data from a CHARACTER SET OCTETS field.
type Octets []byte

// Options controls decoding.
type Options struct {
	// NoneCharset decodes CHARACTER SET NONE text with this character set
	// id instead of passing the bytes through. 0 means pass through.
	NoneCharset int
}

// IsNull reports whether field f is NULL in record rec.
func IsNull(rec []byte, f Field) bool {
	i := f.ID / 8
	return i >= len(rec) || rec[i]>>(f.ID%8)&1 != 0
}

// Decode returns the value of field f in record rec: nil for NULL, or one
// of Decimal, float32, float64, Date, Time, Timestamp, Text, Octets, bool.
func Decode(rec []byte, f Field, opt Options) (any, error) {
	if IsNull(rec, f) {
		return nil, nil
	}
	end := int(f.Offset) + int(f.Length)
	if end > len(rec) {
		return nil, fmt.Errorf("field %d ends at byte %d, the record has %d", f.ID, end, len(rec))
	}
	return DecodeBytes(rec[f.Offset:end], f, opt)
}

// DecodeBytes decodes a value stored in b (exactly f.Length bytes).
func DecodeBytes(b []byte, f Field, opt Options) (any, error) {
	switch f.DType {
	case TypeShort:
		return Decimal{int64(int16(le.Uint16(b))), f.Scale}, nil
	case TypeLong:
		return Decimal{int64(int32(le.Uint32(b))), f.Scale}, nil
	case TypeInt64:
		return Decimal{int64(le.Uint64(b)), f.Scale}, nil
	case TypeReal:
		return math.Float32frombits(le.Uint32(b)), nil
	case TypeDouble:
		return math.Float64frombits(le.Uint64(b)), nil
	case TypeDate:
		return Date(int32(le.Uint32(b))), nil
	case TypeTime:
		return Time(le.Uint32(b)), nil
	case TypeTimestamp:
		return Timestamp{Date(int32(le.Uint32(b))), Time(le.Uint32(b[4:]))}, nil
	case TypeBoolean:
		return b[0] != 0, nil
	case TypeText:
		if f.Charset() == 1 {
			return Octets(append([]byte(nil), b...)), nil
		}
		t := decodeText(b, f.Charset(), opt)
		// Multi-byte CHAR fields are padded with spaces to their length in
		// bytes; the value has the declared length in characters.
		if charsetInfo(f.Charset()).bytesPerChar > 1 && !t.Raw {
			s := strings.TrimRight(t.S, " ")
			if n := f.CharLength() - utf8.RuneCountInString(s); n > 0 {
				s += strings.Repeat(" ", n)
			}
			t.S = s
		}
		return t, nil
	case TypeVarying:
		n := int(le.Uint16(b))
		if 2+n > len(b) {
			return nil, fmt.Errorf("varchar length %d exceeds the field (%d bytes)", n, len(b)-2)
		}
		if f.Charset() == 1 {
			return Octets(append([]byte(nil), b[2:2+n]...)), nil
		}
		return decodeText(b[2:2+n], f.Charset(), opt), nil
	}
	return nil, fmt.Errorf("unsupported data type %d", f.DType)
}

func decodeText(b []byte, cs int, opt Options) Text {
	if cs == 0 && opt.NoneCharset != 0 {
		cs = opt.NoneCharset
	}
	c := charsetInfo(cs)
	switch {
	case c.high != nil:
		var sb strings.Builder
		sb.Grow(len(b))
		for _, x := range b {
			if x < 0x80 {
				sb.WriteByte(x)
			} else {
				sb.WriteRune(c.high[x-0x80])
			}
		}
		return Text{S: sb.String()}
	case utf8.Valid(b):
		// UTF8, UNICODE_FSS, ASCII and NONE holding UTF-8 or ASCII bytes.
		return Text{S: string(b)}
	}
	return Text{S: string(b), Raw: true}
}

type charset struct {
	name         string
	bytesPerChar int
	high         *[128]rune // single-byte character sets: code points of 0x80-0xFF
}

func charsetInfo(id int) charset {
	if c, ok := charsets[id]; ok {
		return c
	}
	return charset{name: fmt.Sprintf("CHARSET_%d", id), bytesPerChar: 1}
}

// CharsetName returns the Firebird name of a character set id.
func CharsetName(id int) string { return charsetInfo(id).name }

// CharsetByName finds a character set id by its Firebird name, ignoring case.
func CharsetByName(name string) (int, bool) {
	for id, c := range charsets {
		if strings.EqualFold(c.name, name) {
			return id, true
		}
	}
	return 0, false
}

// Decodable reports whether text in this character set is converted to
// UTF-8 (true) or may be passed through as raw bytes (false).
func Decodable(id int) bool {
	c := charsetInfo(id)
	return c.high != nil || id == 2 || id == 3 || id == 4
}

// String formats a Decimal with exactly -Scale fraction digits.
func (d Decimal) String() string {
	s := strconv.FormatInt(d.Int, 10)
	if d.Scale >= 0 {
		return s + strings.Repeat("0", int(d.Scale))
	}
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	k := int(-d.Scale)
	if len(s) <= k {
		s = strings.Repeat("0", k-len(s)+1) + s
	}
	s = s[:len(s)-k] + "." + s[len(s)-k:]
	if neg {
		s = "-" + s
	}
	return s
}

var epoch = time.Date(1858, 11, 17, 0, 0, 0, 0, time.UTC)

// String formats a date as YYYY-MM-DD.
func (d Date) String() string { return epoch.AddDate(0, 0, int(d)).Format("2006-01-02") }

// String formats a time as HH:MM:SS.FFFF.
func (t Time) String() string {
	frac := uint32(t) % 10000
	s := uint32(t) / 10000
	return fmt.Sprintf("%02d:%02d:%02d.%04d", s/3600, s/60%60, s%60, frac)
}

// String formats a timestamp as YYYY-MM-DD HH:MM:SS.FFFF.
func (t Timestamp) String() string { return t.Date.String() + " " + t.Time.String() }
