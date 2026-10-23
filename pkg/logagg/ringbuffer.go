// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package logagg

import (
	"math"
	"strings"
	"sync"
)

// DefaultCapacity is the default ring buffer capacity per the battle plan (10K entries).
const DefaultCapacity = 10000

// RingBuffer is a thread-safe circular buffer for log entries.
// It retains the last N entries, evicting the oldest when full.
type RingBuffer struct {
	mu       sync.RWMutex
	entries  []LogEntry
	capacity int
	head     int    // next write position
	count    int    // total entries stored
	written  uint64 // entries ever pushed; the sequence number of the newest
}

// NewRingBuffer creates a ring buffer with the given capacity.
func NewRingBuffer(capacity int) *RingBuffer {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	return &RingBuffer{
		entries:  make([]LogEntry, capacity),
		capacity: capacity,
	}
}

// Push adds a log entry to the ring buffer, evicting the oldest if full.
func (rb *RingBuffer) Push(entry LogEntry) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.entries[rb.head] = entry
	rb.head = (rb.head + 1) % rb.capacity
	if rb.count < rb.capacity {
		rb.count++
	}
	rb.written++
}

// Seq is the sequence number of the newest entry (0 = none yet). It keeps
// growing after the buffer is full, unlike Len.
func (rb *RingBuffer) Seq() uint64 {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.written
}

// Since returns, oldest first, the entries pushed after sequence seq that
// are still in the buffer, and the newest sequence. Entries evicted before
// the caller came back are gone; the rest are never repeated.
func (rb *RingBuffer) Since(seq uint64) ([]LogEntry, uint64) {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	if seq >= rb.written {
		return nil, rb.written
	}
	// n = min(written-seq, count), computed without a narrowing conversion:
	// count bounds it, and a gap too large for an int is larger than count.
	n := rb.count
	if gap := rb.written - seq; gap <= math.MaxInt && int(gap) < n {
		n = int(gap)
	}
	out := make([]LogEntry, 0, n)
	for i := 0; i < n; i++ {
		// head-1 is the newest; walk back n-1 entries, then forward.
		idx := (rb.head - n + i + rb.capacity) % rb.capacity
		out = append(out, rb.entries[idx])
	}
	return out, rb.written
}

// Query returns log entries matching the given query filters.
// Results are returned in chronological order (oldest first).
func (rb *RingBuffer) Query(q LogQuery) []LogEntry {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	limit := q.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}

	if q.Newest {
		return rb.queryNewest(q, limit)
	}

	// Collect matching entries in chronological order
	var matches []LogEntry
	skipped := 0

	// Start from oldest entry
	start := 0
	if rb.count == rb.capacity {
		start = rb.head // oldest entry is at head when buffer is full
	}

	for i := 0; i < rb.count; i++ {
		idx := (start + i) % rb.capacity
		entry := rb.entries[idx]

		if !rb.matchesQuery(entry, q) {
			continue
		}

		// Handle offset
		if skipped < q.Offset {
			skipped++
			continue
		}

		matches = append(matches, entry)
		if len(matches) >= limit {
			break
		}
	}

	if matches == nil {
		return []LogEntry{} // return empty slice, not nil
	}
	return matches
}

// queryNewest is Query with Limit and Offset counted back from the newest
// entry; the result is still oldest first. The caller holds rb.mu.
func (rb *RingBuffer) queryNewest(q LogQuery, limit int) []LogEntry {
	matches := []LogEntry{}
	skipped := 0
	for i := 0; i < rb.count && len(matches) < limit; i++ {
		idx := (rb.head - 1 - i + 2*rb.capacity) % rb.capacity
		entry := rb.entries[idx]
		if !rb.matchesQuery(entry, q) {
			continue
		}
		if skipped < q.Offset {
			skipped++
			continue
		}
		matches = append(matches, entry)
	}
	for i, j := 0, len(matches)-1; i < j; i, j = i+1, j-1 {
		matches[i], matches[j] = matches[j], matches[i]
	}
	return matches
}

// Len returns the number of entries in the buffer.
func (rb *RingBuffer) Len() int {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.count
}

// Cap returns the buffer capacity.
func (rb *RingBuffer) Cap() int {
	return rb.capacity
}

// matchesQuery checks if an entry matches all query filters.
// Must be called with read lock held.
func (rb *RingBuffer) matchesQuery(entry LogEntry, q LogQuery) bool {
	if q.Service != "" && entry.Service != q.Service {
		return false
	}
	if q.Level != "" && !asciiEqualFold(entry.Level, q.Level) {
		return false
	}
	if !q.From.IsZero() && entry.Timestamp.Before(q.From) {
		return false
	}
	if !q.To.IsZero() && entry.Timestamp.After(q.To) {
		return false
	}
	if q.Search != "" && !rb.matchesSearch(entry, q.Search) {
		return false
	}
	return true
}

// matchesSearch performs case-insensitive full-text search on message and fields.
func (rb *RingBuffer) matchesSearch(entry LogEntry, search string) bool {
	search = strings.ToLower(search)

	if strings.Contains(strings.ToLower(entry.Message), search) {
		return true
	}

	for _, v := range entry.Fields {
		if s, ok := v.(string); ok {
			if strings.Contains(strings.ToLower(s), search) {
				return true
			}
		}
	}

	return false
}

// asciiEqualFold compares two strings case-insensitively over ASCII only.
//
// strings.EqualFold decodes invalid UTF-8 to utf8.RuneError, and RuneError
// equals RuneError — so it reports *any* two invalid bytes as equal:
// EqualFold("\xe1", "\xce") is true. A level filter carrying one invalid byte
// therefore matched entries carrying a different one. Found by
// tomb/lich/harnesses LICH-008 within seconds of pointing it at the real
// buffer.
//
// Log levels are a fixed ASCII vocabulary (see levelString), so folding ASCII
// and comparing the remaining bytes exactly is both correct for real input
// and immune to the RuneError collision.
func asciiEqualFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if asciiLower(a[i]) != asciiLower(b[i]) {
			return false
		}
	}
	return true
}

func asciiLower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}
