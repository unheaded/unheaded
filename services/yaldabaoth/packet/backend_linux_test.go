// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

//go:build linux

package packet

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"unheaded/pkg/ebpf/objtrust"
)

// L17: the chaos object is checked before anything happens, pin directory
// included. There is no opt-out: it comes from a package or a root install.
func TestLinuxBackendLoad_ChecksTheObject(t *testing.T) {
	dir := t.TempDir()
	path, pins := filepath.Join(dir, "yaldabaoth-ebpf"), filepath.Join(dir, "pins")
	if err := os.WriteFile(path, []byte("not an object"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := (&LinuxBackend{}).Load(path, pins); !errors.Is(err, objtrust.ErrUntrusted) {
		t.Fatalf("err = %v, want objtrust.ErrUntrusted", err)
	}
	if _, err := os.Stat(pins); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pin dir created before the check: %v", err)
	}
}
