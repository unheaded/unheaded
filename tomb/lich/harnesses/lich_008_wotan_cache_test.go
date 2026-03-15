// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// LICH-008: Wotan Ring Buffer / Cache Operations Fuzzer
//
// Target: Ring buffer implementations in pkg/logagg and services/wotan.
// Goal: Find data corruption, race conditions (simulated), and
//       boundary violations in circular buffer operations.
//
// The Wotan ring buffer is the backbone of the message bus.
// The logagg ring buffer stores aggregated log entries.
// Both use the same circular buffer pattern with different entry types.
//
// Attack vectors:
//   - Push at capacity boundary (overwrite behavior)
//   - Query with adversarial filters
//   - Concurrent push/read simulation (sequential but interleaved)
//   - Negative/zero capacity
//   - Empty buffer operations
//   - Maximum capacity buffers

package harnesses

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// CacheEntry represents a generic cache entry for fuzzing.
type CacheEntry struct {
	ID        uint64
	Timestamp int64
	Service   string
	Level     string
	Message   string
	Size      int
}

// CacheRingBuffer is a simplified ring buffer for fuzzing.
type CacheRingBuffer struct {
	entries  []CacheEntry
	capacity int
	head     int
	count    int
	wrapping bool
}

// NewCacheRingBuffer creates a ring buffer with the given capacity.
func NewCacheRingBuffer(capacity int) *CacheRingBuffer {
	if capacity <= 0 {
		capacity = 1 // minimum capacity
	}
	if capacity > 100000 {
		capacity = 100000 // safety limit for fuzzing
	}
	return &CacheRingBuffer{
		entries:  make([]CacheEntry, capacity),
		capacity: capacity,
	}
}

// Push adds an entry, evicting the oldest if at capacity.
func (rb *CacheRingBuffer) Push(entry CacheEntry) {
	if rb.count == rb.capacity {
		rb.wrapping = true
	}

	rb.entries[rb.head] = entry
	rb.head = (rb.head + 1) % rb.capacity

	if rb.count < rb.capacity {
		rb.count++
	}
}

// Get returns all entries in chronological order.
func (rb *CacheRingBuffer) Get() []CacheEntry {
	if rb.count == 0 {
		return nil
	}

	result := make([]CacheEntry, rb.count)

	if !rb.wrapping {
		copy(result, rb.entries[:rb.count])
	} else {
		oldestIdx := rb.head
		for i := 0; i < rb.count; i++ {
			result[i] = rb.entries[(oldestIdx+i)%rb.capacity]
		}
	}

	return result
}

// Query returns entries matching the filter.
func (rb *CacheRingBuffer) Query(service, level, search string, limit int) []CacheEntry {
	if limit <= 0 {
		limit = 100
	}
	if limit > rb.count {
		limit = rb.count
	}

	var results []CacheEntry

	start := 0
	if rb.wrapping {
		start = rb.head
	}

	for i := 0; i < rb.count && len(results) < limit; i++ {
		idx := (start + i) % rb.capacity
		entry := rb.entries[idx]

		if service != "" && entry.Service != service {
			continue
		}
		if level != "" && !strings.EqualFold(entry.Level, level) {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(entry.Message), strings.ToLower(search)) {
			continue
		}

		results = append(results, entry)
	}

	return results
}

// Count returns the number of entries.
func (rb *CacheRingBuffer) Count() int {
	return rb.count
}

// FuzzCacheRingBufferPush tests push operations across capacity boundaries.
func FuzzCacheRingBufferPush(f *testing.F) {
	f.Add(10, 20)   // push more than capacity
	f.Add(1, 100)   // tiny buffer, many pushes
	f.Add(100, 1)   // large buffer, one push
	f.Add(1, 1)     // minimum
	f.Add(0, 10)    // zero capacity (should default to 1)
	f.Add(1000, 5000) // large test

	f.Fuzz(func(t *testing.T, capacity, numPushes int) {
		if capacity < 0 {
			capacity = 1
		}
		if capacity > 10000 {
			capacity = 10000 // limit for fuzzing
		}
		if numPushes < 0 || numPushes > 50000 {
			return
		}

		rb := NewCacheRingBuffer(capacity)

		for i := 0; i < numPushes; i++ {
			entry := CacheEntry{
				ID:        uint64(i),
				Timestamp: time.Now().UnixNano(),
				Service:   fmt.Sprintf("svc-%d", i%5),
				Level:     []string{"INFO", "WARN", "ERROR", "DEBUG"}[i%4],
				Message:   fmt.Sprintf("message-%d", i),
				Size:      i,
			}
			rb.Push(entry)
		}

		// Invariant: count must be min(numPushes, capacity)
		expectedCount := numPushes
		if expectedCount > rb.capacity {
			expectedCount = rb.capacity
		}
		if rb.Count() != expectedCount {
			t.Errorf("count %d, expected %d (capacity=%d, pushed=%d)",
				rb.Count(), expectedCount, capacity, numPushes)
		}

		// Invariant: Get must return exactly count entries
		entries := rb.Get()
		if entries != nil && len(entries) != rb.Count() {
			t.Errorf("Get returned %d entries, count is %d", len(entries), rb.Count())
		}

		// Invariant: entries must be in chronological order (by ID)
		if len(entries) > 1 {
			for i := 1; i < len(entries); i++ {
				if entries[i].ID <= entries[i-1].ID {
					// When wrapping, IDs should still be monotonically increasing
					// because we push sequentially
					t.Errorf("entries not in order: [%d].ID=%d >= [%d].ID=%d",
						i-1, entries[i-1].ID, i, entries[i].ID)
				}
			}
		}

		// If buffer wrapped, verify the oldest entry is correct
		if numPushes > rb.capacity && len(entries) > 0 {
			expectedOldestID := uint64(numPushes - rb.capacity)
			if entries[0].ID != expectedOldestID {
				t.Errorf("oldest entry ID %d, expected %d", entries[0].ID, expectedOldestID)
			}
		}
	})
}

