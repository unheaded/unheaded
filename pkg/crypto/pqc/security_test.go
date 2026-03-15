// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package pqc

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"testing"

	"unheaded/pkg/monad"
)

// --- Constant-Time Fingerprint Comparison ---

func TestConstantTimeCompareFingerprint_Equal(t *testing.T) {
	a := [4]byte{0xDE, 0xAD, 0xBE, 0xEF}
	b := [4]byte{0xDE, 0xAD, 0xBE, 0xEF}
	if !ConstantTimeCompareFingerprint(a, b) {
		t.Fatal("equal fingerprints should compare as true")
	}
}

func TestConstantTimeCompareFingerprint_Unequal(t *testing.T) {
	a := [4]byte{0xDE, 0xAD, 0xBE, 0xEF}
	b := [4]byte{0xCA, 0xFE, 0xBA, 0xBE}
	if ConstantTimeCompareFingerprint(a, b) {
		t.Fatal("unequal fingerprints should compare as false")
	}
}

func TestConstantTimeCompareFingerprint_Zero(t *testing.T) {
	a := [4]byte{0, 0, 0, 0}
	b := [4]byte{0, 0, 0, 0}
	if !ConstantTimeCompareFingerprint(a, b) {
		t.Fatal("zero fingerprints should compare as true")
	}

	c := [4]byte{0, 0, 0, 1}
	if ConstantTimeCompareFingerprint(a, c) {
		t.Fatal("zero vs non-zero fingerprints should compare as false")
	}
}

func TestConstantTimeCompareFingerprint_SingleBitDifference(t *testing.T) {
	a := [4]byte{0xFF, 0xFF, 0xFF, 0xFF}
	for byteIdx := 0; byteIdx < 4; byteIdx++ {
		for bit := uint(0); bit < 8; bit++ {
			b := a
			b[byteIdx] ^= 1 << bit
			if ConstantTimeCompareFingerprint(a, b) {
				t.Fatalf("single-bit difference at byte %d bit %d should be detected", byteIdx, bit)
			}
		}
	}
}

// --- ImmutableSigBinding ---

func TestImmutableSigBinding_Deterministic(t *testing.T) {
	flowID := []byte("flow-abc-123")
	epoch := uint32(42)
	sig := []byte("test-signature-data")

	binding1 := ImmutableSigBinding(flowID, epoch, sig)
	binding2 := ImmutableSigBinding(flowID, epoch, sig)

	if binding1 != binding2 {
		t.Fatal("ImmutableSigBinding should be deterministic")
	}
}

func TestImmutableSigBinding_DifferentFlowID(t *testing.T) {
	sig := []byte("same-signature")
	epoch := uint32(1)

	b1 := ImmutableSigBinding([]byte("flow-A"), epoch, sig)
	b2 := ImmutableSigBinding([]byte("flow-B"), epoch, sig)

	if b1 == b2 {
		t.Fatal("different flow IDs should produce different bindings")
	}
}

func TestImmutableSigBinding_DifferentEpoch(t *testing.T) {
	flowID := []byte("same-flow")
	sig := []byte("same-signature")

	b1 := ImmutableSigBinding(flowID, 1, sig)
	b2 := ImmutableSigBinding(flowID, 2, sig)

	if b1 == b2 {
		t.Fatal("different epochs should produce different bindings")
	}
}

func TestImmutableSigBinding_DifferentSignature(t *testing.T) {
	flowID := []byte("same-flow")
	epoch := uint32(1)

	b1 := ImmutableSigBinding(flowID, epoch, []byte("sig-1"))
	b2 := ImmutableSigBinding(flowID, epoch, []byte("sig-2"))

	if b1 == b2 {
		t.Fatal("different signatures should produce different bindings")
	}
}

// --- VerifySigBinding ---

func TestVerifySigBinding_Pass(t *testing.T) {
	flowID := []byte("flow-test")
	epoch := uint32(100)
	sig := []byte("verified-signature")

	binding := ImmutableSigBinding(flowID, epoch, sig)
	if !VerifySigBinding(binding, flowID, epoch, sig) {
		t.Fatal("VerifySigBinding should pass for matching binding")
	}
}

