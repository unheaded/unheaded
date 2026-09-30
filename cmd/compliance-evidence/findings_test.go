// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/compliance/crosswalk"
)

func TestReplaceGenerated(t *testing.T) {
	doc := "# ADR\n\ntext\n\n" + beginMarker + "\nold table\n" + endMarker + "\n\ntrailer\n"
	got, err := replaceGenerated(doc, "new | table\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "# ADR\n\ntext\n\n" + beginMarker + "\nnew | table\n" + endMarker + "\n\ntrailer\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	again, err := replaceGenerated(got, "new | table\n")
	if err != nil || again != got {
		t.Errorf("not idempotent: %v\n%s", err, again)
	}

	for name, bad := range map[string]string{
		"no markers":             "# ADR\n",
		"only begin":             beginMarker + "\n",
		"end before":             endMarker + "\n" + beginMarker + "\n",
		"begin twice":            beginMarker + "\n" + beginMarker + "\n" + endMarker + "\n",
		"end twice":              beginMarker + "\n" + endMarker + "\n" + endMarker + "\n",
		"generated has a marker": beginMarker + "\n" + endMarker + "\n",
	} {
		body := "x\n"
		if name == "generated has a marker" {
			body = endMarker + "\n"
		}
		if _, err := replaceGenerated(bad, body); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestRenderFindings(t *testing.T) {
	at := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	entry := &crosswalk.RegisterEntry{Severity: crosswalk.SeverityHigh, Class: "firewall", Opened: "2026-09-30",
		Lockout: "high", Step: 6, Plan: "nft | commit-confirm", Decision: "console access?"}
	rep := &crosswalk.FindingsReport{
		At: at,
		Open: []crosswalk.Finding{
			{Key: "host-probe:east:firewall-inbound-deny", Controls: []string{"UH-FW-01"}, Host: "east",
				Detail: "east: INPUT policy ACCEPT", Severity: crosswalk.SeverityHigh, Entry: entry,
				Due: at.AddDate(0, 0, 30)},
			{Key: "host-sysctl:west:x=0", Controls: []string{"UH-HOST-01"}, Host: "west",
				Detail: "west: x = 1", Severity: crosswalk.SeverityUntriaged},
		},
		Counts:     map[crosswalk.Severity]int{crosswalk.SeverityHigh: 1, crosswalk.SeverityUntriaged: 1},
		Resolved:   []string{"host-sshd:west:x11forwarding=no"},
		Unobserved: []string{"attestation:compliance/attestations/access-review.yaml"},
		Stale:      []string{},
		ZeroReason: "2 open, 1 never observed",
	}
	out := renderFindings(rep)
	for _, want := range []string{
		"**2 open**",
		"zero claimable: **no** (2 open, 1 never observed)",
		"1 high",
		"1 untriaged",
		"`host-probe:east:firewall-inbound-deny`",
		`nft \| commit-confirm`, // a pipe in text must not split the table row
		"2026-10-30",
		"**decision:** console access?",
		"UNTRIAGED",
		"remove from the register: `host-sshd:west:x11forwarding=no`",
		"`attestation:compliance/attestations/access-review.yaml`",
		"As of 2026-09-30T12:00:00Z",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("render missing %q\n%s", want, out)
		}
	}
	if strings.Contains(out, beginMarker) || strings.Contains(out, endMarker) {
		t.Error("render contains a marker")
	}
}

func TestRunFindings_WritesADR(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("compliance/catalog/frameworks/fw.yaml", "frameworks:\n  - id: fw\n    name: FW\n    requirements: [{id: R1}]\n")
	write("compliance/catalog/controls.yaml", `controls:
  - id: UH-X-01
    title: t
    statement: s
    freshness_days: 7
    evidence: [{kind: host-probe, ref: "east:auditd-running"}]
    mappings: {fw: [R1]}
`)
	write("compliance/findings/register.yaml", `findings:
  - source: "host-probe:east:auditd-running"
    severity: medium
    class: service
    opened: 2026-09-30
    lockout: none
    step: 3
`)
	write("docs/adr/ADR-X.md", "# X\n"+beginMarker+"\n"+endMarker+"\n")
	f, err := os.Create(filepath.Join(root, "evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := crosswalk.WriteEvidence(f, []crosswalk.Record{{
		Source:  crosswalk.Source{Kind: crosswalk.KindHostProbe, Ref: "east:auditd-running"},
		Verdict: crosswalk.VerdictFail, ObservedAt: time.Now().Add(-time.Hour), Detail: "east: auditd inactive",
	}}); err != nil {
		t.Fatal(err)
	}
	f.Close()

	var out strings.Builder
	err = runFindings([]string{"-repo-dir", root, "-evidence", filepath.Join(root, "evidence.json"),
		"-adr", "docs/adr/ADR-X.md"}, &out)
	if err != nil {
		t.Fatal(err)
	}
	adr, err := os.ReadFile(filepath.Join(root, "docs/adr/ADR-X.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(adr), "east: auditd inactive") || !strings.Contains(string(adr), "**1 open**") {
		t.Errorf("ADR not regenerated:\n%s", adr)
	}
	if !strings.Contains(out.String(), "**1 open**") {
		t.Errorf("stdout:\n%s", out.String())
	}

	// A register that names a source the catalog does not declare stops the run.
	write("compliance/findings/register.yaml", "findings:\n  - {source: \"host-probe:east:gone\", severity: low, class: service, opened: 2026-09-30, lockout: none, step: 1}\n")
	if err := runFindings([]string{"-repo-dir", root, "-evidence", filepath.Join(root, "evidence.json")}, &out); err == nil {
		t.Error("invalid register accepted")
	}
	// No evidence file: everything unobserved, not an error, and zero is not claimable.
	write("compliance/findings/register.yaml", "findings: []\n")
	out.Reset()
	if err := runFindings([]string{"-repo-dir", root, "-evidence", filepath.Join(root, "missing.json")}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "zero claimable: **no** (1 never observed)") {
		t.Errorf("no evidence:\n%s", out.String())
	}
}
