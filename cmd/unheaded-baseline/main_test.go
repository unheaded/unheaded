// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/compliance/crosswalk/collect"
)

const testBaseline = `version: 1
sources:
  - {kind: host-probe, ref: "east:auditd-running"}
  - {kind: host-probe, ref: "east:firewall-inbound-deny"}
  - {kind: host-sshd, ref: "east:maxauthtries<=4"}
  - {kind: host-sshd, ref: "east:passwordauthentication=no"}
  - {kind: host-sysctl, ref: "east:fs.suid_dumpable=0"}
  - {kind: host-sysctl, ref: "east:kernel.kptr_restrict>=1"}
  - {kind: host-sysctl, ref: "west:fs.suid_dumpable=0"}
`

type fixture struct {
	opts   options
	logBuf *strings.Builder
	dir    string
	calls  []string // hosts the evaluators were asked about
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := t.TempDir()
	f := &fixture{dir: dir, logBuf: &strings.Builder{}}
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("share/baseline.yaml", testBaseline)
	write("proc/sys/fs/suid_dumpable", "2\n")
	write("proc/sys/kernel/kptr_restrict", "1\n")
	f.opts = options{
		baselinePath: filepath.Join(dir, "share/baseline.yaml"),
		configPath:   filepath.Join(dir, "etc/baseline.conf"),
		stateDir:     filepath.Join(dir, "state"),
		textfileDir:  filepath.Join(dir, "textfile"),
		hostname:     "east",
		procSys:      filepath.Join(dir, "proc/sys"),
		trustedUID:   os.Getuid(),
		now:          func() time.Time { return time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC) },
		sshdDump: func(_ context.Context, host string) (string, error) {
			f.calls = append(f.calls, "sshd:"+host)
			return "maxauthtries 6\npasswordauthentication no\n", nil
		},
		runProbes: func(_ context.Context, host, script string) (string, error) {
			f.calls = append(f.calls, "probe:"+host)
			return "@@auditd-running\ninactive\n@@firewall-inbound-deny\n-P INPUT DROP\n---\n-P INPUT DROP\n", nil
		},
	}
	return f
}

func (f *fixture) run(t *testing.T) (*report, error) {
	t.Helper()
	return run(context.Background(), f.opts, f.logBuf)
}

