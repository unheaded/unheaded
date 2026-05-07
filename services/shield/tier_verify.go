// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package shield

import (
	"fmt"
	"sync"
	"time"

	"unheaded/pkg/audit"
	"unheaded/pkg/policy"
	"unheaded/pkg/wotan"
)

// TierVerifyResult embeds PQCVerifyResult with tier, policy, and sovereign
// information for the orchestrated verification pipeline.
type TierVerifyResult struct {
	PQCResult       *PQCVerifyResult
	SovereignResult *SovereignResult
	Tier            policy.ComplianceTier
	PolicyMode      policy.VerificationMode
	TierPolicy      *policy.TierPolicy
	Skipped         bool
	SkipReason      string
	Exempt          bool
}

// Passed returns true if the verification passed or was legitimately skipped.
func (r *TierVerifyResult) Passed() bool {
	if r.Exempt || r.Skipped {
		return true
	}
	if r.SovereignResult != nil {
		return r.SovereignResult.Passed
	}
	if r.PQCResult != nil {
		return r.PQCResult.Passed
	}
	return false
}

// DowngradeCallback is invoked when a tier transition lowers the security level.
type DowngradeCallback func(from, to policy.ComplianceTier)

// TierVerifier orchestrates PQC verification based on compliance tiers.
// It dispatches packets to the correct verification path (skip, PQCVerifier,
// or SovereignVerifier) depending on the active tier and per-service policy.
type TierVerifier struct {
	mu sync.RWMutex

	verifier  *PQCVerifier
	sovereign *SovereignVerifier
	policyEng *Layer2PolicyEngine
	stateMgr  *wotan.PQCStateManager

	activeTier  policy.ComplianceTier
	onDowngrade DowngradeCallback
}

// NewTierVerifier creates a new tier-based verification orchestrator.
func NewTierVerifier(
	verifier *PQCVerifier,
	sovereign *SovereignVerifier,
	policyEng *Layer2PolicyEngine,
	stateMgr *wotan.PQCStateManager,
) *TierVerifier {
	return &TierVerifier{
		verifier:   verifier,
		sovereign:  sovereign,
		policyEng:  policyEng,
		stateMgr:   stateMgr,
		activeTier: policy.TierStandard,
	}
}

// SetDowngradeCallback registers a callback for tier downgrades.
func (tv *TierVerifier) SetDowngradeCallback(cb DowngradeCallback) {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	tv.onDowngrade = cb
}

// ActiveTier returns the current compliance tier.
func (tv *TierVerifier) ActiveTier() policy.ComplianceTier {
	tv.mu.RLock()
	defer tv.mu.RUnlock()
	return tv.activeTier
}

// VerifyPacket dispatches to the correct verification path based on the
// active tier and per-service policy.
//
// Dispatch logic:
//   - NONE / OPTIMISTIC: skip verification, emit warning
//   - STANDARD / ENHANCED: PQCVerifier.Verify()
//   - SOVEREIGN: SovereignVerifier.VerifySovereign()
func (tv *TierVerifier) VerifyPacket(pkt *PQCPacketInfo, sovereignSig *SovereignSig) *TierVerifyResult {
	tv.mu.RLock()
	tier := tv.activeTier
	tv.mu.RUnlock()

	tierPol := policy.DefaultPolicyForTier(tier)
	result := &TierVerifyResult{
		Tier:       tier,
		PolicyMode: tierPol.Mode,
		TierPolicy: tierPol,
	}

	// Check per-service exemption.
	if tv.policyEng != nil && tv.policyEng.IsExempt(pkt.DstSvcID) {
		result.Exempt = true
		result.SkipReason = fmt.Sprintf("service %d is exempt", pkt.DstSvcID)
		return result
	}

	// Check per-service policy override.
	if tv.policyEng != nil {
		if appPol, ok := tv.policyEng.GetPolicy(pkt.DstSvcID); ok {
			if appPol.RequiredTier > uint8(tier) {
				tier = policy.ComplianceTier(appPol.RequiredTier)
				tierPol = policy.DefaultPolicyForTier(tier)
				result.Tier = tier
				result.PolicyMode = tierPol.Mode
				result.TierPolicy = tierPol
			}
		}
	}

	switch tier {
	case policy.TierNone:
		result.Skipped = true
		result.SkipReason = "tier NONE: verification skipped"
		tv.updateState(result)
		return result

	case policy.TierStandard, policy.TierEnhanced:
		pqcResult := tv.verifier.Verify(pkt)
		result.PQCResult = pqcResult
		tv.updateState(result)
		return result

	case policy.TierSovereign:
		if sovereignSig == nil {
			// Fall back to standard verification if no sovereign sig provided.
			pqcResult := tv.verifier.Verify(pkt)
			result.PQCResult = pqcResult
			if pqcResult.Passed {
				// Single sig passed but sovereign requires multi-sig.
				result.PQCResult = &PQCVerifyResult{
					FailStep: StepAlgoCompliance,
					FailMsg:  "SOVEREIGN tier requires multi-sig, only single sig provided",
					SigRef:   pkt.SigRef,
					KeyRef:   pkt.KeyRef,
					SeqNum:   pkt.SeqNum,
				}
			}
			tv.updateState(result)
			return result
		}
		sovResult := tv.sovereign.VerifySovereign(pkt, sovereignSig)
		result.SovereignResult = sovResult
		tv.updateState(result)
		return result

	default:
		result.Skipped = true
		result.SkipReason = fmt.Sprintf("unknown tier %d: verification skipped", tier)
		tv.updateState(result)
		return result
	}
}

