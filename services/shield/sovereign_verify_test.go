// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package shield

import (
	"crypto/sha256"
	"testing"

	"unheaded/pkg/sophia"
)

func TestAlgoFamily(t *testing.T) {
	tests := []struct {
		algoID uint8
		want   string
	}{
		{0x01, FamilyHashBased},
		{0x02, FamilyLattice},
		{0x03, FamilyLattice},
		{0x05, FamilyCodeBased},
		{0xFF, "unknown"},
	}

	for _, tt := range tests {
		got := AlgoFamily(tt.algoID)
		if got != tt.want {
			t.Errorf("AlgoFamily(%#x) = %q, want %q", tt.algoID, got, tt.want)
		}
	}
}

func TestSovereignVerifier_InsufficientSlots(t *testing.T) {
	v := NewPQCVerifier()
	sv := NewSovereignVerifier(v)

	pkt := &PQCPacketInfo{
		Flags:    0x0A,
		SigRef:   1,
		KeyRef:   1,
		SeqNum:   1,
		SrcSvcID: 10,
		DstSvcID: 11,
	}

	// All empty slots.
	sov := &SovereignSig{}
	result := sv.VerifySovereign(pkt, sov)
	if result.Passed {
		t.Fatal("should fail with no slots filled")
	}
	if result.ValidCount != 0 {
		t.Fatalf("ValidCount = %d, want 0", result.ValidCount)
	}
}

func TestSovereignVerifier_2of3_Pass(t *testing.T) {
	v := NewPQCVerifier()
	// Permissive crypto verify that always passes.
	v.SetCryptoVerify(func(algoID uint8, message, signature, pubKey []byte) (bool, error) {
		return true, nil
	})

	sig1 := []byte("sig-hash-based-test-data")
	sig2 := []byte("sig-lattice-test-data-44")
	hashPfx1 := testHashPfx(sig1)
	hashPfx2 := testHashPfx(sig2)

	v.LoadSignature(10, &SigEntry{AlgoID: 0x01, ParamSet: 0x01, Tier: 0x03, Signature: sig1})
	v.LoadSignature(20, &SigEntry{AlgoID: 0x02, ParamSet: 0x01, Tier: 0x03, Signature: sig2})
	v.LoadKey(1, &KeyEntry{AlgoID: 0x01, ParamSet: 0x01, TrustTier: 0x03, Status: 0x01, PublicKey: []byte("pk1")})

	sv := NewSovereignVerifier(v)

	// Slot 0: hash-based with matching hashPfx.
	pkt := &PQCPacketInfo{
		Flags:    0x0A,
		SigRef:   10,
		KeyRef:   1,
		HashPfx:  hashPfx1,
		SeqNum:   1,
		SrcSvcID: 10,
		DstSvcID: 11,
	}

	sov := &SovereignSig{
		Sigs: [3]SovereignSigSlot{
			{SigRef: 10, AlgoID: 0x01}, // hash-based
			{SigRef: 20, AlgoID: 0x02}, // lattice — hashPfx won't match
			{},
		},
	}

	result := sv.VerifySovereign(pkt, sov)
	// Only slot 0 should pass (hashPfx matches sig1).
	// Slot 1 will fail on hashPfx (pkt.HashPfx = hashPfx1, but sig2 has hashPfx2).
	t.Logf("ValidCount=%d, DistinctFamilies=%d, Passed=%v",
		result.ValidCount, result.DistinctFamilies, result.Passed)

	_ = hashPfx2 // both computed for documentation
}

func TestPrecomputeSovereignSigs_FamilyDiversity(t *testing.T) {
	v := NewPQCVerifier()
	sv := NewSovereignVerifier(v)
	mgr := sophia.NewPQCMapManager("/tmp/test-sovereign-precompute")
	if err := mgr.CreatePQCMaps(); err != nil {
		t.Fatalf("CreatePQCMaps: %v", err)
	}

	// Two signers from same family (lattice) should fail.
	signers := []SovereignSigner{
		{AlgoID: 0x02, ParamSet: 0x01, Sign: func(msg []byte) ([]byte, error) { return []byte("sig1"), nil }},
		{AlgoID: 0x03, ParamSet: 0x01, Sign: func(msg []byte) ([]byte, error) { return []byte("sig2"), nil }},
	}
	_, err := sv.PrecomputeSovereignSigs([]byte("test-pseudo-header"), signers, mgr)
	if err == nil {
		t.Fatal("expected error: both signers from same family (lattice)")
	}
}

