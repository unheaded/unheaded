// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package scriptlib

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"unheaded/pkg/gpulease"
)

// switch-model.sh takes the exclusive GPU lease before anything else
// (S-TWO-BRAINS Step 235). None of these cases can reach a model launch:
// they stop at the lease, at usage, or at an unused port.

func runScript(t *testing.T, script string, env []string, args ...string) int {
	t.Helper()
	if _, err := exec.LookPath("flock"); err != nil {
		t.Skip("no flock")
	}
	cmd := exec.Command("bash", append([]string{filepath.Join("..", script)}, args...)...)
	cmd.Env = append([]string{"PATH=" + os.Getenv("PATH")}, env...)
	out, err := cmd.CombinedOutput()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		t.Logf("%s: exit %d\n%s", script, ee.ExitCode(), out)
		return ee.ExitCode()
	}
	if err != nil {
		t.Fatal(err)
	}
	return 0
}

func TestSwitchModel_WaitsForTheLease(t *testing.T) {
	lock := filepath.Join(t.TempDir(), "u", "gpu.lock")
	ex, err := gpulease.Exclusive(context.Background(), lock, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ex.Release() }()
	// LLAMA_BIN points nowhere: if the lease did not hold, the launch fails
	// loudly with another code.
	code := runScript(t, "switch-model.sh", []string{"GPU_LOCK=" + lock, "GPU_LEASE_TIMEOUT=1", "LLAMA_BIN=/nonexistent", "LLAMA_PORT=9"}, "qwen-7b")
	if code != 75 {
		t.Errorf("switch-model.sh under a held lease: exit %d, want 75", code)
	}
}

func TestSwitchModel_UsageAndStopUnderTheLease(t *testing.T) {
	if _, err := exec.LookPath("ss"); err != nil {
		t.Skip("no ss")
	}
	lock := filepath.Join(t.TempDir(), "u", "gpu.lock")
	env := []string{"GPU_LOCK=" + lock, "GPU_LEASE_TIMEOUT=5", "LLAMA_BIN=/nonexistent", "LLAMA_PORT=9"}
	if code := runScript(t, "switch-model.sh", env); code != 2 {
		t.Errorf("no key: exit %d, want 2", code)
	}
	if code := runScript(t, "switch-model.sh", env, "--stop"); code != 0 {
		t.Errorf("--stop with nothing on the port: exit %d", code)
	}
	if gpulease.Busy(lock) {
		t.Error("the lease outlived switch-model.sh")
	}
}

func TestForgeTrain_WaitsForTheLease(t *testing.T) {
	lock := filepath.Join(t.TempDir(), "u", "gpu.lock")
	sh, err := gpulease.Shared(context.Background(), lock, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sh.Release() }()
	// An inference call holds the GPU; training does not start under it.
	code := runScript(t, "forge-train.sh", []string{"GPU_LOCK=" + lock, "GPU_LEASE_TIMEOUT=1", "FORGE_BIN=/nonexistent"}, "train")
	if code != 75 {
		t.Errorf("forge-train.sh under a shared lease: exit %d, want 75", code)
	}
}
