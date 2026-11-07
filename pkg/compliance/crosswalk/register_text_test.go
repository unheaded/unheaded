// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package crosswalk

import (
	"os"
	"testing"
)

// repoRegister loads the register in the repo against the repo's catalog.
func repoRegister(t *testing.T) map[string]*RegisterEntry {
	t.Helper()
	cat, err := LoadFS(os.DirFS("../../.."), "compliance/catalog/frameworks", "compliance/catalog/controls.yaml")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../../compliance/findings/register.yaml")
	if err != nil {
		t.Fatal(err)
	}
	reg, err := LoadRegister(raw, cat)
	if err != nil {
		t.Fatal(err)
	}
	byKey := make(map[string]*RegisterEntry, len(reg.Entries))
	for _, e := range reg.Entries {
		byKey[e.Key()] = e
	}
	return byKey
}

// FND-004 closed on a statement while its verify asked for a test from
// outside. It stays open until that test exists, and no resolved block closes
// it without one.
func TestRegister_FND004StaysOpenUntilTestedFromOutside(t *testing.T) {
	e := repoRegister(t)["FND-004"]
	if e == nil {
		t.Fatal("FND-004 is not in the register")
	}
	if e.Resolved != nil {
		t.Errorf("FND-004 has a resolved block: %+v", e.Resolved)
	}
	if e.Severity != SeverityHigh {
		t.Errorf("FND-004 severity %q, want high", e.Severity)
	}
}
