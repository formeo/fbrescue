package value

import (
	"fmt"
	"math"
	"strconv"
	"testing"
)

func TestDecimalString(t *testing.T) {
	for _, tc := range []struct {
		d    Decimal
		want string
	}{
		{Decimal{0, 0}, "0"},
		{Decimal{0, -2}, "0.00"},
		{Decimal{5, -3}, "0.005"},
		{Decimal{-5, -3}, "-0.005"},
		{Decimal{-999999999, -2}, "-9999999.99"},
		{Decimal{math.MinInt64, 0}, "-9223372036854775808"},
		{Decimal{math.MinInt64, -4}, "-922337203685477.5808"},
	} {
		if got := tc.d.String(); got != tc.want {
			t.Errorf("%+v: got %s, want %s", tc.d, got, tc.want)
		}
	}
}

func TestDateTime(t *testing.T) {
	if got := Date(0).String(); got != "1858-11-17" {
		t.Errorf("epoch: %s", got)
	}
	if got := Date(-678575).String(); got != "0001-01-01" {
		t.Errorf("first day: %s", got)
	}
	if got := Date(2973483).String(); got != "9999-12-31" {
		t.Errorf("last day: %s", got)
	}
	if got := Time(863999999).String(); got != "23:59:59.9999" {
		t.Errorf("last tick: %s", got)
	}
	if got := (Timestamp{Date(0), Time(1)}).String(); got != "1858-11-17 00:00:00.0001" {
		t.Errorf("timestamp: %s", got)
	}
}

func TestText(t *testing.T) {
	// WIN1251 "Привет" in a VARCHAR(10) CHARACTER SET WIN1251.
	f := Field{DType: TypeVarying, Length: 12, SubType: 52}
	v, err := DecodeBytes([]byte{6, 0, 0xCF, 0xF0, 0xE8, 0xE2, 0xE5, 0xF2, 0, 0, 0, 0}, f, Options{})
	if err != nil || v.(Text).S != "Привет" {
		t.Errorf("win1251: %v, %v", v, err)
	}
	// The same bytes in CHARACTER SET NONE: raw unless a charset is given.
	f.SubType = 0
	v, _ = DecodeBytes([]byte{6, 0, 0xCF, 0xF0, 0xE8, 0xE2, 0xE5, 0xF2, 0, 0, 0, 0}, f, Options{})
	if !v.(Text).Raw {
		t.Errorf("NONE without --charset should be raw: %+v", v)
	}
	v, _ = DecodeBytes([]byte{6, 0, 0xCF, 0xF0, 0xE8, 0xE2, 0xE5, 0xF2, 0, 0, 0, 0}, f, Options{NoneCharset: 52})
	if v.(Text).S != "Привет" {
		t.Errorf("NONE as WIN1251: %+v", v)
	}
	// CHAR(3) CHARACTER SET UTF8: 12 bytes, padded to 3 characters.
	c := Field{DType: TypeText, Length: 12, SubType: 4}
	v, _ = DecodeBytes([]byte("Жa          "), c, Options{})
	if v.(Text).S != "Жa " {
		t.Errorf("utf8 char padding: %q", v.(Text).S)
	}
}

func TestConvert(t *testing.T) {
	v, err := Convert(Decimal{1250, -2}, Field{DType: TypeInt64, Scale: -4})
	if err != nil || v.(Decimal) != (Decimal{125000, -4}) {
		t.Errorf("rescale up: %v, %v", v, err)
	}
	v, err = Convert(Decimal{125, -1}, Field{DType: TypeVarying, Length: 22, SubType: 0})
	if err != nil || v.(Text).S != "12.5" {
		t.Errorf("number to varchar: %v, %v", v, err)
	}
	v, err = Convert(Text{S: "zz"}, Field{DType: TypeText, Length: 4})
	if err != nil || v.(Text).S != "zz  " {
		t.Errorf("text to char: %q, %v", v, err)
	}
	if _, err := Convert(Text{S: "x"}, Field{DType: TypeDate}); err == nil {
		t.Error("text to date should fail")
	}
}

func TestFirebird4Types(t *testing.T) {
	// Raw bytes from the fb4types corpus database, as Firebird 4 and 5 store them.
	for _, tc := range []struct {
		hex  string
		f    Field
		want string
	}{
		{"ffffffffffffffffffffffffffffff7f", Field{DType: TypeInt128}, "170141183460469231731687303715884105727"},
		{"00000000000000000000000000000080", Field{DType: TypeInt128}, "-170141183460469231731687303715884105728"},
		{"ffffffffffffffffffffffffffffffff", Field{DType: TypeInt128, Scale: -4}, "-0.0001"},
		{"568ee2c1b934ad46", Field{DType: TypeDec64}, "1.234567890123456E+300"},
		{"010000000000fca1", Field{DType: TypeDec64}, "-1E-15"},
		{"0000000000003822", Field{DType: TypeDec64}, "0"},
		{"1500000000003422", Field{DType: TypeDec64}, "1.5"},
		{"fffcf3cf3ffffcf3cf3ffffcf3cffff7", Field{DType: TypeDec128}, "-9.999999999999999999999999999999999E+6144"},
		{"01000000000000000000000000400800", Field{DType: TypeDec128}, "1E-6143"},
		{"3435827771123c6fe5281e9c4b930726", Field{DType: TypeDec128}, "12345678901234567890123456789012.34"},
	} {
		b, _ := hexDecode(tc.hex)
		v, err := DecodeBytes(b, tc.f, Options{})
		if err != nil || v.(fmt.Stringer).String() != tc.want {
			t.Errorf("%s: got %v, %v; want %s", tc.hex, v, err, tc.want)
		}
	}
	for _, tc := range []struct {
		hex  string
		f    Field
		want string
	}{
		{"d2ca8f1428feffff", Field{DType: TypeTimeTZ}, "12:34:56.7890 Europe/Moscow"},
		{"805ab227e9060000", Field{DType: TypeTimeTZ}, "00:00:00.0000 +05:30"},
		{"d2eb0000ff94ba0a51ffffff", Field{DType: TypeTimestampTZ}, "2024-02-29 23:59:59.9999 America/New_York"},
		{"2b5f2d00ff297515c9fdffff", Field{DType: TypeTimestampTZ}, "9999-12-31 23:59:59.9999 Pacific/Kiritimati"},
	} {
		b, _ := hexDecode(tc.hex)
		v, err := DecodeBytes(b, tc.f, Options{})
		if err != nil {
			t.Fatal(err)
		}
		s, err := v.(interface{ Format() (string, error) }).Format()
		if err != nil || s != tc.want {
			t.Errorf("%s: got %q, %v; want %q", tc.hex, s, err, tc.want)
		}
	}
}

func hexDecode(s string) ([]byte, error) {
	b := make([]byte, len(s)/2)
	for i := range b {
		v, err := strconv.ParseUint(s[2*i:2*i+2], 16, 8)
		if err != nil {
			return nil, err
		}
		b[i] = byte(v)
	}
	return b, nil
}
