// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package kv

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"testing"
	"time"

	"unheaded/pkg/logger"
	"unheaded/pkg/storage"
)

// testLogger returns a silent logger for tests.
func testLogger() *logger.Logger {
	return logger.New(io.Discard)
}

// --- Validate edge cases ---

func TestValidate_ZeroMaxKeySize(t *testing.T) {
	cfg := Config{MaxKeySize: 0, MaxValueSize: 0}
	// MaxKeySize=0 means no limit, so a large key should pass.
	bigKey := make([]byte, 10000)
	bigKey[0] = 'k'
	if err := Validate(bigKey, []byte("v"), cfg); err != nil {
		t.Fatalf("expected no error with zero max key size, got %v", err)
	}
}

func TestValidate_ZeroMaxValueSize(t *testing.T) {
	cfg := Config{MaxKeySize: 0, MaxValueSize: 0}
	bigVal := make([]byte, 10000)
	if err := Validate([]byte("k"), bigVal, cfg); err != nil {
		t.Fatalf("expected no error with zero max value size, got %v", err)
	}
}

func TestValidate_ExactBoundary(t *testing.T) {
	cfg := Config{MaxKeySize: 5, MaxValueSize: 10}
	// Exactly at limit should pass.
	if err := Validate([]byte("12345"), []byte("1234567890"), cfg); err != nil {
		t.Fatalf("expected no error at exact boundary, got %v", err)
	}
	// One over should fail.
	if err := Validate([]byte("123456"), []byte("v"), cfg); !errors.Is(err, storage.ErrKeyTooLarge) {
		t.Fatalf("expected ErrKeyTooLarge, got %v", err)
	}
	if err := Validate([]byte("k"), []byte("12345678901"), cfg); !errors.Is(err, storage.ErrValueTooLarge) {
		t.Fatalf("expected ErrValueTooLarge, got %v", err)
	}
}

// --- Memory store with custom logger ---

func TestMemory_CustomLogger(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Logger = testLogger()
	store := NewMemoryStore(cfg)
	defer store.Close()

	ctx := context.Background()
	if err := store.Set(ctx, []byte("k"), []byte("v")); err != nil {
		t.Fatalf("Set with custom logger: %v", err)
	}
	val, err := store.Get(ctx, []byte("k"))
	if err != nil {
		t.Fatalf("Get with custom logger: %v", err)
	}
	if string(val) != "v" {
		t.Fatalf("got %q, want %q", val, "v")
	}
}

// --- Memory transaction: deeper coverage ---

func TestMemory_TxnGetDeletedKey(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Logger = testLogger()
	store := NewMemoryStore(cfg)
	defer store.Close()
	ctx := context.Background()

	// Pre-populate store.
	store.Set(ctx, []byte("k1"), []byte("v1"))

	txn, err := store.Begin(ctx, true)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	// Delete within txn, then try to get.
	if err := txn.Delete([]byte("k1")); err != nil {
		t.Fatalf("txn.Delete: %v", err)
	}
	_, err = txn.Get([]byte("k1"))
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for deleted key in txn, got %v", err)
	}

	txn.Rollback()
}

func TestMemory_TxnGetWrittenKey(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Logger = testLogger()
	store := NewMemoryStore(cfg)
	defer store.Close()
	ctx := context.Background()

	txn, err := store.Begin(ctx, true)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	// Write a key, then read it within the same txn.
	txn.Set([]byte("newkey"), []byte("newval"))
	val, err := txn.Get([]byte("newkey"))
	if err != nil {
		t.Fatalf("txn.Get after txn.Set: %v", err)
	}
	if string(val) != "newval" {
		t.Fatalf("got %q, want %q", val, "newval")
	}

	txn.Rollback()
}

func TestMemory_TxnGetFromSnapshot(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Logger = testLogger()
	store := NewMemoryStore(cfg)
	defer store.Close()
	ctx := context.Background()

	store.Set(ctx, []byte("snap"), []byte("original"))

	txn, err := store.Begin(ctx, false)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	val, err := txn.Get([]byte("snap"))
	if err != nil {
		t.Fatalf("txn.Get from snapshot: %v", err)
	}
	if string(val) != "original" {
		t.Fatalf("got %q, want %q", val, "original")
	}

	txn.Commit()
}

