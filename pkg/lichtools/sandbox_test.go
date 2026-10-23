// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package lichtools

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// requireBareMetal skips a test unless LICH_BAREMETAL=1. These tests actually
// unshare namespaces and spawn processes, so they only run where that is
// allowed (the sprint's [BARE-METAL] marker).
func requireBareMetal(t *testing.T) {
	t.Helper()
	if os.Getenv("LICH_BAREMETAL") != "1" {
		t.Skip("set LICH_BAREMETAL=1 to run namespace/exec confinement tests")
	}
}

func TestNetnsSandbox_Fit(t *testing.T) {
	s := &netnsSandbox{safety: 1.2, memAvailKB: func() (int64, error) { return 100 * 1024, nil }} // 100 MiB free
	if err := s.fits(512 << 20); !errors.Is(err, ErrWontFit) {
		t.Fatalf("512MiB must not fit in 100MiB free, got %v", err)
	}
	s.memAvailKB = func() (int64, error) { return 4 * 1024 * 1024, nil } // 4 GiB free
	if err := s.fits(512 << 20); err != nil {
		t.Fatalf("512MiB should fit in 4GiB free, got %v", err)
	}
}

func TestNetnsSandbox_Spawn_RefusesWhenWontFit(t *testing.T) {
	s := &netnsSandbox{safety: 1.2, memAvailKB: func() (int64, error) { return 1024, nil }} // 1 MiB free
	_, err := s.Spawn(context.Background(), Target{Kind: KindHarness, Name: "001"}, Limits{MemBytes: 512 << 20})
	if !errors.Is(err, ErrWontFit) {
		t.Fatalf("Spawn should refuse with ErrWontFit, got %v", err)
	}
}

func TestNetnsSandbox_ConfineArgv(t *testing.T) {
	s := &netnsSandbox{unshareBin: "/usr/bin/unshare", prlimitBin: "/usr/bin/prlimit", shBin: "/bin/sh"}
	lim := Limits{PidsMax: 128, MemBytes: 256 << 20, WallTimeout: 30 * time.Second}
	got := strings.Join(s.confineArgv(lim, []string{"echo", "hi"}), " ")
	for _, want := range []string{"unshare", "-r", "-n", "-m", "--pid", "--mount-proc", "--fork", "/bin/sh -c", "remount,ro,bind /", "tmpfs /tmp", "prlimit", "--nproc=128", "--as=268435456", "echo hi"} {
		if !strings.Contains(got, want) {
			t.Errorf("confined argv missing %q: %s", want, got)
		}
	}
}

func TestNetnsSandbox_Prelude_WritableQuoted(t *testing.T) {
	s := &netnsSandbox{writable: []string{"/repo/tomb/lich/results"}}
	p := s.prelude()
	if !strings.Contains(p, "remount,ro,bind /") {
		t.Fatalf("prelude must lock root read-only: %s", p)
	}
	if !strings.Contains(p, "'/repo/tomb/lich/results'") {
		t.Fatalf("prelude must rebind the writable path (quoted): %s", p)
	}
}

