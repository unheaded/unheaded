// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package objtrust

import (
	"errors"
	"os"
	"testing"
)

// The real stat path: /etc/passwd sits in a root-owned, non-writable chain
// on any Linux host; a file this test creates never does (owned by the test
// user, or, under root, inside the world-writable temp dir).
func TestCheck_RealFilesystem(t *testing.T) {
	if err := Check("/etc/passwd"); err != nil {
		t.Errorf("/etc/passwd: %v", err)
	}
	p := t.TempDir() + "/prog.o"
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Check(p); !errors.Is(err, ErrUntrusted) {
		t.Errorf("%s: err = %v, want ErrUntrusted", p, err)
	}
}
