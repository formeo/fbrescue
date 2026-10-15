package main

import (
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/formeo/fbrescue/internal/export"
	"github.com/formeo/fbrescue/internal/fdb"
	"github.com/formeo/fbrescue/internal/meta"
	"github.com/formeo/fbrescue/internal/record"
	"github.com/formeo/fbrescue/internal/value"
)

type pageJSON struct {
	Page       uint32         `json:"page"`
	Type       int            `json:"type"`
	TypeName   string         `json:"type_name"`
	Flags      int            `json:"flags"`
	PageNumber *uint32        `json:"stored_page_number,omitempty"` // ODS 12+
	Fields     map[string]any `json:"fields,omitempty"`
	Records    []recordJSON   `json:"records,omitempty"`
	Pages      []uint32       `json:"pages,omitempty"` // pointer page slots
	Problems   []string       `json:"problems"`
}

type recordJSON struct {
	Line     int               `json:"line"`
	Offset   int               `json:"offset"`
	Length   int               `json:"length"`
	Tx       uint32            `json:"transaction"`
	TxState  string            `json:"transaction_state,omitempty"`
	Flags    []string          `json:"flags"`
	Back     string            `json:"back_version,omitempty"` // page:line
	Format   uint8             `json:"format"`
	Values   map[string]string `json:"values,omitempty"`
	ValueErr string            `json:"values_error,omitempty"`
}

var pageTypes = map[byte]string{
	0: "unused", fdb.TypeHeader: "header", fdb.TypePIP: "page inventory", fdb.TypeTIP: "transaction inventory",
	fdb.TypePointer: "pointer", fdb.TypeData: "data", fdb.TypeIndexRoot: "index root", fdb.TypeBTree: "index",
	fdb.TypeBlob: "blob", fdb.TypeGenerator: "generator", 10: "type 10",
}

var flagNames = []struct {
	bit  uint16
	name string
}{
	{record.FlagDeleted, "deleted"}, {record.FlagChain, "back version"}, {record.FlagFragment, "fragment"},
	{record.FlagIncomplete, "continues"}, {record.FlagBlob, "blob"}, {record.FlagDelta, "back version is a delta"},
	{record.FlagLarge, "large"}, {record.FlagDamaged, "damaged"}, {0x0100, "gc active"}, {0x0200, "key changed"},
	{record.FlagLongTx, "48-bit transaction"}, {record.FlagNotPacked, "not packed"},
}

var txStates = map[int]string{meta.TxActive: "active", meta.TxLimbo: "limbo", meta.TxDead: "dead", meta.TxCommitted: "committed"}

