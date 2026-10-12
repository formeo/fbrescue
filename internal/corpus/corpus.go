// Package corpus gives tests access to the databases built by
// tools/mkcorpus: real files from Firebird 2.5, 3.0, 4.0 and 5.0, each with
// the engine's own `gstat -h` output as the reference.
package corpus

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// EnvVar names the corpus directory. Tests that need the corpus are skipped
// when it is unset.
const EnvVar = "FBRESCUE_CORPUS"

// DB is one corpus database.
type DB struct {
	Path   string // path to the .fdb file
	Engine string // "fb25", "fb30", "fb40" or "fb50"
	Name   string // "base", "deleted", "readonly", ...
	gstat  string
}

// List returns all corpus databases, skipping the test if the corpus is not
// configured and failing it if the configured directory has no databases.
func List(t testing.TB) []DB {
	t.Helper()
	dir := os.Getenv(EnvVar)
	if dir == "" {
		t.Skipf("%s is not set; build the corpus with tools/mkcorpus/mkcorpus.sh", EnvVar)
	}
	paths, err := filepath.Glob(filepath.Join(dir, "fb*", "*.fdb"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatalf("no databases in %s", dir)
	}
	dbs := make([]DB, 0, len(paths))
	for _, p := range paths {
		g, err := os.ReadFile(strings.TrimSuffix(p, ".fdb") + ".gstat.txt")
		if err != nil {
			t.Fatal(err)
		}
		dbs = append(dbs, DB{
			Path:   p,
			Engine: filepath.Base(filepath.Dir(p)),
			Name:   strings.TrimSuffix(filepath.Base(p), ".fdb"),
			gstat:  string(g),
		})
	}
	return dbs
}

// GstatRelation is one table from `gstat -d -r -s`.
type GstatRelation struct {
	Name               string
	PrimaryPointerPage uint32
	DataPages          int
	Records, Versions  int
}

var (
	relHeading = regexp.MustCompile(`^(\S+) \((\d+)\)$`)
	relNumber  = regexp.MustCompile(`(Primary pointer page|Data pages|total records|total versions): (\d+)`)
)

// Relations returns the tables reported by `gstat -d -r -s`, keyed by
// relation id. ok is false if gstat could not produce them (for example,
// for a database in full shutdown).
func (d DB) Relations(t testing.TB) (rels map[uint16]GstatRelation, ok bool) {
	t.Helper()
	b, err := os.ReadFile(strings.TrimSuffix(d.Path, ".fdb") + ".gstat-r.txt")
	if os.IsNotExist(err) {
		return nil, false
	}
	if err != nil {
		t.Fatal(err)
	}
	rels = map[uint16]GstatRelation{}
	var id uint16
	for _, line := range strings.Split(string(b), "\n") {
		if m := relHeading.FindStringSubmatch(line); m != nil {
			n, _ := strconv.Atoi(m[2])
			id = uint16(n)
			rels[id] = GstatRelation{Name: m[1]}
			continue
		}
		r, found := rels[id]
		if !found {
			continue
		}
		for _, m := range relNumber.FindAllStringSubmatch(line, -1) {
			n, _ := strconv.Atoi(m[2])
			switch m[1] {
			case "Primary pointer page":
				r.PrimaryPointerPage = uint32(n)
			case "Data pages":
				r.DataPages = n
			case "total records":
				r.Records = n
			case "total versions":
				r.Versions = n
			}
		}
		rels[id] = r
	}
	if len(rels) == 0 {
		t.Fatalf("%s: no tables in gstat -r output", d.Path)
	}
	return rels, true
}

// Gstat returns the value of a `gstat -h` line such as "Page size" or
// "ODS version", or "" if the line is absent.
func (d DB) Gstat(key string) string {
	for _, line := range strings.Split(d.gstat, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if ok && k == key {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
