// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package kv

import (
	"bytes"
	"context"
	"os"
	"sort"
	"sync"
	"time"

	"unheaded/pkg/logger"
	"unheaded/pkg/storage"
)

// MemoryStore is an in-memory key-value store.
// Thread-safe and supports transactions.
type MemoryStore struct {
	mu       sync.RWMutex
	data     map[string][]byte
	cfg      Config
	log      *logger.Logger
	closed   bool
	keyCount int64
	sizeBytes int64
}

// NewMemoryStore creates a new in-memory store.
func NewMemoryStore(cfg Config) *MemoryStore {
	log := cfg.Logger
	if log == nil {
		log = logger.New(os.Stdout)
	}

	store := &MemoryStore{
		data: make(map[string][]byte),
		cfg:  cfg,
		log:  log.With().Str("backend", "memory").Logger(),
	}

	store.log.Debug().Msg("memory store initialized")
	return store
}

// Get retrieves a value by key.
func (m *MemoryStore) Get(ctx context.Context, key []byte) ([]byte, error) {
	start := time.Now()

	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		recordOp("memory", "get", "error", time.Since(start).Seconds())
		return nil, storage.ErrClosed
	}

	value, ok := m.data[string(key)]
	if !ok {
		recordOp("memory", "get", "not_found", time.Since(start).Seconds())
		return nil, storage.ErrNotFound
	}

	// Return a copy to prevent modification
	result := CopyBytes(value)
	recordOp("memory", "get", "success", time.Since(start).Seconds())
	return result, nil
}

// Set stores a key-value pair.
func (m *MemoryStore) Set(ctx context.Context, key, value []byte) error {
	start := time.Now()

	if err := Validate(key, value, m.cfg); err != nil {
		recordOp("memory", "set", "error", time.Since(start).Seconds())
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		recordOp("memory", "set", "error", time.Since(start).Seconds())
		return storage.ErrClosed
	}

	if m.cfg.ReadOnly {
		recordOp("memory", "set", "error", time.Since(start).Seconds())
		return storage.ErrReadOnly
	}

	keyStr := string(key)

	// Update size tracking
	if existing, ok := m.data[keyStr]; ok {
		m.sizeBytes -= int64(len(existing))
	} else {
		m.keyCount++
	}

	// Store a copy to prevent modification
	m.data[keyStr] = CopyBytes(value)
	m.sizeBytes += int64(len(value))

	kvKeysTotal.WithLabels(map[string]string{"backend": "memory"}).Set(float64(m.keyCount))
	kvSizeBytes.WithLabels(map[string]string{"backend": "memory"}).Set(float64(m.sizeBytes))

	recordOp("memory", "set", "success", time.Since(start).Seconds())
	return nil
}

// Delete removes a key.
func (m *MemoryStore) Delete(ctx context.Context, key []byte) error {
	start := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		recordOp("memory", "delete", "error", time.Since(start).Seconds())
		return storage.ErrClosed
	}

	if m.cfg.ReadOnly {
		recordOp("memory", "delete", "error", time.Since(start).Seconds())
		return storage.ErrReadOnly
	}

	keyStr := string(key)
	if existing, ok := m.data[keyStr]; ok {
		m.sizeBytes -= int64(len(existing))
		m.keyCount--
		delete(m.data, keyStr)

		kvKeysTotal.WithLabels(map[string]string{"backend": "memory"}).Set(float64(m.keyCount))
		kvSizeBytes.WithLabels(map[string]string{"backend": "memory"}).Set(float64(m.sizeBytes))
	}

	recordOp("memory", "delete", "success", time.Since(start).Seconds())
	return nil
}

// Scan iterates over all keys with the given prefix.
func (m *MemoryStore) Scan(ctx context.Context, prefix []byte, fn func(key, value []byte) error) error {
	start := time.Now()

	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		recordOp("memory", "scan", "error", time.Since(start).Seconds())
		return storage.ErrClosed
	}

	// Collect and sort keys for deterministic iteration
	keys := make([]string, 0, len(m.data))
	for k := range m.data {
		if len(prefix) == 0 || bytes.HasPrefix([]byte(k), prefix) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	for _, k := range keys {
		select {
		case <-ctx.Done():
			recordOp("memory", "scan", "cancelled", time.Since(start).Seconds())
			return ctx.Err()
		default:
		}

		if err := fn(CopyBytes([]byte(k)), CopyBytes(m.data[k])); err != nil {
			recordOp("memory", "scan", "error", time.Since(start).Seconds())
			return err
		}
	}

	recordOp("memory", "scan", "success", time.Since(start).Seconds())
	return nil
}

// Close releases resources.
func (m *MemoryStore) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}

	m.closed = true
	m.data = nil
	m.log.Info().Msg("memory store closed")
	return nil
}

// Begin starts a new transaction.
func (m *MemoryStore) Begin(ctx context.Context, update bool) (storage.Transaction, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, storage.ErrClosed
	}

	if update && m.cfg.ReadOnly {
		return nil, storage.ErrReadOnly
	}

	return &memoryTxn{
		store:    m,
		update:   update,
		writes:   make(map[string][]byte),
		deletes:  make(map[string]struct{}),
		snapshot: m.snapshotLocked(),
	}, nil
}

