// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package crosswalk

import (
	"errors"
	"strings"
	"testing"
	"time"
)

var (
	srcSSH   = Source{Kind: KindHostSSHD, Ref: "east:passwordauthentication=no"}
	srcSys   = Source{Kind: KindHostSysctl, Ref: "west:fs.suid_dumpable=0"}
	srcCI    = Source{Kind: KindGitHubJob, Ref: "W/scan"}
	srcAtt   = Source{Kind: KindAttestation, Ref: "compliance/attestations/policy.yaml"}
	fxSource = map[string]Source{"ssh": srcSSH, "sys": srcSys, "ci": srcCI, "att": srcAtt}
)

func findingsCatalog(t *testing.T) *Catalog {
	t.Helper()
	c, err := Load([]byte(`
frameworks:
  - id: fw
    name: FW
    requirements: [{id: R1}]
`), []byte(`
controls:
  - id: UH-SSH-01
    title: t
    statement: s
    freshness_days: 7
    evidence: [{kind: host-sshd, ref: "east:passwordauthentication=no"}, {kind: host-sysctl, ref: "west:fs.suid_dumpable=0"}]
    mappings: {fw: [R1]}
  - id: UH-CI-01
    title: t
    statement: s
    freshness_days: 7
    evidence: [{kind: github-job, ref: "W/scan"}, {kind: host-sysctl, ref: "west:fs.suid_dumpable=0"}]
    mappings: {fw: [R1]}
  - id: UH-POL-01
    title: t
    statement: s
    freshness_days: 365
    evidence: [{kind: attestation, ref: "compliance/attestations/policy.yaml"}]
    mappings: {fw: [R1]}
`))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

const regSSH = `
  - source: "host-sshd:east:passwordauthentication=no"
    severity: medium
    class: sshd
    opened: 2026-09-01
    exposure: LAN only
    lockout: high
    step: 5
    plan: prove key login, then disable
`

func loadReg(t *testing.T, body string) *Register {
	t.Helper()
	r, err := LoadRegister([]byte("findings:"+body), findingsCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestLoadRegister_Rejects(t *testing.T) {
	cat := findingsCatalog(t)
	base := func(s string) string { return "findings:\n  - " + s }
	for _, tc := range []struct{ name, yaml string }{
		{"unknown field", base(`{source: "host-sshd:east:passwordauthentication=no", severity: low, class: sshd, opened: 2026-09-01, lockout: none, step: 1, owner: x}`)},
		{"source not in catalog", base(`{source: "host-sshd:east:gone=no", severity: low, class: sshd, opened: 2026-09-01, lockout: none, step: 1}`)},
		{"both source and id", base(`{source: "host-sshd:east:passwordauthentication=no", id: FND-001, title: t, verify: v, severity: low, class: sshd, opened: 2026-09-01, lockout: none, step: 1}`)},
		{"neither source nor id", base(`{severity: low, class: sshd, opened: 2026-09-01, lockout: none, step: 1}`)},
		{"bad id", base(`{id: FND-1, title: t, verify: v, severity: low, class: app, opened: 2026-09-01, lockout: none, step: 1}`)},
		{"manual without verify", base(`{id: FND-001, title: t, severity: low, class: app, opened: 2026-09-01, lockout: none, step: 1}`)},
		{"manual without title", base(`{id: FND-001, verify: v, severity: low, class: app, opened: 2026-09-01, lockout: none, step: 1}`)},
		{"bad severity", base(`{id: FND-001, title: t, verify: v, severity: urgent, class: app, opened: 2026-09-01, lockout: none, step: 1}`)},
		{"untriaged is not a severity you may write", base(`{id: FND-001, title: t, verify: v, severity: untriaged, class: app, opened: 2026-09-01, lockout: none, step: 1}`)},
		{"bad class", base(`{id: FND-001, title: t, verify: v, severity: low, class: vibes, opened: 2026-09-01, lockout: none, step: 1}`)},
		{"bad lockout", base(`{id: FND-001, title: t, verify: v, severity: low, class: app, opened: 2026-09-01, lockout: maybe, step: 1}`)},
		{"bad opened", base(`{id: FND-001, title: t, verify: v, severity: low, class: app, opened: 30/09/2026, lockout: none, step: 1}`)},
		{"step 0", base(`{id: FND-001, title: t, verify: v, severity: low, class: app, opened: 2026-09-01, lockout: none, step: 0}`)},
		{"duplicate id", "findings:\n" +
			`  - {id: FND-001, title: t, verify: v, severity: low, class: app, opened: 2026-09-01, lockout: none, step: 1}` + "\n" +
			`  - {id: FND-001, title: u, verify: v, severity: low, class: app, opened: 2026-09-01, lockout: none, step: 1}`},
		{"duplicate source", "findings:" + regSSH + regSSH},
		{"resolved on a machine finding", base(`{source: "host-sshd:east:passwordauthentication=no", severity: low, class: sshd, opened: 2026-09-01, lockout: none, step: 1, resolved: {on: 2026-09-02, evidence: e}}`)},
		{"resolved without evidence", base(`{id: FND-001, title: t, verify: v, severity: low, class: app, opened: 2026-09-01, lockout: none, step: 1, resolved: {on: 2026-09-02}}`)},
		{"acceptance longer than 90 days", base(`{id: FND-001, title: t, verify: v, severity: low, class: app, opened: 2026-09-01, lockout: none, step: 1,
      accepted: {reason: r, compensating: c, accepted_on: 2026-09-01, until: 2026-12-31}}`)},
		{"acceptance without compensating control", base(`{id: FND-001, title: t, verify: v, severity: low, class: app, opened: 2026-09-01, lockout: none, step: 1,
      accepted: {reason: r, accepted_on: 2026-09-01, until: 2026-10-01}}`)},
		{"acceptance ending before it starts", base(`{id: FND-001, title: t, verify: v, severity: low, class: app, opened: 2026-09-01, lockout: none, step: 1,
      accepted: {reason: r, compensating: c, accepted_on: 2026-10-01, until: 2026-09-01}}`)},
		{"not a list", "findings: {}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := LoadRegister([]byte(tc.yaml), cat); !errors.Is(err, ErrInvalidRegister) {
				t.Errorf("err = %v, want ErrInvalidRegister", err)
			}
		})
	}
	if _, err := LoadRegister(make([]byte, MaxCatalogBytes+1), cat); !errors.Is(err, ErrInvalidRegister) {
		t.Errorf("oversize: err = %v", err)
	}
	if _, err := LoadRegister([]byte("findings: []"), nil); !errors.Is(err, ErrInvalidRegister) {
		t.Errorf("nil catalog: err = %v", err)
	}
}

func TestFindings_CountsFailuresAndTriage(t *testing.T) {
	cat := findingsCatalog(t)
	day := 24 * time.Hour
	reg := loadReg(t, regSSH+`
  - id: FND-001
    title: alerts route to a null receiver
    verify: alertmanager.yml has a real receiver
    severity: high
    class: app
    opened: 2026-09-30
    lockout: none
    step: 1
`)
	recs := []Record{
		rec(srcSSH, VerdictFail, day),
		rec(srcSys, VerdictPass, 2*day), rec(srcSys, VerdictFail, day), // latest fails: untriaged
		rec(srcCI, VerdictPass, day),
		rec(srcAtt, VerdictPass, day),
		rec(Source{Kind: KindHostSysctl, Ref: "fs.suid_dumpable=0"}, VerdictFail, day), // not in catalog: ignored
	}
	r := Findings(cat, recs, reg, now)

	if got := len(r.Open); got != 3 {
		t.Fatalf("open = %d, want 3 (ssh, sysctl, FND-001): %+v", got, r.Open)
	}
	// Untriaged first (it needs a decision before anything else), then by severity.
	if r.Open[0].Key != srcSys.Key() || r.Open[1].Key != "FND-001" || r.Open[2].Key != srcSSH.Key() {
		t.Errorf("order = %s, %s, %s", r.Open[0].Key, r.Open[1].Key, r.Open[2].Key)
	}
	sys := r.Open[0]
	if sys.Severity != SeverityUntriaged || sys.Entry != nil {
		t.Errorf("sysctl finding = %+v, want untriaged", sys)
	}
	if strings.Join(sys.Controls, ",") != "UH-CI-01,UH-SSH-01" || sys.Host != "west" {
		t.Errorf("sysctl controls/host = %v %q", sys.Controls, sys.Host)
	}
	ssh := r.Open[2]
	if ssh.Host != "east" || !ssh.Due.Equal(time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC)) || ssh.Overdue {
		t.Errorf("ssh finding host %q due %v overdue %v (opened 2026-09-01 + 90d)", ssh.Host, ssh.Due, ssh.Overdue)
	}
	if r.Counts[SeverityHigh] != 1 || r.Counts[SeverityMedium] != 1 || r.Counts[SeverityUntriaged] != 1 {
		t.Errorf("counts = %v", r.Counts)
	}
	if r.ZeroClaimable {
		t.Error("zero claimed with open findings")
	}
}

