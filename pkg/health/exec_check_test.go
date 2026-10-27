// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package health

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// shScript writes a test fixture and returns the argv that runs it with
// /bin/sh (exec-ing the fresh file itself races ETXTBSY under t.Parallel).
// The exec check never adds a shell of its own: argv runs as given.
func shScript(t *testing.T, body string) []string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "check.sh")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return []string{"/bin/sh", p}
}

func TestRunExec_ExitZeroIsOK(t *testing.T) {
	t.Parallel()
	r := RunExec(context.Background(), ExecCheck{Argv: shScript(t, "echo all good")})
	if r.ExitCode != 0 || r.TimedOut || r.Err != nil {
		t.Fatalf("want ok, got %+v", r)
	}
	if !r.Healthy() {
		t.Fatal("exit 0 must be healthy")
	}
	if strings.TrimSpace(string(r.Output)) != "all good" {
		t.Fatalf("output = %q", r.Output)
	}
}

func TestRunExec_NonZeroIsFailing(t *testing.T) {
	t.Parallel()
	for _, code := range []int{1, 2, 7} {
		r := RunExec(context.Background(), ExecCheck{Argv: shScript(t, "echo WARN: x >&2; exit "+strconv.Itoa(code))})
		if r.Healthy() || r.ExitCode != code {
			t.Fatalf("exit %d: want failing with that code, got %+v", code, r)
		}
		if !strings.Contains(string(r.Output), "WARN: x") {
			t.Fatalf("stderr must be captured, got %q", r.Output)
		}
	}
}

func TestRunExec_TimeoutIsFailingAndKillsChildren(t *testing.T) {
	t.Parallel()
	// The script's background child holds stdout open: unless the whole
	// process group is killed, Wait blocks until that child exits.
	start := time.Now()
	r := RunExec(context.Background(), ExecCheck{
		Argv:    shScript(t, "sleep 30 & sleep 30"),
		Timeout: 300 * time.Millisecond,
	})
	if el := time.Since(start); el > 5*time.Second {
		t.Fatalf("timeout not enforced on the process group: took %v", el)
	}
	if !r.TimedOut || r.Healthy() {
		t.Fatalf("want timed out and failing, got %+v", r)
	}
}

func TestRunExec_OutputCapped(t *testing.T) {
	t.Parallel()
	// ~64 KiB of output, the last line distinctive: the cap keeps the tail.
	r := RunExec(context.Background(), ExecCheck{Argv: shScript(t,
		"i=0; while [ $i -lt 1024 ]; do echo 0123456789012345678901234567890123456789012345678901234567890; i=$((i+1)); done; echo LAST-LINE")})
	if len(r.Output) > ExecOutputCap {
		t.Fatalf("output %d bytes, cap %d", len(r.Output), ExecOutputCap)
	}
	if !r.Truncated {
		t.Fatal("Truncated not set")
	}
	if !strings.HasSuffix(strings.TrimSpace(string(r.Output)), "LAST-LINE") {
		t.Fatalf("cap must keep the tail, got %q", string(r.Output[len(r.Output)-40:]))
	}
	if !r.Healthy() {
		t.Fatal("a capped output does not change the verdict")
	}
}

func TestRunExec_ArgvOnlyNoShell(t *testing.T) {
	t.Parallel()
	// Shell syntax in an argument reaches the program verbatim.
	marker := filepath.Join(t.TempDir(), "pwned")
	arg := "$HOME; touch " + marker + " `id`"
	r := RunExec(context.Background(), ExecCheck{Argv: []string{"/bin/echo", arg}})
	if !r.Healthy() {
		t.Fatalf("echo failed: %+v", r)
	}
	if strings.TrimSpace(string(r.Output)) != arg {
		t.Fatalf("argument was interpreted: %q", r.Output)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("argument was run by a shell")
	}
}

func TestRunExec_MinimalEnvironment(t *testing.T) {
	// Not parallel: sets a variable in this process's environment.
	t.Setenv("AKIRA_SECRET_SHOULD_NOT_LEAK", "x")
	r := RunExec(context.Background(), ExecCheck{
		Argv: []string{"/usr/bin/env"},
		Env:  map[string]string{"UNHEADED_TLS_DIR": "/etc/unheaded/tls"},
	})
	out := string(r.Output)
	if strings.Contains(out, "AKIRA_SECRET_SHOULD_NOT_LEAK") {
		t.Fatalf("Akira's environment leaked into the check:\n%s", out)
	}
	for _, want := range []string{"PATH=", "UNHEADED_TLS_DIR=/etc/unheaded/tls"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in\n%s", want, out)
		}
	}
}

