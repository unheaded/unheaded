// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package store

import (
	"testing"
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
			name:    "postgres without data_dir uses default",
			cfg:     Config{Type: PostgresStoreType, Capacity: 1000, DataDir: ""},
			wantErr: false, // PG uses default connstr when DataDir empty
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
			name:    "wal store not implemented",
			cfg:     Config{Type: WALStoreType, DataDir: "/tmp/wal"},
			wantErr: true,
		},
		{
			name:    "sqlite store not implemented",
			cfg:     Config{Type: SQLiteStoreType, DataDir: "/tmp/sqlite"},
			wantErr: true,
		},
		{
			name:    "postgres store with bad connstr",
			cfg:     Config{Type: PostgresStoreType, DataDir: "postgres://badhost:9999/nodb?connect_timeout=1"},
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
	if PostgresStoreType != "postgres" {
		t.Error("PostgresStoreType should be 'postgres'")
	}
}
