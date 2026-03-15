// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package pqc

import (
	"crypto/rand"
	"testing"
)

// --- ML-DSA Sign Benchmarks ---

func BenchmarkMLDSA44Sign(b *testing.B) {
	_, signer, err := GenerateMLDSAKeyPair(MLDSA44)
	if err != nil {
		b.Fatalf("GenerateMLDSAKeyPair: %v", err)
	}
	message := []byte("benchmark message for ML-DSA-44 signing")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := signer.Sign(message)
		if err != nil {
			b.Fatalf("Sign: %v", err)
		}
	}
}

func BenchmarkMLDSA65Sign(b *testing.B) {
	_, signer, err := GenerateMLDSAKeyPair(MLDSA65)
	if err != nil {
		b.Fatalf("GenerateMLDSAKeyPair: %v", err)
	}
	message := []byte("benchmark message for ML-DSA-65 signing")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := signer.Sign(message)
		if err != nil {
			b.Fatalf("Sign: %v", err)
		}
	}
}

func BenchmarkMLDSA87Sign(b *testing.B) {
	_, signer, err := GenerateMLDSAKeyPair(MLDSA87)
	if err != nil {
		b.Fatalf("GenerateMLDSAKeyPair: %v", err)
	}
	message := []byte("benchmark message for ML-DSA-87 signing")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := signer.Sign(message)
		if err != nil {
			b.Fatalf("Sign: %v", err)
		}
	}
}

// --- ML-DSA Verify Benchmarks ---

func BenchmarkMLDSA44Verify(b *testing.B) {
	kp, signer, err := GenerateMLDSAKeyPair(MLDSA44)
	if err != nil {
		b.Fatalf("GenerateMLDSAKeyPair: %v", err)
	}
	message := []byte("benchmark message for ML-DSA-44 verification")
	sig, err := signer.Sign(message)
	if err != nil {
		b.Fatalf("Sign: %v", err)
	}
	verifier, _ := NewMLDSAVerifier(MLDSA44)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := verifier.Verify(message, sig, kp.PublicKey)
		if err != nil {
			b.Fatalf("Verify: %v", err)
		}
	}
}

func BenchmarkMLDSA65Verify(b *testing.B) {
	kp, signer, err := GenerateMLDSAKeyPair(MLDSA65)
	if err != nil {
		b.Fatalf("GenerateMLDSAKeyPair: %v", err)
	}
	message := []byte("benchmark message for ML-DSA-65 verification")
	sig, err := signer.Sign(message)
	if err != nil {
		b.Fatalf("Sign: %v", err)
	}
	verifier, _ := NewMLDSAVerifier(MLDSA65)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := verifier.Verify(message, sig, kp.PublicKey)
		if err != nil {
			b.Fatalf("Verify: %v", err)
		}
	}
}

func BenchmarkMLDSA87Verify(b *testing.B) {
	kp, signer, err := GenerateMLDSAKeyPair(MLDSA87)
	if err != nil {
		b.Fatalf("GenerateMLDSAKeyPair: %v", err)
	}
	message := []byte("benchmark message for ML-DSA-87 verification")
	sig, err := signer.Sign(message)
	if err != nil {
		b.Fatalf("Sign: %v", err)
	}
	verifier, _ := NewMLDSAVerifier(MLDSA87)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := verifier.Verify(message, sig, kp.PublicKey)
		if err != nil {
			b.Fatalf("Verify: %v", err)
		}
	}
}

// --- ML-KEM Encapsulate Benchmarks ---

func BenchmarkMLKEM512Encapsulate(b *testing.B) {
	kp, err := GenerateMLKEMKeyPair(MLKEM512)
	if err != nil {
		b.Fatalf("GenerateMLKEMKeyPair: %v", err)
	}
	encap, _ := NewMLKEMEncapsulator(MLKEM512)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := encap.Encapsulate(kp.PublicKey)
		if err != nil {
			b.Fatalf("Encapsulate: %v", err)
		}
	}
}

func BenchmarkMLKEM768Encapsulate(b *testing.B) {
	kp, err := GenerateMLKEMKeyPair(MLKEM768)
	if err != nil {
		b.Fatalf("GenerateMLKEMKeyPair: %v", err)
	}
	encap, _ := NewMLKEMEncapsulator(MLKEM768)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := encap.Encapsulate(kp.PublicKey)
		if err != nil {
			b.Fatalf("Encapsulate: %v", err)
		}
	}
}

