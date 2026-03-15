// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package pqc_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"testing"
	"time"

	"unheaded/pkg/crypto/pqc"
	"unheaded/pkg/monad"
	"unheaded/pkg/policy"
	pqcPolicy "unheaded/pkg/pqc_policy"
	sophiaPkg "unheaded/pkg/sophia"
	"unheaded/pkg/wotan"
	"unheaded/services/shield"
)

// buildBenchPseudoHeader constructs a pseudo-header byte slice for benchmarks.
// Unlike makeSignedPacket, this does not sign the result -- it is used for
// benchmarks that skip real crypto verification.
func buildBenchPseudoHeader(sigRef uint32, keyRef uint16, hashPfx [4]byte, seqNum uint16) []byte {
	srcIP := [16]byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
	dstIP := [16]byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2}
	monadPfx := [8]byte{0x01, 0x0A, 0x0B, 0x00, 0x03, 0x01, 0x01, monad.FlagPQCActive}

	pqcVal := monad.PQCValue{
		SigRef:  sigRef,
		KeyRef:  keyRef,
		HashPfx: hashPfx,
		SeqNum:  seqNum,
	}
	pqcValBuf := pqcVal.Marshal()

	ph := monad.BuildPseudoHeader(srcIP, dstIP, monadPfx, pqcValBuf)
	buf := ph.Marshal()
	return buf[:]
}

// ---------------------------------------------------------------------------
// BenchmarkCachedVerification -- target: <300ns
// Measures verification when signature/key are already cached in Shield.
// No real crypto verification -- benchmarks the cached fast path:
// flag check + map lookups + hashpfx + seqnum only.
// ---------------------------------------------------------------------------

func BenchmarkCachedVerification(b *testing.B) {
	_, signer, err := pqc.GenerateMLDSAKeyPair(pqc.MLDSA65)
	if err != nil {
		b.Fatalf("keygen: %v", err)
	}

	msg := []byte("cached verification benchmark message")
	sig, err := signer.Sign(msg)
	if err != nil {
		b.Fatalf("sign: %v", err)
	}

	verifier := shield.NewPQCVerifier()
	// Do NOT set cryptoVerify -- benchmarks the cached fast path.

	hashPfx := pqc.HashPrefix(sig)
	var sigRef uint32 = 1
	var keyRef uint16 = 1

	verifier.LoadSignature(sigRef, &shield.SigEntry{
		AlgoID:    uint8(pqc.AlgoMLDSA),
		ParamSet:  uint8(pqc.MLDSA65),
		Tier:      uint8(policy.TierStandard),
		Signature: sig,
		CreatedAt: time.Now(),
	})
	verifier.LoadKey(keyRef, &shield.KeyEntry{
		AlgoID:    uint8(pqc.AlgoMLDSA),
		ParamSet:  uint8(pqc.MLDSA65),
		TrustTier: uint8(policy.TierStandard),
		Status:    sophiaPkg.KeyStatusActive,
		PublicKey: []byte("placeholder-pk"), // Not used for crypto in this benchmark.
	})

	ph := buildBenchPseudoHeader(sigRef, keyRef, hashPfx, 1)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		pkt := &shield.PQCPacketInfo{
			Flags:        monad.FlagPQCActive,
			SigRef:       sigRef,
			KeyRef:       keyRef,
			HashPfx:      hashPfx,
			SeqNum:       uint16(i + 1),
			SrcSvcID:     10,
			DstSvcID:     11,
			PseudoHeader: ph,
		}
		result := verifier.Verify(pkt)
		if !result.Passed {
			b.Fatalf("verification failed at iteration %d: %s", i, result.FailMsg)
		}
	}
}

// ---------------------------------------------------------------------------
// BenchmarkFirstPacketVerification -- target: <5ms
// Measures the full verification path including real cryptographic verification.
// ---------------------------------------------------------------------------

