// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package lifecycle

import (
	"fmt"
	"sync"
	"time"
)

// AuditEventType classifies PQC key lifecycle events.
type AuditEventType string

const (
	AuditKeyGenerated AuditEventType = "KEY_GENERATED"
	AuditKeyRotated   AuditEventType = "KEY_ROTATED"
	AuditKeyRevoked   AuditEventType = "KEY_REVOKED"
	AuditKeyExpired   AuditEventType = "KEY_EXPIRED"
	AuditGraceStart   AuditEventType = "GRACE_PERIOD_START"
	AuditGraceEnd     AuditEventType = "GRACE_PERIOD_END"
	AuditSigLoaded    AuditEventType = "SIG_LOADED"
	AuditSigInvalid   AuditEventType = "SIG_INVALIDATED"
)

// AuditEvent records a PQC key lifecycle event for the Anamnesis audit trail.
type AuditEvent struct {
	ID        string                 `json:"id"`
	Type      AuditEventType         `json:"type"`
	KeyID     string                 `json:"key_id,omitempty"`
	KeyRef    uint16                 `json:"key_ref,omitempty"`
	ServiceID uint16                 `json:"service_id,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

// AuditTrail provides an append-only log of PQC key lifecycle events.
type AuditTrail struct {
	mu     sync.RWMutex
	events []*AuditEvent
	maxLen int
	nextID int64
}

// NewAuditTrail creates a new audit trail with the given capacity.
func NewAuditTrail(maxLen int) *AuditTrail {
	if maxLen <= 0 {
		maxLen = 10000
	}
	return &AuditTrail{
		events: make([]*AuditEvent, 0, maxLen),
		maxLen: maxLen,
	}
}

// Record appends an audit event.
func (a *AuditTrail) Record(event *AuditEvent) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.nextID++
	event.ID = fmt.Sprintf("audit-%d", a.nextID)
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Ring buffer: overwrite oldest when at capacity
	if len(a.events) >= a.maxLen {
		a.events = a.events[1:]
	}
	a.events = append(a.events, event)
}

// Events returns all events (snapshot).
func (a *AuditTrail) Events() []*AuditEvent {
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := make([]*AuditEvent, len(a.events))
	copy(result, a.events)
	return result
}

// EventsByType filters events by type.
func (a *AuditTrail) EventsByType(eventType AuditEventType) []*AuditEvent {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var result []*AuditEvent
	for _, e := range a.events {
		if e.Type == eventType {
			result = append(result, e)
		}
	}
	return result
}

// EventsByKey filters events by key ID.
func (a *AuditTrail) EventsByKey(keyID string) []*AuditEvent {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var result []*AuditEvent
	for _, e := range a.events {
		if e.KeyID == keyID {
			result = append(result, e)
		}
	}
	return result
}

// Count returns total event count.
func (a *AuditTrail) Count() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.events)
}
