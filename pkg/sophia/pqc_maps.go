// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package sophia provides PQC (Post-Quantum Cryptography) BPF map definitions
// and control API for the Sophia service's signature-by-reference architecture.
//
// All serialization uses encoding/binary with BigEndian byte order, matching
// the wire format defined in draft-bellis-unheaded-protocol-foundation-03.
//
// Error wrapping follows the pattern: "sophia: <map_name>: <error>"
package sophia

import (
	"encoding/binary"
	"time"
)

// Ensure binary.BigEndian and time are used.
var (
	_ = binary.BigEndian
	_ = time.Now
)

// Map names for BPF pin paths.
const (
	PQCSigMapName       = "pqc_sig_map"       // SigRef -> signature + metadata
	PQCKeyMapName       = "pqc_key_map"       // KeyRef -> public key + trust
	PQCPolicyMapName    = "pqc_policy_map"    // service pair -> verification policy
	SovereignSigMapName = "sovereign_sig_map" // SigRef -> 3 cross-algo sigs
	KEMKeyMapName       = "kem_key_map"       // tunnel key entries
)

// Map size limits.
const (
	MaxSignatures    = 65536 // 2^16 SigRef space
	MaxKeys          = 4096  // 2^12 KeyRef space (upper 4 bits reserved)
	MaxPolicies      = 1024  // service pair policies
	MaxSovereignSigs = 8192  // sovereign multi-sig entries
	MaxKEMKeys       = 2048  // KEM tunnel keys
)

// PQCSigEntry is stored in PQC_SIG_MAP, keyed by SigRef (uint32).
// Full PQC signatures (up to 49KB for SLH-DSA) live here.
type PQCSigEntry struct {
	AlgoID    uint8    // Algorithm identifier (0x01-0x05)
	ParamSet  uint8    // Parameter set within algorithm family
	Tier      uint8    // Compliance tier (0x00-0x03)
	Status    uint8    // Verification status
	FlowID    [16]byte // Flow identifier for binding
	Epoch     uint32   // Rotation epoch
	CreatedAt int64    // Unix timestamp (seconds)
	ExpiresAt int64    // Expiration timestamp
	SigLen    uint32   // Actual signature length
	Signature []byte   // The actual PQC signature (variable length, up to 49856 bytes)
}

// PQCKeyEntry is stored in PQC_KEY_MAP, keyed by KeyRef (uint16).
type PQCKeyEntry struct {
	AlgoID     uint8  // Algorithm identifier
	ParamSet   uint8  // Parameter set
	TrustTier  uint8  // Trust tier (NONE/STANDARD/ENHANCED/SOVEREIGN)
	Status     uint8  // Key status (active/rotating/revoked)
	ServiceID  uint16 // Owning service ID
	KeyLen     uint32 // Actual public key length
	CreatedAt  int64  // Unix timestamp
	ExpiresAt  int64  // Expiration timestamp
	RotationID uint32 // Rotation identifier (for grace period tracking)
	PublicKey  []byte // The actual public key (variable length)
}

// KeyStatus values.
const (
	KeyStatusActive   uint8 = 0x01
	KeyStatusRotating uint8 = 0x02 // Grace period: old + new both valid
	KeyStatusRevoked  uint8 = 0x03
	KeyStatusExpired  uint8 = 0x04
)

// PQCPolicy defines verification requirements for a source/destination service pair.
type PQCPolicy struct {
	SrcServiceID uint16 // Source service ID (0 = any)
	DstServiceID uint16 // Destination service ID (0 = any)
	Mode         uint8  // PESSIMISTIC (0x01) or OPTIMISTIC (0x02)
	RequiredTier uint8  // Minimum compliance tier
	AllowedAlgos uint8  // Bitmask of allowed algorithm IDs
	MaxSigAge    uint32 // Maximum signature age in seconds
	GracePeriod  uint32 // Key rotation grace period in seconds
	Flags        uint8  // Policy flags
}

// Policy modes.
const (
	PolicyModePessimistic uint8 = 0x01 // Reject unsigned packets
	PolicyModeOptimistic  uint8 = 0x02 // Warn-only for unsigned packets
)

// SovereignSigEntry holds 3 cross-algorithm signatures for SOVEREIGN tier.
// Requires 2-of-3 valid signatures from DISTINCT algorithm families.
type SovereignSigEntry struct {
	SigRef1   uint32 // First signature reference (e.g., SLH-DSA)
	AlgoID1   uint8  // Algorithm family 1
	Status1   uint8  // Verification status 1
	SigRef2   uint32 // Second signature reference (e.g., ML-DSA)
	AlgoID2   uint8  // Algorithm family 2
	Status2   uint8  // Verification status 2
	SigRef3   uint32 // Third signature reference (e.g., FN-DSA)
	AlgoID3   uint8  // Algorithm family 3
	Status3   uint8  // Verification status 3
	Consensus uint8  // 2-of-3 consensus result
	Pad       uint8  // Alignment padding
}

// Consensus values.
const (
	ConsensusUnknown uint8 = 0x00
	ConsensusPassed  uint8 = 0x01
	ConsensusFailed  uint8 = 0x02
	ConsensusPartial uint8 = 0x03 // 1 of 3 passed
)

// KEMKeyEntry stores tunnel key material.
type KEMKeyEntry struct {
	AlgoID       uint8    // ML-KEM or HQC
	ParamSet     uint8    // Parameter set
	Status       uint8    // Active/expired
	Pad          uint8    // Alignment padding
	PeerService  uint16   // Peer service ID
	TunnelID     uint16   // Tunnel identifier
	SharedSecret [32]byte // Derived shared secret (after HKDF)
	CreatedAt    int64    // Unix timestamp
	ExpiresAt    int64    // Expiration
}

// PolicyKey encodes a source/destination service pair into a single uint32
// for use as a BPF map key: (srcSvcID << 16) | dstSvcID.
func PolicyKey(srcSvcID, dstSvcID uint16) uint32 {
	return (uint32(srcSvcID) << 16) | uint32(dstSvcID)
}

// EvaluateConsensus computes the 2-of-3 consensus result for a SovereignSigEntry.
// A status of 0x01 is considered "passed". At least 2 of 3 must pass for ConsensusPassed.
func EvaluateConsensus(entry *SovereignSigEntry) uint8 {
	passed := 0
	if entry.Status1 == 0x01 {
		passed++
	}
	if entry.Status2 == 0x01 {
		passed++
	}
	if entry.Status3 == 0x01 {
		passed++
	}

	switch {
	case passed >= 2:
		return ConsensusPassed
	case passed == 1:
		return ConsensusPartial
	case passed == 0 && (entry.Status1 != 0x00 || entry.Status2 != 0x00 || entry.Status3 != 0x00):
		return ConsensusFailed
	default:
		return ConsensusUnknown
	}
}
