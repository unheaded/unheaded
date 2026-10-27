// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package scriptlib

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

// llama_pid matched LLAMA_PORT as an unanchored regex, so ":3901" also
// matched a listener on ":39017" and --stop killed it. Here the listener is
// this test process on an ephemeral port P, and LLAMA_PORT is P without its
// last digit: an exact match finds nothing on that port, so --stop exits 0
// and the test's own listener is still accepting afterwards.
func TestSwitchModel_StopMatchesThePortExactly(t *testing.T) {
	if _, err := exec.LookPath("ss"); err != nil {
		t.Skip("no ss")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	prefix := port / 10
	if prefix < 1 {
		t.Skipf("ephemeral port %d has no shorter prefix", port)
	}
	lock := filepath.Join(t.TempDir(), "u", "gpu.lock")
	env := []string{"GPU_LOCK=" + lock, "GPU_LEASE_TIMEOUT=5", "LLAMA_BIN=/nonexistent", "LLAMA_PORT=" + strconv.Itoa(prefix)}
	if code := runScript(t, "switch-model.sh", env, "--stop"); code != 0 {
		t.Errorf("--stop with LLAMA_PORT=%d (a prefix of the test's port %d): exit %d, want 0", prefix, port, code)
	}
	c, err := net.DialTimeout("tcp", ln.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatalf("the test's listener on :%d stopped accepting after --stop LLAMA_PORT=%d: %v", port, prefix, err)
	}
	_ = c.Close()
}

// The positive half: a listener in a child process on exactly LLAMA_PORT is
// the one --stop stops. The child is this test binary re-run as a helper.
func TestSwitchModel_StopStopsTheExactPort(t *testing.T) {
	if _, err := exec.LookPath("ss"); err != nil {
		t.Skip("no ss")
	}
	child := exec.Command(os.Args[0], "-test.run=^TestHelperListener$")
	child.Env = append(os.Environ(), "SWITCH_MODEL_HELPER_LISTENER=1")
	stdout, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- child.Wait() }()
	defer func() { _ = child.Process.Kill() }()
	var port int
	if _, err := fmt.Fscanf(stdout, "%d\n", &port); err != nil {
		t.Fatalf("helper port: %v", err)
	}
	lock := filepath.Join(t.TempDir(), "u", "gpu.lock")
	env := []string{"GPU_LOCK=" + lock, "GPU_LEASE_TIMEOUT=5", "LLAMA_BIN=/nonexistent", "LLAMA_PORT=" + strconv.Itoa(port)}
	if code := runScript(t, "switch-model.sh", env, "--stop"); code != 0 {
		t.Errorf("--stop on the helper's port %d: exit %d, want 0", port, code)
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatalf("--stop LLAMA_PORT=%d left the listener on that port running", port)
	}
}

// TestHelperListener is the child for TestSwitchModel_StopStopsTheExactPort;
// it does nothing unless that test starts it.
func TestHelperListener(t *testing.T) {
	if os.Getenv("SWITCH_MODEL_HELPER_LISTENER") != "1" {
		t.Skip("helper process only")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	fmt.Printf("%d\n", ln.Addr().(*net.TCPAddr).Port)
	time.Sleep(60 * time.Second)
}
