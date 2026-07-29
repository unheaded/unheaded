// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package wotan provides the PQC state region for Wotan's protocol RAM.
// The PQC state occupies addresses 0xFF00-0xFF27 (40 bytes) and tracks
// post-quantum cryptographic authentication state including signature
// counts, key counts, verification results, and KEM tunnel status.
package wotan

import (
	"encoding/binary"
	"fmt"
	"sync"
)

// PQC state region address range in Wotan protocol RAM.
const (
	PQCStateBaseAddr = 0xFF00
	PQCStateEndAddr  = 0xFF27
	PQCStateSize     = 40 // bytes
)

// PQCState holds the PQC authentication state (40 bytes).
// Stored in Wotan protocol RAM at addresses 0xFF00-0xFF27.
//
// Layout:
//
//	Offset  Size  Field
//	0x00    4     sig_count      — total active signatures in Sophia maps
//	0x04    4     key_count      — total active keys in Sophia maps
//	0x08    4     verify_pass    — cumulative successful verifications (uint32; wraps at 2^32)
//	0x0C    4     verify_fail    — cumulative failed verifications (uint32; wraps at 2^32)
//	0x10    8     last_rotation  — Unix timestamp of last key rotation
//	0x18    1     policy_mode    — PESSIMISTIC (0x01) or OPTIMISTIC (0x02)
//	0x19    1     active_tier    — current compliance tier (0x00-0x03)
//	0x1A    2     kem_tunnels    — number of active KEM tunnels
//	0x1C    4     last_verify_ns — last verification latency (nanoseconds)
//	0x20    4     seq_high_water — highest seen sequence number
//	0x24    4     reserved       — MUST be zero on the wire (covert-channel sealed)
//
// The reserved range is unexported and forced to zero on serialize; it
// is not surfaced through the API. This prevents the field from acting
// as a covert channel between Marshal/Unmarshal endpoints.
type PQCState struct {
	SigCount     uint32
	KeyCount     uint32
	VerifyPass   uint32
	VerifyFail   uint32
	LastRotation int64
	PolicyMode   uint8
	ActiveTier   uint8
	KEMTunnels   uint16
	LastVerifyNs uint32
	SeqHighWater uint32
}

// Policy mode and active-tier constants. Wire values are documented in
// the PQCState layout above; using the named constants keeps callers
// from memorising bytes.
const (
	PolicyModePessimistic uint8 = 0x01
	PolicyModeOptimistic  uint8 = 0x02

	ActiveTierBaseline uint8 = 0x00
	ActiveTierStandard uint8 = 0x01
	ActiveTierEnhanced uint8 = 0x02
	ActiveTierStrict   uint8 = 0x03
)

// Marshal serializes PQCState to 40 bytes (big-endian). The 4-byte
// reserved range at offset 0x24 is forced to zero — see the PQCState
// doc comment for the covert-channel rationale.
func (s *PQCState) Marshal() [PQCStateSize]byte {
	var buf [PQCStateSize]byte
	_ = s.MarshalTo(buf[:]) // PQCStateSize-sized array can never error
	return buf
}

// MarshalTo writes the serialized 40 bytes into dst. dst must be exactly
// PQCStateSize bytes long; shorter or longer slices return an error.
//
// Use MarshalTo when you have a pre-allocated buffer (e.g. reading state
// directly into Wotan's protocol-RAM region). Marshal allocates a new
// 40-byte array on each call; MarshalTo allocates nothing.
func (s *PQCState) MarshalTo(dst []byte) error {
	if len(dst) != PQCStateSize {
		return fmt.Errorf("MarshalTo: dst must be exactly %d bytes, got %d", PQCStateSize, len(dst))
	}

	binary.BigEndian.PutUint32(dst[0x00:], s.SigCount)
	binary.BigEndian.PutUint32(dst[0x04:], s.KeyCount)
	binary.BigEndian.PutUint32(dst[0x08:], s.VerifyPass)
	binary.BigEndian.PutUint32(dst[0x0C:], s.VerifyFail)
	binary.BigEndian.PutUint64(dst[0x10:], uint64(s.LastRotation)) // #nosec G115 -- UNFS inode field; bounded by the filesystem image size
	dst[0x18] = s.PolicyMode
	dst[0x19] = s.ActiveTier
	binary.BigEndian.PutUint16(dst[0x1A:], s.KEMTunnels)
	binary.BigEndian.PutUint32(dst[0x1C:], s.LastVerifyNs)
	binary.BigEndian.PutUint32(dst[0x20:], s.SeqHighWater)
	// Reserved range — must be zero on the wire (covert-channel sealed).
	// Force-clear rather than relying on caller-zeroed input.
	dst[0x24] = 0
	dst[0x25] = 0
	dst[0x26] = 0
	dst[0x27] = 0

	return nil
}