func TestMemory_TxnGetNotFoundAnywhere(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Logger = testLogger()
	store := NewMemoryStore(cfg)
	defer store.Close()
	ctx := context.Background()

	txn, _ := store.Begin(ctx, false)
	_, err := txn.Get([]byte("nonexistent"))
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	txn.Commit()
}

func TestMemory_TxnSetDeleteInteraction(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Logger = testLogger()
	store := NewMemoryStore(cfg)
	defer store.Close()
	ctx := context.Background()

	txn, _ := store.Begin(ctx, true)

	// Set then delete, should clear from writes and add to deletes.
	txn.Set([]byte("k"), []byte("v"))
	txn.Delete([]byte("k"))
	_, err := txn.Get([]byte("k"))
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after set+delete in txn, got %v", err)
	}

	// Now set again after delete, should clear from deletes.
	txn.Set([]byte("k"), []byte("v2"))
	val, err := txn.Get([]byte("k"))
	if err != nil {
		t.Fatalf("txn.Get after re-set: %v", err)
	}
	if string(val) != "v2" {
		t.Fatalf("got %q, want %q", val, "v2")
	}

	txn.Commit()

	// Verify in store.
	val, err = store.Get(ctx, []byte("k"))
	if err != nil {
		t.Fatalf("store.Get after txn commit: %v", err)
	}
	if string(val) != "v2" {
		t.Fatalf("got %q, want %q", val, "v2")
	}
}

func TestMemory_TxnSetValidation(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Logger = testLogger()
	cfg.MaxKeySize = 5
	store := NewMemoryStore(cfg)
	defer store.Close()
	ctx := context.Background()

	txn, _ := store.Begin(ctx, true)
	err := txn.Set([]byte("toolong"), []byte("v"))
	if !errors.Is(err, storage.ErrKeyTooLarge) {
		t.Fatalf("expected ErrKeyTooLarge in txn, got %v", err)
	}
	txn.Rollback()
}

func TestMemory_TxnCommitOnClosedStore(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Logger = testLogger()
	store := NewMemoryStore(cfg)
	ctx := context.Background()

	txn, _ := store.Begin(ctx, true)
	txn.Set([]byte("k"), []byte("v"))

	// Close the store before committing.
	store.Close()

	err := txn.Commit()
	if !errors.Is(err, storage.ErrClosed) {
		t.Fatalf("expected ErrClosed on commit to closed store, got %v", err)
	}
}

func TestMemory_TxnCommitReadOnly(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Logger = testLogger()
	store := NewMemoryStore(cfg)
	defer store.Close()
	ctx := context.Background()

	// Read-only txn commit should be a no-op success.
	txn, _ := store.Begin(ctx, false)
	if err := txn.Commit(); err != nil {
		t.Fatalf("read-only txn commit should succeed, got %v", err)
	}
}

func TestMemory_TxnCommitDeleteExistingKey(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Logger = testLogger()
	store := NewMemoryStore(cfg)
	defer store.Close()
	ctx := context.Background()

	store.Set(ctx, []byte("exist"), []byte("val"))

	txn, _ := store.Begin(ctx, true)
	txn.Delete([]byte("exist"))
	txn.Commit()

	_, err := store.Get(ctx, []byte("exist"))
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	stats, _ := store.Stats(ctx)
	if stats.Keys != 0 {
		t.Fatalf("expected 0 keys, got %d", stats.Keys)
	}
}

func TestMemory_TxnCommitDeleteNonExistentKey(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Logger = testLogger()
	store := NewMemoryStore(cfg)
	defer store.Close()
	ctx := context.Background()

	txn, _ := store.Begin(ctx, true)
	txn.Delete([]byte("ghost"))
	// Should not error.
	if err := txn.Commit(); err != nil {
		t.Fatalf("commit with delete of non-existent key: %v", err)
	}
}

func TestMemory_TxnCommitOverwrite(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Logger = testLogger()
	store := NewMemoryStore(cfg)
	defer store.Close()
	ctx := context.Background()

	store.Set(ctx, []byte("k"), []byte("old"))

	txn, _ := store.Begin(ctx, true)
	txn.Set([]byte("k"), []byte("new"))
	txn.Commit()

	val, _ := store.Get(ctx, []byte("k"))
	if string(val) != "new" {
		t.Fatalf("got %q, want %q", val, "new")
	}
}