func TestFindings_ResolvedByEvidenceNotByEdit(t *testing.T) {
	cat := findingsCatalog(t)
	reg := loadReg(t, regSSH+`
  - id: FND-002
    title: t
    verify: v
    severity: low
    class: app
    opened: 2026-09-01
    lockout: none
    step: 1
    resolved: {on: 2026-09-29, evidence: "commit abc: receiver configured"}
`)
	recs := []Record{
		rec(srcSSH, VerdictFail, 48*time.Hour), rec(srcSSH, VerdictPass, time.Hour),
		rec(srcSys, VerdictPass, time.Hour), rec(srcCI, VerdictPass, time.Hour), rec(srcAtt, VerdictPass, time.Hour),
	}
	r := Findings(cat, recs, reg, now)
	if len(r.Open) != 0 {
		t.Fatalf("open = %+v", r.Open)
	}
	if strings.Join(r.Resolved, "|") != srcSSH.Key()+"|FND-002" {
		t.Errorf("resolved = %v: a passing source's entry is reported for removal, a manual one by its evidence", r.Resolved)
	}
	if !r.ZeroClaimable {
		t.Errorf("zero not claimable: %s", r.ZeroReason)
	}
}

func TestFindings_ZeroNeedsEveryCheckObservedAndFresh(t *testing.T) {
	cat := findingsCatalog(t)
	reg := loadReg(t, "")
	day := 24 * time.Hour

	r := Findings(cat, []Record{rec(srcSSH, VerdictPass, day), rec(srcSys, VerdictPass, day), rec(srcCI, VerdictPass, day)}, reg, now)
	if r.ZeroClaimable || strings.Join(r.Unobserved, ",") != srcAtt.Key() {
		t.Errorf("missing attestation: zero=%v unobserved=%v; NOT_ASSESSED is not a pass", r.ZeroClaimable, r.Unobserved)
	}

	// srcSys backs a 7-day control; 8 days old is stale even though UH-POL-01 would allow a year.
	r = Findings(cat, []Record{rec(srcSSH, VerdictPass, day), rec(srcSys, VerdictPass, 8*day),
		rec(srcCI, VerdictPass, day), rec(srcAtt, VerdictPass, 300*day)}, reg, now)
	if r.ZeroClaimable || strings.Join(r.Stale, ",") != srcSys.Key() {
		t.Errorf("stale: zero=%v stale=%v", r.ZeroClaimable, r.Stale)
	}

	if r := Findings(cat, nil, reg, now); r.ZeroClaimable || len(r.Unobserved) != 4 {
		t.Errorf("no evidence at all: zero=%v unobserved=%v", r.ZeroClaimable, r.Unobserved)
	}
	future := []Record{rec(srcSSH, VerdictFail, -time.Hour)}
	if r := Findings(cat, future, reg, now); len(r.Open) != 0 {
		t.Errorf("a future-dated record is not evidence: %+v", r.Open)
	}
}