func BenchmarkFirstPacketVerification(b *testing.B) {
	kp, signer, err := pqc.GenerateMLDSAKeyPair(pqc.MLDSA65)
	if err != nil {
		b.Fatalf("keygen: %v", err)
	}

	// Pre-create signed packet for consistent benchmark.
	var sigRef uint32 = 1
	var keyRef uint16 = 1
	sp := makeSignedPacket(signer, kp.PublicKey, sigRef, keyRef, 1, 10, 11)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		// Each iteration creates a fresh verifier (cold path).
		v := shield.NewPQCVerifier()
		v.SetCryptoVerify(realCryptoVerify)

		v.LoadSignature(sigRef, &shield.SigEntry{
			AlgoID:    uint8(pqc.AlgoMLDSA),
			ParamSet:  uint8(pqc.MLDSA65),
			Tier:      uint8(policy.TierStandard),
			Signature: sp.Sig,
			CreatedAt: time.Now(),
		})
		v.LoadKey(keyRef, &shield.KeyEntry{
			AlgoID:    uint8(pqc.AlgoMLDSA),
			ParamSet:  uint8(pqc.MLDSA65),
			TrustTier: uint8(policy.TierStandard),
			Status:    sophiaPkg.KeyStatusActive,
			PublicKey: kp.PublicKey,
		})

		// Use a unique SeqNum per iteration to avoid replay.
		pkt := &shield.PQCPacketInfo{
			Flags:        monad.FlagPQCActive,
			SigRef:       sigRef,
			KeyRef:       keyRef,
			HashPfx:      sp.HashPfx,
			SeqNum:       uint16(i + 1),
			SrcSvcID:     10,
			DstSvcID:     11,
			PseudoHeader: sp.PhBytes,
		}

		result := v.Verify(pkt)
		if !result.Passed {
			b.Fatalf("verification failed: %s", result.FailMsg)
		}
	}
}

// ---------------------------------------------------------------------------
// BenchmarkMLDSA65SignVerify -- full round-trip
// ---------------------------------------------------------------------------

func BenchmarkMLDSA65SignVerify(b *testing.B) {
	kp, signer, err := pqc.GenerateMLDSAKeyPair(pqc.MLDSA65)
	if err != nil {
		b.Fatalf("keygen: %v", err)
	}

	verifier, err := pqc.NewMLDSAVerifier(pqc.MLDSA65)
	if err != nil {
		b.Fatalf("NewMLDSAVerifier: %v", err)
	}

	message := []byte("ML-DSA-65 sign+verify benchmark message")

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sig, err := signer.Sign(message)
		if err != nil {
			b.Fatalf("sign: %v", err)
		}
		valid, err := verifier.Verify(message, sig, kp.PublicKey)
		if err != nil {
			b.Fatalf("verify: %v", err)
		}
		if !valid {
			b.Fatal("verification failed")
		}
	}
}

// ---------------------------------------------------------------------------
// BenchmarkMLKEM768EncapDecap -- full round-trip
// ---------------------------------------------------------------------------

