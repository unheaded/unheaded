// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package crosswalk

import (
	"errors"
	"strings"
	"testing"
)

const testFrameworks = `
frameworks:
  - id: soc2
    name: SOC 2 Trust Services Criteria
    version: "2017 (rev. 2022)"
    granularity: common criteria
    source: https://example.invalid/tsc
    requirements:
      - id: CC7.1
      - id: CC8.1
  - id: ssdf
    name: NIST SP 800-218
    version: "1.1"
    granularity: practices
    source: https://example.invalid/ssdf
    requirements:
      - id: PW.7
      - id: RV.1
`

const testControls = `
controls:
  - id: UH-VULN-01
    title: Go dependency vulnerability scanning gates every push
    statement: govulncheck runs on every push to protected branches and fails the build on a reachable vulnerability.
    freshness_days: 7
    evidence:
      - kind: github-job
        ref: "Security Scan (Daily + PR)/Go Vulnerability Check"
    mappings:
      soc2: [CC7.1]
      ssdf: [RV.1]
`

func TestLoad_Valid(t *testing.T) {
	c, err := Load([]byte(testFrameworks), []byte(testControls))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(c.Frameworks) != 2 || len(c.Controls) != 1 {
		t.Fatalf("got %d frameworks, %d controls", len(c.Frameworks), len(c.Controls))
	}
	ctl := c.Controls[0]
	if got := ctl.Mappings["soc2"]; len(got) != 1 || got[0] != "CC7.1" {
		t.Errorf("soc2 mapping = %v", got)
	}
	if fw := c.Framework("ssdf"); fw == nil || len(fw.Requirements) != 2 {
		t.Errorf("ssdf framework = %+v", fw)
	}
}

