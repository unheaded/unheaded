// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package pqc_test

import (
	"bytes"
	"errors"
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

// ---------------------------------------------------------------------------
// signedPacket bundles a Shield PQCPacketInfo together with the signature and
// key material produced during the proper protocol flow:
//   build pseudo-header -> sign -> compute HashPfx -> package
// ---------------------------------------------------------------------------

type signedPacket struct {
	Pkt       *shield.PQCPacketInfo
	Sig       []byte
	PubKey    []byte
	SigRef    uint32
	KeyRef    uint16
	HashPfx   [4]byte
	PhBytes   []byte // the exact bytes that were signed
}

// makeSignedPacket follows the correct PQC protocol flow:
//  1. Build a pseudo-header (with zero HashPfx placeholder).
//  2. Sign those pseudo-header bytes.
//  3. Compute HashPfx = SHA-256(sig)[0:4].
//  4. Return a PQCPacketInfo whose PseudoHeader is the signed bytes.
func makeSignedPacket(
	signer pqc.Signer,
	pubKey []byte,
	sigRef uint32,
	keyRef uint16,
	seqNum uint16,
	srcSvc, dstSvc uint16,
) signedPacket {
	srcIP := [16]byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
	dstIP := [16]byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2}
	monadPfx := [8]byte{0x01, 0x0A, 0x0B, 0x00, 0x03, 0x01, 0x01, monad.FlagPQCActive}

	// Build PQC value with zero HashPfx (placeholder before signing).
	pqcVal := monad.PQCValue{
		SigRef:  sigRef,
		KeyRef:  keyRef,
		HashPfx: [4]byte{},
		SeqNum:  seqNum,
	}
	pqcValBuf := pqcVal.Marshal()

	ph := monad.BuildPseudoHeader(srcIP, dstIP, monadPfx, pqcValBuf)
	phBytes := ph.Marshal()

	// Sign the pseudo-header bytes.
	sig, err := signer.Sign(phBytes[:])
	if err != nil {
		panic("makeSignedPacket: sign failed: " + err.Error())
	}

	hashPfx := pqc.HashPrefix(sig)

	pkt := &shield.PQCPacketInfo{
		Flags:        monad.FlagPQCActive,
		SigRef:       sigRef,
		KeyRef:       keyRef,
		HashPfx:      hashPfx,
		SeqNum:       seqNum,
		SrcSvcID:     srcSvc,
		DstSvcID:     dstSvc,
		PseudoHeader: phBytes[:],
	}

	return signedPacket{
		Pkt:     pkt,
		Sig:     sig,
		PubKey:  pubKey,
		SigRef:  sigRef,
		KeyRef:  keyRef,
		HashPfx: hashPfx,
		PhBytes: phBytes[:],
	}
}

// loadIntoVerifier loads the signedPacket's signature and key entries into a
// Shield PQCVerifier.
func loadIntoVerifier(v *shield.PQCVerifier, sp signedPacket, algoID uint8, paramSet uint8, tier uint8, keyStatus uint8) {
	v.LoadSignature(sp.SigRef, &shield.SigEntry{
		AlgoID:    algoID,
		ParamSet:  paramSet,
		Tier:      tier,
		Signature: sp.Sig,
		CreatedAt: time.Now(),
	})
	v.LoadKey(sp.KeyRef, &shield.KeyEntry{
		AlgoID:    algoID,
		ParamSet:  paramSet,
		TrustTier: tier,
		Status:    keyStatus,
		PublicKey: sp.PubKey,
	})
}

// ---------------------------------------------------------------------------
// TestFullLifecycle tests: keygen -> sign -> verify -> rotate -> revoke
// ---------------------------------------------------------------------------

func TestFullLifecycle(t *testing.T) {
	// 1. Generate ML-DSA-65 keypair.
	kp, signer, err := pqc.GenerateMLDSAKeyPair(pqc.MLDSA65)
	if err != nil {
		t.Fatalf("GenerateMLDSAKeyPair: %v", err)
	}
	if kp.AlgoID != pqc.AlgoMLDSA {
		t.Fatalf("expected AlgoMLDSA, got %#x", kp.AlgoID)
	}

	// 3. Store in Sophia map.
	mgr := sophiaPkg.NewPQCMapManager("/tmp/test-sophia")
	if err := mgr.CreatePQCMaps(); err != nil {
		t.Fatalf("CreatePQCMaps: %v", err)
	}

	// 2+4. Build signed packet following the proper protocol flow.
	var sigRef uint32 = 0
	var keyRef uint16 = 0
	sp := makeSignedPacket(signer, kp.PublicKey, sigRef, keyRef, 1, 10, 11)

	// Store sig and key in Sophia.
	sophiaSigRef, err := mgr.LoadSignature(&sophiaPkg.PQCSigEntry{
		AlgoID:    uint8(pqc.AlgoMLDSA),
		ParamSet:  uint8(pqc.MLDSA65),
		Tier:      uint8(policy.TierStandard),
		Status:    0x01,
		Epoch:     1,
		CreatedAt: time.Now().Unix(),
		ExpiresAt: time.Now().Add(1 * time.Hour).Unix(),
		SigLen:    uint32(len(sp.Sig)),
		Signature: sp.Sig,
	})
	if err != nil {
		t.Fatalf("LoadSignature: %v", err)
	}
	sophiaKeyRef, err := mgr.LoadKey(&sophiaPkg.PQCKeyEntry{
		AlgoID:    uint8(pqc.AlgoMLDSA),
		ParamSet:  uint8(pqc.MLDSA65),
		TrustTier: uint8(policy.TierStandard),
		Status:    sophiaPkg.KeyStatusActive,
		KeyLen:    uint32(len(kp.PublicKey)),
		CreatedAt: time.Now().Unix(),
		ExpiresAt: time.Now().Add(24 * time.Hour).Unix(),
		PublicKey: kp.PublicKey,
	})
	if err != nil {
		t.Fatalf("LoadKey: %v", err)
	}

	// Verify Sophia refs match what we used for signing.
	if sophiaSigRef != sigRef {
		t.Fatalf("Sophia SigRef %d != expected %d", sophiaSigRef, sigRef)
	}
	if sophiaKeyRef != keyRef {
		t.Fatalf("Sophia KeyRef %d != expected %d", sophiaKeyRef, keyRef)
	}

	// Set up Shield verifier with real crypto.
	verifier := shield.NewPQCVerifier()
	verifier.SetCryptoVerify(realCryptoVerify)
	loadIntoVerifier(verifier, sp, uint8(pqc.AlgoMLDSA), uint8(pqc.MLDSA65),
		uint8(policy.TierStandard), sophiaPkg.KeyStatusActive)

	result := verifier.Verify(sp.Pkt)
	if !result.Passed {
		t.Fatalf("initial verification failed at step %s: %s", result.FailStep, result.FailMsg)
	}

	// 5. Rotate key (generate new, old in grace).
	kp2, signer2, err := pqc.GenerateMLDSAKeyPair(pqc.MLDSA65)
	if err != nil {
		t.Fatalf("GenerateMLDSAKeyPair (rotation): %v", err)
	}

	// Mark old key as rotating.
	verifier.LoadKey(keyRef, &shield.KeyEntry{
		AlgoID:    uint8(pqc.AlgoMLDSA),
		ParamSet:  uint8(pqc.MLDSA65),
		TrustTier: uint8(policy.TierStandard),
		Status:    sophiaPkg.KeyStatusRotating,
		PublicKey: kp.PublicKey,
	})

	// 6. Verify old key still works during grace (status = rotating).
	sp2 := makeSignedPacket(signer, kp.PublicKey, sigRef, keyRef, 2, 10, 11)
	verifier.LoadSignature(sp2.SigRef, &shield.SigEntry{
		AlgoID:    uint8(pqc.AlgoMLDSA),
		ParamSet:  uint8(pqc.MLDSA65),
		Tier:      uint8(policy.TierStandard),
		Signature: sp2.Sig,
		CreatedAt: time.Now(),
	})
	result = verifier.Verify(sp2.Pkt)
	if !result.Passed {
		t.Fatalf("old key during grace failed at step %s: %s", result.FailStep, result.FailMsg)
	}

	// Load new key.
	var newKeyRef uint16 = 1
	mgr.LoadKey(&sophiaPkg.PQCKeyEntry{
		AlgoID:    uint8(pqc.AlgoMLDSA),
		ParamSet:  uint8(pqc.MLDSA65),
		TrustTier: uint8(policy.TierStandard),
		Status:    sophiaPkg.KeyStatusActive,
		KeyLen:    uint32(len(kp2.PublicKey)),
		CreatedAt: time.Now().Unix(),
		ExpiresAt: time.Now().Add(24 * time.Hour).Unix(),
		PublicKey: kp2.PublicKey,
	})
	verifier.LoadKey(newKeyRef, &shield.KeyEntry{
		AlgoID:    uint8(pqc.AlgoMLDSA),
		ParamSet:  uint8(pqc.MLDSA65),
		TrustTier: uint8(policy.TierStandard),
		Status:    sophiaPkg.KeyStatusActive,
		PublicKey: kp2.PublicKey,
	})

	// Sign with new key and verify.
	var newSigRef uint32 = 1
	sp3 := makeSignedPacket(signer2, kp2.PublicKey, newSigRef, newKeyRef, 3, 10, 11)
	verifier.LoadSignature(sp3.SigRef, &shield.SigEntry{
		AlgoID:    uint8(pqc.AlgoMLDSA),
		ParamSet:  uint8(pqc.MLDSA65),
		Tier:      uint8(policy.TierStandard),
		Signature: sp3.Sig,
		CreatedAt: time.Now(),
	})
	result = verifier.Verify(sp3.Pkt)
	if !result.Passed {
		t.Fatalf("new key verification failed at step %s: %s", result.FailStep, result.FailMsg)
	}

	// 7. Revoke old key.
	verifier.LoadKey(keyRef, &shield.KeyEntry{
		AlgoID:    uint8(pqc.AlgoMLDSA),
		ParamSet:  uint8(pqc.MLDSA65),
		TrustTier: uint8(policy.TierStandard),
		Status:    sophiaPkg.KeyStatusRevoked,
		PublicKey: kp.PublicKey,
	})

	// 8. Verify old key now fails (status revoked).
	sp4 := makeSignedPacket(signer, kp.PublicKey, sigRef, keyRef, 4, 10, 11)
	verifier.LoadSignature(sp4.SigRef, &shield.SigEntry{
		AlgoID:    uint8(pqc.AlgoMLDSA),
		ParamSet:  uint8(pqc.MLDSA65),
		Tier:      uint8(policy.TierStandard),
		Signature: sp4.Sig,
		CreatedAt: time.Now(),
	})
	result = verifier.Verify(sp4.Pkt)
	if result.Passed {
		t.Fatal("revoked key verification should have failed, but passed")
	}
	if result.FailStep != shield.StepKeyRefLookup {
		t.Fatalf("expected failure at StepKeyRefLookup, got %s: %s", result.FailStep, result.FailMsg)
	}
}

