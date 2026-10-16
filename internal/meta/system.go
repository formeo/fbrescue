package meta

import (
	"github.com/formeo/fbrescue/internal/fdb"
	"github.com/formeo/fbrescue/internal/value"
)

// System table formats are not in RDB$FORMATS: the engine builds them from
// its own definitions. fbrescue rebuilds the few it reads with the engine's
// layout rules (null bitmap, then fields in order, each aligned for its
// type) and the field lists of RDB$RELATION_FIELDS on each engine.
// docs/format.md has the details and how they were verified.

const (
	relPages          = 0
	relRelationFields = 5
	relRelations      = 6
	relFormats        = 8
)

type sysField struct {
	name  string
	dtype uint8
	len   uint16 // bytes; for VARCHAR without the 2-byte length
}

// names are CHAR(31) UNICODE_FSS before ODS 13, CHAR(63) UTF8 (252 bytes) from ODS 13 on.
func nameField(name string, ods int) sysField {
	if ods >= 13 {
		return sysField{name, value.TypeText, 252}
	}
	return sysField{name, value.TypeText, 31}
}

func sysFields(rel int, ods int) []sysField {
	short := func(n string) sysField { return sysField{n, value.TypeShort, 2} }
	long := func(n string) sysField { return sysField{n, value.TypeLong, 4} }
	blob := func(n string) sysField { return sysField{n, value.TypeBlob, 8} }
	name := func(n string) sysField { return nameField(n, ods) }
	switch rel {
	case relPages:
		return []sysField{long("PAGE_NUMBER"), short("RELATION_ID"), long("PAGE_SEQUENCE"), short("PAGE_TYPE")}
	case relFormats:
		return []sysField{short("RELATION_ID"), short("FORMAT"), blob("DESCRIPTOR")}
	case relRelations:
		// Fields after RELATION_TYPE are not read. Firebird 2.0 and 2.1
		// records end earlier; missing fields read as NULL.
		return []sysField{blob("VIEW_BLR"), blob("VIEW_SOURCE"), blob("DESCRIPTION"), short("RELATION_ID"),
			short("SYSTEM_FLAG"), short("DBKEY_LENGTH"), short("FORMAT"), short("FIELD_ID"), name("RELATION_NAME"),
			name("SECURITY_CLASS"), {"EXTERNAL_FILE", value.TypeVarying, 255}, blob("RUNTIME"),
			blob("EXTERNAL_DESCRIPTION"), name("OWNER_NAME"), name("DEFAULT_CLASS"), short("FLAGS"), short("RELATION_TYPE")}
	case relRelationFields:
		return []sysField{name("FIELD_NAME"), name("RELATION_NAME"), name("FIELD_SOURCE"), name("QUERY_NAME"),
			name("BASE_FIELD"), {"EDIT_STRING", value.TypeVarying, 127}, short("FIELD_POSITION"), blob("QUERY_HEADER"),
			short("UPDATE_FLAG"), short("FIELD_ID"), short("VIEW_CONTEXT"), blob("DESCRIPTION"), blob("DEFAULT_VALUE"),
			short("SYSTEM_FLAG"), name("SECURITY_CLASS"), name("COMPLEX_NAME"), short("NULL_FLAG")}
	}
	panic("no system format for relation")
}

// sysTotalFields is the field count of the full system table, which sets
// the size of the null bitmap; the lists above may be a prefix.
func sysTotalFields(rel, ods int) int {
	switch rel {
	case relRelations:
		if ods >= 13 {
			return 18
		}
		return 17
	case relRelationFields:
		if ods >= 12 {
			return 21
		}
		return 19
	}
	return len(sysFields(rel, ods))
}

// systemFormat returns the fields of a system table, by name.
func systemFormat(rel int, db *fdb.File) map[string]value.Field {
	ods := 11
	if db.Header != nil {
		ods = db.Header.ODSMajor
	}
	cs := 3 // UNICODE_FSS
	if ods >= 13 {
		cs = 4 // UTF8
	}
	n := uint32(sysTotalFields(rel, ods))
	off := ((n + 32) &^ 31) >> 3 // null bitmap, as the engine sizes it
	out := map[string]value.Field{}
	for i, sf := range sysFields(rel, ods) {
		a := value.Alignment(sf.dtype)
		off = (off + a - 1) / a * a
		f := value.Field{ID: i, DType: sf.dtype, Length: sf.len, Offset: off}
		if sf.dtype == value.TypeVarying {
			f.Length += 2
		}
		if sf.dtype == value.TypeText { // names; VARCHAR fields are CHARACTER SET NONE
			f.SubType = int16(cs)
		}
		out[sf.name] = f
		off += uint32(f.Length)
	}
	return out
}
