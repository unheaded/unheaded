// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package store

import (
	"fmt"
	"strings"
)

// StoreType represents the type of storage backend.
type StoreType string

const (
	// MemoryStoreType is an in-memory ring buffer (default).
	MemoryStoreType StoreType = "memory"

	// WALStoreType is a write-ahead log backed store (future).
	WALStoreType StoreType = "wal"

	// SQLiteStoreType is a SQLite backed store (future).
	SQLiteStoreType StoreType = "sqlite"

	// PostgresStoreType is a PostgreSQL backed store (future).
	PostgresStoreType StoreType = "postgres"
)

// Config holds configuration for store creation.
type Config struct {
	// Type specifies the storage backend to use.
	Type StoreType `json:"type" yaml:"type"`

	// Capacity is the maximum number of messages to store.
	// Interpretation depends on store type:
	// - memory: ring buffer size (overwrites oldest when full)
	// - wal: hint for segment size
	// - sqlite/postgres: max rows (may trigger pruning)
	// Default: 10000
	Capacity int `json:"capacity" yaml:"capacity"`

	// DataDir is the directory for persistent stores (wal, sqlite).
	// Ignored for memory store.
	DataDir string `json:"data_dir" yaml:"data_dir"`

	// SyncMode controls durability vs performance tradeoff.
	// Options: "immediate", "batch", "async"
	// Default: "batch"
	SyncMode string `json:"sync_mode" yaml:"sync_mode"`

	// BatchSize is the number of operations to batch before sync.
	// Only used when SyncMode is "batch".
	// Default: 100
	BatchSize int `json:"batch_size" yaml:"batch_size"`

	// RetentionPeriod is how long to keep messages before pruning.
	// Only applies to persistent stores.
	// Format: Go duration string (e.g., "24h", "7d")
	// Default: "" (no expiration)
	RetentionPeriod string `json:"retention_period" yaml:"retention_period"`

	// CompactionInterval is how often to run compaction.
	// Only applies to stores that support compaction.
	// Format: Go duration string
	// Default: "1h"
	CompactionInterval string `json:"compaction_interval" yaml:"compaction_interval"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Type:               MemoryStoreType,
		Capacity:           10000,
		DataDir:            "",
		SyncMode:           "batch",
		BatchSize:          100,
		RetentionPeriod:    "",
		CompactionInterval: "1h",
	}
}

// Validate checks the configuration for errors.
func (c *Config) Validate() error {
	switch c.Type {
	case MemoryStoreType, WALStoreType, SQLiteStoreType, PostgresStoreType:
		// Valid types
	case "":
		return fmt.Errorf("store: type is required")
	default:
		return fmt.Errorf("store: unknown type %q", c.Type)
	}

	if c.Capacity < 0 {
		return fmt.Errorf("store: capacity must be non-negative")
	}

	switch strings.ToLower(c.SyncMode) {
	case "immediate", "batch", "async", "":
		// Valid modes
	default:
		return fmt.Errorf("store: unknown sync_mode %q", c.SyncMode)
	}

	if c.BatchSize < 0 {
		return fmt.Errorf("store: batch_size must be non-negative")
	}

	// DataDir required for persistent stores
	if c.Type != MemoryStoreType && c.DataDir == "" {
		return fmt.Errorf("store: data_dir required for %s store", c.Type)
	}

	return nil
}

// New creates a new MessageStore based on the configuration.
// This is the primary factory function for creating stores.
func New(cfg Config) (MessageStore, error) {
	if cfg.Type == "" {
		cfg = DefaultConfig()
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	capacity := cfg.Capacity
	if capacity <= 0 {
		capacity = 10000
	}

	switch cfg.Type {
	case MemoryStoreType:
		return NewMemoryStore(capacity), nil

	case WALStoreType:
		// Future: return NewWALStore(cfg)
		return nil, fmt.Errorf("store: wal store not yet implemented")

	case SQLiteStoreType:
		// Future: return NewSQLiteStore(cfg)
		return nil, fmt.Errorf("store: sqlite store not yet implemented")

	case PostgresStoreType:
		// Future: return NewPostgresStore(cfg)
		return nil, fmt.Errorf("store: postgres store not yet implemented")

	default:
		return nil, fmt.Errorf("store: unknown type %q", cfg.Type)
	}
}

// MustNew creates a new MessageStore or panics on error.
// Useful for initialization in main() or tests.
func MustNew(cfg Config) MessageStore {
	store, err := New(cfg)
	if err != nil {
		panic(err)
	}
	return store
}
