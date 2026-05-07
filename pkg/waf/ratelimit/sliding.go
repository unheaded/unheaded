// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package ratelimit provides sliding window rate limiting
package ratelimit

import (
	"sync"
	"time"
)

// SlidingWindow implements a sliding window rate limiter
type SlidingWindow struct {
	rate       int
	window     time.Duration
	precision  int
	buckets    map[string]*windowData
	mu         sync.RWMutex
	cleanupInt time.Duration
	stopCh     chan struct{}
}

type windowData struct {
	counts   []int
	total    int
	lastIdx  int
	lastTime time.Time
	mu       sync.Mutex
}

// NewSlidingWindow creates a new sliding window rate limiter
func NewSlidingWindow(rate int, window time.Duration) *SlidingWindow {
	precision := 10 // Number of sub-windows
	sw := &SlidingWindow{
		rate:       rate,
		window:     window,
		precision:  precision,
		buckets:    make(map[string]*windowData),
		cleanupInt: window * 2,
		stopCh:     make(chan struct{}),
	}

	go sw.cleanup()
	return sw
}

// NewSlidingWindowWithPrecision creates a sliding window with custom precision
func NewSlidingWindowWithPrecision(rate int, window time.Duration, precision int) *SlidingWindow {
	sw := &SlidingWindow{
		rate:       rate,
		window:     window,
		precision:  precision,
		buckets:    make(map[string]*windowData),
		cleanupInt: window * 2,
		stopCh:     make(chan struct{}),
	}

	go sw.cleanup()
	return sw
}

// Allow checks if a request should be allowed
func (sw *SlidingWindow) Allow(key string) bool {
	return sw.AllowN(key, 1)
}

// AllowN checks if N requests should be allowed
func (sw *SlidingWindow) AllowN(key string, n int) bool {
	now := time.Now()
	data := sw.getData(key)

	data.mu.Lock()
	defer data.mu.Unlock()

	sw.advanceWindow(data, now)

	// Check if allowing N more would exceed rate
	if data.total+n > sw.rate {
		return false
	}

	// Record the request(s)
	data.counts[data.lastIdx] += n
	data.total += n

	return true
}

// getData gets or creates window data for a key
func (sw *SlidingWindow) getData(key string) *windowData {
	sw.mu.RLock()
	data, ok := sw.buckets[key]
	sw.mu.RUnlock()

	if ok {
		return data
	}

	sw.mu.Lock()
	defer sw.mu.Unlock()

	// Double-check
	if data, ok = sw.buckets[key]; ok {
		return data
	}

	data = &windowData{
		counts:   make([]int, sw.precision),
		lastTime: time.Now(),
	}
	sw.buckets[key] = data
	return data
}

// advanceWindow advances the sliding window
func (sw *SlidingWindow) advanceWindow(data *windowData, now time.Time) {
	bucketDuration := sw.window / time.Duration(sw.precision)
	elapsed := now.Sub(data.lastTime)
	bucketsToAdvance := int(elapsed / bucketDuration)

	if bucketsToAdvance <= 0 {
		return
	}

	if bucketsToAdvance >= sw.precision {
		// Reset everything
		for i := range data.counts {
			data.counts[i] = 0
		}
		data.total = 0
		data.lastIdx = 0
	} else {
		// Advance and clear old buckets
		for i := 0; i < bucketsToAdvance; i++ {
			data.lastIdx = (data.lastIdx + 1) % sw.precision
			data.total -= data.counts[data.lastIdx]
			data.counts[data.lastIdx] = 0
		}
	}

	data.lastTime = now
}

// Reset resets the rate limiter for a key
func (sw *SlidingWindow) Reset(key string) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	delete(sw.buckets, key)
}

// GetRemaining returns the remaining requests allowed
func (sw *SlidingWindow) GetRemaining(key string) int {
	sw.mu.RLock()
	data, ok := sw.buckets[key]
	sw.mu.RUnlock()

	if !ok {
		return sw.rate
	}

	data.mu.Lock()
	defer data.mu.Unlock()

	sw.advanceWindow(data, time.Now())
	remaining := sw.rate - data.total
	if remaining < 0 {
		return 0
	}
	return remaining
}

// Close stops the cleanup goroutine
func (sw *SlidingWindow) Close() error {
	close(sw.stopCh)
	return nil
}

// cleanup periodically removes stale entries
func (sw *SlidingWindow) cleanup() {
	ticker := time.NewTicker(sw.cleanupInt)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sw.doCleanup()
		case <-sw.stopCh:
			return
		}
	}
}

// doCleanup removes expired entries
func (sw *SlidingWindow) doCleanup() {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	now := time.Now()
	for key, data := range sw.buckets {
		data.mu.Lock()
		if now.Sub(data.lastTime) > sw.window*2 {
			delete(sw.buckets, key)
		}
		data.mu.Unlock()
	}
}

// Stats returns statistics about the rate limiter
func (sw *SlidingWindow) Stats() map[string]int {
	sw.mu.RLock()
	defer sw.mu.RUnlock()

	stats := make(map[string]int)
	stats["keys"] = len(sw.buckets)
	stats["rate"] = sw.rate
	stats["precision"] = sw.precision

	return stats
}
