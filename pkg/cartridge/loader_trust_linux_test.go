// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

//go:build linux

package cartridge

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"unheaded/pkg/ebpf/objtrust"
)

// L17: the interpreter object is checked unless the caller opts out.
func TestLoad_ChecksTheObject(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cartridge-ebpf")
	if err := os.WriteFile(path, []byte("not an object"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path, Options{}); !errors.Is(err, objtrust.ErrUntrusted) {
		t.Fatalf("err = %v, want objtrust.ErrUntrusted", err)
	}
	if _, err := Load(path, Options{AllowUntrustedObject: true}); err == nil || errors.Is(err, objtrust.ErrUntrusted) {
		t.Fatalf("with the opt-out: err = %v, want a parse failure", err)
	}
}