func TestLoad_Rejects(t *testing.T) {
	for _, tc := range []struct {
		name       string
		frameworks string
		controls   string
		want       string
	}{
		{"unknown framework in mapping", testFrameworks,
			strings.Replace(testControls, "ssdf: [RV.1]", "iso27001: [A.8.8]", 1), "unknown framework"},
		{"requirement not in framework", testFrameworks,
			strings.Replace(testControls, "ssdf: [RV.1]", "ssdf: [PW.99]", 1), "not a requirement of"},
		{"duplicate control id", testFrameworks,
			testControls + strings.Replace(testControls, "controls:\n", "", 1), "duplicate control"},
		{"duplicate framework id", testFrameworks + strings.Replace(testFrameworks, "frameworks:\n", "", 1),
			testControls, "duplicate framework"},
		{"duplicate requirement id", strings.Replace(testFrameworks, "      - id: CC8.1", "      - id: CC7.1", 1),
			testControls, "duplicate requirement"},
		{"no evidence", testFrameworks,
			strings.Replace(testControls, `      - kind: github-job
        ref: "Security Scan (Daily + PR)/Go Vulnerability Check"
`, "", 1), "no evidence"},
		{"unknown evidence kind", testFrameworks,
			strings.Replace(testControls, "kind: github-job", "kind: vibes", 1), "evidence kind"},
		{"empty evidence ref", testFrameworks,
			strings.Replace(testControls, `ref: "Security Scan (Daily + PR)/Go Vulnerability Check"`, `ref: ""`, 1), "empty ref"},
		{"no mappings", testFrameworks,
			strings.Replace(strings.Replace(testControls, "      soc2: [CC7.1]\n", "", 1), "      ssdf: [RV.1]\n", "", 1), "maps to no requirement"},
		{"freshness zero", testFrameworks,
			strings.Replace(testControls, "freshness_days: 7", "freshness_days: 0", 1), "freshness_days"},
		{"freshness too long", testFrameworks,
			strings.Replace(testControls, "freshness_days: 7", "freshness_days: 400", 1), "freshness_days"},
		{"bad control id", testFrameworks,
			strings.Replace(testControls, "id: UH-VULN-01", "id: vuln 1", 1), "control id"},
		{"framework without requirements", "frameworks:\n  - id: x\n    name: X\n    requirements: []\n",
			testControls, "no requirements"},
		{"unknown field", testFrameworks,
			strings.Replace(testControls, "freshness_days: 7", "freshness_days: 7\n    satisfied: true", 1), "field satisfied not found"},
		{"not yaml", "frameworks: [", testControls, "frameworks"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load([]byte(tc.frameworks), []byte(tc.controls))
			if err == nil {
				t.Fatalf("Load accepted it; want error containing %q", tc.want)
			}
			if !errors.Is(err, ErrInvalidCatalog) {
				t.Errorf("error %v does not wrap ErrInvalidCatalog", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

func TestLoad_SizeLimit(t *testing.T) {
	big := make([]byte, MaxCatalogBytes+1)
	if _, err := Load(big, []byte(testControls)); !errors.Is(err, ErrInvalidCatalog) {
		t.Fatalf("oversized frameworks accepted: %v", err)
	}
	if _, err := Load([]byte(testFrameworks), big); !errors.Is(err, ErrInvalidCatalog) {
		t.Fatalf("oversized controls accepted: %v", err)
	}
}

func FuzzLoad(f *testing.F) {
	f.Add([]byte(testFrameworks), []byte(testControls))
	f.Add([]byte("frameworks: []"), []byte("controls: []"))
	f.Fuzz(func(t *testing.T, fw, ctl []byte) {
		c, err := Load(fw, ctl)
		if (c == nil) == (err == nil) {
			t.Fatalf("Load = %v, %v: want exactly one of catalog or error", c, err)
		}
		if err != nil {
			return
		}
		// A catalog that loaded must be internally consistent.
		for _, ctl := range c.Controls {
			for fwID, reqs := range ctl.Mappings {
				fw := c.Framework(fwID)
				if fw == nil {
					t.Fatalf("control %s maps to unknown framework %q", ctl.ID, fwID)
				}
				for _, r := range reqs {
					if !fw.Has(r) {
						t.Fatalf("control %s maps to %s/%s, not in the framework", ctl.ID, fwID, r)
					}
				}
			}
		}
	})
}

const derivedFrameworks = `
frameworks:
  - id: base
    name: Base
    requirements: [{id: RA-5}, {id: SI-2}, {id: AC-2}]
  - id: baseline-low
    name: Baseline Low
    derived_from: base
    requirements: [{id: RA-5}, {id: AC-2}]
`

// Attest once: a mapping to the base framework projects onto every derived
// baseline that contains the same requirement, with no second mapping.
func TestLoad_DerivedFrameworkProjection(t *testing.T) {
	c, err := Load([]byte(derivedFrameworks), []byte(`
controls:
  - id: UH-VULN-01
    title: t
    statement: s
    freshness_days: 7
    evidence: [{kind: gate-script, ref: a.sh}]
    mappings: {base: [RA-5, SI-2]}
`))
	if err != nil {
		t.Fatal(err)
	}
	got := c.Controls[0].Mappings["baseline-low"]
	if len(got) != 1 || got[0] != "RA-5" {
		t.Fatalf("derived mapping = %v, want [RA-5] (SI-2 is not in the low baseline)", got)
	}
}

func TestLoad_DerivedFrameworkRejects(t *testing.T) {
	ctl := func(m string) string {
		return "controls:\n  - id: UH-VULN-01\n    title: t\n    statement: s\n    freshness_days: 7\n" +
			"    evidence: [{kind: gate-script, ref: a.sh}]\n    mappings: {" + m + "}\n"
	}
	for _, tc := range []struct{ name, fw, ctl, want string }{
		{"direct mapping to a derived framework", derivedFrameworks, ctl("baseline-low: [RA-5]"), "derived"},
		{"derived requirement missing from base",
			strings.Replace(derivedFrameworks, "[{id: RA-5}, {id: AC-2}]", "[{id: RA-5}, {id: ZZ-9}]", 1),
			ctl("base: [RA-5]"), "not in its base"},
		{"unknown base", strings.Replace(derivedFrameworks, "derived_from: base", "derived_from: nope", 1),
			ctl("base: [RA-5]"), "unknown base"},
		{"base is itself derived", derivedFrameworks + "  - id: deeper\n    name: D\n    derived_from: baseline-low\n    requirements: [{id: RA-5}]\n",
			ctl("base: [RA-5]"), "itself derived"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load([]byte(tc.fw), []byte(tc.ctl))
			if !errors.Is(err, ErrInvalidCatalog) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want ErrInvalidCatalog containing %q", err, tc.want)
			}
		})
	}
}
