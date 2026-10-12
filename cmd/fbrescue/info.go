package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/formeo/fbrescue/internal/fdb"
)

type infoJSON struct {
	Files    []fileJSON  `json:"files"`
	PageSize int         `json:"page_size"`
	Pages    uint32      `json:"pages"`
	Header   *headerJSON `json:"header"` // null if the header page is damaged
	Warnings []string    `json:"warnings"`
}

type fileJSON struct {
	Path      string `json:"path"`
	FirstPage uint32 `json:"first_page"`
	Pages     uint32 `json:"pages"`
}

type headerJSON struct {
	ODS             string   `json:"ods"`
	Firebird        string   `json:"firebird,omitempty"`
	Dialect         int      `json:"dialect"`
	Attributes      []string `json:"attributes"`
	UnknownFlags    uint16   `json:"unknown_flags,omitempty"`
	Created         string   `json:"created"`
	CreatedUTC      bool     `json:"created_utc"` // false: server local time
	OIT             uint64   `json:"oldest_transaction"`
	OAT             uint64   `json:"oldest_active"`
	OST             uint64   `json:"oldest_snapshot"`
	NextTransaction uint64   `json:"next_transaction"`
	NextAttachment  uint64   `json:"next_attachment"`
}

func runInfo(path string, asJSON bool, stdout, stderr io.Writer) int {
	db, err := fdb.Open(path)
	if err != nil {
		fmt.Fprintf(stderr, "fbrescue info: %v\n", err)
		return 1
	}
	defer db.Close()

	out := infoJSON{PageSize: db.PageSize, Pages: db.Pages, Warnings: warnings(db)}
	for _, p := range db.Files {
		out.Files = append(out.Files, fileJSON{p.Path, p.FirstPage, p.Pages})
	}
	if h := db.Header; h != nil {
		attrs, unknown := h.Attributes()
		out.Header = &headerJSON{
			ODS: fmt.Sprintf("%d.%d", h.ODSMajor, h.ODSMinor), Firebird: h.FirebirdVersion(),
			Dialect: h.Dialect(), Attributes: attrs, UnknownFlags: unknown,
			Created: h.Created.Format("2006-01-02 15:04:05"), CreatedUTC: h.ODSMajor >= 13,
			OIT: h.OIT, OAT: h.OAT, OST: h.OST, NextTransaction: h.NextTx, NextAttachment: h.NextAttachment,
		}
		if out.Header.Attributes == nil {
			out.Header.Attributes = []string{}
		}
	}
	if asJSON {
		writeJSON(stdout, out)
		return 0
	}

	var size int64
	for _, p := range db.Files {
		if st, err := os.Stat(p.Path); err == nil {
			size += st.Size()
		}
	}
	fmt.Fprintf(stdout, "File        %s · %s · %d pages × %d KB\n", filepath.Base(path), humanSize(size), db.Pages, db.PageSize/1024)
	if len(db.Files) > 1 {
		for _, p := range db.Files {
			fmt.Fprintf(stdout, "            %s: pages %d-%d\n", p.Path, p.FirstPage, p.FirstPage+p.Pages-1)
		}
	}
	if h := out.Header; h != nil {
		engine := "ODS " + h.ODS
		if h.Firebird != "" {
			engine += " (Firebird " + h.Firebird + ")"
		}
		zone := "server local time"
		if h.CreatedUTC {
			zone = "UTC"
		}
		attrs := strings.Join(h.Attributes, ", ")
		if attrs == "" {
			attrs = "–"
		}
		fmt.Fprintf(stdout, "Engine      %s · dialect %d\n", engine, h.Dialect)
		fmt.Fprintf(stdout, "Created     %s (%s)\n", h.Created, zone)
		fmt.Fprintf(stdout, "Tx          OIT %d · OAT %d · OST %d · next %d\n", h.OIT, h.OAT, h.OST, h.NextTransaction)
		fmt.Fprintf(stdout, "Attributes  %s\n", attrs)
	} else {
		family := "ODS 11 (Firebird 2.x)"
		if db.Family == fdb.ODS12Plus {
			family = "ODS 12 or later (Firebird 3.0+)"
		}
		fmt.Fprintf(stdout, "Engine      %s, detected from page layout\n", family)
	}
	for _, w := range out.Warnings {
		fmt.Fprintf(stdout, "⚠ %s\n", w)
	}
	return 0
}

// warnings combines problems found while opening the file with header
// states the user must know about before copying or recovering.
func warnings(db *fdb.File) []string {
	w := append([]string{}, db.Warnings...)
	if h := db.Header; h != nil {
		if h.BackupLocked() {
			w = append(w, "nbackup lock is set: changes made after the lock are in the delta file, not in this file — copy the .delta file together with the database")
		}
		if _, unknown := h.Attributes(); unknown != 0 {
			w = append(w, fmt.Sprintf("unknown header flags %#04x", unknown))
		}
	}
	return w
}
