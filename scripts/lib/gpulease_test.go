// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package scriptlib

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/gpulease"
)

// gpu-lease.sh and pkg/gpulease share one lock file and its semantics
// (S-TWO-BRAINS Step 234): the shell side is what switch-model.sh and
// forge-train.sh take, the Go side what the worker and pkg/localllm take.

func lease(t *testing.T, lock string, args ...string) (int, string) {
	t.Helper()
	if _, err := exec.LookPath("flock"); err != nil {
		t.Skip("no flock")
	}
	cmd := exec.Command("bash", append([]string{"gpu-lease.sh"}, args...)...)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "GPU_LOCK=" + lock}
	out, err := cmd.CombinedOutput()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), string(out)
	}
	if err != nil {
		t.Fatal(err)
	}
	return 0, string(out)
}

func TestGPULeaseShell_RunsCommandAndPassesExit(t *testing.T) {
	lock := filepath.Join(t.TempDir(), "u", "gpu.lock")
	if code, out := lease(t, lock, "shared", "5", "--", "echo", "hi"); code != 0 || strings.TrimSpace(out) != "hi" {
		t.Errorf("shared: %d %q", code, out)
	}
	if code, _ := lease(t, lock, "exclusive", "5", "--", "sh", "-c", "exit 7"); code != 7 {
		t.Errorf("exit status not passed through: %d", code)
	}
	if fi, err := os.Stat(filepath.Dir(lock)); err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("lock dir: %v %v", fi, err)
	}
}

func TestGPULeaseShell_GoExclusiveBlocksShellShared(t *testing.T) {
	lock := filepath.Join(t.TempDir(), "u", "gpu.lock")
	ex, err := gpulease.Exclusive(context.Background(), lock, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Now()
	if code, _ := lease(t, lock, "shared", "1", "--", "true"); code != 75 {
		t.Errorf("shared under a Go exclusive lease: exit %d, want 75", code)
	}
	if time.Since(t0) < 900*time.Millisecond {
		t.Error("timed out too early")
	}
	if code, _ := lease(t, lock, "probe", "0"); code != 75 {
		t.Errorf("probe under exclusive: %d", code)
	}
	_ = ex.Release()
	if code, _ := lease(t, lock, "probe", "0"); code != 0 {
		t.Errorf("probe free: %d", code)
	}
}

func TestGPULeaseShell_ShellExclusiveBlocksGoShared(t *testing.T) {
	lock := filepath.Join(t.TempDir(), "u", "gpu.lock")
	if _, err := exec.LookPath("flock"); err != nil {
		t.Skip("no flock")
	}
	cmd := exec.Command("bash", "gpu-lease.sh", "exclusive", "5", "--", "sleep", "1")
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "GPU_LOCK=" + lock}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Wait() }()
	deadline := time.Now().Add(3 * time.Second)
	for !gpulease.Busy(lock) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if !gpulease.Busy(lock) {
		t.Fatal("the shell's exclusive lease is not seen by Go")
	}
	if _, err := gpulease.Shared(context.Background(), lock, 200*time.Millisecond); !errors.Is(err, gpulease.ErrTimeout) {
		t.Errorf("Go shared under the shell's exclusive lease: %v", err)
	}
}

func TestGPULeaseShell_ChildDoesNotInheritTheLock(t *testing.T) {
	// switch-model.sh nohups llama-server inside its lease: the server must
	// not keep the GPU lease after the script ends.
	dir := t.TempDir()
	lock := filepath.Join(dir, "u", "gpu.lock")
	pidf := filepath.Join(dir, "pid")
	script := "sleep 30 </dev/null >/dev/null 2>&1 & echo $! > " + pidf
	if code, out := lease(t, lock, "exclusive", "5", "--", "bash", "-c", script); code != 0 {
		t.Fatalf("exit %d %s", code, out)
	}
	b, _ := os.ReadFile(pidf)
	if pid := strings.TrimSpace(string(b)); pid != "" {
		defer func() { _ = exec.Command("kill", pid).Run() }()
	}
	if gpulease.Busy(lock) {
		t.Error("a background child kept the GPU lease after its parent's lease ended")
	}
}

func TestGPULeaseShell_NestedInsideExclusiveRunsDirectly(t *testing.T) {
	lock := filepath.Join(t.TempDir(), "u", "gpu.lock")
	// forge-train.sh (exclusive) calls switch-model.sh (exclusive): no deadlock.
	code, out := lease(t, lock, "exclusive", "2", "--", "bash", "gpu-lease.sh", "exclusive", "1", "--", "echo", "nested")
	if code != 0 || strings.TrimSpace(out) != "nested" {
		t.Errorf("nested: %d %q", code, out)
	}
}

func TestGPULeaseShell_BadUsage(t *testing.T) {
	lock := filepath.Join(t.TempDir(), "u", "gpu.lock")
	for _, a := range [][]string{{}, {"shared"}, {"shared", "x", "--", "true"}, {"weird", "1", "--", "true"}, {"shared", "1", "true"}} {
		if code, _ := lease(t, lock, a...); code != 2 {
			t.Errorf("%v: exit %d", a, code)
		}
	}
}
