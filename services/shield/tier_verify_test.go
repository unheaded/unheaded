// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package shield

import (
	"testing"

	"unheaded/pkg/policy"
	"unheaded/pkg/wotan"
)

func TestTierVerifier_NONE_Skips(t *testing.T) {
	v := NewPQCVerifier()
	sv := NewSovereignVerifier(v)
	pe := NewLayer2PolicyEngine()
	sm := wotan.NewPQCStateManager()

	tv := NewTierVerifier(v, sv, pe, sm)
	tv.TransitionTier(policy.TierNone)

	pkt := &PQCPacketInfo{
		Flags:    0x0A,
		SigRef:   1,
		KeyRef:   1,
		SeqNum:   1,
		SrcSvcID: 10,
		DstSvcID: 11,
	}

	result := tv.VerifyPacket(pkt, nil)
	if !result.Passed() {
		t.Fatal("NONE tier should skip and pass")
	}
	if !result.Skipped {
		t.Fatal("NONE tier should be marked as skipped")
	}
}

func TestTierVerifier_STANDARD_Verifies(t *testing.T) {
	v := NewPQCVerifier()
	sv := NewSovereignVerifier(v)
	pe := NewLayer2PolicyEngine()
	sm := wotan.NewPQCStateManager()

	tv := NewTierVerifier(v, sv, pe, sm)
	tv.TransitionTier(policy.TierStandard)

	pkt := &PQCPacketInfo{
		Flags:    0x0A,
		SigRef:   1,
		KeyRef:   1,
		SeqNum:   1,
		SrcSvcID: 10,
		DstSvcID: 11,
	}

	// Without loading sig/key, verification will fail.
	result := tv.VerifyPacket(pkt, nil)
	if result.Passed() {
		t.Fatal("STANDARD tier should fail without loaded sig/key")
	}
	if result.PQCResult == nil {
		t.Fatal("STANDARD tier should produce PQCResult")
	}
}

func TestTierVerifier_SOVEREIGN_RequiresMultiSig(t *testing.T) {
	v := NewPQCVerifier()
	sv := NewSovereignVerifier(v)
	pe := NewLayer2PolicyEngine()
	sm := wotan.NewPQCStateManager()

	tv := NewTierVerifier(v, sv, pe, sm)
	tv.TransitionTier(policy.TierSovereign)

	pkt := &PQCPacketInfo{
		Flags:    0x0A,
		SigRef:   1,
		KeyRef:   1,
		SeqNum:   1,
		SrcSvcID: 10,
		DstSvcID: 11,
	}

	// Without sovereign sig, should fail.
	result := tv.VerifyPacket(pkt, nil)
	if result.Passed() {
		t.Fatal("SOVEREIGN without multi-sig should fail")
	}
}

func TestTierVerifier_ExemptService(t *testing.T) {
	v := NewPQCVerifier()
	sv := NewSovereignVerifier(v)
	pe := NewLayer2PolicyEngine()
	sm := wotan.NewPQCStateManager()

	pe.SetPolicy(&AppPQCPolicy{
		ServiceID: 99,
		Exempt:    true,
	})

	tv := NewTierVerifier(v, sv, pe, sm)
	tv.TransitionTier(policy.TierEnhanced)

	pkt := &PQCPacketInfo{
		Flags:    0x0A,
		SigRef:   1,
		KeyRef:   1,
		SeqNum:   1,
		SrcSvcID: 10,
		DstSvcID: 99,
	}

	result := tv.VerifyPacket(pkt, nil)
	if !result.Passed() {
		t.Fatal("exempt service should pass")
	}
	if !result.Exempt {
		t.Fatal("exempt service should be marked exempt")
	}
}

func TestTierVerifier_TransitionTier_Downgrade(t *testing.T) {
	v := NewPQCVerifier()
	sv := NewSovereignVerifier(v)
	pe := NewLayer2PolicyEngine()
	sm := wotan.NewPQCStateManager()

	tv := NewTierVerifier(v, sv, pe, sm)
	tv.TransitionTier(policy.TierSovereign)

	var downgradeFrom, downgradeTo policy.ComplianceTier
	tv.SetDowngradeCallback(func(from, to policy.ComplianceTier) {
		downgradeFrom = from
		downgradeTo = to
	})

	// Downgrade from SOVEREIGN to STANDARD.
	evt := tv.TransitionTier(policy.TierStandard)
	if downgradeFrom != policy.TierSovereign {
		t.Fatalf("expected downgrade from SOVEREIGN, got %s", downgradeFrom)
	}
	if downgradeTo != policy.TierStandard {
		t.Fatalf("expected downgrade to STANDARD, got %s", downgradeTo)
	}
	if evt == nil {
		t.Fatal("expected audit event on transition")
	}
	if evt.Tier != uint8(policy.TierStandard) {
		t.Fatalf("audit event tier = %d, want %d", evt.Tier, policy.TierStandard)
	}

	// Verify Wotan state updated.
	s := sm.Read()
	if s.ActiveTier != uint8(policy.TierStandard) {
		t.Fatalf("Wotan active tier = %d, want %d", s.ActiveTier, policy.TierStandard)
	}
}

func TestTierVerifier_Stats(t *testing.T) {
	v := NewPQCVerifier()
	sv := NewSovereignVerifier(v)
	pe := NewLayer2PolicyEngine()
	sm := wotan.NewPQCStateManager()

	tv := NewTierVerifier(v, sv, pe, sm)

	stats := tv.Stats()
	if stats["active_tier"] != "STANDARD" {
		t.Fatalf("expected default tier STANDARD, got %v", stats["active_tier"])
	}
}

func TestTierVerifier_PolicyOverride(t *testing.T) {
	v := NewPQCVerifier()
	sv := NewSovereignVerifier(v)
	pe := NewLayer2PolicyEngine()
	sm := wotan.NewPQCStateManager()

	// Service 42 requires ENHANCED even when global is STANDARD.
	pe.SetPolicy(&AppPQCPolicy{
		ServiceID:    42,
		RequiredTier: uint8(policy.TierEnhanced),
	})

	tv := NewTierVerifier(v, sv, pe, sm)
	tv.TransitionTier(policy.TierStandard)

	pkt := &PQCPacketInfo{
		Flags:    0x0A,
		SigRef:   1,
		KeyRef:   1,
		SeqNum:   1,
		SrcSvcID: 10,
		DstSvcID: 42,
	}

	result := tv.VerifyPacket(pkt, nil)
	if result.Tier != policy.TierEnhanced {
		t.Fatalf("expected ENHANCED tier override, got %s", result.Tier)
	}
}

func TestTierVerifier_TeardownAction(t *testing.T) {
	v := NewPQCVerifier()
	sv := NewSovereignVerifier(v)
	pe := NewLayer2PolicyEngine()
	sm := wotan.NewPQCStateManager()

	tv := NewTierVerifier(v, sv, pe, sm)

	// STANDARD uses optimistic -> no teardown.
	tv.TransitionTier(policy.TierStandard)
	if tv.TeardownAction() != policy.TeardownNone {
		t.Fatal("STANDARD should have no teardown action")
	}

	// ENHANCED uses pessimistic -> TCP RST.
	tv.TransitionTier(policy.TierEnhanced)
	if tv.TeardownAction() != policy.TeardownTCPRST {
		t.Fatal("ENHANCED should have TCP_RST teardown action")
	}
}
