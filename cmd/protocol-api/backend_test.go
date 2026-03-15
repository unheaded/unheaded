// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package main

import (
	"testing"
)

func TestMockBackendMode(t *testing.T) {
	b := NewMockBackend()
	if b.Mode() != "mock" {
		t.Errorf("Mode() = %q, want %q", b.Mode(), "mock")
	}
}

func TestBPFBackendMode(t *testing.T) {
	b := NewBPFBackend("/sys/fs/bpf/test")
	if b.Mode() != "bpf" {
		t.Errorf("Mode() = %q, want %q", b.Mode(), "bpf")
	}
}

func TestMockBackendReadWriteDelete(t *testing.T) {
	b := NewMockBackend()

	// Read from nonexistent map returns (nil, nil).
	val, err := b.ReadMap("flows", []byte("key1"))
	if err != nil {
		t.Fatalf("ReadMap error: %v", err)
	}
	if val != nil {
		t.Fatalf("ReadMap of nonexistent key: got %v, want nil", val)
	}

	// Write and read back.
	if err := b.WriteMap("flows", []byte("key1"), []byte("value1")); err != nil {
		t.Fatalf("WriteMap error: %v", err)
	}

	val, err = b.ReadMap("flows", []byte("key1"))
	if err != nil {
		t.Fatalf("ReadMap error: %v", err)
	}
	if string(val) != "value1" {
		t.Errorf("ReadMap = %q, want %q", val, "value1")
	}

	// Returned slice should be a copy.
	val[0] = 'X'
	val2, _ := b.ReadMap("flows", []byte("key1"))
	if string(val2) != "value1" {
		t.Errorf("ReadMap returned mutable reference to internal data")
	}

	// Delete and verify.
	if err := b.DeleteMap("flows", []byte("key1")); err != nil {
		t.Fatalf("DeleteMap error: %v", err)
	}

	val, err = b.ReadMap("flows", []byte("key1"))
	if err != nil {
		t.Fatalf("ReadMap after delete error: %v", err)
	}
	if val != nil {
		t.Errorf("ReadMap after delete = %v, want nil", val)
	}
}

func TestMockBackendIterate(t *testing.T) {
	b := NewMockBackend()

	// Iterate empty map should succeed.
	count := 0
	if err := b.IterateMap("empty", func(k, v []byte) error { count++; return nil }); err != nil {
		t.Fatalf("IterateMap error: %v", err)
	}
	if count != 0 {
		t.Errorf("IterateMap on empty map visited %d entries", count)
	}

	// Write some entries and iterate.
	b.WriteMap("test", []byte("a"), []byte("1"))
	b.WriteMap("test", []byte("b"), []byte("2"))
	b.WriteMap("test", []byte("c"), []byte("3"))

	seen := make(map[string]string)
	if err := b.IterateMap("test", func(k, v []byte) error {
		seen[string(k)] = string(v)
		return nil
	}); err != nil {
		t.Fatalf("IterateMap error: %v", err)
	}

	if len(seen) != 3 {
		t.Errorf("IterateMap visited %d entries, want 3", len(seen))
	}
	if seen["a"] != "1" || seen["b"] != "2" || seen["c"] != "3" {
		t.Errorf("IterateMap entries: %v", seen)
	}
}

func TestMockBackendDeleteNonexistent(t *testing.T) {
	b := NewMockBackend()

	// Delete from nonexistent map should not error.
	if err := b.DeleteMap("nope", []byte("key")); err != nil {
		t.Errorf("DeleteMap on nonexistent map: %v", err)
	}
}

func TestBPFBackendStubsReturnErrors(t *testing.T) {
	b := NewBPFBackend("/sys/fs/bpf/test")

	if _, err := b.ReadMap("flows", []byte("key")); err == nil {
		t.Error("BPFBackend.ReadMap should return error (not implemented)")
	}
	if err := b.WriteMap("flows", []byte("key"), []byte("val")); err == nil {
		t.Error("BPFBackend.WriteMap should return error (not implemented)")
	}
	if err := b.DeleteMap("flows", []byte("key")); err == nil {
		t.Error("BPFBackend.DeleteMap should return error (not implemented)")
	}
	if err := b.IterateMap("flows", func(k, v []byte) error { return nil }); err == nil {
		t.Error("BPFBackend.IterateMap should return error (not implemented)")
	}
}

func TestIsMockModeDefault(t *testing.T) {
	// Save and restore global.
	saved := globalBackend
	defer func() { globalBackend = saved }()

	globalBackend = nil
	if !IsMockMode() {
		t.Error("IsMockMode() with nil backend should return true")
	}

	globalBackend = NewMockBackend()
	if !IsMockMode() {
		t.Error("IsMockMode() with MockBackend should return true")
	}

	globalBackend = NewBPFBackend("/tmp/test")
	if IsMockMode() {
		t.Error("IsMockMode() with BPFBackend should return false")
	}
}

func TestBackendInterfaceCompliance(t *testing.T) {
	// Compile-time check that both types satisfy Backend.
	var _ Backend = (*MockBackend)(nil)
	var _ Backend = (*BPFBackend)(nil)
}
