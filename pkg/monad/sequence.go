// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package monad

import (
	"fmt"
	"sync"
)

const (
	// SeqWindowSize is the number of packets tracked per flow.
	SeqWindowSize = 16384
	// seqBitmapBytes is the size of the bitmap in bytes.
	seqBitmapBytes = SeqWindowSize / 8
)

// SeqTracker provides per-flow sliding window replay protection.
// It tracks the most recent 16384 sequence numbers per flow.
//
// The bitmap tracks which of the last SeqWindowSize sequence numbers
// have been seen. When a new maximum is established, the window slides
// forward and bits for old sequence numbers are cleared.
type SeqTracker struct {
	mu     sync.Mutex
	maxSeq uint16
	bitmap [seqBitmapBytes]byte
}

// NewSeqTracker creates a new sequence tracker.
func NewSeqTracker() *SeqTracker { return &SeqTracker{} }

// Check validates a sequence number and marks it as seen.
// Returns true if the sequence number is valid (not a replay).
// Returns false if it's a replay or too old.
func (t *SeqTracker) Check(seqNum uint16) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	// First packet ever: accept and set as maximum.
	if t.maxSeq == 0 && !t.getBit(0) {
		t.maxSeq = seqNum
		t.setBit(seqNum)
		return true
	}

	if seqNum == t.maxSeq {
		// Exact duplicate of the current maximum.
		return false
	}

	// Use the half-space approach to determine direction in the uint16
	// wraparound space. A difference in [1, 32767] is considered "ahead";
	// [32768, 65535] is considered "behind" (i.e., diff > 32767).
	diff := seqNum - t.maxSeq // wraps naturally for uint16

	if diff > 0 && diff <= 32767 {
		// seqNum is ahead of maxSeq: advance the window.
		if diff >= SeqWindowSize {
			// Jump is larger than the window; clear entire bitmap.
			t.bitmap = [seqBitmapBytes]byte{}
		} else {
			t.advanceWindow(diff)
		}
		t.maxSeq = seqNum
		t.setBit(seqNum)
		return true
	}

	// seqNum is behind maxSeq.
	back := t.maxSeq - seqNum // wraps naturally for uint16

	if back >= SeqWindowSize {
		// Too old: outside the replay window.
		return false
	}

	// Within the window behind maxSeq. Check for replay.
	if t.getBit(seqNum) {
		return false // replay
	}

	t.setBit(seqNum)
	return true
}

// MaxSeq returns the highest seen sequence number.
func (t *SeqTracker) MaxSeq() uint16 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.maxSeq
}

// setBit marks a sequence number as seen in the bitmap.
func (t *SeqTracker) setBit(seqNum uint16) {
	idx := seqNum % SeqWindowSize
	byteIdx := idx / 8
	bitIdx := idx % 8
	t.bitmap[byteIdx] |= 1 << bitIdx
}

// getBit checks if a sequence number is marked as seen in the bitmap.
func (t *SeqTracker) getBit(seqNum uint16) bool {
	idx := seqNum % SeqWindowSize
	byteIdx := idx / 8
	bitIdx := idx % 8
	return t.bitmap[byteIdx]&(1<<bitIdx) != 0
}

// clearBit clears a single bit in the bitmap.
func (t *SeqTracker) clearBit(seqNum uint16) {
	idx := seqNum % SeqWindowSize
	byteIdx := idx / 8
	bitIdx := idx % 8
	t.bitmap[byteIdx] &^= 1 << bitIdx
}

// advanceWindow clears bits for sequence numbers that are sliding out of
// the window as it advances by `distance` positions.
func (t *SeqTracker) advanceWindow(distance uint16) {
	if distance >= SeqWindowSize {
		// Entire window is invalidated; clear everything.
		t.bitmap = [seqBitmapBytes]byte{}
		return
	}

	// Clear the bits that will be occupied by the new range.
	// These are the positions from (maxSeq+1) to (maxSeq+distance), mapped
	// into the circular bitmap.
	for i := uint16(1); i <= distance; i++ {
		t.clearBit(t.maxSeq + i)
	}
}

// FlowSeqTracker manages per-flow sequence trackers.
type FlowSeqTracker struct {
	mu       sync.RWMutex
	trackers map[string]*SeqTracker
	maxFlows int
}

// NewFlowSeqTracker creates a new per-flow tracker.
// maxFlows limits the number of concurrent flows tracked. When exceeded,
// new flows are rejected.
func NewFlowSeqTracker(maxFlows int) *FlowSeqTracker {
	return &FlowSeqTracker{
		trackers: make(map[string]*SeqTracker),
		maxFlows: maxFlows,
	}
}

// Check validates a sequence number for a flow.
// Creates a new per-flow tracker if one does not exist (subject to maxFlows).
// Returns false if the flow limit is exceeded or the sequence number is a replay.
func (f *FlowSeqTracker) Check(flowID string, seqNum uint16) bool {
	// Fast path: read lock to check existing tracker.
	f.mu.RLock()
	tracker, ok := f.trackers[flowID]
	f.mu.RUnlock()

	if ok {
		return tracker.Check(seqNum)
	}

	// Slow path: write lock to create new tracker.
	f.mu.Lock()
	defer f.mu.Unlock()

	// Double-check after acquiring write lock.
	tracker, ok = f.trackers[flowID]
	if ok {
		return tracker.Check(seqNum)
	}

	// Enforce flow limit.
	if len(f.trackers) >= f.maxFlows {
		return false
	}

	tracker = NewSeqTracker()
	f.trackers[flowID] = tracker
	return tracker.Check(seqNum)
}

// FlowCount returns the number of tracked flows.
func (f *FlowSeqTracker) FlowCount() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.trackers)
}

// String returns a human-readable representation of the FlowSeqTracker.
func (f *FlowSeqTracker) String() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return fmt.Sprintf("FlowSeqTracker{flows=%d, max=%d}", len(f.trackers), f.maxFlows)
}
