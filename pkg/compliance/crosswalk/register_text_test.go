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
// these words is rewritten, not excused. Case does not matter.
func TestRegister_TextStaysGeneral(t *testing.T) {
	banned := []string{
		"passwordless", "nopasswd", "writable by", "without a token", "auth_enabled", "shared account",
		"listeners on all addresses", "from the wire", "input accept", "i.e. root",
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
		folded := strings.ToLower(strings.Join(strings.Fields(text), " "))
		for _, b := range banned {
			if strings.Contains(folded, b) {
				t.Errorf("%s says %q", rel, b)
			}
		}
	}
}

// The same rule for what an open entry says about today's state. A list of
// words cannot read for meaning, so this holds the phrases a sweep of the open
// entries on 2026-11-07 rewrote; resolved entries are history and keep theirs.
func TestRegister_OpenEntriesStayGeneral(t *testing.T) {
	banned := []string{
		"reach nobody", "reach anyone", "tells nobody", "goes unnoticed", "nothing alerted", "not by any alert",
		"plain http", "readable by anyone", "reachable from the lan", "no default-deny", "no rules",
		"unprivileged user namespaces", "user-namespace setting", "root-on-west", "fail-open", "chaos guard", "wlp",
	}
	for key, e := range repoRegister(t) {
		if e.Resolved != nil {
			continue
		}
		text := strings.ToLower(strings.Join(strings.Fields(strings.Join([]string{e.Title, e.Exposure, e.Plan, e.Decision}, " ")), " "))
		for _, b := range banned {
			if strings.Contains(text, b) {
				t.Errorf("%s says %q", key, b)
			}
		}
	}
}

// History is not rewritten, so FND-038's verify must name a window it can
// meet, and it keeps the attestation half of the finding: sign-attestation.sh
// reads the git identity, which an agent session shares.
func TestRegister_FND038CanBeMetAndKeepsAttestation(t *testing.T) {
	e := repoRegister(t)["FND-038"]
	if e == nil {
		t.Fatal("FND-038 is not in the register")
	}
	verify := strings.Join(strings.Fields(e.Verify), " ")
	if !strings.Contains(verify, "7a93b734..develop") {
		t.Errorf("verify names no window: %q", verify)
	}
	if strings.Contains(verify, "every agent commit on develop") {
		t.Errorf("verify asks for all of history, which is not being rewritten: %q", verify)
	}
	for field, text := range map[string]string{"title": e.Title, "verify": e.Verify} {
		if !strings.Contains(text, "sign-attestation.sh") {
			t.Errorf("%s drops sign-attestation.sh: %q", field, text)
		}
	}
}

// A row with two ways out says both, until a ruling picks one.
func TestRegister_SudoDecisionKeepsBothOptions(t *testing.T) {
	reg := repoRegister(t)
	for _, key := range []string{"host-probe:west:sudo-password-required", "host-probe:east:sudo-password-required"} {
		e := reg[key]
		if e == nil {
			t.Fatalf("%s is not in the register", key)
		}
		if !strings.Contains(e.Decision, "keep it") || !strings.Contains(e.Decision, "narrow it") {
			t.Errorf("%s decision %q", key, e.Decision)
		}
	}
}

// FND-033's verify counts on four classes; ADR-125 is what lists them.
func TestRegister_FND033PointsAtADR125(t *testing.T) {
	e := repoRegister(t)["FND-033"]
	if e == nil {
		t.Fatal("FND-033 is not in the register")
	}
	if !strings.Contains(strings.Join(strings.Fields(e.Verify), " "), "the four ADR-125 lists") {
		t.Errorf("verify %q", e.Verify)
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
