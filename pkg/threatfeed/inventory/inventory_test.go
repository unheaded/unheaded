// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package inventory

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func open(t *testing.T, name string) *os.File {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func TestReadGoMod(t *testing.T) {
	got, err := ReadGoMod(open(t, "fixture.go-mod"), "go.mod")
	if err != nil {
		t.Fatal(err)
	}
	want := []Component{
		{"Go", "example.test/alpha", "1.2.3", "go.mod"},
		{"Go", "example.test/beta/v2", "2.0.1", "go.mod"},
		{"Go", "example.test/gamma", "0.9.0", "go.mod"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v", got)
	}
}

func TestReadCargoLock(t *testing.T) {
	got, err := ReadCargoLock(open(t, "fixture.cargo-lock"), "Cargo.lock")
	if err != nil {
		t.Fatal(err)
	}
	want := []Component{{"crates.io", "serde", "1.0.230", "Cargo.lock"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v", got)
	}
}

func TestReadRequirements(t *testing.T) {
	got, unpinned, err := ReadRequirements(open(t, "fixture.reqs"), "r.txt")
	if err != nil {
		t.Fatal(err)
	}
	want := []Component{
		{"PyPI", "sentence-transformers", "6.2.0", "r.txt"},
		{"PyPI", "ruff", "0.17.3", "r.txt"},
		{"PyPI", "uvicorn", "0.40.0", "r.txt"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v", got)
	}
	if !reflect.DeepEqual(unpinned, []string{"r.txt: chromadb>=1.5.9,<2.0.0"}) {
		t.Errorf("unpinned = %q", unpinned)
	}
}

func TestReadSBOMJSON(t *testing.T) {
	got, err := ReadSBOMJSON(open(t, "fixture.sbom-json"), "s.json")
	if err != nil {
		t.Fatal(err)
	}
	want := []Component{
		{"Go", "example.test/alpha", "1.2.3", "s.json"},
		{"crates.io", "serde", "1.0.230", "s.json"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v", got)
	}
	if _, err := ReadSBOMJSON(strings.NewReader(`{"packages":[{"version":"1"}]}`), "s.json"); err == nil {
		t.Error("a package with no name was accepted")
	}
}

func TestScanTree(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module m\n\nrequire example.test/a v1.0.0\n")
	write("sub/requirements.txt", "ruff==0.17.3\nchromadb>=1\n")
	write("other/requirements-dev.txt", "ruff==0.17.3\n")
	write("sbom/python-pip-freeze.txt", "tqdm==4.70.1\n")
	write("testdata/go.mod", "module x\n\nrequire example.test/skipped v1.0.0\n")
	write("target/Cargo.lock", "[[package]]\nname = \"skipped\"\nversion = \"1\"\nsource = \"registry+x\"\n")
	comps, unpinned, err := ScanTree(root)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, c := range comps {
		names = append(names, c.Ecosystem+"/"+c.Name+"@"+c.Version)
	}
	want := []string{"Go/example.test/a@1.0.0", "PyPI/ruff@0.17.3", "PyPI/tqdm@4.70.1"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("components = %q", names)
	}
	if !reflect.DeepEqual(unpinned, []string{"sub/requirements.txt: chromadb>=1"}) {
		t.Errorf("unpinned = %q", unpinned)
	}
}
