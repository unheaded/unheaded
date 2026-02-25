package logagg

import (
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
	head     int // next write position
	count    int // total entries stored
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
	if q.Level != "" && !strings.EqualFold(entry.Level, q.Level) {
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
