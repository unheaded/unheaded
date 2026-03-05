// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package pqc

import (
	"github.com/cloudflare/circl/kem/mlkem/mlkem512"
	"github.com/cloudflare/circl/kem/mlkem/mlkem768"
	"github.com/cloudflare/circl/kem/mlkem/mlkem1024"
	"github.com/cloudflare/circl/sign/mldsa/mldsa44"
	"github.com/cloudflare/circl/sign/mldsa/mldsa65"
	"github.com/cloudflare/circl/sign/mldsa/mldsa87"
)

// AlgoID is a 1-byte algorithm identifier per
// draft-bellis-unheaded-pqc-authentication-00.
type AlgoID uint8

const (
	// AlgoSLHDSA identifies SLH-DSA (FIPS 205) - Hash-based signature.
	AlgoSLHDSA AlgoID = 0x01

	// AlgoMLDSA identifies ML-DSA (FIPS 204) - Lattice-based signature
	// (ML-DSA-44/65/87).
	AlgoMLDSA AlgoID = 0x02

	// AlgoFNDSA identifies FN-DSA (FIPS 206) - Lattice-based signature.
	// Userspace-only; requires floating point operations and cannot run in
	// BPF context.
	AlgoFNDSA AlgoID = 0x03

	// AlgoMLKEM identifies ML-KEM (FIPS 203) - Lattice-based key
	// encapsulation mechanism (ML-KEM-512/768/1024).
	AlgoMLKEM AlgoID = 0x04

	// AlgoHQC identifies HQC (FIPS 207) - Code-based key encapsulation
	// mechanism.
	AlgoHQC AlgoID = 0x05
)

// String returns the human-readable name for an AlgoID.
func (a AlgoID) String() string {
	info, ok := Registry(a)
	if !ok {
		return "unknown"
	}
	return info.Name
}

// ParameterSet identifies a specific parameter set within an algorithm family.
type ParameterSet uint8

const (
	// ML-DSA parameter sets (FIPS 204).
	MLDSA44 ParameterSet = 0x01
	MLDSA65 ParameterSet = 0x02
	MLDSA87 ParameterSet = 0x03

	// ML-KEM parameter sets (FIPS 203).
	MLKEM512  ParameterSet = 0x01
	MLKEM768  ParameterSet = 0x02
	MLKEM1024 ParameterSet = 0x03

	// HQC parameter sets (FIPS 207).
	HQC128 ParameterSet = 0x01
	HQC192 ParameterSet = 0x02
)

// AlgoInfo describes an algorithm's properties.
type AlgoInfo struct {
	// ID is the 1-byte algorithm identifier.
	ID AlgoID

	// Name is the human-readable algorithm name.
	Name string

	// Standard is the FIPS standard number (e.g., "FIPS 205").
	Standard string

	// Type is either "signature" or "kem".
	Type string

	// Family describes the cryptographic family: "hash-based", "lattice",
	// or "code-based".
	Family string

	// MaxSigSize is the maximum signature size in bytes (0 for KEMs).
	MaxSigSize int

	// MaxKeySize is the maximum public key size in bytes.
	MaxKeySize int

	// BPFSafe indicates whether this algorithm is safe for BPF-side
	// reference (e.g., no floating point requirements).
	BPFSafe bool

	// Available indicates whether a circl implementation is available for
	// this algorithm.
	Available bool
}

// registry holds the static algorithm info table.
var registry = map[AlgoID]AlgoInfo{
	AlgoSLHDSA: {
		ID:         AlgoSLHDSA,
		Name:       "SLH-DSA",
		Standard:   "FIPS 205",
		Type:       "signature",
		Family:     "hash-based",
		MaxSigSize: 49856, // SLH-DSA-SHA2-128f max sig size
		MaxKeySize: 64,    // SLH-DSA-SHA2-128f public key size
		BPFSafe:    true,
		Available:  false, // circl does not yet have SLH-DSA
	},
	AlgoMLDSA: {
		ID:         AlgoMLDSA,
		Name:       "ML-DSA",
		Standard:   "FIPS 204",
		Type:       "signature",
		Family:     "lattice",
		MaxSigSize: mldsa87.SignatureSize, // ML-DSA-87 has largest signatures
		MaxKeySize: mldsa87.PublicKeySize, // ML-DSA-87 has largest keys
		BPFSafe:    true,
		Available:  true,
	},
	AlgoFNDSA: {
		ID:         AlgoFNDSA,
		Name:       "FN-DSA",
		Standard:   "FIPS 206",
		Type:       "signature",
		Family:     "lattice",
		MaxSigSize: 1280, // FN-DSA-512 approximate max sig size
		MaxKeySize: 897,  // FN-DSA-512 approximate public key size
		BPFSafe:    false, // Requires floating point; cannot run in BPF
		Available:  false, // circl does not yet have FN-DSA
	},
	AlgoMLKEM: {
		ID:         AlgoMLKEM,
		Name:       "ML-KEM",
		Standard:   "FIPS 203",
		Type:       "kem",
		Family:     "lattice",
		MaxSigSize: 0, // Not a signature scheme
		MaxKeySize: mlkem1024.PublicKeySize,
		BPFSafe:    true,
		Available:  true,
	},
	AlgoHQC: {
		ID:         AlgoHQC,
		Name:       "HQC",
		Standard:   "FIPS 207",
		Type:       "kem",
		Family:     "code-based",
		MaxSigSize: 0,    // Not a signature scheme
		MaxKeySize: 2249, // HQC-128 approximate public key size
		BPFSafe:    true,
		Available:  false, // circl does not yet have HQC
	},
}

