package export

import (
	"github.com/formeo/fbrescue/internal/meta"
	"github.com/formeo/fbrescue/internal/value"
)

// Columns that come first in <table>.versions.<format>: where the row's
// primary version is (the same for all versions of a row), the version's
// place in the chain (0 is the newest), its transaction and that
// transaction's state, and whether the version is a delete.
var versionColumns = []*meta.Column{
	{Name: "_PAGE", Field: value.Field{DType: value.TypeInt64, Length: 8}},
	{Name: "_LINE", Field: value.Field{DType: value.TypeLong, Length: 4}},
	{Name: "_VERSION", Field: value.Field{DType: value.TypeLong, Length: 4}},
	{Name: "_TRANSACTION", Field: value.Field{DType: value.TypeInt64, Length: 8}},
	{Name: "_STATE", Field: value.Field{DType: value.TypeVarying, Length: 2 + 11}},
	{Name: "_DELETED", Field: value.Field{DType: value.TypeShort, Length: 2}},
}

func versionsTable(t *meta.Table, cols []*meta.Column) *meta.Table {
	vt := &meta.Table{ID: t.ID, Name: t.Name + "_VERSIONS", Columns: append([]*meta.Column{}, versionColumns...)}
	for _, c := range cols {
		nc := *c
		nc.NotNull = false // delete stubs have no values
		vt.Columns = append(vt.Columns, &nc)
	}
	return vt
}

var stateNames = map[int]string{meta.TxActive: "active", meta.TxLimbo: "limbo", meta.TxDead: "rolled back", meta.TxCommitted: "committed"}

// writeVersions writes every version of a row, newest first. A version
// that cannot be read ends the chain with an entry in _errors.jsonl.
func writeVersions(cat *meta.Catalog, v meta.Version, rec []byte, out *output,
	values func(meta.Row) (string, error), vals []any, tr *TableResult, logAt func(uint32, int, string, string) error) error {
	chain, err := cat.Chain(v, rec)
	row := make([]any, len(versionColumns)+len(vals))
	for i, r := range chain {
		state := "unknown"
		if r.Header.Tx == 0 {
			state = "committed" // the system transaction
		} else if s, err := cat.TxState(r.Header.Tx); err == nil {
			state = stateNames[s]
		}
		deleted := 0
		for j := range vals {
			vals[j] = nil
		}
		if r.Deleted {
			deleted = 1
		} else if why, err := values(r); err != nil {
			return err
		} else if why != "" {
			if err := logAt(r.Page, r.Line, "", "version: "+why); err != nil {
				return err
			}
			continue
		}
		row[0], row[1], row[2] = value.Decimal{Int: int64(v.Page)}, value.Decimal{Int: int64(v.Line)}, value.Decimal{Int: int64(i)}
		row[3], row[4], row[5] = value.Decimal{Int: int64(r.Header.Tx)}, value.Text{S: state}, value.Decimal{Int: int64(deleted)}
		copy(row[len(versionColumns):], vals)
		if err := out.w.row(row); err != nil {
			return err
		}
		tr.Versions++
	}
	if err != nil {
		return logAt(v.Page, v.Line, "", "older versions: "+err.Error())
	}
	return nil
}
