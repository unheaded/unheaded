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

// L17: the kprobe path takes the same check as the native loader, and the same
// opt-out, before it opens the object.
func TestLoadLatencyKprobes_ChecksTheObject(t *testing.T) {
	path := filepath.Join(t.TempDir(), "latency-probe")
	if err := os.WriteFile(path, []byte("not an object"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadLatencyKprobes(path, false); !errors.Is(err, objtrust.ErrUntrusted) {
		t.Fatalf("err = %v, want objtrust.ErrUntrusted", err)
	}
	if _, _, err := LoadLatencyKprobes(path, true); err == nil || errors.Is(err, objtrust.ErrUntrusted) {
		t.Fatalf("with the opt-out: err = %v, want a parse failure", err)
	}
}
