// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package shield

import (
	"crypto/sha256"
	"testing"
	"time"
)

func newTestVerifier() *PQCVerifier {
	v := NewPQCVerifier()
	// Always-pass crypto verifier for tests
	v.SetCryptoVerify(func(algoID uint8, message, signature, pubKey []byte) (bool, error) {
		return true, nil
	})
	return v
}

func loadTestData(v *PQCVerifier) (*SigEntry, *KeyEntry) {
	sig := &SigEntry{
		AlgoID:    0x02, // ML-DSA
		ParamSet:  0x01,
		Tier:      0x01,
		Signature: []byte("test-signature-data-for-pqc-verification"),
		CreatedAt: time.Now(),
	}
	key := &KeyEntry{
		AlgoID:    0x02,
		ParamSet:  0x01,
		TrustTier: 0x01,
		Status:    0x01, // active
		PublicKey: []byte("test-public-key"),
	}

	v.LoadSignature(1, sig)
	v.LoadKey(1, key)
	return sig, key
}

func makePacket(sig *SigEntry) *PQCPacketInfo {
	hash := sha256.Sum256(sig.Signature)
	var pfx [4]byte
	copy(pfx[:], hash[:4])

	return &PQCPacketInfo{
		Flags:        0x0A, // S+CUSTOM
		SigRef:       1,
		KeyRef:       1,
		HashPfx:      pfx,
		SeqNum:       1,
		SrcSvcID:     100,
		DstSvcID:     200,
		PseudoHeader: make([]byte, 52),
	}
}

func TestVerify7PointSuccess(t *testing.T) {
	v := newTestVerifier()
	sig, _ := loadTestData(v)
	pkt := makePacket(sig)

	result := v.Verify(pkt)
	if !result.Passed {
		t.Fatalf("verification should pass, failed at step %s: %s", result.FailStep, result.FailMsg)
	}
	if result.AlgoID != 0x02 {
		t.Errorf("AlgoID = 0x%02x, want 0x02", result.AlgoID)
	}
	if v.VerifyPassCount() != 1 {
		t.Errorf("pass count = %d, want 1", v.VerifyPassCount())
	}
}

func TestStep1FlagCheck(t *testing.T) {
	v := newTestVerifier()
	loadTestData(v)

	pkt := &PQCPacketInfo{Flags: 0x00} // No PQC flags
	result := v.Verify(pkt)
	if result.Passed {
		t.Fatal("should fail on flag check")
	}
	if result.FailStep != StepFlagCheck {
		t.Errorf("FailStep = %s, want flag_check", result.FailStep)
	}
}

func TestStep2SigRefNotFound(t *testing.T) {
	v := newTestVerifier()
	loadTestData(v)

	pkt := &PQCPacketInfo{Flags: 0x0A, SigRef: 999}
	result := v.Verify(pkt)
	if result.Passed {
		t.Fatal("should fail on SigRef lookup")
	}
	if result.FailStep != StepSigRefLookup {
		t.Errorf("FailStep = %s, want sigref_lookup", result.FailStep)
	}
}

func TestStep3KeyRefNotFound(t *testing.T) {
	v := newTestVerifier()
	loadTestData(v)

	pkt := &PQCPacketInfo{Flags: 0x0A, SigRef: 1, KeyRef: 999}
	result := v.Verify(pkt)
	if result.Passed {
		t.Fatal("should fail on KeyRef lookup")
	}
	if result.FailStep != StepKeyRefLookup {
		t.Errorf("FailStep = %s, want keyref_lookup", result.FailStep)
	}
}

func TestStep3KeyRevoked(t *testing.T) {
	v := newTestVerifier()
	sig, _ := loadTestData(v)

	// Load a revoked key
	v.LoadKey(2, &KeyEntry{
		AlgoID:    0x02,
		TrustTier: 0x01,
		Status:    0x03, // revoked
		PublicKey: []byte("revoked-key"),
	})

	pkt := makePacket(sig)
	pkt.KeyRef = 2
	result := v.Verify(pkt)
	if result.Passed {
		t.Fatal("should fail on revoked key")
	}
	if result.FailStep != StepKeyRefLookup {
		t.Errorf("FailStep = %s, want keyref_lookup", result.FailStep)
	}
}