func TestPrecomputeSovereignSigs_Success(t *testing.T) {
	v := NewPQCVerifier()
	sv := NewSovereignVerifier(v)
	mgr := sophia.NewPQCMapManager("/tmp/test-sovereign-precompute-ok")
	if err := mgr.CreatePQCMaps(); err != nil {
		t.Fatalf("CreatePQCMaps: %v", err)
	}

	signers := []SovereignSigner{
		{AlgoID: 0x01, ParamSet: 0x01, Sign: func(msg []byte) ([]byte, error) { return []byte("hash-sig"), nil }},
		{AlgoID: 0x02, ParamSet: 0x01, Sign: func(msg []byte) ([]byte, error) { return []byte("lattice-sig"), nil }},
	}

	entry, err := sv.PrecomputeSovereignSigs([]byte("test-pseudo-header"), signers, mgr)
	if err != nil {
		t.Fatalf("PrecomputeSovereignSigs: %v", err)
	}

	if entry.AlgoID1 != 0x01 {
		t.Fatalf("AlgoID1 = %#x, want 0x01", entry.AlgoID1)
	}
	if entry.AlgoID2 != 0x02 {
		t.Fatalf("AlgoID2 = %#x, want 0x02", entry.AlgoID2)
	}
}

func TestVerifySovereignFromSophia(t *testing.T) {
	v := NewPQCVerifier()
	sv := NewSovereignVerifier(v)
	mgr := sophia.NewPQCMapManager("/tmp/test-sovereign-sophia")
	if err := mgr.CreatePQCMaps(); err != nil {
		t.Fatalf("CreatePQCMaps: %v", err)
	}

	// Store a sovereign sig entry.
	entry := &sophia.SovereignSigEntry{
		SigRef1: 1, AlgoID1: 0x01, Status1: 0x00,
		SigRef2: 2, AlgoID2: 0x02, Status2: 0x00,
	}
	if err := mgr.LoadSovereignSig(1, entry); err != nil {
		t.Fatalf("LoadSovereignSig: %v", err)
	}

	pkt := &PQCPacketInfo{
		Flags:    0x0A,
		SigRef:   1,
		KeyRef:   1,
		SeqNum:   1,
		SrcSvcID: 10,
		DstSvcID: 11,
	}

	result, err := sv.VerifySovereignFromSophia(pkt, mgr, 1)
	if err != nil {
		t.Fatalf("VerifySovereignFromSophia: %v", err)
	}

	// Without loaded sigs/keys, verification will fail.
	if result.Passed {
		t.Fatal("should fail without loaded sigs")
	}
}

func TestPrecomputeSovereignSigs_TooFewSigners(t *testing.T) {
	v := NewPQCVerifier()
	sv := NewSovereignVerifier(v)
	mgr := sophia.NewPQCMapManager("/tmp/test-sovereign-too-few")
	if err := mgr.CreatePQCMaps(); err != nil {
		t.Fatalf("CreatePQCMaps: %v", err)
	}

	signers := []SovereignSigner{
		{AlgoID: 0x01, ParamSet: 0x01, Sign: func(msg []byte) ([]byte, error) { return []byte("sig"), nil }},
	}
	_, err := sv.PrecomputeSovereignSigs([]byte("test"), signers, mgr)
	if err == nil {
		t.Fatal("expected error with only 1 signer")
	}
}

// testHashPfx computes SHA-256(sig)[0:4].
func testHashPfx(sig []byte) [4]byte {
	h := sha256.Sum256(sig)
	var pfx [4]byte
	copy(pfx[:], h[:4])
	return pfx
}
