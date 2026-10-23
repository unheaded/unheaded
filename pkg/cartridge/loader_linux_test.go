// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package cartridge

import (
	"os"
	"path/filepath"
	"testing"
)

// objPath is the built cartridge-ebpf object (CARTRIDGE_OBJ overrides).
func objPath(t *testing.T) string {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("needs root (CAP_BPF)")
	}
	p := os.Getenv("CARTRIDGE_OBJ")
	if p == "" {
		p = filepath.Join("..", "..", "ebpf", "target", "bpfel-unknown-none", "release", "cartridge-ebpf")
	}
	if _, err := os.Stat(p); err != nil {
		t.Skipf("cartridge-ebpf not built (cargo build --release -p cartridge-ebpf in ebpf/): %v", err)
	}
	return p
}

// VerifierBudget is the ADR-108 per-program gate.
const VerifierBudget = 500_000

// TestLoad_VerifierBudget (root-gated, S5 Steps 281-282): both programs load
// and each stays within half the 1M ceiling.
func TestLoad_VerifierBudget(t *testing.T) {
	c, err := Load(objPath(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for _, p := range []string{"cart_xdp", "cart_tick"} {
		n, ok := c.Verifier[p]
		if !ok {
			t.Fatalf("%s: no verifier stats", p)
		}
		t.Logf("VERIFIER %s processed %d insns (gate %d)", p, n, VerifierBudget)
		if n > VerifierBudget {
			t.Errorf("%s: %d processed insns > %d", p, n, VerifierBudget)
		}
	}
}
