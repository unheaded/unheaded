// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package executor

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/compliance/remediation"
)

// Tests for what S-WELL-SPRING Step 267 added: mode on log files, packages
// without recommends, enabling a unit, AppArmor enforce, reloads that run
// after the restore on undo, check, and --record-existing.

func inline(t *testing.T, id string, actions ...remediation.Action) remediation.Fix {
	t.Helper()
	return remediation.Fix{ID: id, Title: id, Controls: []string{"UH-HOST-01"}, Risk: "low", Class: "service", Actions: actions}
}

func yes() *bool { b := true; return &b }
func no() *bool  { b := false; return &b }

// Undo used to replay a fix's reload (sysctl --system, sshd reload, journald
// restart) BEFORE restoring the files, so the old config was never reloaded.
// A fake sysctl notes whether the drop-in exists when it runs.
func TestUndoReloadsAfterRestoring(t *testing.T) {
	e := newEnv(t)
	probe := "#!/bin/sh\nif [ -e \"$PROBE_FILE\" ]; then s=present; else s=absent; fi\necho \"sysctl $* conf=$s\" >>\"$FAKE_CALLS\"\n"
	if err := os.WriteFile(filepath.Join(e.bin, "sysctl"), []byte(probe), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROBE_FILE", e.root+"/etc/sysctl.d/60-unheaded-cis.conf")
	dir, err := e.executor("east").Apply(context.Background(), fix(t, "sysctl-cis"))
	if err != nil {
		t.Fatal(err)
	}
	if err := Undo(context.Background(), dir, nil, true); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(e.callLog()), "\n")
	if last := lines[len(lines)-1]; last != "sysctl --system conf=absent" {
		t.Errorf("undo's reload ran with the drop-in still there: %q", lines)
	}
}