func TestMemory_RollbackAlreadyClosed(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Logger = testLogger()
	store := NewMemoryStore(cfg)
	defer store.Close()
	ctx := context.Background()

	txn, _ := store.Begin(ctx, true)
	txn.Rollback()

	err := txn.Rollback()
	if !errors.Is(err, storage.ErrTxnClosed) {
		t.Fatalf("expected ErrTxnClosed on double rollback, got %v", err)
	}
}

// --- Memory batch write edge cases ---

func TestMemory_WriteBatchOverwrite(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Logger = testLogger()
	store := NewMemoryStore(cfg)
	defer store.Close()
	ctx := context.Background()

	store.Set(ctx, []byte("k"), []byte("old"))

	ops := []storage.BatchOp{
		{Type: storage.OpSet, Key: []byte("k"), Value: []byte("new")},
	}
	store.Write(ctx, ops)

	val, _ := store.Get(ctx, []byte("k"))
	if string(val) != "new" {
		t.Fatalf("got %q, want %q", val, "new")
	}
}

func TestMemory_WriteBatchDeleteNonExistent(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Logger = testLogger()
	store := NewMemoryStore(cfg)
	defer store.Close()
	ctx := context.Background()

	ops := []storage.BatchOp{
		{Type: storage.OpDelete, Key: []byte("nope")},
	}
	if err := store.Write(ctx, ops); err != nil {
		t.Fatalf("batch delete non-existent: %v", err)
	}
}

func TestMemory_WriteBatchValidationError(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Logger = testLogger()
	cfg.MaxKeySize = 3
	store := NewMemoryStore(cfg)
	defer store.Close()
	ctx := context.Background()

	ops := []storage.BatchOp{
		{Type: storage.OpSet, Key: []byte("toolong"), Value: []byte("v")},
	}
	err := store.Write(ctx, ops)
	if !errors.Is(err, storage.ErrKeyTooLarge) {
		t.Fatalf("expected ErrKeyTooLarge, got %v", err)
	}
}

// --- Prefix edge cases ---

func TestPrefix_ScanWithSubPrefix(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Logger = testLogger()
	store := NewMemoryStore(cfg)
	defer store.Close()
	ctx := context.Background()

	p := NewPrefix(store, []byte("ns:"))
	p.Set(ctx, []byte("alpha:1"), []byte("a"))
	p.Set(ctx, []byte("alpha:2"), []byte("b"))
	p.Set(ctx, []byte("beta:1"), []byte("c"))

	var keys []string
	p.Scan(ctx, []byte("alpha:"), func(key, value []byte) error {
		keys = append(keys, string(key))
		return nil
	})
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys with sub-prefix 'alpha:', got %d: %v", len(keys), keys)
	}
}

func TestPrefix_PrefixKey(t *testing.T) {
	store := NewMemoryStore(DefaultConfig())
	defer store.Close()

	p := NewPrefix(store, []byte("pfx:"))
	ctx := context.Background()

	// Set and verify underlying key.
	p.Set(ctx, []byte("mykey"), []byte("myval"))
	val, err := store.Get(ctx, []byte("pfx:mykey"))
	if err != nil {
		t.Fatalf("underlying store should have prefixed key: %v", err)
	}
	if string(val) != "myval" {
		t.Fatalf("got %q, want %q", val, "myval")
	}
}

// --- Badger extended tests ---

func newTestBadgerStore(t *testing.T) *BadgerStore {
	t.Helper()
	cfg := DefaultConfig()
	cfg.Backend = BackendBadger
	cfg.Path = t.TempDir()
	cfg.Logger = testLogger()
	store, err := NewBadgerStore(cfg)
	if err != nil {
		t.Fatalf("NewBadgerStore: %v", err)
	}
	return store
}

