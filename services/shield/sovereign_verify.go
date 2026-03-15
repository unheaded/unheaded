// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package shield

import (
	"fmt"
	"sync"

	"unheaded/pkg/sophia"
)

// AlgorithmFamily groups algorithms by cryptographic construction.
const (
	FamilyHashBased  = "hash-based"  // SLH-DSA
	FamilyLattice    = "lattice"     // ML-DSA, FN-DSA
	FamilyCodeBased  = "code-based"  // HQC
)

// AlgoFamily returns the algorithm family for a given AlgoID.
func AlgoFamily(algoID uint8) string {
	switch algoID {
	case 0x01:
		return FamilyHashBased
	case 0x02, 0x03:
		return FamilyLattice
	case 0x05:
		return FamilyCodeBased
	default:
		return "unknown"
	}
}

// SovereignSig holds 3 cross-algorithm signatures for SOVEREIGN tier.
type SovereignSig struct {
	Sigs [3]SovereignSigSlot
}

// SovereignSigSlot is one of the 3 signature slots.
type SovereignSigSlot struct {
	SigRef  uint32
	AlgoID  uint8
	Valid   bool
	Checked bool
}

// SovereignVerifier validates SOVEREIGN tier 2-of-3 multi-sig.
// Requires signatures from DISTINCT algorithm families.
type SovereignVerifier struct {
	mu       sync.RWMutex
	verifier *PQCVerifier
}

// NewSovereignVerifier creates a new sovereign verifier.
func NewSovereignVerifier(verifier *PQCVerifier) *SovereignVerifier {
	return &SovereignVerifier{
		verifier: verifier,
	}
}

// SovereignResult holds the multi-sig verification result.
type SovereignResult struct {
	Passed          bool
	ValidCount      int
	TotalChecked    int
	DistinctFamilies int
	FailReason      string
	SlotResults     [3]SlotResult
}

// SlotResult is the result for one signature slot.
type SlotResult struct {
	SigRef uint32
	AlgoID uint8
	Family string
	Valid  bool
	Error  string
}

// VerifySovereign validates 2-of-3 cross-algorithm multi-sig.
func (sv *SovereignVerifier) VerifySovereign(pkt *PQCPacketInfo, sov *SovereignSig) *SovereignResult {
	result := &SovereignResult{}

	familiesSeen := make(map[string]bool)

	for i, slot := range sov.Sigs {
		if slot.SigRef == 0 {
			continue
		}

		result.TotalChecked++
		sr := SlotResult{
			SigRef: slot.SigRef,
			AlgoID: slot.AlgoID,
			Family: AlgoFamily(slot.AlgoID),
		}

		// Create a modified packet for this sig
		slotPkt := *pkt
		slotPkt.SigRef = slot.SigRef

		verifyResult := sv.verifier.Verify(&slotPkt)
		if verifyResult.Passed {
			sr.Valid = true
			result.ValidCount++
			familiesSeen[sr.Family] = true
		} else {
			sr.Error = verifyResult.FailMsg
		}

		result.SlotResults[i] = sr
	}

	result.DistinctFamilies = len(familiesSeen)

	// SOVEREIGN requires 2-of-3 valid signatures from distinct families
	if result.ValidCount >= 2 && result.DistinctFamilies >= 2 {
		result.Passed = true
	} else if result.ValidCount < 2 {
		result.FailReason = fmt.Sprintf("insufficient valid signatures: %d/3 (need 2)", result.ValidCount)
	} else {
		result.FailReason = fmt.Sprintf("insufficient distinct families: %d (need 2)", result.DistinctFamilies)
	}

	return result
}

// PrecomputeSovereignSigs pre-signs a pseudo-header with multiple signers
// and stores the results in Sophia maps. This is used in the control plane
// to prepare sovereign signatures before data plane transmission.
func (sv *SovereignVerifier) PrecomputeSovereignSigs(
	pseudoHeader []byte,
	signers []SovereignSigner,
	mapMgr *sophia.PQCMapManager,
) (*sophia.SovereignSigEntry, error) {
	if len(signers) < 2 || len(signers) > 3 {
		return nil, fmt.Errorf("shield: sovereign pre-compute requires 2-3 signers, got %d", len(signers))
	}

	// Validate family diversity before crypto work (fail-fast).
	families := make(map[string]bool)
	for _, s := range signers {
		families[AlgoFamily(s.AlgoID)] = true
	}
	if len(families) < 2 {
		return nil, fmt.Errorf("shield: sovereign pre-compute requires at least 2 distinct families, got %d", len(families))
	}

	entry := &sophia.SovereignSigEntry{}

	for i, signer := range signers {
		sig, err := signer.Sign(pseudoHeader)
		if err != nil {
			return nil, fmt.Errorf("shield: sovereign pre-compute signer %d: %w", i, err)
		}

		sigEntry := &sophia.PQCSigEntry{
			AlgoID:    signer.AlgoID,
			ParamSet:  signer.ParamSet,
			Tier:      0x03, // SOVEREIGN
			Status:    0x01, // active
			SigLen:    uint32(len(sig)),
			Signature: sig,
		}
		sigRef, err := mapMgr.LoadSignature(sigEntry)
		if err != nil {
			return nil, fmt.Errorf("shield: sovereign pre-compute store sig %d: %w", i, err)
		}

		switch i {
		case 0:
			entry.SigRef1, entry.AlgoID1, entry.Status1 = sigRef, signer.AlgoID, 0x00
		case 1:
			entry.SigRef2, entry.AlgoID2, entry.Status2 = sigRef, signer.AlgoID, 0x00
		case 2:
			entry.SigRef3, entry.AlgoID3, entry.Status3 = sigRef, signer.AlgoID, 0x00
		}
	}

	return entry, nil
}

// SovereignSigner provides the data needed to sign with one of the 3 slots.
type SovereignSigner struct {
	AlgoID   uint8
	ParamSet uint8
	Sign     func(message []byte) ([]byte, error)
}

// VerifySovereignFromSophia loads a sovereign signature entry from Sophia
// maps and verifies it. This is the data plane path where the entry was
// previously stored by PrecomputeSovereignSigs.
func (sv *SovereignVerifier) VerifySovereignFromSophia(
	pkt *PQCPacketInfo,
	mapMgr *sophia.PQCMapManager,
	sovSigRef uint32,
) (*SovereignResult, error) {
	entry, err := mapMgr.LookupSovereignSig(sovSigRef)
	if err != nil {
		return nil, fmt.Errorf("shield: sovereign lookup: %w", err)
	}

	// Build SovereignSig from Sophia entry.
	sov := &SovereignSig{
		Sigs: [3]SovereignSigSlot{
			{SigRef: entry.SigRef1, AlgoID: entry.AlgoID1},
			{SigRef: entry.SigRef2, AlgoID: entry.AlgoID2},
			{SigRef: entry.SigRef3, AlgoID: entry.AlgoID3},
		},
	}

	result := sv.VerifySovereign(pkt, sov)

	// Update Sophia entry with verification results.
	updateStatus := func(passed bool) uint8 {
		if passed {
			return 0x01
		}
		return 0x02
	}
	entry.Status1 = updateStatus(result.SlotResults[0].Valid)
	entry.Status2 = updateStatus(result.SlotResults[1].Valid)
	entry.Status3 = updateStatus(result.SlotResults[2].Valid)
	entry.Consensus = sophia.EvaluateConsensus(entry)

	if storeErr := mapMgr.LoadSovereignSig(sovSigRef, entry); storeErr != nil {
		return result, fmt.Errorf("shield: sovereign update status: %w", storeErr)
	}

	return result, nil
}
