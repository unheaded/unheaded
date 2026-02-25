// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package doom

import (
	"fmt"
	"sync"
)

// mockMap implements MapAccessor for testing without real BPF maps.
type mockMap struct {
	mu   sync.Mutex
	data map[string][]byte // key = hex(key bytes) -> value bytes
}

func newMockMap() *mockMap {
	return &mockMap{
		data: make(map[string][]byte),
	}
}

func (m *mockMap) LookupElem(key []byte, valueSize int) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	k := fmt.Sprintf("%x", key)
	val, ok := m.data[k]
	if !ok {
		return nil, fmt.Errorf("key not found: %s", k)
	}

	// Pad or truncate to requested size
	result := make([]byte, valueSize)
	copy(result, val)
	return result, nil
}

func (m *mockMap) UpdateElem(key, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	k := fmt.Sprintf("%x", key)
	v := make([]byte, len(value))
	copy(v, value)
	m.data[k] = v
	return nil
}

// count returns the number of entries in the mock map.
func (m *mockMap) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.data)
}

// errorMap is a MapAccessor that always returns an error.
type errorMap struct {
	err error
}

func newErrorMap(msg string) *errorMap {
	return &errorMap{err: fmt.Errorf("%s", msg)}
}

func (m *errorMap) LookupElem(key []byte, valueSize int) ([]byte, error) {
	return nil, m.err
}

func (m *errorMap) UpdateElem(key, value []byte) error {
	return m.err
}
