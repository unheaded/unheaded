// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package kv

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"unheaded/pkg/storage"
)

// --- Factory helpers ---

// newMemoryStore returns a MemoryStore with default config for testing.
func newMemoryStore(t *testing.T) *MemoryStore {
	t.Helper()
	cfg := DefaultConfig()
	cfg.Backend = BackendMemory
	return NewMemoryStore(cfg)
}

// --- New / factory tests ---

func TestNew_Memory(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Backend = BackendMemory
	store, err := New(cfg)
	if err != nil {
		t.Fatalf("New(memory): %v", err)
	}
	defer store.Close()
}

func TestNew_Badger(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Backend = BackendBadger
	cfg.Path = t.TempDir()
	store, err := New(cfg)
	if err != nil {
		t.Fatalf("New(badger): %v", err)
	}
	defer store.Close()
}

func TestNew_Bolt(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Backend = BackendBolt
	cfg.Path = filepath.Join(t.TempDir(), "test.db")
	store, err := New(cfg)
	if err != nil {
		t.Fatalf("New(bolt): %v", err)
	}
	defer store.Close()
}

func TestNew_UnknownBackend(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Backend = "unknown"
	_, err := New(cfg)
	if err == nil {
		t.Fatal("expected error for unknown backend")
	}
}

// --- Basic CRUD tests (memory backend) ---

