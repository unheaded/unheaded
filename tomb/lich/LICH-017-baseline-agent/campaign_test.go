// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package lich017 is LICH-017: the unheaded-baseline enforcing path
// (ADR-098 phase 4, S-WELL-SPRING Step 279) attacked from outside, through
// the built binary and the files it trusts. Every tamper must be a critical
// event, and mode_effective must never read enforcing without the 7-day
// soak. The binary runs unprivileged against a fake /proc/sys and a private
// state directory; it touches nothing on the host.
//
//	go test ./tomb/lich/LICH-017-baseline-agent/... -count=1 -v
package lich017

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

var binary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "lich017-")
	if err != nil {
		panic(err)
	}
	binary = filepath.Join(dir, "unheaded-baseline")
	build := exec.Command("go", "build", "-o", binary, "unheaded/cmd/unheaded-baseline")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		_ = os.RemoveAll(dir)
		panic("build unheaded-baseline: " + err.Error())
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// target is one private host for the agent: a root-trusted (here: our own
// uid) baseline, a fake sysctl tree, a mode file and a state directory.
type target struct {
	t                                 *testing.T
	dir, baseline, conf, state, procs string
}

func newTarget(t *testing.T) *target {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	x := &target{t: t, dir: dir, baseline: filepath.Join(dir, "share", "baseline.yaml"), conf: filepath.Join(dir, "baseline.conf"),
		state: filepath.Join(dir, "state"), procs: filepath.Join(dir, "proc", "sys")}
	x.write(x.baseline, "version: 1\nsources:\n  - {kind: host-sysctl, ref: \"east:fs.suid_dumpable=0\"}\n  - {kind: host-sysctl, ref: \"east:kernel.kptr_restrict>=1\"}\n", 0o644)
	x.write(filepath.Join(x.procs, "fs", "suid_dumpable"), "2\n", 0o644)
	x.write(filepath.Join(x.procs, "kernel", "kptr_restrict"), "1\n", 0o644)
	x.mode("audit")
	return x
}

func (x *target) write(p, body string, mode os.FileMode) {
	x.t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		x.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), mode); err != nil {
		x.t.Fatal(err)
	}
	if err := os.Chmod(p, mode); err != nil {
		x.t.Fatal(err)
	}
}

func (x *target) mode(m string) { x.write(x.conf, "mode="+m+"\n", 0o644) }

// run runs the agent once and returns its exit code, its log and the report.
func (x *target) run() (int, string, map[string]any) {
	x.t.Helper()
	cmd := exec.Command(binary, "-baseline", x.baseline, "-config", x.conf, "-state-dir", x.state, "-hostname", "east",
		"-proc-sys", x.procs, "-snapshot", "", "-remediate-state", filepath.Join(x.dir, "remediate"),
		"-trusted-uid", strconv.Itoa(os.Getuid()), "-timeout", "30s")
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + x.dir} // no WOTAN_ADDR: nothing leaves the test
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	code := 0
	if err := cmd.Run(); err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			x.t.Fatal(err)
		}
		code = ee.ExitCode()
	}
	var rep map[string]any
	if b, err := os.ReadFile(filepath.Join(x.state, "report.json")); err == nil {
		if err := json.Unmarshal(b, &rep); err != nil {
			x.t.Fatalf("report.json: %v", err)
		}
	}
	return code, out.String(), rep
}

// neverEnforcing is the campaign's standing invariant.
func neverEnforcing(t *testing.T, rep map[string]any) {
	t.Helper()
	if rep != nil && rep["mode_effective"] != "audit" {
		t.Fatalf("BYPASS: mode_effective %v without a soak", rep["mode_effective"])
	}
}

func critical(t *testing.T, log string, rep map[string]any, what string) {
	t.Helper()
	if !strings.Contains(log, "CRITICAL") || !strings.Contains(log, what) {
		t.Errorf("tamper %q is not a critical event:\n%s", what, log)
	}
	if rep != nil && rep["critical_events"] == nil && !strings.Contains(log, "enforcing requested") && !strings.Contains(log, "baseline-untrusted") {
		t.Errorf("report carries no critical_events for %q", what)
	}
}