func TestNewNetnsSandbox_AvailableHere(t *testing.T) {
	// unshare is present in this environment (checked in preflight); construction
	// should succeed. If this ever returns ErrNoConfinement, run_harness must
	// fail closed — which is the correct safe behaviour.
	if _, err := NewNetnsSandbox(1.2); err != nil && !errors.Is(err, ErrNoConfinement) {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- [BARE-METAL] confinement proofs ---

func bareMetalSandbox(t *testing.T) Sandbox {
	t.Helper()
	s, err := NewNetnsSandbox(1.2)
	if err != nil {
		t.Skipf("no confinement substrate: %v", err)
	}
	return s
}

func TestNetnsSandbox_NoEgress(t *testing.T) {
	requireBareMetal(t)
	s := bareMetalSandbox(t)
	h, err := s.Spawn(context.Background(), Target{Kind: KindHarness, Name: "001"}, Limits{})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer func() { _ = s.Destroy(h) }()
	res, err := s.Exec(context.Background(), h, []string{"sh", "-c", "ip route get 1.1.1.1"})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.ExitCode == 0 {
		t.Fatalf("egress route lookup should FAIL inside the sandbox; stdout=%q", res.Stdout)
	}
}

func TestNetnsSandbox_WallTimeout(t *testing.T) {
	requireBareMetal(t)
	s := bareMetalSandbox(t)
	h, err := s.Spawn(context.Background(), Target{Kind: KindHarness, Name: "001"}, Limits{WallTimeout: time.Second})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer func() { _ = s.Destroy(h) }()
	res, err := s.Exec(context.Background(), h, []string{"sh", "-c", "sleep 30"})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if !res.TimedOut {
		t.Fatalf("sleep 30 under a 1s wall timeout must be killed, got %+v", res)
	}
	if res.Duration > 10*time.Second {
		t.Fatalf("timeout took too long: %v", res.Duration)
	}
}

func TestNetnsSandbox_RlimitsApplied(t *testing.T) {
	requireBareMetal(t)
	s := bareMetalSandbox(t)
	if s.(*netnsSandbox).prlimitBin == "" {
		t.Skip("prlimit unavailable; rlimit proof needs it")
	}
	const memBytes = 512 << 20 // comfortable AS for the shell; 512 MiB
	h, err := s.Spawn(context.Background(), Target{Kind: KindHarness, Name: "001"}, Limits{PidsMax: 123, MemBytes: memBytes})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer func() { _ = s.Destroy(h) }()
	// Read /proc/self/limits — shell-agnostic (dash's ulimit lacks -u).
	res, err := s.Exec(context.Background(), h, []string{"cat", "/proc/self/limits"})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	nproc := limitSoft(res.Stdout, "Max processes")
	as := limitSoft(res.Stdout, "Max address space")
	if nproc != "123" {
		t.Fatalf("Max processes soft limit = %q, want 123; out=%q stderr=%q", nproc, res.Stdout, res.Stderr)
	}
	if v, perr := strconv.ParseInt(as, 10, 64); perr != nil || v != memBytes {
		t.Fatalf("Max address space soft limit = %q, want %d", as, int64(memBytes))
	}
}

// limitSoft returns the soft-limit column for a named line of /proc/self/limits.
func limitSoft(procLimits, name string) string {
	for _, line := range strings.Split(procLimits, "\n") {
		if strings.HasPrefix(line, name) {
			rest := strings.Fields(strings.TrimPrefix(line, name))
			if len(rest) >= 1 {
				return rest[0]
			}
		}
	}
	return ""
}

// TestNetnsSandbox_FSConfinement proves the mount-namespace confinement: a
// write to a host path outside the sandbox (HOME) FAILS (read-only root), while
// the private /tmp scratch is writable. This is the Step-67 "write outside the
// mount must fail" escape check.
func TestNetnsSandbox_FSConfinement(t *testing.T) {
	requireBareMetal(t)
	s := bareMetalSandbox(t)
	h, err := s.Spawn(context.Background(), Target{Kind: KindHarness, Name: "001"}, Limits{})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer func() { _ = s.Destroy(h) }()
	script := `if echo x > /tmp/scratch_ok 2>/dev/null; then echo TMPOK; else echo TMPNO; fi; ` +
		`if echo y > "$HOME/.lich_fs_escape_probe" 2>/dev/null; then echo HLEAK; rm -f "$HOME/.lich_fs_escape_probe"; else echo HBLOCK; fi`
	res, err := s.Exec(context.Background(), h, []string{"sh", "-c", script})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if !strings.Contains(res.Stdout, "TMPOK") {
		t.Fatalf("private /tmp scratch should be writable: out=%q err=%q", res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "HBLOCK") {
		t.Fatalf("writing outside the sandbox (HOME) must FAIL (read-only root): out=%q", res.Stdout)
	}
}

func TestNetnsSandbox_Teardown(t *testing.T) {
	requireBareMetal(t)
	s := bareMetalSandbox(t)
	h, err := s.Spawn(context.Background(), Target{Kind: KindHarness, Name: "001"}, Limits{WallTimeout: 30 * time.Second})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	// Start a long sleep in the background, then Destroy mid-flight.
	go func() { _, _ = s.Exec(context.Background(), h, []string{"sh", "-c", "sleep 30"}) }()
	time.Sleep(300 * time.Millisecond)
	if err := s.Destroy(h); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if !h.done {
		t.Fatalf("handle should be marked done after Destroy")
	}
}
