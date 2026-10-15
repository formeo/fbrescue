package value

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // region time zones without depending on the host
)

// BigDecimal is an INT128 or NUMERIC/DECIMAL(19..38): Int * 10^Scale.
type BigDecimal struct {
	Int   *big.Int
	Scale int8
}

func decodeInt128(b []byte, scale int8) BigDecimal {
	// Little-endian two's complement.
	be := make([]byte, 16)
	for i := range be {
		be[i] = b[15-i]
	}
	v := new(big.Int).SetBytes(be)
	if b[15]&0x80 != 0 {
		v.Sub(v, new(big.Int).Lsh(big.NewInt(1), 128))
	}
	return BigDecimal{v, scale}
}

// String formats with exactly -Scale fraction digits.
func (d BigDecimal) String() string {
	s := d.Int.String()
	if d.Scale >= 0 {
		return s + strings.Repeat("0", int(d.Scale))
	}
	return insertPoint(s, int(-d.Scale))
}

// IsMin reports the most negative INT128, whose literal does not parse.
func (d BigDecimal) IsMin() bool {
	return d.Int.Cmp(new(big.Int).Neg(new(big.Int).Lsh(big.NewInt(1), 127))) == 0
}

func insertPoint(s string, k int) string {
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	if len(s) <= k {
		s = strings.Repeat("0", k-len(s)+1) + s
	}
	s = s[:len(s)-k] + "." + s[len(s)-k:]
	if neg {
		s = "-" + s
	}
	return s
}

// DecFloat is a DECFLOAT value, formatted as decNumber's to-scientific-string
// does (and as Firebird shows it).
type DecFloat string

func (d DecFloat) String() string { return string(d) }

// decodeDecFloat decodes IEEE 754 decimal64 (8 bytes) or decimal128 (16
// bytes) with densely packed decimal coefficients, stored little-endian.
func decodeDecFloat(b []byte) DecFloat {
	n := len(b) * 8
	bit := func(i int) uint { // i counts from the most significant bit
		k := n - 1 - i
		return uint(b[k/8]>>(k%8)) & 1
	}
	bits := func(from, count int) uint {
		var v uint
		for i := 0; i < count; i++ {
			v = v<<1 | bit(from+i)
		}
		return v
	}
	expCont, bias, declets := 8, 398, 5
	if len(b) == 16 {
		expCont, bias, declets = 12, 6176, 11
	}
	sign := ""
	if bit(0) == 1 {
		sign = "-"
	}
	g := bits(1, 5)
	var expHi, lead uint
	switch {
	case g>>1 == 0xF:
		if g&1 == 0 {
			return DecFloat(sign + "Infinity")
		}
		if bit(6) == 1 {
			return DecFloat(sign + "sNaN")
		}
		return DecFloat(sign + "NaN")
	case g>>3 == 3:
		expHi, lead = (g>>1)&3, 8+g&1
	default:
		expHi, lead = g>>3, g&7
	}
	exp := int(expHi<<uint(expCont)|bits(6, expCont)) - bias
	var digits strings.Builder
	digits.WriteByte(byte('0' + lead))
	for d := 0; d < declets; d++ {
		digits.WriteString(dpd(bits(6+expCont+10*d, 10)))
	}
	coef := strings.TrimLeft(digits.String(), "0")
	if coef == "" {
		coef = "0"
	}
	return DecFloat(sign + sciString(coef, exp))
}

// dpd decodes one densely packed decimal declet (10 bits) into 3 digits.
func dpd(v uint) string {
	b := func(i uint) uint { return v >> i & 1 }
	var d2, d1, d0 uint
	switch {
	case b(3) == 0:
		d2, d1, d0 = v>>7&7, v>>4&7, v&7
	case b(2) == 0 && b(1) == 0:
		d2, d1, d0 = v>>7&7, v>>4&7, 8+b(0)
	case b(2) == 0 && b(1) == 1:
		d2, d1, d0 = v>>7&7, 8+b(4), b(6)<<2|b(5)<<1|b(0)
	case b(2) == 1 && b(1) == 0:
		d2, d1, d0 = 8+b(7), v>>4&7, b(9)<<2|b(8)<<1|b(0)
	case b(6) == 0 && b(5) == 0:
		d2, d1, d0 = 8+b(7), 8+b(4), b(9)<<2|b(8)<<1|b(0)
	case b(6) == 0 && b(5) == 1:
		d2, d1, d0 = 8+b(7), b(9)<<2|b(8)<<1|b(4), 8+b(0)
	case b(6) == 1 && b(5) == 0:
		d2, d1, d0 = v>>7&7, 8+b(4), 8+b(0)
	default:
		d2, d1, d0 = 8+b(7), 8+b(4), 8+b(0)
	}
	return string([]byte{byte('0' + d2), byte('0' + d1), byte('0' + d0)})
}

