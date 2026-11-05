// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

//go:build linux

package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"unheaded/pkg/ebpf/objtrust"
)

// L17: --bpf-dir is held to the same rule as every other load path.
func TestLoadObject_ChecksTheObject(t *testing.T) {
	path := filepath.Join(t.TempDir(), "heimdall.o")
	if err := os.WriteFile(path, []byte("not an object"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadObject(path); !errors.Is(err, objtrust.ErrUntrusted) {
		t.Fatalf("err = %v, want objtrust.ErrUntrusted", err)
	}
}
