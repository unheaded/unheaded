// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package kv provides key-value storage implementations for the Kingdom.
// This package contains multiple backends including memory, BadgerDB, and BoltDB.
package kv

import (
	"context"
	"fmt"

	"unheaded/pkg/logger"
	"unheaded/pkg/metrics"
	"unheaded/pkg/storage"
)

// Backend represents a storage backend type.
type Backend string

const (
	// BackendMemory is the in-memory storage backend.
	BackendMemory Backend = "memory"
	// BackendBadger is the BadgerDB storage backend.
	BackendBadger Backend = "badger"
	// BackendBolt is the BoltDB storage backend.
	BackendBolt Backend = "bolt"
)

// Config holds key-value store configuration.
type Config struct {
	// Backend is the storage backend to use.
	Backend Backend `json:"backend" yaml:"backend"`

	// Path is the data directory for persistent backends.
	Path string `json:"path" yaml:"path"`

	// MaxKeySize is the maximum key size in bytes.
	MaxKeySize int `json:"max_key_size" yaml:"max_key_size"`

	// MaxValueSize is the maximum value size in bytes.
	MaxValueSize int `json:"max_value_size" yaml:"max_value_size"`

	// SyncWrites enables synchronous writes.
	SyncWrites bool `json:"sync_writes" yaml:"sync_writes"`

	// ReadOnly opens the store in read-only mode.
	ReadOnly bool `json:"read_only" yaml:"read_only"`

	// Logger is the logger instance.
	Logger *logger.Logger

	// Metrics is the metrics registry.
	Metrics *metrics.Registry
}

// DefaultConfig returns a default configuration.
func DefaultConfig() Config {
	return Config{
		Backend:      BackendMemory,
		Path:         "/var/lib/unheaded/kv",
		MaxKeySize:   1024,
		MaxValueSize: 64 * 1024 * 1024,
		SyncWrites:   true,
		ReadOnly:     false,
	}
}

// New creates a new key-value store based on configuration.
func New(cfg Config) (storage.KVStore, error) {
	switch cfg.Backend {
	case BackendMemory:
		return NewMemoryStore(cfg), nil
	case BackendBadger:
		return NewBadgerStore(cfg)
	case BackendBolt:
		return NewBoltStore(cfg)
	default:
		return nil, fmt.Errorf("kv: unknown backend: %s", cfg.Backend)
	}
}

// Metrics for key-value operations.
var (
	kvOpsTotal = metrics.NewCounterVec(
		"unheaded_kv_ops_total",
		"Total key-value operations",
		nil,
		[]string{"backend", "operation", "status"},
	)

	kvOpsDuration = metrics.NewHistogramVec(
		metrics.HistogramOpts{
			Name:    "unheaded_kv_ops_duration_seconds",
			Help:    "Key-value operation duration",
			Buckets: metrics.ExponentialBuckets(0.0001, 2, 12),
		},
		[]string{"backend", "operation"},
	)

	kvKeysTotal = metrics.NewGaugeVec(
		"unheaded_kv_keys_total",
		"Total number of keys in store",
		nil,
		[]string{"backend"},
	)

	kvSizeBytes = metrics.NewGaugeVec(
		"unheaded_kv_size_bytes",
		"Total size of key-value data",
		nil,
		[]string{"backend"},
	)
)

// recordOp records a key-value operation for metrics.
func recordOp(backend, operation, status string, duration float64) {
	kvOpsTotal.WithLabels(metrics.Labels{
		"backend":   backend,
		"operation": operation,
		"status":    status,
	}).Inc()

	kvOpsDuration.WithLabels(metrics.Labels{
		"backend":   backend,
		"operation": operation,
	}).Observe(duration)
}

// Iterator provides iteration over key-value pairs.
type Iterator interface {
	// Next advances to the next key-value pair.
	// Returns false when iteration is complete.
	Next() bool

	// Key returns the current key.
	Key() []byte

	// Value returns the current value.
	Value() []byte

	// Error returns any error encountered during iteration.
	Error() error

	// Close releases iterator resources.
	Close()
}

// Prefix creates a prefixed view of a store.
type Prefix struct {
	store  storage.KVStore
	prefix []byte
}

// NewPrefix creates a prefixed store view.
func NewPrefix(store storage.KVStore, prefix []byte) *Prefix {
	return &Prefix{
		store:  store,
		prefix: prefix,
	}
}

// Get retrieves a value with the prefix applied.
func (p *Prefix) Get(ctx context.Context, key []byte) ([]byte, error) {
	return p.store.Get(ctx, p.prefixKey(key))
}

// Set stores a value with the prefix applied.
func (p *Prefix) Set(ctx context.Context, key, value []byte) error {
	return p.store.Set(ctx, p.prefixKey(key), value)
}

// Delete removes a key with the prefix applied.
func (p *Prefix) Delete(ctx context.Context, key []byte) error {
	return p.store.Delete(ctx, p.prefixKey(key))
}

// Scan iterates over keys with the prefix applied.
func (p *Prefix) Scan(ctx context.Context, prefix []byte, fn func(key, value []byte) error) error {
	fullPrefix := p.prefixKey(prefix)
	return p.store.Scan(ctx, fullPrefix, func(key, value []byte) error {
		// Strip the prefix from the key before passing to callback
		unprefixedKey := key[len(p.prefix):]
		return fn(unprefixedKey, value)
	})
}

// Close is a no-op for prefix views.
func (p *Prefix) Close() error {
	return nil
}

func (p *Prefix) prefixKey(key []byte) []byte {
	result := make([]byte, len(p.prefix)+len(key))
	copy(result, p.prefix)
	copy(result[len(p.prefix):], key)
	return result
}

// Validate checks if a key and value are valid.
func Validate(key, value []byte, cfg Config) error {
	if len(key) == 0 {
		return fmt.Errorf("kv: empty key")
	}
	if cfg.MaxKeySize > 0 && len(key) > cfg.MaxKeySize {
		return storage.ErrKeyTooLarge
	}
	if cfg.MaxValueSize > 0 && len(value) > cfg.MaxValueSize {
		return storage.ErrValueTooLarge
	}
	return nil
}

// CopyBytes creates a copy of a byte slice.
func CopyBytes(b []byte) []byte {
	if b == nil {
		return nil
	}
	cp := make([]byte, len(b))
	copy(cp, b)
	return cp
}