func BenchmarkMLKEM768EncapDecap(b *testing.B) {
	kp, err := pqc.GenerateMLKEMKeyPair(pqc.MLKEM768)
	if err != nil {
		b.Fatalf("keygen: %v", err)
	}

	encap, err := pqc.NewMLKEMEncapsulator(pqc.MLKEM768)
	if err != nil {
		b.Fatalf("NewMLKEMEncapsulator: %v", err)
	}

	decap, err := pqc.NewMLKEMDecapsulator(pqc.MLKEM768)
	if err != nil {
		b.Fatalf("NewMLKEMDecapsulator: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ct, ss1, err := encap.Encapsulate(kp.PublicKey)
		if err != nil {
			b.Fatalf("encapsulate: %v", err)
		}
		ss2, err := decap.Decapsulate(ct, kp.SecretKey)
		if err != nil {
			b.Fatalf("decapsulate: %v", err)
		}
		if !bytes.Equal(ss1, ss2) {
			b.Fatal("shared secrets mismatch")
		}
	}
}

// ---------------------------------------------------------------------------
// BenchmarkHashPrefix -- target: <100ns
// ---------------------------------------------------------------------------

func BenchmarkHashPrefix(b *testing.B) {
	// Use realistic signature-sized input.
	sig := make([]byte, 3309) // ML-DSA-65 signature size
	if _, err := rand.Read(sig); err != nil {
		b.Fatalf("rand.Read: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		pqc.HashPrefix(sig)
	}
}

// ---------------------------------------------------------------------------
// BenchmarkPQCValueMarshal -- target: <50ns
// ---------------------------------------------------------------------------

func BenchmarkPQCValueMarshal(b *testing.B) {
	v := monad.PQCValue{
		SigRef:  0x00ABCDEF,
		KeyRef:  0x1234,
		HashPfx: [4]byte{0xDE, 0xAD, 0xBE, 0xEF},
		SeqNum:  0x5678,
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = v.Marshal()
	}
}

// ---------------------------------------------------------------------------
// BenchmarkPQCValueUnmarshal
// ---------------------------------------------------------------------------

func BenchmarkPQCValueUnmarshal(b *testing.B) {
	v := monad.PQCValue{
		SigRef:  0x00ABCDEF,
		KeyRef:  0x1234,
		HashPfx: [4]byte{0xDE, 0xAD, 0xBE, 0xEF},
		SeqNum:  0x5678,
	}
	buf := v.Marshal()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := monad.UnmarshalPQCValue(buf[:])
		if err != nil {
			b.Fatalf("UnmarshalPQCValue: %v", err)
		}
	}
}

// ---------------------------------------------------------------------------
// BenchmarkSeqTrackerCheck -- target: <50ns
// Measures the replay protection sliding window check (no real crypto).
// ---------------------------------------------------------------------------

func BenchmarkSeqTrackerCheck(b *testing.B) {
	_, signer, err := pqc.GenerateMLDSAKeyPair(pqc.MLDSA44)
	if err != nil {
		b.Fatalf("keygen: %v", err)
	}

	msg := []byte("seqnum tracker benchmark")
	sig, err := signer.Sign(msg)
	if err != nil {
		b.Fatalf("sign: %v", err)
	}

	verifier := shield.NewPQCVerifier()
	// No crypto verify -- just measure the map lookup + seqnum path.

	hashPfx := pqc.HashPrefix(sig)
	var sigRef uint32 = 1
	var keyRef uint16 = 1

	verifier.LoadSignature(sigRef, &shield.SigEntry{
		AlgoID:    uint8(pqc.AlgoMLDSA),
		ParamSet:  uint8(pqc.MLDSA44),
		Tier:      uint8(policy.TierStandard),
		Signature: sig,
		CreatedAt: time.Now(),
	})
	verifier.LoadKey(keyRef, &shield.KeyEntry{
		AlgoID:    uint8(pqc.AlgoMLDSA),
		ParamSet:  uint8(pqc.MLDSA44),
		TrustTier: uint8(policy.TierStandard),
		Status:    sophiaPkg.KeyStatusActive,
		PublicKey: []byte("placeholder-pk"),
	})

	ph := buildBenchPseudoHeader(sigRef, keyRef, hashPfx, 1)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		pkt := &shield.PQCPacketInfo{
			Flags:        monad.FlagPQCActive,
			SigRef:       sigRef,
			KeyRef:       keyRef,
			HashPfx:      hashPfx,
			SeqNum:       uint16(i + 1),
			SrcSvcID:     10,
			DstSvcID:     11,
			PseudoHeader: ph,
		}
		result := verifier.Verify(pkt)
		if !result.Passed {
			b.Fatalf("verify failed at %d: %s", i, result.FailMsg)
		}
	}
}

// ---------------------------------------------------------------------------
// BenchmarkPseudoHeaderBuild -- target: <100ns
// ---------------------------------------------------------------------------

func BenchmarkPseudoHeaderBuild(b *testing.B) {
	srcIP := [16]byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
	dstIP := [16]byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2}
	monadPfx := [8]byte{0x01, 0x0A, 0x0B, 0x00, 0x03, 0x01, 0x01, monad.FlagPQCActive}
	pqcVal := [12]byte{0, 0, 0, 42, 0, 7, 0xDE, 0xAD, 0xBE, 0xEF, 0, 1}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ph := monad.BuildPseudoHeader(srcIP, dstIP, monadPfx, pqcVal)
		_ = ph.Marshal()
	}
}

// ---------------------------------------------------------------------------
// BenchmarkVerifyHashPrefix
// ---------------------------------------------------------------------------

func BenchmarkVerifyHashPrefix(b *testing.B) {
	sig := make([]byte, 3309)
	if _, err := rand.Read(sig); err != nil {
		b.Fatalf("rand.Read: %v", err)
	}
	pfx := pqc.HashPrefix(sig)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		pqc.VerifyHashPrefix(pfx, sig)
	}
}

// ---------------------------------------------------------------------------
// BenchmarkWotanStateMarshal
// ---------------------------------------------------------------------------

func BenchmarkWotanStateMarshal(b *testing.B) {
	state := wotan.PQCState{
		SigCount:     100,
		KeyCount:     50,
		VerifyPass:   10000,
		VerifyFail:   42,
		LastRotation: time.Now().Unix(),
		PolicyMode:   uint8(policy.ModePessimistic),
		ActiveTier:   uint8(policy.TierEnhanced),
		KEMTunnels:   5,
		LastVerifyNs: 150,
		SeqHighWater: 65535,
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = state.Marshal()
	}
}

// ---------------------------------------------------------------------------
// BenchmarkMLDSAAllParamsSignVerify -- comparative across all ML-DSA params
// ---------------------------------------------------------------------------

