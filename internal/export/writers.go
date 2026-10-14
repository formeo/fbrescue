package export

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/formeo/fbrescue/internal/meta"
	"github.com/formeo/fbrescue/internal/value"
)

// text formats a non-NULL value the way CSV and JSON Lines show it.
func text(v any) string {
	switch x := v.(type) {
	case value.Decimal:
		return x.String()
	case float32:
		return strconv.FormatFloat(float64(x), 'g', -1, 32)
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	case value.Date, value.Time, value.Timestamp:
		return x.(fmt.Stringer).String()
	case value.Text:
		return x.S
	case value.Octets:
		return hex.EncodeToString(x)
	case bool:
		return strconv.FormatBool(x)
	}
	return fmt.Sprint(v)
}

// CSV: RFC 4180 with a header row. NULL is an empty unquoted field; an
// empty string is "" so the two stay distinct.
type csvWriter struct{ w *bufio.Writer }

func newCSV(w *bufio.Writer, cols []*meta.Column) (*csvWriter, error) {
	c := &csvWriter{w}
	names := make([]any, len(cols))
	for i, col := range cols {
		names[i] = value.Text{S: col.Name}
	}
	return c, c.row(names)
}

func (c *csvWriter) row(vals []any) error {
	for i, v := range vals {
		if i > 0 {
			c.w.WriteByte(',')
		}
		if v == nil {
			continue
		}
		s := text(v)
		if s == "" || strings.ContainsAny(s, ",\"\r\n") || s[0] == ' ' || s[len(s)-1] == ' ' {
			s = `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
		}
		c.w.WriteString(s)
	}
	_, err := c.w.WriteString("\r\n")
	return err
}

func (c *csvWriter) flush() error { return nil }

// JSON Lines: one object per row, keys in column order. Exact numbers are
// JSON numbers written digit for digit; dates, times and binary data are
// strings.
type jsonlWriter struct {
	w    *bufio.Writer
	keys [][]byte
}

func newJSONL(w *bufio.Writer, cols []*meta.Column) (*jsonlWriter, error) {
	j := &jsonlWriter{w: w}
	for i, c := range cols {
		k, _ := json.Marshal(c.Name)
		if i > 0 {
			k = append([]byte{','}, k...)
		}
		j.keys = append(j.keys, append(k, ':'))
	}
	return j, nil
}

func (j *jsonlWriter) row(vals []any) error {
	j.w.WriteByte('{')
	for i, v := range vals {
		j.w.Write(j.keys[i])
		switch x := v.(type) {
		case nil:
			j.w.WriteString("null")
		case value.Decimal, bool:
			j.w.WriteString(text(x))
		case float32:
			j.number(float64(x), text(x))
		case float64:
			j.number(x, text(x))
		default:
			b, _ := json.Marshal(text(x))
			j.w.Write(b)
		}
	}
	_, err := j.w.WriteString("}\n")
	return err
}

func (j *jsonlWriter) number(f float64, s string) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		b, _ := json.Marshal(s)
		j.w.Write(b)
		return
	}
	j.w.WriteString(s)
}

func (j *jsonlWriter) flush() error { return nil }

// SQL: INSERT statements for isql, a COMMIT every commitEvery rows.
type sqlWriter struct {
	w      *bufio.Writer
	prefix string
	n      int
}

const commitEvery = 1000

func newSQL(w *bufio.Writer, t *meta.Table, cols []*meta.Column) (*sqlWriter, error) {
	names := make([]string, len(cols))
	for i, c := range cols {
		names[i] = quote(c.Name)
	}
	fmt.Fprintf(w, "-- Rows of %s recovered by fbrescue. Load after schema.sql: isql -ch UTF8 -i <this file> <database>\n", t.Name)
	return &sqlWriter{w: w, prefix: "INSERT INTO " + quote(t.Name) + " (" + strings.Join(names, ", ") + ") VALUES ("}, nil
}

func (s *sqlWriter) row(vals []any) error {
	s.w.WriteString(s.prefix)
	for i, v := range vals {
		if i > 0 {
			s.w.WriteString(", ")
		}
		s.w.WriteString(literal(v))
	}
	s.w.WriteString(");\n")
	s.n++
	if s.n%commitEvery == 0 {
		s.w.WriteString("COMMIT;\n")
	}
	return nil
}

func (s *sqlWriter) flush() error {
	_, err := s.w.WriteString("COMMIT;\n")
	return err
}

// literal formats a value as a Firebird SQL literal (dialect 3).
func literal(v any) string {
	switch x := v.(type) {
	case nil:
		return "NULL"
	case value.Decimal:
		if x.Int == math.MinInt64 {
			// The literal -9223372036854775808 overflows before negation.
			return "(" + value.Decimal{Int: x.Int + 1, Scale: x.Scale}.String() + " - " + value.Decimal{Int: 1, Scale: x.Scale}.String() + ")"
		}
		return x.String()
	case float32:
		return strconv.FormatFloat(float64(x), 'E', -1, 32)
	case float64:
		return strconv.FormatFloat(x, 'E', -1, 64)
	case value.Date:
		return "DATE '" + x.String() + "'"
	case value.Time:
		return "TIME '" + x.String() + "'"
	case value.Timestamp:
		return "TIMESTAMP '" + x.String() + "'"
	case value.Text:
		return "'" + strings.ReplaceAll(x.S, "'", "''") + "'"
	case value.Octets:
		return "x'" + strings.ToUpper(hex.EncodeToString(x)) + "'"
	case bool:
		if x {
			return "TRUE"
		}
		return "FALSE"
	}
	return "NULL"
}