func BenchmarkMLKEM1024Encapsulate(b *testing.B) {
	kp, err := GenerateMLKEMKeyPair(MLKEM1024)
	if err != nil {
		b.Fatalf("GenerateMLKEMKeyPair: %v", err)
	}
	encap, _ := NewMLKEMEncapsulator(MLKEM1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := encap.Encapsulate(kp.PublicKey)
		if err != nil {
			b.Fatalf("Encapsulate: %v", err)
		}
	}
}

// --- ML-KEM Decapsulate Benchmarks ---

func BenchmarkMLKEM512Decapsulate(b *testing.B) {
	kp, err := GenerateMLKEMKeyPair(MLKEM512)
	if err != nil {
		b.Fatalf("GenerateMLKEMKeyPair: %v", err)
	}
	encap, _ := NewMLKEMEncapsulator(MLKEM512)
	ct, _, err := encap.Encapsulate(kp.PublicKey)
	if err != nil {
		b.Fatalf("Encapsulate: %v", err)
	}
	decap, _ := NewMLKEMDecapsulator(MLKEM512)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := decap.Decapsulate(ct, kp.SecretKey)
		if err != nil {
			b.Fatalf("Decapsulate: %v", err)
		}
	}
}

func BenchmarkMLKEM768Decapsulate(b *testing.B) {
	kp, err := GenerateMLKEMKeyPair(MLKEM768)
	if err != nil {
		b.Fatalf("GenerateMLKEMKeyPair: %v", err)
	}
	encap, _ := NewMLKEMEncapsulator(MLKEM768)
	ct, _, err := encap.Encapsulate(kp.PublicKey)
	if err != nil {
		b.Fatalf("Encapsulate: %v", err)
	}
	decap, _ := NewMLKEMDecapsulator(MLKEM768)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := decap.Decapsulate(ct, kp.SecretKey)
		if err != nil {
			b.Fatalf("Decapsulate: %v", err)
		}
	}
}

func BenchmarkMLKEM1024Decapsulate(b *testing.B) {
	kp, err := GenerateMLKEMKeyPair(MLKEM1024)
	if err != nil {
		b.Fatalf("GenerateMLKEMKeyPair: %v", err)
	}
	encap, _ := NewMLKEMEncapsulator(MLKEM1024)
	ct, _, err := encap.Encapsulate(kp.PublicKey)
	if err != nil {
		b.Fatalf("Encapsulate: %v", err)
	}
	decap, _ := NewMLKEMDecapsulator(MLKEM1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := decap.Decapsulate(ct, kp.SecretKey)
		if err != nil {
			b.Fatalf("Decapsulate: %v", err)
		}
	}
}

// --- HashPrefix Benchmark ---

func BenchmarkHashPrefix(b *testing.B) {
	// Use a realistic signature-sized input (ML-DSA-44 signature).
	sig := make([]byte, 2420) // Approximate ML-DSA-44 signature size
	_, _ = rand.Read(sig)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		HashPrefix(sig)
	}
}

func BenchmarkVerifyHashPrefix(b *testing.B) {
	sig := make([]byte, 2420)
	_, _ = rand.Read(sig)
	pfx := HashPrefix(sig)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		VerifyHashPrefix(pfx, sig)
	}
}

// --- Key Generation Benchmarks ---

func BenchmarkMLDSA44KeyGen(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _, err := GenerateMLDSAKeyPair(MLDSA44)
		if err != nil {
			b.Fatalf("GenerateMLDSAKeyPair: %v", err)
		}
	}
}

func BenchmarkMLDSA65KeyGen(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _, err := GenerateMLDSAKeyPair(MLDSA65)
		if err != nil {
			b.Fatalf("GenerateMLDSAKeyPair: %v", err)
		}
	}
}

func BenchmarkMLDSA87KeyGen(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _, err := GenerateMLDSAKeyPair(MLDSA87)
		if err != nil {
			b.Fatalf("GenerateMLDSAKeyPair: %v", err)
		}
	}
}

func BenchmarkMLKEM768KeyGen(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := GenerateMLKEMKeyPair(MLKEM768)
		if err != nil {
			b.Fatalf("GenerateMLKEMKeyPair: %v", err)
		}
	}
}
