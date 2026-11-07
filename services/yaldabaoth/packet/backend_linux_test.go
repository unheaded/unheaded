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
// included. A build tree is not the deb's install, so the lab reaches it through YALD_OBJ
// and says so with YALD_ALLOW_UNTRUSTED_OBJ.
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

func TestLinuxBackendLoad_OptOutSkipsOnlyTheCheck(t *testing.T) {
	dir := t.TempDir()
	path, pins := filepath.Join(dir, "yaldabaoth-ebpf"), filepath.Join(dir, "pins")
	if err := os.WriteFile(path, []byte("not an object"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := (&LinuxBackend{AllowUntrustedObject: true}).Load(path, pins)
	if err == nil || errors.Is(err, objtrust.ErrUntrusted) {
		t.Fatalf("err = %v, want the object itself refused", err)
	}
}

func TestNewLinuxBackend_OptOutComesFromTheEnvironment(t *testing.T) {
	for value, want := range map[string]bool{"": false, "0": false, "true": false, "1": true} {
		t.Setenv("YALD_ALLOW_UNTRUSTED_OBJ", value)
		if got := NewLinuxBackend().AllowUntrustedObject; got != want {
			t.Errorf("YALD_ALLOW_UNTRUSTED_OBJ=%q: AllowUntrustedObject = %v, want %v", value, got, want)
		}
	}
}
