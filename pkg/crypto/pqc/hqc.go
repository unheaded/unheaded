// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package pqc

// HQC (FIPS 207) - Hamming Quasi-Cyclic Key Encapsulation Mechanism.
//
// HQC is a code-based key encapsulation mechanism selected by NIST for
// standardization in FIPS 207. It provides an alternative to lattice-based
// KEMs (ML-KEM) by relying on the hardness of decoding random quasi-cyclic
// codes, offering cryptographic diversity.
//
// HQC supports two parameter sets:
//   - HQC-128: NIST security level 1
//   - HQC-192: NIST security level 3
//
// Status: STUB. The cloudflare/circl library does not yet provide an HQC
// implementation. When circl adds support, this file should be updated to use:
//
//   import "github.com/cloudflare/circl/kem/hqc"
//
// The implementation should follow the same pattern as ML-KEM:
//   - hqc.GenerateKeyPair(rand) (*PublicKey, *PrivateKey, error)
//   - pk.EncapsulateTo(ct, ss, seed)
//   - sk.DecapsulateTo(ss, ct)

// HQCEncapsulator is a stub implementation of the KEMEncapsulator interface
// for HQC.
type HQCEncapsulator struct{}

// Encapsulate returns ErrAlgorithmNotAvailable. HQC is not yet available in
// circl.
func (e *HQCEncapsulator) Encapsulate(publicKey []byte) (ciphertext, sharedSecret []byte, err error) {
	return nil, nil, ErrAlgorithmNotAvailable
}

// AlgoID returns AlgoHQC.
func (e *HQCEncapsulator) AlgoID() AlgoID {
	return AlgoHQC
}

// HQCDecapsulator is a stub implementation of the KEMDecapsulator interface
// for HQC.
type HQCDecapsulator struct{}

// Decapsulate returns ErrAlgorithmNotAvailable. HQC is not yet available in
// circl.
func (d *HQCDecapsulator) Decapsulate(ciphertext, secretKey []byte) (sharedSecret []byte, err error) {
	return nil, ErrAlgorithmNotAvailable
}

// AlgoID returns AlgoHQC.
func (d *HQCDecapsulator) AlgoID() AlgoID {
	return AlgoHQC
}

// GenerateHQCKeyPair returns ErrAlgorithmNotAvailable. HQC is not yet
// available in circl.
func GenerateHQCKeyPair(params ParameterSet) (*KeyPair, error) {
	return nil, ErrAlgorithmNotAvailable
}
