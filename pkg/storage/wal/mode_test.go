// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

//go:build unix

package wal

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// openUmask clears the umask for one test, so the mode a file gets is the mode
// the code asked for. Not for a parallel test: the umask is per process.
func openUmask(t *testing.T) {
	t.Helper()
	old := syscall.Umask(0)
	t.Cleanup(func() { syscall.Umask(old) })
}

func assertOwnerOnly(t *testing.T, path string) {
	t.Helper()
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := st.Mode().Perm(); perm&0o077 != 0 {
		t.Fatalf("segment mode %04o, want no group or other bits", perm)
	}
}

// A segment holds message payloads, so its mode must not depend on the umask
// or the directory of whoever runs the store.
func TestCreateSegment_OwnerOnly(t *testing.T) {
	openUmask(t)
	path := filepath.Join(t.TempDir(), "seg.wal")
	s, err := CreateSegment(path, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	assertOwnerOnly(t, path)
}

// A segment written before that rule is narrowed when it is next opened.
func TestOpenSegment_NarrowsAnOlderSegment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seg.wal")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Chmod(0o644); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := OpenSegment(path, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	assertOwnerOnly(t, path)
}
