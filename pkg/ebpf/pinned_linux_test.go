// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package ebpf

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	cebpf "github.com/cilium/ebpf"
)

func TestOpenPinnedMap_MissingPath(t *testing.T) {
	if _, err := OpenPinnedMap("/sys/fs/bpf/does-not-exist-wire-honest"); err == nil {
		t.Fatal("OpenPinnedMap on a missing pin returned nil error")
	}
}

func TestPinnedMap_KeyAndValueSizeRefused(t *testing.T) {
	m := &PinnedMap{fd: -1, keySize: 4, valueSize: 8}
	if _, err := m.Lookup([]byte{1, 2}); !errors.Is(err, ErrKeySize) {
		t.Fatalf("Lookup short key: %v, want ErrKeySize", err)
	}
	if _, err := m.Lookup(nil); !errors.Is(err, ErrKeySize) {
		t.Fatalf("Lookup nil key: %v, want ErrKeySize", err)
	}
	if err := m.Update([]byte{1, 2, 3, 4}, []byte{1}); !errors.Is(err, ErrValueSize) {
		t.Fatalf("Update short value: %v, want ErrValueSize", err)
	}
	if err := m.Delete([]byte{1}); !errors.Is(err, ErrKeySize) {
		t.Fatalf("Delete short key: %v, want ErrKeySize", err)
	}
	if err := m.Close(); err != nil {
		t.Fatalf("Close on fd -1: %v", err)
	}
}

func pinTestMap(t *testing.T, typ cebpf.MapType) string {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("needs root (CAP_BPF)")
	}
	cm, err := cebpf.NewMap(&cebpf.MapSpec{Type: typ, KeySize: 4, ValueSize: 8, MaxEntries: 8})
	if err != nil {
		t.Fatalf("create map: %v", err)
	}
	t.Cleanup(func() { _ = cm.Close() })
	dir, err := os.MkdirTemp("/sys/fs/bpf", "wire-honest-")
	if err != nil {
		t.Fatalf("mkdir in bpffs: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "m")
	if err := cm.Pin(path); err != nil {
		t.Fatalf("pin: %v", err)
	}
	return path
}

func TestPinnedMap_RoundTrip(t *testing.T) {
	m, err := OpenPinnedMap(pinTestMap(t, cebpf.Hash))
	if err != nil {
		t.Fatalf("OpenPinnedMap: %v", err)
	}
	defer m.Close()
	if m.KeySize() != 4 || m.ValueSize() != 8 {
		t.Fatalf("sizes = %d/%d, want 4/8", m.KeySize(), m.ValueSize())
	}
	k := []byte{1, 0, 0, 0}
	v := []byte{9, 8, 7, 6, 5, 4, 3, 2}
	if got, err := m.Lookup(k); err != nil || got != nil {
		t.Fatalf("Lookup absent = %v, %v; want nil, nil", got, err)
	}
	if err := m.Update(k, v); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got, err := m.Lookup(k); err != nil || !bytes.Equal(got, v) {
		t.Fatalf("Lookup = %v, %v; want %v", got, err, v)
	}
	if err := m.Update([]byte{2, 0, 0, 0}, v); err != nil {
		t.Fatalf("Update 2: %v", err)
	}
	n := 0
	if err := m.Iterate(func(key, value []byte) error { n++; return nil }); err != nil {
		t.Fatalf("Iterate: %v", err)
	}
	if n != 2 {
		t.Fatalf("Iterate saw %d entries, want 2", n)
	}
	if err := m.Delete(k); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := m.Delete(k); err != nil {
		t.Fatalf("Delete absent: %v", err)
	}
}

func TestOpenPinnedMap_RefusesPerCPU(t *testing.T) {
	if _, err := OpenPinnedMap(pinTestMap(t, cebpf.PerCPUHash)); !errors.Is(err, ErrUnsupportedMapType) {
		t.Fatalf("OpenPinnedMap(percpu) = %v, want ErrUnsupportedMapType", err)
	}
}