func TestVerifySigBinding_Fail(t *testing.T) {
	flowID := []byte("flow-test")
	epoch := uint32(100)
	sig := []byte("original-signature")

	binding := ImmutableSigBinding(flowID, epoch, sig)

	// Wrong flow ID.
	if VerifySigBinding(binding, []byte("wrong-flow"), epoch, sig) {
		t.Fatal("VerifySigBinding should fail for wrong flow ID")
	}

	// Wrong epoch.
	if VerifySigBinding(binding, flowID, 999, sig) {
		t.Fatal("VerifySigBinding should fail for wrong epoch")
	}

	// Wrong signature.
	if VerifySigBinding(binding, flowID, epoch, []byte("wrong-sig")) {
		t.Fatal("VerifySigBinding should fail for wrong signature")
	}

	// Corrupted binding.
	corrupted := binding
	corrupted[0] ^= 0xFF
	if VerifySigBinding(corrupted, flowID, epoch, sig) {
		t.Fatal("VerifySigBinding should fail for corrupted binding")
	}
}

// --- AlgoConfusionGuard ---

func TestAlgoConfusionGuard_FirstCallSetsAlgo(t *testing.T) {
	guard := NewAlgoConfusionGuard()
	err := guard.Check("flow-1", uint8(AlgoMLDSA))
	if err != nil {
		t.Fatalf("first call should succeed: %v", err)
	}
}

func TestAlgoConfusionGuard_SameAlgoPasses(t *testing.T) {
	guard := NewAlgoConfusionGuard()

	if err := guard.Check("flow-1", uint8(AlgoMLDSA)); err != nil {
		t.Fatalf("first call: %v", err)
	}

	// Same algorithm should pass.
	if err := guard.Check("flow-1", uint8(AlgoMLDSA)); err != nil {
		t.Fatalf("same algo should pass: %v", err)
	}

	// Multiple repeated calls should all pass.
	for i := 0; i < 10; i++ {
		if err := guard.Check("flow-1", uint8(AlgoMLDSA)); err != nil {
			t.Fatalf("repeated same algo call %d: %v", i, err)
		}
	}
}

func TestAlgoConfusionGuard_DifferentAlgoFails(t *testing.T) {
	guard := NewAlgoConfusionGuard()

	if err := guard.Check("flow-1", uint8(AlgoMLDSA)); err != nil {
		t.Fatalf("first call: %v", err)
	}

	// Different algorithm should fail.
	err := guard.Check("flow-1", uint8(AlgoFNDSA))
	if err == nil {
		t.Fatal("different algo should return error")
	}

	// Error message should contain relevant context.
	expected := fmt.Sprintf("algorithm confusion: flow %q established algo %#x, got %#x",
		"flow-1", uint8(AlgoMLDSA), uint8(AlgoFNDSA))
	if err.Error() != expected {
		t.Fatalf("error = %q, want %q", err.Error(), expected)
	}
}

func TestAlgoConfusionGuard_DifferentFlowsIndependent(t *testing.T) {
	guard := NewAlgoConfusionGuard()

	// Flow 1 uses ML-DSA.
	if err := guard.Check("flow-1", uint8(AlgoMLDSA)); err != nil {
		t.Fatalf("flow-1 ML-DSA: %v", err)
	}

	// Flow 2 can use a different algorithm.
	if err := guard.Check("flow-2", uint8(AlgoSLHDSA)); err != nil {
		t.Fatalf("flow-2 SLH-DSA: %v", err)
	}

	// Flow 1 still locked to ML-DSA.
	if err := guard.Check("flow-1", uint8(AlgoMLDSA)); err != nil {
		t.Fatalf("flow-1 same algo: %v", err)
	}

	// Flow 1 cannot switch.
	if err := guard.Check("flow-1", uint8(AlgoSLHDSA)); err == nil {
		t.Fatal("flow-1 should not be able to switch to SLH-DSA")
	}

	// Flow 2 cannot switch either.
	if err := guard.Check("flow-2", uint8(AlgoMLDSA)); err == nil {
		t.Fatal("flow-2 should not be able to switch to ML-DSA")
	}
}

func TestAlgoConfusionGuard_ConcurrentAccess(t *testing.T) {
	guard := NewAlgoConfusionGuard()

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			flowID := fmt.Sprintf("flow-%d", id)
			// Each goroutine registers its own flow.
			for j := 0; j < 10; j++ {
				_ = guard.Check(flowID, uint8(AlgoMLDSA))
			}
		}(i)
	}

	wg.Wait()
	// No panics or data races is the success criterion.
}

