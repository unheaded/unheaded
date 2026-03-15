// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package compute

import (
	"fmt"
	"sync"
)

// BPFMapCache abstracts interaction with the BPF RAM_MAP hash map.
// In production this wraps cilium/ebpf map operations on the pinned
// /sys/fs/bpf/unheaded/monad_cpu/ram_map. In tests it's a simple
// in-memory mock.
type BPFMapCache interface {
	// ReadWord reads a 32-bit word at the given word address (byte_addr >> 2).
	ReadWord(wordAddr uint32) (uint32, error)

	// WriteWord writes a 32-bit word at the given word address.
	WriteWord(wordAddr uint32, value uint32) error

	// WritePage writes a contiguous run of words starting at startWordAddr.
	WritePage(startWordAddr uint32, words []uint32) error

	// DeleteWord removes a word from the cache (for eviction).
	DeleteWord(wordAddr uint32) error

	// Len returns the number of entries currently in the cache.
	Len() int
}

// MemoryBPFMap is an in-memory mock of the BPF RAM_MAP for testing
// and development without real eBPF. Thread-safe.
type MemoryBPFMap struct {
	mu      sync.RWMutex
	entries map[uint32]uint32
	maxSize int
}

// NewMemoryBPFMap creates a mock BPF map with the given max entry count.
// Pass 0 for unlimited (test use).
func NewMemoryBPFMap(maxSize int) *MemoryBPFMap {
	return &MemoryBPFMap{
		entries: make(map[uint32]uint32),
		maxSize: maxSize,
	}
}

func (m *MemoryBPFMap) ReadWord(wordAddr uint32) (uint32, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.entries[wordAddr]
	if !ok {
		return 0, nil // BPF hash map returns 0 for missing keys
	}
	return v, nil
}

func (m *MemoryBPFMap) WriteWord(wordAddr uint32, value uint32) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.maxSize > 0 && len(m.entries) >= m.maxSize {
		if _, exists := m.entries[wordAddr]; !exists {
			return fmt.Errorf("bpf map full (%d entries)", m.maxSize)
		}
	}
	m.entries[wordAddr] = value
	return nil
}

func (m *MemoryBPFMap) WritePage(startWordAddr uint32, words []uint32) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, w := range words {
		addr := startWordAddr + uint32(i)
		if m.maxSize > 0 && len(m.entries) >= m.maxSize {
			if _, exists := m.entries[addr]; !exists {
				return fmt.Errorf("bpf map full at word %d of page", i)
			}
		}
		m.entries[addr] = w
	}
	return nil
}

func (m *MemoryBPFMap) DeleteWord(wordAddr uint32) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.entries, wordAddr)
	return nil
}

func (m *MemoryBPFMap) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.entries)
}

// Snapshot returns a copy of all entries (for testing).
func (m *MemoryBPFMap) Snapshot() map[uint32]uint32 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cp := make(map[uint32]uint32, len(m.entries))
	for k, v := range m.entries {
		cp[k] = v
	}
	return cp
}