func runPage(a cliArgs, stdout, stderr io.Writer) int {
	n, err := strconv.ParseUint(a.pageNo, 10, 32)
	if err != nil {
		fmt.Fprintf(stderr, "fbrescue page: page number %q: %v\n", a.pageNo, err)
		return 1
	}
	db, err := fdb.Open(a.path)
	if err != nil {
		fmt.Fprintf(stderr, "fbrescue page: %v\n", err)
		return 1
	}
	defer db.Close()
	buf := make([]byte, db.PageSize)
	if err := db.ReadPage(uint32(n), buf); err != nil {
		fmt.Fprintf(stderr, "fbrescue page: %v\n", err)
		return 1
	}
	// Names and values need the catalog; the page is shown without them if
	// the system tables cannot be read.
	cat, catErr := meta.Load(db)

	p := pageJSON{Page: uint32(n), Type: int(buf[0]), TypeName: pageTypes[buf[0]], Flags: int(buf[1]), Fields: map[string]any{}, Problems: []string{}}
	if p.TypeName == "" {
		p.TypeName = fmt.Sprintf("invalid type %d", buf[0])
	}
	if db.Family == fdb.ODS12Plus {
		pn := fdb.PageNumber(buf)
		p.PageNumber = &pn
		if pn != uint32(n) && buf[0] != 0 {
			p.Problems = append(p.Problems, fmt.Sprintf("holds the contents of page %d", pn))
		}
	}
	tableName := func(rel uint16) string {
		if cat != nil {
			for _, t := range cat.Tables {
				if t.ID == rel {
					return t.Name
				}
			}
		}
		return ""
	}
	switch buf[0] {
	case fdb.TypePointer:
		rel := le16(buf[0x1A:])
		p.Fields["relation"], p.Fields["table"] = rel, tableName(rel)
		p.Fields["sequence"], p.Fields["next"] = le32(buf[0x10:]), le32(buf[0x14:])
		for i := 0; i < int(le16(buf[0x18:])) && 0x20+4*i+4 <= len(buf); i++ {
			p.Pages = append(p.Pages, le32(buf[0x20+4*i:]))
		}
	case fdb.TypeTIP:
		p.Fields["next"] = le32(buf[0x10:])
		counts := map[string]int{}
		for i := 0; i < (len(buf)-0x14)*4; i++ {
			counts[txStates[int(buf[0x14+i/4]>>(i%4*2)&3)]]++
		}
		p.Fields["states"] = counts
	case fdb.TypeBlob:
		p.Fields["lead_page"], p.Fields["sequence"], p.Fields["length"] = le32(buf[0x10:]), le32(buf[0x14:]), le16(buf[0x18:])
	case fdb.TypeData:
		p.Records = dataRecords(db, cat, uint32(n), buf, &p, tableName)
	}
	if catErr != nil && (buf[0] == fdb.TypeData || buf[0] == fdb.TypePointer) {
		p.Problems = append(p.Problems, "table names and values unavailable: "+catErr.Error())
	}

	if a.json {
		writeJSON(stdout, p)
		return 0
	}
	fmt.Fprintf(stdout, "Page %d: %s (type %d, flags %#02x)\n", p.Page, p.TypeName, p.Type, p.Flags)
	if p.PageNumber != nil {
		fmt.Fprintf(stdout, "Stored page number %d\n", *p.PageNumber)
	}
	for _, k := range []string{"relation", "table", "sequence", "next", "lead_page", "length", "states"} {
		if v, ok := p.Fields[k]; ok && v != "" {
			fmt.Fprintf(stdout, "%-10s %v\n", strings.ReplaceAll(k, "_", " "), v)
		}
	}
	if len(p.Pages) > 0 {
		fmt.Fprintf(stdout, "data pages %v\n", p.Pages)
	}
	for _, r := range p.Records {
		fmt.Fprintf(stdout, "\nline %d  offset %d  length %d  tx %d %s  format %d  [%s]", r.Line, r.Offset, r.Length, r.Tx, r.TxState, r.Format, strings.Join(r.Flags, ", "))
		if r.Back != "" {
			fmt.Fprintf(stdout, "  older version at %s", r.Back)
		}
		fmt.Fprintln(stdout)
		for _, kv := range sortedValues(r.Values) {
			fmt.Fprintf(stdout, "    %s\n", kv)
		}
		if r.ValueErr != "" {
			fmt.Fprintf(stdout, "    (%s)\n", r.ValueErr)
		}
	}
	for _, pr := range p.Problems {
		fmt.Fprintf(stdout, "⚠ %s\n", pr)
	}
	return 0
}