// --- SigRefRateLimiter ---

func TestSigRefRateLimiter_AllowsUpToMaxRate(t *testing.T) {
	limiter := NewSigRefRateLimiter(10, 1000, 0.9)

	// Should allow exactly maxRate allocations.
	for i := int64(0); i < 10; i++ {
		if !limiter.Allow() {
			t.Fatalf("Allow() returned false at count %d (maxRate=10)", i)
		}
	}
}

func TestSigRefRateLimiter_BlocksBeyondMaxRate(t *testing.T) {
	limiter := NewSigRefRateLimiter(5, 1000, 0.9)

	// Exhaust the rate limit.
	for i := int64(0); i < 5; i++ {
		if !limiter.Allow() {
			t.Fatalf("Allow() returned false at count %d (maxRate=5)", i)
		}
	}

	// Next allocation should be blocked.
	if limiter.Allow() {
		t.Fatal("Allow() should return false beyond maxRate")
	}

	// Multiple additional calls should also be blocked.
	if limiter.Allow() {
		t.Fatal("Allow() should still return false beyond maxRate")
	}
}

func TestSigRefRateLimiter_Utilization(t *testing.T) {
	limiter := NewSigRefRateLimiter(100, 1000, 0.9)

	tests := []struct {
		count    int64
		expected float64
	}{
		{0, 0.0},
		{100, 0.1},
		{500, 0.5},
		{900, 0.9},
		{1000, 1.0},
	}

	for _, tt := range tests {
		got := limiter.Utilization(tt.count)
		if got != tt.expected {
			t.Errorf("Utilization(%d) = %f, want %f", tt.count, got, tt.expected)
		}
	}
}

func TestSigRefRateLimiter_UtilizationZeroMaxRefs(t *testing.T) {
	limiter := NewSigRefRateLimiter(100, 0, 0.9)
	if got := limiter.Utilization(100); got != 0.0 {
		t.Fatalf("Utilization() with maxRefs=0 should return 0.0, got %f", got)
	}
}

func TestSigRefRateLimiter_NeedsEviction(t *testing.T) {
	limiter := NewSigRefRateLimiter(100, 1000, 0.9)

	// Below threshold.
	if limiter.NeedsEviction(899) {
		t.Fatal("NeedsEviction(899) should be false (threshold=0.9, 899/1000=0.899)")
	}

	// At threshold.
	if !limiter.NeedsEviction(900) {
		t.Fatal("NeedsEviction(900) should be true (threshold=0.9, 900/1000=0.9)")
	}

	// Above threshold.
	if !limiter.NeedsEviction(950) {
		t.Fatal("NeedsEviction(950) should be true (threshold=0.9, 950/1000=0.95)")
	}

	// At capacity.
	if !limiter.NeedsEviction(1000) {
		t.Fatal("NeedsEviction(1000) should be true")
	}
}

// --- Replay Attack Scenario ---

func TestReplayAttack_DuplicateSeqNumAcrossFlows(t *testing.T) {
	ft := monad.NewFlowSeqTracker(100)

	// Simulate an attacker capturing a packet from flow-A and replaying
	// it to flow-B. Each flow should track independently.
	if !ft.Check("flow-A", 42) {
		t.Fatal("flow-A seqNum 42 should pass")
	}

	// Attacker replays the same seqNum on flow-B. This is allowed because
	// flows are independent (the attacker would also need the correct
	// signature, which is per-flow).
	if !ft.Check("flow-B", 42) {
		t.Fatal("flow-B seqNum 42 should pass (independent flow)")
	}

	// The real replay attack: attacker replays on the SAME flow.
	if ft.Check("flow-A", 42) {
		t.Fatal("replay of flow-A seqNum 42 should be detected")
	}

	if ft.Check("flow-B", 42) {
		t.Fatal("replay of flow-B seqNum 42 should be detected")
	}
}

// --- Tier Downgrade Prevention ---

