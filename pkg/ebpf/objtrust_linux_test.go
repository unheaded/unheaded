// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package ebpf

import (
	"errors"
	"os"
	"testing"
)

// The real stat path: /etc/passwd sits in a root-owned, non-writable chain
// on any Linux host; a file this test creates never does (owned by the test
// user, or, under root, inside the world-writable temp dir).
func TestCheckObjectTrust_RealFilesystem(t *testing.T) {
	if err := checkObjectTrust("/etc/passwd"); err != nil {
		t.Errorf("/etc/passwd: %v", err)
	}
	p := t.TempDir() + "/prog.o"
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := checkObjectTrust(p); !errors.Is(err, ErrUntrustedObject) {
		t.Errorf("%s: err = %v, want ErrUntrustedObject", p, err)
	}
}
