// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package registry

import (
	"sync"
	"testing"
)

func TestRegistry_RegisterAndLookup(t *testing.T) {
	r := New[uint16, string]("test-codes", nil)

	if err := r.Register(0x01, "hello"); err != nil {
		t.Fatalf("Register error: %v", err)
	}

	v, ok := r.Lookup(0x01)
	if !ok || v != "hello" {
		t.Errorf("Lookup(0x01) = (%q, %v), want (\"hello\", true)", v, ok)
	}

	_, ok = r.Lookup(0xFF)
	if ok {
		t.Error("Lookup(0xFF) should return false for unregistered key")
	}
}

func TestRegistry_DuplicateReject(t *testing.T) {
	r := New[uint8, int]("dup-test", nil)
	r.MustRegister(0x01, 100)

	err := r.Register(0x01, 200)
	if err == nil {
		t.Error("Register duplicate key should return error")
	}

	// Original value unchanged.
	v, _ := r.Lookup(0x01)
	if v != 100 {
		t.Errorf("original value changed after duplicate attempt: got %d, want 100", v)
	}
}

func TestRegistry_RangePolicy(t *testing.T) {
	// Policy: reject keys >= 0x80 (reserved range).
	policy := func(key uint8) error {
		if key >= 0x80 {
			return ErrKeyInReservedRange
		}
		return nil
	}

	r := New[uint8, string]("policy-test", policy)

	if err := r.Register(0x01, "allowed"); err != nil {
		t.Fatalf("Register(0x01) error: %v", err)
	}

	err := r.Register(0x80, "blocked")
	if err == nil {
		t.Error("Register(0x80) should be rejected by policy")
	}
}

func TestRegistry_Freeze(t *testing.T) {
	r := New[uint16, string]("freeze-test", nil)
	r.MustRegister(0x01, "before-freeze")

	r.Freeze()

	if !r.IsFrozen() {
		t.Error("registry should be frozen after Freeze()")
	}

	err := r.Register(0x02, "after-freeze")
	if err == nil {
		t.Error("Register after Freeze should return error")
	}

	// Existing entries still accessible.
	v, ok := r.Lookup(0x01)
	if !ok || v != "before-freeze" {
		t.Error("frozen registry should still allow lookups")
	}
}

func TestRegistry_Range(t *testing.T) {
	r := New[uint8, int]("range-test", nil)
	r.MustRegister(1, 10)
	r.MustRegister(2, 20)
	r.MustRegister(3, 30)

	sum := 0
	r.Range(func(k uint8, v int) bool {
		sum += v
		return true
	})
	if sum != 60 {
		t.Errorf("Range sum = %d, want 60", sum)
	}
}

func TestRegistry_RangeEarlyStop(t *testing.T) {
	r := New[uint8, int]("early-stop", nil)
	r.MustRegister(1, 10)
	r.MustRegister(2, 20)
	r.MustRegister(3, 30)

	count := 0
	r.Range(func(k uint8, v int) bool {
		count++
		return count < 2 // Stop after 2 entries.
	})
	if count != 2 {
		t.Errorf("Range early stop count = %d, want 2", count)
	}
}

func TestRegistry_Len(t *testing.T) {
	r := New[uint16, string]("len-test", nil)
	if r.Len() != 0 {
		t.Errorf("empty Len() = %d, want 0", r.Len())
	}

	r.MustRegister(0x01, "a")
	r.MustRegister(0x02, "b")

	if r.Len() != 2 {
		t.Errorf("Len() = %d, want 2", r.Len())
	}
}

func TestRegistry_Keys(t *testing.T) {
	r := New[uint8, string]("keys-test", nil)
	r.MustRegister(1, "a")
	r.MustRegister(2, "b")
	r.MustRegister(3, "c")

	keys := r.Keys()
	if len(keys) != 3 {
		t.Fatalf("Keys() len = %d, want 3", len(keys))
	}

	// Verify all keys present (order not guaranteed).
	found := map[uint8]bool{}
	for _, k := range keys {
		found[k] = true
	}
	for _, want := range []uint8{1, 2, 3} {
		if !found[want] {
			t.Errorf("Keys() missing %d", want)
		}
	}
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	r := New[uint16, int]("concurrent-test", nil)

	// Pre-populate.
	for i := uint16(0); i < 100; i++ {
		r.MustRegister(i, int(i))
	}

	// Concurrent reads and writes.
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			// Readers.
			for j := uint16(0); j < 100; j++ {
				r.Lookup(j)
			}
			r.Range(func(k uint16, v int) bool { return true })
			r.Len()
			r.Keys()
		}(i)
	}

	wg.Wait()
	// No panic = success. Race detector would catch issues.
}

func TestRegistry_MustRegister_Panics(t *testing.T) {
	r := New[uint8, string]("panic-test", nil)
	r.MustRegister(0x01, "first")

	defer func() {
		if recover() == nil {
			t.Error("MustRegister(duplicate) should panic")
		}
	}()

	r.MustRegister(0x01, "duplicate")
}

func TestRegistry_Name(t *testing.T) {
	r := New[uint8, int]("Unheaded Error Codes", nil)
	if r.Name() != "Unheaded Error Codes" {
		t.Errorf("Name() = %q, want %q", r.Name(), "Unheaded Error Codes")
	}
}
