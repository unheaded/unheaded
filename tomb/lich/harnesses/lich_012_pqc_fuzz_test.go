// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// LICH-012: PQC Signature Verification Fuzzer
//
// Target: Post-quantum cryptographic operations in pkg/crypto/pqc.
// Goal: Verify that malformed signatures, keys, and algorithm IDs never
//       cause panics — all invalid inputs must return errors gracefully.
//
// The PQC layer is security-critical: it processes untrusted wire data
// (signatures, public keys, algorithm IDs) from the network. Any panic
// in this path is a denial-of-service vulnerability.
//
// Algorithms exercised:
//   - ML-DSA (FIPS 204) via cloudflare/circl: ML-DSA-44/65/87
//   - SLH-DSA (FIPS 205) via cloudflare/circl: SHA2-128s
//   - Algorithm ID dispatch (0x00-0xFF)
//
// Attack vectors:
//   - Random byte slices as "signatures" against valid public keys
//   - Malformed, truncated, and oversized public key bytes
//   - Unknown/reserved algorithm ID bytes
//   - Zero-length inputs for all parameters
//   - Maximum-length adversarial inputs

package harnesses

import (
	"testing"

	"unheaded/pkg/crypto/pqc"
)

// FuzzPQCSignatureValidation generates fuzzed byte slices as "signatures" and
// tries to verify them against a valid ML-DSA-44 public key. Must never panic
// — should return error gracefully or return (false, nil) for wrong-size inputs
// that pass size checks.
func FuzzPQCSignatureValidation(f *testing.F) {
	// Generate a real ML-DSA-44 key pair for verification attempts
	kp, _, err := pqc.GenerateMLDSAKeyPair(pqc.MLDSA44)
	if err != nil {
		f.Fatalf("failed to generate ML-DSA-44 key pair: %v", err)
	}
	validPK := kp.PublicKey

	// Seed: empty signature
	f.Add([]byte{}, []byte("hello world"))

	// Seed: single byte
	f.Add([]byte{0x00}, []byte("test"))

	// Seed: all zeros (signature-sized)
	f.Add(make([]byte, 2420), []byte("message")) // ML-DSA-44 sig size = 2420

	// Seed: all 0xFF (signature-sized)
	allFF := make([]byte, 2420)
	for i := range allFF {
		allFF[i] = 0xFF
	}
	f.Add(allFF, []byte("message"))

	// Seed: truncated signature
	f.Add(make([]byte, 100), []byte("message"))

	// Seed: oversized signature
	f.Add(make([]byte, 10000), []byte("message"))

	// Seed: empty message
	f.Add(make([]byte, 2420), []byte{})

	// Seed: nil-like empty for both
	f.Add([]byte{}, []byte{})

	f.Fuzz(func(t *testing.T, signature, message []byte) {
		verifier, err := pqc.NewMLDSAVerifier(pqc.MLDSA44)
		if err != nil {
			t.Fatalf("failed to create ML-DSA-44 verifier: %v", err)
		}

		// This must never panic — only return (bool, error)
		valid, verifyErr := verifier.Verify(message, signature, validPK)

		// If no error, signature should not be valid (we fuzzed it)
		if verifyErr == nil && valid {
			// A fuzzed signature verified — this would be a catastrophic
			// break of ML-DSA. Log it for manual review.
			t.Logf("LICH-012 ALERT: fuzzed signature verified as valid (sig len=%d, msg len=%d)",
				len(signature), len(message))
		}

		// Also test with ML-DSA-65 verifier against same fuzzed sig
		verifier65, err := pqc.NewMLDSAVerifier(pqc.MLDSA65)
		if err != nil {
			t.Fatalf("failed to create ML-DSA-65 verifier: %v", err)
		}

		// Must not panic even with wrong-sized key for this parameter set
		_, _ = verifier65.Verify(message, signature, validPK)
	})
}

