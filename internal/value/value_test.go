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

func TestCyrillicAndNextCharsets(t *testing.T) {
	for _, tc := range []struct {
		name string
		id   int
		high string
	}{
		{"CYRL", 50, "\u0402\u0403\u201A\u0453\u201E\u2026\u2020\u2021\u20AC\u2030\u0409\u2039\u040A\u040C\u040B\u040F\u0452\u2018\u2019\u201C\u201D\u2022\u2013\u2014\uFFFD\u2122\u0459\u203A\u045A\u045C\u045B\u045F\u00A0\u040E\u045E\u0408\u00A4\u0490\u00A6\u00A7\u0401\u00A9\u0404\u00AB\u00AC\u00AD\u00AE\u0407\u00B0\u00B1\u0406\u0456\u0491\u00B5\u00B6\u00B7\u0451\u2116\u0454\u00BB\u0458\u0405\u0455\u0457\u0410\u0411\u0412\u0413\u0414\u0415\u0416\u0417\u0418\u0419\u041A\u041B\u041C\u041D\u041E\u041F\u0420\u0421\u0422\u0423\u0424\u0425\u0426\u0427\u0428\u0429\u042A\u042B\u042C\u042D\u042E\u042F\u0430\u0431\u0432\u0433\u0434\u0435\u0436\u0437\u0438\u0439\u043A\u043B\u043C\u043D\u043E\u043F\u0440\u0441\u0442\u0443\u0444\u0445\u0446\u0447\u0448\u0449\u044A\u044B\u044C\u044D\u044E\u044F"},
		{"NEXT", 19, "\u00A0\u00C0\u00C1\u00C2\u00C3\u00C4\u00C5\u00C7\u00C8\u00C9\u00CA\u00CB\u00CC\u00CD\u00CE\u00CF\u00D0\u00D1\u00D2\u00D3\u00D4\u00D5\u00D6\u00D9\u00DA\u00DB\u00DC\u00DD\u00DE\u00B5\u00D7\u00F7\u00A9\u00A1\u00A2\u00A3\u2044\u00A5\u0192\u00A7\u00A4\u2019\u201C\u00AB\u2039\u203A\uFB01\uFB02\u00AE\u2013\u2020\u2021\u00B7\u00A6\u00B6\u2022\u201A\u201E\u201D\u00BB\u2026\u2030\u00AC\u00BF\u00B9\u02CB\u00B4\u02C6\u02DC\u00AF\u02D8\u02D9\u00A8\u00B2\u02DA\u00B8\u00B3\u02DD\u02DB\u02C7\u2014\u00B1\u00BC\u00BD\u00BE\u00E0\u00E1\u00E2\u00E3\u00E4\u00E5\u00E7\u00E8\u00E9\u00EA\u00EB\u00EC\u00C6\u00ED\u00AA\u00EE\u00EF\u00F0\u00F1\u0141\u00D8\u0152\u00BA\u00F2\u00F3\u00F4\u00F5\u00F6\u00E6\u00F9\u00FA\u00FB\u0131\u00FC\u00FD\u0142\u00F8\u0153\u00DF\u00FE\u00FF\uFFFD\uFFFD"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			high := []rune(tc.high)
			if len(high) != 128 {
				t.Fatalf("test table has %d entries, want 128", len(high))
			}
			for b := range 256 {
				want := string(rune(b))
				if b >= 0x80 {
					want = string(high[b-0x80])
				}
				got := DecodeText([]byte{byte(b)}, tc.id, Options{})
				if got.Raw || got.S != want {
					t.Errorf("byte %02X: got %q (raw %v), want %q", b, got.S, got.Raw, want)
				}
			}
		})
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