func TestStep3TrustTierTooLow(t *testing.T) {
	v := newTestVerifier()
	sig, _ := loadTestData(v)
	v.SetRequiredTier(0x03) // SOVEREIGN

	pkt := makePacket(sig)
	result := v.Verify(pkt)
	if result.Passed {
		t.Fatal("should fail on trust tier")
	}
	if result.FailStep != StepKeyRefLookup {
		t.Errorf("FailStep = %s, want keyref_lookup", result.FailStep)
	}
}

func TestStep4AlgoNotAllowed(t *testing.T) {
	v := newTestVerifier()

	sig := &SigEntry{
		AlgoID:    0x05, // HQC - not a signature algo
		Signature: []byte("hqc-data"),
		CreatedAt: time.Now(),
	}
	v.LoadSignature(1, sig)
	v.LoadKey(1, &KeyEntry{Status: 0x01, TrustTier: 0x01})

	pkt := makePacket(sig)
	result := v.Verify(pkt)
	if result.Passed {
		t.Fatal("should fail on algo compliance")
	}
	if result.FailStep != StepAlgoCompliance {
		t.Errorf("FailStep = %s, want algo_compliance", result.FailStep)
	}
}

func TestStep5CryptoVerifyFails(t *testing.T) {
	v := NewPQCVerifier()
	v.SetCryptoVerify(func(algoID uint8, message, signature, pubKey []byte) (bool, error) {
		return false, nil
	})
	sig, _ := loadTestData(v)
	pkt := makePacket(sig)

	result := v.Verify(pkt)
	if result.Passed {
		t.Fatal("should fail on crypto verify")
	}
	if result.FailStep != StepCryptoVerify {
		t.Errorf("FailStep = %s, want crypto_verify", result.FailStep)
	}
}

func TestStep6HashPfxMismatch(t *testing.T) {
	v := newTestVerifier()
	sig, _ := loadTestData(v)
	pkt := makePacket(sig)
	pkt.HashPfx = [4]byte{0xFF, 0xFF, 0xFF, 0xFF} // Wrong prefix

	result := v.Verify(pkt)
	if result.Passed {
		t.Fatal("should fail on HashPfx mismatch")
	}
	if result.FailStep != StepHashPfxMatch {
		t.Errorf("FailStep = %s, want hashpfx_match", result.FailStep)
	}
}

func TestStep7SeqNumReplay(t *testing.T) {
	v := newTestVerifier()
	sig, _ := loadTestData(v)
	pkt := makePacket(sig)

	// First packet passes
	result1 := v.Verify(pkt)
	if !result1.Passed {
		t.Fatalf("first packet should pass: %s", result1.FailMsg)
	}

	// Same SeqNum = replay
	result2 := v.Verify(pkt)
	if result2.Passed {
		t.Fatal("replayed packet should fail")
	}
	if result2.FailStep != StepSeqNumReplay {
		t.Errorf("FailStep = %s, want seqnum_replay", result2.FailStep)
	}
}

func TestSeqNumWindowAdvance(t *testing.T) {
	v := newTestVerifier()
	sig, _ := loadTestData(v)

	for seq := uint16(1); seq <= 100; seq++ {
		pkt := makePacket(sig)
		pkt.SeqNum = seq
		result := v.Verify(pkt)
		if !result.Passed {
			t.Fatalf("seq %d should pass: %s", seq, result.FailMsg)
		}
	}

	if v.VerifyPassCount() != 100 {
		t.Errorf("pass count = %d, want 100", v.VerifyPassCount())
	}
}

func TestVerificationStepString(t *testing.T) {
	tests := []struct {
		step PQCVerificationStep
		want string
	}{
		{StepFlagCheck, "flag_check"},
		{StepSigRefLookup, "sigref_lookup"},
		{StepCryptoVerify, "crypto_verify"},
		{StepHashPfxMatch, "hashpfx_match"},
		{StepSeqNumReplay, "seqnum_replay"},
		{PQCVerificationStep(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.step.String(); got != tt.want {
			t.Errorf("Step(%d).String() = %q, want %q", tt.step, got, tt.want)
		}
	}
}
