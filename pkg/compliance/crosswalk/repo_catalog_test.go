// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package crosswalk

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The catalog in the repo loads, and each framework keeps the publisher's
// requirement count: a regenerated or hand-edited file that drops or adds
// requirements changes every coverage denominator, so it fails here first.
func TestRepoCatalog(t *testing.T) {
	c, err := LoadFS(os.DirFS("../../.."), "compliance/catalog/frameworks", "compliance/catalog/controls.yaml")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{
		"nist-800-53r5":     1014, // Rev 5.2.0, withdrawn excluded
		"fedramp-low":       156,
		"fedramp-moderate":  323,
		"fedramp-high":      410,
		"nist-csf-2":        106,
		"nist-800-171r3":    97,
		"nist-ssdf":         19,
		"soc2-security":     33,
		"iso27001-2022":     93,
		"cis-ubuntu2404":    312, // v1.0.0 Level 2 Server (includes Level 1), ComplianceAsCode transcription
		"cis-ubuntu2404-l1": 251, // Level 1 Server
	}
	for id, n := range want {
		fw := c.Framework(id)
		if fw == nil {
			t.Errorf("framework %s missing", id)
			continue
		}
		if len(fw.Requirements) != n {
			t.Errorf("%s: %d requirements, want %d", id, len(fw.Requirements), n)
		}
	}
	if len(c.Frameworks) != len(want) {
		t.Errorf("%d frameworks, want %d (add new ones to this table)", len(c.Frameworks), len(want))
	}
	if len(c.Controls) == 0 {
		t.Error("no controls")
	}
}

// uncorroborated lists mappings that knowingly disagree with NIST's own CSF
// informative references, each with the reason. An entry that NIST's
// references come to support must be removed (the test says so).
var uncorroborated = map[string]string{
	"UH-GATE-01/iso27001-2022": "NIST pairs ID.IM-02 with A.5.35 (independent review) and A.5.19; " +
		"the meta-gate is an internal check that the security gates work, which is A.5.36 " +
		"(compliance with the organisation's own security rules), not an independent review.",
	"UH-PATCH-01/iso27001-2022": "NIST pairs PR.PS-02 with A.8.7 (malware) and A.5.9 (inventory); " +
		"automatic security updates are patch deployment, A.8.8 (management of technical vulnerabilities).",
}

// Every control mapped to a CSF 2.0 subcategory must be corroborated, on
// its 800-53 and ISO 27001 mappings, by NIST's informative references for
// that subcategory (compliance/catalog/references, generated from NIST's
// CSF 2.0 Reference Tool). A mapping only Unheaded believes is marked, not
// hidden.
func TestMappingsCorroborated(t *testing.T) {
	c, err := LoadFS(os.DirFS("../../.."), "compliance/catalog/frameworks", "compliance/catalog/controls.yaml")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../../compliance/catalog/references/nist-csf2-informative.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var refs struct {
		Subcategories map[string]struct {
			SP80053 []string `yaml:"sp800_53"`
			ISO     []string `yaml:"iso27001_2022"`
		} `yaml:"subcategories"`
	}
	if err := yaml.Unmarshal(raw, &refs); err != nil {
		t.Fatal(err)
	}
	base := func(id string) string { b, _, _ := strings.Cut(id, "("); return b }
	agrees := func(mine, nist []string, loose bool) bool {
		for _, m := range mine {
			for _, n := range nist {
				if m == n || (loose && base(m) == base(n)) {
					return true
				}
			}
		}
		return false
	}
	used := map[string]bool{}
	for _, ctl := range c.Controls {
		for _, sub := range ctl.Mappings["nist-csf-2"] {
			r, ok := refs.Subcategories[sub]
			if !ok {
				t.Errorf("%s: %s has no NIST references entry", ctl.ID, sub)
				continue
			}
			for fw, check := range map[string]struct {
				nist  []string
				loose bool
			}{
				"nist-800-53r5": {r.SP80053, true}, // RA-5(2) is corroborated by RA-5
				"iso27001-2022": {r.ISO, false},
			} {
				mine := ctl.Mappings[fw]
				if len(mine) == 0 {
					continue
				}
				key := ctl.ID + "/" + fw
				ok := agrees(mine, check.nist, check.loose)
				switch reason, excused := uncorroborated[key]; {
				case !ok && !excused:
					t.Errorf("%s maps %s %v, but NIST's references for %s list %v: fix the mapping or record why in uncorroborated", ctl.ID, fw, mine, sub, check.nist)
				case ok && excused:
					t.Errorf("%s: now corroborated by NIST; remove its uncorroborated entry (%q)", key, reason)
				case excused:
					used[key] = true
				}
			}
		}
	}
	for key := range uncorroborated {
		if !used[key] {
			t.Errorf("uncorroborated entry %s matches no mapping; remove it", key)
		}
	}
}

// The findings register in the repo loads against the catalog in the repo:
// every entry names a source some control still declares (ADR-098).
func TestRepoRegister(t *testing.T) {
	c, err := LoadFS(os.DirFS("../../.."), "compliance/catalog/frameworks", "compliance/catalog/controls.yaml")
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile("../../../compliance/findings/register.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRegister(b, c); err != nil {
		t.Fatal(err)
	}
}
