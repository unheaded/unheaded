// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package prefetch provides explicit prefetch hints management for the Unheaded protocol.
package prefetch

import (
	"fmt"
	"sync"
)

// MaxOutstandingPrefetch is the default maximum number of outstanding prefetch hints.
const MaxOutstandingPrefetch uint16 = 16

// PrefetchHint represents an explicit prefetch hint.
type PrefetchHint struct {
	FlowLabel  uint32
	BaseAddr   uint32
	PageCount  uint8
	ID         uint64
}

// CancelPrefetch represents a prefetch cancellation request.
type CancelPrefetch struct {
	FlowLabel uint32
	HintID    uint64
}

// PrefetchManager tracks outstanding prefetch hints and enforces limits.
type PrefetchManager struct {
	mu                       sync.RWMutex
	hints                    map[uint64]*PrefetchHint // hintID -> hint
	flowHints                map[uint32][]uint64      // flowLabel -> []hintID
	nextID                   uint64
	maxOutstandingPrefetch   uint16
	maxOutstandingPerFlow    uint16
}

// NewPrefetchManager creates a new prefetch manager with default limits.
func NewPrefetchManager() *PrefetchManager {
	return &PrefetchManager{
		hints:                  make(map[uint64]*PrefetchHint),
		flowHints:              make(map[uint32][]uint64),
		nextID:                 1,
		maxOutstandingPrefetch: MaxOutstandingPrefetch,
		maxOutstandingPerFlow:  8, // reasonable per-flow limit
	}
}

// NewPrefetchManagerWithLimits creates a manager with custom limits.
func NewPrefetchManagerWithLimits(maxOutstanding, maxPerFlow uint16) *PrefetchManager {
	return &PrefetchManager{
		hints:                  make(map[uint64]*PrefetchHint),
		flowHints:              make(map[uint32][]uint64),
		nextID:                 1,
		maxOutstandingPrefetch: maxOutstanding,
		maxOutstandingPerFlow:  maxPerFlow,
	}
}

// AddHint adds a prefetch hint and returns its ID.
// Returns error if limits would be exceeded.
func (pm *PrefetchManager) AddHint(flowLabel uint32, baseAddr uint32, pageCount uint8) (uint64, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Check global limit
	if uint16(len(pm.hints)) >= pm.maxOutstandingPrefetch {
		return 0, fmt.Errorf("exceeded maximum outstanding prefetch hints: %d", pm.maxOutstandingPrefetch)
	}

	// Check per-flow limit
	flowCount := len(pm.flowHints[flowLabel])
	if uint16(flowCount) >= pm.maxOutstandingPerFlow {
		return 0, fmt.Errorf("exceeded maximum prefetch hints for flow %d: %d", flowLabel, pm.maxOutstandingPerFlow)
	}

	// Create hint
	hintID := pm.nextID
	pm.nextID++

	hint := &PrefetchHint{
		FlowLabel: flowLabel,
		BaseAddr:  baseAddr,
		PageCount: pageCount,
		ID:        hintID,
	}

	pm.hints[hintID] = hint
	pm.flowHints[flowLabel] = append(pm.flowHints[flowLabel], hintID)

	return hintID, nil
}

// GetHint retrieves a hint by ID.
func (pm *PrefetchManager) GetHint(hintID uint64) (*PrefetchHint, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	hint, exists := pm.hints[hintID]
	return hint, exists
}

// CancelHint removes a prefetch hint.
// Returns error if hint does not exist.
func (pm *PrefetchManager) CancelHint(hintID uint64) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	hint, exists := pm.hints[hintID]
	if !exists {
		return fmt.Errorf("prefetch hint not found: %d", hintID)
	}

	delete(pm.hints, hintID)

	// Remove from flow hints
	flowLabel := hint.FlowLabel
	flowHints := pm.flowHints[flowLabel]
	for i, id := range flowHints {
		if id == hintID {
			pm.flowHints[flowLabel] = append(flowHints[:i], flowHints[i+1:]...)
			break
		}
	}

	// Clean up empty flow entries
	if len(pm.flowHints[flowLabel]) == 0 {
		delete(pm.flowHints, flowLabel)
	}

	return nil
}

// CancelFlowHints cancels all hints for a specific flow.
func (pm *PrefetchManager) CancelFlowHints(flowLabel uint32) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	hintIDs, exists := pm.flowHints[flowLabel]
	if !exists {
		return nil // no hints for this flow
	}

	// Delete all hints for this flow
	for _, hintID := range hintIDs {
		delete(pm.hints, hintID)
	}

	delete(pm.flowHints, flowLabel)

	return nil
}

// GetFlowHints returns all hints for a specific flow.
func (pm *PrefetchManager) GetFlowHints(flowLabel uint32) []*PrefetchHint {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	hintIDs, exists := pm.flowHints[flowLabel]
	if !exists {
		return nil
	}

	hints := make([]*PrefetchHint, 0, len(hintIDs))
	for _, hintID := range hintIDs {
		if hint, ok := pm.hints[hintID]; ok {
			hints = append(hints, hint)
		}
	}

	return hints
}

// OutstandingCount returns the current count of outstanding hints.
func (pm *PrefetchManager) OutstandingCount() uint16 {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	return uint16(len(pm.hints))
}

// SetMaxOutstanding updates the maximum outstanding prefetch limit.
func (pm *PrefetchManager) SetMaxOutstanding(max uint16) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.maxOutstandingPrefetch = max
}

// GetMaxOutstanding returns the current maximum outstanding limit.
func (pm *PrefetchManager) GetMaxOutstanding() uint16 {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	return pm.maxOutstandingPrefetch
}