func TestTierDowngradePrevention(t *testing.T) {
	guard := NewAlgoConfusionGuard()

	// Flow establishes with a higher-security algorithm (ML-DSA-87 is tier 0x02).
	highTierAlgo := uint8(AlgoMLDSA) // ML-DSA, highest available tier
	if err := guard.Check("secure-flow", highTierAlgo); err != nil {
		t.Fatalf("establish high tier: %v", err)
	}

	// Attacker attempts to downgrade to a different algorithm family.
	// FN-DSA (0x03) is a weaker choice (requires floating point, not BPF-safe).
	lowerTierAlgo := uint8(AlgoFNDSA)
	err := guard.Check("secure-flow", lowerTierAlgo)
	if err == nil {
		t.Fatal("downgrade from ML-DSA to FN-DSA should be rejected")
	}

	// Attempt downgrade to SLH-DSA (different family, even if hash-based is
	// theoretically strong, it's a different algorithm).
	err = guard.Check("secure-flow", uint8(AlgoSLHDSA))
	if err == nil {
		t.Fatal("switch from ML-DSA to SLH-DSA should be rejected (algo confusion)")
	}

	// Original algorithm still works.
	if err := guard.Check("secure-flow", highTierAlgo); err != nil {
		t.Fatalf("original algo should still work: %v", err)
	}
}

// --- TOCTOU Pinning ---

func TestTOCTOUPinning_SigBinding(t *testing.T) {
	// TOCTOU (Time-Of-Check-Time-Of-Use) pinning ensures that the result
	// verified at check time cannot be swapped before use time.
	// We create a SHA-256 binding of (flow_id || result) that pins the
	// verified result to its flow context.
	flowID := []byte("toctou-flow-001")
	result := []byte("verified-pqc-result-data")

	// Create binding: SHA-256(flow_id || result).
	h := sha256.New()
	h.Write(flowID)
	h.Write(result)
	var binding [32]byte
	copy(binding[:], h.Sum(nil))

	// Verify the binding matches.
	h2 := sha256.New()
	h2.Write(flowID)
	h2.Write(result)
	var expected [32]byte
	copy(expected[:], h2.Sum(nil))

	if binding != expected {
		t.Fatal("TOCTOU binding should be deterministic")
	}

	// Attacker swaps the result: binding should not match.
	h3 := sha256.New()
	h3.Write(flowID)
	h3.Write([]byte("attacker-swapped-result"))
	var attackerBinding [32]byte
	copy(attackerBinding[:], h3.Sum(nil))

	if binding == attackerBinding {
		t.Fatal("TOCTOU binding should differ for swapped result")
	}

	// Attacker swaps the flow ID: binding should not match.
	h4 := sha256.New()
	h4.Write([]byte("attacker-flow"))
	h4.Write(result)
	var wrongFlowBinding [32]byte
	copy(wrongFlowBinding[:], h4.Sum(nil))

	if binding == wrongFlowBinding {
		t.Fatal("TOCTOU binding should differ for different flow ID")
	}
}

// --- ImmutableSigBinding with SigRef epoch boundary ---

func TestImmutableSigBinding_EpochBoundary(t *testing.T) {
	flowID := []byte("epoch-boundary-flow")
	sig := []byte("same-signature")

	// Bindings at epoch boundary should differ.
	b1 := ImmutableSigBinding(flowID, 0xFFFFFFFF, sig)
	b2 := ImmutableSigBinding(flowID, 0x00000000, sig)

	if b1 == b2 {
		t.Fatal("bindings at epoch 0xFFFFFFFF and 0x00000000 should differ")
	}
}

// --- VerifySigBinding round-trip with real signature ---

func TestVerifySigBinding_WithMLDSASignature(t *testing.T) {
	// Generate a real ML-DSA signature and verify the binding round-trip.
	_, signer, err := GenerateMLDSAKeyPair(MLDSA44)
	if err != nil {
		t.Fatalf("GenerateMLDSAKeyPair: %v", err)
	}

	message := []byte("binding integration test")
	sig, err := signer.Sign(message)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	flowID := []byte("integration-flow-1")
	epoch := uint32(7)

	binding := ImmutableSigBinding(flowID, epoch, sig)
	if !VerifySigBinding(binding, flowID, epoch, sig) {
		t.Fatal("VerifySigBinding should pass for correct binding")
	}

	// Wrong epoch.
	if VerifySigBinding(binding, flowID, 8, sig) {
		t.Fatal("VerifySigBinding should fail for wrong epoch")
	}
}
