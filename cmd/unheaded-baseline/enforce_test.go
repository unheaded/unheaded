// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/compliance/remediation"
	"unheaded/pkg/compliance/snapshot"
	"unheaded/pkg/compliance/well"
	"unheaded/pkg/gungnir"
)

// fakeEnforcer applies nothing; it records applies and answers the breaker
// from them, as the executor answers from its journal.
type fakeEnforcer struct {
	applied []time.Time
	ids     []string
	now     func() time.Time
	err     error
}

func (f *fakeEnforcer) Apply(_ context.Context, fix remediation.Fix) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.applied = append(f.applied, f.now())
	f.ids = append(f.ids, fix.ID)
	return "/var/lib/unheaded/remediate/x-" + fix.ID, nil
}

func (f *fakeEnforcer) AppliesSince(fixID string, since time.Time) (int, error) {
	n := 0
	for i, at := range f.applied {
		if f.ids[i] == fixID && !at.Before(since) {
			n++
		}
	}
	return n, nil
}

// testModel maps east's fs.suid_dumpable deviation to one fix; the other
// two deviations of the fixture (auditd, maxauthtries) map to none.
func testModel() *remediation.Model {
	return &remediation.Model{Fixes: []remediation.Fix{{ID: "sysctl-cis", Title: "t", Controls: []string{"UH-HOST-01"},
		Closes: []string{"host-sysctl:*:fs.suid_dumpable=0"}, Risk: "none", Class: "sysctl",
		Actions: []remediation.Action{{Run: "sysctl --system"}}}}}
}

type enfFixture struct {
	*fixture
	enf *fakeEnforcer
	now time.Time
}

func newEnfFixture(t *testing.T, mode string) *enfFixture {
	t.Helper()
	f := &enfFixture{fixture: newFixture(t), now: time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)}
	f.enf = &fakeEnforcer{now: func() time.Time { return f.now }}
	f.opts.now = func() time.Time { return f.now }
	f.opts.loadModel = func(string) (*remediation.Model, error) { return testModel(), nil }
	f.opts.newEnforcer = func(context.Context, string, io.Writer) (enforcer, error) { return f.enf, nil }
	f.opts.remediateState = filepath.Join(f.dir, "remediate")
	if err := os.MkdirAll(f.opts.remediateState, 0o700); err != nil {
		t.Fatal(err)
	}
	f.setMode(t, mode)
	return f
}

