// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package policy defines verification policies for PQC (Post-Quantum
// Cryptography) compliance tiers.  Each tier specifies which signature
// algorithms are required/allowed, how stale a signature may be before
// rejection, and what teardown action to take when verification fails in
// PESSIMISTIC mode.
package policy

import (
	"fmt"
	"time"
)

// VerificationMode determines how unsigned packets are handled.
type VerificationMode uint8

const (
	// ModePessimistic rejects packets that lack valid PQC authentication.
	// Used for SOVEREIGN and ENHANCED tiers.
	ModePessimistic VerificationMode = 0x01
	// ModeOptimistic warns but allows packets without PQC authentication.
	// Used for STANDARD tier and during migration.
	ModeOptimistic VerificationMode = 0x02
)

// String returns a human-readable name for the mode.
func (m VerificationMode) String() string {
	switch m {
	case ModePessimistic:
		return "PESSIMISTIC"
	case ModeOptimistic:
		return "OPTIMISTIC"
	default:
		return fmt.Sprintf("VerificationMode(0x%02x)", uint8(m))
	}
}

// ComplianceTier represents the PQC compliance level.
type ComplianceTier uint8

const (
	TierNone      ComplianceTier = 0x00 // No PQC required
	TierStandard  ComplianceTier = 0x01 // At least one PQC signature
	TierEnhanced  ComplianceTier = 0x02 // PQC + specific algorithm requirements
	TierSovereign ComplianceTier = 0x03 // 2-of-3 cross-algorithm multi-sig
)

// String returns the tier name.
func (t ComplianceTier) String() string {
	switch t {
	case TierNone:
		return "NONE"
	case TierStandard:
		return "STANDARD"
	case TierEnhanced:
		return "ENHANCED"
	case TierSovereign:
		return "SOVEREIGN"
	default:
		return fmt.Sprintf("ComplianceTier(0x%02x)", uint8(t))
	}
}

// AlgoID constants (duplicated here to avoid circular import with pkg/crypto/pqc)
const (
	AlgoSLHDSA uint8 = 0x01
	AlgoMLDSA  uint8 = 0x02
	AlgoFNDSA  uint8 = 0x03
	AlgoMLKEM  uint8 = 0x04
	AlgoHQC    uint8 = 0x05
)

// TierPolicy defines the verification requirements for a compliance tier.
type TierPolicy struct {
	Tier             ComplianceTier
	Mode             VerificationMode
	RequiredAlgos    []uint8        // Algorithm IDs that MUST be present
	AllowedAlgos     []uint8        // Algorithm IDs that are accepted
	MinTrustTier     ComplianceTier // Minimum trust tier for signing keys
	MaxSigAge        time.Duration  // Maximum age of a signature
	GracePeriod      time.Duration  // Grace period during key rotation
	RequireMultiSig  bool           // Require multiple signatures (SOVEREIGN)
	MinMultiSigCount int            // Minimum number of valid sigs for multi-sig
	DistinctFamilies bool           // Require sigs from distinct algorithm families
}

// DefaultPolicyForTier returns the default policy for a given compliance tier.
func DefaultPolicyForTier(tier ComplianceTier) *TierPolicy {
	switch tier {
	case TierNone:
		return &TierPolicy{
			Tier:         TierNone,
			Mode:         ModeOptimistic,
			AllowedAlgos: []uint8{AlgoSLHDSA, AlgoMLDSA, AlgoFNDSA},
			MaxSigAge:    24 * time.Hour,
			GracePeriod:  60 * time.Second,
		}
	case TierStandard:
		return &TierPolicy{
			Tier:          TierStandard,
			Mode:          ModeOptimistic,
			RequiredAlgos: []uint8{}, // Any one PQC sig suffices
			AllowedAlgos:  []uint8{AlgoSLHDSA, AlgoMLDSA},
			MinTrustTier:  TierStandard,
			MaxSigAge:     1 * time.Hour,
			GracePeriod:   60 * time.Second,
		}
	case TierEnhanced:
		return &TierPolicy{
			Tier:          TierEnhanced,
			Mode:          ModePessimistic,
			RequiredAlgos: []uint8{AlgoMLDSA}, // ML-DSA required
			AllowedAlgos:  []uint8{AlgoSLHDSA, AlgoMLDSA},
			MinTrustTier:  TierEnhanced,
			MaxSigAge:     30 * time.Minute,
			GracePeriod:   60 * time.Second,
		}
	case TierSovereign:
		return &TierPolicy{
			Tier:             TierSovereign,
			Mode:             ModePessimistic,
			RequiredAlgos:    []uint8{AlgoSLHDSA, AlgoMLDSA},
			AllowedAlgos:     []uint8{AlgoSLHDSA, AlgoMLDSA, AlgoFNDSA},
			MinTrustTier:     TierSovereign,
			MaxSigAge:        15 * time.Minute,
			GracePeriod:      60 * time.Second,
			RequireMultiSig:  true,
			MinMultiSigCount: 2, // 2-of-3
			DistinctFamilies: true,
		}
	default:
		return DefaultPolicyForTier(TierNone)
	}
}

