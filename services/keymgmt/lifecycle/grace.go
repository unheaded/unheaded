// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package lifecycle manages PQC key lifecycle events: grace periods,
// revocation cascades, and audit trail generation.
package lifecycle

import (
	"sync"
	"time"
)

// GraceEntry tracks a key that is in rotation grace period.
type GraceEntry struct {
	OldKeyID  string
	NewKeyID  string
	OldKeyRef uint16
	NewKeyRef uint16
	StartedAt time.Time
	ExpiresAt time.Time
	Completed bool
}

// GraceManager manages grace periods during key rotation.
// During a grace period, both old and new keys are valid.
type GraceManager struct {
	mu      sync.RWMutex
	entries map[string]*GraceEntry // oldKeyID → entry
	period  time.Duration
}

// NewGraceManager creates a new grace manager.
func NewGraceManager(period time.Duration) *GraceManager {
	return &GraceManager{
		entries: make(map[string]*GraceEntry),
		period:  period,
	}
}

// StartGrace begins a grace period for a key rotation.
func (g *GraceManager) StartGrace(oldKeyID, newKeyID string, oldKeyRef, newKeyRef uint16) *GraceEntry {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now()
	entry := &GraceEntry{
		OldKeyID:  oldKeyID,
		NewKeyID:  newKeyID,
		OldKeyRef: oldKeyRef,
		NewKeyRef: newKeyRef,
		StartedAt: now,
		ExpiresAt: now.Add(g.period),
	}
	g.entries[oldKeyID] = entry
	return entry
}

// IsInGrace checks if a key ID is currently in a grace period.
func (g *GraceManager) IsInGrace(keyID string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()

	entry, ok := g.entries[keyID]
	if !ok {
		return false
	}
	return !entry.Completed && time.Now().Before(entry.ExpiresAt)
}

// CompleteGrace marks a grace period as completed.
func (g *GraceManager) CompleteGrace(oldKeyID string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if entry, ok := g.entries[oldKeyID]; ok {
		entry.Completed = true
	}
}

// ExpiredEntries returns grace entries that have expired but not been completed.
func (g *GraceManager) ExpiredEntries() []*GraceEntry {
	g.mu.RLock()
	defer g.mu.RUnlock()

	now := time.Now()
	var expired []*GraceEntry
	for _, entry := range g.entries {
		if !entry.Completed && now.After(entry.ExpiresAt) {
			expired = append(expired, entry)
		}
	}
	return expired
}

// ActiveCount returns the number of active grace periods.
func (g *GraceManager) ActiveCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()

	now := time.Now()
	count := 0
	for _, entry := range g.entries {
		if !entry.Completed && now.Before(entry.ExpiresAt) {
			count++
		}
	}
	return count
}

// Cleanup removes completed entries older than the given duration.
func (g *GraceManager) Cleanup(maxAge time.Duration) int {
	g.mu.Lock()
	defer g.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	removed := 0
	for id, entry := range g.entries {
		if entry.Completed && entry.ExpiresAt.Before(cutoff) {
			delete(g.entries, id)
			removed++
		}
	}
	return removed
}