// sciString is the to-scientific-string conversion of the decimal
// arithmetic specification: plain notation when the exponent is not
// positive and the adjusted exponent is at least -6, exponent otherwise.
func sciString(coef string, exp int) string {
	adj := exp + len(coef) - 1
	if exp <= 0 && adj >= -6 {
		if exp == 0 {
			return coef
		}
		return insertPoint(coef, -exp)
	}
	s := coef[:1]
	if len(coef) > 1 {
		s += "." + coef[1:]
	}
	e := strconv.Itoa(adj)
	if adj >= 0 {
		e = "+" + e
	}
	return s + "E" + e
}

// TimeTZ is a TIME WITH TIME ZONE: UTC time of day and a zone.
type TimeTZ struct {
	UTC  Time
	Zone uint16
}

// TimestampTZ is a TIMESTAMP WITH TIME ZONE: UTC timestamp and a zone.
type TimestampTZ struct {
	UTC  Timestamp
	Zone uint16
}

// Zone ids up to 2878 are offsets: minutes + 1439. Larger ones count down
// from 65535 through the region names of RDB$TIME_ZONES.
const maxOffsetZone = 2 * 1439

func zoneName(z uint16) (name string, offset int, region bool, err error) {
	if z <= maxOffsetZone {
		m := int(z) - 1439
		sign := '+'
		if m < 0 {
			sign, m = '-', -m
		}
		return fmt.Sprintf("%c%02d:%02d", sign, m/60, m%60), int(z) - 1439, false, nil
	}
	name, ok := timeZones[z]
	if !ok {
		return "", 0, false, fmt.Errorf("unknown time zone id %d", z)
	}
	return name, 0, true, nil
}

// local converts a UTC instant to the zone's local time.
func local(utc time.Time, z uint16) (time.Time, string, error) {
	name, off, region, err := zoneName(z)
	if err != nil {
		return time.Time{}, "", err
	}
	if !region {
		return utc.Add(time.Duration(off) * time.Minute), name, nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("time zone %s: %v", name, err)
	}
	return utc.In(loc), name, nil
}

func ticksTime(d Date, t Time) time.Time {
	return epoch.AddDate(0, 0, int(d)).Add(time.Duration(t) * 100 * time.Microsecond)
}

func formatTicks(t time.Time) string {
	return fmt.Sprintf("%02d:%02d:%02d.%04d", t.Hour(), t.Minute(), t.Second(), t.Nanosecond()/100000)
}

// timeRefDate is the date Firebird uses to convert TIME WITH TIME ZONE
// between UTC and a region zone, whose offset depends on the date.
var timeRefDate = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

// Format shows the local time and the zone, as SELECT does.
func (t TimeTZ) Format() (string, error) {
	l, name, err := local(timeRefDate.Add(time.Duration(t.UTC)*100*time.Microsecond), t.Zone)
	if err != nil {
		return "", err
	}
	return formatTicks(l) + " " + name, nil
}

// Format shows the local timestamp and the zone, as SELECT does.
func (t TimestampTZ) Format() (string, error) {
	l, name, err := local(ticksTime(t.UTC.Date, t.UTC.Time), t.Zone)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%04d-%02d-%02d %s %s", l.Year(), l.Month(), l.Day(), formatTicks(l), name), nil
}

// BlobRef is a blob id field; the blob is read separately.
type BlobRef struct {
	Relation uint16
	Number   uint64 // record number of the blob header
	SubType  int16  // 1 text, 0 binary, others user-defined
	Charset  int    // text blobs
}
