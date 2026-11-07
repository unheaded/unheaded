// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package crosswalk

import (
	"os"
	"strings"
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

// The repository is public. A register entry says what is guaranteed, what is
// open and what closes it, not how a gap could be used. A row that needs one of
// these words is rewritten, not excused.
func TestRegister_TextStaysGeneral(t *testing.T) {
	banned := []string{
		"passwordless", "writable by", "without a token", "AUTH_ENABLED", "shared account",
		"listeners on all addresses", "from the wire", "INPUT ACCEPT", "i.e. root",
	}
	// ADR-098's generated section prints the register's text.
	for _, rel := range []string{"compliance/findings/register.yaml", "docs/adr/ADR-098-findings-register-and-baseline-modes.md"} {
		raw, err := os.ReadFile("../../../" + rel)
		if err != nil {
			t.Fatal(err)
		}
		text := string(raw)
		if _, gen, ok := strings.Cut(text, "<!-- BEGIN GENERATED FINDINGS -->"); ok {
			text = gen
		}
		folded := strings.Join(strings.Fields(text), " ")
		for _, b := range banned {
			if strings.Contains(folded, b) {
				t.Errorf("%s says %q", rel, b)
			}
		}
	}
}

// Facts the register holds about the repository itself, each open until its
// verify is met.
func TestRegister_RepositoryRowsAreOpen(t *testing.T) {
	reg := repoRegister(t)
	for id, want := range map[string]string{
		"FND-041": "public",
		"FND-042": "branch protection",
	} {
		e := reg[id]
		if e == nil {
			t.Errorf("%s is not in the register", id)
			continue
		}
		if e.Resolved != nil || !strings.Contains(strings.ToLower(e.Title), want) {
			t.Errorf("%s: resolved %v, title %q, want open and about %q", id, e.Resolved, e.Title, want)
		}
	}
}

// FND-025's checks moved to hop-ebpf (ADR-125), so a verify naming shield-ebpf
// could never be met.
func TestRegister_FND025VerifyNamesHopEBPF(t *testing.T) {
	e := repoRegister(t)["FND-025"]
	if e == nil {
		t.Fatal("FND-025 is not in the register")
	}
	if !strings.Contains(e.Verify, "hop-ebpf") || strings.Contains(e.Verify, "cmd/shield") ||
		strings.Contains(e.Verify, "with shield-ebpf attached") {
		t.Errorf("FND-025 verify: %q", e.Verify)
	}
}
