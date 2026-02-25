// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package storage provides audit event storage backends.
package storage

import (
	"context"
	"fmt"
	"sync"
	"time"

	"unheaded/pkg/audit"
)

// MemoryStorage is an in-memory storage backend for testing and development.
type MemoryStorage struct {
	mu       sync.RWMutex
	events   []*audit.AuditEvent
	byID     map[string]*audit.AuditEvent
	lastHash string
}

// NewMemoryStorage creates a new in-memory storage.
func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		events: make([]*audit.AuditEvent, 0),
		byID:   make(map[string]*audit.AuditEvent),
	}
}

// Store persists an audit event.
func (s *MemoryStorage) Store(ctx context.Context, event *audit.AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.byID[event.ID]; exists {
		return fmt.Errorf("event already exists: %s", event.ID)
	}

	s.events = append(s.events, event)
	s.byID[event.ID] = event
	s.lastHash = event.Hash

	return nil
}

// StoreBatch persists multiple events.
func (s *MemoryStorage) StoreBatch(ctx context.Context, events []*audit.AuditEvent) error {
	for _, event := range events {
		if err := s.Store(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

// Get retrieves a single event by ID.
func (s *MemoryStorage) Get(ctx context.Context, id string) (*audit.AuditEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	event, ok := s.byID[id]
	if !ok {
		return nil, fmt.Errorf("event not found: %s", id)
	}

	return event, nil
}

// Query retrieves events matching the query.
func (s *MemoryStorage) Query(ctx context.Context, query *audit.AuditQuery) ([]*audit.AuditEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	results := make([]*audit.AuditEvent, 0)

	for _, event := range s.events {
		if s.matchesQuery(event, query) {
			results = append(results, event)
		}
	}

	// Apply sorting
	if query.OrderBy != "" {
		results = s.sortEvents(results, query.OrderBy, query.Descending)
	}

	// Apply pagination
	if query.Offset > 0 && query.Offset < len(results) {
		results = results[query.Offset:]
	} else if query.Offset >= len(results) {
		return []*audit.AuditEvent{}, nil
	}

	if query.Limit > 0 && query.Limit < len(results) {
		results = results[:query.Limit]
	}

	return results, nil
}

// GetLastHash returns the hash of the most recent event.
func (s *MemoryStorage) GetLastHash(ctx context.Context) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastHash, nil
}

// Count returns the number of events matching the query.
func (s *MemoryStorage) Count(ctx context.Context, query *audit.AuditQuery) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := int64(0)
	for _, event := range s.events {
		if s.matchesQuery(event, query) {
			count++
		}
	}

	return count, nil
}

// Close releases resources.
func (s *MemoryStorage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = nil
	s.byID = nil
	return nil
}

// matchesQuery checks if an event matches the query criteria.
func (s *MemoryStorage) matchesQuery(event *audit.AuditEvent, query *audit.AuditQuery) bool {
	// Time range filter
	if !query.StartTime.IsZero() && event.Timestamp.Before(query.StartTime) {
		return false
	}
	if !query.EndTime.IsZero() && event.Timestamp.After(query.EndTime) {
		return false
	}

	// Type filter
	if len(query.Types) > 0 {
		found := false
		for _, t := range query.Types {
			if event.Type == t {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Severity filter
	if len(query.Severities) > 0 {
		found := false
		for _, s := range query.Severities {
			if event.Severity == s {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Actor filter
	if len(query.ActorIDs) > 0 && event.Actor != nil {
		found := false
		for _, id := range query.ActorIDs {
			if event.Actor.ID == id {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Action filter
	if len(query.Actions) > 0 {
		found := false
		for _, a := range query.Actions {
			if event.Action == a {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Outcome filter
	if len(query.Outcomes) > 0 {
		found := false
		for _, o := range query.Outcomes {
			if event.Outcome == o {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Resource type filter
	if len(query.ResourceTypes) > 0 && event.Resource != nil {
		found := false
		for _, rt := range query.ResourceTypes {
			if event.Resource.Type == rt {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// sortEvents sorts events by the specified field.
func (s *MemoryStorage) sortEvents(events []*audit.AuditEvent, field string, descending bool) []*audit.AuditEvent {
	// Simple bubble sort for now - could use sort.Slice for production
	n := len(events)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			shouldSwap := false
			switch field {
			case "timestamp":
				if descending {
					shouldSwap = events[j].Timestamp.Before(events[j+1].Timestamp)
				} else {
					shouldSwap = events[j].Timestamp.After(events[j+1].Timestamp)
				}
			case "severity":
				if descending {
					shouldSwap = severityOrder(events[j].Severity) < severityOrder(events[j+1].Severity)
				} else {
					shouldSwap = severityOrder(events[j].Severity) > severityOrder(events[j+1].Severity)
				}
			}
			if shouldSwap {
				events[j], events[j+1] = events[j+1], events[j]
			}
		}
	}
	return events
}

func severityOrder(s audit.Severity) int {
	switch s {
	case audit.SeverityInfo:
		return 1
	case audit.SeverityWarning:
		return 2
	case audit.SeverityError:
		return 3
	case audit.SeverityCritical:
		return 4
	default:
		return 0
	}
}

// RetentionCleaner handles automatic cleanup of old events.
type RetentionCleaner struct {
	storage       audit.Storage
	retentionDays int
	interval      time.Duration
	stopCh        chan struct{}
	doneCh        chan struct{}
}

// NewRetentionCleaner creates a new retention cleaner.
func NewRetentionCleaner(storage audit.Storage, retentionDays int, interval time.Duration) *RetentionCleaner {
	return &RetentionCleaner{
		storage:       storage,
		retentionDays: retentionDays,
		interval:      interval,
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
	}
}

// Start begins the cleanup loop.
func (c *RetentionCleaner) Start() {
	go c.cleanupLoop()
}

// Stop gracefully shuts down the cleaner.
func (c *RetentionCleaner) Stop() {
	close(c.stopCh)
	<-c.doneCh
}

func (c *RetentionCleaner) cleanupLoop() {
	defer close(c.doneCh)
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			// Cleanup logic would go here
			// For memory storage, we'd need to add a Delete method
		}
	}
}