func TestRun_AuditReportsDeviationsAndWouldEnforce(t *testing.T) {
	f := newFixture(t)
	rep, err := f.run(t)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Host != "east" || rep.ModeConfig != "audit" || rep.ModeEffective != "audit" {
		t.Errorf("modes = %+v", rep)
	}
	if rep.Evaluated != 6 || rep.Deviations != 3 || rep.Unevaluated != 0 {
		t.Errorf("evaluated %d deviations %d unevaluated %d, want 6/3/0", rep.Evaluated, rep.Deviations, rep.Unevaluated)
	}
	byKey := map[string]check{}
	for _, c := range rep.Checks {
		byKey[c.Key] = c
	}
	for key, want := range map[string]string{
		"host-sysctl:east:fs.suid_dumpable=0":      "sysctl -w fs.suid_dumpable=0",
		"host-sshd:east:maxauthtries<=4":           "MaxAuthTries",
		"host-probe:east:auditd-running":           "systemctl enable --now auditd",
		"host-sysctl:east:kernel.kptr_restrict>=1": "",
	} {
		c, ok := byKey[key]
		if !ok {
			t.Errorf("%s missing from report", key)
			continue
		}
		if !strings.Contains(c.WouldEnforce, want) || (want == "") != (c.Verdict == "pass") {
			t.Errorf("%s: verdict %s would_enforce %q, want containing %q", key, c.Verdict, c.WouldEnforce, want)
		}
	}
	if _, ok := byKey["host-sysctl:west:fs.suid_dumpable=0"]; ok {
		t.Error("another host's check evaluated")
	}
	for _, c := range f.calls {
		if !strings.HasSuffix(c, ":east") {
			t.Errorf("evaluator called for another host: %s", c)
		}
	}

	log := f.logBuf.String()
	if strings.Count(log, "DEVIATION ") != 3 || !strings.Contains(log, `would_enforce="sysctl -w fs.suid_dumpable=0 (and persist in sysctl.d)"`) ||
		!strings.Contains(log, "mode=audit") {
		t.Errorf("log:\n%s", log)
	}

	raw, err := os.ReadFile(filepath.Join(f.opts.stateDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var onDisk report
	if err := json.Unmarshal(raw, &onDisk); err != nil || onDisk.Deviations != 3 || len(onDisk.BaselineSHA256) != 64 {
		t.Errorf("report.json: %v %+v", err, onDisk)
	}

	prom, err := os.ReadFile(filepath.Join(f.opts.textfileDir, "unheaded_baseline.prom"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`unheaded_baseline_check{key="host-sysctl:east:fs.suid_dumpable=0"} 0`,
		`unheaded_baseline_check{key="host-sysctl:east:kernel.kptr_restrict>=1"} 1`,
		"unheaded_baseline_deviations 3",
		`unheaded_baseline_mode_config{mode="audit"} 1`,
		"unheaded_baseline_enforcing 0",
		"unheaded_baseline_last_run_timestamp_seconds 1790769600",
		"# TYPE unheaded_baseline_check gauge",
	} {
		if !strings.Contains(string(prom), want) {
			t.Errorf("textfile missing %q:\n%s", want, prom)
		}
	}
}

func TestRun_EnforcingRequestedRunsAudit(t *testing.T) {
	f := newFixture(t)
	if err := os.MkdirAll(filepath.Dir(f.opts.configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.opts.configPath, []byte("# ADR-098\nmode=enforcing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := f.run(t)
	if err != nil {
		t.Fatal(err)
	}
	if rep.ModeConfig != "enforcing" || rep.ModeEffective != "audit" {
		t.Errorf("modes %s/%s: this build has no enforcing path", rep.ModeConfig, rep.ModeEffective)
	}
	if !strings.Contains(f.logBuf.String(), "CRITICAL enforcing requested") {
		t.Errorf("log:\n%s", f.logBuf.String())
	}
}

func TestRun_BadConfigIsAuditAndLoud(t *testing.T) {
	for name, body := range map[string]string{
		"unknown mode": "mode=permissive\n",
		"unknown key":  "mode=audit\nauto_revert=yes\n",
		"no equals":    "enforcing\n",
		"mode twice":   "mode=audit\nmode=enforcing\n",
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			if err := os.MkdirAll(filepath.Dir(f.opts.configPath), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(f.opts.configPath, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			rep, err := f.run(t)
			if err != nil {
				t.Fatal(err)
			}
			if rep.ModeEffective != "audit" || rep.ConfigError == "" || !strings.Contains(f.logBuf.String(), "CRITICAL") {
				t.Errorf("rep %+v log %s", rep, f.logBuf.String())
			}
		})
	}
}

func TestRun_Refusals(t *testing.T) {
	t.Run("host not in baseline", func(t *testing.T) {
		f := newFixture(t)
		f.opts.hostname = "north"
		if _, err := f.run(t); err == nil || !strings.Contains(err.Error(), "no baseline checks") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("bad hostname", func(t *testing.T) {
		f := newFixture(t)
		f.opts.hostname = "../east"
		if _, err := f.run(t); err == nil {
			t.Error("accepted")
		}
	})
	t.Run("invalid baseline", func(t *testing.T) {
		f := newFixture(t)
		if err := os.WriteFile(f.opts.baselinePath, []byte("version: 1\nsources: [{kind: github-job, ref: x}]\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := f.run(t); !errors.Is(err, collect.ErrInvalidBaseline) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("nothing evaluable", func(t *testing.T) {
		f := newFixture(t)
		f.opts.procSys = filepath.Join(f.dir, "nope")
		f.opts.sshdDump = func(context.Context, string) (string, error) { return "", errors.New("no sshd") }
		f.opts.runProbes = func(context.Context, string, string) (string, error) { return "", errors.New("no sh") }
		if _, err := f.run(t); err == nil || !strings.Contains(err.Error(), "no check could be evaluated") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("some unevaluated", func(t *testing.T) {
		f := newFixture(t)
		f.opts.sshdDump = func(context.Context, string) (string, error) { return "", errors.New("no sshd") }
		rep, err := f.run(t)
		if err != nil {
			t.Fatal(err)
		}
		if rep.Unevaluated != 2 || rep.Evaluated != 4 {
			t.Errorf("evaluated %d unevaluated %d", rep.Evaluated, rep.Unevaluated)
		}
	})
}

func TestTrust(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil { // TempDir follows the umask
		t.Fatal(err)
	}
	good := filepath.Join(dir, "baseline.yaml")
	if err := os.WriteFile(good, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	me := os.Getuid()
	if err := checkTrust(good, me); err != nil {
		t.Errorf("own 0644 file in own 0700 dir: %v", err)
	}
	if me != 0 {
		if err := checkTrust(good, 0); err == nil {
			t.Error("a file not owned by root accepted as root-trusted")
		}
	}
	if err := os.Chmod(good, 0o664); err != nil {
		t.Fatal(err)
	}
	if err := checkTrust(good, me); err == nil {
		t.Error("group-writable accepted")
	}
	if err := os.Chmod(good, 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.yaml")
	if err := os.Symlink(good, link); err != nil {
		t.Fatal(err)
	}
	if err := checkTrust(link, me); err == nil {
		t.Error("symlink accepted")
	}
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o700) //nolint:errcheck // test cleanup
	if err := checkTrust(good, me); err == nil {
		t.Error("world-writable parent accepted")
	}
}

// Every probe the baseline may contain has a would-enforce action, so a new
// baseline probe cannot ship without saying what enforcing would do.
func TestEveryBaselineProbeHasAnAction(t *testing.T) {
	for name, p := range collect.Probes {
		if p.Baseline {
			if _, ok := probeActions[name]; !ok {
				t.Errorf("baseline probe %q has no entry in probeActions", name)
			}
		}
	}
}