// mldsaParamInfo holds parameter-set-specific sizes for ML-DSA.
var mldsaParamInfo = map[ParameterSet]struct {
	Name          string
	SignatureSize int
	PublicKeySize int
	PrivateKeySize int
}{
	MLDSA44: {
		Name:           "ML-DSA-44",
		SignatureSize:  mldsa44.SignatureSize,
		PublicKeySize:  mldsa44.PublicKeySize,
		PrivateKeySize: mldsa44.PrivateKeySize,
	},
	MLDSA65: {
		Name:           "ML-DSA-65",
		SignatureSize:  mldsa65.SignatureSize,
		PublicKeySize:  mldsa65.PublicKeySize,
		PrivateKeySize: mldsa65.PrivateKeySize,
	},
	MLDSA87: {
		Name:           "ML-DSA-87",
		SignatureSize:  mldsa87.SignatureSize,
		PublicKeySize:  mldsa87.PublicKeySize,
		PrivateKeySize: mldsa87.PrivateKeySize,
	},
}

// mlkemParamInfo holds parameter-set-specific sizes for ML-KEM.
var mlkemParamInfo = map[ParameterSet]struct {
	Name           string
	PublicKeySize  int
	PrivateKeySize int
	CiphertextSize int
	SharedKeySize  int
}{
	MLKEM512: {
		Name:           "ML-KEM-512",
		PublicKeySize:  mlkem512.PublicKeySize,
		PrivateKeySize: mlkem512.PrivateKeySize,
		CiphertextSize: mlkem512.CiphertextSize,
		SharedKeySize:  mlkem512.SharedKeySize,
	},
	MLKEM768: {
		Name:           "ML-KEM-768",
		PublicKeySize:  mlkem768.PublicKeySize,
		PrivateKeySize: mlkem768.PrivateKeySize,
		CiphertextSize: mlkem768.CiphertextSize,
		SharedKeySize:  mlkem768.SharedKeySize,
	},
	MLKEM1024: {
		Name:           "ML-KEM-1024",
		PublicKeySize:  mlkem1024.PublicKeySize,
		PrivateKeySize: mlkem1024.PrivateKeySize,
		CiphertextSize: mlkem1024.CiphertextSize,
		SharedKeySize:  mlkem1024.SharedKeySize,
	},
}

// Registry returns AlgoInfo for a given AlgoID. The second return value
// indicates whether the ID was found.
func Registry(id AlgoID) (AlgoInfo, bool) {
	info, ok := registry[id]
	return info, ok
}

// AllAlgorithms returns all registered algorithm infos in ID order.
func AllAlgorithms() []AlgoInfo {
	algos := make([]AlgoInfo, 0, len(registry))
	// Return in AlgoID order (0x01 through 0x05).
	for id := AlgoID(0x01); id <= AlgoID(0x05); id++ {
		if info, ok := registry[id]; ok {
			algos = append(algos, info)
		}
	}
	return algos
}

// MLDSAParamInfo returns the parameter set info for a given ML-DSA parameter
// set. Returns false if the parameter set is unknown.
func MLDSAParamInfo(ps ParameterSet) (name string, sigSize, pkSize, skSize int, ok bool) {
	info, found := mldsaParamInfo[ps]
	if !found {
		return "", 0, 0, 0, false
	}
	return info.Name, info.SignatureSize, info.PublicKeySize, info.PrivateKeySize, true
}

// MLKEMParamInfo returns the parameter set info for a given ML-KEM parameter
// set. Returns false if the parameter set is unknown.
func MLKEMParamInfo(ps ParameterSet) (name string, pkSize, skSize, ctSize, ssSize int, ok bool) {
	info, found := mlkemParamInfo[ps]
	if !found {
		return "", 0, 0, 0, 0, false
	}
	return info.Name, info.PublicKeySize, info.PrivateKeySize, info.CiphertextSize, info.SharedKeySize, true
}