// ---------------------------------------------------------------------------
// TestAllTiersCrossAlgorithm tests NONE/STANDARD/ENHANCED/SOVEREIGN x algos
// ---------------------------------------------------------------------------

func TestAllTiersCrossAlgorithm(t *testing.T) {
	type algoCase struct {
		name    string
		algoID  uint8
		ps      pqc.ParameterSet
		genKey  bool // true if real key generation is available
	}

	algos := []algoCase{
		{"ML-DSA-44", uint8(pqc.AlgoMLDSA), pqc.MLDSA44, true},
		{"ML-DSA-65", uint8(pqc.AlgoMLDSA), pqc.MLDSA65, true},
		{"ML-DSA-87", uint8(pqc.AlgoMLDSA), pqc.MLDSA87, true},
	}

	tiers := []policy.ComplianceTier{
		policy.TierNone,
		policy.TierStandard,
		policy.TierEnhanced,
		policy.TierSovereign,
	}

	for _, tier := range tiers {
		tierPolicy := policy.DefaultPolicyForTier(tier)

		for _, algo := range algos {
			t.Run(tierPolicy.Tier.String()+"/"+algo.name, func(t *testing.T) {
				if !algo.genKey {
					t.Skip("algorithm not available")
				}

				kp, signer, err := pqc.GenerateMLDSAKeyPair(algo.ps)
				if err != nil {
					t.Fatalf("keygen: %v", err)
				}

				msg := []byte("tier cross-algorithm test")
				sig, err := signer.Sign(msg)
				if err != nil {
					t.Fatalf("sign: %v", err)
				}

				verifierPQC, err := pqc.NewMLDSAVerifier(algo.ps)
				if err != nil {
					t.Fatalf("NewMLDSAVerifier: %v", err)
				}
				valid, err := verifierPQC.Verify(msg, sig, kp.PublicKey)
				if err != nil {
					t.Fatalf("verify: %v", err)
				}
				if !valid {
					t.Fatal("signature verification failed")
				}

				// Check policy compliance.
				vr := &policy.VerificationResult{
					Passed:        true,
					Tier:          tier,
					AlgoID:        algo.algoID,
					SigAge:        1 * time.Second,
					MultiSigCount: 0,
				}

				// For SOVEREIGN, multi-sig is required.
				if tier == policy.TierSovereign {
					vr.MultiSigCount = 2
				}

				err = tierPolicy.Validate(vr)

				// ML-DSA should be allowed in all tiers.
				isAllowed := tierPolicy.IsAlgoAllowed(algo.algoID)

				if isAllowed {
					// Check required algo constraint.
					if len(tierPolicy.RequiredAlgos) > 0 {
						found := false
						for _, req := range tierPolicy.RequiredAlgos {
							if req == algo.algoID {
								found = true
								break
							}
						}
						if !found && err != nil {
							// Expected: not in required list.
							return
						}
					}
					if err != nil {
						t.Errorf("expected policy pass for allowed algo, got: %v", err)
					}
				}
			})
		}
	}
}

// ---------------------------------------------------------------------------
// TestStubAlgorithmsReturnNotAvailable tests stubs return ErrAlgorithmNotAvailable
// ---------------------------------------------------------------------------

