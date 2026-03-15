// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package yaldabaoth

import (
	"encoding/json"
	"sync"
	"time"

	"unheaded/pkg/logger"
)

// ── AuditEntry ───────────────────────────────────────────────────────────

// AuditEntry records a single auditable action performed by the chaos
// controller, including who did it, what they did, and the outcome.
type AuditEntry struct {
	Timestamp time.Time `json:"timestamp"`
	UserID    string    `json:"user_id,omitempty"`
	Action    string    `json:"action"`
	RuleID    string    `json:"rule_id,omitempty"`
	RuleSpec  string    `json:"rule_spec,omitempty"`
	Result    string    `json:"result"`
}

// ── AuditFilter ──────────────────────────────────────────────────────────

// AuditFilter specifies criteria for querying audit entries.
type AuditFilter struct {
	StartTime *time.Time `json:"start_time,omitempty"`
	EndTime   *time.Time `json:"end_time,omitempty"`
	UserID    string     `json:"user_id,omitempty"`
	Action    string     `json:"action,omitempty"`
}

// ── AuditTrail ───────────────────────────────────────────────────────────

// AuditTrail maintains an ordered, bounded log of chaos engineering
// actions for compliance and forensic analysis.
type AuditTrail struct {
	log        *logger.Logger
	mu         sync.RWMutex
	entries    []AuditEntry
	maxEntries int
}

// NewAuditTrail creates a new AuditTrail with the given maximum capacity.
func NewAuditTrail(log *logger.Logger, maxEntries int) *AuditTrail {
	if maxEntries <= 0 {
		maxEntries = 10000
	}
	return &AuditTrail{
		log:        log,
		entries:    make([]AuditEntry, 0, 256),
		maxEntries: maxEntries,
	}
}

// Record appends an entry to the audit trail, trimming the oldest entries
// if the capacity limit is reached.
func (at *AuditTrail) Record(entry AuditEntry) {
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	at.mu.Lock()
	at.entries = append(at.entries, entry)

	// Trim when over capacity — drop the oldest quarter.
	if len(at.entries) > at.maxEntries {
		trim := at.maxEntries / 4
		if trim < 1 {
			trim = 1
		}
		at.entries = at.entries[trim:]
	}
	at.mu.Unlock()

	at.log.Info().
		Str("action", entry.Action).
		Str("user_id", entry.UserID).
		Str("rule_id", entry.RuleID).
		Str("result", entry.Result).
		Msg("audit entry recorded")
}

// Query returns audit entries matching the given filter. All filter
// fields are optional; an empty filter returns all entries.
func (at *AuditTrail) Query(filter AuditFilter) []AuditEntry {
	at.mu.RLock()
	defer at.mu.RUnlock()

	results := make([]AuditEntry, 0, len(at.entries))
	for _, entry := range at.entries {
		if !at.matchesFilter(entry, filter) {
			continue
		}
		results = append(results, entry)
	}
	return results
}

// matchesFilter returns true if the entry matches all non-zero filter fields.
func (at *AuditTrail) matchesFilter(entry AuditEntry, filter AuditFilter) bool {
	if filter.StartTime != nil && entry.Timestamp.Before(*filter.StartTime) {
		return false
	}
	if filter.EndTime != nil && entry.Timestamp.After(*filter.EndTime) {
		return false
	}
	if filter.UserID != "" && entry.UserID != filter.UserID {
		return false
	}
	if filter.Action != "" && entry.Action != filter.Action {
		return false
	}
	return true
}

// Export serializes the entire audit trail as JSON for compliance export.
func (at *AuditTrail) Export() []byte {
	at.mu.RLock()
	defer at.mu.RUnlock()

	data, err := json.Marshal(map[string]interface{}{
		"exported_at": time.Now().UTC().Format(time.RFC3339),
		"count":       len(at.entries),
		"entries":     at.entries,
	})
	if err != nil {
		at.log.Error().Err(err).Msg("failed to export audit trail")
		return []byte(`{"error":"export failed"}`)
	}
	return data
}

// Len returns the current number of audit entries.
func (at *AuditTrail) Len() int {
	at.mu.RLock()
	defer at.mu.RUnlock()
	return len(at.entries)
}

// Clear removes all audit entries. Use with caution — this is intended
// for testing or explicit compliance-approved rotation only.
func (at *AuditTrail) Clear() {
	at.mu.Lock()
	at.entries = at.entries[:0]
	at.mu.Unlock()

	at.log.Warn().Msg("audit trail cleared")
}
