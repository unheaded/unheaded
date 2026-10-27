// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update", false, "rewrite testdata/ssp/SSP.golden.md")

// sspArgs runs the fixture catalog (3 controls, 2 frameworks) at a fixed
// evaluation date, so the output is a pure function of the fixture.
func sspArgs(out string, extra ...string) []string {
	return append([]string{"-repo-dir", "testdata/ssp", "-catalog", "controls.yaml", "-frameworks", "frameworks",
		"-evidence", "evidence.json", "-at", "2026-10-27", "-out", out}, extra...)
}

func renderFixture(t *testing.T, extra ...string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "SSP.md")
	if err := runSSP(sspArgs(out, extra...), io.Discard); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSSPGolden(t *testing.T) {
	got := renderFixture(t)
	golden := filepath.Join("testdata", "ssp", "SSP.golden.md")
	if *updateGolden {
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("SSP differs from %s (go test -run TestSSPGolden -update to accept):\n%s", golden, got)
	}
}

func TestSSPDeterministic(t *testing.T) {
	a, b := renderFixture(t), renderFixture(t)
	if !bytes.Equal([]byte(a), []byte(b)) {
		t.Fatal("two runs on the same inputs differ")
	}
}

func TestSSPContent(t *testing.T) {
	got := renderFixture(t)
	lines := strings.Split(got, "\n")
	count := func(pred func(string) bool) int {
		n := 0
		for _, l := range lines {
			if pred(l) {
				n++
			}
		}
		return n
	}
	if n := count(func(l string) bool { return strings.HasPrefix(l, "### UH-") }); n != 3 {
		t.Errorf("%d control sections, want 3", n)
	}
	// One line per failing control and no other line says FAIL, so
	// `grep -c FAIL` on the SSP equals the collector's FAIL rows.
	if n := count(func(l string) bool { return strings.Contains(l, "FAIL") }); n != 1 {
		t.Errorf("%d lines contain FAIL, want 1 (UH-FX-02 only)", n)
	}
	for _, want := range []string{
		"`compliance/policies/risk-management-strategy.md`",
		"`compliance/policies/single-operator-posture.md`",
		"**Status:** PASS, evidence as of 2026-10-25",      // weakest link: check-a's pass, not develop's later one
		"**Status:** FAIL, evidence as of 2026-10-24",      // the 2026-10-28 pass is in the future
		"**Status:** NOT_ASSESSED, no evidence for 1 of 1", // attestation never signed
		"- `gate-script:scripts/check-a.sh`: pass 2026-10-25",
		"- `attestation:compliance/attestations/policy.yaml`: no record",
		"- fx-a: A-1, A-2",
		"- fx-b: B.1",
		"## SoA",
		"| fx-a | Fixture Framework A | 1 | controls | 4 | 2 | 1 | 0 | 1 |",
		"| fx-b | Fixture Framework B | 2 | criteria | 2 | 1 | 0 | 1 | 0 |",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if strings.Contains(got, "INPUT policy") || strings.Contains(got, "0123456789abcdef") {
		t.Error("record details or commits leaked into the SSP")
	}
	// Mappings come out in framework-id order, whatever the YAML order.
	if strings.Index(got, "- fx-a: A-1, A-2") > strings.Index(got, "- fx-b: B.1") {
		t.Error("mappings not sorted by framework id")
	}
}

func TestSSPWithoutEvidenceFile(t *testing.T) {
	got := renderFixture(t, "-evidence", "absent.json")
	if n := strings.Count(got, "**Status:** NOT_ASSESSED"); n != 3 {
		t.Errorf("%d NOT_ASSESSED controls without evidence, want 3", n)
	}
	if strings.Contains(got, "FAIL") {
		t.Error("FAIL without evidence")
	}
}

func TestSSPRejects(t *testing.T) {
	out := filepath.Join(t.TempDir(), "SSP.md")
	for name, args := range map[string][]string{
		"bad date":       sspArgs(out, "-at", "27/10/2026"),
		"no out":         {"-repo-dir", "testdata/ssp", "-catalog", "controls.yaml", "-frameworks", "frameworks"},
		"bad evidence":   sspArgs(out, "-evidence", "controls.yaml"),
		"missing ctl":    sspArgs(out, "-catalog", "nope.yaml"),
		"stray argument": append(sspArgs(out), "extra"),
	} {
		if err := runSSP(args, io.Discard); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := os.Stat(out); err == nil {
		t.Error("a rejected run wrote the output file")
	}
}