func TestStubAlgorithmsReturnNotAvailable(t *testing.T) {
	t.Run("SLH-DSA/keygen", func(t *testing.T) {
		_, err := pqc.GenerateSLHDSAKeyPair()
		if !errors.Is(err, pqc.ErrAlgorithmNotAvailable) {
			t.Fatalf("expected ErrAlgorithmNotAvailable, got: %v", err)
		}
	})

	t.Run("SLH-DSA/sign", func(t *testing.T) {
		s := &pqc.SLHDSASigner{}
		_, err := s.Sign([]byte("test"))
		if !errors.Is(err, pqc.ErrAlgorithmNotAvailable) {
			t.Fatalf("expected ErrAlgorithmNotAvailable, got: %v", err)
		}
	})

	t.Run("SLH-DSA/verify", func(t *testing.T) {
		v := &pqc.SLHDSAVerifier{}
		_, err := v.Verify([]byte("msg"), []byte("sig"), []byte("pk"))
		if !errors.Is(err, pqc.ErrAlgorithmNotAvailable) {
			t.Fatalf("expected ErrAlgorithmNotAvailable, got: %v", err)
		}
	})

	t.Run("FN-DSA/keygen", func(t *testing.T) {
		_, err := pqc.GenerateFNDSAKeyPair()
		if !errors.Is(err, pqc.ErrAlgorithmNotAvailable) {
			t.Fatalf("expected ErrAlgorithmNotAvailable, got: %v", err)
		}
	})

	t.Run("FN-DSA/sign", func(t *testing.T) {
		s := &pqc.FNDSASigner{}
		_, err := s.Sign([]byte("test"))
		if !errors.Is(err, pqc.ErrAlgorithmNotAvailable) {
			t.Fatalf("expected ErrAlgorithmNotAvailable, got: %v", err)
		}
	})

	t.Run("FN-DSA/verify", func(t *testing.T) {
		v := &pqc.FNDSAVerifier{}
		_, err := v.Verify([]byte("msg"), []byte("sig"), []byte("pk"))
		if !errors.Is(err, pqc.ErrAlgorithmNotAvailable) {
			t.Fatalf("expected ErrAlgorithmNotAvailable, got: %v", err)
		}
	})

	t.Run("HQC/keygen", func(t *testing.T) {
		_, err := pqc.GenerateHQCKeyPair(pqc.HQC128)
		if !errors.Is(err, pqc.ErrAlgorithmNotAvailable) {
			t.Fatalf("expected ErrAlgorithmNotAvailable, got: %v", err)
		}
	})

	t.Run("HQC/encapsulate", func(t *testing.T) {
		e := &pqc.HQCEncapsulator{}
		_, _, err := e.Encapsulate([]byte("pk"))
		if !errors.Is(err, pqc.ErrAlgorithmNotAvailable) {
			t.Fatalf("expected ErrAlgorithmNotAvailable, got: %v", err)
		}
	})

	t.Run("HQC/decapsulate", func(t *testing.T) {
		d := &pqc.HQCDecapsulator{}
		_, err := d.Decapsulate([]byte("ct"), []byte("sk"))
		if !errors.Is(err, pqc.ErrAlgorithmNotAvailable) {
			t.Fatalf("expected ErrAlgorithmNotAvailable, got: %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// TestKeyRotationMidFlow tests key rotation during active packet flow
// ---------------------------------------------------------------------------

func TestKeyRotationMidFlow(t *testing.T) {
	kp, signer, err := pqc.GenerateMLDSAKeyPair(pqc.MLDSA65)
	if err != nil {
		t.Fatalf("GenerateMLDSAKeyPair: %v", err)
	}

	verifier := shield.NewPQCVerifier()
	verifier.SetCryptoVerify(realCryptoVerify)

	var sigRef uint32 = 100
	var keyRef uint16 = 10

	// Simulate packets flowing: send 5 packets.
	for seq := uint16(1); seq <= 5; seq++ {
		sp := makeSignedPacket(signer, kp.PublicKey, sigRef, keyRef, seq, 10, 20)
		loadIntoVerifier(verifier, sp, uint8(pqc.AlgoMLDSA), uint8(pqc.MLDSA65),
			uint8(policy.TierStandard), sophiaPkg.KeyStatusActive)
		result := verifier.Verify(sp.Pkt)
		if !result.Passed {
			t.Fatalf("pre-rotation packet %d failed: %s: %s", seq, result.FailStep, result.FailMsg)
		}
	}

	// Rotate: mark old key as rotating.
	verifier.LoadKey(keyRef, &shield.KeyEntry{
		AlgoID:    uint8(pqc.AlgoMLDSA),
		ParamSet:  uint8(pqc.MLDSA65),
		TrustTier: uint8(policy.TierStandard),
		Status:    sophiaPkg.KeyStatusRotating,
		PublicKey: kp.PublicKey,
	})

	// Generate new key.
	kp2, signer2, err := pqc.GenerateMLDSAKeyPair(pqc.MLDSA65)
	if err != nil {
		t.Fatalf("GenerateMLDSAKeyPair (new): %v", err)
	}
	var newKeyRef uint16 = 11
	verifier.LoadKey(newKeyRef, &shield.KeyEntry{
		AlgoID:    uint8(pqc.AlgoMLDSA),
		ParamSet:  uint8(pqc.MLDSA65),
		TrustTier: uint8(policy.TierStandard),
		Status:    sophiaPkg.KeyStatusActive,
		PublicKey: kp2.PublicKey,
	})

	// Old key packets still work during grace.
	for seq := uint16(6); seq <= 8; seq++ {
		sp := makeSignedPacket(signer, kp.PublicKey, sigRef, keyRef, seq, 10, 20)
		verifier.LoadSignature(sp.SigRef, &shield.SigEntry{
			AlgoID:    uint8(pqc.AlgoMLDSA),
			ParamSet:  uint8(pqc.MLDSA65),
			Tier:      uint8(policy.TierStandard),
			Signature: sp.Sig,
			CreatedAt: time.Now(),
		})
		result := verifier.Verify(sp.Pkt)
		if !result.Passed {
			t.Fatalf("grace period packet %d failed: %s: %s", seq, result.FailStep, result.FailMsg)
		}
	}

	// New key packets also work.
	var newSigRef uint32 = 200
	sp := makeSignedPacket(signer2, kp2.PublicKey, newSigRef, newKeyRef, 9, 10, 20)
	verifier.LoadSignature(sp.SigRef, &shield.SigEntry{
		AlgoID:    uint8(pqc.AlgoMLDSA),
		ParamSet:  uint8(pqc.MLDSA65),
		Tier:      uint8(policy.TierStandard),
		Signature: sp.Sig,
		CreatedAt: time.Now(),
	})
	result := verifier.Verify(sp.Pkt)
	if !result.Passed {
		t.Fatalf("new key packet failed: %s: %s", result.FailStep, result.FailMsg)
	}

	// Revoke old key, simulate grace period expired.
	verifier.LoadKey(keyRef, &shield.KeyEntry{
		AlgoID:    uint8(pqc.AlgoMLDSA),
		ParamSet:  uint8(pqc.MLDSA65),
		TrustTier: uint8(policy.TierStandard),
		Status:    sophiaPkg.KeyStatusRevoked,
		PublicKey: kp.PublicKey,
	})

	// Old key packets now fail.
	spOld := makeSignedPacket(signer, kp.PublicKey, sigRef, keyRef, 10, 10, 20)
	verifier.LoadSignature(spOld.SigRef, &shield.SigEntry{
		AlgoID:    uint8(pqc.AlgoMLDSA),
		ParamSet:  uint8(pqc.MLDSA65),
		Tier:      uint8(policy.TierStandard),
		Signature: spOld.Sig,
		CreatedAt: time.Now(),
	})
	result = verifier.Verify(spOld.Pkt)
	if result.Passed {
		t.Fatal("expired/revoked key should fail verification")
	}
}

// ---------------------------------------------------------------------------
// TestRevocationMidFlow tests immediate revocation during active flow
// ---------------------------------------------------------------------------

func TestRevocationMidFlow(t *testing.T) {
	kp, signer, err := pqc.GenerateMLDSAKeyPair(pqc.MLDSA44)
	if err != nil {
		t.Fatalf("GenerateMLDSAKeyPair: %v", err)
	}

	verifier := shield.NewPQCVerifier()
	verifier.SetCryptoVerify(realCryptoVerify)

	var sigRef uint32 = 300
	var keyRef uint16 = 30

	// Packets flowing - should pass.
	for seq := uint16(1); seq <= 3; seq++ {
		sp := makeSignedPacket(signer, kp.PublicKey, sigRef, keyRef, seq, 30, 31)
		loadIntoVerifier(verifier, sp, uint8(pqc.AlgoMLDSA), uint8(pqc.MLDSA44),
			uint8(policy.TierStandard), sophiaPkg.KeyStatusActive)
		result := verifier.Verify(sp.Pkt)
		if !result.Passed {
			t.Fatalf("pre-revocation packet %d failed: %s", seq, result.FailMsg)
		}
	}

	// Immediate revocation.
	verifier.LoadKey(keyRef, &shield.KeyEntry{
		AlgoID:    uint8(pqc.AlgoMLDSA),
		ParamSet:  uint8(pqc.MLDSA44),
		TrustTier: uint8(policy.TierStandard),
		Status:    sophiaPkg.KeyStatusRevoked,
		PublicKey: kp.PublicKey,
	})

	// Next packet: immediate rejection.
	sp := makeSignedPacket(signer, kp.PublicKey, sigRef, keyRef, 4, 30, 31)
	verifier.LoadSignature(sp.SigRef, &shield.SigEntry{
		AlgoID:    uint8(pqc.AlgoMLDSA),
		ParamSet:  uint8(pqc.MLDSA44),
		Tier:      uint8(policy.TierStandard),
		Signature: sp.Sig,
		CreatedAt: time.Now(),
	})
	result := verifier.Verify(sp.Pkt)
	if result.Passed {
		t.Fatal("packet after revocation should fail")
	}
	if result.FailStep != shield.StepKeyRefLookup {
		t.Fatalf("expected failure at StepKeyRefLookup, got %s", result.FailStep)
	}
}

// ---------------------------------------------------------------------------
// TestTierTransition tests moving from lower to higher compliance tiers
// ---------------------------------------------------------------------------

func TestTierTransition(t *testing.T) {
	// Start at STANDARD.
	stdPolicy := policy.DefaultPolicyForTier(policy.TierStandard)

	// ML-DSA-65 at STANDARD: should pass.
	vr := &policy.VerificationResult{
		Passed: true,
		Tier:   policy.TierStandard,
		AlgoID: policy.AlgoMLDSA,
		SigAge: 30 * time.Second,
	}
	if err := stdPolicy.Validate(vr); err != nil {
		t.Fatalf("STANDARD validation failed: %v", err)
	}

	// Upgrade to ENHANCED.
	enhPolicy := policy.DefaultPolicyForTier(policy.TierEnhanced)

	// ML-DSA is required for ENHANCED; should pass.
	vr.Tier = policy.TierEnhanced
	if err := enhPolicy.Validate(vr); err != nil {
		t.Fatalf("ENHANCED validation with ML-DSA should pass: %v", err)
	}

	// ENHANCED requires PESSIMISTIC mode.
	if enhPolicy.Mode != policy.ModePessimistic {
		t.Fatalf("ENHANCED mode should be PESSIMISTIC, got %s", enhPolicy.Mode)
	}

	// Check MaxSigAge is stricter on ENHANCED.
	if enhPolicy.MaxSigAge >= stdPolicy.MaxSigAge {
		t.Fatalf("ENHANCED MaxSigAge (%s) should be stricter than STANDARD (%s)",
			enhPolicy.MaxSigAge, stdPolicy.MaxSigAge)
	}

	// Verify SOVEREIGN requires multi-sig.
	sovPolicy := policy.DefaultPolicyForTier(policy.TierSovereign)
	if !sovPolicy.RequireMultiSig {
		t.Fatal("SOVEREIGN should require multi-sig")
	}
	if sovPolicy.MinMultiSigCount != 2 {
		t.Fatalf("SOVEREIGN MinMultiSigCount should be 2, got %d", sovPolicy.MinMultiSigCount)
	}

	// Single-sig fails SOVEREIGN.
	vrSov := &policy.VerificationResult{
		Passed:        true,
		Tier:          policy.TierSovereign,
		AlgoID:        policy.AlgoMLDSA,
		SigAge:        5 * time.Second,
		MultiSigCount: 1,
	}
	err := sovPolicy.Validate(vrSov)
	if err == nil {
		t.Fatal("SOVEREIGN with 1 sig should fail validation")
	}

	// 2-of-3 passes.
	vrSov.MultiSigCount = 2
	if err := sovPolicy.Validate(vrSov); err != nil {
		t.Fatalf("SOVEREIGN with 2 sigs should pass: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TestPQCValueRoundTrip tests PQC value serialization through the full stack
// ---------------------------------------------------------------------------

func TestPQCValueRoundTrip(t *testing.T) {
	original := monad.PQCValue{
		SigRef:  0x00ABCDEF,
		KeyRef:  0x1234,
		HashPfx: [4]byte{0xDE, 0xAD, 0xBE, 0xEF},
		SeqNum:  0x5678,
	}

	// Marshal.
	buf := original.Marshal()
	if len(buf) != monad.PQCValueSize {
		t.Fatalf("Marshal size = %d, want %d", len(buf), monad.PQCValueSize)
	}

	// Unmarshal.
	restored, err := monad.UnmarshalPQCValue(buf[:])
	if err != nil {
		t.Fatalf("UnmarshalPQCValue: %v", err)
	}

	// Verify all fields.
	if restored.SigRef != original.SigRef {
		t.Errorf("SigRef = %#x, want %#x", restored.SigRef, original.SigRef)
	}
	if restored.KeyRef != original.KeyRef {
		t.Errorf("KeyRef = %#x, want %#x", restored.KeyRef, original.KeyRef)
	}
	if restored.HashPfx != original.HashPfx {
		t.Errorf("HashPfx = %x, want %x", restored.HashPfx, original.HashPfx)
	}
	if restored.SeqNum != original.SeqNum {
		t.Errorf("SeqNum = %#x, want %#x", restored.SeqNum, original.SeqNum)
	}

	// Verify IsZero for zero value.
	zero := monad.PQCValue{}
	if !zero.IsZero() {
		t.Error("zero PQCValue should be IsZero")
	}
	if original.IsZero() {
		t.Error("non-zero PQCValue should not be IsZero")
	}

	// Round-trip through Sophia map storage.
	mgr := sophiaPkg.NewPQCMapManager("/tmp/test-sophia-rt")
	if err := mgr.CreatePQCMaps(); err != nil {
		t.Fatalf("CreatePQCMaps: %v", err)
	}

	_, signer, err := pqc.GenerateMLDSAKeyPair(pqc.MLDSA65)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	sig, err := signer.Sign([]byte("round-trip test"))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	sigRef, err := mgr.LoadSignature(&sophiaPkg.PQCSigEntry{
		AlgoID:    uint8(pqc.AlgoMLDSA),
		ParamSet:  uint8(pqc.MLDSA65),
		Tier:      uint8(policy.TierStandard),
		Status:    0x01,
		CreatedAt: time.Now().Unix(),
		ExpiresAt: time.Now().Add(1 * time.Hour).Unix(),
		SigLen:    uint32(len(sig)),
		Signature: sig,
	})
	if err != nil {
		t.Fatalf("LoadSignature: %v", err)
	}

	// Lookup and verify contents.
	looked, err := mgr.LookupSignature(sigRef)
	if err != nil {
		t.Fatalf("LookupSignature: %v", err)
	}
	if !bytes.Equal(looked.Signature, sig) {
		t.Error("retrieved signature does not match stored signature")
	}
	if looked.AlgoID != uint8(pqc.AlgoMLDSA) {
		t.Errorf("AlgoID = %#x, want %#x", looked.AlgoID, pqc.AlgoMLDSA)
	}
}

// ---------------------------------------------------------------------------
// TestPseudoHeaderSigning tests end-to-end pseudo-header signing
// ---------------------------------------------------------------------------

func TestPseudoHeaderSigning(t *testing.T) {
	kp, signer, err := pqc.GenerateMLDSAKeyPair(pqc.MLDSA65)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}

	srcIP := [16]byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
	dstIP := [16]byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2}
	monadPfx := [8]byte{0x01, 0x0A, 0x0B, 0x00, 0x03, 0x01, 0x01, monad.FlagPQCActive}
	pqcValBytes := [12]byte{0, 0, 0, 42, 0, 7, 0xDE, 0xAD, 0xBE, 0xEF, 0, 1}

	ph := monad.BuildPseudoHeader(srcIP, dstIP, monadPfx, pqcValBytes)
	phBuf := ph.Marshal()

	// Sign the pseudo-header.
	sig, err := signer.Sign(phBuf[:])
	if err != nil {
		t.Fatalf("Sign pseudo-header: %v", err)
	}

	// Verify signature.
	verifierPQC, err := pqc.NewMLDSAVerifier(pqc.MLDSA65)
	if err != nil {
		t.Fatalf("NewMLDSAVerifier: %v", err)
	}
	valid, err := verifierPQC.Verify(phBuf[:], sig, kp.PublicKey)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !valid {
		t.Fatal("pseudo-header signature verification failed")
	}

	// Check HashPfx matches.
	hashPfx := pqc.HashPrefix(sig)
	if !pqc.VerifyHashPrefix(hashPfx, sig) {
		t.Fatal("HashPfx does not match signature")
	}

	// Verify the pseudo-header layout.
	if len(phBuf) != monad.PseudoHeaderSize {
		t.Fatalf("pseudo-header size = %d, want %d", len(phBuf), monad.PseudoHeaderSize)
	}
	if phBuf[0] != srcIP[0] {
		t.Errorf("srcIP byte 0: got %#x, want %#x", phBuf[0], srcIP[0])
	}
	if phBuf[16] != dstIP[0] {
		t.Errorf("dstIP byte 0: got %#x, want %#x", phBuf[16], dstIP[0])
	}
	if phBuf[32] != monadPfx[0] {
		t.Errorf("monadPfx byte 0: got %#x, want %#x", phBuf[32], monadPfx[0])
	}
}

// ---------------------------------------------------------------------------
// TestReplayProtection tests that duplicate SeqNums are detected
// ---------------------------------------------------------------------------

func TestReplayProtection(t *testing.T) {
	kp, signer, err := pqc.GenerateMLDSAKeyPair(pqc.MLDSA44)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}

	verifier := shield.NewPQCVerifier()
	verifier.SetCryptoVerify(realCryptoVerify)

	var sigRef uint32 = 500
	var keyRef uint16 = 50

	// First send: should pass.
	sp := makeSignedPacket(signer, kp.PublicKey, sigRef, keyRef, 42, 50, 51)
	loadIntoVerifier(verifier, sp, uint8(pqc.AlgoMLDSA), uint8(pqc.MLDSA44),
		uint8(policy.TierStandard), sophiaPkg.KeyStatusActive)

	result := verifier.Verify(sp.Pkt)
	if !result.Passed {
		t.Fatalf("first packet failed: %s: %s", result.FailStep, result.FailMsg)
	}

	// Same SeqNum: should fail (replay).
	result = verifier.Verify(sp.Pkt)
	if result.Passed {
		t.Fatal("replay packet should have failed")
	}
	if result.FailStep != shield.StepSeqNumReplay {
		t.Fatalf("expected failure at StepSeqNumReplay, got %s: %s", result.FailStep, result.FailMsg)
	}

	// Different SeqNum: should pass.
	sp2 := makeSignedPacket(signer, kp.PublicKey, sigRef, keyRef, 43, 50, 51)
	verifier.LoadSignature(sp2.SigRef, &shield.SigEntry{
		AlgoID:    uint8(pqc.AlgoMLDSA),
		ParamSet:  uint8(pqc.MLDSA44),
		Tier:      uint8(policy.TierStandard),
		Signature: sp2.Sig,
		CreatedAt: time.Now(),
	})
	result = verifier.Verify(sp2.Pkt)
	if !result.Passed {
		t.Fatalf("new seqnum packet failed: %s: %s", result.FailStep, result.FailMsg)
	}
}

// ---------------------------------------------------------------------------
// TestWotanStateSync tests PQC state synchronization via Wotan
// ---------------------------------------------------------------------------

func TestWotanStateSync(t *testing.T) {
	mgr := wotan.NewPQCStateManager()

	// Initial state should be zero.
	s := mgr.Read()
	if s.SigCount != 0 || s.KeyCount != 0 || s.VerifyPass != 0 || s.VerifyFail != 0 {
		t.Fatal("initial state should be all zeros")
	}

	// Update counters.
	mgr.UpdateSigCount(10)
	mgr.UpdateKeyCount(5)
	mgr.IncrVerifyPass()
	mgr.IncrVerifyPass()
	mgr.IncrVerifyPass()
	mgr.IncrVerifyFail()

	// Read snapshot and verify consistency.
	s = mgr.Read()
	if s.SigCount != 10 {
		t.Errorf("SigCount = %d, want 10", s.SigCount)
	}
	if s.KeyCount != 5 {
		t.Errorf("KeyCount = %d, want 5", s.KeyCount)
	}
	if s.VerifyPass != 3 {
		t.Errorf("VerifyPass = %d, want 3", s.VerifyPass)
	}
	if s.VerifyFail != 1 {
		t.Errorf("VerifyFail = %d, want 1", s.VerifyFail)
	}

	// Set policy/tier.
	mgr.SetPolicyMode(uint8(policy.ModePessimistic))
	mgr.SetActiveTier(uint8(policy.TierEnhanced))
	mgr.SetLastRotation(time.Now().Unix())

	s = mgr.Read()
	if s.PolicyMode != uint8(policy.ModePessimistic) {
		t.Errorf("PolicyMode = %#x, want %#x", s.PolicyMode, policy.ModePessimistic)
	}
	if s.ActiveTier != uint8(policy.TierEnhanced) {
		t.Errorf("ActiveTier = %#x, want %#x", s.ActiveTier, policy.TierEnhanced)
	}
	if s.LastRotation == 0 {
		t.Error("LastRotation should be non-zero after SetLastRotation")
	}

	// Marshal/Unmarshal round-trip.
	buf := s.Marshal()
	restored, err := wotan.UnmarshalPQCState(buf[:])
	if err != nil {
		t.Fatalf("UnmarshalPQCState: %v", err)
	}
	if restored != s {
		t.Errorf("state round-trip mismatch: got %+v, want %+v", restored, s)
	}

	// CAS: compare-and-swap.
	current := mgr.Read()
	desired := current
	desired.SigCount = 20
	if !mgr.CAS(current, desired) {
		t.Fatal("CAS should succeed when expected matches current")
	}
	if mgr.Read().SigCount != 20 {
		t.Errorf("SigCount after CAS = %d, want 20", mgr.Read().SigCount)
	}

	// CAS with stale expected should fail.
	stale := current // uses old SigCount=10
	desired.SigCount = 30
	if mgr.CAS(stale, desired) {
		t.Fatal("CAS should fail with stale expected state")
	}
}

// ---------------------------------------------------------------------------
// TestSovereignMultiSig tests 2-of-3 cross-algorithm multi-sig
// ---------------------------------------------------------------------------

func TestSovereignMultiSig(t *testing.T) {
	// Generate 3 keypairs.
	// Slot 1: SLH-DSA stub (will fail - not available).
	// Slot 2: ML-DSA-44.
	// Slot 3: ML-DSA-65.
	kp44, signer44, err := pqc.GenerateMLDSAKeyPair(pqc.MLDSA44)
	if err != nil {
		t.Fatalf("GenerateMLDSAKeyPair(44): %v", err)
	}
	kp65, signer65, err := pqc.GenerateMLDSAKeyPair(pqc.MLDSA65)
	if err != nil {
		t.Fatalf("GenerateMLDSAKeyPair(65): %v", err)
	}

	pqcVerifier := shield.NewPQCVerifier()
	pqcVerifier.SetCryptoVerify(realCryptoVerify)

	// SLH-DSA stub: SigRef 1000.
	var sigRefSLH uint32 = 1000
	pqcVerifier.LoadSignature(sigRefSLH, &shield.SigEntry{
		AlgoID:    uint8(pqc.AlgoSLHDSA),
		ParamSet:  0x01,
		Tier:      uint8(policy.TierSovereign),
		Signature: []byte("stub-not-real"),
		CreatedAt: time.Now(),
	})
	pqcVerifier.LoadKey(100, &shield.KeyEntry{
		AlgoID:    uint8(pqc.AlgoSLHDSA),
		ParamSet:  0x01,
		TrustTier: uint8(policy.TierSovereign),
		Status:    sophiaPkg.KeyStatusActive,
		PublicKey: []byte("stub-pk-not-real"),
	})

	// ML-DSA-44: SigRef 1001, KeyRef 101.
	var sigRef44 uint32 = 1001
	var keyRef44 uint16 = 101
	sp44 := makeSignedPacket(signer44, kp44.PublicKey, sigRef44, keyRef44, 1, 100, 101)
	pqcVerifier.LoadSignature(sigRef44, &shield.SigEntry{
		AlgoID:    uint8(pqc.AlgoMLDSA),
		ParamSet:  uint8(pqc.MLDSA44),
		Tier:      uint8(policy.TierSovereign),
		Signature: sp44.Sig,
		CreatedAt: time.Now(),
	})
	pqcVerifier.LoadKey(keyRef44, &shield.KeyEntry{
		AlgoID:    uint8(pqc.AlgoMLDSA),
		ParamSet:  uint8(pqc.MLDSA44),
		TrustTier: uint8(policy.TierSovereign),
		Status:    sophiaPkg.KeyStatusActive,
		PublicKey: kp44.PublicKey,
	})

	// ML-DSA-65: SigRef 1002, KeyRef 102.
	var sigRef65 uint32 = 1002
	var keyRef65 uint16 = 102
	sp65 := makeSignedPacket(signer65, kp65.PublicKey, sigRef65, keyRef65, 1, 100, 101)
	pqcVerifier.LoadSignature(sigRef65, &shield.SigEntry{
		AlgoID:    uint8(pqc.AlgoMLDSA),
		ParamSet:  uint8(pqc.MLDSA65),
		Tier:      uint8(policy.TierSovereign),
		Signature: sp65.Sig,
		CreatedAt: time.Now(),
	})
	pqcVerifier.LoadKey(keyRef65, &shield.KeyEntry{
		AlgoID:    uint8(pqc.AlgoMLDSA),
		ParamSet:  uint8(pqc.MLDSA65),
		TrustTier: uint8(policy.TierSovereign),
		Status:    sophiaPkg.KeyStatusActive,
		PublicKey: kp65.PublicKey,
	})

	sovVerifier := shield.NewSovereignVerifier(pqcVerifier)

	// Use sp44 as the base packet for the sovereign verification.
	basePkt := sp44.Pkt

	sov := &shield.SovereignSig{
		Sigs: [3]shield.SovereignSigSlot{
			{SigRef: sigRefSLH, AlgoID: uint8(pqc.AlgoSLHDSA)},   // Will fail (stub)
			{SigRef: sigRef44, AlgoID: uint8(pqc.AlgoMLDSA)},      // Will pass
			{SigRef: sigRef65, AlgoID: uint8(pqc.AlgoMLDSA)},      // Will pass
		},
	}

	result := sovVerifier.VerifySovereign(basePkt, sov)

	// SLH-DSA will fail crypto verify (stub). Both ML-DSA sigs are from
	// "lattice" family, so distinct families = 1. ValidCount should be 2
	// but distinct families < 2, so SOVEREIGN consensus fails.
	t.Logf("ValidCount = %d, DistinctFamilies = %d, Passed = %v",
		result.ValidCount, result.DistinctFamilies, result.Passed)

	// With both ML-DSA sigs valid, we get 2 valid but only 1 distinct family.
	if result.ValidCount >= 2 && result.DistinctFamilies >= 2 {
		// Sovereign passes.
		if !result.Passed {
			t.Fatal("sovereign with 2 valid distinct families should pass")
		}
	} else {
		// Expected: insufficient distinct families for true sovereign.
		if result.Passed {
			t.Fatal("sovereign should not pass with < 2 distinct families")
		}
	}

	// Test Sophia EvaluateConsensus helper.
	sophiaEntry := &sophiaPkg.SovereignSigEntry{
		SigRef1: sigRefSLH, AlgoID1: uint8(pqc.AlgoSLHDSA), Status1: 0x02, // failed
		SigRef2: sigRef44, AlgoID2: uint8(pqc.AlgoMLDSA), Status2: 0x01, // passed
		SigRef3: sigRef65, AlgoID3: uint8(pqc.AlgoMLDSA), Status3: 0x01, // passed
	}
	consensus := sophiaPkg.EvaluateConsensus(sophiaEntry)
	if consensus != sophiaPkg.ConsensusPassed {
		t.Fatalf("EvaluateConsensus: expected ConsensusPassed, got %#x", consensus)
	}

	// Test 1-of-3: only one passes -> ConsensusPartial.
	sophiaEntry2 := &sophiaPkg.SovereignSigEntry{
		SigRef1: sigRefSLH, AlgoID1: uint8(pqc.AlgoSLHDSA), Status1: 0x02,
		SigRef2: sigRef44, AlgoID2: uint8(pqc.AlgoMLDSA), Status2: 0x01,
		SigRef3: sigRef65, AlgoID3: uint8(pqc.AlgoMLDSA), Status3: 0x02,
	}
	consensus2 := sophiaPkg.EvaluateConsensus(sophiaEntry2)
	if consensus2 != sophiaPkg.ConsensusPartial {
		t.Fatalf("EvaluateConsensus(1-of-3): expected ConsensusPartial, got %#x", consensus2)
	}

	// Test 0-of-3: all fail -> ConsensusFailed.
	sophiaEntry3 := &sophiaPkg.SovereignSigEntry{
		SigRef1: sigRefSLH, AlgoID1: uint8(pqc.AlgoSLHDSA), Status1: 0x02,
		SigRef2: sigRef44, AlgoID2: uint8(pqc.AlgoMLDSA), Status2: 0x02,
		SigRef3: sigRef65, AlgoID3: uint8(pqc.AlgoMLDSA), Status3: 0x02,
	}
	consensus3 := sophiaPkg.EvaluateConsensus(sophiaEntry3)
	if consensus3 != sophiaPkg.ConsensusFailed {
		t.Fatalf("EvaluateConsensus(0-of-3): expected ConsensusFailed, got %#x", consensus3)
	}
}

// ---------------------------------------------------------------------------
// TestMLKEMFullRoundTrip tests ML-KEM encapsulation/decapsulation E2E
// ---------------------------------------------------------------------------

func TestMLKEMFullRoundTrip(t *testing.T) {
	params := []struct {
		name string
		ps   pqc.ParameterSet
	}{
		{"ML-KEM-512", pqc.MLKEM512},
		{"ML-KEM-768", pqc.MLKEM768},
		{"ML-KEM-1024", pqc.MLKEM1024},
	}

	for _, tt := range params {
		t.Run(tt.name, func(t *testing.T) {
			kp, err := pqc.GenerateMLKEMKeyPair(tt.ps)
			if err != nil {
				t.Fatalf("GenerateMLKEMKeyPair: %v", err)
			}

			encap, err := pqc.NewMLKEMEncapsulator(tt.ps)
			if err != nil {
				t.Fatalf("NewMLKEMEncapsulator: %v", err)
			}

			ct, ss1, err := encap.Encapsulate(kp.PublicKey)
			if err != nil {
				t.Fatalf("Encapsulate: %v", err)
			}

			decap, err := pqc.NewMLKEMDecapsulator(tt.ps)
			if err != nil {
				t.Fatalf("NewMLKEMDecapsulator: %v", err)
			}

			ss2, err := decap.Decapsulate(ct, kp.SecretKey)
			if err != nil {
				t.Fatalf("Decapsulate: %v", err)
			}

			if !bytes.Equal(ss1, ss2) {
				t.Fatal("shared secrets do not match")
			}

			// Verify shared secret is non-trivial.
			allZero := true
			for _, b := range ss1 {
				if b != 0 {
					allZero = false
					break
				}
			}
			if allZero {
				t.Fatal("shared secret is all zeros")
			}

			// Store in Sophia KEM map.
			mgr := sophiaPkg.NewPQCMapManager("/tmp/test-kem-rt")
			if err := mgr.CreatePQCMaps(); err != nil {
				t.Fatalf("CreatePQCMaps: %v", err)
			}

			var ss32 [32]byte
			copy(ss32[:], ss1)

			kemEntry := &sophiaPkg.KEMKeyEntry{
				AlgoID:       uint8(pqc.AlgoMLKEM),
				ParamSet:     uint8(tt.ps),
				Status:       sophiaPkg.KeyStatusActive,
				PeerService:  20,
				TunnelID:     1,
				SharedSecret: ss32,
				CreatedAt:    time.Now().Unix(),
				ExpiresAt:    time.Now().Add(1 * time.Hour).Unix(),
			}
			err = mgr.LoadKEMKey(1, kemEntry)
			if err != nil {
				t.Fatalf("LoadKEMKey: %v", err)
			}

			looked, err := mgr.LookupKEMKey(1)
			if err != nil {
				t.Fatalf("LookupKEMKey: %v", err)
			}
			if looked.SharedSecret != ss32 {
				t.Error("retrieved shared secret does not match")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestKEMTunnelLifecycle tests KEM tunnel initiation/completion/close
// ---------------------------------------------------------------------------

func TestKEMTunnelLifecycle(t *testing.T) {
	tunnelMgr := shield.NewKEMTunnelManager()

	// Generate ML-KEM-768 keys.
	kp, err := pqc.GenerateMLKEMKeyPair(pqc.MLKEM768)
	if err != nil {
		t.Fatalf("GenerateMLKEMKeyPair: %v", err)
	}

	// Initiate tunnel.
	tunnel, err := tunnelMgr.InitiateTunnel(10, 20, uint8(pqc.AlgoMLKEM), uint8(pqc.MLKEM768))
	if err != nil {
		t.Fatalf("InitiateTunnel: %v", err)
	}
	if tunnel.State != shield.TunnelInit {
		t.Fatalf("tunnel state = %s, want init", tunnel.State)
	}

	// Encapsulate.
	encap, err := pqc.NewMLKEMEncapsulator(pqc.MLKEM768)
	if err != nil {
		t.Fatalf("NewMLKEMEncapsulator: %v", err)
	}
	ct, ss, err := encap.Encapsulate(kp.PublicKey)
	if err != nil {
		t.Fatalf("Encapsulate: %v", err)
	}
	tunnel.Ciphertext = ct

	// Complete tunnel with shared secret.
	var ss32 [32]byte
	copy(ss32[:], ss)
	if err := tunnelMgr.CompleteTunnel(tunnel.ID, ss32); err != nil {
		t.Fatalf("CompleteTunnel: %v", err)
	}

	got, ok := tunnelMgr.GetTunnel(tunnel.ID)
	if !ok {
		t.Fatal("tunnel not found after completion")
	}
	if got.State != shield.TunnelActive {
		t.Fatalf("tunnel state = %s, want active", got.State)
	}
	if tunnelMgr.ActiveCount() != 1 {
		t.Fatalf("ActiveCount = %d, want 1", tunnelMgr.ActiveCount())
	}

	// Close tunnel.
	if err := tunnelMgr.CloseTunnel(tunnel.ID); err != nil {
		t.Fatalf("CloseTunnel: %v", err)
	}
	got, _ = tunnelMgr.GetTunnel(tunnel.ID)
	if got.State != shield.TunnelClosed {
		t.Fatalf("tunnel state = %s, want closed", got.State)
	}
	// Verify shared secret is zeroed.
	var zero32 [32]byte
	if got.SharedSecret != zero32 {
		t.Fatal("shared secret should be zeroed after close")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// TestTierMatrixAllAlgos — 4 tiers x all available algorithms (E2E)
// ---------------------------------------------------------------------------

func TestTierMatrixAllAlgos(t *testing.T) {
	type testCase struct {
		name    string
		algoID  uint8
		ps      pqc.ParameterSet
		genKey  bool
	}

	algos := []testCase{
		{"ML-DSA-44", uint8(pqc.AlgoMLDSA), pqc.MLDSA44, true},
		{"ML-DSA-65", uint8(pqc.AlgoMLDSA), pqc.MLDSA65, true},
		{"ML-DSA-87", uint8(pqc.AlgoMLDSA), pqc.MLDSA87, true},
	}

	tiers := []policy.ComplianceTier{
		policy.TierNone,
		policy.TierStandard,
		policy.TierEnhanced,
		policy.TierSovereign,
	}

	for _, tier := range tiers {
		for _, algo := range algos {
			t.Run(tier.String()+"/"+algo.name, func(t *testing.T) {
				if !algo.genKey {
					t.Skip("algorithm not available")
				}

				kp, signer, err := pqc.GenerateMLDSAKeyPair(algo.ps)
				if err != nil {
					t.Fatalf("keygen: %v", err)
				}

				// Build verification infrastructure.
				verifier := shield.NewPQCVerifier()
				verifier.SetCryptoVerify(realCryptoVerify)
				sovereign := shield.NewSovereignVerifier(verifier)
				policyEng := shield.NewLayer2PolicyEngine()
				stateMgr := wotan.NewPQCStateManager()

				tv := shield.NewTierVerifier(verifier, sovereign, policyEng, stateMgr)
				tv.TransitionTier(tier)

				var sigRef uint32 = 0
				var keyRef uint16 = 0
				sp := makeSignedPacket(signer, kp.PublicKey, sigRef, keyRef, 1, 10, 11)
				loadIntoVerifier(verifier, sp, algo.algoID, uint8(algo.ps), uint8(tier), sophiaPkg.KeyStatusActive)

				result := tv.VerifyPacket(sp.Pkt, nil)

				switch tier {
				case policy.TierNone:
					if !result.Passed() {
						t.Fatal("NONE tier should always pass (skip)")
					}
				case policy.TierStandard:
					if !result.Passed() {
						t.Fatalf("STANDARD should pass for %s: %v", algo.name, result.PQCResult.FailMsg)
					}
				case policy.TierEnhanced:
					if result.PQCResult != nil && result.PQCResult.Passed {
						t.Logf("ENHANCED passed for %s", algo.name)
					}
				case policy.TierSovereign:
					// Single sig should fail SOVEREIGN (requires multi-sig).
					if result.Passed() {
						t.Fatal("SOVEREIGN should fail with single sig")
					}
				}

				// Verify Wotan state updated.
				s := stateMgr.Read()
				if tier == policy.TierNone {
					if s.VerifyPass == 0 {
						t.Error("NONE tier should still increment verify pass")
					}
				}
			})
		}
	}
}

// ---------------------------------------------------------------------------
// TestSovereignConsensusE2E — real multi-sig with pre-computation
// ---------------------------------------------------------------------------

func TestSovereignConsensusE2E(t *testing.T) {
	kp44, signer44, err := pqc.GenerateMLDSAKeyPair(pqc.MLDSA44)
	if err != nil {
		t.Fatalf("keygen 44: %v", err)
	}
	kp65, signer65, err := pqc.GenerateMLDSAKeyPair(pqc.MLDSA65)
	if err != nil {
		t.Fatalf("keygen 65: %v", err)
	}

	mgr := sophiaPkg.NewPQCMapManager("/tmp/test-sovereign-e2e")
	if err := mgr.CreatePQCMaps(); err != nil {
		t.Fatalf("CreatePQCMaps: %v", err)
	}

	pqcVerifier := shield.NewPQCVerifier()
	pqcVerifier.SetCryptoVerify(realCryptoVerify)
	sovVerifier := shield.NewSovereignVerifier(pqcVerifier)

	// Pre-compute sovereign signatures.
	pseudoHeader := []byte("sovereign-consensus-test-pseudo-header-data")

	// Note: both ML-DSA are "lattice" family, so we can't get true sovereign
	// consensus without SLH-DSA. Test the flow anyway.
	signers := []shield.SovereignSigner{
		{
			AlgoID:   uint8(pqc.AlgoMLDSA),
			ParamSet: uint8(pqc.MLDSA44),
			Sign:     signer44.Sign,
		},
		{
			AlgoID:   uint8(pqc.AlgoMLDSA),
			ParamSet: uint8(pqc.MLDSA65),
			Sign:     signer65.Sign,
		},
	}

	// This should fail because both are lattice family.
	_, err = sovVerifier.PrecomputeSovereignSigs(pseudoHeader, signers, mgr)
	if err == nil {
		t.Fatal("expected family diversity error")
	}

	// Verify the individual sigs work through the verifier.
	sp44 := makeSignedPacket(signer44, kp44.PublicKey, 0, 0, 1, 100, 101)
	loadIntoVerifier(pqcVerifier, sp44, uint8(pqc.AlgoMLDSA), uint8(pqc.MLDSA44),
		uint8(policy.TierSovereign), sophiaPkg.KeyStatusActive)
	result := pqcVerifier.Verify(sp44.Pkt)
	if !result.Passed {
		t.Fatalf("ML-DSA-44 individual verify failed: %s", result.FailMsg)
	}

	sp65 := makeSignedPacket(signer65, kp65.PublicKey, 1, 1, 2, 100, 101)
	loadIntoVerifier(pqcVerifier, sp65, uint8(pqc.AlgoMLDSA), uint8(pqc.MLDSA65),
		uint8(policy.TierSovereign), sophiaPkg.KeyStatusActive)
	result = pqcVerifier.Verify(sp65.Pkt)
	if !result.Passed {
		t.Fatalf("ML-DSA-65 individual verify failed: %s", result.FailMsg)
	}
}

// ---------------------------------------------------------------------------
// TestPolicyEnforcementE2E — YAML policy → PolicyChecker
// ---------------------------------------------------------------------------

func TestPolicyEnforcementE2E(t *testing.T) {
	yamlPolicy := []byte(`
version: "1.0"
policies:
  - service_id: 100
    service_name: "critical-service"
    allowed_algos: [2]
    min_nist_level: 3
    max_key_age_sec: 3600
    require_pqc: true
  - service_id: 200
    service_name: "optional-service"
    allowed_algos: [1, 2, 3]
    min_nist_level: 1
    max_key_age_sec: 86400
    require_pqc: false
`)

	pf, err := pqcPolicy.ParsePolicyBytes(yamlPolicy)
	if err != nil {
		t.Fatalf("ParsePolicyBytes: %v", err)
	}

	checker := pqcPolicy.NewChecker()
	checker.LoadPolicies(pf)

	ctx := t.Context()

	// Critical service: ML-DSA-65 should pass (Level 3).
	r := checker.Check(ctx, pqcPolicy.PolicyCheckRequest{
		ServiceID:    100,
		AlgoID:       0x02, // ML-DSA
		ParamSet:     0x02, // ML-DSA-65
		PQCVerified:  true,
		KeyCreatedAt: time.Now(),
	})
	if !r.Allowed {
		t.Fatalf("critical-service ML-DSA-65 should pass: step %d: %s", r.FailStep, r.FailReason)
	}

	// Critical service: ML-DSA-44 should fail (Level 2 < required Level 3).
	r = checker.Check(ctx, pqcPolicy.PolicyCheckRequest{
		ServiceID:    100,
		AlgoID:       0x02,
		ParamSet:     0x01, // ML-DSA-44 = Level 2
		PQCVerified:  true,
		KeyCreatedAt: time.Now(),
	})
	if r.Allowed {
		t.Fatal("critical-service ML-DSA-44 should fail (NIST level too low)")
	}

	// Optional service: no PQC is fine.
	r = checker.Check(ctx, pqcPolicy.PolicyCheckRequest{
		ServiceID:   200,
		PQCVerified: false,
	})
	if !r.Allowed {
		t.Fatal("optional-service without PQC should pass")
	}
}

// ---------------------------------------------------------------------------
// TestKEMTunnelHKDFE2E — full tunnel negotiation with HKDF
// ---------------------------------------------------------------------------

func TestKEMTunnelHKDFE2E(t *testing.T) {
	params := []struct {
		name string
		ps   pqc.ParameterSet
	}{
		{"ML-KEM-512", pqc.MLKEM512},
		{"ML-KEM-768", pqc.MLKEM768},
		{"ML-KEM-1024", pqc.MLKEM1024},
	}

	for _, tt := range params {
		t.Run(tt.name, func(t *testing.T) {
			kp, err := pqc.GenerateMLKEMKeyPair(tt.ps)
			if err != nil {
				t.Fatalf("keygen: %v", err)
			}

			neg, err := pqc.NewTunnelNegotiator(tt.ps)
			if err != nil {
				t.Fatalf("NewTunnelNegotiator: %v", err)
			}

			salt := []byte("e2e-test-salt")
			ctx := "e2e-tunnel"

			// Full handshake.
			initResult, err := neg.InitiatorHandshake(kp.PublicKey, salt, ctx)
			if err != nil {
				t.Fatalf("InitiatorHandshake: %v", err)
			}
			defer initResult.Zero()

			respResult, err := neg.ResponderHandshake(initResult.Ciphertext, kp.SecretKey, salt, ctx)
			if err != nil {
				t.Fatalf("ResponderHandshake: %v", err)
			}
			defer respResult.Zero()

			// Verify shared secrets match.
			if initResult.SharedSecret != respResult.SharedSecret {
				t.Fatal("shared secrets mismatch")
			}

			// Verify derived keys match.
			if initResult.Keys.SigningKey != respResult.Keys.SigningKey {
				t.Fatal("signing keys mismatch")
			}
			if initResult.Keys.AuthKey != respResult.Keys.AuthKey {
				t.Fatal("auth keys mismatch")
			}

			// Verify HKDF signing key derivation.
			flowID := []byte("e2e-flow-001")
			key1, err := pqc.DeriveSigningKey(initResult.SharedSecret, salt, flowID, 1)
			if err != nil {
				t.Fatalf("DeriveSigningKey: %v", err)
			}
			key2, err := pqc.DeriveSigningKey(respResult.SharedSecret, salt, flowID, 1)
			if err != nil {
				t.Fatalf("DeriveSigningKey responder: %v", err)
			}
			if key1 != key2 {
				t.Fatal("HKDF signing keys should match for same shared secret")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestSecurityHardeningE2E — algo confusion, TOCTOU, entropy
// ---------------------------------------------------------------------------

func TestSecurityHardeningE2E(t *testing.T) {
	sh := shield.NewSecurityHardening()

	// Test algo confusion detection.
	pkt1 := &shield.PQCPacketInfo{
		Flags:  0x0A,
		SigRef: 1,
		KeyRef: 1,
		SeqNum: 1,
	}
	if err := sh.ValidateWithAlgo(pkt1, "flow-sec-1", 0x02); err != nil {
		t.Fatalf("first algo should pass: %v", err)
	}

	pkt2 := &shield.PQCPacketInfo{
		Flags:  0x0A,
		SigRef: 2,
		KeyRef: 1,
		SeqNum: 2,
	}
	if err := sh.ValidateWithAlgo(pkt2, "flow-sec-1", 0x01); err == nil {
		t.Fatal("algo confusion should be detected")
	}

	// TOCTOU: pin a result, then verify it's cached.
	sh.PinVerification("flow-sec-2", 500)
	pkt3 := &shield.PQCPacketInfo{
		Flags:  0x0A,
		SigRef: 500,
		KeyRef: 1,
		SeqNum: 1,
	}
	if err := sh.Validate(pkt3, "flow-sec-2"); err != nil {
		t.Fatalf("pinned verification should pass: %v", err)
	}

	// Entropy check.
	bits := sh.EntropyBits()
	if bits == -1 {
		t.Log("entropy check skipped (non-Linux)")
	} else {
		t.Logf("entropy pool: %d bits", bits)
	}
}

// ---------------------------------------------------------------------------
// TestWotanStateConsistencyE2E — concurrent state updates
// ---------------------------------------------------------------------------

func TestWotanStateConsistencyE2E(t *testing.T) {
	mgr := wotan.NewPQCStateManager()

	// Concurrent increments.
	const n = 1000
	done := make(chan struct{})

	go func() {
		for i := 0; i < n; i++ {
			mgr.IncrVerifyPass()
		}
		done <- struct{}{}
	}()

	go func() {
		for i := 0; i < n; i++ {
			mgr.IncrVerifyFail()
		}
		done <- struct{}{}
	}()

	go func() {
		for i := 0; i < n; i++ {
			mgr.UpdateSigCount(uint32(i))
			mgr.UpdateKeyCount(uint32(i))
		}
		done <- struct{}{}
	}()

	<-done
	<-done
	<-done

	s := mgr.Read()
	if s.VerifyPass != n {
		t.Errorf("VerifyPass = %d, want %d", s.VerifyPass, n)
	}
	if s.VerifyFail != n {
		t.Errorf("VerifyFail = %d, want %d", s.VerifyFail, n)
	}

	// CAS should be consistent.
	current := mgr.Read()
	desired := current
	desired.SigCount = 9999
	if !mgr.CAS(current, desired) {
		t.Fatal("CAS should succeed")
	}
	if mgr.Read().SigCount != 9999 {
		t.Fatalf("SigCount after CAS = %d, want 9999", mgr.Read().SigCount)
	}

	// Marshal/unmarshal round-trip.
	s = mgr.Read()
	buf := s.Marshal()
	restored, err := wotan.UnmarshalPQCState(buf[:])
	if err != nil {
		t.Fatalf("UnmarshalPQCState: %v", err)
	}
	if restored != s {
		t.Error("state round-trip mismatch")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// realCryptoVerify implements the pluggable crypto verification function using
// the real circl ML-DSA verifiers. It dispatches based on AlgoID.
func realCryptoVerify(algoID uint8, message, signature, pubKey []byte) (bool, error) {
	switch pqc.AlgoID(algoID) {
	case pqc.AlgoMLDSA:
		// Determine parameter set from signature length.
		var ps pqc.ParameterSet
		_, sigSize44, _, _, ok44 := pqc.MLDSAParamInfo(pqc.MLDSA44)
		_, sigSize65, _, _, ok65 := pqc.MLDSAParamInfo(pqc.MLDSA65)
		_, sigSize87, _, _, ok87 := pqc.MLDSAParamInfo(pqc.MLDSA87)

		switch {
		case ok44 && len(signature) == sigSize44:
			ps = pqc.MLDSA44
		case ok65 && len(signature) == sigSize65:
			ps = pqc.MLDSA65
		case ok87 && len(signature) == sigSize87:
			ps = pqc.MLDSA87
		default:
			return false, pqc.ErrInvalidSignatureSize
		}

		verifier, err := pqc.NewMLDSAVerifier(ps)
		if err != nil {
			return false, err
		}
		return verifier.Verify(message, signature, pubKey)

	case pqc.AlgoSLHDSA:
		v := &pqc.SLHDSAVerifier{}
		return v.Verify(message, signature, pubKey)

	case pqc.AlgoFNDSA:
		v := &pqc.FNDSAVerifier{}
		return v.Verify(message, signature, pubKey)

	default:
		return false, pqc.ErrAlgorithmNotAvailable
	}
}
