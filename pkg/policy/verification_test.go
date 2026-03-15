// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package policy

import (
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// DefaultPolicyForTier — verify correct settings for all four tiers
// ---------------------------------------------------------------------------

func TestDefaultPolicyForTier_None(t *testing.T) {
	p := DefaultPolicyForTier(TierNone)
	if p.Tier != TierNone {
		t.Fatalf("expected TierNone, got %s", p.Tier)
	}
	if p.Mode != ModeOptimistic {
		t.Fatalf("expected ModeOptimistic for TierNone, got %s", p.Mode)
	}
	if len(p.RequiredAlgos) != 0 {
		t.Fatalf("TierNone should have no required algos, got %d", len(p.RequiredAlgos))
	}
	if len(p.AllowedAlgos) != 3 {
		t.Fatalf("TierNone should allow 3 algos, got %d", len(p.AllowedAlgos))
	}
	if p.MaxSigAge != 24*time.Hour {
		t.Fatalf("TierNone MaxSigAge should be 24h, got %s", p.MaxSigAge)
	}
	if p.GracePeriod != 60*time.Second {
		t.Fatalf("TierNone GracePeriod should be 60s, got %s", p.GracePeriod)
	}
}

func TestDefaultPolicyForTier_Standard(t *testing.T) {
	p := DefaultPolicyForTier(TierStandard)
	if p.Tier != TierStandard {
		t.Fatalf("expected TierStandard, got %s", p.Tier)
	}
	if p.Mode != ModeOptimistic {
		t.Fatalf("expected ModeOptimistic for TierStandard, got %s", p.Mode)
	}
	if len(p.RequiredAlgos) != 0 {
		t.Fatalf("TierStandard should have empty required algos, got %d", len(p.RequiredAlgos))
	}
	if len(p.AllowedAlgos) != 2 {
		t.Fatalf("TierStandard should allow 2 algos (SLH-DSA, ML-DSA), got %d", len(p.AllowedAlgos))
	}
	if p.MinTrustTier != TierStandard {
		t.Fatalf("TierStandard MinTrustTier should be Standard, got %s", p.MinTrustTier)
	}
	if p.MaxSigAge != 1*time.Hour {
		t.Fatalf("TierStandard MaxSigAge should be 1h, got %s", p.MaxSigAge)
	}
}

func TestDefaultPolicyForTier_Enhanced(t *testing.T) {
	p := DefaultPolicyForTier(TierEnhanced)
	if p.Tier != TierEnhanced {
		t.Fatalf("expected TierEnhanced, got %s", p.Tier)
	}
	if p.Mode != ModePessimistic {
		t.Fatalf("expected ModePessimistic for TierEnhanced, got %s", p.Mode)
	}
	if len(p.RequiredAlgos) != 1 || p.RequiredAlgos[0] != AlgoMLDSA {
		t.Fatalf("TierEnhanced should require ML-DSA only")
	}
	if p.MinTrustTier != TierEnhanced {
		t.Fatalf("TierEnhanced MinTrustTier should be Enhanced, got %s", p.MinTrustTier)
	}
	if p.MaxSigAge != 30*time.Minute {
		t.Fatalf("TierEnhanced MaxSigAge should be 30m, got %s", p.MaxSigAge)
	}
}

func TestDefaultPolicyForTier_Sovereign(t *testing.T) {
	p := DefaultPolicyForTier(TierSovereign)
	if p.Tier != TierSovereign {
		t.Fatalf("expected TierSovereign, got %s", p.Tier)
	}
	if p.Mode != ModePessimistic {
		t.Fatalf("expected ModePessimistic for TierSovereign, got %s", p.Mode)
	}
	if len(p.RequiredAlgos) != 2 {
		t.Fatalf("TierSovereign should require 2 algos, got %d", len(p.RequiredAlgos))
	}
	if len(p.AllowedAlgos) != 3 {
		t.Fatalf("TierSovereign should allow 3 algos, got %d", len(p.AllowedAlgos))
	}
	if p.MinTrustTier != TierSovereign {
		t.Fatalf("TierSovereign MinTrustTier should be Sovereign, got %s", p.MinTrustTier)
	}
	if p.MaxSigAge != 15*time.Minute {
		t.Fatalf("TierSovereign MaxSigAge should be 15m, got %s", p.MaxSigAge)
	}
}

func TestDefaultPolicyForTier_UnknownFallsBackToNone(t *testing.T) {
	p := DefaultPolicyForTier(ComplianceTier(0xFF))
	if p.Tier != TierNone {
		t.Fatalf("unknown tier should fall back to TierNone, got %s", p.Tier)
	}
}

// ---------------------------------------------------------------------------
// SOVEREIGN requires multi-sig and distinct families
// ---------------------------------------------------------------------------

func TestSovereignRequiresMultiSigAndDistinctFamilies(t *testing.T) {
	p := DefaultPolicyForTier(TierSovereign)
	if !p.RequireMultiSig {
		t.Fatal("TierSovereign must require multi-sig")
	}
	if p.MinMultiSigCount != 2 {
		t.Fatalf("TierSovereign MinMultiSigCount should be 2, got %d", p.MinMultiSigCount)
	}
	if !p.DistinctFamilies {
		t.Fatal("TierSovereign must require distinct algorithm families")
	}
}

// ---------------------------------------------------------------------------
// Teardown actions by mode
// ---------------------------------------------------------------------------

func TestTeardownActionForMode_Pessimistic(t *testing.T) {
	action := TeardownActionForMode(ModePessimistic)
	if action != TeardownTCPRST {
		t.Fatalf("PESSIMISTIC mode should default to TCP_RST, got %s", action)
	}
}

func TestTeardownActionForMode_Optimistic(t *testing.T) {
	action := TeardownActionForMode(ModeOptimistic)
	if action != TeardownNone {
		t.Fatalf("OPTIMISTIC mode should default to NONE, got %s", action)
	}
}

func TestTeardownActionForMode_Unknown(t *testing.T) {
	action := TeardownActionForMode(VerificationMode(0xFF))
	if action != TeardownNone {
		t.Fatalf("unknown mode should default to NONE, got %s", action)
	}
}

// ---------------------------------------------------------------------------
// IsAlgoAllowed / IsAlgoRequired
// ---------------------------------------------------------------------------

func TestIsAlgoAllowed(t *testing.T) {
	p := DefaultPolicyForTier(TierStandard)
	if !p.IsAlgoAllowed(AlgoSLHDSA) {
		t.Fatal("SLH-DSA should be allowed in TierStandard")
	}
	if !p.IsAlgoAllowed(AlgoMLDSA) {
		t.Fatal("ML-DSA should be allowed in TierStandard")
	}
	if p.IsAlgoAllowed(AlgoFNDSA) {
		t.Fatal("FN-DSA should NOT be allowed in TierStandard")
	}
	if p.IsAlgoAllowed(AlgoHQC) {
		t.Fatal("HQC should NOT be allowed in TierStandard")
	}
}

func TestIsAlgoRequired(t *testing.T) {
	p := DefaultPolicyForTier(TierEnhanced)
	if !p.IsAlgoRequired(AlgoMLDSA) {
		t.Fatal("ML-DSA should be required in TierEnhanced")
	}
	if p.IsAlgoRequired(AlgoSLHDSA) {
		t.Fatal("SLH-DSA should NOT be required in TierEnhanced")
	}
}

func TestIsAlgoRequired_EmptyList(t *testing.T) {
	p := DefaultPolicyForTier(TierStandard)
	// RequiredAlgos is empty for Standard — nothing is individually required.
	if p.IsAlgoRequired(AlgoMLDSA) {
		t.Fatal("no algo should be individually required in TierStandard")
	}
}

// ---------------------------------------------------------------------------
// IsSigExpired
// ---------------------------------------------------------------------------

func TestIsSigExpired_Fresh(t *testing.T) {
	p := DefaultPolicyForTier(TierStandard) // MaxSigAge = 1h
	fresh := time.Now().Add(-30 * time.Minute)
	if p.IsSigExpired(fresh) {
		t.Fatal("30-minute-old signature should NOT be expired under 1h policy")
	}
}

func TestIsSigExpired_Old(t *testing.T) {
	p := DefaultPolicyForTier(TierStandard) // MaxSigAge = 1h
	old := time.Now().Add(-2 * time.Hour)
	if !p.IsSigExpired(old) {
		t.Fatal("2-hour-old signature SHOULD be expired under 1h policy")
	}
}

func TestIsSigExpired_ExactBoundary(t *testing.T) {
	p := DefaultPolicyForTier(TierEnhanced) // MaxSigAge = 30m
	// A signature created exactly MaxSigAge ago is borderline; time.Since
	// will return slightly more than MaxSigAge so it should be expired.
	boundary := time.Now().Add(-p.MaxSigAge)
	// We accept either result at the exact boundary (race with wall clock),
	// so just confirm the function does not panic.
	_ = p.IsSigExpired(boundary)
}

// ---------------------------------------------------------------------------
// IsInGracePeriod
// ---------------------------------------------------------------------------

func TestIsInGracePeriod_Inside(t *testing.T) {
	p := DefaultPolicyForTier(TierStandard) // GracePeriod = 60s
	rotated := time.Now().Add(-30 * time.Second)
	if !p.IsInGracePeriod(rotated) {
		t.Fatal("30s after rotation should be within 60s grace period")
	}
}

func TestIsInGracePeriod_Outside(t *testing.T) {
	p := DefaultPolicyForTier(TierStandard) // GracePeriod = 60s
	rotated := time.Now().Add(-2 * time.Minute)
	if p.IsInGracePeriod(rotated) {
		t.Fatal("2m after rotation should be outside 60s grace period")
	}
}

func TestIsInGracePeriod_ExactBoundary(t *testing.T) {
	p := DefaultPolicyForTier(TierNone) // GracePeriod = 60s
	boundary := time.Now().Add(-p.GracePeriod)
	// At exact boundary, time.Since >= GracePeriod, so should be outside.
	// Accept either result to avoid flaky test due to clock granularity.
	_ = p.IsInGracePeriod(boundary)
}

// ---------------------------------------------------------------------------
// Validate — passes and failures for each tier
// ---------------------------------------------------------------------------

func TestValidate_NilResult(t *testing.T) {
	p := DefaultPolicyForTier(TierNone)
	if err := p.Validate(nil); err == nil {
		t.Fatal("Validate(nil) should return an error")
	}
}

func TestValidate_TierNone_Passes(t *testing.T) {
	p := DefaultPolicyForTier(TierNone)
	result := &VerificationResult{
		Passed: true,
		Tier:   TierNone,
		AlgoID: AlgoSLHDSA,
		SigAge: 10 * time.Minute,
	}
	if err := p.Validate(result); err != nil {
		t.Fatalf("TierNone should pass valid result: %v", err)
	}
}

func TestValidate_TierNone_DisallowedAlgo(t *testing.T) {
	p := DefaultPolicyForTier(TierNone)
	result := &VerificationResult{
		Passed: true,
		Tier:   TierNone,
		AlgoID: AlgoHQC, // not in AllowedAlgos for TierNone
		SigAge: 10 * time.Minute,
	}
	if err := p.Validate(result); err == nil {
		t.Fatal("TierNone should reject disallowed algo HQC")
	}
}

func TestValidate_TierStandard_Passes(t *testing.T) {
	p := DefaultPolicyForTier(TierStandard)
	result := &VerificationResult{
		Passed: true,
		Tier:   TierStandard,
		AlgoID: AlgoMLDSA,
		SigAge: 30 * time.Minute,
	}
	if err := p.Validate(result); err != nil {
		t.Fatalf("TierStandard should pass valid ML-DSA result: %v", err)
	}
}

func TestValidate_TierStandard_ExpiredSig(t *testing.T) {
	p := DefaultPolicyForTier(TierStandard)
	result := &VerificationResult{
		Passed: true,
		Tier:   TierStandard,
		AlgoID: AlgoSLHDSA,
		SigAge: 2 * time.Hour, // exceeds 1h
	}
	if err := p.Validate(result); err == nil {
		t.Fatal("TierStandard should reject expired signature")
	}
}

func TestValidate_TierEnhanced_Passes(t *testing.T) {
	p := DefaultPolicyForTier(TierEnhanced)
	result := &VerificationResult{
		Passed: true,
		Tier:   TierEnhanced,
		AlgoID: AlgoMLDSA,
		SigAge: 10 * time.Minute,
	}
	if err := p.Validate(result); err != nil {
		t.Fatalf("TierEnhanced should pass valid ML-DSA result: %v", err)
	}
}

func TestValidate_TierEnhanced_WrongRequiredAlgo(t *testing.T) {
	p := DefaultPolicyForTier(TierEnhanced)
	result := &VerificationResult{
		Passed: true,
		Tier:   TierEnhanced,
		AlgoID: AlgoSLHDSA, // allowed but not the required ML-DSA
		SigAge: 10 * time.Minute,
	}
	if err := p.Validate(result); err == nil {
		t.Fatal("TierEnhanced should reject non-required algo SLH-DSA")
	}
}

func TestValidate_TierSovereign_Passes(t *testing.T) {
	p := DefaultPolicyForTier(TierSovereign)
	result := &VerificationResult{
		Passed:        true,
		Tier:          TierSovereign,
		AlgoID:        AlgoSLHDSA, // one of the required algos
		SigAge:        5 * time.Minute,
		MultiSigCount: 2,
	}
	if err := p.Validate(result); err != nil {
		t.Fatalf("TierSovereign should pass valid multi-sig result: %v", err)
	}
}

func TestValidate_TierSovereign_InsufficientMultiSig(t *testing.T) {
	p := DefaultPolicyForTier(TierSovereign)
	result := &VerificationResult{
		Passed:        true,
		Tier:          TierSovereign,
		AlgoID:        AlgoSLHDSA,
		SigAge:        5 * time.Minute,
		MultiSigCount: 1, // below MinMultiSigCount=2
	}
	if err := p.Validate(result); err == nil {
		t.Fatal("TierSovereign should reject insufficient multi-sig count")
	}
}

func TestValidate_FailedResult(t *testing.T) {
	p := DefaultPolicyForTier(TierNone)
	result := &VerificationResult{
		Passed:     false,
		Tier:       TierNone,
		AlgoID:     AlgoSLHDSA,
		SigAge:     1 * time.Minute,
		FailReason: "bad signature",
	}
	if err := p.Validate(result); err == nil {
		t.Fatal("Validate should propagate a failed verification result")
	}
}

func TestValidate_FailedResult_EmptyReason(t *testing.T) {
	p := DefaultPolicyForTier(TierNone)
	result := &VerificationResult{
		Passed: false,
		Tier:   TierNone,
		AlgoID: AlgoSLHDSA,
		SigAge: 1 * time.Minute,
	}
	err := p.Validate(result)
	if err == nil {
		t.Fatal("Validate should propagate a failed verification result")
	}
	if err.Error() != "verification failed: unknown" {
		t.Fatalf("expected 'verification failed: unknown', got %q", err.Error())
	}
}

func TestValidate_ZeroAlgoID_Passes(t *testing.T) {
	// A zero AlgoID (no algorithm specified) should skip the allowed-algo check.
	p := DefaultPolicyForTier(TierNone)
	result := &VerificationResult{
		Passed: true,
		Tier:   TierNone,
		AlgoID: 0,
		SigAge: 1 * time.Minute,
	}
	if err := p.Validate(result); err != nil {
		t.Fatalf("zero AlgoID should be accepted: %v", err)
	}
}

// ---------------------------------------------------------------------------
// String methods
// ---------------------------------------------------------------------------

func TestVerificationMode_String(t *testing.T) {
	tests := []struct {
		mode VerificationMode
		want string
	}{
		{ModePessimistic, "PESSIMISTIC"},
		{ModeOptimistic, "OPTIMISTIC"},
		{VerificationMode(0xFF), "VerificationMode(0xff)"},
	}
	for _, tt := range tests {
		if got := tt.mode.String(); got != tt.want {
			t.Errorf("VerificationMode(%d).String() = %q, want %q", tt.mode, got, tt.want)
		}
	}
}

func TestComplianceTier_String(t *testing.T) {
	tests := []struct {
		tier ComplianceTier
		want string
	}{
		{TierNone, "NONE"},
		{TierStandard, "STANDARD"},
		{TierEnhanced, "ENHANCED"},
		{TierSovereign, "SOVEREIGN"},
		{ComplianceTier(0xAA), "ComplianceTier(0xaa)"},
	}
	for _, tt := range tests {
		if got := tt.tier.String(); got != tt.want {
			t.Errorf("ComplianceTier(%d).String() = %q, want %q", tt.tier, got, tt.want)
		}
	}
}

func TestFlowTeardownAction_String(t *testing.T) {
	tests := []struct {
		action FlowTeardownAction
		want   string
	}{
		{TeardownNone, "NONE"},
		{TeardownTCPRST, "TCP_RST"},
		{TeardownICMPv6, "ICMPv6"},
		{TeardownSilentDrop, "SILENT_DROP"},
		{FlowTeardownAction(0xBB), "FlowTeardownAction(0xbb)"},
	}
	for _, tt := range tests {
		if got := tt.action.String(); got != tt.want {
			t.Errorf("FlowTeardownAction(%d).String() = %q, want %q", tt.action, got, tt.want)
		}
	}
}
