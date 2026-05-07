// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package pqc

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"testing"
)

// --- Registry Tests ---

func TestRegistryLookupAllAlgorithms(t *testing.T) {
	tests := []struct {
		id        AlgoID
		name      string
		standard  string
		algoType  string
		family    string
		bpfSafe   bool
		available bool
	}{
		{AlgoSLHDSA, "SLH-DSA", "FIPS 205", "signature", "hash-based", true, true},
		{AlgoMLDSA, "ML-DSA", "FIPS 204", "signature", "lattice", true, true},
		{AlgoFNDSA, "FN-DSA", "FIPS 206", "signature", "lattice", false, false},
		{AlgoMLKEM, "ML-KEM", "FIPS 203", "kem", "lattice", true, true},
		{AlgoHQC, "HQC", "FIPS 207", "kem", "code-based", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, ok := Registry(tt.id)
			if !ok {
				t.Fatalf("Registry(%#x) not found", tt.id)
			}
			if info.Name != tt.name {
				t.Errorf("Name = %q, want %q", info.Name, tt.name)
			}
			if info.Standard != tt.standard {
				t.Errorf("Standard = %q, want %q", info.Standard, tt.standard)
			}
			if info.Type != tt.algoType {
				t.Errorf("Type = %q, want %q", info.Type, tt.algoType)
			}
			if info.Family != tt.family {
				t.Errorf("Family = %q, want %q", info.Family, tt.family)
			}
			if info.BPFSafe != tt.bpfSafe {
				t.Errorf("BPFSafe = %v, want %v", info.BPFSafe, tt.bpfSafe)
			}
			if info.Available != tt.available {
				t.Errorf("Available = %v, want %v", info.Available, tt.available)
			}
		})
	}
}

func TestRegistryLookupUnknown(t *testing.T) {
	_, ok := Registry(0xFF)
	if ok {
		t.Error("Registry(0xFF) should return false for unknown ID")
	}
}

func TestAllAlgorithms(t *testing.T) {
	algos := AllAlgorithms()
	if len(algos) != 5 {
		t.Fatalf("AllAlgorithms() returned %d entries, want 5", len(algos))
	}
	// Verify ordering by AlgoID.
	for i, algo := range algos {
		expectedID := AlgoID(i + 1)
		if algo.ID != expectedID {
			t.Errorf("AllAlgorithms()[%d].ID = %#x, want %#x", i, algo.ID, expectedID)
		}
	}
}

func TestAlgoIDString(t *testing.T) {
	tests := []struct {
		id   AlgoID
		want string
	}{
		{AlgoSLHDSA, "SLH-DSA"},
		{AlgoMLDSA, "ML-DSA"},
		{AlgoFNDSA, "FN-DSA"},
		{AlgoMLKEM, "ML-KEM"},
		{AlgoHQC, "HQC"},
		{0xFF, "unknown"},
	}
	for _, tt := range tests {
		if got := tt.id.String(); got != tt.want {
			t.Errorf("AlgoID(%#x).String() = %q, want %q", tt.id, got, tt.want)
		}
	}
}

