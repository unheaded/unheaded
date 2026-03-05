// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package lifecycle

import (
	"fmt"
	"sync"
	"time"
)

// RevocationEntry records a key revocation event.
type RevocationEntry struct {
	KeyID       string
	KeyRef      uint16
	RevokedAt   time.Time
	Reason      string
	CascadedSig []uint32 // SigRefs that were invalidated
}

// RevocationManager handles cascade invalidation when keys are revoked.
type RevocationManager struct {
	mu      sync.RWMutex
	entries map[string]*RevocationEntry // keyID → entry

	// Callback for signature invalidation
	onInvalidateSig func(sigRef uint32) error
}

// NewRevocationManager creates a new revocation manager.
func NewRevocationManager(onInvalidateSig func(sigRef uint32) error) *RevocationManager {
	return &RevocationManager{
		entries:         make(map[string]*RevocationEntry),
		onInvalidateSig: onInvalidateSig,
	}
}

// RevokeKey records a revocation and cascades to dependent SigRefs.
func (r *RevocationManager) RevokeKey(keyID string, keyRef uint16, reason string, dependentSigRefs []uint32) (*RevocationEntry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.entries[keyID]; exists {
		return nil, fmt.Errorf("lifecycle: key %s already revoked", keyID)
	}

	entry := &RevocationEntry{
		KeyID:     keyID,
		KeyRef:    keyRef,
		RevokedAt: time.Now(),
		Reason:    reason,
	}

	// Cascade invalidation to dependent signatures
	for _, sigRef := range dependentSigRefs {
		if r.onInvalidateSig != nil {
			if err := r.onInvalidateSig(sigRef); err != nil {
				// Log but continue — best effort cascade
				continue
			}
		}
		entry.CascadedSig = append(entry.CascadedSig, sigRef)
	}

	r.entries[keyID] = entry
	return entry, nil
}

// IsRevoked checks if a key has been revoked.
func (r *RevocationManager) IsRevoked(keyID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.entries[keyID]
	return ok
}

// GetRevocation returns a revocation entry.
func (r *RevocationManager) GetRevocation(keyID string) (*RevocationEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.entries[keyID]
	return entry, ok
}

// RevokedCount returns the total number of revoked keys.
func (r *RevocationManager) RevokedCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.entries)
}