func TestMemory_SetAndGet(t *testing.T) {
	store := newMemoryStore(t)
	defer store.Close()
	ctx := context.Background()

	if err := store.Set(ctx, []byte("key1"), []byte("value1")); err != nil {
		t.Fatalf("Set: %v", err)
	}

	val, err := store.Get(ctx, []byte("key1"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(val) != "value1" {
		t.Fatalf("got %q, want %q", val, "value1")
	}
}

func TestMemory_GetNotFound(t *testing.T) {
	store := newMemoryStore(t)
	defer store.Close()
	ctx := context.Background()

	_, err := store.Get(ctx, []byte("missing"))
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestMemory_SetOverwrite(t *testing.T) {
	store := newMemoryStore(t)
	defer store.Close()
	ctx := context.Background()

	store.Set(ctx, []byte("k"), []byte("v1"))
	store.Set(ctx, []byte("k"), []byte("v2"))

	val, _ := store.Get(ctx, []byte("k"))
	if string(val) != "v2" {
		t.Fatalf("overwrite failed: got %q", val)
	}
}

func TestMemory_Delete(t *testing.T) {
	store := newMemoryStore(t)
	defer store.Close()
	ctx := context.Background()

	store.Set(ctx, []byte("k"), []byte("v"))
	if err := store.Delete(ctx, []byte("k")); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := store.Get(ctx, []byte("k"))
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestMemory_DeleteNonExistent(t *testing.T) {
	store := newMemoryStore(t)
	defer store.Close()
	ctx := context.Background()

	// Deleting a non-existent key should not error.
	if err := store.Delete(ctx, []byte("nope")); err != nil {
		t.Fatalf("Delete non-existent key: %v", err)
	}
}

func TestMemory_Scan(t *testing.T) {
	store := newMemoryStore(t)
	defer store.Close()
	ctx := context.Background()

	store.Set(ctx, []byte("user:1"), []byte("alice"))
	store.Set(ctx, []byte("user:2"), []byte("bob"))
	store.Set(ctx, []byte("order:1"), []byte("pizza"))

	var keys []string
	err := store.Scan(ctx, []byte("user:"), func(key, value []byte) error {
		keys = append(keys, string(key))
		return nil
	})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys with prefix 'user:', got %d: %v", len(keys), keys)
	}
}

func TestMemory_ScanAllKeys(t *testing.T) {
	store := newMemoryStore(t)
	defer store.Close()
	ctx := context.Background()

	store.Set(ctx, []byte("a"), []byte("1"))
	store.Set(ctx, []byte("b"), []byte("2"))

	var count int
	store.Scan(ctx, nil, func(key, value []byte) error {
		count++
		return nil
	})
	if count != 2 {
		t.Fatalf("expected 2 keys, got %d", count)
	}
}

func TestMemory_ScanCallbackError(t *testing.T) {
	store := newMemoryStore(t)
	defer store.Close()
	ctx := context.Background()

	store.Set(ctx, []byte("a"), []byte("1"))

	sentinel := fmt.Errorf("stop")
	err := store.Scan(ctx, nil, func(key, value []byte) error {
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
}

func TestMemory_ScanContextCancelled(t *testing.T) {
	store := newMemoryStore(t)
	defer store.Close()

	ctx, cancel := context.WithCancel(context.Background())

	store.Set(ctx, []byte("a"), []byte("1"))
	store.Set(ctx, []byte("b"), []byte("2"))

	cancel()

	err := store.Scan(ctx, nil, func(key, value []byte) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected context cancelled error")
	}
}

// --- Closed store tests ---

func TestMemory_ClosedGet(t *testing.T) {
	store := newMemoryStore(t)
	store.Close()

	_, err := store.Get(context.Background(), []byte("k"))
	if !errors.Is(err, storage.ErrClosed) {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestMemory_ClosedSet(t *testing.T) {
	store := newMemoryStore(t)
	store.Close()

	err := store.Set(context.Background(), []byte("k"), []byte("v"))
	if !errors.Is(err, storage.ErrClosed) {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestMemory_ClosedDelete(t *testing.T) {
	store := newMemoryStore(t)
	store.Close()

	err := store.Delete(context.Background(), []byte("k"))
	if !errors.Is(err, storage.ErrClosed) {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestMemory_ClosedScan(t *testing.T) {
	store := newMemoryStore(t)
	store.Close()

	err := store.Scan(context.Background(), nil, func(key, value []byte) error { return nil })
	if !errors.Is(err, storage.ErrClosed) {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestMemory_DoubleClose(t *testing.T) {
	store := newMemoryStore(t)
	if err := store.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("second close should be nil, got %v", err)
	}
}

// --- ReadOnly tests ---

func TestMemory_ReadOnly(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ReadOnly = true
	store := NewMemoryStore(cfg)
	defer store.Close()
	ctx := context.Background()

	err := store.Set(ctx, []byte("k"), []byte("v"))
	if !errors.Is(err, storage.ErrReadOnly) {
		t.Fatalf("Set on read-only: expected ErrReadOnly, got %v", err)
	}

	err = store.Delete(ctx, []byte("k"))
	if !errors.Is(err, storage.ErrReadOnly) {
		t.Fatalf("Delete on read-only: expected ErrReadOnly, got %v", err)
	}
}

// --- Validation tests ---

func TestValidate_EmptyKey(t *testing.T) {
	cfg := DefaultConfig()
	err := Validate(nil, []byte("v"), cfg)
	if err == nil {
		t.Fatal("expected error for empty key")
	}

	err = Validate([]byte{}, []byte("v"), cfg)
	if err == nil {
		t.Fatal("expected error for zero-length key")
	}
}

func TestValidate_KeyTooLarge(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxKeySize = 10
	bigKey := make([]byte, 11)
	err := Validate(bigKey, []byte("v"), cfg)
	if !errors.Is(err, storage.ErrKeyTooLarge) {
		t.Fatalf("expected ErrKeyTooLarge, got %v", err)
	}
}

func TestValidate_ValueTooLarge(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxValueSize = 10
	bigVal := make([]byte, 11)
	err := Validate([]byte("k"), bigVal, cfg)
	if !errors.Is(err, storage.ErrValueTooLarge) {
		t.Fatalf("expected ErrValueTooLarge, got %v", err)
	}
}

func TestValidate_OK(t *testing.T) {
	cfg := DefaultConfig()
	err := Validate([]byte("k"), []byte("v"), cfg)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestMemory_SetEmptyKey(t *testing.T) {
	store := newMemoryStore(t)
	defer store.Close()

	err := store.Set(context.Background(), nil, []byte("v"))
	if err == nil {
		t.Fatal("expected error for nil key")
	}
}

// --- CopyBytes tests ---

func TestCopyBytes(t *testing.T) {
	orig := []byte("hello")
	cp := CopyBytes(orig)
	if string(cp) != "hello" {
		t.Fatalf("copy mismatch: %q", cp)
	}
	// Mutating copy should not affect original.
	cp[0] = 'X'
	if orig[0] == 'X' {
		t.Fatal("CopyBytes did not make an independent copy")
	}
}

func TestCopyBytes_Nil(t *testing.T) {
	if CopyBytes(nil) != nil {
		t.Fatal("CopyBytes(nil) should return nil")
	}
}

// --- Get returns independent copy ---

func TestMemory_GetReturnsCopy(t *testing.T) {
	store := newMemoryStore(t)
	defer store.Close()
	ctx := context.Background()

	store.Set(ctx, []byte("k"), []byte("original"))
	val, _ := store.Get(ctx, []byte("k"))
	val[0] = 'X' // mutate returned slice

	val2, _ := store.Get(ctx, []byte("k"))
	if string(val2) != "original" {
		t.Fatal("Get did not return an independent copy")
	}
}

// --- Stats tests ---

func TestMemory_Stats(t *testing.T) {
	store := newMemoryStore(t)
	defer store.Close()
	ctx := context.Background()

	store.Set(ctx, []byte("a"), []byte("123"))
	store.Set(ctx, []byte("b"), []byte("4567"))

	stats, err := store.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.Keys != 2 {
		t.Fatalf("expected 2 keys, got %d", stats.Keys)
	}
	if stats.Size != 7 {
		t.Fatalf("expected size 7, got %d", stats.Size)
	}
}

// --- Batch write tests ---

func TestMemory_Write(t *testing.T) {
	store := newMemoryStore(t)
	defer store.Close()
	ctx := context.Background()

	ops := []storage.BatchOp{
		{Type: storage.OpSet, Key: []byte("a"), Value: []byte("1")},
		{Type: storage.OpSet, Key: []byte("b"), Value: []byte("2")},
	}
	if err := store.Write(ctx, ops); err != nil {
		t.Fatalf("Write: %v", err)
	}

	v, _ := store.Get(ctx, []byte("a"))
	if string(v) != "1" {
		t.Fatalf("batch set failed: got %q", v)
	}

	// Batch delete.
	ops2 := []storage.BatchOp{
		{Type: storage.OpDelete, Key: []byte("a")},
	}
	if err := store.Write(ctx, ops2); err != nil {
		t.Fatalf("Write delete: %v", err)
	}
	_, err := store.Get(ctx, []byte("a"))
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after batch delete, got %v", err)
	}
}

func TestMemory_WriteClosed(t *testing.T) {
	store := newMemoryStore(t)
	store.Close()

	err := store.Write(context.Background(), nil)
	if !errors.Is(err, storage.ErrClosed) {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestMemory_WriteReadOnly(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ReadOnly = true
	store := NewMemoryStore(cfg)
	defer store.Close()

	err := store.Write(context.Background(), []storage.BatchOp{
		{Type: storage.OpSet, Key: []byte("k"), Value: []byte("v")},
	})
	if !errors.Is(err, storage.ErrReadOnly) {
		t.Fatalf("expected ErrReadOnly, got %v", err)
	}
}

// --- Transaction tests ---

func TestMemory_TransactionCommit(t *testing.T) {
	store := newMemoryStore(t)
	defer store.Close()
	ctx := context.Background()

	txn, err := store.Begin(ctx, true)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	if err := txn.Set([]byte("txkey"), []byte("txval")); err != nil {
		t.Fatalf("txn.Set: %v", err)
	}

	// Before commit, store should not have the key.
	_, err = store.Get(ctx, []byte("txkey"))
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatal("key visible before commit")
	}

	if err := txn.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// After commit, key should be visible.
	val, err := store.Get(ctx, []byte("txkey"))
	if err != nil {
		t.Fatalf("Get after commit: %v", err)
	}
	if string(val) != "txval" {
		t.Fatalf("got %q, want %q", val, "txval")
	}
}

func TestMemory_TransactionRollback(t *testing.T) {
	store := newMemoryStore(t)
	defer store.Close()
	ctx := context.Background()

	txn, err := store.Begin(ctx, true)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	txn.Set([]byte("txkey"), []byte("txval"))

	if err := txn.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	_, err = store.Get(ctx, []byte("txkey"))
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatal("key visible after rollback")
	}
}

func TestMemory_TransactionDelete(t *testing.T) {
	store := newMemoryStore(t)
	defer store.Close()
	ctx := context.Background()

	store.Set(ctx, []byte("k"), []byte("v"))

	txn, _ := store.Begin(ctx, true)
	txn.Delete([]byte("k"))
	txn.Commit()

	_, err := store.Get(ctx, []byte("k"))
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after txn delete, got %v", err)
	}
}

func TestMemory_TransactionReadOnly(t *testing.T) {
	store := newMemoryStore(t)
	defer store.Close()
	ctx := context.Background()

	store.Set(ctx, []byte("k"), []byte("v"))

	txn, err := store.Begin(ctx, false)
	if err != nil {
		t.Fatalf("Begin(readonly): %v", err)
	}

	val, err := txn.Get([]byte("k"))
	if err != nil {
		t.Fatalf("txn.Get: %v", err)
	}
	if string(val) != "v" {
		t.Fatalf("got %q, want %q", val, "v")
	}

	err = txn.Set([]byte("k2"), []byte("v2"))
	if !errors.Is(err, storage.ErrReadOnly) {
		t.Fatalf("Set in read-only txn: expected ErrReadOnly, got %v", err)
	}

	err = txn.Delete([]byte("k"))
	if !errors.Is(err, storage.ErrReadOnly) {
		t.Fatalf("Delete in read-only txn: expected ErrReadOnly, got %v", err)
	}

	txn.Commit()
}

func TestMemory_TransactionClosed(t *testing.T) {
	store := newMemoryStore(t)
	defer store.Close()
	ctx := context.Background()

	txn, _ := store.Begin(ctx, true)
	txn.Commit()

	_, err := txn.Get([]byte("k"))
	if !errors.Is(err, storage.ErrTxnClosed) {
		t.Fatalf("expected ErrTxnClosed on Get, got %v", err)
	}
	if err := txn.Set([]byte("k"), []byte("v")); !errors.Is(err, storage.ErrTxnClosed) {
		t.Fatalf("expected ErrTxnClosed on Set, got %v", err)
	}
	if err := txn.Delete([]byte("k")); !errors.Is(err, storage.ErrTxnClosed) {
		t.Fatalf("expected ErrTxnClosed on Delete, got %v", err)
	}
	if err := txn.Commit(); !errors.Is(err, storage.ErrTxnClosed) {
		t.Fatalf("expected ErrTxnClosed on double Commit, got %v", err)
	}
	if err := txn.Rollback(); !errors.Is(err, storage.ErrTxnClosed) {
		t.Fatalf("expected ErrTxnClosed on Rollback after Commit, got %v", err)
	}
}

func TestMemory_BeginOnClosedStore(t *testing.T) {
	store := newMemoryStore(t)
	store.Close()

	_, err := store.Begin(context.Background(), true)
	if !errors.Is(err, storage.ErrClosed) {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestMemory_BeginReadOnlyStore(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ReadOnly = true
	store := NewMemoryStore(cfg)
	defer store.Close()

	_, err := store.Begin(context.Background(), true)
	if !errors.Is(err, storage.ErrReadOnly) {
		t.Fatalf("expected ErrReadOnly, got %v", err)
	}
}

// --- Compact / GC (no-ops) ---

func TestMemory_CompactAndGC(t *testing.T) {
	store := newMemoryStore(t)
	defer store.Close()
	ctx := context.Background()

	if err := store.Compact(ctx); err != nil {
		t.Fatalf("Compact: %v", err)
	}
	if err := store.GC(ctx); err != nil {
		t.Fatalf("GC: %v", err)
	}
}

// --- Prefix tests ---

func TestPrefix_CRUD(t *testing.T) {
	store := newMemoryStore(t)
	defer store.Close()
	ctx := context.Background()

	p := NewPrefix(store, []byte("ns:"))

	if err := p.Set(ctx, []byte("key1"), []byte("val1")); err != nil {
		t.Fatalf("Prefix.Set: %v", err)
	}

	val, err := p.Get(ctx, []byte("key1"))
	if err != nil {
		t.Fatalf("Prefix.Get: %v", err)
	}
	if string(val) != "val1" {
		t.Fatalf("got %q, want %q", val, "val1")
	}

	// Verify the actual key in the underlying store has the prefix.
	raw, err := store.Get(ctx, []byte("ns:key1"))
	if err != nil {
		t.Fatalf("underlying store missing prefixed key: %v", err)
	}
	if string(raw) != "val1" {
		t.Fatalf("underlying store: got %q", raw)
	}

	// Delete through prefix.
	if err := p.Delete(ctx, []byte("key1")); err != nil {
		t.Fatalf("Prefix.Delete: %v", err)
	}
	_, err = p.Get(ctx, []byte("key1"))
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after Prefix.Delete, got %v", err)
	}
}

func TestPrefix_Scan(t *testing.T) {
	store := newMemoryStore(t)
	defer store.Close()
	ctx := context.Background()

	p := NewPrefix(store, []byte("ns:"))
	p.Set(ctx, []byte("a"), []byte("1"))
	p.Set(ctx, []byte("b"), []byte("2"))

	// Also set a key outside the prefix namespace.
	store.Set(ctx, []byte("other"), []byte("3"))

	var keys []string
	err := p.Scan(ctx, nil, func(key, value []byte) error {
		keys = append(keys, string(key))
		return nil
	})
	if err != nil {
		t.Fatalf("Prefix.Scan: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d: %v", len(keys), keys)
	}
}

func TestPrefix_Close(t *testing.T) {
	store := newMemoryStore(t)
	defer store.Close()

	p := NewPrefix(store, []byte("ns:"))
	if err := p.Close(); err != nil {
		t.Fatalf("Prefix.Close should be no-op, got %v", err)
	}
}

// --- DefaultConfig tests ---

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Backend != BackendMemory {
		t.Fatalf("default backend: got %q, want %q", cfg.Backend, BackendMemory)
	}
	if cfg.MaxKeySize != 1024 {
		t.Fatalf("default MaxKeySize: got %d, want 1024", cfg.MaxKeySize)
	}
	if !cfg.SyncWrites {
		t.Fatal("default SyncWrites should be true")
	}
}

// --- Concurrent access tests ---

func TestMemory_ConcurrentSetGet(t *testing.T) {
	store := newMemoryStore(t)
	defer store.Close()
	ctx := context.Background()

	const goroutines = 50
	const ops = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			key := []byte(fmt.Sprintf("key-%d", id))
			val := []byte(fmt.Sprintf("val-%d", id))
			for j := 0; j < ops; j++ {
				store.Set(ctx, key, val)
				store.Get(ctx, key)
			}
		}(i)
	}
	wg.Wait()
}

func TestMemory_ConcurrentScan(t *testing.T) {
	store := newMemoryStore(t)
	defer store.Close()
	ctx := context.Background()

	for i := 0; i < 20; i++ {
		store.Set(ctx, []byte(fmt.Sprintf("k%02d", i)), []byte("v"))
	}

	var wg sync.WaitGroup
	wg.Add(10)
	for i := 0; i < 10; i++ {
		go func() {
			defer wg.Done()
			store.Scan(ctx, nil, func(key, value []byte) error { return nil })
		}()
	}
	wg.Wait()
}

// --- Bolt backend tests ---

func TestBolt_CRUD(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Backend = BackendBolt
	cfg.Path = filepath.Join(t.TempDir(), "test.db")

	store, err := NewBoltStore(cfg)
	if err != nil {
		t.Fatalf("NewBoltStore: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	store.Set(ctx, []byte("k"), []byte("v"))
	val, err := store.Get(ctx, []byte("k"))
	if err != nil {
		t.Fatalf("BoltStore.Get: %v", err)
	}
	if string(val) != "v" {
		t.Fatalf("got %q", val)
	}

	store.Delete(ctx, []byte("k"))
	_, err = store.Get(ctx, []byte("k"))
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestBolt_ClosedOperations(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Backend = BackendBolt
	cfg.Path = filepath.Join(t.TempDir(), "test.db")

	store, _ := NewBoltStore(cfg)
	store.Close()
	ctx := context.Background()

	_, err := store.Get(ctx, []byte("k"))
	if !errors.Is(err, storage.ErrClosed) {
		t.Fatalf("expected ErrClosed on Get, got %v", err)
	}
	if err := store.Set(ctx, []byte("k"), []byte("v")); !errors.Is(err, storage.ErrClosed) {
		t.Fatalf("expected ErrClosed on Set, got %v", err)
	}
	if err := store.Delete(ctx, []byte("k")); !errors.Is(err, storage.ErrClosed) {
		t.Fatalf("expected ErrClosed on Delete, got %v", err)
	}
	if err := store.Scan(ctx, nil, nil); !errors.Is(err, storage.ErrClosed) {
		t.Fatalf("expected ErrClosed on Scan, got %v", err)
	}
}

func TestBolt_Stats(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Backend = BackendBolt
	cfg.Path = filepath.Join(t.TempDir(), "test.db")

	store, _ := NewBoltStore(cfg)
	defer store.Close()
	ctx := context.Background()

	store.Set(ctx, []byte("a"), []byte("1"))
	stats, err := store.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.Keys != 1 {
		t.Fatalf("expected 1 key, got %d", stats.Keys)
	}
}

func TestBolt_Transaction(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Backend = BackendBolt
	cfg.Path = filepath.Join(t.TempDir(), "test.db")

	store, _ := NewBoltStore(cfg)
	defer store.Close()
	ctx := context.Background()

	txn, err := store.Begin(ctx, true)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	txn.Set([]byte("k"), []byte("v"))
	txn.Commit()

	val, _ := store.Get(ctx, []byte("k"))
	if string(val) != "v" {
		t.Fatalf("got %q after txn commit", val)
	}
}

func TestBolt_Batch(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Backend = BackendBolt
	cfg.Path = filepath.Join(t.TempDir(), "test.db")

	store, _ := NewBoltStore(cfg)
	defer store.Close()
	ctx := context.Background()

	store.Write(ctx, []storage.BatchOp{
		{Type: storage.OpSet, Key: []byte("x"), Value: []byte("y")},
	})

	val, _ := store.Get(ctx, []byte("x"))
	if string(val) != "y" {
		t.Fatalf("batch write: got %q", val)
	}
}

func TestBolt_CompactAndGC(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Backend = BackendBolt
	cfg.Path = filepath.Join(t.TempDir(), "test.db")

	store, _ := NewBoltStore(cfg)
	defer store.Close()
	ctx := context.Background()

	if err := store.Compact(ctx); err != nil {
		t.Fatalf("Compact: %v", err)
	}
	if err := store.GC(ctx); err != nil {
		t.Fatalf("GC: %v", err)
	}
}

func TestBolt_FirstAndLast(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Backend = BackendBolt
	cfg.Path = filepath.Join(t.TempDir(), "test.db")

	store, _ := NewBoltStore(cfg)
	defer store.Close()
	ctx := context.Background()

	store.Set(ctx, []byte("a"), []byte("1"))
	store.Set(ctx, []byte("b"), []byte("2"))
	store.Set(ctx, []byte("c"), []byte("3"))

	fk, fv, err := store.First(ctx)
	if err != nil {
		t.Fatalf("First: %v", err)
	}
	if string(fk) != "a" || string(fv) != "1" {
		t.Fatalf("First: got key=%q val=%q", fk, fv)
	}

	lk, lv, err := store.Last(ctx)
	if err != nil {
		t.Fatalf("Last: %v", err)
	}
	if string(lk) != "c" || string(lv) != "3" {
		t.Fatalf("Last: got key=%q val=%q", lk, lv)
	}
}

func TestBolt_FirstAndLast_Empty(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Backend = BackendBolt
	cfg.Path = filepath.Join(t.TempDir(), "test.db")

	store, _ := NewBoltStore(cfg)
	defer store.Close()
	ctx := context.Background()

	_, _, err := store.First(ctx)
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("First on empty: expected ErrNotFound, got %v", err)
	}

	_, _, err = store.Last(ctx)
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("Last on empty: expected ErrNotFound, got %v", err)
	}
}

func TestBolt_ScanRange(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Backend = BackendBolt
	cfg.Path = filepath.Join(t.TempDir(), "test.db")

	store, _ := NewBoltStore(cfg)
	defer store.Close()
	ctx := context.Background()

	store.Set(ctx, []byte("a"), []byte("1"))
	store.Set(ctx, []byte("b"), []byte("2"))
	store.Set(ctx, []byte("c"), []byte("3"))
	store.Set(ctx, []byte("d"), []byte("4"))

	var keys []string
	store.ScanRange(ctx, []byte("b"), []byte("d"), func(key, value []byte) error {
		keys = append(keys, string(key))
		return nil
	})
	if len(keys) != 2 || keys[0] != "b" || keys[1] != "c" {
		t.Fatalf("ScanRange [b,d): got %v", keys)
	}
}

// --- Badger backend tests ---

func TestBadger_CRUD(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Backend = BackendBadger
	cfg.Path = t.TempDir()

	store, err := NewBadgerStore(cfg)
	if err != nil {
		t.Fatalf("NewBadgerStore: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	store.Set(ctx, []byte("k"), []byte("v"))
	val, err := store.Get(ctx, []byte("k"))
	if err != nil {
		t.Fatalf("BadgerStore.Get: %v", err)
	}
	if string(val) != "v" {
		t.Fatalf("got %q", val)
	}

	store.Delete(ctx, []byte("k"))
	_, err = store.Get(ctx, []byte("k"))
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestBadger_ClosedOperations(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Backend = BackendBadger
	cfg.Path = t.TempDir()

	store, _ := NewBadgerStore(cfg)
	store.Close()
	ctx := context.Background()

	_, err := store.Get(ctx, []byte("k"))
	if !errors.Is(err, storage.ErrClosed) {
		t.Fatalf("expected ErrClosed on Get, got %v", err)
	}
	if err := store.Set(ctx, []byte("k"), []byte("v")); !errors.Is(err, storage.ErrClosed) {
		t.Fatalf("expected ErrClosed on Set, got %v", err)
	}
	if err := store.Delete(ctx, []byte("k")); !errors.Is(err, storage.ErrClosed) {
		t.Fatalf("expected ErrClosed on Delete, got %v", err)
	}
	if err := store.Scan(ctx, nil, nil); !errors.Is(err, storage.ErrClosed) {
		t.Fatalf("expected ErrClosed on Scan, got %v", err)
	}
}

func TestBadger_KeyExists(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Backend = BackendBadger
	cfg.Path = t.TempDir()

	store, _ := NewBadgerStore(cfg)
	defer store.Close()
	ctx := context.Background()

	exists, err := store.KeyExists(ctx, []byte("nope"))
	if err != nil {
		t.Fatalf("KeyExists: %v", err)
	}
	if exists {
		t.Fatal("expected false for missing key")
	}

	store.Set(ctx, []byte("k"), []byte("v"))
	exists, err = store.KeyExists(ctx, []byte("k"))
	if err != nil {
		t.Fatalf("KeyExists: %v", err)
	}
	if !exists {
		t.Fatal("expected true for existing key")
	}
}

func TestBadger_Keys(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Backend = BackendBadger
	cfg.Path = t.TempDir()

	store, _ := NewBadgerStore(cfg)
	defer store.Close()
	ctx := context.Background()

	store.Set(ctx, []byte("user:1"), []byte("a"))
	store.Set(ctx, []byte("user:2"), []byte("b"))
	store.Set(ctx, []byte("order:1"), []byte("c"))

	keys, err := store.Keys(ctx, []byte("user:"))
	if err != nil {
		t.Fatalf("Keys: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
}

func TestBadger_ScanReverse(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Backend = BackendBadger
	cfg.Path = t.TempDir()

	store, _ := NewBadgerStore(cfg)
	defer store.Close()
	ctx := context.Background()

	store.Set(ctx, []byte("a"), []byte("1"))
	store.Set(ctx, []byte("b"), []byte("2"))
	store.Set(ctx, []byte("c"), []byte("3"))

	var keys []string
	store.ScanReverse(ctx, nil, func(key, value []byte) error {
		keys = append(keys, string(key))
		return nil
	})
	if len(keys) != 3 || keys[0] != "c" || keys[2] != "a" {
		t.Fatalf("ScanReverse: got %v", keys)
	}
}

func TestBadger_CompactAndGC(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Backend = BackendBadger
	cfg.Path = t.TempDir()

	store, _ := NewBadgerStore(cfg)
	defer store.Close()
	ctx := context.Background()

	if err := store.Compact(ctx); err != nil {
		t.Fatalf("Compact: %v", err)
	}
	if err := store.GC(ctx); err != nil {
		t.Fatalf("GC: %v", err)
	}
}
