// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

//go:build linux

package health

import (
	"context"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// asReaper makes this test process the reaper for its orphans, as Akira is
// when it runs as PID 1 in its container: a killed check's grandchildren are
// reparented to it, not to the host's init. Not parallel: the setting is
// process-wide, and parallel tests start only after this one returns.
func asReaper(t *testing.T) {
	t.Helper()
	if err := unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0); err != nil {
		t.Skipf("PR_SET_CHILD_SUBREAPER: %v", err)
	}
	t.Cleanup(func() { _ = unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 0, 0, 0, 0) })
}

// leftovers counts this process's children (running or zombie) once the
// check has returned, then kills and reaps them so one failure does not
// leak into the next test.
func leftovers(t *testing.T) int {
	t.Helper()
	time.Sleep(300 * time.Millisecond) // let a late orphan finish dying
	proc := os.DirFS("/proc/self/task")
	tasks, err := fs.Glob(proc, "*/children")
	if err != nil || len(tasks) == 0 {
		t.Skip("no /proc/self/task/*/children on this kernel")
	}
	n := 0
	for _, f := range tasks {
		b, err := fs.ReadFile(proc, f)
		if err != nil {
			continue
		}
		for _, s := range strings.Fields(string(b)) {
			if pid, err := strconv.Atoi(s); err == nil {
				n++
				_ = unix.Kill(pid, unix.SIGKILL)
			}
		}
	}
	for {
		var ws unix.WaitStatus
		pid, err := unix.Wait4(-1, &ws, 0, nil)
		if err != nil || pid <= 0 {
			return n
		}
	}
}

// 2026-11-16: 23 zombies (bash, openssl, cut, find, date) under Akira on
// west, one batch per cert-expiry timeout: the group kill reparents the
// script's children to PID 1, and os/exec waits only for its own child.
func TestRunExec_TimeoutLeavesNoZombiesForTheReaper(t *testing.T) {
	asReaper(t)
	r := RunExec(context.Background(), ExecCheck{
		Argv:    shScript(t, "sleep 30 & sleep 30 & wait"),
		Timeout: 300 * time.Millisecond,
	})
	if !r.TimedOut {
		t.Fatalf("want timed out, got %+v", r)
	}
	if n := leftovers(t); n != 0 {
		t.Fatalf("%d killed check processes left unreaped", n)
	}
}

// A check that exits but leaves a background child holding the output pipe
// ends with that child killed and reaped, not running on as an orphan.
func TestRunExec_ExitedCheckLeavesNothingBehind(t *testing.T) {
	asReaper(t)
	start := time.Now()
	r := RunExec(context.Background(), ExecCheck{
		Argv:    shScript(t, "sleep 30 & echo done"),
		Timeout: 10 * time.Second,
	})
	if r.TimedOut {
		t.Fatalf("did not want a timeout: %+v", r)
	}
	if el := time.Since(start); el > 5*time.Second {
		t.Fatalf("took %v", el)
	}
	if n := leftovers(t); n != 0 {
		t.Fatalf("%d check processes left unreaped", n)
	}
}

// A clean check returns without waiting on a group that is already gone.
func TestRunExec_CleanCheckIsNotSlowed(t *testing.T) {
	asReaper(t)
	start := time.Now()
	r := RunExec(context.Background(), ExecCheck{Argv: shScript(t, "true")})
	if !r.Healthy() {
		t.Fatalf("want healthy, got %+v", r)
	}
	if el := time.Since(start); el > time.Second {
		t.Fatalf("clean check took %v", el)
	}
	if n := leftovers(t); n != 0 {
		t.Fatalf("%d left unreaped", n)
	}
}
