// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package wal

import (
	"os"
	"path/filepath"
	"testing"
)

// A segment holds message payloads, so its mode must not depend on the umask
// or the directory of whoever runs the store.
func TestCreateSegment_OwnerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seg.wal")
	s, err := CreateSegment(path, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := st.Mode().Perm(); perm&0o077 != 0 {
		t.Fatalf("segment mode %04o, want no group or other bits", perm)
	}
}