func TestMLDSAParamInfo(t *testing.T) {
	tests := []struct {
		ps     ParameterSet
		name   string
		wantOK bool
	}{
		{MLDSA44, "ML-DSA-44", true},
		{MLDSA65, "ML-DSA-65", true},
		{MLDSA87, "ML-DSA-87", true},
		{ParameterSet(0xFF), "", false},
	}
	for _, tt := range tests {
		name, sigSize, pkSize, skSize, ok := MLDSAParamInfo(tt.ps)
		if ok != tt.wantOK {
			t.Errorf("MLDSAParamInfo(%#x) ok = %v, want %v", tt.ps, ok, tt.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if name != tt.name {
			t.Errorf("MLDSAParamInfo(%#x) name = %q, want %q", tt.ps, name, tt.name)
		}
		if sigSize <= 0 {
			t.Errorf("MLDSAParamInfo(%#x) sigSize = %d, want > 0", tt.ps, sigSize)
		}
		if pkSize <= 0 {
			t.Errorf("MLDSAParamInfo(%#x) pkSize = %d, want > 0", tt.ps, pkSize)
		}
		if skSize <= 0 {
			t.Errorf("MLDSAParamInfo(%#x) skSize = %d, want > 0", tt.ps, skSize)
		}
	}
}

func TestMLKEMParamInfo(t *testing.T) {
	tests := []struct {
		ps     ParameterSet
		name   string
		wantOK bool
	}{
		{MLKEM512, "ML-KEM-512", true},
		{MLKEM768, "ML-KEM-768", true},
		{MLKEM1024, "ML-KEM-1024", true},
		{ParameterSet(0xFF), "", false},
	}
	for _, tt := range tests {
		name, pkSize, skSize, ctSize, ssSize, ok := MLKEMParamInfo(tt.ps)
		if ok != tt.wantOK {
			t.Errorf("MLKEMParamInfo(%#x) ok = %v, want %v", tt.ps, ok, tt.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if name != tt.name {
			t.Errorf("MLKEMParamInfo(%#x) name = %q, want %q", tt.ps, name, tt.name)
		}
		if pkSize <= 0 {
			t.Errorf("MLKEMParamInfo(%#x) pkSize = %d, want > 0", tt.ps, pkSize)
		}
		if skSize <= 0 {
			t.Errorf("MLKEMParamInfo(%#x) skSize = %d, want > 0", tt.ps, skSize)
		}
		if ctSize <= 0 {
			t.Errorf("MLKEMParamInfo(%#x) ctSize = %d, want > 0", tt.ps, ctSize)
		}
		if ssSize <= 0 {
			t.Errorf("MLKEMParamInfo(%#x) ssSize = %d, want > 0", tt.ps, ssSize)
		}
	}
}

// --- HashPrefix Tests ---

func TestHashPrefix(t *testing.T) {
	sig := []byte("test signature data for hashing")
	pfx := HashPrefix(sig)

	// Verify it matches the first 4 bytes of SHA-256.
	h := sha256.Sum256(sig)
	if pfx[0] != h[0] || pfx[1] != h[1] || pfx[2] != h[2] || pfx[3] != h[3] {
		t.Errorf("HashPrefix mismatch: got %x, want %x", pfx[:], h[:4])
	}
}

func TestHashPrefixDeterministic(t *testing.T) {
	sig := []byte("deterministic test input")
	pfx1 := HashPrefix(sig)
	pfx2 := HashPrefix(sig)
	if pfx1 != pfx2 {
		t.Errorf("HashPrefix not deterministic: %x != %x", pfx1, pfx2)
	}
}

func TestHashPrefixDifferentInputs(t *testing.T) {
	pfx1 := HashPrefix([]byte("input one"))
	pfx2 := HashPrefix([]byte("input two"))
	if pfx1 == pfx2 {
		t.Error("HashPrefix produced same prefix for different inputs (unlikely collision)")
	}
}

func TestVerifyHashPrefix(t *testing.T) {
	sig := []byte("verify hash prefix test")
	pfx := HashPrefix(sig)

	if !VerifyHashPrefix(pfx, sig) {
		t.Error("VerifyHashPrefix returned false for matching prefix")
	}

	// Flip a bit in the prefix.
	badPfx := pfx
	badPfx[0] ^= 0x01
	if VerifyHashPrefix(badPfx, sig) {
		t.Error("VerifyHashPrefix returned true for non-matching prefix")
	}
}

func TestVerifyHashPrefixConstantTime(t *testing.T) {
	// This test verifies the constant-time property indirectly by ensuring
	// the function uses crypto/subtle. A timing analysis would be needed for
	// a full verification, but we can at least confirm correctness.
	sig := []byte("constant time test")
	pfx := HashPrefix(sig)

	// Verify correct prefix passes.
	if !VerifyHashPrefix(pfx, sig) {
		t.Error("VerifyHashPrefix failed for correct prefix")
	}

	// Verify each single-bit flip in the prefix is detected.
	for byteIdx := 0; byteIdx < 4; byteIdx++ {
		for bit := uint(0); bit < 8; bit++ {
			badPfx := pfx
			badPfx[byteIdx] ^= 1 << bit
			if VerifyHashPrefix(badPfx, sig) {
				t.Errorf("VerifyHashPrefix missed bit flip at byte %d, bit %d", byteIdx, bit)
			}
		}
	}
}

func TestHashPrefixEmptyInput(t *testing.T) {
	pfx := HashPrefix([]byte{})
	// SHA-256 of empty string is well-defined.
	h := sha256.Sum256([]byte{})
	var expected [4]byte
	copy(expected[:], h[:4])
	if pfx != expected {
		t.Errorf("HashPrefix(empty) = %x, want %x", pfx, expected)
	}
}

// --- ML-DSA Tests ---

func TestMLDSAKeyGeneration(t *testing.T) {
	params := []struct {
		name string
		ps   ParameterSet
	}{
		{"ML-DSA-44", MLDSA44},
		{"ML-DSA-65", MLDSA65},
		{"ML-DSA-87", MLDSA87},
	}

	for _, tt := range params {
		t.Run(tt.name, func(t *testing.T) {
			kp, signer, err := GenerateMLDSAKeyPair(tt.ps)
			if err != nil {
				t.Fatalf("GenerateMLDSAKeyPair(%s): %v", tt.name, err)
			}
			if kp.AlgoID != AlgoMLDSA {
				t.Errorf("AlgoID = %#x, want %#x", kp.AlgoID, AlgoMLDSA)
			}
			if len(kp.PublicKey) == 0 {
				t.Error("PublicKey is empty")
			}
			if len(kp.SecretKey) == 0 {
				t.Error("SecretKey is empty")
			}
			if signer == nil {
				t.Fatal("signer is nil")
			}
			if signer.AlgoID() != AlgoMLDSA {
				t.Errorf("signer.AlgoID() = %#x, want %#x", signer.AlgoID(), AlgoMLDSA)
			}
			if signer.ParameterSetID() != tt.ps {
				t.Errorf("signer.ParameterSetID() = %#x, want %#x", signer.ParameterSetID(), tt.ps)
			}
			// Verify the signer's public key matches the key pair.
			signerPK := signer.PublicKey()
			if !bytes.Equal(signerPK, kp.PublicKey) {
				t.Error("signer.PublicKey() does not match kp.PublicKey")
			}
		})
	}
}

func TestMLDSAKeyGenInvalidParams(t *testing.T) {
	_, _, err := GenerateMLDSAKeyPair(ParameterSet(0xFF))
	if !errors.Is(err, ErrInvalidParameterSet) {
		t.Errorf("GenerateMLDSAKeyPair(0xFF) err = %v, want ErrInvalidParameterSet", err)
	}
}

func TestMLDSASignVerifyRoundTrip(t *testing.T) {
	params := []struct {
		name string
		ps   ParameterSet
	}{
		{"ML-DSA-44", MLDSA44},
		{"ML-DSA-65", MLDSA65},
		{"ML-DSA-87", MLDSA87},
	}

	for _, tt := range params {
		t.Run(tt.name, func(t *testing.T) {
			kp, signer, err := GenerateMLDSAKeyPair(tt.ps)
			if err != nil {
				t.Fatalf("GenerateMLDSAKeyPair: %v", err)
			}

			message := []byte("hello, post-quantum world")

			sig, err := signer.Sign(message)
			if err != nil {
				t.Fatalf("Sign: %v", err)
			}
			if len(sig) == 0 {
				t.Fatal("signature is empty")
			}

			verifier, err := NewMLDSAVerifier(tt.ps)
			if err != nil {
				t.Fatalf("NewMLDSAVerifier: %v", err)
			}
			if verifier.AlgoID() != AlgoMLDSA {
				t.Errorf("verifier.AlgoID() = %#x, want %#x", verifier.AlgoID(), AlgoMLDSA)
			}

			valid, err := verifier.Verify(message, sig, kp.PublicKey)
			if err != nil {
				t.Fatalf("Verify: %v", err)
			}
			if !valid {
				t.Error("Verify returned false for valid signature")
			}

			// Verify with wrong message fails.
			wrongMsg := []byte("wrong message")
			valid, err = verifier.Verify(wrongMsg, sig, kp.PublicKey)
			if err != nil {
				t.Fatalf("Verify(wrong msg): %v", err)
			}
			if valid {
				t.Error("Verify returned true for wrong message")
			}

			// Verify with corrupted signature fails.
			badSig := make([]byte, len(sig))
			copy(badSig, sig)
			badSig[0] ^= 0xFF
			valid, err = verifier.Verify(message, badSig, kp.PublicKey)
			if err != nil {
				t.Fatalf("Verify(bad sig): %v", err)
			}
			if valid {
				t.Error("Verify returned true for corrupted signature")
			}
		})
	}
}

func TestMLDSAVerifierInvalidParams(t *testing.T) {
	_, err := NewMLDSAVerifier(ParameterSet(0xFF))
	if !errors.Is(err, ErrInvalidParameterSet) {
		t.Errorf("NewMLDSAVerifier(0xFF) err = %v, want ErrInvalidParameterSet", err)
	}
}

func TestMLDSAVerifyInvalidKeySize(t *testing.T) {
	verifier, err := NewMLDSAVerifier(MLDSA44)
	if err != nil {
		t.Fatalf("NewMLDSAVerifier: %v", err)
	}
	_, err = verifier.Verify([]byte("msg"), []byte("sig"), []byte("short"))
	if !errors.Is(err, ErrInvalidKeySize) {
		t.Errorf("Verify(short key) err = %v, want ErrInvalidKeySize", err)
	}
}

// --- ML-KEM Tests ---

func TestMLKEMKeyGeneration(t *testing.T) {
	params := []struct {
		name string
		ps   ParameterSet
	}{
		{"ML-KEM-512", MLKEM512},
		{"ML-KEM-768", MLKEM768},
		{"ML-KEM-1024", MLKEM1024},
	}

	for _, tt := range params {
		t.Run(tt.name, func(t *testing.T) {
			kp, err := GenerateMLKEMKeyPair(tt.ps)
			if err != nil {
				t.Fatalf("GenerateMLKEMKeyPair(%s): %v", tt.name, err)
			}
			if kp.AlgoID != AlgoMLKEM {
				t.Errorf("AlgoID = %#x, want %#x", kp.AlgoID, AlgoMLKEM)
			}
			if len(kp.PublicKey) == 0 {
				t.Error("PublicKey is empty")
			}
			if len(kp.SecretKey) == 0 {
				t.Error("SecretKey is empty")
			}
		})
	}
}

func TestMLKEMKeyGenInvalidParams(t *testing.T) {
	_, err := GenerateMLKEMKeyPair(ParameterSet(0xFF))
	if !errors.Is(err, ErrInvalidParameterSet) {
		t.Errorf("GenerateMLKEMKeyPair(0xFF) err = %v, want ErrInvalidParameterSet", err)
	}
}

func TestMLKEMEncapDecapRoundTrip(t *testing.T) {
	params := []struct {
		name string
		ps   ParameterSet
	}{
		{"ML-KEM-512", MLKEM512},
		{"ML-KEM-768", MLKEM768},
		{"ML-KEM-1024", MLKEM1024},
	}

	for _, tt := range params {
		t.Run(tt.name, func(t *testing.T) {
			kp, err := GenerateMLKEMKeyPair(tt.ps)
			if err != nil {
				t.Fatalf("GenerateMLKEMKeyPair: %v", err)
			}

			encap, err := NewMLKEMEncapsulator(tt.ps)
			if err != nil {
				t.Fatalf("NewMLKEMEncapsulator: %v", err)
			}
			if encap.AlgoID() != AlgoMLKEM {
				t.Errorf("encap.AlgoID() = %#x, want %#x", encap.AlgoID(), AlgoMLKEM)
			}

			ct, ss1, err := encap.Encapsulate(kp.PublicKey)
			if err != nil {
				t.Fatalf("Encapsulate: %v", err)
			}
			if len(ct) == 0 {
				t.Fatal("ciphertext is empty")
			}
			if len(ss1) == 0 {
				t.Fatal("shared secret is empty")
			}

			decap, err := NewMLKEMDecapsulator(tt.ps)
			if err != nil {
				t.Fatalf("NewMLKEMDecapsulator: %v", err)
			}
			if decap.AlgoID() != AlgoMLKEM {
				t.Errorf("decap.AlgoID() = %#x, want %#x", decap.AlgoID(), AlgoMLKEM)
			}

			ss2, err := decap.Decapsulate(ct, kp.SecretKey)
			if err != nil {
				t.Fatalf("Decapsulate: %v", err)
			}

			if !bytes.Equal(ss1, ss2) {
				t.Error("shared secrets do not match after encap/decap round trip")
			}
		})
	}
}

func TestMLKEMEncapsulatorInvalidParams(t *testing.T) {
	_, err := NewMLKEMEncapsulator(ParameterSet(0xFF))
	if !errors.Is(err, ErrInvalidParameterSet) {
		t.Errorf("NewMLKEMEncapsulator(0xFF) err = %v, want ErrInvalidParameterSet", err)
	}
}

func TestMLKEMDecapsulatorInvalidParams(t *testing.T) {
	_, err := NewMLKEMDecapsulator(ParameterSet(0xFF))
	if !errors.Is(err, ErrInvalidParameterSet) {
		t.Errorf("NewMLKEMDecapsulator(0xFF) err = %v, want ErrInvalidParameterSet", err)
	}
}

func TestMLKEMEncapsulateInvalidKeySize(t *testing.T) {
	encap, err := NewMLKEMEncapsulator(MLKEM768)
	if err != nil {
		t.Fatalf("NewMLKEMEncapsulator: %v", err)
	}
	_, _, err = encap.Encapsulate([]byte("short"))
	if !errors.Is(err, ErrInvalidKeySize) {
		t.Errorf("Encapsulate(short key) err = %v, want ErrInvalidKeySize", err)
	}
}

func TestMLKEMDecapsulateInvalidKeySize(t *testing.T) {
	decap, err := NewMLKEMDecapsulator(MLKEM768)
	if err != nil {
		t.Fatalf("NewMLKEMDecapsulator: %v", err)
	}
	_, err = decap.Decapsulate([]byte("short ct"), []byte("short key"))
	if !errors.Is(err, ErrInvalidKeySize) {
		t.Errorf("Decapsulate(short key) err = %v, want ErrInvalidKeySize", err)
	}
}

// --- Stub Algorithm Tests ---

// --- SLH-DSA Tests ---

func TestSLHDSAKeyGeneration(t *testing.T) {
	// Use 128s (smallest signatures) for fast tests.
	params := []struct {
		name string
		ps   ParameterSet
	}{
		{"SLH-DSA-SHA2-128s", SLHDSA_SHA2_128s},
		{"SLH-DSA-SHA2-128f", SLHDSA_SHA2_128f},
	}

	for _, tt := range params {
		t.Run(tt.name, func(t *testing.T) {
			kp, signer, err := GenerateSLHDSAKeyPair(tt.ps)
			if err != nil {
				t.Fatalf("GenerateSLHDSAKeyPair(%s): %v", tt.name, err)
			}
			if kp.AlgoID != AlgoSLHDSA {
				t.Errorf("AlgoID = %#x, want %#x", kp.AlgoID, AlgoSLHDSA)
			}
			if len(kp.PublicKey) == 0 {
				t.Error("PublicKey is empty")
			}
			if len(kp.SecretKey) == 0 {
				t.Error("SecretKey is empty")
			}
			if signer == nil {
				t.Fatal("signer is nil")
			}
			if signer.AlgoID() != AlgoSLHDSA {
				t.Errorf("signer.AlgoID() = %#x, want %#x", signer.AlgoID(), AlgoSLHDSA)
			}
			if signer.ParameterSetID() != tt.ps {
				t.Errorf("signer.ParameterSetID() = %#x, want %#x", signer.ParameterSetID(), tt.ps)
			}
			signerPK := signer.PublicKey()
			if !bytes.Equal(signerPK, kp.PublicKey) {
				t.Error("signer.PublicKey() does not match kp.PublicKey")
			}
		})
	}
}

func TestSLHDSAKeyGenInvalidParams(t *testing.T) {
	_, _, err := GenerateSLHDSAKeyPair(ParameterSet(0xFF))
	if !errors.Is(err, ErrInvalidParameterSet) {
		t.Errorf("GenerateSLHDSAKeyPair(0xFF) err = %v, want ErrInvalidParameterSet", err)
	}
}

func TestSLHDSASignVerifyRoundTrip(t *testing.T) {
	// Use SHA2-128s for fast test execution (smallest signatures: 7856 bytes).
	kp, signer, err := GenerateSLHDSAKeyPair(SLHDSA_SHA2_128s)
	if err != nil {
		t.Fatalf("GenerateSLHDSAKeyPair: %v", err)
	}

	message := []byte("hello, post-quantum hash-based world")

	sig, err := signer.Sign(message)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if len(sig) == 0 {
		t.Fatal("signature is empty")
	}

	verifier, err := NewSLHDSAVerifier(SLHDSA_SHA2_128s)
	if err != nil {
		t.Fatalf("NewSLHDSAVerifier: %v", err)
	}
	if verifier.AlgoID() != AlgoSLHDSA {
		t.Errorf("verifier.AlgoID() = %#x, want %#x", verifier.AlgoID(), AlgoSLHDSA)
	}

	valid, err := verifier.Verify(message, sig, kp.PublicKey)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !valid {
		t.Error("Verify returned false for valid signature")
	}

	// Verify with wrong message fails.
	valid, err = verifier.Verify([]byte("wrong message"), sig, kp.PublicKey)
	if err != nil {
		t.Fatalf("Verify(wrong msg): %v", err)
	}
	if valid {
		t.Error("Verify returned true for wrong message")
	}

	// Verify with corrupted signature fails.
	badSig := make([]byte, len(sig))
	copy(badSig, sig)
	badSig[0] ^= 0xFF
	valid, err = verifier.Verify(message, badSig, kp.PublicKey)
	if err != nil {
		t.Fatalf("Verify(bad sig): %v", err)
	}
	if valid {
		t.Error("Verify returned true for corrupted signature")
	}
}

func TestSLHDSAVerifierInvalidParams(t *testing.T) {
	_, err := NewSLHDSAVerifier(ParameterSet(0xFF))
	if !errors.Is(err, ErrInvalidParameterSet) {
		t.Errorf("NewSLHDSAVerifier(0xFF) err = %v, want ErrInvalidParameterSet", err)
	}
}

func TestSLHDSAVerifyInvalidKeySize(t *testing.T) {
	verifier, err := NewSLHDSAVerifier(SLHDSA_SHA2_128s)
	if err != nil {
		t.Fatalf("NewSLHDSAVerifier: %v", err)
	}
	_, err = verifier.Verify([]byte("msg"), []byte("sig"), []byte("short"))
	if !errors.Is(err, ErrInvalidKeySize) {
		t.Errorf("Verify(short key) err = %v, want ErrInvalidKeySize", err)
	}
}

func TestSLHDSAParamInfo(t *testing.T) {
	tests := []struct {
		ps     ParameterSet
		name   string
		wantOK bool
	}{
		{SLHDSA_SHA2_128s, "SLH-DSA-SHA2-128s", true},
		{SLHDSA_SHA2_128f, "SLH-DSA-SHA2-128f", true},
		{SLHDSA_SHA2_192s, "SLH-DSA-SHA2-192s", true},
		{SLHDSA_SHA2_192f, "SLH-DSA-SHA2-192f", true},
		{SLHDSA_SHA2_256s, "SLH-DSA-SHA2-256s", true},
		{SLHDSA_SHA2_256f, "SLH-DSA-SHA2-256f", true},
		{ParameterSet(0xFF), "", false},
	}
	for _, tt := range tests {
		name, sigSize, pkSize, skSize, ok := SLHDSAParamInfo(tt.ps)
		if ok != tt.wantOK {
			t.Errorf("SLHDSAParamInfo(%#x) ok = %v, want %v", tt.ps, ok, tt.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if name != tt.name {
			t.Errorf("SLHDSAParamInfo(%#x) name = %q, want %q", tt.ps, name, tt.name)
		}
		if sigSize <= 0 {
			t.Errorf("SLHDSAParamInfo(%#x) sigSize = %d, want > 0", tt.ps, sigSize)
		}
		if pkSize <= 0 {
			t.Errorf("SLHDSAParamInfo(%#x) pkSize = %d, want > 0", tt.ps, pkSize)
		}
		if skSize <= 0 {
			t.Errorf("SLHDSAParamInfo(%#x) skSize = %d, want > 0", tt.ps, skSize)
		}
	}
}

func TestFNDSAStubReturnsNotAvailable(t *testing.T) {
	signer := &FNDSASigner{}
	_, err := signer.Sign([]byte("test"))
	if !errors.Is(err, ErrAlgorithmNotAvailable) {
		t.Errorf("FNDSASigner.Sign err = %v, want ErrAlgorithmNotAvailable", err)
	}
	if pk := signer.PublicKey(); pk != nil {
		t.Errorf("FNDSASigner.PublicKey() = %v, want nil", pk)
	}
	if id := signer.AlgoID(); id != AlgoFNDSA {
		t.Errorf("FNDSASigner.AlgoID() = %#x, want %#x", id, AlgoFNDSA)
	}

	_, err = NewFNDSAVerifier(FNDSA512)
	if !errors.Is(err, ErrAlgorithmNotAvailable) {
		t.Errorf("NewFNDSAVerifier err = %v, want ErrAlgorithmNotAvailable", err)
	}

	// Direct verifier still returns not available.
	verifier := &FNDSAVerifier{}
	_, err = verifier.Verify([]byte("msg"), []byte("sig"), []byte("pk"))
	if !errors.Is(err, ErrAlgorithmNotAvailable) {
		t.Errorf("FNDSAVerifier.Verify err = %v, want ErrAlgorithmNotAvailable", err)
	}
	if id := verifier.AlgoID(); id != AlgoFNDSA {
		t.Errorf("FNDSAVerifier.AlgoID() = %#x, want %#x", id, AlgoFNDSA)
	}

	_, _, err = GenerateFNDSAKeyPair(FNDSA512)
	if !errors.Is(err, ErrAlgorithmNotAvailable) {
		t.Errorf("GenerateFNDSAKeyPair err = %v, want ErrAlgorithmNotAvailable", err)
	}
}

func TestFNDSAParamInfo(t *testing.T) {
	name, sigSize, pkSize, skSize, ok := FNDSAParamInfo(FNDSA512)
	if !ok {
		t.Fatal("FNDSAParamInfo(FNDSA512) not found")
	}
	if name != "FN-DSA-512" {
		t.Errorf("name = %q, want FN-DSA-512", name)
	}
	if sigSize <= 0 || pkSize <= 0 || skSize <= 0 {
		t.Errorf("sizes should be > 0: sig=%d pk=%d sk=%d", sigSize, pkSize, skSize)
	}
}

func TestHQCStubReturnsNotAvailable(t *testing.T) {
	encap := &HQCEncapsulator{}
	_, _, err := encap.Encapsulate([]byte("pk"))
	if !errors.Is(err, ErrAlgorithmNotAvailable) {
		t.Errorf("HQCEncapsulator.Encapsulate err = %v, want ErrAlgorithmNotAvailable", err)
	}
	if id := encap.AlgoID(); id != AlgoHQC {
		t.Errorf("HQCEncapsulator.AlgoID() = %#x, want %#x", id, AlgoHQC)
	}

	decap := &HQCDecapsulator{}
	_, err = decap.Decapsulate([]byte("ct"), []byte("sk"))
	if !errors.Is(err, ErrAlgorithmNotAvailable) {
		t.Errorf("HQCDecapsulator.Decapsulate err = %v, want ErrAlgorithmNotAvailable", err)
	}
	if id := decap.AlgoID(); id != AlgoHQC {
		t.Errorf("HQCDecapsulator.AlgoID() = %#x, want %#x", id, AlgoHQC)
	}

	_, err = GenerateHQCKeyPair(HQC128)
	if !errors.Is(err, ErrAlgorithmNotAvailable) {
		t.Errorf("GenerateHQCKeyPair err = %v, want ErrAlgorithmNotAvailable", err)
	}
}

// --- Interface Compliance Tests ---

func TestSignerInterface(t *testing.T) {
	// Verify all signer types implement the Signer interface.
	var _ Signer = &MLDSASigner{}
	var _ Signer = &SLHDSASigner{}
	var _ Signer = &FNDSASigner{}
}

func TestVerifierInterface(t *testing.T) {
	// Verify all verifier types implement the Verifier interface.
	var _ Verifier = &MLDSAVerifier{}
	var _ Verifier = &SLHDSAVerifier{}
	var _ Verifier = &FNDSAVerifier{}
}

func TestKEMEncapsulatorInterface(t *testing.T) {
	// Verify all encapsulator types implement the KEMEncapsulator interface.
	var _ KEMEncapsulator = &MLKEMEncapsulator{}
	var _ KEMEncapsulator = &HQCEncapsulator{}
}

func TestKEMDecapsulatorInterface(t *testing.T) {
	// Verify all decapsulator types implement the KEMDecapsulator interface.
	var _ KEMDecapsulator = &MLKEMDecapsulator{}
	var _ KEMDecapsulator = &HQCDecapsulator{}
}

// --- FN-DSA BPF Safety Tests ---

func TestFNDSANotBPFSafe(t *testing.T) {
	info, ok := Registry(AlgoFNDSA)
	if !ok {
		t.Fatal("FN-DSA not found in registry")
	}
	if info.BPFSafe {
		t.Error("FN-DSA should NOT be BPF-safe (requires floating point)")
	}
}

// --- KeyPair Structure Tests ---

func TestKeyPairFields(t *testing.T) {
	kp, _, err := GenerateMLDSAKeyPair(MLDSA44)
	if err != nil {
		t.Fatalf("GenerateMLDSAKeyPair: %v", err)
	}

	// Verify KeyRef defaults to zero.
	if kp.KeyRef != 0 {
		t.Errorf("KeyRef = %d, want 0 (default)", kp.KeyRef)
	}

	// Verify we can set KeyRef.
	kp.KeyRef = 0xABCD
	if kp.KeyRef != 0xABCD {
		t.Errorf("KeyRef = %#x, want 0xABCD", kp.KeyRef)
	}
}

// --- Cross-key Verification Tests ---

func TestMLDSACrossKeyVerificationFails(t *testing.T) {
	// Generate two different key pairs and ensure one cannot verify the
	// other's signatures.
	_, signer1, err := GenerateMLDSAKeyPair(MLDSA44)
	if err != nil {
		t.Fatalf("GenerateMLDSAKeyPair (1): %v", err)
	}

	kp2, _, err := GenerateMLDSAKeyPair(MLDSA44)
	if err != nil {
		t.Fatalf("GenerateMLDSAKeyPair (2): %v", err)
	}

	message := []byte("cross-key test message")
	sig, err := signer1.Sign(message)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	verifier, _ := NewMLDSAVerifier(MLDSA44)
	valid, err := verifier.Verify(message, sig, kp2.PublicKey)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if valid {
		t.Error("Verify returned true with wrong public key")
	}
}

func TestMLKEMCrossKeyDecapsulationDiffers(t *testing.T) {
	// Generate two key pairs. Encapsulate with pk1, decapsulate with sk2.
	// The shared secrets should differ (ML-KEM uses implicit rejection).
	kp1, err := GenerateMLKEMKeyPair(MLKEM768)
	if err != nil {
		t.Fatalf("GenerateMLKEMKeyPair (1): %v", err)
	}

	kp2, err := GenerateMLKEMKeyPair(MLKEM768)
	if err != nil {
		t.Fatalf("GenerateMLKEMKeyPair (2): %v", err)
	}

	encap, _ := NewMLKEMEncapsulator(MLKEM768)
	ct, ss1, err := encap.Encapsulate(kp1.PublicKey)
	if err != nil {
		t.Fatalf("Encapsulate: %v", err)
	}

	decap, _ := NewMLKEMDecapsulator(MLKEM768)
	ss2, err := decap.Decapsulate(ct, kp2.SecretKey)
	if err != nil {
		t.Fatalf("Decapsulate: %v", err)
	}

	if bytes.Equal(ss1, ss2) {
		t.Error("shared secrets should differ when using wrong secret key")
	}
}

// --- HashPrefix Integration with Signatures ---

func TestHashPrefixWithMLDSASignature(t *testing.T) {
	_, signer, err := GenerateMLDSAKeyPair(MLDSA44)
	if err != nil {
		t.Fatalf("GenerateMLDSAKeyPair: %v", err)
	}

	message := []byte("hash prefix integration test")
	sig, err := signer.Sign(message)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	pfx := HashPrefix(sig)
	if !VerifyHashPrefix(pfx, sig) {
		t.Error("VerifyHashPrefix failed for ML-DSA signature")
	}

	// Verify a different signature produces a different prefix.
	message2 := []byte("different message")
	sig2, err := signer.Sign(message2)
	if err != nil {
		t.Fatalf("Sign(2): %v", err)
	}
	pfx2 := HashPrefix(sig2)
	if pfx == pfx2 {
		t.Error("HashPrefix produced same prefix for different signatures (unlikely collision)")
	}
}