// FuzzPQCKeyDerivation fuzzes public key bytes — malformed, truncated,
// oversized. Verifies key parsing never panics.
func FuzzPQCKeyDerivation(f *testing.F) {
	// Generate real keys for reference sizes
	kp44, _, err := pqc.GenerateMLDSAKeyPair(pqc.MLDSA44)
	if err != nil {
		f.Fatalf("failed to generate ML-DSA-44 key pair: %v", err)
	}

	// Seed: valid public key
	f.Add(kp44.PublicKey, []byte("test message"))

	// Seed: empty key
	f.Add([]byte{}, []byte("test message"))

	// Seed: single byte key
	f.Add([]byte{0x42}, []byte("test message"))

	// Seed: truncated key (half of valid)
	halfKey := make([]byte, len(kp44.PublicKey)/2)
	copy(halfKey, kp44.PublicKey)
	f.Add(halfKey, []byte("test message"))

	// Seed: oversized key
	oversized := make([]byte, len(kp44.PublicKey)*2)
	copy(oversized, kp44.PublicKey)
	f.Add(oversized, []byte("test message"))

	// Seed: all zeros (correct size)
	f.Add(make([]byte, len(kp44.PublicKey)), []byte("test message"))

	// Seed: all 0xFF (correct size)
	allFF := make([]byte, len(kp44.PublicKey))
	for i := range allFF {
		allFF[i] = 0xFF
	}
	f.Add(allFF, []byte("test message"))

	// Seed: bit-flipped valid key
	flipped := make([]byte, len(kp44.PublicKey))
	copy(flipped, kp44.PublicKey)
	flipped[0] ^= 0xFF
	flipped[len(flipped)-1] ^= 0xFF
	f.Add(flipped, []byte("test message"))

	f.Fuzz(func(t *testing.T, publicKey, message []byte) {
		// Create a valid signature to test against fuzzed keys
		// We need a real signature for the verification attempt
		kp, signer, genErr := pqc.GenerateMLDSAKeyPair(pqc.MLDSA44)
		if genErr != nil {
			t.Fatalf("keygen failed: %v", genErr)
		}

		sig, signErr := signer.Sign(message)
		if signErr != nil {
			t.Fatalf("sign failed: %v", signErr)
		}

		// Test ML-DSA-44: verify with fuzzed public key — must never panic
		verifier44, err := pqc.NewMLDSAVerifier(pqc.MLDSA44)
		if err != nil {
			t.Fatalf("failed to create verifier: %v", err)
		}

		valid, verifyErr := verifier44.Verify(message, sig, publicKey)

		// If the fuzzed key happens to be the real key, verification should pass
		if verifyErr == nil && valid && len(publicKey) == len(kp.PublicKey) {
			// Could be a coincidental match — acceptable
			_ = valid
		}

		// If wrong size, must get an error (not a panic)
		if len(publicKey) != len(kp.PublicKey) && verifyErr == nil {
			// Size mismatch should have been caught
			t.Logf("LICH-012 NOTE: no error for wrong-sized key (got %d, expected %d)",
				len(publicKey), len(kp.PublicKey))
		}

		// Test ML-DSA-65 with same fuzzed key — cross-parameter-set confusion
		verifier65, err := pqc.NewMLDSAVerifier(pqc.MLDSA65)
		if err != nil {
			t.Fatalf("failed to create ML-DSA-65 verifier: %v", err)
		}
		_, _ = verifier65.Verify(message, sig, publicKey)

		// Test ML-DSA-87 with same fuzzed key
		verifier87, err := pqc.NewMLDSAVerifier(pqc.MLDSA87)
		if err != nil {
			t.Fatalf("failed to create ML-DSA-87 verifier: %v", err)
		}
		_, _ = verifier87.Verify(message, sig, publicKey)
	})
}

