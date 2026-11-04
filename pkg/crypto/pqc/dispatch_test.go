// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package pqc

import (
	"errors"
	"testing"
)

// signedFixture is one valid (message, signature, public key) under a set.
type signedFixture struct {
	name   string
	algo   AlgoID
	params ParameterSet
	msg    []byte
	sig    []byte
	pk     []byte
}

func dispatchFixtures(t *testing.T) []signedFixture {
	t.Helper()
	msg := []byte("one verify dispatch")

	mlKP, mlSigner, err := GenerateMLDSAKeyPair(MLDSA65)
	if err != nil {
		t.Fatalf("GenerateMLDSAKeyPair: %v", err)
	}
	mlSig, err := mlSigner.Sign(msg)
	if err != nil {
		t.Fatalf("ML-DSA-65 Sign: %v", err)
	}

	slhKP, slhSigner, err := GenerateSLHDSAKeyPair(SLHDSA_SHA2_128f)
	if err != nil {
		t.Fatalf("GenerateSLHDSAKeyPair: %v", err)
	}
	slhSig, err := slhSigner.Sign(msg)
	if err != nil {
		t.Fatalf("SLH-DSA-SHA2-128f Sign: %v", err)
	}

	return []signedFixture{
		{"ML-DSA-65", AlgoMLDSA, MLDSA65, msg, mlSig, mlKP.PublicKey},
		{"SLH-DSA-SHA2-128f", AlgoSLHDSA, SLHDSA_SHA2_128f, msg, slhSig, slhKP.PublicKey},
	}
}

func TestVerifySignatureRoundTrip(t *testing.T) {
	for _, f := range dispatchFixtures(t) {
		t.Run(f.name, func(t *testing.T) {
			ok, err := VerifySignature(f.algo, f.params, f.msg, f.sig, f.pk)
			if err != nil {
				t.Fatalf("VerifySignature: %v", err)
			}
			if !ok {
				t.Fatal("VerifySignature = false for a valid signature")
			}
		})
	}
}

func TestVerifySignatureFlippedBitFails(t *testing.T) {
	for _, f := range dispatchFixtures(t) {
		t.Run(f.name, func(t *testing.T) {
			for _, at := range []int{0, len(f.sig) / 2, len(f.sig) - 1} {
				bad := append([]byte(nil), f.sig...)
				bad[at] ^= 0x01
				ok, err := VerifySignature(f.algo, f.params, f.msg, bad, f.pk)
				if err != nil {
					t.Fatalf("signature bit at byte %d: %v", at, err)
				}
				if ok {
					t.Errorf("signature bit at byte %d: verified", at)
				}
			}

			badMsg := append([]byte(nil), f.msg...)
			badMsg[0] ^= 0x01
			ok, err := VerifySignature(f.algo, f.params, badMsg, f.sig, f.pk)
			if err != nil {
				t.Fatalf("message bit: %v", err)
			}
			if ok {
				t.Error("message bit: verified")
			}
		})
	}
}

// A real signature must not get through an algorithm the dispatch refuses.
func TestVerifySignatureRefusesOtherAlgorithms(t *testing.T) {
	f := dispatchFixtures(t)[0]
	for _, algo := range []AlgoID{AlgoWithdrawn03, AlgoHQC, AlgoMLKEM, 0x00, 0xFF} {
		ok, err := VerifySignature(algo, f.params, f.msg, f.sig, f.pk)
		if !errors.Is(err, ErrAlgorithmNotAvailable) {
			t.Errorf("algo %#02x: err = %v, want ErrAlgorithmNotAvailable", uint8(algo), err)
		}
		if ok {
			t.Errorf("algo %#02x: ok = true on a refusal", uint8(algo))
		}
	}
}

// Parameter ids are per family: a value that is valid in one family must not
// select anything in the other.
func TestVerifySignatureRefusesCrossFamilyParams(t *testing.T) {
	fixtures := dispatchFixtures(t)
	ml, slh := fixtures[0], fixtures[1]
	for _, tt := range []struct {
		name   string
		algo   AlgoID
		params ParameterSet
		f      signedFixture
	}{
		{"ML-DSA with an SLH-DSA set", AlgoMLDSA, SLHDSA_SHA2_128f, ml},
		{"ML-DSA with a SHAKE set", AlgoMLDSA, SLHDSA_SHAKE_128f, ml},
		{"SLH-DSA with an ML-DSA set", AlgoSLHDSA, MLDSA65, slh},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ok, err := VerifySignature(tt.algo, tt.params, tt.f.msg, tt.f.sig, tt.f.pk)
			if !errors.Is(err, ErrInvalidParameterSet) {
				t.Errorf("err = %v, want ErrInvalidParameterSet", err)
			}
			if ok {
				t.Error("ok = true on a refusal")
			}
		})
	}
}
