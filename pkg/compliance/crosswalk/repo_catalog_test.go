// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package crosswalk

import (
	"os"
	"testing"
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
		"nist-800-53r5":    1014, // Rev 5.2.0, withdrawn excluded
		"fedramp-low":      156,
		"fedramp-moderate": 323,
		"fedramp-high":     410,
		"nist-csf-2":       106,
		"nist-800-171r3":   97,
		"nist-ssdf":        19,
		"soc2-security":    33,
		"iso27001-2022":    93,
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
