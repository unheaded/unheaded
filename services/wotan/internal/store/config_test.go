// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package store

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Type != MemoryStoreType {
		t.Errorf("Type = %v, want %v", cfg.Type, MemoryStoreType)
	}
	if cfg.Capacity != 10000 {
		t.Errorf("Capacity = %v, want 10000", cfg.Capacity)
	}
	if cfg.SyncMode != "batch" {
		t.Errorf("SyncMode = %v, want batch", cfg.SyncMode)
	}
	if cfg.BatchSize != 100 {
		t.Errorf("BatchSize = %v, want 100", cfg.BatchSize)
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name:    "valid memory config",
			cfg:     Config{Type: MemoryStoreType, Capacity: 1000},
			wantErr: false,
		},
		{
			name:    "empty type",
			cfg:     Config{Type: "", Capacity: 1000},
			wantErr: true,
		},
		{
			name:    "unknown type",
			cfg:     Config{Type: "invalid", Capacity: 1000},
			wantErr: true,
		},
		{
			name:    "negative capacity",
			cfg:     Config{Type: MemoryStoreType, Capacity: -1},
			wantErr: true,
		},
		{
			name:    "zero capacity is valid",
			cfg:     Config{Type: MemoryStoreType, Capacity: 0},
			wantErr: false,
		},
		{
			name:    "invalid sync mode",
			cfg:     Config{Type: MemoryStoreType, SyncMode: "invalid"},
			wantErr: true,
		},
		{
			name:    "valid sync modes",
			cfg:     Config{Type: MemoryStoreType, SyncMode: "immediate"},
			wantErr: false,
		},
		{
			name:    "negative batch size",
			cfg:     Config{Type: MemoryStoreType, BatchSize: -1},
			wantErr: true,
		},
		{
			name:    "wal without data_dir",
			cfg:     Config{Type: WALStoreType, Capacity: 1000, DataDir: ""},
			wantErr: true,
		},
		{
			name:    "wal with data_dir",
			cfg:     Config{Type: WALStoreType, Capacity: 1000, DataDir: "/tmp/wal"},
			wantErr: false,
		},
		{
			name:    "sqlite without data_dir",
			cfg:     Config{Type: SQLiteStoreType, Capacity: 1000, DataDir: ""},
			wantErr: true,
		},
		{
			name:    "postgres is no longer a store type",
			cfg:     Config{Type: "postgres", Capacity: 1000, DataDir: "/tmp/x"},
			wantErr: true,
		},
		{
			name:    "hybrid is no longer a store type",
			cfg:     Config{Type: "hybrid", Capacity: 1000, DataDir: "/tmp/x"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name:    "memory store",
			cfg:     Config{Type: MemoryStoreType, Capacity: 100},
			wantErr: false,
		},
		{
			name:    "default config",
			cfg:     Config{},
			wantErr: false, // Empty type becomes default
		},
		{
			name:    "wal store with valid dir",
			cfg:     Config{Type: WALStoreType, DataDir: "/tmp/wal-test-" + fmt.Sprintf("%d", time.Now().UnixNano())},
			wantErr: false,
		},
		{
			name:    "sqlite store not implemented",
			cfg:     Config{Type: SQLiteStoreType, DataDir: "/tmp/sqlite"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, err := New(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if store != nil {
				store.Close()
			}
		})
	}
}

func TestMustNew(t *testing.T) {
	// Should not panic with valid config
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("MustNew() panicked with valid config: %v", r)
		}
	}()

	store := MustNew(Config{Type: MemoryStoreType, Capacity: 100})
	store.Close()
}

func TestMustNew_Panic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustNew() should panic with invalid config")
		}
	}()

	// This should panic - invalid type
	MustNew(Config{Type: "definitely-invalid-type"})
}

func TestStoreType_String(t *testing.T) {
	if MemoryStoreType != "memory" {
		t.Error("MemoryStoreType should be 'memory'")
	}
	if WALStoreType != "wal" {
		t.Error("WALStoreType should be 'wal'")
	}
	if SQLiteStoreType != "sqlite" {
		t.Error("SQLiteStoreType should be 'sqlite'")
	}
}

// Wotan's Postgres and hybrid (WAL + Postgres) stores were removed
// 2026-11-09 (ADR-005: Wotan is an in-memory store; ADR-035 amendment).
// Naming either must fail as an unknown type, never quietly pick a backend.
func TestNew_RefusesRemovedStoreTypes(t *testing.T) {
	for _, typ := range []StoreType{"postgres", "hybrid"} {
		t.Run(string(typ), func(t *testing.T) {
			s, err := New(Config{Type: typ, Capacity: 10, DataDir: t.TempDir()})
			if err == nil {
				if s != nil {
					s.Close()
				}
				t.Fatalf("New(%q) succeeded; want an unknown-type error", typ)
			}
			if !strings.Contains(err.Error(), "unknown type") {
				t.Errorf("New(%q) error = %v; want it to say unknown type", typ, err)
			}
		})
	}
}
