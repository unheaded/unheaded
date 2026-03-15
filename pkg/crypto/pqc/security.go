// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package pqc

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"fmt"
	"sync"
	"time"
)

// ConstantTimeCompareFingerprint compares two 4-byte fingerprints in constant time.
// This prevents timing side-channel attacks when validating HashPfx values
// in the Monad wire format.
func ConstantTimeCompareFingerprint(a, b [4]byte) bool {
	return subtle.ConstantTimeCompare(a[:], b[:]) == 1
}

// ImmutableSigBinding creates a SHA-256 binding of a SigRef to its context.
// This prevents SigRef reuse across different flows or epochs.
//
//	binding = SHA-256(flow_id || epoch || signature)
//
// The epoch is encoded as a 4-byte big-endian uint32 to ensure a canonical
// representation that cannot be confused with variable-length flow_id bytes.
func ImmutableSigBinding(flowID []byte, epoch uint32, signature []byte) [32]byte {
	h := sha256.New()
	h.Write(flowID)
	var epochBytes [4]byte
	binary.BigEndian.PutUint32(epochBytes[:], epoch)
	h.Write(epochBytes[:])
	h.Write(signature)
	var binding [32]byte
	copy(binding[:], h.Sum(nil))
	return binding
}

// VerifySigBinding checks that a SigRef binding matches the expected
// combination of flow ID, epoch, and signature. Uses constant-time
// comparison to prevent timing side-channel attacks.
func VerifySigBinding(binding [32]byte, flowID []byte, epoch uint32, signature []byte) bool {
	expected := ImmutableSigBinding(flowID, epoch, signature)
	return subtle.ConstantTimeCompare(binding[:], expected[:]) == 1
}

// AlgoConfusionGuard prevents algorithm family changes mid-flow.
// Once a flow establishes its algorithm (e.g., ML-DSA), any attempt to
// switch to a different algorithm (e.g., FN-DSA) on the same flow is
// rejected. This prevents downgrade attacks where an attacker forces
// a weaker algorithm after initial negotiation.
type AlgoConfusionGuard struct {
	mu    sync.RWMutex
	flows map[string]uint8 // flowID -> first-seen algoID
}

// NewAlgoConfusionGuard creates a new algorithm confusion guard.
func NewAlgoConfusionGuard() *AlgoConfusionGuard {
	return &AlgoConfusionGuard{
		flows: make(map[string]uint8),
	}
}

// Check returns an error if the algorithm changed for an existing flow.
// On first call for a flow, the algorithm is recorded. Subsequent calls
// must use the same algorithm ID or an error is returned.
func (g *AlgoConfusionGuard) Check(flowID string, algoID uint8) error {
	// Fast path: read lock for existing flows.
	g.mu.RLock()
	existing, ok := g.flows[flowID]
	g.mu.RUnlock()

	if ok {
		if existing != algoID {
			return fmt.Errorf("algorithm confusion: flow %q established algo %#x, got %#x", flowID, existing, algoID)
		}
		return nil
	}

	// Slow path: write lock to register new flow.
	g.mu.Lock()
	defer g.mu.Unlock()

	// Double-check after acquiring write lock.
	existing, ok = g.flows[flowID]
	if ok {
		if existing != algoID {
			return fmt.Errorf("algorithm confusion: flow %q established algo %#x, got %#x", flowID, existing, algoID)
		}
		return nil
	}

	g.flows[flowID] = algoID
	return nil
}

// SigRefRateLimiter prevents SigRef exhaustion attacks.
// Limits SigRef allocation to maxRate per second with LRU eviction
// triggered when utilization exceeds evictAt threshold.
//
// The SigRef space is finite (e.g., 2^24 entries). An attacker could
// attempt to exhaust it by rapidly allocating SigRefs. This limiter
// caps allocation rate and signals when eviction is needed.
type SigRefRateLimiter struct {
	mu          sync.Mutex
	count       int64
	maxRate     int64     // max allocations per second
	windowStart time.Time // start of current rate window
	evictAt     float64   // eviction threshold (e.g., 0.9 = 90%)
	maxRefs     int64     // total SigRef space
}

// NewSigRefRateLimiter creates a new rate limiter for SigRef allocations.
//
// Parameters:
//   - maxRate: maximum number of SigRef allocations allowed per second
//   - maxRefs: total SigRef address space (e.g., 1<<24 for 24-bit refs)
//   - evictThreshold: utilization ratio (0.0-1.0) at which eviction is needed
func NewSigRefRateLimiter(maxRate, maxRefs int64, evictThreshold float64) *SigRefRateLimiter {
	return &SigRefRateLimiter{
		maxRate:     maxRate,
		maxRefs:     maxRefs,
		evictAt:     evictThreshold,
		windowStart: time.Now(),
	}
}

// Allow checks if a SigRef allocation is allowed under the rate limit.
// Returns true if the allocation is permitted, false if the rate limit
// has been reached for the current one-second window.
func (r *SigRefRateLimiter) Allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()

	// Reset counter if we have moved into a new one-second window.
	if now.Sub(r.windowStart) >= time.Second {
		r.count = 0
		r.windowStart = now
	}

	if r.count >= r.maxRate {
		return false
	}

	r.count++
	return true
}

// Utilization returns the current SigRef space utilization as a ratio (0.0-1.0).
// currentCount is the number of SigRefs currently allocated in the system.
func (r *SigRefRateLimiter) Utilization(currentCount int64) float64 {
	if r.maxRefs <= 0 {
		return 0.0
	}
	return float64(currentCount) / float64(r.maxRefs)
}

// NeedsEviction returns true if utilization exceeds the configured
// eviction threshold, signaling that the caller should begin evicting
// least-recently-used SigRef entries.
func (r *SigRefRateLimiter) NeedsEviction(currentCount int64) bool {
	return r.Utilization(currentCount) >= r.evictAt
}