// snapshotLocked creates a snapshot while holding the read lock.
func (m *MemoryStore) snapshotLocked() map[string][]byte {
	snapshot := make(map[string][]byte, len(m.data))
	for k, v := range m.data {
		snapshot[k] = CopyBytes(v)
	}
	return snapshot
}

// Stats returns store statistics.
func (m *MemoryStore) Stats(ctx context.Context) (storage.Stats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return storage.Stats{
		Keys: m.keyCount,
		Size: m.sizeBytes,
	}, nil
}

// memoryTxn is a transaction for the memory store.
type memoryTxn struct {
	store    *MemoryStore
	update   bool
	closed   bool
	writes   map[string][]byte
	deletes  map[string]struct{}
	snapshot map[string][]byte
	mu       sync.Mutex
}

// Get retrieves a value within the transaction.
func (t *memoryTxn) Get(key []byte) ([]byte, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil, storage.ErrTxnClosed
	}

	keyStr := string(key)

	// Check deletes first
	if _, deleted := t.deletes[keyStr]; deleted {
		return nil, storage.ErrNotFound
	}

	// Check writes
	if value, ok := t.writes[keyStr]; ok {
		return CopyBytes(value), nil
	}

	// Check snapshot
	if value, ok := t.snapshot[keyStr]; ok {
		return CopyBytes(value), nil
	}

	return nil, storage.ErrNotFound
}

// Set stores a key-value pair within the transaction.
func (t *memoryTxn) Set(key, value []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return storage.ErrTxnClosed
	}

	if !t.update {
		return storage.ErrReadOnly
	}

	if err := Validate(key, value, t.store.cfg); err != nil {
		return err
	}

	keyStr := string(key)
	delete(t.deletes, keyStr)
	t.writes[keyStr] = CopyBytes(value)
	return nil
}

// Delete removes a key within the transaction.
func (t *memoryTxn) Delete(key []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return storage.ErrTxnClosed
	}

	if !t.update {
		return storage.ErrReadOnly
	}

	keyStr := string(key)
	delete(t.writes, keyStr)
	t.deletes[keyStr] = struct{}{}
	return nil
}

// Commit applies all changes atomically.
func (t *memoryTxn) Commit() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return storage.ErrTxnClosed
	}

	t.closed = true

	if !t.update {
		return nil
	}

	t.store.mu.Lock()
	defer t.store.mu.Unlock()

	if t.store.closed {
		return storage.ErrClosed
	}

	// Apply deletes
	for k := range t.deletes {
		if existing, ok := t.store.data[k]; ok {
			t.store.sizeBytes -= int64(len(existing))
			t.store.keyCount--
			delete(t.store.data, k)
		}
	}

	// Apply writes
	for k, v := range t.writes {
		if existing, ok := t.store.data[k]; ok {
			t.store.sizeBytes -= int64(len(existing))
		} else {
			t.store.keyCount++
		}
		t.store.data[k] = v
		t.store.sizeBytes += int64(len(v))
	}

	kvKeysTotal.WithLabels(map[string]string{"backend": "memory"}).Set(float64(t.store.keyCount))
	kvSizeBytes.WithLabels(map[string]string{"backend": "memory"}).Set(float64(t.store.sizeBytes))

	return nil
}

// Rollback discards all changes.
func (t *memoryTxn) Rollback() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return storage.ErrTxnClosed
	}

	t.closed = true
	t.writes = nil
	t.deletes = nil
	t.snapshot = nil
	return nil
}

// Write applies a batch of operations atomically.
func (m *MemoryStore) Write(ctx context.Context, ops []storage.BatchOp) error {
	start := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		recordOp("memory", "batch", "error", time.Since(start).Seconds())
		return storage.ErrClosed
	}

	if m.cfg.ReadOnly {
		recordOp("memory", "batch", "error", time.Since(start).Seconds())
		return storage.ErrReadOnly
	}

	for _, op := range ops {
		keyStr := string(op.Key)

		switch op.Type {
		case storage.OpSet:
			if err := Validate(op.Key, op.Value, m.cfg); err != nil {
				return err
			}
			if existing, ok := m.data[keyStr]; ok {
				m.sizeBytes -= int64(len(existing))
			} else {
				m.keyCount++
			}
			m.data[keyStr] = CopyBytes(op.Value)
			m.sizeBytes += int64(len(op.Value))

		case storage.OpDelete:
			if existing, ok := m.data[keyStr]; ok {
				m.sizeBytes -= int64(len(existing))
				m.keyCount--
				delete(m.data, keyStr)
			}
		}
	}

	kvKeysTotal.WithLabels(map[string]string{"backend": "memory"}).Set(float64(m.keyCount))
	kvSizeBytes.WithLabels(map[string]string{"backend": "memory"}).Set(float64(m.sizeBytes))

	recordOp("memory", "batch", "success", time.Since(start).Seconds())
	return nil
}

// Compact is a no-op for memory stores.
func (m *MemoryStore) Compact(ctx context.Context) error {
	return nil
}

// GC is a no-op for memory stores.
func (m *MemoryStore) GC(ctx context.Context) error {
	return nil
}