// TransitionTier changes the active compliance tier. If the new tier is lower
// than the current tier, the downgrade callback is invoked and an audit event
// is generated.
func (tv *TierVerifier) TransitionTier(newTier policy.ComplianceTier) *audit.PQCEvent {
	tv.mu.Lock()
	oldTier := tv.activeTier
	tv.activeTier = newTier
	cb := tv.onDowngrade
	tv.mu.Unlock()

	// Update Wotan state.
	if tv.stateMgr != nil {
		tv.stateMgr.SetActiveTier(uint8(newTier))
		if newTier >= policy.TierEnhanced {
			tv.stateMgr.SetPolicyMode(uint8(policy.ModePessimistic))
		} else {
			tv.stateMgr.SetPolicyMode(uint8(policy.ModeOptimistic))
		}
	}

	// Generate audit event.
	evt := audit.NewPQCEvent(audit.PQCTierChange).
		WithTier(uint8(newTier)).
		WithDetail("old_tier", oldTier.String()).
		WithDetail("new_tier", newTier.String())

	// Invoke downgrade callback if applicable.
	if newTier < oldTier && cb != nil {
		evt.WithDetail("downgrade", true)
		cb(oldTier, newTier)
	}

	return evt
}

// updateState syncs verification results to Wotan state.
func (tv *TierVerifier) updateState(result *TierVerifyResult) {
	if tv.stateMgr == nil {
		return
	}

	if result.Passed() {
		tv.stateMgr.IncrVerifyPass()
	} else {
		tv.stateMgr.IncrVerifyFail()
	}

	if result.PQCResult != nil {
		s := tv.stateMgr.Read()
		s.LastVerifyNs = uint32(result.PQCResult.Duration.Nanoseconds())
		if result.PQCResult.SeqNum > uint16(s.SeqHighWater) {
			s.SeqHighWater = uint32(result.PQCResult.SeqNum)
		}
		tv.stateMgr.Write(s)
	}
}

// TeardownAction returns the appropriate flow teardown action when
// verification fails, based on the current tier's policy mode.
func (tv *TierVerifier) TeardownAction() policy.FlowTeardownAction {
	tv.mu.RLock()
	tier := tv.activeTier
	tv.mu.RUnlock()

	tierPol := policy.DefaultPolicyForTier(tier)
	return policy.TeardownActionForMode(tierPol.Mode)
}

// Stats returns a snapshot of verification statistics.
func (tv *TierVerifier) Stats() map[string]interface{} {
	tv.mu.RLock()
	tier := tv.activeTier
	tv.mu.RUnlock()

	stats := map[string]interface{}{
		"active_tier": tier.String(),
		"verify_pass": tv.verifier.VerifyPassCount(),
		"verify_fail": tv.verifier.VerifyFailCount(),
	}

	if tv.stateMgr != nil {
		s := tv.stateMgr.Read()
		stats["last_verify_ns"] = s.LastVerifyNs
		stats["seq_high_water"] = s.SeqHighWater
		stats["policy_mode"] = policy.VerificationMode(s.PolicyMode).String()
		stats["updated_at"] = time.Now().Unix()
	}

	return stats
}