func (f *enfFixture) setMode(t *testing.T, mode string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(f.opts.configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.opts.configPath, []byte("mode="+mode+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// soakClean runs audit with nothing to enforce (no model mapping) so the
// soak clock starts, then moves the clock past the soak.
func (f *enfFixture) soakClean(t *testing.T, days int) {
	t.Helper()
	f.opts.loadModel = func(string) (*remediation.Model, error) { return &remediation.Model{}, nil }
	f.setMode(t, "audit")
	if _, err := f.run(t); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(time.Duration(days) * 24 * time.Hour)
	if _, err := f.run(t); err != nil {
		t.Fatal(err)
	}
	f.opts.loadModel = func(string) (*remediation.Model, error) { return testModel(), nil }
}

func byKey(rep *report) map[string]check {
	m := map[string]check{}
	for _, c := range rep.Checks {
		m[c.Key] = c
	}
	return m
}

func TestAudit_MappedDeviationIsWouldEnforceAndResetsTheSoak(t *testing.T) {
	f := newEnfFixture(t, "audit")
	rep, err := f.run(t)
	if err != nil {
		t.Fatal(err)
	}
	if rep.ModeEffective != modeAudit || rep.WouldEnforce != 1 || rep.SoakCleanSince != nil || len(f.enf.ids) != 0 {
		t.Errorf("rep %+v applies %v", rep, f.enf.ids)
	}
	if c := byKey(rep)["host-sysctl:east:fs.suid_dumpable=0"]; c.Fix != "sysctl-cis" {
		t.Errorf("deviation not mapped to its fix: %+v", c)
	}
	if !strings.Contains(f.logBuf.String(), `WOULD_ENFORCE host=east key="host-sysctl:east:fs.suid_dumpable=0" fix=sysctl-cis`) {
		t.Errorf("log:\n%s", f.logBuf.String())
	}
	prom, _ := os.ReadFile(filepath.Join(f.opts.textfileDir, "unheaded_baseline.prom"))
	for _, want := range []string{"unheaded_baseline_would_enforce 1", "unheaded_baseline_would_enforce_total 1",
		"unheaded_baseline_soak_clean_seconds 0", "unheaded_baseline_enforcing 0"} {
		if !strings.Contains(string(prom), want) {
			t.Errorf("textfile lacks %q:\n%s", want, prom)
		}
	}
	// The counter keeps counting across runs.
	if _, err := f.run(t); err != nil {
		t.Fatal(err)
	}
	prom, _ = os.ReadFile(filepath.Join(f.opts.textfileDir, "unheaded_baseline.prom"))
	if !strings.Contains(string(prom), "unheaded_baseline_would_enforce_total 2") {
		t.Errorf("counter did not accumulate:\n%s", prom)
	}
}

func TestEnforcing_NeedsTheSoak(t *testing.T) {
	f := newEnfFixture(t, "enforcing")
	rep, err := f.run(t)
	if err != nil {
		t.Fatal(err)
	}
	if rep.ModeConfig != modeEnforcing || rep.ModeEffective != modeAudit || len(f.enf.ids) != 0 {
		t.Fatalf("flipped without a soak: %+v applies %v", rep, f.enf.ids)
	}
	if !strings.Contains(f.logBuf.String(), "CRITICAL enforcing requested") || !strings.Contains(f.logBuf.String(), "soak") {
		t.Errorf("log:\n%s", f.logBuf.String())
	}
	// Six clean days are not seven.
	f.soakClean(t, 6)
	f.setMode(t, "enforcing")
	if rep, _ := f.run(t); rep.ModeEffective != modeAudit || len(f.enf.ids) != 0 {
		t.Errorf("flipped after six days: %+v", rep)
	}
}

func TestEnforcing_AfterTheSoakAppliesOnlyMappedFixes(t *testing.T) {
	f := newEnfFixture(t, "enforcing")
	f.soakClean(t, 7)
	f.setMode(t, "enforcing")
	rep, err := f.run(t)
	if err != nil {
		t.Fatal(err)
	}
	if rep.ModeEffective != modeEnforcing || strings.Join(f.enf.ids, ",") != "sysctl-cis" {
		t.Fatalf("rep %+v applies %v\n%s", rep, f.enf.ids, f.logBuf.String())
	}
	c := byKey(rep)
	if c["host-sysctl:east:fs.suid_dumpable=0"].Enforced == "" || c["host-probe:east:auditd-running"].Enforced != "" {
		t.Errorf("checks: %+v", rep.Checks)
	}
	// The two unmapped deviations stay audit and are left over.
	if rep.Enforced != 1 || rep.CriticalDeviations != 2 {
		t.Errorf("enforced %d critical_deviations %d, want 1 and 2", rep.Enforced, rep.CriticalDeviations)
	}
	// scripts/lib/deb-maintscripts.sh's preinst gate reads these two lines as written.
	raw, _ := os.ReadFile(filepath.Join(f.opts.stateDir, "report.json"))
	if !strings.Contains(string(raw), "\n  \"mode_effective\": \"enforcing\",\n") || !strings.Contains(string(raw), "\n  \"critical_deviations\": 2,\n") {
		t.Errorf("report.json lacks the lines the preinst gate reads:\n%s", raw)
	}
	log := f.logBuf.String()
	if !strings.Contains(log, "ENFORCED host=east") || !strings.Contains(log, "fix=sysctl-cis") {
		t.Errorf("log:\n%s", log)
	}
	prom, _ := os.ReadFile(filepath.Join(f.opts.textfileDir, "unheaded_baseline.prom"))
	if !strings.Contains(string(prom), "unheaded_baseline_enforcing 1") || !strings.Contains(string(prom), "unheaded_baseline_enforced_total 1") {
		t.Errorf("textfile:\n%s", prom)
	}
}

// The same fix applied again and again means something keeps undoing it:
// the third apply inside 24 h is not made, and the fix drops to audit.
func TestEnforcing_BreakerTripsOnTheThirdApply(t *testing.T) {
	f := newEnfFixture(t, "enforcing")
	f.soakClean(t, 7)
	f.setMode(t, "enforcing")
	for i := 0; i < 3; i++ {
		f.logBuf.Reset()
		rep, err := f.run(t)
		if err != nil {
			t.Fatal(err)
		}
		if i < 2 && len(f.enf.ids) != i+1 {
			t.Fatalf("run %d: applies %v", i+1, f.enf.ids)
		}
		if i == 2 {
			if len(f.enf.ids) != 2 {
				t.Fatalf("third apply was made: %v", f.enf.ids)
			}
			if !strings.Contains(f.logBuf.String(), "CRITICAL") || !strings.Contains(f.logBuf.String(), "breaker") ||
				len(rep.CriticalEvents) == 0 || byKey(rep)["host-sysctl:east:fs.suid_dumpable=0"].Enforced != "breaker open" {
				t.Errorf("breaker not reported: %+v\n%s", rep, f.logBuf.String())
			}
			prom, _ := os.ReadFile(filepath.Join(f.opts.textfileDir, "unheaded_baseline.prom"))
			if !strings.Contains(string(prom), `unheaded_baseline_breaker_open{fix="sysctl-cis"} 1`) {
				t.Errorf("textfile:\n%s", prom)
			}
		}
		f.now = f.now.Add(time.Hour)
	}
	// A day later the breaker closes again.
	f.now = f.now.Add(25 * time.Hour)
	if _, err := f.run(t); err != nil || len(f.enf.ids) != 3 {
		t.Errorf("after 24 h: %v %v", f.enf.ids, err)
	}
}

func TestEnforcing_MissingJournalDirFailsLoud(t *testing.T) {
	f := newEnfFixture(t, "enforcing")
	f.soakClean(t, 7)
	f.setMode(t, "enforcing")
	if err := os.RemoveAll(f.opts.remediateState); err != nil {
		t.Fatal(err)
	}
	_, err := f.run(t)
	if err == nil || !strings.Contains(err.Error(), "journal") || !strings.Contains(f.logBuf.String(), "CRITICAL") {
		t.Fatalf("err %v log %s", err, f.logBuf.String())
	}
	if len(f.enf.ids) != 0 {
		t.Error("applied without a journal")
	}
	if _, err := os.Stat(f.opts.remediateState); !errors.Is(err, os.ErrNotExist) {
		t.Error("created the journal directory instead of failing")
	}
}

func TestEnforcing_FailedApplyIsCritical(t *testing.T) {
	f := newEnfFixture(t, "enforcing")
	f.soakClean(t, 7)
	f.setMode(t, "enforcing")
	f.enf.err = errors.New("fix refused: apply needs root")
	rep, err := f.run(t)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Enforced != 0 || rep.CriticalDeviations != 3 || len(rep.CriticalEvents) == 0 {
		t.Errorf("%+v", rep)
	}
}

func TestNoModel_NoSoakNoEnforcing(t *testing.T) {
	f := newEnfFixture(t, "audit")
	f.opts.loadModel = func(string) (*remediation.Model, error) { return nil, errors.New("no snapshot") }
	for i := 0; i < 2; i++ {
		rep, err := f.run(t)
		if err != nil {
			t.Fatal(err)
		}
		if rep.SoakCleanSince != nil {
			t.Fatal("the soak clock ran without a fix model to judge what enforcing would do")
		}
		f.now = f.now.Add(8 * 24 * time.Hour)
	}
	f.setMode(t, "enforcing")
	if rep, _ := f.run(t); rep.ModeEffective != modeAudit {
		t.Error("enforcing without a model")
	}
}

func TestTamper_ReportEditedBetweenRunsIsCritical(t *testing.T) {
	f := newEnfFixture(t, "enforcing")
	f.soakClean(t, 7)
	p := filepath.Join(f.opts.stateDir, "report.json")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var r map[string]any
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatal(err)
	}
	r["deviations"] = 0
	forged, _ := json.Marshal(r)
	if err := os.WriteFile(p, forged, 0o644); err != nil {
		t.Fatal(err)
	}
	f.setMode(t, "enforcing")
	f.logBuf.Reset()
	rep, err := f.run(t)
	if err != nil {
		t.Fatal(err)
	}
	if rep.ModeEffective != modeAudit || rep.SoakCleanSince != nil || len(f.enf.ids) != 0 {
		t.Errorf("a forged report did not reset the soak: %+v", rep)
	}
	if !strings.Contains(f.logBuf.String(), "CRITICAL") || !strings.Contains(f.logBuf.String(), "report.json") {
		t.Errorf("log:\n%s", f.logBuf.String())
	}
}

func TestTamper_SoakFileMovedBackIsCritical(t *testing.T) {
	f := newEnfFixture(t, "audit")
	f.soakClean(t, 1)
	p := filepath.Join(f.opts.stateDir, "soak.json")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var s soakState
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	back := s.CleanSince.Add(-30 * 24 * time.Hour)
	s.CleanSince = &back
	forged, _ := json.Marshal(s)
	if err := os.WriteFile(p, forged, 0o644); err != nil {
		t.Fatal(err)
	}
	f.setMode(t, "enforcing")
	f.logBuf.Reset()
	rep, err := f.run(t)
	if err != nil {
		t.Fatal(err)
	}
	if rep.ModeEffective != modeAudit || len(f.enf.ids) != 0 || !strings.Contains(f.logBuf.String(), "CRITICAL") {
		t.Errorf("a back-dated soak flipped the mode: %+v\n%s", rep, f.logBuf.String())
	}
}

// The real model source: the host's sealed snapshot, trust-checked.
func TestLoadSnapshotModel(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := well.NewYAMLStore(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	s, err := snapshot.Build(context.Background(), store, "east", "test", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	signer, err := gungnir.NewSignerWithID(gungnir.GenerateDevKey())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := snapshot.Seal(s, signer, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	snap, pub := filepath.Join(dir, "snapshot.json"), filepath.Join(dir, "snapshot.pub")
	if err := os.WriteFile(snap, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pub, signer.PublicKey(), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := loadSnapshotModel(snap, pub, "east", os.Getuid())
	if err != nil {
		t.Fatal(err)
	}
	// east is lab-hardened: aide-install maps both AIDE checks.
	byKey := fixesByKey(m, "east")
	if byKey["host-probe:east:aide-installed"].ID != "aide-install" || byKey["host-sysctl:east:fs.suid_dumpable=0"].ID != "sysctl-cis" {
		t.Errorf("mapping: %d keys", len(byKey))
	}
	if _, ok := byKey["host-probe:west:aide-installed"]; ok {
		t.Error("mapped another host's key")
	}
	if _, err := loadSnapshotModel(snap, pub, "west", os.Getuid()); err == nil {
		t.Error("east's snapshot accepted for west")
	}
	if err := os.Chmod(snap, 0o666); err != nil {
		t.Fatal(err)
	}
	if _, err := loadSnapshotModel(snap, pub, "east", os.Getuid()); err == nil {
		t.Error("a world-writable snapshot accepted")
	}
	if _, err := loadSnapshotModel("", pub, "east", os.Getuid()); err == nil {
		t.Error("no snapshot configured, yet a model")
	}
}
