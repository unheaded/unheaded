// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package sophia

import (
	"fmt"
	"sync"
)

// SovereignEntry holds a multi-sig entry for SOVEREIGN tier.
type SovereignEntry struct {
	SigRef    uint32
	Slots     [3]SovereignSlot
	Consensus uint8 // 0=unknown, 1=passed, 2=failed, 3=partial
}

// SovereignSlot is one of the 3 signature slots.
type SovereignSlot struct {
	SigRef uint32
	AlgoID uint8
	Status uint8 // 0=pending, 1=valid, 2=invalid, 3=expired
}

// SovereignMap manages multi-sig entries for SOVEREIGN tier.
type SovereignMap struct {
	mu      sync.RWMutex
	entries map[uint32]*SovereignEntry // primary SigRef → entry
}

// NewSovereignMap creates a new sovereign map.
func NewSovereignMap() *SovereignMap {
	return &SovereignMap{
		entries: make(map[uint32]*SovereignEntry),
	}
}

// Store adds or updates a sovereign entry.
func (sm *SovereignMap) Store(entry *SovereignEntry) error {
	if entry == nil {
		return fmt.Errorf("sophia: sovereign_map: nil entry")
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.entries[entry.SigRef] = entry
	return nil
}

// Lookup retrieves a sovereign entry.
func (sm *SovereignMap) Lookup(sigRef uint32) (*SovereignEntry, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	e, ok := sm.entries[sigRef]
	return e, ok
}

// Delete removes a sovereign entry.
func (sm *SovereignMap) Delete(sigRef uint32) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	_, ok := sm.entries[sigRef]
	if ok {
		delete(sm.entries, sigRef)
	}
	return ok
}

// UpdateConsensus recalculates the consensus for an entry.
// Requires 2-of-3 valid signatures from distinct algorithm families.
func (sm *SovereignMap) UpdateConsensus(sigRef uint32) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	entry, ok := sm.entries[sigRef]
	if !ok {
		return fmt.Errorf("sophia: sovereign_map: sigRef %d not found", sigRef)
	}

	validCount := 0
	families := make(map[uint8]bool) // track distinct families
	for _, slot := range entry.Slots {
		if slot.Status == 1 { // valid
			validCount++
			families[algoFamily(slot.AlgoID)] = true
		}
	}

	if validCount >= 2 && len(families) >= 2 {
		entry.Consensus = 1 // passed
	} else if validCount == 1 {
		entry.Consensus = 3 // partial
	} else if validCount == 0 {
		entry.Consensus = 2 // failed
	} else {
		entry.Consensus = 3 // partial (2+ valid but same family)
	}

	return nil
}

// algoFamily maps AlgoID to a family ID for distinct-family checks.
func algoFamily(algoID uint8) uint8 {
	switch algoID {
	case 0x01: // SLH-DSA = hash-based
		return 1
	case 0x02, 0x03: // ML-DSA, FN-DSA = lattice
		return 2
	case 0x05: // HQC = code-based
		return 3
	default:
		return 0
	}
}

// Count returns the number of sovereign entries.
func (sm *SovereignMap) Count() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.entries)
}
