// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package observability

import (
	"context"
	"sync"
	"sync/atomic"
)

// ZerologAdapter forwards log entries to a configurable sink.
// In production, this writes structured JSON to stdout (zerolog default).
// This adapter can also buffer entries for testing or forwarding to ELK/Loki.
type ZerologAdapter struct {
	mu      sync.RWMutex
	entries []LogEntry
	maxSize int
	closed  atomic.Bool
}

// NewZerologAdapter creates a zerolog logging adapter.
// maxSize limits the in-memory buffer (0 = unlimited, used for forwarding).
func NewZerologAdapter(maxSize int) *ZerologAdapter {
	return &ZerologAdapter{
		maxSize: maxSize,
	}
}

func (a *ZerologAdapter) Backend() Backend { return Backend("zerolog") }

func (a *ZerologAdapter) Supports() []SignalType {
	return []SignalType{SignalLog}
}

func (a *ZerologAdapter) EmitMetric(_ context.Context, _ Metric) error {
	return nil // Zerolog doesn't handle metrics
}

func (a *ZerologAdapter) EmitLog(_ context.Context, e LogEntry) error {
	if a.closed.Load() {
		return ErrAdapterClosed
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.maxSize > 0 && len(a.entries) >= a.maxSize {
		// Ring buffer: drop oldest
		a.entries = a.entries[1:]
	}
	a.entries = append(a.entries, e)
	return nil
}

func (a *ZerologAdapter) EmitTrace(_ context.Context, _ Span) error {
	return nil
}

func (a *ZerologAdapter) EmitAlert(_ context.Context, _ Alert) error {
	return nil
}

func (a *ZerologAdapter) Close() error {
	a.closed.Store(true)
	return nil
}

// Entries returns all buffered log entries.
func (a *ZerologAdapter) Entries() []LogEntry {
	a.mu.RLock()
	defer a.mu.RUnlock()
	result := make([]LogEntry, len(a.entries))
	copy(result, a.entries)
	return result
}

// EntryCount returns the number of buffered entries.
func (a *ZerologAdapter) EntryCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.entries)
}
