// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package logger

import (
	"sync"
	"time"

	"unheaded/pkg/audit"
)

// Buffer provides event buffering with automatic flushing.
type Buffer struct {
	capacity      int
	flushInterval time.Duration
	flushFn       func([]*audit.AuditEvent) error

	mu       sync.Mutex
	events   []*audit.AuditEvent
	timer    *time.Timer
	stopCh   chan struct{}
	doneCh   chan struct{}
	flushing bool
}

// NewBuffer creates a new Buffer.
func NewBuffer(capacity int, flushInterval time.Duration, flushFn func([]*audit.AuditEvent) error) *Buffer {
	if capacity <= 0 {
		capacity = 100
	}
	if flushInterval <= 0 {
		flushInterval = 5 * time.Second
	}

	return &Buffer{
		capacity:      capacity,
		flushInterval: flushInterval,
		flushFn:       flushFn,
		events:        make([]*audit.AuditEvent, 0, capacity),
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
	}
}

// Start begins the background flush timer.
func (b *Buffer) Start() {
	b.mu.Lock()
	b.timer = time.NewTimer(b.flushInterval)
	b.mu.Unlock()

	go b.flushLoop()
}

// Stop gracefully shuts down the buffer and flushes remaining events.
func (b *Buffer) Stop() error {
	close(b.stopCh)
	<-b.doneCh
	return b.Flush()
}

// Add adds an event to the buffer.
func (b *Buffer) Add(event *audit.AuditEvent) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.events = append(b.events, event)

	// Flush if capacity reached
	if len(b.events) >= b.capacity {
		return b.flushLocked()
	}

	return nil
}

// Flush manually flushes the buffer.
func (b *Buffer) Flush() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.flushLocked()
}

// flushLocked flushes the buffer (must be called with lock held).
func (b *Buffer) flushLocked() error {
	if len(b.events) == 0 || b.flushing {
		return nil
	}

	b.flushing = true
	events := b.events
	b.events = make([]*audit.AuditEvent, 0, b.capacity)
	b.flushing = false

	// Reset timer
	if b.timer != nil {
		b.timer.Reset(b.flushInterval)
	}

	if b.flushFn != nil {
		return b.flushFn(events)
	}
	return nil
}

// flushLoop runs the periodic flush.
func (b *Buffer) flushLoop() {
	defer close(b.doneCh)

	for {
		select {
		case <-b.stopCh:
			b.mu.Lock()
			if b.timer != nil {
				b.timer.Stop()
			}
			b.mu.Unlock()
			return
		case <-b.timer.C:
			b.mu.Lock()
			_ = b.flushLocked()
			b.mu.Unlock()
		}
	}
}

// Size returns the current buffer size.
func (b *Buffer) Size() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.events)
}

// Capacity returns the buffer capacity.
func (b *Buffer) Capacity() int {
	return b.capacity
}

// BufferStats contains buffer statistics.
type BufferStats struct {
	Size          int           `json:"size"`
	Capacity      int           `json:"capacity"`
	FlushInterval time.Duration `json:"flush_interval"`
}

// Stats returns current buffer statistics.
func (b *Buffer) Stats() *BufferStats {
	b.mu.Lock()
	defer b.mu.Unlock()
	return &BufferStats{
		Size:          len(b.events),
		Capacity:      b.capacity,
		FlushInterval: b.flushInterval,
	}
}
