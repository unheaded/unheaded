// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package osv

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompare(t *testing.T) {
	cases := []struct {
		eco, a, b string
		want      int
	}{
		{EcoGo, "v0.61.0", "0.62.0", -1},
		{EcoGo, "0.62.0", "v0.62.0", 0},
		{EcoGo, "1.2.3-rc.1", "1.2.3", -1},
		{EcoGo, "1.2.3-rc.2", "1.2.3-rc.10", -1},
		{EcoGo, "1.2.3-alpha", "1.2.3-1", 1},
		{EcoGo, "1.0.0+incompatible", "1.0.0", 0},
		{EcoGo, "0.58.1-0.20261009181718-2f1f834330b2", "0.58.0", 1},
		{EcoCrates, "0.10.3", "0.8.8", 1},
		{EcoPyPI, "5.20.2", "5.8.1", 1},
		{EcoPyPI, "1.0.dev1", "1.0a1", -1},
		{EcoPyPI, "1.0a1", "1.0b1", -1},
		{EcoPyPI, "1.0rc1", "1.0", -1},
		{EcoPyPI, "1.0", "1.0.post1", -1},
		{EcoPyPI, "1.0a1.dev1", "1.0a1", -1},
		{EcoPyPI, "1!0.5", "2.0", 1},
		{EcoPyPI, "2.14.1+cpu", "2.14.1", 0},
		{EcoPyPI, "5.0.0rc3", "5.0.0", -1},
	}
	for _, c := range cases {
		got, err := Compare(c.eco, c.a, c.b)
		if err != nil {
			t.Fatalf("Compare(%s, %s, %s): %v", c.eco, c.a, c.b, err)
		}
		if sign(got) != c.want {
			t.Errorf("Compare(%s, %s, %s) = %d, want %d", c.eco, c.a, c.b, got, c.want)
		}
	}
	if _, err := Compare("npm", "1", "2"); !errors.Is(err, ErrInvalid) {
		t.Errorf("unsupported ecosystem: err = %v, want ErrInvalid", err)
	}
	if _, err := Compare(EcoPyPI, "not a version", "1"); !errors.Is(err, ErrInvalid) {
		t.Errorf("bad version: err = %v, want ErrInvalid", err)
	}
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
}

func TestParseRejects(t *testing.T) {
	bad := map[string]string{
		"not json":      `{`,
		"trailing":      `{"id":"A-1","affected":[{"package":{"name":"x","ecosystem":"Go"}}]} {}`,
		"bad id":        `{"id":"a b","affected":[{"package":{"name":"x","ecosystem":"Go"}}]}`,
		"no affected":   `{"id":"A-1"}`,
		"no package":    `{"id":"A-1","affected":[{"package":{"name":"","ecosystem":"Go"}}]}`,
		"two-field evt": `{"id":"A-1","affected":[{"package":{"name":"x","ecosystem":"Go"},"ranges":[{"type":"SEMVER","events":[{"introduced":"0","fixed":"1"}]}]}]}`,
	}
	for name, in := range bad {
		if _, err := Parse([]byte(in)); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
	if _, err := Parse(make([]byte, MaxRecordBytes+1)); !errors.Is(err, ErrInvalid) {
		t.Errorf("oversize: err = %v, want ErrInvalid", err)
	}
}

func TestMatches(t *testing.T) {
	a := Affected{
		Package: Package{Name: "Sentence_Transformers", Ecosystem: EcoPyPI},
		Ranges: []Range{
			{Type: "GIT", Events: []Event{{Introduced: "0"}}},
			{Type: "ECOSYSTEM", Events: []Event{{Introduced: "2.0"}, {Fixed: "5.6.0"}}},
		},
	}
	b := Affected{
		Package: Package{Name: "chromadb", Ecosystem: EcoPyPI},
		Ranges:  []Range{{Type: "ECOSYSTEM", Events: []Event{{LastAffected: "1.5.9"}, {Introduced: "0.4.17"}}}},
	}
	c := Affected{Package: Package{Name: "golang.org/x/net", Ecosystem: EcoGo}, Versions: []string{"0.61.0"}}
	d := Affected{
		Package: Package{Name: "golang.org/x/net", Ecosystem: EcoGo},
		Ranges:  []Range{{Type: "SEMVER", Events: []Event{{Introduced: "0"}, {Fixed: "0.62.0"}}}},
	}
	cases := []struct {
		a         Affected
		eco, n, v string
		want      bool
	}{
		{a, EcoPyPI, "sentence-transformers", "2.7.0", true},
		{a, EcoPyPI, "sentence-transformers", "5.6.0", false},
		{a, EcoPyPI, "sentence-transformers", "1.9", false},
		{a, EcoGo, "sentence-transformers", "2.7.0", false},
		{b, EcoPyPI, "chromadb", "1.5.9", true},
		{b, EcoPyPI, "chromadb", "1.5.10", false},
		{b, EcoPyPI, "chromadb", "0.4.16", false},
		{c, EcoGo, "golang.org/x/net", "v0.61.0", true},
		{c, EcoGo, "golang.org/x/net", "v0.62.0", false},
		{d, EcoGo, "golang.org/x/net", "v0.61.0", true},
		{d, EcoGo, "golang.org/x/net", "v0.62.0", false},
	}
	for i, tc := range cases {
		got, err := tc.a.Matches(tc.eco, tc.n, tc.v)
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		if got != tc.want {
			t.Errorf("case %d (%s %s): got %v, want %v", i, tc.n, tc.v, got, tc.want)
		}
	}
	if got := d.FirstFixed(EcoGo); got != "0.62.0" {
		t.Errorf("FirstFixed = %q", got)
	}
}

func TestLoadDir(t *testing.T) {
	dir := t.TempDir()
	rec := `{"id":"A-1","affected":[{"package":{"name":"x","ecosystem":"Go"}}]}`
	if _, err := LoadDir(dir); !errors.Is(err, ErrInvalid) {
		t.Errorf("empty dir: err = %v, want ErrInvalid", err)
	}
	for _, n := range []string{"a.json", "b.json"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(rec), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := LoadDir(dir); err == nil || !strings.Contains(err.Error(), "duplicate id A-1") {
		t.Errorf("duplicate: err = %v", err)
	}
	if err := os.Remove(filepath.Join(dir, "b.json")); err != nil {
		t.Fatal(err)
	}
	vs, err := LoadDir(dir)
	if err != nil || len(vs) != 1 || vs[0].ID != "A-1" {
		t.Errorf("LoadDir = %v, %v", vs, err)
	}
}