func TestRunExec_RunsAsOwnUID(t *testing.T) {
	t.Parallel()
	r := RunExec(context.Background(), ExecCheck{Argv: []string{"/usr/bin/id", "-u"}})
	if got := strings.TrimSpace(string(r.Output)); got != strconv.Itoa(os.Getuid()) {
		t.Fatalf("check ran as uid %s, Akira is %d", got, os.Getuid())
	}
}

func TestRunExec_RejectsBadConfig(t *testing.T) {
	t.Parallel()
	for _, argv := range [][]string{nil, {}, {"echo", "relative"}, {""}} {
		r := RunExec(context.Background(), ExecCheck{Argv: argv})
		if r.Healthy() || r.Err == nil {
			t.Fatalf("argv %q: want an error, got %+v", argv, r)
		}
	}
	if err := (ExecCheck{Argv: []string{"/bin/true"}, Timeout: MaxExecTimeout + time.Second}).Validate(); err == nil {
		t.Fatal("timeout above MaxExecTimeout accepted")
	}
	if err := (ExecCheck{Argv: []string{"/bin/true"}, Timeout: -time.Second}).Validate(); err == nil {
		t.Fatal("negative timeout accepted")
	}
	if err := (ExecCheck{Argv: []string{"/bin/true"}, Env: map[string]string{"bad name": "x"}}).Validate(); err == nil {
		t.Fatal("bad env name accepted")
	}
	if err := (ExecCheck{Argv: []string{"/bin/true"}, Env: map[string]string{"OK_NAME": "a\x00b"}}).Validate(); err == nil {
		t.Fatal("NUL in env value accepted")
	}
}

func TestRunExec_MissingProgramIsFailing(t *testing.T) {
	t.Parallel()
	r := RunExec(context.Background(), ExecCheck{Argv: []string{"/nonexistent/check"}})
	if r.Healthy() || r.Err == nil {
		t.Fatalf("want failing with an error, got %+v", r)
	}
}

func TestAkira_ExecTargetVotes(t *testing.T) {
	t.Parallel()
	fail := shScript(t, "echo 'ERROR: timeguru (20% remaining, 0h left)'; exit 1")
	a := NewAkira("west", []ServiceTarget{{Name: "cert-expiry", Exec: &ExecCheck{Argv: fail}}})
	reports := a.CheckAll()
	if len(reports) != 1 {
		t.Fatalf("got %d reports", len(reports))
	}
	r := reports[0]
	if r.Healthy || r.Service != "cert-expiry" || r.Reporter != "west" {
		t.Fatalf("report = %+v", r)
	}
	if !strings.Contains(r.Error, "exit 1") || !strings.Contains(r.Error, "ERROR: timeguru") {
		t.Fatalf("error should carry exit code and output tail, got %q", r.Error)
	}
	a.EvaluateConsensus()
	if got := a.GetStates()["cert-expiry"]; got.Failing != 1 {
		t.Fatalf("verdict = %+v", got)
	}
}

func TestAkira_ExecIntervalReusesResult(t *testing.T) {
	t.Parallel()
	count := filepath.Join(t.TempDir(), "count")
	script := shScript(t, "echo run >> "+count)
	a := NewAkira("west", []ServiceTarget{{Name: "cert-expiry", Exec: &ExecCheck{Argv: script, Interval: time.Hour}}})
	first := a.CheckAll()[0]
	time.Sleep(5 * time.Millisecond)
	second := a.CheckAll()[0]
	b, err := os.ReadFile(count) // #nosec G304 -- test temp file
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(b), "run"); n != 1 {
		t.Fatalf("command ran %d times within its interval, want 1", n)
	}
	if !second.Healthy || !second.Timestamp.After(first.Timestamp) {
		t.Fatalf("a reused result must be re-voted with a fresh timestamp: first %v second %v", first.Timestamp, second.Timestamp)
	}

	// Interval 0: every sweep runs it.
	b2 := NewAkira("west", []ServiceTarget{{Name: "c", Exec: &ExecCheck{Argv: script}}})
	b2.CheckAll()
	b2.CheckAll()
	b, _ = os.ReadFile(count) // #nosec G304 -- test temp file
	if n := strings.Count(string(b), "run"); n != 3 {
		t.Fatalf("interval 0: ran %d times in total, want 3", n)
	}
}
