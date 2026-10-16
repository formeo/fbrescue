// Package ddl reads table definitions from a Firebird SQL script (CREATE
// TABLE, CREATE DOMAIN, ALTER TABLE ... ADD) and computes the record layout
// the engine gives such a table. It is used when the database's own record
// formats are lost.
//
// The layout is right for a table whose columns are listed in the order
// they were created: field ids follow that order. Everything that is not a
// table or domain definition (procedures, triggers, inserts) is skipped.
package ddl

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/formeo/fbrescue/internal/value"
)

// Table is a table definition with its computed record layout.
type Table struct {
	Name    string
	Columns []Column
}

// Column is a column of a table. Field.ID and Field.Offset are set; a
// COMPUTED BY column has a field id but no offset.
type Column struct {
	Name     string
	Field    value.Field
	NotNull  bool
	Computed bool
}

// Length is the length of a record with this layout.
func (t *Table) Length() int {
	end := 0
	for _, c := range t.Columns {
		if !c.Computed {
			end = max(end, int(c.Field.Offset)+int(c.Field.Length))
		}
	}
	return end
}

// Fields returns the fields in field id order, as a record format holds them.
func (t *Table) Fields() []value.Field {
	out := make([]value.Field, len(t.Columns))
	for i, c := range t.Columns {
		out[i] = c.Field
	}
	return out
}

type domain struct {
	f       value.Field
	cs      bool // character set given
	notNull bool
}

type parser struct {
	defaultCS int
	domains   map[string]domain
	tables    []*Table
	byName    map[string]*Table
	warnings  []string
}

var defaultCharset = regexp.MustCompile(`(?i)\bDEFAULT\s+CHARACTER\s+SET\s+"?([A-Z0-9_]+)`)

// Parse reads the table definitions of a script. The database's default
// character set, used for text columns without one, comes from a CREATE
// DATABASE ... DEFAULT CHARACTER SET clause, also inside a comment as isql
// -x writes it; without one it is NONE. Warnings list what was skipped.
func Parse(script string) ([]*Table, []string, error) {
	p := &parser{domains: map[string]domain{}, byName: map[string]*Table{}}
	if m := defaultCharset.FindStringSubmatch(script); m != nil {
		cs, ok := value.CharsetByName(m[1])
		if !ok {
			return nil, nil, fmt.Errorf("unknown default character set %s", m[1])
		}
		p.defaultCS = cs
	}
	toks, err := tokenize(script)
	if err != nil {
		return nil, nil, err
	}
	for _, st := range statements(toks) {
		if err := p.statement(st); err != nil {
			return nil, nil, err
		}
	}
	for _, t := range p.tables {
		layout(t)
	}
	return p.tables, p.warnings, nil
}

// Tokens: words (upper-cased unless quoted), strings, numbers, punctuation.
type token struct {
	s      string
	quoted bool // "identifier" or 'string'
	str    bool // 'string'
}

func (t token) is(words ...string) bool {
	if t.quoted {
		return false
	}
	for _, w := range words {
		if t.s == w {
			return true
		}
	}
	return false
}

func tokenize(s string) ([]token, error) {
	var out []token
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			i++
		case strings.HasPrefix(s[i:], "--"):
			for i < len(s) && s[i] != '\n' {
				i++
			}
		case strings.HasPrefix(s[i:], "/*"):
			end := strings.Index(s[i+2:], "*/")
			if end < 0 {
				return nil, fmt.Errorf("comment at byte %d does not end", i)
			}
			i += end + 4
		case c == '\'' || c == '"':
			j := i + 1
			var b strings.Builder
			for ; ; j++ {
				if j >= len(s) {
					return nil, fmt.Errorf("quoted text at byte %d does not end", i)
				}
				if s[j] == c {
					if j+1 < len(s) && s[j+1] == c {
						b.WriteByte(c)
						j++
						continue
					}
					break
				}
				b.WriteByte(s[j])
			}
			out = append(out, token{s: b.String(), quoted: true, str: c == '\''})
			i = j + 1
		case isWord(c):
			j := i
			for j < len(s) && isWord(s[j]) {
				j++
			}
			out = append(out, token{s: strings.ToUpper(s[i:j])})
			i = j
		case c == '(' || c == ')' || c == ',' || c == ';':
			out = append(out, token{s: string(c)})
			i++
		default: // other punctuation, in runs: ^, ||, !! ...
			j := i
			for j < len(s) && !isWord(s[j]) && !strings.ContainsRune(" \t\r\n()',;\"", rune(s[j])) {
				j++
			}
			out = append(out, token{s: s[i:j]})
			i = j
		}
	}
	return out, nil
}

