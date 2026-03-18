// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package pqc

// HQC (FIPS 207) - Hamming Quasi-Cyclic Key Encapsulation Mechanism.
//
// HQC is a code-based key encapsulation mechanism selected by NIST for
// standardization in FIPS 207 (announced 2024). It provides an alternative
// to lattice-based KEMs (ML-KEM) by relying on the hardness of decoding
// random quasi-cyclic codes, offering cryptographic diversity against
// potential lattice-specific attacks.
//
// HQC supports two parameter sets:
//   - HQC-128: NIST security level 1 (pk ~2249 bytes, ct ~4481 bytes)
//   - HQC-192: NIST security level 3 (pk ~4522 bytes, ct ~6882 bytes)
//
// Status: ROADMAP (Q3 2026). As of circl v1.6.3, cloudflare/circl does not
// provide an HQC implementation. The circl kem/ directory contains: frodo, hybrid,
// kyber, mlkem, schemes, sike, xwing. No hqc package exists.
//
// Alternative Go libraries considered:
//   - No mature, audited Go implementation of HQC exists as of March 2026.
//   - HQC was only announced as a FIPS 207 selection in 2024, making it
//     the newest of the NIST PQC standards.
//   - The NIST reference implementation is in C; a CGo wrapper is possible
//     but not yet available as a maintained library.
//
// When a Go implementation becomes available (likely circl), this file
// should be updated following the ML-KEM pattern:
//   - hqc.GenerateKeyPair(rand) (*PublicKey, *PrivateKey, error)
//   - pk.EncapsulateTo(ct, ss, seed)
//   - sk.DecapsulateTo(ss, ct)
//
// HQC is BPF-safe as it requires no floating-point arithmetic.

// hqcParamInfo holds expected parameter-set-specific sizes for HQC.
// These sizes are based on the NIST submission specifications; final FIPS 207
// values may differ slightly.
var hqcParamInfo = map[ParameterSet]struct {
	Name           string
	PublicKeySize  int
	PrivateKeySize int
	CiphertextSize int
	SharedKeySize  int
}{
	HQC128: {
		Name:           "HQC-128",
		PublicKeySize:  2249,
		PrivateKeySize: 2289,
		CiphertextSize: 4481,
		SharedKeySize:  32,
	},
	HQC192: {
		Name:           "HQC-192",
		PublicKeySize:  4522,
		PrivateKeySize: 4562,
		CiphertextSize: 6882,
		SharedKeySize:  32,
	},
}

// HQCEncapsulator is a stub implementation of the KEMEncapsulator interface
// for HQC. Returns ErrAlgorithmNotAvailable for all operations until a Go
// implementation of HQC becomes available.
type HQCEncapsulator struct {
	params ParameterSet
}

// NewHQCEncapsulator creates a new HQC encapsulator stub. Returns
// ErrAlgorithmNotAvailable.
func NewHQCEncapsulator(params ParameterSet) (*HQCEncapsulator, error) {
	return nil, ErrAlgorithmNotAvailable
}

// Encapsulate returns ErrAlgorithmNotAvailable. HQC is not yet available in
// any Go library.
func (e *HQCEncapsulator) Encapsulate(publicKey []byte) (ciphertext, sharedSecret []byte, err error) {
	return nil, nil, ErrAlgorithmNotAvailable
}

// AlgoID returns AlgoHQC.
func (e *HQCEncapsulator) AlgoID() AlgoID {
	return AlgoHQC
}

// HQCDecapsulator is a stub implementation of the KEMDecapsulator interface
// for HQC. Returns ErrAlgorithmNotAvailable for all operations until a Go
// implementation of HQC becomes available.
type HQCDecapsulator struct {
	params ParameterSet
}

// NewHQCDecapsulator creates a new HQC decapsulator stub. Returns
// ErrAlgorithmNotAvailable.
func NewHQCDecapsulator(params ParameterSet) (*HQCDecapsulator, error) {
	return nil, ErrAlgorithmNotAvailable
}

// Decapsulate returns ErrAlgorithmNotAvailable. HQC is not yet available in
// any Go library.
func (d *HQCDecapsulator) Decapsulate(ciphertext, secretKey []byte) (sharedSecret []byte, err error) {
	return nil, ErrAlgorithmNotAvailable
}

// AlgoID returns AlgoHQC.
func (d *HQCDecapsulator) AlgoID() AlgoID {
	return AlgoHQC
}

// GenerateHQCKeyPair returns ErrAlgorithmNotAvailable. HQC is not yet
// available in any Go library as of circl v1.6.3.
func GenerateHQCKeyPair(params ParameterSet) (*KeyPair, error) {
	return nil, ErrAlgorithmNotAvailable
}

// HQCParamInfo returns the expected parameter set info for a given HQC
// parameter set. Returns false if the parameter set is unknown.
// Note: these are expected sizes based on the NIST submission; the algorithm
// is not yet implemented.
func HQCParamInfo(ps ParameterSet) (name string, pkSize, skSize, ctSize, ssSize int, ok bool) {
	info, found := hqcParamInfo[ps]
	if !found {
		return "", 0, 0, 0, 0, false
	}
	return info.Name, info.PublicKeySize, info.PrivateKeySize, info.CiphertextSize, info.SharedKeySize, true
}