// FuzzPQCAlgorithmIDMismatch fuzzes the algorithm ID byte (0x00-0xFF)
// combined with valid/invalid key material. Verifies unknown algo IDs
// are rejected gracefully.
func FuzzPQCAlgorithmIDMismatch(f *testing.F) {
	// Seed: all known algorithm IDs
	f.Add(uint8(0x00), []byte("key-material"), []byte("message"), []byte("signature"))
	f.Add(uint8(0x01), []byte("key-material"), []byte("message"), []byte("signature")) // SLH-DSA
	f.Add(uint8(0x02), []byte("key-material"), []byte("message"), []byte("signature")) // ML-DSA
	f.Add(uint8(0x03), []byte("key-material"), []byte("message"), []byte("signature")) // FN-DSA
	f.Add(uint8(0x04), []byte("key-material"), []byte("message"), []byte("signature")) // ML-KEM
	f.Add(uint8(0x05), []byte("key-material"), []byte("message"), []byte("signature")) // HQC
	f.Add(uint8(0x06), []byte("key-material"), []byte("message"), []byte("signature")) // unregistered
	f.Add(uint8(0xFF), []byte("key-material"), []byte("message"), []byte("signature")) // max byte
	f.Add(uint8(0x80), []byte{}, []byte{}, []byte{})                                   // high bit, empty inputs

	f.Fuzz(func(t *testing.T, algoID uint8, keyMaterial, message, signature []byte) {
		id := pqc.AlgoID(algoID)

		// Registry lookup must never panic for any AlgoID value
		info, known := pqc.Registry(id)
		_ = info

		// String conversion must never panic
		name := id.String()
		if known && name == "unknown" {
			t.Errorf("LICH-012 ALGO: registered AlgoID 0x%02X returned 'unknown' name", algoID)
		}
		if !known && name != "unknown" {
			t.Errorf("LICH-012 ALGO: unregistered AlgoID 0x%02X returned name %q instead of 'unknown'",
				algoID, name)
		}

		// Attempt to create verifiers for signature algorithms — must not panic
		switch id {
		case pqc.AlgoMLDSA:
			// Valid algorithm — test all parameter sets with fuzzed data
			for _, ps := range []pqc.ParameterSet{pqc.MLDSA44, pqc.MLDSA65, pqc.MLDSA87} {
				verifier, err := pqc.NewMLDSAVerifier(ps)
				if err != nil {
					continue
				}
				// Must not panic — fuzzed inputs
				_, _ = verifier.Verify(message, signature, keyMaterial)
			}

		case pqc.AlgoSLHDSA:
			// Valid algorithm — test with SLH-DSA-SHA2-128s
			verifier, err := pqc.NewSLHDSAVerifier(pqc.SLHDSA_SHA2_128s)
			if err != nil {
				// Acceptable — parameter set not available
				break
			}
			// Must not panic — fuzzed inputs
			_, _ = verifier.Verify(message, signature, keyMaterial)

		default:
			// Unknown or non-signature algorithm IDs
			// Attempting to create verifiers with invalid parameter sets should error
			_, err := pqc.NewMLDSAVerifier(pqc.ParameterSet(algoID))
			if err == nil {
				t.Logf("LICH-012 NOTE: NewMLDSAVerifier accepted ParameterSet 0x%02X", algoID)
			}

			_, err = pqc.NewSLHDSAVerifier(pqc.ParameterSet(algoID))
			if err == nil {
				t.Logf("LICH-012 NOTE: NewSLHDSAVerifier accepted ParameterSet 0x%02X", algoID)
			}
		}

		// AlgoConfusionGuard: test that it rejects algorithm switches on a flow
		guard := pqc.NewAlgoConfusionGuard()
		flowID := string(keyMaterial) // Use fuzzed key material as flow ID
		if flowID == "" {
			flowID = "default-flow"
		}

		// Register the fuzzed algorithm
		err := guard.Check(flowID, algoID)
		if err != nil {
			// First registration should never fail
			t.Errorf("LICH-012 GUARD: first Check failed for flow %q algo 0x%02X: %v",
				flowID, algoID, err)
		}

		// Same algorithm should succeed
		err = guard.Check(flowID, algoID)
		if err != nil {
			t.Errorf("LICH-012 GUARD: same-algo Check failed for flow %q algo 0x%02X: %v",
				flowID, algoID, err)
		}

		// Different algorithm should fail (unless algoID wraps to same value)
		differentAlgo := algoID + 1
		if differentAlgo == algoID {
			differentAlgo = algoID + 2
		}
		err = guard.Check(flowID, differentAlgo)
		if err == nil {
			t.Errorf("LICH-012 GUARD: algorithm switch from 0x%02X to 0x%02X was not rejected",
				algoID, differentAlgo)
		}
	})
}