// T1: the baseline is made writable by others (or its directory is): the
// agent refuses to evaluate it, loudly, and writes no report.
func TestLICH017_BaselinePermissions(t *testing.T) {
	for name, tamper := range map[string]func(x *target){
		"file 0666":      func(x *target) { _ = os.Chmod(x.baseline, 0o666) },
		"directory 0777": func(x *target) { _ = os.Chmod(filepath.Dir(x.baseline), 0o777) },
		"symlink": func(x *target) {
			real := x.baseline + ".real"
			_ = os.Rename(x.baseline, real)
			_ = os.Symlink(real, x.baseline)
		},
	} {
		t.Run(name, func(t *testing.T) {
			x := newTarget(t)
			tamper(x)
			code, log, rep := x.run()
			if code == 0 {
				t.Fatalf("evaluated a tampered baseline:\n%s", log)
			}
			critical(t, log, nil, "baseline-untrusted")
			if rep != nil {
				t.Error("wrote a report from an untrusted baseline")
			}
		})
	}
}

// T2: the mode file is swapped to enforcing with no soak behind it.
func TestLICH017_ModeSwapWithoutSoak(t *testing.T) {
	x := newTarget(t)
	x.mode("enforcing")
	code, log, rep := x.run()
	if code != 0 {
		t.Fatalf("exit %d:\n%s", code, log)
	}
	neverEnforcing(t, rep)
	critical(t, log, rep, "enforcing requested")
	// Variants of the spelling are config errors, never enforcing.
	for _, m := range []string{"Enforcing", "enforcing ", "enforce", "enforcing\nmode=enforcing"} {
		x.mode(m)
		_, log, rep := x.run()
		neverEnforcing(t, rep)
		if !strings.Contains(log, "CRITICAL") {
			t.Errorf("mode %q not a critical event:\n%s", m, log)
		}
	}
}

// T3: report.json is forged between runs (claims enforcing with critical
// deviations: the install gate's input). The agent catches it on its next
// run, restarts the soak and never reads it back as truth.
func TestLICH017_ForgedReport(t *testing.T) {
	x := newTarget(t)
	if code, log, _ := x.run(); code != 0 {
		t.Fatalf("first run: %s", log)
	}
	p := filepath.Join(x.state, "report.json")
	b, _ := os.ReadFile(p)
	forged := strings.Replace(string(b), `"mode_effective": "audit"`, `"mode_effective": "enforcing"`, 1)
	forged = strings.Replace(forged, `"critical_deviations": 0`, `"critical_deviations": 5`, 1)
	if forged == string(b) {
		t.Fatal("forgery changed nothing")
	}
	x.write(p, forged, 0o644)
	x.mode("enforcing")
	_, log, rep := x.run()
	neverEnforcing(t, rep)
	critical(t, log, rep, "changed outside the agent")
	if rep["soak_clean_since"] != nil {
		t.Error("the soak survived a forged report")
	}
}

// T4: an old report is put back (replay), hoping to restore an older state.
func TestLICH017_ReplayedReport(t *testing.T) {
	x := newTarget(t)
	x.run()
	p := filepath.Join(x.state, "report.json")
	old, _ := os.ReadFile(p)
	time.Sleep(10 * time.Millisecond)
	x.run()
	x.write(p, string(old), 0o644)
	_, log, rep := x.run()
	neverEnforcing(t, rep)
	critical(t, log, rep, "forged or replayed")
}