// FlowTeardownAction defines what happens when verification fails in PESSIMISTIC mode.
type FlowTeardownAction uint8

const (
	// TeardownNone takes no action (OPTIMISTIC mode).
	TeardownNone FlowTeardownAction = 0x00
	// TeardownTCPRST sends a TCP RST to tear down the connection.
	TeardownTCPRST FlowTeardownAction = 0x01
	// TeardownICMPv6 sends an ICMPv6 Destination Unreachable (Admin Prohibited).
	TeardownICMPv6 FlowTeardownAction = 0x02
	// TeardownSilentDrop silently drops the packet.
	TeardownSilentDrop FlowTeardownAction = 0x03
)

// String returns a readable name.
func (a FlowTeardownAction) String() string {
	switch a {
	case TeardownNone:
		return "NONE"
	case TeardownTCPRST:
		return "TCP_RST"
	case TeardownICMPv6:
		return "ICMPv6"
	case TeardownSilentDrop:
		return "SILENT_DROP"
	default:
		return fmt.Sprintf("FlowTeardownAction(0x%02x)", uint8(a))
	}
}

// TeardownActionForMode returns the default teardown action for a verification mode.
func TeardownActionForMode(mode VerificationMode) FlowTeardownAction {
	switch mode {
	case ModePessimistic:
		return TeardownTCPRST
	case ModeOptimistic:
		return TeardownNone
	default:
		return TeardownNone
	}
}

// VerificationResult represents the outcome of PQC verification.
type VerificationResult struct {
	Passed        bool
	Tier          ComplianceTier
	AlgoID        uint8
	SigAge        time.Duration
	MultiSigCount int                // For SOVEREIGN: how many valid sigs
	FailReason    string             // Empty if passed
	Action        FlowTeardownAction
}

// IsAlgoAllowed checks if an algorithm ID is in the allowed list.
func (p *TierPolicy) IsAlgoAllowed(algoID uint8) bool {
	for _, a := range p.AllowedAlgos {
		if a == algoID {
			return true
		}
	}
	return false
}

// IsAlgoRequired checks if an algorithm ID is in the required list.
func (p *TierPolicy) IsAlgoRequired(algoID uint8) bool {
	for _, a := range p.RequiredAlgos {
		if a == algoID {
			return true
		}
	}
	return false
}

// IsSigExpired checks if a signature has exceeded MaxSigAge.
func (p *TierPolicy) IsSigExpired(sigCreatedAt time.Time) bool {
	return time.Since(sigCreatedAt) > p.MaxSigAge
}

// IsInGracePeriod checks if we're within the grace period after key rotation.
func (p *TierPolicy) IsInGracePeriod(rotationTime time.Time) bool {
	return time.Since(rotationTime) <= p.GracePeriod
}

// Validate checks if a verification result satisfies this policy.
func (p *TierPolicy) Validate(result *VerificationResult) error {
	if result == nil {
		return fmt.Errorf("nil verification result")
	}

	// Check that the algorithm is allowed.
	if result.AlgoID != 0 && !p.IsAlgoAllowed(result.AlgoID) {
		return fmt.Errorf("algorithm 0x%02x is not allowed by %s policy", result.AlgoID, p.Tier)
	}

	// Check signature age.
	if result.SigAge > p.MaxSigAge {
		return fmt.Errorf("signature age %s exceeds maximum %s for %s policy", result.SigAge, p.MaxSigAge, p.Tier)
	}

	// Check required algorithms — at least one must match if the list is non-empty.
	if len(p.RequiredAlgos) > 0 {
		found := false
		for _, req := range p.RequiredAlgos {
			if result.AlgoID == req {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("algorithm 0x%02x does not satisfy required algorithms for %s policy", result.AlgoID, p.Tier)
		}
	}

	// Check multi-sig requirements for SOVEREIGN tier.
	if p.RequireMultiSig {
		if result.MultiSigCount < p.MinMultiSigCount {
			return fmt.Errorf("multi-sig count %d below minimum %d for %s policy", result.MultiSigCount, p.MinMultiSigCount, p.Tier)
		}
	}

	// If the result itself reports failure, honour it.
	if !result.Passed {
		reason := result.FailReason
		if reason == "" {
			reason = "unknown"
		}
		return fmt.Errorf("verification failed: %s", reason)
	}

	return nil
}
