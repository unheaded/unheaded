// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// loader.go — BPF program loader interface and mock implementation.
//
// Defines the interface between the trace-collector and BPF programs.
// In production, the real implementation uses pkg/ebpf (which wraps the
// bpf() syscall directly). For testing, MockBPFLoader provides in-memory
// map storage.
//
// Per RFC 9669: we use "BPF" (not "eBPF") throughout.
package main

import (
	"context"
	"fmt"
	"sync"
)

// ── BPFLoader interface ─────────────────────────────────────────────────────

// BPFLoader abstracts loading and attaching BPF programs.
// Real implementation backed by pkg/ebpf; tests use MockBPFLoader.
type BPFLoader interface {
	// Load loads a BPF program from an ELF object file.
	Load(programPath string) error

	// AttachXDP attaches the loaded XDP program to a network interface.
	AttachXDP(iface string) error

	// GetStatsMap returns a reader for flow_tracker's FLOW_STATS counters.
	GetStatsMap() MapReader

	// GetFlowStateMap returns a reader for the FLOW_STATE map.
	GetFlowStateMap() MapReader

	// GetLatencyMap returns a reader for the LATENCY_MAP.
	GetLatencyMap() MapReader

	// GetPacketEventsCh returns a channel that streams raw packet events
	// from the PACKET_EVENTS ring buffer via mmap, or an error saying why
	// there is none (program not attached, no kernel support). It is the
	// only source of packet traces: there is no poll fallback.
	GetPacketEventsCh(ctx context.Context) (<-chan []byte, error)

	// Close detaches programs and closes all map file descriptors.
	Close() error
}

// MapReader provides access to BPF map contents.
type MapReader interface {
	// Iterate calls fn for each key-value pair in the map.
	// The callback receives copies of the key and value bytes.
	Iterate(fn func(key, value []byte) error) error

	// Lookup reads the value for a given key. Returns nil if not found.
	Lookup(key []byte) ([]byte, error)

	// Delete removes a key from the map.
	Delete(key []byte) error

	// Len returns the current number of entries (if supported).
	Len() int
}

// ── MockBPFLoader ───────────────────────────────────────────────────────────

// MockBPFLoader provides an in-memory BPF loader for testing without kernel.
type MockBPFLoader struct {
	mu             sync.Mutex
	loaded         bool
	attached       bool
	iface          string
	programPath    string
	statsMap       *MockMapReader
	flowMap        *MockMapReader
	latencyMap     *MockMapReader
	packetEventsCh chan []byte // optional: set by test to simulate ringbuf
	closed         bool
}

// NewMockBPFLoader creates a new mock loader with empty maps.
func NewMockBPFLoader() *MockBPFLoader {
	return &MockBPFLoader{
		statsMap:   NewMockMapReader(),
		flowMap:    NewMockMapReader(),
		latencyMap: NewMockMapReader(),
	}
}

func (m *MockBPFLoader) Load(programPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return fmt.Errorf("loader closed")
	}
	m.programPath = programPath
	m.loaded = true
	return nil
}

func (m *MockBPFLoader) AttachXDP(iface string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.loaded {
		return fmt.Errorf("program not loaded")
	}
	if m.closed {
		return fmt.Errorf("loader closed")
	}
	m.iface = iface
	m.attached = true
	return nil
}

func (m *MockBPFLoader) GetStatsMap() MapReader {
	return m.statsMap
}

func (m *MockBPFLoader) GetFlowStateMap() MapReader {
	return m.flowMap
}

func (m *MockBPFLoader) GetLatencyMap() MapReader {
	return m.latencyMap
}

func (m *MockBPFLoader) GetPacketEventsCh(_ context.Context) (<-chan []byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.packetEventsCh == nil {
		return nil, fmt.Errorf("mock loader has no PACKET_EVENTS ring buffer")
	}
	return m.packetEventsCh, nil
}

func (m *MockBPFLoader) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	m.attached = false
	return nil
}

// IsLoaded returns whether the program has been loaded.
func (m *MockBPFLoader) IsLoaded() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loaded
}

// IsAttached returns whether the program is attached to an interface.
func (m *MockBPFLoader) IsAttached() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.attached
}

// Interface returns the interface name the program is attached to.
func (m *MockBPFLoader) Interface() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.iface
}

// ── MockMapReader ───────────────────────────────────────────────────────────

// MockMapReader provides in-memory BPF map storage for testing.
type MockMapReader struct {
	mu      sync.Mutex
	entries map[string][]byte // key hex -> value bytes
}

// NewMockMapReader creates a new empty mock map.
func NewMockMapReader() *MockMapReader {
	return &MockMapReader{
		entries: make(map[string][]byte),
	}
}

func (m *MockMapReader) Iterate(fn func(key, value []byte) error) error {
	m.mu.Lock()
	// Copy entries to avoid holding lock during callback
	entries := make(map[string][]byte, len(m.entries))
	for k, v := range m.entries {
		entries[k] = v
	}
	m.mu.Unlock()

	for k, v := range entries {
		key := []byte(k)
		if err := fn(key, v); err != nil {
			return err
		}
	}
	return nil
}

func (m *MockMapReader) Lookup(key []byte) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.entries[string(key)]
	if !ok {
		return nil, nil
	}
	// Return a copy
	result := make([]byte, len(v))
	copy(result, v)
	return result, nil
}

func (m *MockMapReader) Delete(key []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.entries, string(key))
	return nil
}

func (m *MockMapReader) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.entries)
}

// Insert adds a key-value pair to the mock map (test helper).
func (m *MockMapReader) Insert(key, value []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v := make([]byte, len(value))
	copy(v, value)
	m.entries[string(key)] = v
}

// NativeBPFLoader is implemented in loader_native.go (linux build tag).
// It bridges BPFLoader to pkg/ebpf.NativeLoader for production BPF operations.