func isWord(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '$' || c == '.' || c >= 0x80
}

// statements splits tokens at the terminator, which SET TERM changes.
func statements(toks []token) [][]token {
	var out [][]token
	term := ";"
	var cur []token
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if len(cur) == 0 && t.is("SET") && i+2 < len(toks) && toks[i+1].is("TERM") {
			term = toks[i+2].s
			i += 2
			for i+1 < len(toks) && !toks[i+1].quoted && toks[i+1].s != term && (toks[i+1].s == ";" || toks[i+1].s == "^") {
				i++ // the old terminator ending SET TERM
			}
			continue
		}
		if !t.quoted && t.s == term {
			if len(cur) > 0 {
				out = append(out, cur)
			}
			cur = nil
			continue
		}
		cur = append(cur, t)
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

func (p *parser) statement(st []token) error {
	switch {
	case len(st) >= 3 && (st[0].is("CREATE", "RECREATE") && st[1].is("TABLE")):
		return p.createTable(st[2], st[3:])
	case len(st) >= 5 && st[0].is("CREATE") && st[1].is("GLOBAL") && st[3].is("TABLE"):
		p.warn("%s is a temporary table: its data is not kept in the database", st[4].s)
	case len(st) >= 3 && st[0].is("CREATE") && st[1].is("DOMAIN"):
		return p.createDomain(st[2], st[3:])
	case len(st) >= 4 && st[0].is("ALTER") && st[1].is("TABLE"):
		return p.alterTable(st[2], st[3:])
	case len(st) >= 4 && st[0].is("SET") && st[1].is("SQL") && st[2].is("DIALECT") && st[3].s != "3":
		return fmt.Errorf("SQL dialect %s scripts are not supported, only dialect 3", st[3].s)
	}
	return nil
}

func (p *parser) warn(format string, args ...any) {
	p.warnings = append(p.warnings, fmt.Sprintf(format, args...))
}

func (p *parser) createTable(name token, rest []token) error {
	if len(rest) == 0 || rest[0].s != "(" {
		if len(rest) > 0 && rest[0].is("EXTERNAL") {
			p.warn("%s is an external table: its data is not in the database", name.s)
			return nil
		}
		return fmt.Errorf("CREATE TABLE %s: expected (", name.s)
	}
	body, _, err := parens(rest)
	if err != nil {
		return fmt.Errorf("CREATE TABLE %s: %w", name.s, err)
	}
	t := &Table{Name: name.s}
	for _, el := range splitTop(body) {
		if err := p.element(t, el); err != nil {
			return fmt.Errorf("table %s: %w", name.s, err)
		}
	}
	if old, ok := p.byName[t.Name]; ok {
		*old = *t // RECREATE, or the same table twice: the last one counts
		return nil
	}
	p.byName[t.Name] = t
	p.tables = append(p.tables, t)
	return nil
}

func (p *parser) alterTable(name token, rest []token) error {
	t, ok := p.byName[name.s]
	if !ok {
		return nil // a table the script does not create
	}
	for _, el := range splitTop(rest) {
		switch {
		case len(el) >= 2 && el[0].is("ADD") && !el[1].is("CONSTRAINT", "PRIMARY", "UNIQUE", "FOREIGN", "CHECK"):
			if err := p.element(t, el[1:]); err != nil {
				return fmt.Errorf("ALTER TABLE %s: %w", name.s, err)
			}
		case len(el) >= 1 && el[0].is("DROP", "ALTER") && !(len(el) >= 2 && el[1].is("CONSTRAINT")):
			// The engine keeps a dropped or changed column's field id and
			// stores older records with older layouts: not derivable here.
			p.warn("table %s is left out: after ALTER TABLE ... %s its record layout cannot be derived from a script", name.s, el[0].s)
			p.drop(t)
			return nil
		}
	}
	return nil
}

func (p *parser) drop(t *Table) {
	delete(p.byName, t.Name)
	for i, x := range p.tables {
		if x == t {
			p.tables = append(p.tables[:i], p.tables[i+1:]...)
			break
		}
	}
}

func (p *parser) createDomain(name token, rest []token) error {
	if len(rest) > 0 && rest[0].is("AS") {
		rest = rest[1:]
	}
	f, cs, n, err := p.dataType(rest)
	if err != nil {
		return fmt.Errorf("domain %s: %w", name.s, err)
	}
	p.domains[name.s] = domain{f, cs, notNull(rest[n:])}
	return nil
}

// element is a column definition or a table constraint.
func (p *parser) element(t *Table, el []token) error {
	if len(el) == 0 || el[0].is("CONSTRAINT", "PRIMARY", "UNIQUE", "FOREIGN", "CHECK") {
		return nil
	}
	col := Column{Name: el[0].s}
	rest := el[1:]
	if len(rest) > 0 && (rest[0].is("COMPUTED") || rest[0].is("GENERATED") && len(rest) > 1 && rest[1].is("ALWAYS")) {
		col.Computed = true
		t.Columns = append(t.Columns, col)
		return nil
	}
	if len(rest) == 0 {
		return fmt.Errorf("column %s has no type", col.Name)
	}
	if d, ok := p.domains[rest[0].s]; ok {
		col.Field, col.NotNull = d.f, d.notNull || notNull(rest[1:])
		if !d.cs {
			col.Field = withCharset(col.Field, p.defaultCS)
		}
	} else {
		f, cs, n, err := p.dataType(rest)
		if err != nil {
			return fmt.Errorf("column %s: %w", col.Name, err)
		}
		if !cs {
			f = withCharset(f, p.defaultCS)
		}
		col.Field, col.NotNull = f, notNull(rest[n:])
	}
	t.Columns = append(t.Columns, col)
	return nil
}

func notNull(rest []token) bool {
	for i := 0; i+1 < len(rest); i++ {
		if rest[i].is("NOT") && rest[i+1].is("NULL") {
			return true
		}
	}
	return false
}

// withCharset sets the character set of a text field that did not name one.
func withCharset(f value.Field, cs int) value.Field {
	switch f.DType {
	case value.TypeText, value.TypeVarying:
		n := f.Length
		if f.DType == value.TypeVarying {
			n -= 2
		}
		chars := int(n) // made with one byte per character
		f.Length = uint16(chars * value.BytesPerChar(cs))
		if f.DType == value.TypeVarying {
			f.Length += 2
		}
		f.SubType = int16(cs)
	case value.TypeBlob:
		if f.SubType == 1 {
			f.Scale = int8(uint8(cs))
		}
	}
	return f
}

// dataType parses a type at the start of toks. It returns the field (text
// lengths for one byte per character unless a character set is given),
// whether a character set was given, and how many tokens were used.
func (p *parser) dataType(toks []token) (value.Field, bool, int, error) {
	at := func(i int) token {
		if i < len(toks) {
			return toks[i]
		}
		return token{}
	}
	// args reads "(a[, b])" at i.
	args := func(i int) ([]int, int, error) {
		if at(i).s != "(" {
			return nil, i, nil
		}
		var out []int
		j := i + 1
		for ; j < len(toks) && toks[j].s != ")"; j++ {
			if toks[j].s == "," {
				continue
			}
			n, err := strconv.Atoi(toks[j].s)
			if err != nil {
				return nil, j, fmt.Errorf("expected a number, found %q", toks[j].s)
			}
			out = append(out, n)
		}
		return out, j + 1, nil
	}
	if len(toks) == 0 {
		return value.Field{}, false, 0, fmt.Errorf("missing type")
	}
	f := value.Field{}
	n := 1
	var err error
	var a []int
	switch w := toks[0]; {
	case w.is("SMALLINT"):
		f.DType, f.Length = value.TypeShort, 2
	case w.is("INTEGER", "INT"):
		f.DType, f.Length = value.TypeLong, 4
	case w.is("BIGINT"):
		f.DType, f.Length = value.TypeInt64, 8
	case w.is("INT128"):
		f.DType, f.Length = value.TypeInt128, 16
	case w.is("BOOLEAN"):
		f.DType, f.Length = value.TypeBoolean, 1
	case w.is("REAL"):
		f.DType, f.Length = value.TypeReal, 4
	case w.is("FLOAT"):
		if a, n, err = args(1); err == nil {
			f.DType, f.Length = value.TypeReal, 4
			if len(a) > 0 && a[0] > 7 {
				f.DType, f.Length = value.TypeDouble, 8
			}
		}
	case w.is("DOUBLE"):
		f.DType, f.Length = value.TypeDouble, 8
		if at(1).is("PRECISION") {
			n = 2
		}
	case w.is("DECFLOAT"):
		if a, n, err = args(1); err == nil {
			f.DType, f.Length = value.TypeDec128, 16
			if len(a) > 0 && a[0] == 16 {
				f.DType, f.Length = value.TypeDec64, 8
			}
		}
	case w.is("NUMERIC", "DECIMAL", "DEC"):
		if a, n, err = args(1); err == nil {
			prec, scale := 9, 0
			if len(a) > 0 {
				prec = a[0]
			}
			if len(a) > 1 {
				scale = a[1]
			}
			f.Scale, f.SubType = int8(-scale), 1
			if !w.is("NUMERIC") {
				f.SubType = 2
			}
			switch {
			case prec < 1 || prec > 38 || scale < 0 || scale > prec:
				err = fmt.Errorf("%s(%d,%d) is not a valid precision and scale", w.s, prec, scale)
			case prec <= 4 && w.is("NUMERIC"):
				f.DType, f.Length = value.TypeShort, 2
			case prec <= 9:
				f.DType, f.Length = value.TypeLong, 4
			case prec <= 18:
				f.DType, f.Length = value.TypeInt64, 8
			default:
				f.DType, f.Length = value.TypeInt128, 16
			}
		}
	case w.is("DATE"):
		f.DType, f.Length = value.TypeDate, 4
	case w.is("TIME", "TIMESTAMP"):
		f.DType, f.Length = value.TypeTime, 4
		if w.is("TIMESTAMP") {
			f.DType, f.Length = value.TypeTimestamp, 8
		}
		switch {
		case at(1).is("WITH") && at(2).is("TIME") && at(3).is("ZONE"):
			n = 4
			f.DType, f.Length = value.TypeTimeTZ, 8
			if w.is("TIMESTAMP") {
				f.DType, f.Length = value.TypeTimestampTZ, 12
			}
		case at(1).is("WITHOUT") && at(2).is("TIME") && at(3).is("ZONE"):
			n = 4
		}
	case w.is("CHAR", "CHARACTER", "VARCHAR", "NCHAR", "NATIONAL", "BINARY", "VARBINARY"):
		i := 1
		varying := w.is("VARCHAR", "VARBINARY")
		national := w.is("NCHAR", "NATIONAL")
		if w.is("NATIONAL") {
			if !at(1).is("CHAR", "CHARACTER") {
				return f, false, 0, fmt.Errorf("expected NATIONAL CHARACTER")
			}
			i = 2
		}
		if at(i).is("VARYING") {
			varying = true
			i++
		}
		if a, n, err = args(i); err != nil {
			break
		}
		length := 1
		if len(a) > 0 {
			length = a[0]
		} else if varying {
			return f, false, 0, fmt.Errorf("%s needs a length", w.s)
		}
		f.DType, f.Length = value.TypeText, uint16(length)
		if varying {
			f.DType, f.Length = value.TypeVarying, uint16(length+2)
		}
		switch {
		case w.is("BINARY", "VARBINARY"):
			return withCharset(f, 1), true, n, nil // OCTETS
		case national:
			return withCharset(f, 21), true, n, nil // ISO8859_1
		}
		if at(n).is("CHARACTER") && at(n+1).is("SET") {
			cs, ok := value.CharsetByName(at(n + 2).s)
			if !ok {
				return f, false, 0, fmt.Errorf("unknown character set %s", at(n+2).s)
			}
			return withCharset(f, cs), true, n + 3, nil
		}
		return f, false, n, nil
	case w.is("BLOB"):
		f.DType, f.Length = value.TypeBlob, 8
		if a, n, err = args(1); err != nil {
			break
		}
		if len(a) > 1 {
			f.SubType = int16(a[1])
		}
		cs, hasCS := 0, false
		for n < len(toks) {
			switch {
			case at(n).is("SUB_TYPE"):
				switch s := at(n + 1); {
				case s.is("TEXT"):
					f.SubType = 1
				case s.is("BINARY"):
					f.SubType = 0
				default:
					st, err := strconv.Atoi(s.s)
					if err != nil {
						return f, false, 0, fmt.Errorf("blob sub type %q is not supported", s.s)
					}
					f.SubType = int16(st)
				}
				n += 2
			case at(n).is("SEGMENT") && at(n+1).is("SIZE"):
				n += 3
			case at(n).is("CHARACTER") && at(n+1).is("SET"):
				var ok bool
				if cs, ok = value.CharsetByName(at(n + 2).s); !ok {
					return f, false, 0, fmt.Errorf("unknown character set %s", at(n+2).s)
				}
				hasCS = true
				n += 3
			default:
				goto blobDone
			}
		}
	blobDone:
		if hasCS {
			if f.SubType == 0 && cs != 0 {
				f.SubType = 1 // a character set makes it a text blob
			}
			return withCharset(f, cs), true, n, nil
		}
		return f, false, n, nil
	default:
		return f, false, 0, fmt.Errorf("unknown type or domain %s", w.s)
	}
	if err != nil {
		return f, false, 0, err
	}
	return f, false, n, nil
}

// parens returns the tokens inside the parentheses that toks starts with.
func parens(toks []token) ([]token, int, error) {
	depth := 0
	for i, t := range toks {
		switch {
		case t.quoted:
		case t.s == "(":
			depth++
		case t.s == ")":
			depth--
			if depth == 0 {
				return toks[1:i], i + 1, nil
			}
		}
	}
	return nil, 0, fmt.Errorf("parenthesis does not close")
}

// splitTop splits at commas outside parentheses.
func splitTop(toks []token) [][]token {
	var out [][]token
	depth, start := 0, 0
	for i, t := range toks {
		switch {
		case t.quoted:
		case t.s == "(":
			depth++
		case t.s == ")":
			depth--
		case t.s == "," && depth == 0:
			out = append(out, toks[start:i])
			start = i + 1
		}
	}
	return append(out, toks[start:])
}

// layout sets field ids and offsets: null bitmap first, then the stored
// fields in field id order, each aligned for its type.
func layout(t *Table) {
	n := uint32(len(t.Columns))
	off := ((n + 32) &^ 31) >> 3
	for i := range t.Columns {
		c := &t.Columns[i]
		c.Field.ID = i
		if c.Computed {
			continue
		}
		a := value.Alignment(c.Field.DType)
		off = (off + a - 1) / a * a
		c.Field.Offset = off
		off += uint32(c.Field.Length)
	}
}