func TestBadger_Stats(t *testing.T) {
	store := newTestBadgerStore(t)
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

func TestBadger_StatsClosed(t *testing.T) {
	store := newTestBadgerStore(t)
	store.Close()

	_, err := store.Stats(context.Background())
	if !errors.Is(err, storage.ErrClosed) {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestBadger_ReadOnly(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Backend = BackendBadger
	cfg.Path = t.TempDir()
	cfg.Logger = testLogger()
	cfg.ReadOnly = true

	store, err := NewBadgerStore(cfg)
	if err != nil {
		t.Fatalf("NewBadgerStore: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	err = store.Set(ctx, []byte("k"), []byte("v"))
	if !errors.Is(err, storage.ErrReadOnly) {
		t.Fatalf("expected ErrReadOnly on Set, got %v", err)
	}
	err = store.Delete(ctx, []byte("k"))
	if !errors.Is(err, storage.ErrReadOnly) {
		t.Fatalf("expected ErrReadOnly on Delete, got %v", err)
	}
}

func TestBadger_WriteBatch(t *testing.T) {
	store := newTestBadgerStore(t)
	defer store.Close()
	ctx := context.Background()

	ops := []storage.BatchOp{
		{Type: storage.OpSet, Key: []byte("a"), Value: []byte("1")},
		{Type: storage.OpSet, Key: []byte("b"), Value: []byte("2")},
	}
	if err := store.Write(ctx, ops); err != nil {
		t.Fatalf("Write: %v", err)
	}

	val, _ := store.Get(ctx, []byte("a"))
	if string(val) != "1" {
		t.Fatalf("got %q, want %q", val, "1")
	}
}

func TestBadger_WriteClosed(t *testing.T) {
	store := newTestBadgerStore(t)
	store.Close()

	err := store.Write(context.Background(), nil)
	if !errors.Is(err, storage.ErrClosed) {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestBadger_WriteReadOnly(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Backend = BackendBadger
	cfg.Path = t.TempDir()
	cfg.Logger = testLogger()
	cfg.ReadOnly = true

	store, _ := NewBadgerStore(cfg)
	defer store.Close()

	err := store.Write(context.Background(), []storage.BatchOp{
		{Type: storage.OpSet, Key: []byte("k"), Value: []byte("v")},
	})
	if !errors.Is(err, storage.ErrReadOnly) {
		t.Fatalf("expected ErrReadOnly, got %v", err)
	}
}

func TestBadger_Transaction(t *testing.T) {
	store := newTestBadgerStore(t)
	defer store.Close()
	ctx := context.Background()

	txn, err := store.Begin(ctx, true)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	txn.Set([]byte("txk"), []byte("txv"))
	txn.Commit()

	val, _ := store.Get(ctx, []byte("txk"))
	if string(val) != "txv" {
		t.Fatalf("got %q after txn commit", val)
	}
}

func TestBadger_BeginClosed(t *testing.T) {
	store := newTestBadgerStore(t)
	store.Close()

	_, err := store.Begin(context.Background(), true)
	if !errors.Is(err, storage.ErrClosed) {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestBadger_BeginReadOnlyStore(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Backend = BackendBadger
	cfg.Path = t.TempDir()
	cfg.Logger = testLogger()
	cfg.ReadOnly = true

	store, _ := NewBadgerStore(cfg)
	defer store.Close()

	_, err := store.Begin(context.Background(), true)
	if !errors.Is(err, storage.ErrReadOnly) {
		t.Fatalf("expected ErrReadOnly, got %v", err)
	}
}

func TestBadger_CompactClosed(t *testing.T) {
	store := newTestBadgerStore(t)
	store.Close()

	err := store.Compact(context.Background())
	if !errors.Is(err, storage.ErrClosed) {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestBadger_GCClosed(t *testing.T) {
	store := newTestBadgerStore(t)
	store.Close()

	err := store.GC(context.Background())
	if !errors.Is(err, storage.ErrClosed) {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestBadger_DoubleClose(t *testing.T) {
	store := newTestBadgerStore(t)
	if err := store.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("second close should be nil, got %v", err)
	}
}

func TestBadger_ScanReverseClosed(t *testing.T) {
	store := newTestBadgerStore(t)
	store.Close()

	err := store.ScanReverse(context.Background(), nil, func(key, value []byte) error { return nil })
	if !errors.Is(err, storage.ErrClosed) {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestBadger_SetWithTTL(t *testing.T) {
	store := newTestBadgerStore(t)
	defer store.Close()
	ctx := context.Background()

	err := store.SetWithTTL(ctx, []byte("ttlkey"), []byte("ttlval"), 1*time.Hour)
	if err != nil {
		t.Fatalf("SetWithTTL: %v", err)
	}

	val, err := store.Get(ctx, []byte("ttlkey"))
	if err != nil {
		t.Fatalf("Get after SetWithTTL: %v", err)
	}
	if string(val) != "ttlval" {
		t.Fatalf("got %q, want %q", val, "ttlval")
	}
}

func TestBadger_SetWithTTLClosed(t *testing.T) {
	store := newTestBadgerStore(t)
	store.Close()

	err := store.SetWithTTL(context.Background(), []byte("k"), []byte("v"), time.Hour)
	if !errors.Is(err, storage.ErrClosed) {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestBadger_SetWithTTLReadOnly(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Backend = BackendBadger
	cfg.Path = t.TempDir()
	cfg.Logger = testLogger()
	cfg.ReadOnly = true

	store, _ := NewBadgerStore(cfg)
	defer store.Close()

	err := store.SetWithTTL(context.Background(), []byte("k"), []byte("v"), time.Hour)
	if !errors.Is(err, storage.ErrReadOnly) {
		t.Fatalf("expected ErrReadOnly, got %v", err)
	}
}

func TestBadger_SetWithTTLValidation(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Backend = BackendBadger
	cfg.Path = t.TempDir()
	cfg.Logger = testLogger()
	cfg.MaxKeySize = 3

	store, _ := NewBadgerStore(cfg)
	defer store.Close()

	err := store.SetWithTTL(context.Background(), []byte("toolong"), []byte("v"), time.Hour)
	if !errors.Is(err, storage.ErrKeyTooLarge) {
		t.Fatalf("expected ErrKeyTooLarge, got %v", err)
	}
}

func TestBadger_KeyExistsClosed(t *testing.T) {
	store := newTestBadgerStore(t)
	store.Close()

	_, err := store.KeyExists(context.Background(), []byte("k"))
	if !errors.Is(err, storage.ErrClosed) {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestBadger_KeysClosed(t *testing.T) {
	store := newTestBadgerStore(t)
	store.Close()

	_, err := store.Keys(context.Background(), nil)
	if !errors.Is(err, storage.ErrClosed) {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestBadger_ScanRangeClosed(t *testing.T) {
	store := newTestBadgerStore(t)
	store.Close()

	err := store.ScanRange(context.Background(), []byte("a"), []byte("z"), func(key, value []byte) error { return nil })
	if !errors.Is(err, storage.ErrClosed) {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestBadger_ScanRange(t *testing.T) {
	store := newTestBadgerStore(t)
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

func TestBadger_ScanRangeContextCancel(t *testing.T) {
	store := newTestBadgerStore(t)
	defer store.Close()
	ctx := context.Background()

	store.Set(ctx, []byte("a"), []byte("1"))
	store.Set(ctx, []byte("b"), []byte("2"))
	store.Set(ctx, []byte("c"), []byte("3"))

	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()

	err := store.ScanRange(cancelCtx, []byte("a"), []byte("z"), func(key, value []byte) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected context error")
	}
}

func TestBadger_ScanReverseContextCancel(t *testing.T) {
	store := newTestBadgerStore(t)
	defer store.Close()
	ctx := context.Background()

	store.Set(ctx, []byte("a"), []byte("1"))
	store.Set(ctx, []byte("b"), []byte("2"))

	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()

	err := store.ScanReverse(cancelCtx, nil, func(key, value []byte) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected context error")
	}
}

func TestBadger_ScanReverseCallbackError(t *testing.T) {
	store := newTestBadgerStore(t)
	defer store.Close()
	ctx := context.Background()

	store.Set(ctx, []byte("a"), []byte("1"))
	store.Set(ctx, []byte("b"), []byte("2"))

	sentinel := errors.New("stop-reverse")
	err := store.ScanReverse(ctx, nil, func(key, value []byte) error {
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
}

// --- Bolt extended tests ---

func newTestBoltStore(t *testing.T) *BoltStore {
	t.Helper()
	cfg := DefaultConfig()
	cfg.Backend = BackendBolt
	cfg.Path = filepath.Join(t.TempDir(), "test.db")
	cfg.Logger = testLogger()
	store, err := NewBoltStore(cfg)
	if err != nil {
		t.Fatalf("NewBoltStore: %v", err)
	}
	return store
}

func TestBolt_ReadOnly(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Backend = BackendBolt
	cfg.Path = filepath.Join(t.TempDir(), "test.db")
	cfg.Logger = testLogger()
	cfg.ReadOnly = true

	store, err := NewBoltStore(cfg)
	if err != nil {
		t.Fatalf("NewBoltStore: %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	err = store.Set(ctx, []byte("k"), []byte("v"))
	if !errors.Is(err, storage.ErrReadOnly) {
		t.Fatalf("expected ErrReadOnly on Set, got %v", err)
	}
	err = store.Delete(ctx, []byte("k"))
	if !errors.Is(err, storage.ErrReadOnly) {
		t.Fatalf("expected ErrReadOnly on Delete, got %v", err)
	}
}

func TestBolt_WriteClosed(t *testing.T) {
	store := newTestBoltStore(t)
	store.Close()

	err := store.Write(context.Background(), nil)
	if !errors.Is(err, storage.ErrClosed) {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestBolt_WriteReadOnly(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Backend = BackendBolt
	cfg.Path = filepath.Join(t.TempDir(), "test.db")
	cfg.Logger = testLogger()
	cfg.ReadOnly = true

	store, _ := NewBoltStore(cfg)
	defer store.Close()

	err := store.Write(context.Background(), []storage.BatchOp{
		{Type: storage.OpSet, Key: []byte("k"), Value: []byte("v")},
	})
	if !errors.Is(err, storage.ErrReadOnly) {
		t.Fatalf("expected ErrReadOnly, got %v", err)
	}
}

func TestBolt_StatsClosed(t *testing.T) {
	store := newTestBoltStore(t)
	store.Close()

	_, err := store.Stats(context.Background())
	if !errors.Is(err, storage.ErrClosed) {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestBolt_BeginClosed(t *testing.T) {
	store := newTestBoltStore(t)
	store.Close()

	_, err := store.Begin(context.Background(), true)
	if !errors.Is(err, storage.ErrClosed) {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestBolt_BeginReadOnlyStore(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Backend = BackendBolt
	cfg.Path = filepath.Join(t.TempDir(), "test.db")
	cfg.Logger = testLogger()
	cfg.ReadOnly = true

	store, _ := NewBoltStore(cfg)
	defer store.Close()

	_, err := store.Begin(context.Background(), true)
	if !errors.Is(err, storage.ErrReadOnly) {
		t.Fatalf("expected ErrReadOnly, got %v", err)
	}
}

func TestBolt_DoubleClose(t *testing.T) {
	store := newTestBoltStore(t)
	if err := store.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("second close should be nil, got %v", err)
	}
}

func TestBolt_CreateBucket(t *testing.T) {
	store := newTestBoltStore(t)
	defer store.Close()

	if err := store.CreateBucket("mybucket"); err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}
}

func TestBolt_CreateBucketClosed(t *testing.T) {
	store := newTestBoltStore(t)
	store.Close()

	err := store.CreateBucket("mybucket")
	if !errors.Is(err, storage.ErrClosed) {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestBolt_CreateBucketReadOnly(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Backend = BackendBolt
	cfg.Path = filepath.Join(t.TempDir(), "test.db")
	cfg.Logger = testLogger()
	cfg.ReadOnly = true

	store, _ := NewBoltStore(cfg)
	defer store.Close()

	err := store.CreateBucket("mybucket")
	if !errors.Is(err, storage.ErrReadOnly) {
		t.Fatalf("expected ErrReadOnly, got %v", err)
	}
}

func TestBolt_DeleteBucket(t *testing.T) {
	store := newTestBoltStore(t)
	defer store.Close()

	if err := store.DeleteBucket("mybucket"); err != nil {
		t.Fatalf("DeleteBucket: %v", err)
	}
}

func TestBolt_DeleteBucketClosed(t *testing.T) {
	store := newTestBoltStore(t)
	store.Close()

	err := store.DeleteBucket("mybucket")
	if !errors.Is(err, storage.ErrClosed) {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestBolt_DeleteBucketReadOnly(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Backend = BackendBolt
	cfg.Path = filepath.Join(t.TempDir(), "test.db")
	cfg.Logger = testLogger()
	cfg.ReadOnly = true

	store, _ := NewBoltStore(cfg)
	defer store.Close()

	err := store.DeleteBucket("mybucket")
	if !errors.Is(err, storage.ErrReadOnly) {
		t.Fatalf("expected ErrReadOnly, got %v", err)
	}
}

func TestBolt_UseBucket(t *testing.T) {
	store := newTestBoltStore(t)
	defer store.Close()

	sub := store.UseBucket("custom")
	if sub == nil {
		t.Fatal("UseBucket returned nil")
	}
	if string(sub.bucket) != "custom" {
		t.Fatalf("expected bucket 'custom', got %q", sub.bucket)
	}
}

func TestBolt_NextSequence(t *testing.T) {
	store := newTestBoltStore(t)
	defer store.Close()

	seq, err := store.NextSequence("mybucket")
	if err != nil {
		t.Fatalf("NextSequence: %v", err)
	}
	if seq == 0 {
		t.Fatal("expected non-zero sequence")
	}
}

func TestBolt_NextSequenceClosed(t *testing.T) {
	store := newTestBoltStore(t)
	store.Close()

	_, err := store.NextSequence("mybucket")
	if !errors.Is(err, storage.ErrClosed) {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestBolt_NextSequenceReadOnly(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Backend = BackendBolt
	cfg.Path = filepath.Join(t.TempDir(), "test.db")
	cfg.Logger = testLogger()
	cfg.ReadOnly = true

	store, _ := NewBoltStore(cfg)
	defer store.Close()

	_, err := store.NextSequence("mybucket")
	if !errors.Is(err, storage.ErrReadOnly) {
		t.Fatalf("expected ErrReadOnly, got %v", err)
	}
}

func TestBolt_ScanBucket(t *testing.T) {
	store := newTestBoltStore(t)
	defer store.Close()
	ctx := context.Background()

	store.Set(ctx, []byte("a"), []byte("1"))
	store.Set(ctx, []byte("b"), []byte("2"))

	var keys []string
	err := store.ScanBucket(ctx, "default", func(key, value []byte) error {
		keys = append(keys, string(key))
		return nil
	})
	if err != nil {
		t.Fatalf("ScanBucket: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
}

func TestBolt_ScanBucketClosed(t *testing.T) {
	store := newTestBoltStore(t)
	store.Close()

	err := store.ScanBucket(context.Background(), "default", func(key, value []byte) error { return nil })
	if !errors.Is(err, storage.ErrClosed) {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestBolt_Backup(t *testing.T) {
	store := newTestBoltStore(t)
	defer store.Close()
	ctx := context.Background()

	store.Set(ctx, []byte("bk1"), []byte("v1"))
	store.Set(ctx, []byte("bk2"), []byte("v2"))

	var keys []string
	err := store.Backup(ctx, func(key, value []byte) error {
		keys = append(keys, string(key))
		return nil
	})
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 backup entries, got %d", len(keys))
	}
}

func TestBolt_BackupClosed(t *testing.T) {
	store := newTestBoltStore(t)
	store.Close()

	err := store.Backup(context.Background(), func(key, value []byte) error { return nil })
	if !errors.Is(err, storage.ErrClosed) {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestBolt_FirstClosed(t *testing.T) {
	store := newTestBoltStore(t)
	store.Close()

	_, _, err := store.First(context.Background())
	if !errors.Is(err, storage.ErrClosed) {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestBolt_LastClosed(t *testing.T) {
	store := newTestBoltStore(t)
	store.Close()

	_, _, err := store.Last(context.Background())
	if !errors.Is(err, storage.ErrClosed) {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestBolt_ScanRangeClosed(t *testing.T) {
	store := newTestBoltStore(t)
	store.Close()

	err := store.ScanRange(context.Background(), []byte("a"), []byte("z"), func(key, value []byte) error { return nil })
	if !errors.Is(err, storage.ErrClosed) {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestBolt_ScanRangeContextCancel(t *testing.T) {
	store := newTestBoltStore(t)
	defer store.Close()
	ctx := context.Background()

	store.Set(ctx, []byte("a"), []byte("1"))
	store.Set(ctx, []byte("b"), []byte("2"))

	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()

	err := store.ScanRange(cancelCtx, []byte("a"), []byte("z"), func(key, value []byte) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected context error")
	}
}

func TestBolt_Scan(t *testing.T) {
	store := newTestBoltStore(t)
	defer store.Close()
	ctx := context.Background()

	store.Set(ctx, []byte("prefix:1"), []byte("a"))
	store.Set(ctx, []byte("prefix:2"), []byte("b"))
	store.Set(ctx, []byte("other:1"), []byte("c"))

	var keys []string
	store.Scan(ctx, []byte("prefix:"), func(key, value []byte) error {
		keys = append(keys, string(key))
		return nil
	})
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d: %v", len(keys), keys)
	}
}

// --- boltTxn tests ---

func TestBoltTxn_ClosedOps(t *testing.T) {
	txn := &boltTxn{closed: true, update: true}

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
		t.Fatalf("expected ErrTxnClosed on Commit, got %v", err)
	}
	if err := txn.Rollback(); !errors.Is(err, storage.ErrTxnClosed) {
		t.Fatalf("expected ErrTxnClosed on Rollback, got %v", err)
	}
}

func TestBoltTxn_ReadOnlyOps(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Logger = testLogger()
	memStore := NewMemoryStore(cfg)
	defer memStore.Close()

	memTxn, _ := memStore.Begin(context.Background(), false)
	txn := &boltTxn{memTxn: memTxn, update: false}

	if err := txn.Set([]byte("k"), []byte("v")); !errors.Is(err, storage.ErrReadOnly) {
		t.Fatalf("expected ErrReadOnly on Set, got %v", err)
	}
	if err := txn.Delete([]byte("k")); !errors.Is(err, storage.ErrReadOnly) {
		t.Fatalf("expected ErrReadOnly on Delete, got %v", err)
	}

	txn.Rollback()
}

// --- badgerTxn tests ---

func TestBadgerTxn_ClosedOps(t *testing.T) {
	txn := &badgerTxn{closed: true, update: true}

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
		t.Fatalf("expected ErrTxnClosed on Commit, got %v", err)
	}
	if err := txn.Rollback(); !errors.Is(err, storage.ErrTxnClosed) {
		t.Fatalf("expected ErrTxnClosed on Rollback, got %v", err)
	}
}

func TestBadgerTxn_ReadOnlyOps(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Logger = testLogger()
	memStore := NewMemoryStore(cfg)
	defer memStore.Close()

	memTxn, _ := memStore.Begin(context.Background(), false)
	txn := &badgerTxn{memTxn: memTxn, update: false}

	if err := txn.Set([]byte("k"), []byte("v")); !errors.Is(err, storage.ErrReadOnly) {
		t.Fatalf("expected ErrReadOnly on Set, got %v", err)
	}
	if err := txn.Delete([]byte("k")); !errors.Is(err, storage.ErrReadOnly) {
		t.Fatalf("expected ErrReadOnly on Delete, got %v", err)
	}

	txn.Rollback()
}

// --- DefaultBoltOptions ---

func TestDefaultBoltOptions(t *testing.T) {
	opts := DefaultBoltOptions()
	if opts.Bucket != defaultBoltBucket {
		t.Fatalf("expected bucket %q, got %q", defaultBoltBucket, opts.Bucket)
	}
	if opts.NoSync {
		t.Fatal("expected NoSync=false")
	}
	if opts.ReadOnly {
		t.Fatal("expected ReadOnly=false")
	}
	if opts.Timeout != 1*time.Second {
		t.Fatalf("expected timeout 1s, got %v", opts.Timeout)
	}
}

// --- recordOp coverage (exercises the metrics path) ---

func TestRecordOp(t *testing.T) {
	// Just ensure recordOp does not panic.
	recordOp("test", "get", "success", 0.001)
	recordOp("test", "set", "error", 0.002)
}

// --- CopyBytes with empty slice ---

func TestCopyBytes_Empty(t *testing.T) {
	result := CopyBytes([]byte{})
	if result == nil {
		t.Fatal("CopyBytes of empty slice should not be nil")
	}
	if len(result) != 0 {
		t.Fatalf("expected empty slice, got len %d", len(result))
	}
}

// --- New with custom logger for each backend ---

func TestNew_MemoryWithLogger(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Backend = BackendMemory
	cfg.Logger = testLogger()
	store, err := New(cfg)
	if err != nil {
		t.Fatalf("New(memory): %v", err)
	}
	defer store.Close()
}

func TestNew_BadgerWithLogger(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Backend = BackendBadger
	cfg.Path = t.TempDir()
	cfg.Logger = testLogger()
	store, err := New(cfg)
	if err != nil {
		t.Fatalf("New(badger): %v", err)
	}
	defer store.Close()
}

func TestNew_BoltWithLogger(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Backend = BackendBolt
	cfg.Path = filepath.Join(t.TempDir(), "test.db")
	cfg.Logger = testLogger()
	store, err := New(cfg)
	if err != nil {
		t.Fatalf("New(bolt): %v", err)
	}
	defer store.Close()
}
