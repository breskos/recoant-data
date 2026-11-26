package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/breskos/recoant-data/synthetic"
	"github.com/breskos/recoant-data/synthetic/internal/gen"
)

// tiny is a small gzip-compressed data set with a table.
func tiny() *gen.Set {
	s := gen.EdgeCases()
	s.Meta.Name, s.Meta.Folder = "basic/tiny", "basic"
	s.Gzip = true
	s.Meta.Ordered = true // metadata only
	s.Tables = []gen.Table{{Name: "items.csv", Header: []string{"item_id"}, Rows: [][]string{{"i1"}}}}
	return s
}

func nolog(string, ...any) {}

func TestRun(t *testing.T) {
	dir := t.TempDir()
	if err := run(dir, "", []func() *gen.Set{tiny, gen.Invalid}, nolog); err != nil {
		t.Fatal(err)
	}
	catalog, err := os.ReadFile(filepath.Join(dir, "catalog.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"basic/tiny"`, `"signals.csv.gz"`, `"loading/invalid"`, "Invalid:", "Ordered:", `{Name: "items.csv", Rows: 1`} {
		if !strings.Contains(string(catalog), want) {
			t.Errorf("catalog.go lacks %s", want)
		}
	}
	readme, err := os.ReadFile(filepath.Join(dir, "README.md"))
	if err != nil || !strings.Contains(string(readme), "#### [basic/tiny](basic/tiny)") || !strings.Contains(string(readme), "attributes.csv") || !strings.Contains(string(readme), "ordered by time") {
		t.Errorf("README.md: %v", err)
	}
	n := 0
	if err := synthetic.ReadSignals(filepath.Join(dir, "basic/tiny/signals.csv.gz"), func(synthetic.Signal) error { n++; return nil }); err != nil || n != 18 {
		t.Errorf("gzip signals: %d %v", n, err)
	}

	// With a filter only matching data sets are written, no catalog.
	only := t.TempDir()
	if err := run(only, "invalid", []func() *gen.Set{tiny, gen.Invalid}, nolog); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(only)
	if len(entries) != 1 || entries[0].Name() != "loading" {
		t.Errorf("filtered run wrote %v", entries)
	}
}

func TestRunErrors(t *testing.T) {
	dir := t.TempDir()
	// A file where a folder must go.
	if err := os.WriteFile(filepath.Join(dir, "basic"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run(dir, "", []func() *gen.Set{tiny}, nolog); err == nil {
		t.Error("data set folder blocked by a file")
	}
	// A folder where a file must go.
	for _, name := range []string{"catalog.go", "README.md", "loading/invalid/signals.csv"} {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := run(dir, "", []func() *gen.Set{gen.Invalid}, nolog); err == nil {
			t.Errorf("%s blocked by a folder", name)
		}
	}
	// A table blocked by a folder.
	dir = t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "basic/tiny/items.csv"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := run(dir, "", []func() *gen.Set{tiny}, nolog); err == nil {
		t.Error("table blocked by a folder")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestWriteErrors(t *testing.T) {
	for _, gz := range []bool{false, true} {
		if err := encodeCSV(failingWriter{}, gz, []string{"a"}, [][]string{{"1"}}); err == nil {
			t.Errorf("gzip %v: write error ignored", gz)
		}
	}
	ds := []synthetic.Dataset{{Name: "basic/x", Folder: "basic"}}
	if err := catalog(failingWriter{}, ds); err == nil {
		t.Error("catalog: write error ignored")
	}
	if err := readme(failingWriter{}, ds); err == nil {
		t.Error("readme: write error ignored")
	}
	if err := writeFile(filepath.Join(t.TempDir(), "x"), func(io.Writer) error { return errors.New("boom") }); err == nil {
		t.Error("writeFile: error of fn ignored")
	}
}
