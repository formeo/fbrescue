package meta

import (
	"errors"
	"fmt"

	"github.com/formeo/fbrescue/internal/fdb"
	"github.com/formeo/fbrescue/internal/scan"
)

// guessHeader loads the catalog of a database whose header page is damaged.
// The page size and page layout family are known from the other pages
// (fdb.Open); of the ODS versions with that layout, the one under which the
// system tables read back their own names is taken. It is set as
// db.Header, with the fields the catalog does not need left zero.
func guessHeader(db *fdb.File, rep *scan.Report) (*Catalog, error) {
	versions := [][2]int{{11, 2}}
	if db.Family == fdb.ODS12Plus {
		versions = [][2]int{{12, 0}, {13, 0}, {13, 1}}
	}
	var best *Catalog
	var bestHeader *fdb.Header
	bestScore := 0
	for _, v := range versions {
		db.Header = &fdb.Header{PageSize: db.PageSize, ODSMajor: v[0], ODSMinor: v[1]}
		c, err := load(db, rep)
		if err != nil {
			continue
		}
		if s := c.systemScore(); s > bestScore {
			best, bestHeader, bestScore = c, db.Header, s
		}
	}
	db.Header = bestHeader
	if best == nil {
		return nil, errors.New("the header page is damaged, and the system tables do not read correctly under any ODS version")
	}
	best.Warnings = append([]string{fmt.Sprintf("the header page is damaged: read as ODS %d.%d, the version under which the system tables read correctly",
		bestHeader.ODSMajor, bestHeader.ODSMinor)}, best.Warnings...)
	return best, nil
}

// systemScore measures how well the system tables read: rows of
// RDB$RELATIONS that carry the right name for their system relation id
// count most, then the user columns found.
func (c *Catalog) systemScore() int {
	names := map[int64]string{0: "RDB$PAGES", 1: "RDB$DATABASE", 2: "RDB$FIELDS", 3: "RDB$INDEX_SEGMENTS",
		4: "RDB$INDICES", 5: "RDB$RELATION_FIELDS", 6: "RDB$RELATIONS", 8: "RDB$FORMATS"}
	f := systemFormat(relRelations, c.db)
	matched := 0
	defer func(w []string) { c.Warnings = w }(c.Warnings) // already reported by load
	c.visible(relRelations, func(v Version, rec []byte) {
		if n, ok := names[intField(rec, f["RELATION_ID"])]; ok && textField(rec, f["RELATION_NAME"]) == n {
			matched++
		}
	})
	if matched == 0 {
		return 0
	}
	cols := 0
	for _, t := range c.Tables {
		cols += len(t.Columns)
	}
	return matched<<20 + cols
}

// relocate points pages whose contents are stored at another location (ODS
// 12+ pages carry their own number) to that location, when the page's own
// location does not hold it and only one location does.
func relocate(db *fdb.File, rep *scan.Report) int {
	buf := make([]byte, db.PageSize)
	n := 0
	for page, at := range rep.Misplaced {
		if len(at) != 1 || page >= db.Pages {
			continue
		}
		if err := db.ReadPage(page, buf); err == nil && buf[0] != 0 && fdb.PageNumber(buf) == page {
			continue // the page is in its place; the other copy is stale
		}
		db.Relocate(page, at[0])
		n++
	}
	return n
}
