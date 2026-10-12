package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/formeo/fbrescue/internal/corpus"
)

// TestNeverWrites runs every command on read-only copies of every corpus
// database and checks that the files are unchanged. Opening a file for
// writing fails on a 0444 file (when not running as root), and the content
// hash and modification time catch any write that would slip through.
func TestNeverWrites(t *testing.T) {
	for _, db := range corpus.List(t) {
		t.Run(db.Engine+"/"+db.Name, func(t *testing.T) {
			// The main file plus a secondary file, if the database has one.
			dir := t.TempDir()
			path := filepath.Join(dir, filepath.Base(db.Path))
			srcs := []string{db.Path}
			if second := strings.TrimSuffix(db.Path, ".fdb") + ".f2"; fileExists(second) {
				srcs = append(srcs, second)
			}
			old := time.Now().Add(-time.Hour).Truncate(time.Second)
			sums := map[string][32]byte{}
			for _, src := range srcs {
				b, err := os.ReadFile(src)
				if err != nil {
					t.Fatal(err)
				}
				dst := filepath.Join(dir, filepath.Base(src))
				if err := os.WriteFile(dst, b, 0o444); err != nil {
					t.Fatal(err)
				}
				if err := os.Chtimes(dst, old, old); err != nil {
					t.Fatal(err)
				}
				sums[dst] = sha256.Sum256(b)
			}

			for _, args := range [][]string{{"info", path}, {"info", "--json", path}, {"scan", path}, {"scan", "--json", path}} {
				var out, errOut bytes.Buffer
				if code := run(args, &out, &errOut); code == 1 {
					t.Errorf("%v: exit 1: %s", args, errOut.String())
				}
			}

			for dst, sum := range sums {
				after, err := os.ReadFile(dst)
				if err != nil {
					t.Fatal(err)
				}
				st, err := os.Stat(dst)
				if err != nil {
					t.Fatal(err)
				}
				if sha256.Sum256(after) != sum || !st.ModTime().Equal(old) || st.Mode().Perm() != 0o444 {
					t.Errorf("%s changed: mtime %v (was %v), mode %v", dst, st.ModTime(), old, st.Mode())
				}
			}
		})
	}
}

func TestExitCodesAndJSON(t *testing.T) {
	var base string
	for _, db := range corpus.List(t) {
		if db.Engine == "fb50" && db.Name == "base" {
			base = db.Path
		}
	}
	if base == "" {
		t.Fatal("fb50/base not in corpus")
	}
	var out, errOut bytes.Buffer
	if code := run([]string{"scan", "--json", base}, &out, &errOut); code != 0 {
		t.Fatalf("healthy scan: exit %d: %s", code, errOut.String())
	}
	var r struct {
		Problems  []any `json:"problems"`
		Relations []any `json:"relations"`
	}
	if err := json.Unmarshal(out.Bytes(), &r); err != nil || len(r.Relations) == 0 || r.Problems == nil {
		t.Errorf("scan --json: %v, %d relations, problems %v", err, len(r.Relations), r.Problems)
	}

	// Truncated by part of a page: scan still works, but reports it.
	b, _ := os.ReadFile(base)
	damaged := filepath.Join(t.TempDir(), "damaged.fdb")
	os.WriteFile(damaged, b[:len(b)-100], 0o644)
	if code := run([]string{"scan", damaged}, &out, &errOut); code != 2 {
		t.Errorf("damaged scan: exit %d, want 2", code)
	}
	if code := run([]string{"info", filepath.Join(t.TempDir(), "missing.fdb")}, &out, &errOut); code != 1 {
		t.Errorf("missing file: exit %d, want 1", code)
	}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
