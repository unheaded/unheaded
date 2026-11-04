// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"fmt"
	"path/filepath"
	"sync"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"unheaded/pkg/ebpf"
)

// Backend abstracts BPF map operations so services can run against
// either real pinned BPF maps or an in-memory mock store.
type Backend interface {
	// Mode returns "mock" or "bpf".
	Mode() string

	// ReadMap reads a value from the named map by key.
	// Returns (nil, nil) when the key does not exist.
	ReadMap(mapName string, key []byte) ([]byte, error)

	// WriteMap writes a key/value pair into the named map.
	WriteMap(mapName string, key, value []byte) error

	// DeleteMap removes a key from the named map.
	DeleteMap(mapName string, key []byte) error

	// IterateMap calls fn for every key/value pair in the named map.
	// Return a non-nil error from fn to stop iteration early.
	IterateMap(mapName string, fn func(key, value []byte) error) error
}

// ---------------------------------------------------------------------------
// MockBackend — in-memory maps for dev/testing
// ---------------------------------------------------------------------------

// MockBackend stores BPF-like maps entirely in memory.
type MockBackend struct {
	mu   sync.RWMutex
	maps map[string]map[string][]byte // mapName -> hex(key) -> value
}

// NewMockBackend creates a ready-to-use MockBackend.
func NewMockBackend() *MockBackend {
	return &MockBackend{
		maps: make(map[string]map[string][]byte),
	}
}

func (m *MockBackend) Mode() string { return "mock" }

func (m *MockBackend) ReadMap(mapName string, key []byte) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	table, ok := m.maps[mapName]
	if !ok {
		return nil, nil
	}
	val, ok := table[string(key)]
	if !ok {
		return nil, nil
	}
	// Return a copy so callers can't mutate the store.
	out := make([]byte, len(val))
	copy(out, val)
	return out, nil
}

func (m *MockBackend) WriteMap(mapName string, key, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	table, ok := m.maps[mapName]
	if !ok {
		table = make(map[string][]byte)
		m.maps[mapName] = table
	}
	dst := make([]byte, len(value))
	copy(dst, value)
	table[string(key)] = dst
	return nil
}

func (m *MockBackend) DeleteMap(mapName string, key []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	table, ok := m.maps[mapName]
	if !ok {
		return nil
	}
	delete(table, string(key))
	return nil
}

func (m *MockBackend) IterateMap(mapName string, fn func(key, value []byte) error) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	table, ok := m.maps[mapName]
	if !ok {
		return nil
	}
	for k, v := range table {
		if err := fn([]byte(k), v); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// BPFBackend — pinned BPF maps through the in-house pkg/ebpf (DP-211)
// ---------------------------------------------------------------------------

// BPFBackend reads and writes maps pinned under PinPath with pkg/ebpf. A map
// name is relative to PinPath: cmd/ebpf-loader --pin-maps pins each program's
// maps at <base>/<program>/<MAP>, so names read "hop-ebpf/SOPHIA".
type BPFBackend struct {
	PinPath string // e.g. /sys/fs/bpf/unheaded
}

// NewBPFBackend creates a BPFBackend targeting the given pin path.
func NewBPFBackend(pinPath string) *BPFBackend {
	return &BPFBackend{PinPath: pinPath}
}

func (b *BPFBackend) Mode() string { return "bpf" }

func (b *BPFBackend) open(mapName string) (*ebpf.PinnedMap, error) {
	m, err := ebpf.OpenPinnedMap(filepath.Join(b.PinPath, mapName))
	if err != nil {
		return nil, fmt.Errorf("open pinned map %s: %w", mapName, err)
	}
	return m, nil
}

// ReadMap returns (nil, nil) for an absent key, as the Backend contract says.
func (b *BPFBackend) ReadMap(mapName string, key []byte) ([]byte, error) {
	m, err := b.open(mapName)
	if err != nil {
		return nil, err
	}
	defer m.Close()
	value, err := m.Lookup(key)
	if err != nil {
		return nil, fmt.Errorf("lookup key in %s: %w", mapName, err)
	}
	return value, nil
}

func (b *BPFBackend) WriteMap(mapName string, key, value []byte) error {
	m, err := b.open(mapName)
	if err != nil {
		return err
	}
	defer m.Close()
	if err := m.Update(key, value); err != nil {
		return fmt.Errorf("update key in %s: %w", mapName, err)
	}
	return nil
}

func (b *BPFBackend) DeleteMap(mapName string, key []byte) error {
	m, err := b.open(mapName)
	if err != nil {
		return err
	}
	defer m.Close()
	if err := m.Delete(key); err != nil {
		return fmt.Errorf("delete key in %s: %w", mapName, err)
	}
	return nil
}

func (b *BPFBackend) IterateMap(mapName string, fn func(key, value []byte) error) error {
	m, err := b.open(mapName)
	if err != nil {
		return err
	}
	defer m.Close()
	if err := m.Iterate(fn); err != nil {
		return fmt.Errorf("iterate %s: %w", mapName, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Global backend instance — set once in main()
// ---------------------------------------------------------------------------

var globalBackend Backend

// IsMockMode returns true when running against the in-memory mock backend.
// This replaces the old globalMockMode variable and keeps the check in one
// place so callers don't depend on the concrete backend type.
func IsMockMode() bool {
	if globalBackend == nil {
		return true // safe default: mock
	}
	return globalBackend.Mode() == "mock"
}

// errBPFNotImplemented answers a gRPC call --mode=bpf cannot serve yet.
// Until 2026-10-23 these calls returned the mock data while /mode said
// "bpf".
func errBPFNotImplemented(op string) error {
	return status.Errorf(codes.Unimplemented, "%s: not implemented in bpf mode; --mode=mock serves sample data", op)
}
