// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

//go:build usersystemd

package scriptlib

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"unheaded/pkg/gpulease"
)

// forge-train.sh holds the exclusive GPU lease for its whole run (a user
// systemd scope is needed: go test -tags usersystemd ./scripts/lib/).
func TestForgeTrain_HoldsExclusiveLeaseForTheRun(t *testing.T) {
	dir := t.TempDir()
	lock := filepath.Join(dir, "u", "gpu.lock")
	bin := filepath.Join(dir, "forge")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nsleep 2\necho trained \"$@\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", filepath.Join("..", "forge-train.sh"), "train", "--fake")
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME"), "XDG_RUNTIME_DIR=" + os.Getenv("XDG_RUNTIME_DIR"),
		"DBUS_SESSION_BUS_ADDRESS=" + os.Getenv("DBUS_SESSION_BUS_ADDRESS"),
		"GPU_LOCK=" + lock, "FORGE_BIN=" + bin, "FORGE_LOG=" + filepath.Join(dir, "forge.log"), "FORGE_MIN_FREE_G=0",
		"FORGE_GPU_HANDOFF=0", "FORGE_MAX_SEC=60"}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(700 * time.Millisecond)
	busy := gpulease.Busy(lock)
	if err := cmd.Wait(); err != nil {
		t.Fatalf("forge-train.sh: %v", err)
	}
	if !busy {
		t.Error("the GPU lease was not held exclusively during training")
	}
	if gpulease.Busy(lock) {
		t.Error("the lease outlived the training run")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "forge.log")); len(b) == 0 {
		t.Error("no training output")
	}
}
