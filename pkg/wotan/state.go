// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package wotan provides the PQC state region for Wotan's protocol RAM.
// The PQC state occupies addresses 0xFF00-0xFF27 (40 bytes) and tracks
// post-quantum cryptographic authentication state including signature
// counts, key counts, verification results, and KEM tunnel status.
package wotan

import (
	"encoding/binary"
	"fmt"
	"sync"
	"sync/atomic"
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
//	0x08    4     verify_pass    — cumulative successful verifications
//	0x0C    4     verify_fail    — cumulative failed verifications
//	0x10    8     last_rotation  — Unix timestamp of last key rotation
//	0x18    1     policy_mode    — PESSIMISTIC (0x01) or OPTIMISTIC (0x02)
//	0x19    1     active_tier    — current compliance tier (0x00-0x03)
//	0x1A    2     kem_tunnels    — number of active KEM tunnels
//	0x1C    4     last_verify_ns — last verification latency (nanoseconds)
//	0x20    4     seq_high_water — highest seen sequence number
//	0x24    4     reserved       — reserved for future use
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
	Reserved     uint32
}

// Marshal serializes PQCState to 40 bytes (big-endian).
func (s *PQCState) Marshal() [PQCStateSize]byte {
	var buf [PQCStateSize]byte

	binary.BigEndian.PutUint32(buf[0x00:], s.SigCount)
	binary.BigEndian.PutUint32(buf[0x04:], s.KeyCount)
	binary.BigEndian.PutUint32(buf[0x08:], s.VerifyPass)
	binary.BigEndian.PutUint32(buf[0x0C:], s.VerifyFail)
	binary.BigEndian.PutUint64(buf[0x10:], uint64(s.LastRotation))
	buf[0x18] = s.PolicyMode
	buf[0x19] = s.ActiveTier
	binary.BigEndian.PutUint16(buf[0x1A:], s.KEMTunnels)
	binary.BigEndian.PutUint32(buf[0x1C:], s.LastVerifyNs)
	binary.BigEndian.PutUint32(buf[0x20:], s.SeqHighWater)
	binary.BigEndian.PutUint32(buf[0x24:], s.Reserved)

	return buf
}

// UnmarshalPQCState deserializes 40 bytes into PQCState.
func UnmarshalPQCState(b []byte) (PQCState, error) {
	if len(b) < PQCStateSize {
		return PQCState{}, fmt.Errorf("buffer too small: need %d bytes, got %d", PQCStateSize, len(b))
	}

	return PQCState{
		SigCount:     binary.BigEndian.Uint32(b[0x00:]),
		KeyCount:     binary.BigEndian.Uint32(b[0x04:]),
		VerifyPass:   binary.BigEndian.Uint32(b[0x08:]),
		VerifyFail:   binary.BigEndian.Uint32(b[0x0C:]),
		LastRotation: int64(binary.BigEndian.Uint64(b[0x10:])),
		PolicyMode:   b[0x18],
		ActiveTier:   b[0x19],
		KEMTunnels:   binary.BigEndian.Uint16(b[0x1A:]),
		LastVerifyNs: binary.BigEndian.Uint32(b[0x1C:]),
		SeqHighWater: binary.BigEndian.Uint32(b[0x20:]),
		Reserved:     binary.BigEndian.Uint32(b[0x24:]),
	}, nil
}

// PQCStateManager provides atomic read/write/CAS operations on PQC state
// stored in Wotan's protocol RAM.
type PQCStateManager struct {
	mu    sync.RWMutex
	state PQCState

	// Atomic counters for lock-free hot-path updates.
	verifyPass atomic.Uint64
	verifyFail atomic.Uint64
}

// NewPQCStateManager creates a new state manager with default values.
func NewPQCStateManager() *PQCStateManager {
	return &PQCStateManager{}
}

// Read returns a snapshot of the current PQC state.
// The returned snapshot includes the latest atomic counter values
// merged into the state fields.
func (m *PQCStateManager) Read() PQCState {
	m.mu.RLock()
	s := m.state
	m.mu.RUnlock()

	// Merge atomic counters into the snapshot.
	s.VerifyPass = uint32(m.verifyPass.Load())
	s.VerifyFail = uint32(m.verifyFail.Load())

	return s
}

// Write replaces the entire PQC state.
// The atomic counters are also updated to match the written state.
func (m *PQCStateManager) Write(state PQCState) {
	m.mu.Lock()
	m.state = state
	m.mu.Unlock()

	// Sync atomic counters with the written state.
	m.verifyPass.Store(uint64(state.VerifyPass))
	m.verifyFail.Store(uint64(state.VerifyFail))
}

// CAS performs compare-and-swap on the PQC state.
// Returns true if the swap was successful. The comparison uses the
// serialized wire format for byte-exact equality.
func (m *PQCStateManager) CAS(expected, desired PQCState) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Merge atomic counters into current state for comparison.
	current := m.state
	current.VerifyPass = uint32(m.verifyPass.Load())
	current.VerifyFail = uint32(m.verifyFail.Load())

	if current.Marshal() != expected.Marshal() {
		return false
	}

	m.state = desired
	m.verifyPass.Store(uint64(desired.VerifyPass))
	m.verifyFail.Store(uint64(desired.VerifyFail))
	return true
}

// IncrVerifyPass atomically increments the verify pass counter.
func (m *PQCStateManager) IncrVerifyPass() {
	m.verifyPass.Add(1)
}

// IncrVerifyFail atomically increments the verify fail counter.
func (m *PQCStateManager) IncrVerifyFail() {
	m.verifyFail.Add(1)
}

// UpdateSigCount atomically updates the signature count.
func (m *PQCStateManager) UpdateSigCount(count uint32) {
	m.mu.Lock()
	m.state.SigCount = count
	m.mu.Unlock()
}

// UpdateKeyCount atomically updates the key count.
func (m *PQCStateManager) UpdateKeyCount(count uint32) {
	m.mu.Lock()
	m.state.KeyCount = count
	m.mu.Unlock()
}

// SetLastRotation updates the last rotation timestamp.
func (m *PQCStateManager) SetLastRotation(ts int64) {
	m.mu.Lock()
	m.state.LastRotation = ts
	m.mu.Unlock()
}

// SetPolicyMode updates the policy mode.
func (m *PQCStateManager) SetPolicyMode(mode uint8) {
	m.mu.Lock()
	m.state.PolicyMode = mode
	m.mu.Unlock()
}

// SetActiveTier updates the active compliance tier.
func (m *PQCStateManager) SetActiveTier(tier uint8) {
	m.mu.Lock()
	m.state.ActiveTier = tier
	m.mu.Unlock()
}
