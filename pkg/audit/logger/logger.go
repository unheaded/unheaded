// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package logger provides the core audit logging implementation.
package logger

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"

	"unheaded/pkg/audit"
)

// Logger implements the audit.AuditLogger interface with async capabilities.
type Logger struct {
	config   *audit.Config
	storage  audit.Storage
	registry *audit.Registry

	buffer    *Buffer
	async     *AsyncProcessor
	lastHash  string
	hashMu    sync.Mutex

	closed   bool
	closedMu sync.RWMutex
}

// New creates a new Logger with the given configuration.
func New(config *audit.Config, storage audit.Storage, registry *audit.Registry) (*Logger, error) {
	if config == nil {
		config = audit.DefaultConfig()
	}
	if storage == nil {
		return nil, fmt.Errorf("storage is required")
	}
	if registry == nil {
		registry = audit.DefaultRegistry
	}

	l := &Logger{
		config:   config,
		storage:  storage,
		registry: registry,
	}

	// Initialize last hash for chain integrity
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	lastHash, err := storage.GetLastHash(ctx)
	if err != nil {
		// If no events exist yet, start with empty hash
		lastHash = ""
	}
	l.lastHash = lastHash

	// Create buffer and async processor
	l.buffer = NewBuffer(config.BufferSize, config.FlushInterval, l.flushBuffer)
	l.async = NewAsyncProcessor(config.AsyncWorkers, l.processEvent)

	// Start background workers
	l.buffer.Start()
	l.async.Start()

	return l, nil
}

// Log records a single audit event.
func (l *Logger) Log(ctx context.Context, event *audit.AuditEvent) error {
	l.closedMu.RLock()
	if l.closed {
		l.closedMu.RUnlock()
		return fmt.Errorf("logger is closed")
	}
	l.closedMu.RUnlock()

	// Generate ID if not provided
	if event.ID == "" {
		event.ID = uuid.New().String()
	}

	// Set timestamp if not provided
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	// Validate the event
	if err := event.Validate(); err != nil {
		return fmt.Errorf("invalid event: %w", err)
	}

	// Compute hash chain if enabled
	if l.config.EnableTamperDetection {
		l.hashMu.Lock()
		event.PreviousHash = l.lastHash
		event.Hash = event.ComputeHash()
		l.lastHash = event.Hash
		l.hashMu.Unlock()
	}

	// Add to buffer for async processing
	return l.buffer.Add(event)
}

// LogBatch records multiple audit events.
func (l *Logger) LogBatch(ctx context.Context, events []*audit.AuditEvent) error {
	for _, event := range events {
		if err := l.Log(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

// Query retrieves audit events matching the query.
func (l *Logger) Query(ctx context.Context, query *audit.AuditQuery) ([]*audit.AuditEvent, error) {
	l.closedMu.RLock()
	if l.closed {
		l.closedMu.RUnlock()
		return nil, fmt.Errorf("logger is closed")
	}
	l.closedMu.RUnlock()

	// Flush buffer first to ensure all events are queryable
	if err := l.buffer.Flush(); err != nil {
		return nil, fmt.Errorf("failed to flush buffer: %w", err)
	}

	return l.storage.Query(ctx, query)
}

// Export exports audit events in the specified format.
func (l *Logger) Export(ctx context.Context, format string, filter *audit.AuditFilter) (io.Reader, error) {
	l.closedMu.RLock()
	if l.closed {
		l.closedMu.RUnlock()
		return nil, fmt.Errorf("logger is closed")
	}
	l.closedMu.RUnlock()

	exporter, ok := l.registry.GetExporter(format)
	if !ok {
		return nil, fmt.Errorf("unknown export format: %s", format)
	}

	// Query events based on filter
	var query *audit.AuditQuery
	if filter != nil && filter.Query != nil {
		query = filter.Query
	} else {
		query = &audit.AuditQuery{}
	}

	events, err := l.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}

	return exporter.Export(ctx, events, filter)
}

// VerifyIntegrity checks the hash chain integrity between two events.
func (l *Logger) VerifyIntegrity(ctx context.Context, startID, endID string) (bool, error) {
	l.closedMu.RLock()
	if l.closed {
		l.closedMu.RUnlock()
		return false, fmt.Errorf("logger is closed")
	}
	l.closedMu.RUnlock()

	if !l.config.EnableTamperDetection {
		return false, fmt.Errorf("tamper detection is not enabled")
	}

	// Query all events in range
	query := &audit.AuditQuery{
		OrderBy:    "timestamp",
		Descending: false,
	}

	events, err := l.storage.Query(ctx, query)
	if err != nil {
		return false, fmt.Errorf("failed to query events: %w", err)
	}

	// Find start and end indices
	startIdx := -1
	endIdx := -1
	for i, e := range events {
		if e.ID == startID {
			startIdx = i
		}
		if e.ID == endID {
			endIdx = i
		}
	}

	if startIdx == -1 || endIdx == -1 {
		return false, fmt.Errorf("start or end event not found")
	}

	if startIdx > endIdx {
		startIdx, endIdx = endIdx, startIdx
	}

	// Verify hash chain
	for i := startIdx; i <= endIdx; i++ {
		event := events[i]

		// Verify individual hash
		if !event.VerifyHash() {
			return false, nil
		}

		// Verify chain link (except for first event)
		if i > startIdx {
			if event.PreviousHash != events[i-1].Hash {
				return false, nil
			}
		}
	}

	return true, nil
}

// Close releases resources and ensures all events are persisted.
func (l *Logger) Close() error {
	l.closedMu.Lock()
	if l.closed {
		l.closedMu.Unlock()
		return nil
	}
	l.closed = true
	l.closedMu.Unlock()

	// Stop buffer and flush remaining events
	if err := l.buffer.Stop(); err != nil {
		return fmt.Errorf("failed to stop buffer: %w", err)
	}

	// Stop async processor
	l.async.Stop()

	// Close storage
	if err := l.storage.Close(); err != nil {
		return fmt.Errorf("failed to close storage: %w", err)
	}

	return nil
}

// flushBuffer is called when the buffer needs to be flushed.
func (l *Logger) flushBuffer(events []*audit.AuditEvent) error {
	for _, event := range events {
		l.async.Submit(event)
	}
	return nil
}

// processEvent processes a single event asynchronously.
func (l *Logger) processEvent(event *audit.AuditEvent) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return l.storage.Store(ctx, event)
}

// Stats returns statistics about the logger.
func (l *Logger) Stats() *LoggerStats {
	return &LoggerStats{
		BufferSize:     l.buffer.Size(),
		BufferCapacity: l.config.BufferSize,
		QueueSize:      l.async.QueueSize(),
		Workers:        l.config.AsyncWorkers,
	}
}

// LoggerStats contains logger statistics.
type LoggerStats struct {
	BufferSize     int `json:"buffer_size"`
	BufferCapacity int `json:"buffer_capacity"`
	QueueSize      int `json:"queue_size"`
	Workers        int `json:"workers"`
}
