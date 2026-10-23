// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadObjects (root-gated, S5 Step 152 / H-G2): cilium/ebpf loads every
// Aya-built heimdall-bpf object and the verifier accepts every program.
// The processed-insn counts it logs are ADR-043 K1 evidence. Nothing is
// attached; closing the collection unloads it.
//
//	sudo -E go test ./cmd/heimdall-daemon/ -run TestLoadObjects -v
func TestLoadObjects(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root (CAP_BPF)")
	}
	dir := os.Getenv("HEIMDALL_BPF_DIR")
	if dir == "" {
		dir = filepath.Join("..", "..", "crates", "heimdall-bpf", "target", "bpfel-unknown-none", "release")
	}
	want := map[string]string{
		objVFSWrite: "heimdall_vfs_write",
		objExecve:   "heimdall_execve",
		objHornXDP:  "heimdall_gjallarhorn",
	}
	for obj, prog := range want {
		path := filepath.Join(dir, obj)
		if _, err := os.Stat(path); err != nil {
			t.Skipf("%s not built (cargo build --release in crates/heimdall-bpf): %v", obj, err)
		}
		coll, counts, err := loadObject(path)
		if err != nil {
			t.Fatalf("%s: %v", obj, err)
		}
		if _, ok := coll.Programs[prog]; !ok {
			t.Errorf("%s: program %s missing", obj, prog)
		}
		if counts[prog] == 0 {
			t.Errorf("%s: no verifier stats for %s", obj, prog)
		}
		t.Logf("K1 %s/%s: verifier processed %d insns", obj, prog, counts[prog])
		coll.Close()
	}
}
