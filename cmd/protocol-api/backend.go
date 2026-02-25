// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package main

import (
	"fmt"
	"sync"
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
// BPFBackend — stub for real pinned BPF maps (bare-metal only)
// ---------------------------------------------------------------------------

// BPFBackend reads and writes pinned BPF maps via /sys/fs/bpf.
// This is a compile-time stub; the real implementation will use
// cilium/ebpf to open and manipulate pinned maps.
type BPFBackend struct {
	PinPath string // e.g. /sys/fs/bpf/unheaded
}

// NewBPFBackend creates a BPFBackend targeting the given pin path.
func NewBPFBackend(pinPath string) *BPFBackend {
	return &BPFBackend{PinPath: pinPath}
}

func (b *BPFBackend) Mode() string { return "bpf" }

func (b *BPFBackend) ReadMap(mapName string, key []byte) ([]byte, error) {
	// TODO: open pinned map at b.PinPath/<mapName>, lookup key
	return nil, fmt.Errorf("bpf: ReadMap not implemented (map=%s, pin=%s)", mapName, b.PinPath)
}

func (b *BPFBackend) WriteMap(mapName string, key, value []byte) error {
	// TODO: open pinned map at b.PinPath/<mapName>, update key
	return fmt.Errorf("bpf: WriteMap not implemented (map=%s, pin=%s)", mapName, b.PinPath)
}

func (b *BPFBackend) DeleteMap(mapName string, key []byte) error {
	// TODO: open pinned map at b.PinPath/<mapName>, delete key
	return fmt.Errorf("bpf: DeleteMap not implemented (map=%s, pin=%s)", mapName, b.PinPath)
}

func (b *BPFBackend) IterateMap(mapName string, fn func(key, value []byte) error) error {
	// TODO: open pinned map, iterate all entries
	return fmt.Errorf("bpf: IterateMap not implemented (map=%s, pin=%s)", mapName, b.PinPath)
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
