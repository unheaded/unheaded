// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

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
// Status: ROADMAP (Q3 2026). As of circl v1.6.3, cloudflare/circl does not
// provide an FN-DSA or Falcon implementation. The circl sign/ directory contains:
// bls, dilithium, ed25519, ed448, eddilithium2, eddilithium3, mldsa,
// slhdsa, schemes. No falcon or fndsa package exists.
//
// Alternative Go libraries considered:
//   - No mature, audited Go implementation of FN-DSA/Falcon exists as of
//     March 2026.
//   - The NIST reference implementation is in C; a CGo wrapper would be
//     possible but adds complexity, cross-compilation issues, and CGo
//     overhead that conflicts with Unheaded's pure-Go preference.
//
// When a Go implementation becomes available (circl or otherwise), this
// file should be updated to implement:
//   - FNDSASigner.Sign(message) using the private key
//   - FNDSASigner.PublicKey() returning the packed public key
//   - FNDSAVerifier.Verify(message, signature, publicKey)
//   - GenerateFNDSAKeyPair(params) producing a KeyPair and FNDSASigner
//
// Even with a full implementation, FN-DSA should NEVER be used for BPF-side
// signature verification. Use ML-DSA or SLH-DSA instead.
//
// Expected parameter sets (when implemented):
//   - FN-DSA-512:  NIST security level 1, pk ~897 bytes, sig ~666 bytes
//   - FN-DSA-1024: NIST security level 5, pk ~1793 bytes, sig ~1280 bytes

// FN-DSA parameter set constants (reserved for future implementation).
const (
	FNDSA512  ParameterSet = 0x20
	FNDSA1024 ParameterSet = 0x21
)

// fndsaParamInfo holds expected parameter-set-specific sizes for FN-DSA.
// These sizes are approximate and based on the NIST submission; final FIPS 206
// values may differ slightly.
var fndsaParamInfo = map[ParameterSet]struct {
	Name           string
	SignatureSize  int
	PublicKeySize  int
	PrivateKeySize int
}{
	FNDSA512: {Name: "FN-DSA-512", SignatureSize: 666, PublicKeySize: 897, PrivateKeySize: 1281},
	FNDSA1024: {Name: "FN-DSA-1024", SignatureSize: 1280, PublicKeySize: 1793, PrivateKeySize: 2305},
}

// FNDSASigner is a stub implementation of the Signer interface for FN-DSA.
// FN-DSA is not available: no mature Go implementation exists, and the
// algorithm requires floating-point operations incompatible with BPF.
type FNDSASigner struct {
	params ParameterSet
}

// Sign returns ErrAlgorithmNotAvailable. No Go FN-DSA implementation exists.
func (s *FNDSASigner) Sign(message []byte) ([]byte, error) {
	return nil, ErrAlgorithmNotAvailable
}

// PublicKey returns nil. No Go FN-DSA implementation exists.
func (s *FNDSASigner) PublicKey() []byte {
	return nil
}

// AlgoID returns AlgoFNDSA.
func (s *FNDSASigner) AlgoID() AlgoID {
	return AlgoFNDSA
}

// ParameterSetID returns the parameter set for this signer.
func (s *FNDSASigner) ParameterSetID() ParameterSet {
	return s.params
}

// FNDSAVerifier is a stub implementation of the Verifier interface for FN-DSA.
type FNDSAVerifier struct {
	params ParameterSet
}

// NewFNDSAVerifier creates a new FN-DSA verifier stub for the given parameter
// set. Returns ErrAlgorithmNotAvailable.
func NewFNDSAVerifier(params ParameterSet) (*FNDSAVerifier, error) {
	return nil, ErrAlgorithmNotAvailable
}

// Verify returns ErrAlgorithmNotAvailable. No Go FN-DSA implementation exists.
func (v *FNDSAVerifier) Verify(message, signature, publicKey []byte) (bool, error) {
	return false, ErrAlgorithmNotAvailable
}

// AlgoID returns AlgoFNDSA.
func (v *FNDSAVerifier) AlgoID() AlgoID {
	return AlgoFNDSA
}

// GenerateFNDSAKeyPair returns ErrAlgorithmNotAvailable. No Go FN-DSA
// implementation exists as of circl v1.6.3.
func GenerateFNDSAKeyPair(params ParameterSet) (*KeyPair, *FNDSASigner, error) {
	return nil, nil, ErrAlgorithmNotAvailable
}

// FNDSAParamInfo returns the expected parameter set info for a given FN-DSA
// parameter set. Returns false if the parameter set is unknown.
// Note: these are expected sizes based on the NIST submission; the algorithm
// is not yet implemented.
func FNDSAParamInfo(ps ParameterSet) (name string, sigSize, pkSize, skSize int, ok bool) {
	info, found := fndsaParamInfo[ps]
	if !found {
		return "", 0, 0, 0, false
	}
	return info.Name, info.SignatureSize, info.PublicKeySize, info.PrivateKeySize, true
}
