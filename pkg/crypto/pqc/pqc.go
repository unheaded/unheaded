// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package pqc provides post-quantum cryptographic primitives for the Unheaded
// platform, wrapping FIPS 203-207 algorithms via cloudflare/circl.
//
// Supported algorithms:
//   - ML-DSA (FIPS 204) - Lattice-based digital signature (ML-DSA-44/65/87)
//   - ML-KEM (FIPS 203) - Lattice-based key encapsulation (ML-KEM-512/768/1024)
//   - SLH-DSA (FIPS 205) - Hash-based digital signature (stub, pending circl support)
//   - FN-DSA (FIPS 206) - Lattice signature (stub, requires floating point, no BPF)
//   - HQC (FIPS 207) - Code-based KEM (stub, pending circl support)
//
// All algorithm wrappers implement unified interfaces (Signer, Verifier,
// KEMEncapsulator, KEMDecapsulator) for use by the Sophia BPF maps and Monad
// wire format.
package pqc

import "errors"

// ErrAlgorithmNotAvailable is returned when an algorithm implementation is not
// yet available (e.g., SLH-DSA, FN-DSA, HQC stubs).
var ErrAlgorithmNotAvailable = errors.New("pqc: algorithm not available")

// ErrInvalidParameterSet is returned when an unsupported parameter set is
// requested for an algorithm family.
var ErrInvalidParameterSet = errors.New("pqc: invalid parameter set")

// ErrInvalidKeySize is returned when a key does not match the expected size
// for the algorithm and parameter set.
var ErrInvalidKeySize = errors.New("pqc: invalid key size")

// ErrInvalidSignatureSize is returned when a signature does not match the
// expected size for the algorithm and parameter set.
var ErrInvalidSignatureSize = errors.New("pqc: invalid signature size")

// ErrInvalidCiphertextSize is returned when a ciphertext does not match the
// expected size for the algorithm and parameter set.
var ErrInvalidCiphertextSize = errors.New("pqc: invalid ciphertext size")

// Signer signs messages with PQC algorithms.
type Signer interface {
	// Sign produces a signature over message using the private key held by
	// the signer. Returns the raw signature bytes.
	Sign(message []byte) ([]byte, error)

	// PublicKey returns the packed public key bytes.
	PublicKey() []byte

	// AlgoID returns the algorithm identifier for this signer.
	AlgoID() AlgoID
}

// Verifier verifies PQC signatures.
type Verifier interface {
	// Verify checks whether signature is a valid signature of message under
	// publicKey. Returns true if valid, false otherwise.
	Verify(message, signature, publicKey []byte) (bool, error)

	// AlgoID returns the algorithm identifier for this verifier.
	AlgoID() AlgoID
}

// KEMEncapsulator performs key encapsulation.
type KEMEncapsulator interface {
	// Encapsulate generates a shared secret and ciphertext for the given
	// public key. The ciphertext must be sent to the holder of the
	// corresponding private key who can recover the shared secret via
	// Decapsulate.
	Encapsulate(publicKey []byte) (ciphertext, sharedSecret []byte, err error)

	// AlgoID returns the algorithm identifier for this encapsulator.
	AlgoID() AlgoID
}

// KEMDecapsulator performs key decapsulation.
type KEMDecapsulator interface {
	// Decapsulate recovers the shared secret from ciphertext using secretKey.
	Decapsulate(ciphertext, secretKey []byte) (sharedSecret []byte, err error)

	// AlgoID returns the algorithm identifier for this decapsulator.
	AlgoID() AlgoID
}

// KeyPair holds a PQC key pair.
type KeyPair struct {
	// AlgoID identifies the algorithm used to generate this key pair.
	AlgoID AlgoID

	// PublicKey is the packed public key bytes.
	PublicKey []byte

	// SecretKey is the packed private/secret key bytes.
	SecretKey []byte

	// KeyRef is the 2-byte map reference used in Sophia BPF maps.
	KeyRef uint16
}
