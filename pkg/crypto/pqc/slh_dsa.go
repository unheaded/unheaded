// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package pqc

// SLH-DSA (FIPS 205) - Stateless Hash-Based Digital Signature Algorithm.
//
// SLH-DSA is a conservative, hash-based signature scheme that relies only on
// the security of hash functions (no lattice assumptions). It produces larger
// signatures (up to ~49KB) but offers the strongest security guarantees against
// quantum computers.
//
// Status: STUB. The cloudflare/circl library does not yet provide an SLH-DSA
// implementation. When circl adds support, this file should be updated to use:
//
//   import "github.com/cloudflare/circl/sign/slhdsa"
//
// The circl SLH-DSA API is expected to follow the same pattern as mldsa:
//   - slhdsa.GenerateKey(rand io.Reader) (*PublicKey, *PrivateKey, error)
//   - slhdsa.SignTo(sk, msg, ctx, randomized, sig)
//   - slhdsa.Verify(pk, msg, ctx, sig) bool
//
// SLH-DSA uses SHA-256 internally and produces signatures up to 49,856 bytes
// (SLH-DSA-SHA2-128f). It is BPF-safe as it requires no floating point.

// SLHDSASigner is a stub implementation of the Signer interface for SLH-DSA.
type SLHDSASigner struct{}

// Sign returns ErrAlgorithmNotAvailable. SLH-DSA is not yet available in circl.
func (s *SLHDSASigner) Sign(message []byte) ([]byte, error) {
	return nil, ErrAlgorithmNotAvailable
}

// PublicKey returns nil. SLH-DSA is not yet available in circl.
func (s *SLHDSASigner) PublicKey() []byte {
	return nil
}

// AlgoID returns AlgoSLHDSA.
func (s *SLHDSASigner) AlgoID() AlgoID {
	return AlgoSLHDSA
}

// SLHDSAVerifier is a stub implementation of the Verifier interface for SLH-DSA.
type SLHDSAVerifier struct{}

// Verify returns ErrAlgorithmNotAvailable. SLH-DSA is not yet available in circl.
func (v *SLHDSAVerifier) Verify(message, signature, publicKey []byte) (bool, error) {
	return false, ErrAlgorithmNotAvailable
}

// AlgoID returns AlgoSLHDSA.
func (v *SLHDSAVerifier) AlgoID() AlgoID {
	return AlgoSLHDSA
}

// GenerateSLHDSAKeyPair returns ErrAlgorithmNotAvailable. SLH-DSA is not yet
// available in circl.
func GenerateSLHDSAKeyPair() (*KeyPair, error) {
	return nil, ErrAlgorithmNotAvailable
}