// T5: soak.json is back-dated (or deleted) to fake seven clean days.
func TestLICH017_SoakForgery(t *testing.T) {
	t.Run("back-dated", func(t *testing.T) {
		x := newTarget(t)
		x.run()
		p := filepath.Join(x.state, "soak.json")
		b, _ := os.ReadFile(p)
		var s map[string]any
		if err := json.Unmarshal(b, &s); err != nil {
			t.Fatal(err)
		}
		s["clean_since"] = time.Now().Add(-30 * 24 * time.Hour).UTC().Format(time.RFC3339Nano)
		nb, _ := json.Marshal(s)
		x.write(p, string(nb), 0o644)
		x.mode("enforcing")
		_, log, rep := x.run()
		neverEnforcing(t, rep)
		critical(t, log, rep, "does not agree")
	})
	t.Run("deleted", func(t *testing.T) {
		x := newTarget(t)
		x.run()
		if err := os.Remove(filepath.Join(x.state, "soak.json")); err != nil {
			t.Fatal(err)
		}
		_, log, rep := x.run()
		neverEnforcing(t, rep)
		critical(t, log, rep, "soak.json is gone")
	})
	t.Run("garbage", func(t *testing.T) {
		x := newTarget(t)
		x.run()
		x.write(filepath.Join(x.state, "soak.json"), "{\"clean_since\": 7", 0o644)
		_, log, rep := x.run()
		neverEnforcing(t, rep)
		critical(t, log, rep, "soak.json unreadable")
	})
}

// T6: no fix snapshot (or an untrusted one): the soak clock never starts,
// so enforcing can never follow, however long the agent runs.
func TestLICH017_NoSnapshotNoSoak(t *testing.T) {
	x := newTarget(t)
	for i := 0; i < 3; i++ {
		_, _, rep := x.run()
		if rep["soak_clean_since"] != nil {
			t.Fatal("the soak clock ran without a fix snapshot")
		}
	}
	x.mode("enforcing")
	_, _, rep := x.run()
	neverEnforcing(t, rep)
}

// T7: the install gate (preinst) against forged reports. A forged enforcing
// report blocks installs: fail closed is the intent. A forged audit report
// lets them through, which is the state of every host until the flip; the
// agent then reports the forgery (T3).
func TestLICH017_PreinstGate(t *testing.T) {
	repo, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	gen := exec.Command("bash", "-c", `. "$1/scripts/lib/deb-maintscripts.sh" && deb_preinst unheaded-wotan`, "gen", repo)
	script, err := gen.Output()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	pre, report := filepath.Join(dir, "preinst"), filepath.Join(dir, "report.json")
	if err := os.WriteFile(pre, script, 0o755); err != nil {
		t.Fatal(err)
	}
	gate := func(body string, env ...string) int {
		if err := os.WriteFile(report, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(pre, "install")
		cmd.Env = append([]string{"PATH=" + os.Getenv("PATH"), "UNHEADED_BASELINE_REPORT=" + report}, env...)
		if err := cmd.Run(); err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				return ee.ExitCode()
			}
			t.Fatal(err)
		}
		return 0
	}
	for name, c := range map[string]struct {
		body string
		want int
	}{
		"forged enforcing with deviations":   {"{\n  \"mode_effective\": \"enforcing\",\n  \"critical_deviations\": 3,\n}\n", 1},
		"forged enforcing, count unreadable": {"{\n  \"mode_effective\": \"enforcing\",\n  \"critical_deviations\": \"0\",\n}\n", 1},
		"forged enforcing, no count":         {"{\n  \"mode_effective\": \"enforcing\"\n}\n", 1},
		"enforcing, clean":                   {"{\n  \"mode_effective\": \"enforcing\",\n  \"critical_deviations\": 0,\n}\n", 0},
		"audit":                              {"{\n  \"mode_effective\": \"audit\",\n  \"critical_deviations\": 9,\n}\n", 0},
		"one-line JSON claiming enforcing":   {`{"mode_effective":"enforcing","critical_deviations":3}`, 1},
		"one-line JSON, enforcing and clean": {`{"mode_effective":"enforcing","critical_deviations":0}`, 0},
	} {
		if got := gate(c.body); got != c.want {
			t.Errorf("%s: exit %d, want %d", name, got, c.want)
		}
	}
	if got := gate("{\n  \"mode_effective\": \"enforcing\",\n  \"critical_deviations\": 3,\n}\n", "UNHEADED_BASELINE_GATE=off"); got != 0 {
		t.Errorf("override: exit %d", got)
	}
}