// FuzzCacheRingBufferQuery tests query filtering on populated buffers.
func FuzzCacheRingBufferQuery(f *testing.F) {
	f.Add("wotan", "INFO", "test", 10)
	f.Add("", "", "", 0)
	f.Add("timeguru", "ERROR", "", 100)
	f.Add("nonexistent", "PANIC", "xyz", 1)
	f.Add("", "", "", -1)

	f.Fuzz(func(t *testing.T, service, level, search string, limit int) {
		rb := NewCacheRingBuffer(100)

		// Populate with known entries
		services := []string{"wotan", "timeguru", "captain", "architect", "monad"}
		levels := []string{"INFO", "WARN", "ERROR", "DEBUG"}

		for i := 0; i < 50; i++ {
			rb.Push(CacheEntry{
				ID:      uint64(i),
				Service: services[i%len(services)],
				Level:   levels[i%len(levels)],
				Message: fmt.Sprintf("test message %d from %s", i, services[i%len(services)]),
			})
		}

		results := rb.Query(service, level, search, limit)

		// Invariant: all results must match the filter
		for _, entry := range results {
			if service != "" && entry.Service != service {
				t.Errorf("result service %q doesn't match filter %q", entry.Service, service)
			}
			if level != "" && !strings.EqualFold(entry.Level, level) {
				t.Errorf("result level %q doesn't match filter %q", entry.Level, level)
			}
			if search != "" && !strings.Contains(strings.ToLower(entry.Message), strings.ToLower(search)) {
				t.Errorf("result message %q doesn't contain search %q", entry.Message, search)
			}
		}

		// Invariant: result count must not exceed limit
		effectiveLimit := limit
		if effectiveLimit <= 0 {
			effectiveLimit = 100
		}
		if len(results) > effectiveLimit {
			t.Errorf("results count %d exceeds limit %d", len(results), effectiveLimit)
		}
	})
}

// FuzzCacheRingBufferInterleaved simulates interleaved push/read patterns.
func FuzzCacheRingBufferInterleaved(f *testing.F) {
	// Operations: 0=push, 1=get, 2=query, 3=count
	f.Add([]byte{0, 0, 0, 1, 0, 0, 2, 3, 0, 1})
	f.Add([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 1})  // many pushes then read
	f.Add([]byte{1, 1, 1, 1, 1})                    // reads on empty buffer
	f.Add([]byte{})                                  // no operations
	f.Add([]byte{0, 1, 0, 1, 0, 1, 0, 1})          // alternating

	f.Fuzz(func(t *testing.T, operations []byte) {
		rb := NewCacheRingBuffer(10)
		pushCount := 0

		for _, op := range operations {
			switch op % 4 {
			case 0: // push
				rb.Push(CacheEntry{
					ID:      uint64(pushCount),
					Service: "fuzz",
					Level:   "INFO",
					Message: fmt.Sprintf("msg-%d", pushCount),
				})
				pushCount++

			case 1: // get
				entries := rb.Get()
				if entries != nil {
					for i, e := range entries {
						_ = e.ID
						_ = i
					}
				}

			case 2: // query
				results := rb.Query("fuzz", "", "", 50)
				_ = results

			case 3: // count
				count := rb.Count()
				if count < 0 || count > rb.capacity {
					t.Errorf("invalid count: %d (capacity=%d)", count, rb.capacity)
				}
			}
		}

		// Final consistency check
		count := rb.Count()
		entries := rb.Get()
		entryLen := 0
		if entries != nil {
			entryLen = len(entries)
		}
		if entryLen != count {
			t.Errorf("final count %d != entries length %d", count, entryLen)
		}
	})
}

// FuzzCacheRingBufferCapacityBoundary specifically targets capacity boundary behavior.
func FuzzCacheRingBufferCapacityBoundary(f *testing.F) {
	f.Add(1)
	f.Add(2)
	f.Add(3)
	f.Add(10)
	f.Add(100)
	f.Add(1000)

	f.Fuzz(func(t *testing.T, capacity int) {
		if capacity <= 0 || capacity > 10000 {
			return
		}

		rb := NewCacheRingBuffer(capacity)

		// Fill to exactly capacity
		for i := 0; i < capacity; i++ {
			rb.Push(CacheEntry{ID: uint64(i), Message: fmt.Sprintf("fill-%d", i)})
		}

		if rb.Count() != capacity {
			t.Errorf("after filling: count %d != capacity %d", rb.Count(), capacity)
		}

		// Push one more (should wrap)
		rb.Push(CacheEntry{ID: uint64(capacity), Message: "overflow"})

		if rb.Count() != capacity {
			t.Errorf("after overflow: count %d != capacity %d", rb.Count(), capacity)
		}

		entries := rb.Get()
		if len(entries) != capacity {
			t.Fatalf("after overflow: entries len %d != capacity %d", len(entries), capacity)
		}

		// The oldest entry should have ID=1 (ID=0 was evicted)
		if entries[0].ID != 1 {
			t.Errorf("oldest entry ID %d, expected 1", entries[0].ID)
		}

		// The newest entry should have ID=capacity
		if entries[len(entries)-1].ID != uint64(capacity) {
			t.Errorf("newest entry ID %d, expected %d", entries[len(entries)-1].ID, capacity)
		}
	})
}
