// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package kev

import (
	"encoding/json"
	"errors"
	"testing"
)

// FuzzParse: the catalog arrives from the network. Parse must never panic,
// must reject with ErrInvalid (nothing else), and anything it accepts must
// hold the invariants Diff and Store rely on.
func FuzzParse(f *testing.F) {
	f.Add([]byte(`{"catalogVersion":"1","count":1,"vulnerabilities":[{"cveID":"CVE-2024-0001"}]}`))
	f.Add([]byte(`{"catalogVersion":"1","count":2,"vulnerabilities":[{"cveID":"CVE-2024-0001"},{"cveID":"CVE-2024-0001"}]}`))
	f.Add([]byte(`[]`))
	f.Add([]byte(`{"catalogVersion":"x","count":1,"vulnerabilities":[{"cveID":"CVE-2024-0001","cwes":["CWE-79"]}]} {}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		c, err := Parse(data)
		if err != nil {
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("error not wrapped in ErrInvalid: %v", err)
			}
			return
		}
		if c.CatalogVersion == "" || c.Count != len(c.Vulnerabilities) || c.Count == 0 {
			t.Fatalf("accepted a catalog that breaks the invariants: %+v", c)
		}
		seen := map[string]bool{}
		for _, v := range c.Vulnerabilities {
			if !cveIDPattern.MatchString(v.CVEID) || seen[v.CVEID] {
				t.Fatalf("accepted cveID %q", v.CVEID)
			}
			seen[v.CVEID] = true
		}
		if d := Diff(c, c); !d.Empty() {
			t.Fatalf("a catalog differs from itself: %+v", d)
		}
		if _, err := json.Marshal(c); err != nil {
			t.Fatalf("accepted catalog does not re-encode: %v", err)
		}
	})
}

// FuzzParseStack: a stack file must never panic the parser, and one it
// accepts must have named categories, each with a vendor in every rule.
func FuzzParseStack(f *testing.F) {
	f.Add([]byte("categories:\n  - {name: a, match: [{vendor: Linux, product: Kernel}]}\n"))
	f.Add([]byte("categories: []"))
	f.Add([]byte("categories:\n  - name: a\n    match:\n      - {vendor: X, prodcut: Y}\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		s, err := ParseStack(data)
		if err != nil {
			return
		}
		for _, c := range s.Categories {
			if c.Name == "" || len(c.Match) == 0 {
				t.Fatalf("accepted category %+v", c)
			}
			for _, r := range c.Match {
				if r.Vendor == "" {
					t.Fatalf("accepted a rule with no vendor in %s", c.Name)
				}
			}
		}
		_ = s.Classify(Vulnerability{VendorProject: "Linux", Product: "Kernel"})
	})
}