// An undo record written before reloads were split out ("run") still replays.
func TestOldUndoRecordsStillReplay(t *testing.T) {
	e := newEnv(t)
	dir := filepath.Join(t.TempDir(), "old")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	rec := `[{"op":"run","argv":["` + filepath.Join(e.bin, "sysctl") + `","--system"],"uid":0,"gid":0}]`
	if err := os.WriteFile(filepath.Join(dir, "undo.json"), []byte(rec), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Undo(context.Background(), dir, nil, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(e.callLog(), "sysctl --system") {
		t.Errorf("calls: %q", e.callLog())
	}
}

func TestModeOnALogFile(t *testing.T) {
	e := newEnv(t)
	f := inline(t, "rocm-log-perms", remediation.Action{Mode: &remediation.ModeAction{
		Paths: []string{"/var/log/rocm_smi_lib/ROCm-SMI-lib.log", "/var/log/amd_smi_lib/AMD-SMI-lib.log"}, Mode: "0640", Owner: "root", Group: "root"}})
	before := e.snapshot()
	dir, err := e.executor("west").Apply(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(e.root + "/var/log/rocm_smi_lib/ROCm-SMI-lib.log"); fi.Mode().Perm() != 0o640 {
		t.Errorf("mode %o", fi.Mode().Perm())
	}
	if _, err := os.Stat(e.root + "/var/log/amd_smi_lib"); !errors.Is(err, os.ErrNotExist) {
		t.Error("a missing log was created")
	}
	if err := Undo(context.Background(), dir, nil, true); err != nil {
		t.Fatal(err)
	}
	if e.snapshot() != before {
		t.Error("undo did not restore the mode")
	}
}

func TestPackageWithoutRecommends(t *testing.T) {
	e := newEnv(t)
	f := inline(t, "p", remediation.Action{Package: &remediation.PackageAction{Name: "aide", NoRecommends: true}})
	if _, err := e.executor("west").Apply(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(e.callLog(), "apt-get install -y -q --no-install-recommends aide\n") {
		t.Errorf("calls: %q", e.callLog())
	}
}

func TestEnableTimerAndUndo(t *testing.T) {
	e := newEnv(t)
	f := inline(t, "t", remediation.Action{Service: &remediation.ServiceAction{Name: "dailyaidecheck.timer", Enabled: yes(), Active: yes()}})
	dir, err := e.executor("west").Apply(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	if err := Undo(context.Background(), dir, nil, true); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.TrimSpace(e.callLog()), "systemctl enable --now dailyaidecheck.timer\nsystemctl disable --now dailyaidecheck.timer"; got != want {
		t.Errorf("calls:\n%s\nwant\n%s", got, want)
	}
}

// Enabling without starting, or unmasking, has no defined undo here; refused.
func TestServiceHalfStatesRefused(t *testing.T) {
	for name, s := range map[string]*remediation.ServiceAction{
		"enable but stopped": {Name: "x.timer", Enabled: yes(), Active: no()},
		"start only":         {Name: "x.timer", Active: yes()},
		"unmask":             {Name: "x.timer", Masked: no()},
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			if _, err := e.executor("west").Apply(context.Background(), inline(t, "s", remediation.Action{Service: s})); !errors.Is(err, ErrRefused) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

// daemon-reload runs after the unit file is gone, so systemd forgets it.
func TestDaemonReloadLastOnUndo(t *testing.T) {
	e := newEnv(t)
	f := inline(t, "a",
		remediation.Action{File: &remediation.FileAction{Path: "/etc/systemd/system/unheaded-aide-check.timer", Mode: "0644",
			Parts: []remediation.Part{{Text: "[Timer]\nOnCalendar=weekly\n"}}}},
		remediation.Action{Run: "systemctl daemon-reload"})
	before := e.snapshot()
	dir, err := e.executor("east").Apply(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(e.calls, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Undo(context.Background(), dir, nil, true); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(e.callLog()); got != "systemctl daemon-reload" {
		t.Errorf("undo calls: %q", got)
	}
	if e.snapshot() != before {
		t.Error("undo left the unit or its directories")
	}
}

// A diversion is added once and removed by undo; one that is already there
// is left alone both ways.
func TestDivertAndUndo(t *testing.T) {
	e := newEnv(t)
	f := inline(t, "d", remediation.Action{Divert: "/etc/cron.daily/dailyaidecheck"})
	dir, err := e.executor("west").Apply(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	want := "dpkg-divert --listpackage /etc/cron.daily/dailyaidecheck\ndpkg-divert --local --rename --add /etc/cron.daily/dailyaidecheck"
	if got := strings.TrimSpace(e.callLog()); got != want {
		t.Errorf("apply calls:\n%s\nwant:\n%s", got, want)
	}
	if err := os.WriteFile(e.calls, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Undo(context.Background(), dir, nil, true); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(e.callLog()); got != "dpkg-divert --local --rename --remove /etc/cron.daily/dailyaidecheck" {
		t.Errorf("undo calls: %q", got)
	}

	e = newEnv(t)
	e.answer("dpkg-divert", "--listpackage /etc/cron.daily/dailyaidecheck", "LOCAL")
	dir, err = e.executor("west").Apply(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(e.callLog()); got != "dpkg-divert --listpackage /etc/cron.daily/dailyaidecheck" {
		t.Errorf("already diverted, apply calls: %q", got)
	}
	if err := os.WriteFile(e.calls, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Undo(context.Background(), dir, nil, true); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(e.callLog()); got != "" {
		t.Errorf("undo removed a diversion this apply did not add: %q", got)
	}

	// A dpkg-divert that fails is an error, and nothing was recorded to undo.
	e = newEnv(t)
	e.failing("dpkg-divert")
	if _, err := e.executor("west").Apply(context.Background(), f); err == nil {
		t.Error("a failing dpkg-divert was not an error")
	}
}

func repoFix(t *testing.T, id string) remediation.Fix {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "compliance", "remediation", "fixes.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := remediation.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range m.Fixes {
		if f.ID == id {
			return f
		}
	}
	t.Fatalf("no fix %s", id)
	return remediation.Fix{}
}

// The shipped aide-install, end to end against fakes: the package without
// recommends, our files, the package's paths off, the config check, our
// timer on. No call builds or updates a database. Undo puts it all back.
func TestAideInstallAppliesQuietlyAndUndoes(t *testing.T) {
	e := newEnv(t)
	f := repoFix(t, "aide-install")
	before := e.snapshot()
	x := e.executor("west")
	dir, err := x.Apply(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	conf := e.root + "/etc/aide/aide-unheaded.conf"
	want := strings.Join([]string{
		"apt-get install -y -q --no-install-recommends aide",
		"systemctl daemon-reload",
		"systemctl mask dailyaidecheck.timer",
		"dpkg-divert --listpackage /etc/cron.daily/dailyaidecheck",
		"dpkg-divert --local --rename --add /etc/cron.daily/dailyaidecheck",
		"dpkg-divert --listpackage /etc/cron.daily/aide",
		"dpkg-divert --local --rename --add /etc/cron.daily/aide",
		"aide --config=" + conf + " --config-check",
		"systemctl enable --now unheaded-aide-check.timer",
	}, "\n")
	if got := strings.TrimSpace(e.callLog()); got != want {
		t.Errorf("apply calls:\n%s\nwant:\n%s", got, want)
	}
	for _, l := range strings.Split(e.callLog(), "\n") {
		if strings.Contains(l, "init") || strings.Contains(l, "--update") {
			t.Errorf("apply ran %q", l)
		}
	}
	fi, err := os.Stat(conf)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("config: %v, mode %v; want 0600", err, fi)
	}
	for _, u := range []string{"unheaded-aide-check.service", "unheaded-aide-check.timer"} {
		if fi, err := os.Stat(e.root + "/etc/systemd/system/" + u); err != nil || fi.Mode().Perm() != 0o644 {
			t.Errorf("%s: %v", u, err)
		}
	}
	if err := os.WriteFile(e.calls, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Undo(context.Background(), dir, nil, true); err != nil {
		t.Fatal(err)
	}
	wantUndo := strings.Join([]string{
		"systemctl disable --now unheaded-aide-check.timer",
		"dpkg-divert --local --rename --remove /etc/cron.daily/aide",
		"dpkg-divert --local --rename --remove /etc/cron.daily/dailyaidecheck",
		"systemctl unmask dailyaidecheck.timer",
		"apt-get remove -y aide",
		"systemctl daemon-reload",
	}, "\n")
	if got := strings.TrimSpace(e.callLog()); got != wantUndo {
		t.Errorf("undo calls:\n%s\nwant:\n%s", got, wantUndo)
	}
	if e.snapshot() != before {
		t.Error("undo left files behind")
	}

	// A config AIDE does not parse is refused, and everything is restored.
	e = newEnv(t)
	e.failing("aide")
	before = e.snapshot()
	if _, err := e.executor("west").Apply(context.Background(), f); !errors.Is(err, ErrRefused) {
		t.Fatalf("got %v, want ErrRefused", err)
	}
	if e.snapshot() != before {
		t.Error("a refused apply left files behind")
	}
	if strings.Contains(e.callLog(), "enable --now unheaded-aide-check.timer") {
		t.Error("the timer was enabled on a config that does not parse")
	}
}

// No allowlisted string maps to a command that builds a database.
func TestNoCommandBuildsAnAideDatabase(t *testing.T) {
	e := newEnv(t)
	for _, s := range []string{"aideinit -y -f", "aideinit", "aide --init", "aide --update"} {
		if _, err := e.cmds().argv(s); !errors.Is(err, ErrRefused) {
			t.Errorf("argv(%q) = %v, want ErrRefused", s, err)
		}
	}
	for cmd := range remediation.AllowedRun {
		argv, err := e.cmds().argv(cmd)
		if err != nil {
			t.Errorf("allowlisted run %q has no argv: %v", cmd, err)
		}
		for _, a := range argv {
			if strings.Contains(a, "aideinit") || a == "--init" || a == "--update" {
				t.Errorf("run %q maps to %v", cmd, argv)
			}
		}
	}
}

func TestAppArmorEnforceAndUndo(t *testing.T) {
	e := newEnv(t)
	// aa-enforce rewrites the profile's flags; the fake does the same.
	rewrite := "#!/bin/sh\necho \"aa-enforce $*\" >>\"$FAKE_CALLS\"\nprintf 'profile Xorg /usr/lib/xorg/Xorg {\\n}\\n' >\"$1\"\n"
	if err := os.WriteFile(filepath.Join(e.bin, "aa-enforce"), []byte(rewrite), 0o755); err != nil {
		t.Fatal(err)
	}
	f := inline(t, "apparmor-enforce", remediation.Action{AppArmor: &remediation.AppArmorAction{Enforce: []string{"Xorg", "not-installed"}}})
	before := e.snapshot()
	dir, err := e.executor("west").Apply(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	profile := e.root + "/etc/apparmor.d/Xorg"
	if !strings.Contains(e.callLog(), "aa-enforce "+profile+"\n") || strings.Contains(e.callLog(), "not-installed") {
		t.Errorf("calls: %q", e.callLog())
	}
	if err := os.WriteFile(e.calls, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Undo(context.Background(), dir, nil, true); err != nil {
		t.Fatal(err)
	}
	if e.snapshot() != before {
		t.Error("undo did not restore the profile file")
	}
	if got := strings.TrimSpace(e.callLog()); got != "apparmor_parser -r "+profile {
		t.Errorf("undo calls: %q", got)
	}
}

func TestAppArmorOnlyOnItsHosts(t *testing.T) {
	e := newEnv(t)
	f := inline(t, "a", remediation.Action{AppArmor: &remediation.AppArmorAction{Enforce: []string{"Xorg"}, Hosts: []string{"west"}}})
	if _, err := e.executor("east").Apply(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(e.callLog(), "aa-enforce") {
		t.Errorf("enforced on east: %q", e.callLog())
	}
}

// A profile the operator disabled or forced to complain stays as it is.
func TestAppArmorRefusesOperatorOverrides(t *testing.T) {
	for _, d := range []string{"disable", "force-complain"} {
		t.Run(d, func(t *testing.T) {
			e := newEnv(t)
			if err := os.MkdirAll(e.root+"/etc/apparmor.d/"+d, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("/etc/apparmor.d/Xorg", e.root+"/etc/apparmor.d/"+d+"/Xorg"); err != nil {
				t.Fatal(err)
			}
			f := inline(t, "a", remediation.Action{AppArmor: &remediation.AppArmorAction{Enforce: []string{"Xorg"}}})
			if _, err := e.executor("west").Apply(context.Background(), f); !errors.Is(err, ErrRefused) {
				t.Fatalf("got %v", err)
			}
			if strings.Contains(e.callLog(), "aa-enforce") {
				t.Errorf("enforced anyway: %q", e.callLog())
			}
		})
	}
}

func TestCheckFilesModesAbsent(t *testing.T) {
	e := newEnv(t)
	x := e.executor("east")
	f := fix(t, "cron-perms")
	devs, err := x.Check(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) == 0 {
		t.Fatal("an unapplied fix checks clean")
	}
	if _, err := x.Apply(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if devs, err = x.Check(context.Background(), f); err != nil || len(devs) != 0 {
		t.Fatalf("after apply: %v %v", devs, err)
	}
	// Check changes nothing.
	before := e.snapshot()
	if _, err := x.Check(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if e.snapshot() != before {
		t.Error("check changed files")
	}
}

func TestCheckPackageServiceAppArmor(t *testing.T) {
	e := newEnv(t)
	x := e.executor("west")
	pkg := inline(t, "p", remediation.Action{Package: &remediation.PackageAction{Name: "aide"}})
	svc := inline(t, "s", remediation.Action{Service: &remediation.ServiceAction{Name: "apport.service", Enabled: no(), Active: no(), Masked: yes()}})
	on := inline(t, "o", remediation.Action{Service: &remediation.ServiceAction{Name: "dailyaidecheck.timer", Enabled: yes(), Active: yes()}})
	aa := inline(t, "a", remediation.Action{AppArmor: &remediation.AppArmorAction{Enforce: []string{"Xorg"}}})
	for _, f := range []remediation.Fix{pkg, svc, on, aa} {
		if devs, err := x.Check(context.Background(), f); err != nil || len(devs) == 0 {
			t.Errorf("%s before: %v %v", f.ID, devs, err)
		}
	}
	e.answer("dpkg-query", "-W -f=${db:Status-Status} aide", "installed")
	e.answer("systemctl", "is-enabled apport.service", "masked")
	e.answer("systemctl", "is-active apport.service", "inactive")
	e.answer("systemctl", "is-enabled dailyaidecheck.timer", "enabled")
	e.answer("systemctl", "is-active dailyaidecheck.timer", "active")
	e.write("/sys/kernel/security/apparmor/profiles", "Xorg (enforce)\n", 0o644)
	for _, f := range []remediation.Fix{pkg, svc, on, aa} {
		if devs, err := x.Check(context.Background(), f); err != nil || len(devs) != 0 {
			t.Errorf("%s after: %v %v", f.ID, devs, err)
		}
	}
}

func TestRecordExistingJournalsWithoutChanging(t *testing.T) {
	e := newEnv(t)
	x := e.executor("east")
	f := fix(t, "banner-cis")
	if _, err := x.Apply(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	// Start from a host where the fix is in place but nothing journaled it.
	if err := os.RemoveAll(e.state); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(e.calls, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	before := e.snapshot()
	at := time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)
	x.o.Now = func() time.Time { return at }
	if err := x.RecordExisting(context.Background(), f, "remediate.sh (2026-10-01)"); err != nil {
		t.Fatal(err)
	}
	if e.snapshot() != before || e.callLog() != "" {
		t.Errorf("record-existing changed the host: %q", e.callLog())
	}
	j := readJournal(t, e)
	if len(j) != 1 {
		t.Fatalf("%d entries", len(j))
	}
	sum, _ := f.SHA256()
	got := j[0]
	if got.Action != "apply" || got.Outcome != "success" || !got.Backfilled || got.FixSHA256 != sum || got.BackupDir != "" ||
		!strings.Contains(got.Operator, "applied_by remediate.sh (2026-10-01), recorded 2026-10-28") {
		t.Errorf("entry: %+v", got)
	}
}

func TestRecordExistingRefusesWhenTheCheckFails(t *testing.T) {
	e := newEnv(t)
	x := e.executor("east")
	err := x.RecordExisting(context.Background(), fix(t, "banner-cis"), "remediate.sh (2026-10-01)")
	if !errors.Is(err, ErrRefused) {
		t.Fatalf("got %v", err)
	}
	if _, err := os.Stat(filepath.Join(e.state, journalName)); !errors.Is(err, os.ErrNotExist) {
		t.Error("a journal entry was written for a fix that is not in place")
	}
}

func TestRecordExistingRejectsBadAppliedBy(t *testing.T) {
	e := newEnv(t)
	x := e.executor("east")
	for _, s := range []string{"", "x\ny", strings.Repeat("a", 31)} {
		if err := x.RecordExisting(context.Background(), fix(t, "banner-cis"), s); !errors.Is(err, ErrRefused) {
			t.Errorf("%q: got %v", s, err)
		}
	}
}

func TestPlanNamesNewActions(t *testing.T) {
	e := newEnv(t)
	var out bytes.Buffer
	x, err := New(context.Background(), Options{Root: e.root, State: e.state, Host: "west", Cmd: e.cmds(), Out: &out, AllowNonRoot: true})
	if err != nil {
		t.Fatal(err)
	}
	f := inline(t, "n",
		remediation.Action{AppArmor: &remediation.AppArmorAction{Enforce: []string{"Xorg"}}},
		remediation.Action{Package: &remediation.PackageAction{Name: "aide", NoRecommends: true}},
		remediation.Action{Service: &remediation.ServiceAction{Name: "dailyaidecheck.timer", Enabled: yes(), Active: yes()}})
	if err := x.Plan(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"aa-enforce", "--no-install-recommends aide", "enable --now dailyaidecheck.timer"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("plan lacks %q:\n%s", want, out.String())
		}
	}
	if e.callLog() != "" {
		t.Errorf("plan ran commands: %q", e.callLog())
	}
}

// The baseline agent's flapping breaker counts applies from the journal.
func TestAppliesSince(t *testing.T) {
	e := newEnv(t)
	x := e.executor("east")
	t0 := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	now := t0
	x.o.Now = func() time.Time { return now }
	f := fix(t, "banner-cis")
	for i := 0; i < 3; i++ {
		now = t0.Add(time.Duration(i) * 10 * time.Hour)
		if _, err := x.Apply(context.Background(), f); err != nil {
			t.Fatal(err)
		}
	}
	if err := x.RecordExisting(context.Background(), f, "test"); err != nil {
		t.Fatal(err)
	}
	for since, want := range map[time.Time]int{t0: 3, t0.Add(time.Hour): 2, t0.Add(25 * time.Hour): 0} {
		if n, err := x.AppliesSince("banner-cis", since); err != nil || n != want {
			t.Errorf("since %v: %d %v, want %d (the recorded entry never counts)", since, n, err, want)
		}
	}
	if n, _ := x.AppliesSince("kmod-cis", t0); n != 0 {
		t.Errorf("another fix counted: %d", n)
	}
	p := filepath.Join(e.state, journalName)
	b, _ := os.ReadFile(p)
	if err := os.WriteFile(p, []byte(strings.Replace(string(b), `"success"`, `"failure"`, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := x.AppliesSince("banner-cis", t0); err == nil {
		t.Error("a broken journal counted as an answer")
	}
}
