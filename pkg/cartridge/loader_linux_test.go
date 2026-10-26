// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package cartridge

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
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

// TestLoad_EventsGroupPin (root-gated): EventsGID opens the CART_EVENTS pin
// to its group and nothing else; OpenEvents drains it without the other
// maps.
func TestLoad_EventsGroupPin(t *testing.T) {
	obj := objPath(t)
	pin := filepath.Join("/sys/fs/bpf", fmt.Sprintf("unheaded-test-%d", os.Getpid()))
	const gid = 16734 // pkg/uids.Anomaly; need not exist on the host
	c, err := Load(obj, Options{PinDir: pin, EventsGID: gid})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	defer func() { _ = c.Unpin() }()
	for _, n := range MapNames {
		st, err := os.Stat(filepath.Join(pin, n))
		if err != nil {
			t.Fatal(err)
		}
		mode, g := st.Mode().Perm(), st.Sys().(*syscall.Stat_t).Gid
		want, wantG := os.FileMode(0o600), uint32(0)
		if n == "CART_EVENTS" {
			want, wantG = 0o660, gid
		}
		if mode != want || g != wantG {
			t.Errorf("%s: %v gid %d, want %v gid %d", n, mode, g, want, wantG)
		}
	}
	// MapNames and ProgramNames must cover every pin, or Open misses a map
	// and Unpin through it (upc-cartridgectl down) leaves the pin behind.
	ents, err := os.ReadDir(pin)
	if err != nil {
		t.Fatal(err)
	}
	if want := len(MapNames) + len(ProgramNames); len(ents) != want {
		t.Errorf("%d pins, MapNames+ProgramNames name %d", len(ents), want)
	}
	ev, err := OpenEvents(pin)
	if err != nil {
		t.Fatal(err)
	}
	defer ev.Close()
	rd, err := ev.Events()
	if err != nil {
		t.Fatal(err)
	}
	defer rd.Close()
	if _, err := ev.DrainEvents(rd); err != nil {
		t.Fatal(err)
	}
}
