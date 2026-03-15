// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package pqc

// FN-DSA (FIPS 206) - FFT-based Number-Theoretic Digital Signature Algorithm.
//
// FN-DSA (formerly known as Falcon) is a lattice-based digital signature
// scheme that uses NTRU lattices and fast Fourier sampling. It produces
// compact signatures (~666 bytes for FN-DSA-512) but has a critical
// limitation: its signing operation requires IEEE 754 floating-point
// arithmetic with strict precision guarantees.
//
// BPF INCOMPATIBILITY: FN-DSA CANNOT run in BPF context because:
//   1. BPF programs cannot perform floating-point operations.
//   2. The FFT-based Gaussian sampler requires double-precision arithmetic.
//   3. Incorrect floating-point rounding can leak the private key.
//
// For these reasons, FN-DSA is restricted to userspace-only operations.
// The BPFSafe field in AlgoInfo is set to false for this algorithm.
//
// Status: STUB. The cloudflare/circl library does not yet provide an FN-DSA
// implementation. When circl adds support, this file should be updated to use:
//
//   import "github.com/cloudflare/circl/sign/fndsa"
//
// Even with a circl implementation, FN-DSA should NEVER be used for BPF-side
// signature verification. Use ML-DSA or SLH-DSA instead.

// FNDSASigner is a stub implementation of the Signer interface for FN-DSA.
// FN-DSA is not available: it requires floating-point operations incompatible
// with BPF, and circl does not yet provide an implementation.
type FNDSASigner struct{}

// Sign returns ErrAlgorithmNotAvailable.
func (s *FNDSASigner) Sign(message []byte) ([]byte, error) {
	return nil, ErrAlgorithmNotAvailable
}

// PublicKey returns nil.
func (s *FNDSASigner) PublicKey() []byte {
	return nil
}

// AlgoID returns AlgoFNDSA.
func (s *FNDSASigner) AlgoID() AlgoID {
	return AlgoFNDSA
}

// FNDSAVerifier is a stub implementation of the Verifier interface for FN-DSA.
type FNDSAVerifier struct{}

// Verify returns ErrAlgorithmNotAvailable.
func (v *FNDSAVerifier) Verify(message, signature, publicKey []byte) (bool, error) {
	return false, ErrAlgorithmNotAvailable
}

// AlgoID returns AlgoFNDSA.
func (v *FNDSAVerifier) AlgoID() AlgoID {
	return AlgoFNDSA
}

// GenerateFNDSAKeyPair returns ErrAlgorithmNotAvailable.
func GenerateFNDSAKeyPair() (*KeyPair, error) {
	return nil, ErrAlgorithmNotAvailable
}
