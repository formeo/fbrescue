package value

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

// Convert converts a value read with an older format of a column to the
// column's current type, as the engine does when it reads old records.
// Only conversions that ALTER COLUMN TYPE allows between the types fbrescue
// decodes are supported.
func Convert(v any, to Field) (any, error) {
	if v == nil {
		return nil, nil
	}
	switch to.DType {
	case TypeShort, TypeLong, TypeInt64:
		if d, ok := v.(Decimal); ok {
			return rescale(d, to.Scale)
		}
	case TypeReal:
		switch x := v.(type) {
		case float32:
			return x, nil
		case Decimal:
			return float32(x.Float()), nil
		}
	case TypeDouble:
		switch x := v.(type) {
		case float32:
			return float64(x), nil
		case float64:
			return x, nil
		case Decimal:
			return x.Float(), nil
		}
	case TypeText, TypeVarying:
		var s string
		switch x := v.(type) {
		case Text:
			if to.DType == TypeVarying {
				x.S = strings.TrimRight(x.S, " ")
			}
			s = x.S
			if x.Raw {
				return x, nil
			}
		case Octets:
			return x, nil
		case Decimal, Date, Time, Timestamp:
			s = x.(fmt.Stringer).String()
		default:
			return nil, fmt.Errorf("cannot convert %T to text", v)
		}
		if to.DType == TypeText {
			if n := to.CharLength() - utf8.RuneCountInString(s); n > 0 {
				s += strings.Repeat(" ", n)
			}
		}
		return Text{S: s}, nil
	case TypeDate:
		if x, ok := v.(Date); ok {
			return x, nil
		}
	case TypeTime:
		if x, ok := v.(Time); ok {
			return x, nil
		}
	case TypeTimestamp:
		switch x := v.(type) {
		case Timestamp:
			return x, nil
		case Date:
			return Timestamp{Date: x}, nil
		}
	case TypeBoolean:
		if x, ok := v.(bool); ok {
			return x, nil
		}
	}
	return nil, fmt.Errorf("cannot convert %T to data type %d", v, to.DType)
}

func rescale(d Decimal, scale int8) (Decimal, error) {
	for d.Scale > scale {
		if d.Int > math.MaxInt64/10 || d.Int < math.MinInt64/10 {
			return Decimal{}, fmt.Errorf("value %s overflows scale %d", d, scale)
		}
		d.Int *= 10
		d.Scale--
	}
	for d.Scale < scale {
		// Firebird rounds half away from zero when reducing the scale.
		r := d.Int % 10
		d.Int /= 10
		if r >= 5 {
			d.Int++
		} else if r <= -5 {
			d.Int--
		}
		d.Scale++
	}
	return d, nil
}

// Float returns the value as float64.
func (d Decimal) Float() float64 { return float64(d.Int) * math.Pow10(int(d.Scale)) }

// SQLType returns the column type for CREATE TABLE. Precision of NUMERIC
// and DECIMAL is not stored in the format; the maximum for the storage size
// is used, so every stored value fits.
func (f Field) SQLType() string {
	num := func(prec int, name string) string {
		if f.Scale == 0 && f.SubType == 0 {
			return name
		}
		kind := "NUMERIC"
		if f.SubType == 2 {
			kind = "DECIMAL"
		}
		return fmt.Sprintf("%s(%d,%d)", kind, prec, -int(f.Scale))
	}
	cs := func() string {
		return " CHARACTER SET " + CharsetName(f.Charset())
	}
	switch f.DType {
	case TypeShort:
		return num(4, "SMALLINT")
	case TypeLong:
		return num(9, "INTEGER")
	case TypeInt64:
		return num(18, "BIGINT")
	case TypeReal:
		return "FLOAT"
	case TypeDouble:
		return "DOUBLE PRECISION"
	case TypeDate:
		return "DATE"
	case TypeTime:
		return "TIME"
	case TypeTimestamp:
		return "TIMESTAMP"
	case TypeBoolean:
		return "BOOLEAN"
	case TypeText:
		return fmt.Sprintf("CHAR(%d)%s", f.CharLength(), cs())
	case TypeVarying:
		return fmt.Sprintf("VARCHAR(%d)%s", f.CharLength(), cs())
	case TypeBlob:
		return fmt.Sprintf("BLOB SUB_TYPE %d", f.SubType)
	case TypeInt128:
		return num(38, "INT128")
	case TypeDec64:
		return "DECFLOAT(16)"
	case TypeDec128:
		return "DECFLOAT(34)"
	case TypeTimeTZ:
		return "TIME WITH TIME ZONE"
	case TypeTimestampTZ:
		return "TIMESTAMP WITH TIME ZONE"
	}
	return fmt.Sprintf("UNKNOWN_TYPE_%d", f.DType)
}