func dataRecords(db *fdb.File, cat *meta.Catalog, n uint32, buf []byte, p *pageJSON, tableName func(uint16) string) []recordJSON {
	rel := le16(buf[0x14:])
	p.Fields["relation"], p.Fields["table"], p.Fields["sequence"] = rel, tableName(rel), le32(buf[0x10:])
	var table *meta.Table
	if cat != nil {
		for _, t := range cat.Tables {
			if t.ID == rel {
				table = t
			}
		}
	}
	rd := record.NewReader(db)
	_, _, slots, err := rd.DataPage(n)
	if err != nil {
		p.Problems = append(p.Problems, err.Error())
	}
	// A back version is stored as differences from the newer version when
	// that one has FlagDelta. Only newer versions on this page are known.
	newer := map[int]string{}
	isDelta := map[int]bool{}
	for _, s := range slots {
		if h := s.Header; h.BackPage == n && h.Flags&record.FlagBlob == 0 {
			newer[int(h.BackLine)] = fmt.Sprintf("%d:%d", n, s.Line)
			isDelta[int(h.BackLine)] = h.Flags&record.FlagDelta != 0
		}
	}
	var out []recordJSON
	for _, s := range slots {
		h := s.Header
		off, ln := int(le16(buf[0x18+4*s.Line:])), int(le16(buf[0x1A+4*s.Line:]))
		r := recordJSON{Line: s.Line, Offset: off, Length: ln, Tx: h.Tx, Format: h.Format, Flags: []string{}}
		for _, f := range flagNames {
			if h.Flags&f.bit != 0 {
				r.Flags = append(r.Flags, f.name)
			}
		}
		if h.Flags&record.FlagBlob == 0 && cat != nil {
			if st, err := cat.TxState(h.Tx); err == nil && h.Tx != 0 {
				r.TxState = txStates[st]
			}
		}
		if h.BackPage != 0 && h.Flags&record.FlagBlob == 0 {
			r.Back = fmt.Sprintf("%d:%d", h.BackPage, h.BackLine)
		}
		// Values of complete records: primary versions and full back versions.
		chained := h.Flags&record.FlagChain != 0
		switch {
		case chained && isDelta[s.Line]:
			r.ValueErr = "stored as differences from the newer version at " + newer[s.Line]
		case table != nil && h.Flags&(record.FlagFragment|record.FlagBlob|record.FlagDeleted) == 0:
			r.Values, r.ValueErr = recordValues(rd, table, n, s.Line, h)
			if _, known := newer[s.Line]; chained && !known {
				r.ValueErr = "the newer version is on another page; if it has 'back version is a delta', these values are not real"
			}
		}
		out = append(out, r)
	}
	return out
}

func recordValues(rd *record.Reader, t *meta.Table, n uint32, line int, h record.Header) (map[string]string, string) {
	fm, ok := t.Formats[uint16(h.Format)]
	if !ok {
		return nil, fmt.Sprintf("format %d unknown", h.Format)
	}
	_, data, err := rd.Read(n, line)
	if err != nil {
		return nil, err.Error()
	}
	vals := map[string]string{}
	for _, c := range t.Columns {
		if c.Field.ID >= len(fm.Fields) || !fm.Fields[c.Field.ID].Stored() {
			continue
		}
		f := fm.Fields[c.Field.ID]
		if !f.Supported() {
			vals[c.Name] = "(unsupported type)"
			continue
		}
		v, err := value.Decode(data, f, value.Options{})
		switch {
		case err != nil:
			vals[c.Name] = "(" + err.Error() + ")"
		case v == nil:
			vals[c.Name] = "NULL"
		default:
			if b, ok := v.(value.BlobRef); ok {
				vals[c.Name] = fmt.Sprintf("blob %d:%d", b.Relation, b.Number)
			} else {
				s := export.Text(v)
				if r := []rune(s); len(r) > 60 {
					s = string(r[:60]) + "…"
				}
				vals[c.Name] = strconv.Quote(s)
			}
		}
	}
	return vals, ""
}

func sortedValues(m map[string]string) []string {
	var out []string
	for k, v := range m {
		out = append(out, k+" = "+v)
	}
	sortStrings(out)
	return out
}

func le16(b []byte) uint16 { return uint16(b[0]) | uint16(b[1])<<8 }
func le32(b []byte) uint32 { return uint32(le16(b)) | uint32(le16(b[2:]))<<16 }

func sortStrings(s []string) { slices.Sort(s) }
