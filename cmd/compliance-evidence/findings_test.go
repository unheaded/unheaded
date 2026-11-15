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
}

func TestRunFindings_WritesOut(t *testing.T) {
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
		"-out", "var/compliance/findings.md"}, &out)
	if err != nil {
		t.Fatal(err)
	}
	report, err := os.ReadFile(filepath.Join(root, "var/compliance/findings.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(report), "east: auditd inactive") || !strings.Contains(string(report), "**1 open**") {
		t.Errorf("report not written:\n%s", report)
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

func TestRunFindings_RepoPathRefused(t *testing.T) {
	repo := t.TempDir()
	for rel, body := range map[string]string{
		"compliance/catalog/frameworks/fw.yaml": "frameworks:\n  - id: fw\n    name: FW\n    requirements: [{id: R1}]\n",
		"compliance/catalog/controls.yaml":      "controls:\n  - id: UH-X-01\n    title: t\n    statement: s\n    freshness_days: 7\n    evidence: [{kind: github-job, ref: W/x}]\n    mappings: {fw: [R1]}\n",
		"compliance/findings/register.yaml":     "findings: []\n",
		"docs/adr/ADR-X.md":                     "# X\n",
	} {
		p := filepath.Join(repo, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var out strings.Builder
	if err := runFindings([]string{"-repo-dir", repo, "-out", "docs/adr/ADR-X.md"}, &out); err == nil {
		t.Error("findings wrote into the repository tree")
	}
	if b, _ := os.ReadFile(filepath.Join(repo, "docs/adr/ADR-X.md")); string(b) != "# X\n" {
		t.Errorf("a tracked doc was rewritten:\n%s", b)
	}
	if err := runFindings([]string{"-repo-dir", repo, "-adr", "docs/adr/ADR-X.md"}, &out); err == nil {
		t.Error("the removed -adr flag was accepted")
	}
}

// ADR-098 condition 3: a host may be flipped to enforcing only with zero open
// findings in the classes enforcing acts on. -enforce-ready lists them.
func TestRenderEnforceReady(t *testing.T) {
	at := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	entry := func(class string) *crosswalk.RegisterEntry {
		return &crosswalk.RegisterEntry{Severity: crosswalk.SeverityMedium, Class: class, Opened: "2026-11-01",
			Lockout: "none", Step: 3, Plan: "fix | it"}
	}
	open := func(key, host, class string) crosswalk.Finding {
		return crosswalk.Finding{Key: key, Host: host, Detail: host + ": " + key, Severity: crosswalk.SeverityMedium,
			Entry: entry(class)}
	}

	// Every class with an enforcing action counts; every other class does not.
	enforceable := []string{"sysctl", "service", "sshd", "firewall", "kmod", "fileperm", "sudo", "shell"}
	alertOnly := []string{"mount", "package", "boot", "mac", "account", "app", "container", "ci", "backup", "process"}
	rep := &crosswalk.FindingsReport{At: at}
	for _, c := range enforceable {
		rep.Open = append(rep.Open, open("host-probe:east:"+c+"-check", "east", c))
	}
	for _, c := range alertOnly {
		rep.Open = append(rep.Open, open("host-probe:east:"+c+"-check", "east", c))
	}
	out := renderEnforceReady(rep, "east")
	if !strings.HasSuffix(out, "ready: no (8)\n") {
		t.Errorf("want the last line \"ready: no (8)\":\n%s", out)
	}
	for _, c := range enforceable {
		if !strings.Contains(out, "`host-probe:east:"+c+"-check`") {
			t.Errorf("class %s has an enforcing action and is not listed:\n%s", c, out)
		}
		if !crosswalk.IsEnforceableClass(c) || !crosswalk.IsFindingClass(c) {
			t.Errorf("class %s: enforceable %v, finding class %v", c, crosswalk.IsEnforceableClass(c), crosswalk.IsFindingClass(c))
		}
	}
	for _, c := range alertOnly {
		if strings.Contains(out, "`host-probe:east:"+c+"-check`") {
			t.Errorf("class %s has no enforcing action and is listed:\n%s", c, out)
		}
		if crosswalk.IsEnforceableClass(c) {
			t.Errorf("class %s is alert-only in ADR-098 and reads enforceable", c)
		}
	}
	if !strings.Contains(out, `fix \| it`) {
		t.Errorf("a pipe in a plan must not split the table row:\n%s", out)
	}

	// Another host's findings, an accepted row, an untriaged row and a row
	// that names no host.
	accepted := open("host-sshd:west:x11forwarding=no", "west", "sshd")
	accepted.Accepted = true
	accepted.Entry.Accepted = &crosswalk.Acceptance{Until: "2027-02-11"}
	rep = &crosswalk.FindingsReport{At: at, Open: []crosswalk.Finding{
		open("host-probe:west:firewall-inbound-deny", "west", "firewall"),
		accepted,
		open("host-probe:west:var-mount", "west", "mount"),
		open("host-probe:east:firewall-inbound-deny", "east", "firewall"),
		{Key: "host-sysctl:west:x=0", Host: "west", Detail: "west: x = 1", Severity: crosswalk.SeverityUntriaged},
		{Key: "host-sysctl:east:y=0", Host: "east", Detail: "east: y = 1", Severity: crosswalk.SeverityUntriaged},
		open("FND-010", "", "service"),
		open("FND-099", "", "app"),
		open("github-job:CI/build", "", "ci"),
	}}
	out = renderEnforceReady(rep, "west")
	for _, want := range []string{
		"`host-probe:west:firewall-inbound-deny`",
		"`host-sshd:west:x11forwarding=no`", // accepted rows are still counted
		"accepted until 2027-02-11",
		"`host-sysctl:west:x=0`", // no class yet: it cannot be shown to be outside the table
		"untriaged",
		"`FND-010`", // names no host, so it may be this one
		"no host named",
		"firewall 1",
		"sshd 1",
		"service 1",
		"untriaged 1",
		"As of 2026-11-15T12:00:00Z",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("west: missing %q\n%s", want, out)
		}
	}
	for _, not := range []string{"east", "var-mount", "FND-099", "github-job"} {
		if strings.Contains(out, not) {
			t.Errorf("west: %q does not belong in the list\n%s", not, out)
		}
	}
	if !strings.HasSuffix(out, "ready: no (4)\n") {
		t.Errorf("west: want the last line \"ready: no (4)\":\n%s", out)
	}

	// Nothing open in the table for the host: ready, even with other classes open.
	rep = &crosswalk.FindingsReport{At: at, Open: []crosswalk.Finding{
		open("host-probe:west:var-mount", "west", "mount"),
		open("host-probe:east:firewall-inbound-deny", "east", "firewall"),
		open("FND-099", "", "app"),
	}}
	out = renderEnforceReady(rep, "west")
	if !strings.HasSuffix(out, "ready: yes\n") {
		t.Errorf("want the last line \"ready: yes\":\n%s", out)
	}
}

func TestRunFindings_EnforceReady(t *testing.T) {
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
    evidence:
      - {kind: host-probe, ref: "east:auditd-running"}
      - {kind: host-probe, ref: "east:var-mount"}
      - {kind: host-probe, ref: "west:auditd-running"}
    mappings: {fw: [R1]}
`)
	write("compliance/findings/register.yaml", `findings:
  - source: "host-probe:east:auditd-running"
    severity: medium
    class: service
    opened: 2026-09-30
    lockout: none
    step: 3
  - source: "host-probe:east:var-mount"
    severity: low
    class: mount
    opened: 2026-09-30
    lockout: none
    step: 3
`)
	write("compliance/hosts/east.yaml", "host: east\nprofile: lab\nmode: audit\n")
	write("compliance/hosts/west.yaml", "host: west\nprofile: lab\nmode: audit\n")
	f, err := os.Create(filepath.Join(root, "evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	seen := time.Now().Add(-time.Hour)
	if err := crosswalk.WriteEvidence(f, []crosswalk.Record{
		{Source: crosswalk.Source{Kind: crosswalk.KindHostProbe, Ref: "east:auditd-running"},
			Verdict: crosswalk.VerdictFail, ObservedAt: seen, Detail: "east: auditd inactive"},
		{Source: crosswalk.Source{Kind: crosswalk.KindHostProbe, Ref: "east:var-mount"},
			Verdict: crosswalk.VerdictFail, ObservedAt: seen, Detail: "east: /var is not a mount"},
		{Source: crosswalk.Source{Kind: crosswalk.KindHostProbe, Ref: "west:auditd-running"},
			Verdict: crosswalk.VerdictPass, ObservedAt: seen, Detail: "west: auditd active"},
	}); err != nil {
		t.Fatal(err)
	}
	f.Close()
	args := func(extra ...string) []string {
		return append([]string{"-repo-dir", root, "-evidence", filepath.Join(root, "evidence.json")}, extra...)
	}

	// Not ready is an answer, not a failure: the command exits 0.
	var out strings.Builder
	if err := runFindings(args("-enforce-ready", "east"), &out); err != nil {
		t.Fatalf("east: %v", err)
	}
	if !strings.HasSuffix(out.String(), "ready: no (1)\n") || !strings.Contains(out.String(), "east: auditd inactive") {
		t.Errorf("east:\n%s", out.String())
	}
	if strings.Contains(out.String(), "var-mount") || strings.Contains(out.String(), "**2 open**") {
		t.Errorf("east: the whole report, or a mount row, was printed:\n%s", out.String())
	}
	out.Reset()
	if err := runFindings(args("-enforce-ready", "west"), &out); err != nil {
		t.Fatalf("west: %v", err)
	}
	if !strings.HasSuffix(out.String(), "ready: yes\n") {
		t.Errorf("west:\n%s", out.String())
	}

	// The answer is host data: -out sends it under var/ like the report.
	out.Reset()
	if err := runFindings(args("-enforce-ready", "east", "-out", "var/compliance/enforce-ready-east.txt"), &out); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(root, "var/compliance/enforce-ready-east.txt"))
	if err != nil || string(b) != out.String() {
		t.Errorf("-out: %v\n%s", err, b)
	}

	// A host with no file in compliance/hosts is a usage error that names
	// the hosts there are, and prints no verdict.
	for _, h := range []string{"nowhere", "", "../east", "east.yaml", "EAST"} {
		out.Reset()
		err := runFindings(args("-enforce-ready", h), &out)
		if err == nil {
			t.Errorf("host %q accepted:\n%s", h, out.String())
			continue
		}
		if h != "" && (!strings.Contains(err.Error(), "east") || !strings.Contains(err.Error(), "west")) {
			t.Errorf("host %q: the error does not name the known hosts: %v", h, err)
		}
		if strings.Contains(out.String(), "ready:") {
			t.Errorf("host %q: a verdict was printed:\n%s", h, out.String())
		}
	}
}