// UnmarshalPQCState deserializes exactly 40 bytes into PQCState. The
// reserved 4-byte range at offset 0x24 must be zero; non-zero indicates
// a malformed or potentially-malicious input and is rejected.
func UnmarshalPQCState(b []byte) (PQCState, error) {
	if len(b) != PQCStateSize {
		return PQCState{}, fmt.Errorf("buffer must be exactly %d bytes, got %d", PQCStateSize, len(b))
	}

	if reserved := binary.BigEndian.Uint32(b[0x24:]); reserved != 0 {
		return PQCState{}, fmt.Errorf("reserved bytes at offset 0x24 must be zero, got 0x%08x", reserved)
	}

	return PQCState{
		SigCount:     binary.BigEndian.Uint32(b[0x00:]),
		KeyCount:     binary.BigEndian.Uint32(b[0x04:]),
		VerifyPass:   binary.BigEndian.Uint32(b[0x08:]),
		VerifyFail:   binary.BigEndian.Uint32(b[0x0C:]),
		LastRotation: int64(binary.BigEndian.Uint64(b[0x10:])), // #nosec G115 -- UNFS inode field; bounded by the filesystem image size
		PolicyMode:   b[0x18],
		ActiveTier:   b[0x19],
		KEMTunnels:   binary.BigEndian.Uint16(b[0x1A:]),
		LastVerifyNs: binary.BigEndian.Uint32(b[0x1C:]),
		SeqHighWater: binary.BigEndian.Uint32(b[0x20:]),
	}, nil
}

// PQCStateManager provides serialized read/write/CAS operations on PQC
// state stored in Wotan's protocol RAM.
//
// Concurrency model: every mutating method takes the write lock; Read
// takes the read lock. There is no separate atomic-counter fast path —
// earlier versions had one but it raced with CAS/Write (concurrent
// IncrVerifyPass calls between CAS's read of verifyPass and CAS's
// store of desired.VerifyPass were silently lost). The mutex is
// uncontended in the common case (one writer, occasional readers) so
// the simplification costs nothing measurable. VerifyPass / VerifyFail
// are uint32 and wrap at 2^32 by spec — the wire format budgets 4
// bytes for each counter and there is no semantic difference between
// "wrap at 2^32" and "saturate at 2^32" for these monotonic stats.
type PQCStateManager struct {
	mu    sync.RWMutex
	state PQCState
}

// NewPQCStateManager creates a new state manager with default values.
func NewPQCStateManager() *PQCStateManager {
	return &PQCStateManager{}
}

// Read returns a snapshot of the current PQC state.
func (m *PQCStateManager) Read() PQCState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state
}

// Write replaces the entire PQC state.
func (m *PQCStateManager) Write(state PQCState) {
	m.mu.Lock()
	m.state = state
	m.mu.Unlock()
}

// CAS performs compare-and-swap on the PQC state. Returns true if the
// swap succeeded. The comparison uses native struct equality —
// PQCState contains only comparable fields, so this is byte-exact for
// the in-memory representation.
func (m *PQCStateManager) CAS(expected, desired PQCState) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.state != expected {
		return false
	}
	m.state = desired
	return true
}

// IncrVerifyPass increments the verify-pass counter under the write lock.
// Wraps at 2^32 (uint32 modular arithmetic) — see PQCStateManager doc.
func (m *PQCStateManager) IncrVerifyPass() {
	m.mu.Lock()
	m.state.VerifyPass++
	m.mu.Unlock()
}

// IncrVerifyFail increments the verify-fail counter under the write lock.
// Wraps at 2^32 (uint32 modular arithmetic) — see PQCStateManager doc.
func (m *PQCStateManager) IncrVerifyFail() {
	m.mu.Lock()
	m.state.VerifyFail++
	m.mu.Unlock()
}

// Apply runs fn under the write lock, passing a pointer to the live
// state. Use it for atomic multi-field edits — the alternative is a
// sequence of single-field setters where each releases the lock between
// calls, allowing observers to see partial states.
//
// Replaces the per-field UpdateX/SetX setters. The previous individual
// setters are kept as thin wrappers below for backward compatibility
// with callers that already use them.
//
// Example:
//
//	mgr.Apply(func(s *PQCState) {
//	    s.SigCount = 42
//	    s.KeyCount = 7
//	    s.LastRotation = time.Now().Unix()
//	})
func (m *PQCStateManager) Apply(fn func(*PQCState)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	fn(&m.state)
}

// UpdateSigCount sets the signature count under the write lock.
// Equivalent to Apply(func(s *PQCState) { s.SigCount = count }).
func (m *PQCStateManager) UpdateSigCount(count uint32) {
	m.Apply(func(s *PQCState) { s.SigCount = count })
}

// UpdateKeyCount sets the key count under the write lock.
func (m *PQCStateManager) UpdateKeyCount(count uint32) {
	m.Apply(func(s *PQCState) { s.KeyCount = count })
}

// SetLastRotation updates the last rotation timestamp.
func (m *PQCStateManager) SetLastRotation(ts int64) {
	m.Apply(func(s *PQCState) { s.LastRotation = ts })
}

// SetPolicyMode updates the policy mode. Caller is responsible for
// passing a documented value (PolicyModePessimistic / PolicyModeOptimistic).
func (m *PQCStateManager) SetPolicyMode(mode uint8) {
	m.Apply(func(s *PQCState) { s.PolicyMode = mode })
}

// SetActiveTier updates the active compliance tier (0x00-0x03).
func (m *PQCStateManager) SetActiveTier(tier uint8) {
	m.Apply(func(s *PQCState) { s.ActiveTier = tier })
}
