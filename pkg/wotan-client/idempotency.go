// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package wotanClient — idempotency cache for duplicate message detection.
//
// IdempotencyCache tracks processed message IDs so that duplicate deliveries
// (from retries or network hiccups) are detected and return the cached result
// without re-processing. Entries expire after a configurable TTL (default 24h).
package wotanClient

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Default idempotency settings.
const (
	DefaultIdempotencyTTL      = 24 * time.Hour
	idempotencyCleanupInterval = 10 * time.Minute
)

// Prometheus metrics for idempotency.
var (
	wotanIdempotencyHits = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "wotan_idempotency_hits_total",
			Help: "Total number of duplicate messages detected and rejected",
		},
	)
	wotanIdempotencyEntries = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "wotan_idempotency_entries",
			Help: "Current number of entries in the idempotency cache",
		},
	)
)

// ProcessResult is the cached outcome of processing a message.
type ProcessResult struct {
	// Err is nil on success, or the processing error.
	Err error
	// Processed is the wall-clock time the message was first handled.
	Processed time.Time
}

// idempotencyEntry stores a cached result with an expiry time.
type idempotencyEntry struct {
	Result  ProcessResult
	Expires time.Time
}

// IdempotencyCache provides at-most-once processing for messages identified
// by a unique ID. Duplicate IDs within the TTL window return the cached result.
type IdempotencyCache struct {
	mu      sync.Mutex
	entries map[string]idempotencyEntry
	ttl     time.Duration
	done    chan struct{}
}

// NewIdempotencyCache creates a cache with the given TTL.
// Call Stop() when done to halt the background cleanup goroutine.
func NewIdempotencyCache(ttl time.Duration) *IdempotencyCache {
	if ttl <= 0 {
		ttl = DefaultIdempotencyTTL
	}
	ic := &IdempotencyCache{
		entries: make(map[string]idempotencyEntry),
		ttl:     ttl,
		done:    make(chan struct{}),
	}
	go ic.cleanupLoop()
	return ic
}

// Check returns the cached result for a message ID if it exists and hasn't
// expired. Returns (result, true) on hit, (zero, false) on miss.
func (ic *IdempotencyCache) Check(messageID string) (ProcessResult, bool) {
	ic.mu.Lock()
	defer ic.mu.Unlock()

	entry, ok := ic.entries[messageID]
	if !ok {
		return ProcessResult{}, false
	}
	if time.Now().After(entry.Expires) {
		delete(ic.entries, messageID)
		wotanIdempotencyEntries.Dec()
		return ProcessResult{}, false
	}

	wotanIdempotencyHits.Inc()
	return entry.Result, true
}

// Record stores the processing result for a message ID.
// Subsequent Check calls with the same ID will return this result until TTL expires.
func (ic *IdempotencyCache) Record(messageID string, result ProcessResult) {
	ic.mu.Lock()
	defer ic.mu.Unlock()

	if result.Processed.IsZero() {
		result.Processed = time.Now()
	}

	_, existed := ic.entries[messageID]
	ic.entries[messageID] = idempotencyEntry{
		Result:  result,
		Expires: time.Now().Add(ic.ttl),
	}
	if !existed {
		wotanIdempotencyEntries.Inc()
	}
}

// Len returns the number of entries in the cache (including expired but not yet cleaned).
func (ic *IdempotencyCache) Len() int {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	return len(ic.entries)
}

// Stop halts the background cleanup goroutine.
func (ic *IdempotencyCache) Stop() {
	select {
	case <-ic.done:
	default:
		close(ic.done)
	}
}

// cleanupLoop periodically removes expired entries.
func (ic *IdempotencyCache) cleanupLoop() {
	ticker := time.NewTicker(idempotencyCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ic.done:
			return
		case <-ticker.C:
			ic.cleanup()
		}
	}
}

// cleanup removes all expired entries.
func (ic *IdempotencyCache) cleanup() {
	ic.mu.Lock()
	defer ic.mu.Unlock()

	now := time.Now()
	removed := 0
	for id, entry := range ic.entries {
		if now.After(entry.Expires) {
			delete(ic.entries, id)
			removed++
		}
	}
	wotanIdempotencyEntries.Set(float64(len(ic.entries)))
}