func BenchmarkMLDSAAllParamsSignVerify(b *testing.B) {
	params := []struct {
		name string
		ps   pqc.ParameterSet
	}{
		{"ML-DSA-44", pqc.MLDSA44},
		{"ML-DSA-65", pqc.MLDSA65},
		{"ML-DSA-87", pqc.MLDSA87},
	}

	for _, tt := range params {
		b.Run(tt.name, func(b *testing.B) {
			kp, signer, err := pqc.GenerateMLDSAKeyPair(tt.ps)
			if err != nil {
				b.Fatalf("keygen: %v", err)
			}
			verifier, err := pqc.NewMLDSAVerifier(tt.ps)
			if err != nil {
				b.Fatalf("NewMLDSAVerifier: %v", err)
			}
			message := []byte("benchmark message for all params")

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				sig, err := signer.Sign(message)
				if err != nil {
					b.Fatalf("sign: %v", err)
				}
				valid, err := verifier.Verify(message, sig, kp.PublicKey)
				if err != nil {
					b.Fatalf("verify: %v", err)
				}
				if !valid {
					b.Fatal("verification failed")
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// BenchmarkMLKEMAllParamsEncapDecap -- comparative across all ML-KEM params
// ---------------------------------------------------------------------------

func BenchmarkMLKEMAllParamsEncapDecap(b *testing.B) {
	params := []struct {
		name string
		ps   pqc.ParameterSet
	}{
		{"ML-KEM-512", pqc.MLKEM512},
		{"ML-KEM-768", pqc.MLKEM768},
		{"ML-KEM-1024", pqc.MLKEM1024},
	}

	for _, tt := range params {
		b.Run(tt.name, func(b *testing.B) {
			kp, err := pqc.GenerateMLKEMKeyPair(tt.ps)
			if err != nil {
				b.Fatalf("keygen: %v", err)
			}
			encap, err := pqc.NewMLKEMEncapsulator(tt.ps)
			if err != nil {
				b.Fatalf("encapsulator: %v", err)
			}
			decap, err := pqc.NewMLKEMDecapsulator(tt.ps)
			if err != nil {
				b.Fatalf("decapsulator: %v", err)
			}

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				ct, ss1, err := encap.Encapsulate(kp.PublicKey)
				if err != nil {
					b.Fatalf("encapsulate: %v", err)
				}
				ss2, err := decap.Decapsulate(ct, kp.SecretKey)
				if err != nil {
					b.Fatalf("decapsulate: %v", err)
				}
				if !bytes.Equal(ss1, ss2) {
					b.Fatal("shared secrets mismatch")
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// BenchmarkTierVerification — measures tier-based verification dispatch
// ---------------------------------------------------------------------------

func BenchmarkTierVerification(b *testing.B) {
	kp, signer, err := pqc.GenerateMLDSAKeyPair(pqc.MLDSA65)
	if err != nil {
		b.Fatalf("keygen: %v", err)
	}

	v := shield.NewPQCVerifier()
	v.SetCryptoVerify(func(algoID uint8, message, signature, pubKey []byte) (bool, error) {
		return true, nil // Skip real crypto for dispatch benchmark
	})
	sv := shield.NewSovereignVerifier(v)
	pe := shield.NewLayer2PolicyEngine()
	sm := wotan.NewPQCStateManager()
	tv := shield.NewTierVerifier(v, sv, pe, sm)

	sp := makeSignedPacket(signer, kp.PublicKey, 1, 1, 1, 10, 11)
	v.LoadSignature(1, &shield.SigEntry{
		AlgoID:    uint8(pqc.AlgoMLDSA),
		ParamSet:  uint8(pqc.MLDSA65),
		Tier:      uint8(policy.TierStandard),
		Signature: sp.Sig,
		CreatedAt: time.Now(),
	})
	v.LoadKey(1, &shield.KeyEntry{
		AlgoID:    uint8(pqc.AlgoMLDSA),
		ParamSet:  uint8(pqc.MLDSA65),
		TrustTier: uint8(policy.TierStandard),
		Status:    sophiaPkg.KeyStatusActive,
		PublicKey: kp.PublicKey,
	})

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		pkt := &shield.PQCPacketInfo{
			Flags:        monad.FlagPQCActive,
			SigRef:       1,
			KeyRef:       1,
			HashPfx:      sp.HashPfx,
			SeqNum:       uint16(i + 1),
			SrcSvcID:     10,
			DstSvcID:     11,
			PseudoHeader: sp.PhBytes,
		}
		result := tv.VerifyPacket(pkt, nil)
		if !result.Passed() {
			b.Fatalf("verify failed at %d", i)
		}
	}
}

// ---------------------------------------------------------------------------
// BenchmarkSovereignVerification — sovereign multi-sig verification
// ---------------------------------------------------------------------------

func BenchmarkSovereignVerification(b *testing.B) {
	v := shield.NewPQCVerifier()
	v.SetCryptoVerify(func(algoID uint8, message, signature, pubKey []byte) (bool, error) {
		return true, nil
	})

	sig := []byte("bench-sovereign-sig")
	hashPfx := pqc.HashPrefix(sig)

	v.LoadSignature(1, &shield.SigEntry{AlgoID: 0x01, Tier: 0x03, Signature: sig, CreatedAt: time.Now()})
	v.LoadSignature(2, &shield.SigEntry{AlgoID: 0x02, Tier: 0x03, Signature: sig, CreatedAt: time.Now()})
	v.LoadKey(1, &shield.KeyEntry{AlgoID: 0x01, TrustTier: 0x03, Status: 0x01, PublicKey: []byte("pk")})

	sv := shield.NewSovereignVerifier(v)
	ph := buildBenchPseudoHeader(1, 1, hashPfx, 1)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		pkt := &shield.PQCPacketInfo{
			Flags:        monad.FlagPQCActive,
			SigRef:       1,
			KeyRef:       1,
			HashPfx:      hashPfx,
			SeqNum:       uint16(i + 1),
			SrcSvcID:     10,
			DstSvcID:     11,
			PseudoHeader: ph,
		}
		sov := &shield.SovereignSig{
			Sigs: [3]shield.SovereignSigSlot{
				{SigRef: 1, AlgoID: 0x01},
				{SigRef: 2, AlgoID: 0x02},
			},
		}
		_ = sv.VerifySovereign(pkt, sov)
	}
}

// ---------------------------------------------------------------------------
// BenchmarkKEMHandshake — full KEM handshake including key derivation
// ---------------------------------------------------------------------------

func BenchmarkKEMHandshake(b *testing.B) {
	kp, err := pqc.GenerateMLKEMKeyPair(pqc.MLKEM768)
	if err != nil {
		b.Fatalf("keygen: %v", err)
	}

	neg, err := pqc.NewTunnelNegotiator(pqc.MLKEM768)
	if err != nil {
		b.Fatalf("NewTunnelNegotiator: %v", err)
	}

	salt := []byte("bench-salt")

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		initResult, err := neg.InitiatorHandshake(kp.PublicKey, salt, "bench")
		if err != nil {
			b.Fatalf("InitiatorHandshake: %v", err)
		}
		respResult, err := neg.ResponderHandshake(initResult.Ciphertext, kp.SecretKey, salt, "bench")
		if err != nil {
			b.Fatalf("ResponderHandshake: %v", err)
		}
		if initResult.SharedSecret != respResult.SharedSecret {
			b.Fatal("mismatch")
		}
		initResult.Zero()
		respResult.Zero()
	}
}

// ---------------------------------------------------------------------------
// BenchmarkHKDFKeyDerivation — HKDF-SHA256 key derivation only
// ---------------------------------------------------------------------------

func BenchmarkHKDFKeyDerivation(b *testing.B) {
	ss := [32]byte{}
	rand.Read(ss[:])
	flowID := []byte("bench-flow-001")
	salt := []byte("bench-salt")

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := pqc.DeriveSigningKey(ss, salt, flowID, uint32(i))
		if err != nil {
			b.Fatalf("DeriveSigningKey: %v", err)
		}
	}
}

// ---------------------------------------------------------------------------
// BenchmarkPolicyCheck — 7-point policy verification
// ---------------------------------------------------------------------------

func BenchmarkPolicyCheck(b *testing.B) {
	checker := pqcPolicy.NewChecker()
	checker.SetPolicy(&pqcPolicy.AppPolicy{
		ServiceID:    100,
		AllowedAlgos: []uint8{0x02},
		MinNISTLevel: pqcPolicy.NISTLevel3,
		MaxKeyAgeSec: 3600,
		RequirePQC:   true,
	})

	ctx := context.Background()
	req := pqcPolicy.PolicyCheckRequest{
		ServiceID:    100,
		AlgoID:       0x02,
		ParamSet:     0x02,
		PQCVerified:  true,
		KeyCreatedAt: time.Now(),
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		r := checker.Check(ctx, req)
		if !r.Allowed {
			b.Fatalf("check failed: %s", r.FailReason)
		}
	}
}
