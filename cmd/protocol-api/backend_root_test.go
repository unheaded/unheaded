// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	cebpf "github.com/cilium/ebpf"
)

// A real pinned map through BPFBackend (needs root; run the compiled test
// binary under sudo).
func TestBPFBackend_RealPinnedMap(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root (CAP_BPF)")
	}
	dir, err := os.MkdirTemp("/sys/fs/bpf", "wire-honest-api-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	cm, err := cebpf.NewMap(&cebpf.MapSpec{Type: cebpf.Hash, KeySize: 2, ValueSize: 32, MaxEntries: 4})
	if err != nil {
		t.Fatal(err)
	}
	defer cm.Close()
	if err := os.MkdirAll(filepath.Join(dir, "hop-ebpf"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := cm.Pin(filepath.Join(dir, mapSophia)); err != nil {
		t.Fatal(err)
	}
	b := NewBPFBackend(dir)
	key, val := []byte{3, 1}, bytes.Repeat([]byte{7}, 32)
	if got, err := b.ReadMap(mapSophia, key); err != nil || got != nil {
		t.Fatalf("ReadMap absent = %v, %v; want nil, nil", got, err)
	}
	if err := b.WriteMap(mapSophia, key, val); err != nil {
		t.Fatal(err)
	}
	got, err := b.ReadMap(mapSophia, key)
	if err != nil || !bytes.Equal(got, val) {
		t.Fatalf("ReadMap = %v, %v", got, err)
	}
	n := 0
	if err := b.IterateMap(mapSophia, func(k, v []byte) error { n++; return nil }); err != nil || n != 1 {
		t.Fatalf("IterateMap n=%d err=%v", n, err)
	}
	if err := b.DeleteMap(mapSophia, key); err != nil {
		t.Fatal(err)
	}
}
