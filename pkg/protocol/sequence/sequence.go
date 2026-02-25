// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package sequence provides namespace sequence counter tracking for the Unheaded protocol.
package sequence

import (
	"sync"
)

// NamespaceCounter represents a monotonically increasing sequence counter per namespace.
type NamespaceCounter struct {
	mu       sync.RWMutex
	counter  uint32
	namespace string
}

// NewNamespaceCounter creates a new namespace counter for the given namespace.
func NewNamespaceCounter(namespace string) *NamespaceCounter {
	return &NamespaceCounter{
		namespace: namespace,
		counter:   0,
	}
}

// Increment increments the counter and returns the new value.
func (nc *NamespaceCounter) Increment() uint32 {
	nc.mu.Lock()
	defer nc.mu.Unlock()
	nc.counter++
	return nc.counter
}

// Current returns the current counter value without incrementing.
func (nc *NamespaceCounter) Current() uint32 {
	nc.mu.RLock()
	defer nc.mu.RUnlock()
	return nc.counter
}

// DetectGap analyzes the difference between expected and received sequence numbers.
// Returns the number of lost packets and whether reordering was detected.
func (nc *NamespaceCounter) DetectGap(expected, received uint32) (lost int, reordered bool) {
	if received < expected {
		return 0, true // packet arrived out of order
	}
	if received == expected {
		return 0, false // no gap, no reorder
	}
	// received > expected: gap indicates packet loss
	gap := int(received - expected)
	return gap - 1, false
}

// SequenceTracker tracks per-hop namespace counters for the Unheaded protocol.
type SequenceTracker struct {
	mu        sync.RWMutex
	counters  map[string]*NamespaceCounter
	expected  map[string]uint32 // expected next sequence per namespace
}

// NewSequenceTracker creates a new sequence tracker.
func NewSequenceTracker() *SequenceTracker {
	return &SequenceTracker{
		counters: make(map[string]*NamespaceCounter),
		expected: make(map[string]uint32),
	}
}

// GetOrCreateCounter returns or creates a counter for the given namespace.
func (st *SequenceTracker) GetOrCreateCounter(namespace string) *NamespaceCounter {
	st.mu.Lock()
	defer st.mu.Unlock()

	if counter, exists := st.counters[namespace]; exists {
		return counter
	}

	counter := NewNamespaceCounter(namespace)
	st.counters[namespace] = counter
	st.expected[namespace] = 1 // expect first sequence to be 1
	return counter
}

// ValidateMonotonicity validates that the received sequence is monotonically increasing.
// Returns the number of lost packets and whether reordering was detected.
func (st *SequenceTracker) ValidateMonotonicity(namespace string, received uint32) (lost int, reordered bool, err error) {
	counter := st.GetOrCreateCounter(namespace)

	st.mu.RLock()
	expectedSeq := st.expected[namespace]
	st.mu.RUnlock()

	lost, reordered = counter.DetectGap(expectedSeq, received)

	// Update expected to next expected sequence
	st.mu.Lock()
	st.expected[namespace] = received + 1
	st.mu.Unlock()

	return lost, reordered, nil
}

// Increment increments the counter for the given namespace and returns the new value.
func (st *SequenceTracker) Increment(namespace string) uint32 {
	counter := st.GetOrCreateCounter(namespace)
	return counter.Increment()
}

// Current returns the current counter value for the given namespace.
func (st *SequenceTracker) Current(namespace string) uint32 {
	counter := st.GetOrCreateCounter(namespace)
	return counter.Current()
}
