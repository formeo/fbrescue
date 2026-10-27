package export

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/formeo/fbrescue/internal/meta"
	"github.com/formeo/fbrescue/internal/value"
)

// Text formats a value the way CSV shows it ("" for NULL).
func Text(v any) string {
	if v == nil {
		return ""
	}
	return text(v)
}

// text formats a non-NULL value the way CSV and JSON Lines show it.
func text(v any) string {
	switch x := v.(type) {
	case value.Decimal:
		return x.String()
	case value.BigDecimal:
		return x.String()
	case value.DecFloat:
		return string(x)
	case value.TimeTZ:
		s, _ := x.Format() // checked when the row was read
		return s
	case value.TimestampTZ:
		s, _ := x.Format()
		return s
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
// empty string is "" so the two stay distinct. For Excel the file starts
// with a UTF-8 BOM, fields are separated by ';' and numbers have a decimal
// comma: what an Excel with a comma decimal separator (Brazil, Russia, most
// of Europe) opens by double-click.
type csvWriter struct {
	w     *bufio.Writer
	excel bool
	sep   byte
}

func newCSV(w *bufio.Writer, cols []*meta.Column, excel bool) (*csvWriter, error) {
	c := &csvWriter{w: w, excel: excel, sep: ','}
	if excel {
		c.sep = ';'
		w.WriteString("\uFEFF")
	}
	names := make([]any, len(cols))
	for i, col := range cols {
		names[i] = value.Text{S: col.Name}
	}
	return c, c.row(names)
}

func (c *csvWriter) row(vals []any) error {
	for i, v := range vals {
		if i > 0 {
			c.w.WriteByte(c.sep)
		}
		if v == nil {
			continue
		}
		s := text(v)
		if c.excel && isNumber(v) {
			s = strings.Replace(s, ".", ",", 1)
		}
		if s == "" || strings.ContainsAny(s, string(c.sep)+"\"\r\n") || s[0] == ' ' || s[len(s)-1] == ' ' {
			s = `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
		}
		c.w.WriteString(s)
	}
	_, err := c.w.WriteString("\r\n")
	return err
}

func (c *csvWriter) flush() error { return nil }

// isNumber reports whether v is a number with a possible decimal point.
func isNumber(v any) bool {
	switch v.(type) {
	case value.Decimal, value.BigDecimal, value.DecFloat, float32, float64:
		return true
	}
	return false
}

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
		case value.Decimal, value.BigDecimal, bool:
			j.w.WriteString(text(x))
		case value.DecFloat:
			if strings.ContainsAny(string(x), "IN") { // Infinity, NaN
				b, _ := json.Marshal(string(x))
				j.w.Write(b)
			} else {
				j.w.WriteString(string(x))
			}
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
	cols   []*meta.Column
	blobs  blobSink
	n      int
}

const commitEvery = 1000

// A string literal may hold at most 16383 characters in a UTF8 connection
// (Firebird 3 and later), so longer values are joined from pieces.
const literalChunk = 8000

// Blobs larger than this go to files instead of the SQL text: isql of
// Firebird 3+ takes statements up to 10 MB, and joining many pieces copies
// the blob each time.
const blobInlineLimit = 1 << 20

// blobSink stores a blob too large for the SQL file and returns its path.
type blobSink interface {
	write(row int, column string, data []byte, text bool) (string, error)
}

type blobFiles struct {
	dir, rel string
	n        int
}

func (b *blobFiles) write(row int, column string, data []byte, text bool) (string, error) {
	if err := os.MkdirAll(b.dir, 0o755); err != nil {
		return "", err
	}
	ext := "bin"
	if text {
		ext = "txt"
	}
	name := fmt.Sprintf("%d.%s.%s", row, unsafeName.ReplaceAllString(column, "_"), ext)
	b.n++
	return filepath.Join(b.rel, name), os.WriteFile(filepath.Join(b.dir, name), data, 0o644)
}

func newSQL(w *bufio.Writer, t *meta.Table, cols []*meta.Column, blobs blobSink) (*sqlWriter, error) {
	names := make([]string, len(cols))
	for i, c := range cols {
		names[i] = quote(c.Name)
	}
	fmt.Fprintf(w, "-- Rows of %s recovered by fbrescue. Load after schema.sql: isql -ch UTF8 -i <this file> <database>\n", t.Name)
	return &sqlWriter{w: w, prefix: "INSERT INTO " + quote(t.Name) + " (" + strings.Join(names, ", ") + ") VALUES (", cols: cols, blobs: blobs}, nil
}

func (s *sqlWriter) row(vals []any) error {
	s.n++
	var notes []string
	s.w.WriteString(s.prefix)
	for i, v := range vals {
		if i > 0 {
			s.w.WriteString(", ")
		}
		f := s.cols[i].Field
		if f.DType == value.TypeBlob && size(v) > blobInlineLimit {
			data, isText := blobBytes(v)
			path, err := s.blobs.write(s.n, s.cols[i].Name, data, isText)
			if err != nil {
				return err
			}
			s.w.WriteString("NULL")
			notes = append(notes, fmt.Sprintf("%s: %d bytes in %s", s.cols[i].Name, len(data), path))
			continue
		}
		s.w.WriteString(literalFor(v, f))
	}
	s.w.WriteString(");")
	if len(notes) > 0 {
		s.w.WriteString(" -- " + strings.Join(notes, "; "))
	}
	s.w.WriteString("\n")
	if s.n%commitEvery == 0 {
		s.w.WriteString("COMMIT;\n")
	}
	return nil
}

func size(v any) int {
	switch x := v.(type) {
	case value.Text:
		return len(x.S)
	case value.Octets:
		return len(x)
	}
	return 0
}

func blobBytes(v any) ([]byte, bool) {
	if t, ok := v.(value.Text); ok {
		return []byte(t.S), true
	}
	return v.(value.Octets), false
}

func (s *sqlWriter) flush() error {
	_, err := s.w.WriteString("COMMIT;\n")
	return err
}

// literalFor formats a value for a column of type f: long text and binary
// values become joined pieces, starting with a CAST to BLOB for blob
// columns so the join does not hit the 32 KB string limit.
func literalFor(v any, f value.Field) string {
	var pieces []string
	var castTo string
	switch x := v.(type) {
	case value.Text:
		r := []rune(x.S)
		if len(r) <= literalChunk {
			return literal(v)
		}
		for i := 0; i < len(r); i += literalChunk {
			pieces = append(pieces, literal(value.Text{S: string(r[i:min(i+literalChunk, len(r))])}))
		}
		castTo = "BLOB SUB_TYPE TEXT CHARACTER SET UTF8"
	case value.Octets:
		if len(x) <= literalChunk {
			return literal(v)
		}
		for i := 0; i < len(x); i += literalChunk {
			pieces = append(pieces, literal(x[i:min(i+literalChunk, len(x))]))
		}
		castTo = "BLOB SUB_TYPE BINARY"
	case value.DecFloat:
		if f.DType == value.TypeDec64 {
			return "CAST('" + string(x) + "' AS DECFLOAT(16))"
		}
		return "CAST('" + string(x) + "' AS DECFLOAT(34))"
	default:
		return literal(v)
	}
	if f.DType == value.TypeBlob {
		pieces[0] = "CAST(" + pieces[0] + " AS " + castTo + ")"
	}
	return strings.Join(pieces, " || ")
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
	case value.BigDecimal:
		if x.IsMin() {
			one := new(big.Int).SetInt64(1)
			return "(" + value.BigDecimal{Int: new(big.Int).Add(x.Int, one), Scale: x.Scale}.String() + " - " + value.BigDecimal{Int: one, Scale: x.Scale}.String() + ")"
		}
		return x.String()
	case value.DecFloat:
		return "CAST('" + string(x) + "' AS DECFLOAT(34))"
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
	case value.TimeTZ:
		return "TIME '" + text(x) + "'"
	case value.TimestampTZ:
		return "TIMESTAMP '" + text(x) + "'"
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