func TestFindings_AcceptedStillCountsAndExpires(t *testing.T) {
	cat := findingsCatalog(t)
	accepted := strings.Replace(regSSH, "plan:", "accepted: {reason: r, compensating: LAN only, accepted_on: 2026-09-01, until: 2026-09-29}\n    plan:", 1)
	reg := loadReg(t, accepted)
	r := Findings(cat, []Record{rec(srcSSH, VerdictFail, time.Hour)}, reg, now)
	if len(r.Open) != 1 || !r.Open[0].Accepted || !r.Open[0].AcceptanceExpired {
		t.Fatalf("open = %+v, want one accepted, expired", r.Open)
	}
	if r.Accepted != 1 || r.Counts[SeverityMedium] != 1 {
		t.Errorf("accepted=%d counts=%v: acceptance is counted, never subtracted", r.Accepted, r.Counts)
	}

	live := strings.Replace(accepted, "until: 2026-09-29", "until: 2026-10-15", 1)
	r = Findings(cat, []Record{rec(srcSSH, VerdictFail, time.Hour)}, loadReg(t, live), now)
	if r.Open[0].AcceptanceExpired {
		t.Error("acceptance until 2026-10-15 reported expired on 2026-09-30")
	}
}

func TestFindings_Overdue(t *testing.T) {
	cat := findingsCatalog(t)
	old := strings.Replace(regSSH, "opened: 2026-09-01", "opened: 2026-05-01", 1)
	r := Findings(cat, []Record{rec(srcSSH, VerdictFail, time.Hour)}, loadReg(t, old), now)
	if !r.Open[0].Overdue || r.Overdue != 1 {
		t.Errorf("opened 2026-05-01, medium (90d): overdue=%v total=%d", r.Open[0].Overdue, r.Overdue)
	}
}

func TestFindings_NilRegisterIsAllUntriaged(t *testing.T) {
	r := Findings(findingsCatalog(t), []Record{rec(srcSSH, VerdictFail, time.Hour)}, nil, now)
	if len(r.Open) != 1 || r.Open[0].Severity != SeverityUntriaged {
		t.Errorf("open = %+v", r.Open)
	}
}

func TestHostOf(t *testing.T) {
	for src, want := range map[Source]string{
		srcSSH: "east",
		srcSys: "west",
		{Kind: KindHostSysctl, Ref: "fs.suid_dumpable=0"}: "local",
		{Kind: KindHostProbe, Ref: "east:auditd-running"}: "east",
		srcCI:  "",
		srcAtt: "",
	} {
		if got := hostOf(src); got != want {
			t.Errorf("hostOf(%s) = %q, want %q", src.Key(), got, want)
		}
	}
}
