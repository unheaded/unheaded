// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package pqc_policy provides application-level PQC policy checking including
// NIST security level mapping, YAML policy parsing, and 7-point Layer 2
// verification for per-application PQC requirements.
package pqc_policy

// NISTSecurityLevel maps NIST security levels (1-5) for PQC algorithms.
type NISTSecurityLevel uint8

const (
	NISTLevel0 NISTSecurityLevel = 0 // Unknown / not applicable
	NISTLevel1 NISTSecurityLevel = 1 // AES-128 equivalent
	NISTLevel2 NISTSecurityLevel = 2 // SHA-256 collision equivalent
	NISTLevel3 NISTSecurityLevel = 3 // AES-192 equivalent
	NISTLevel4 NISTSecurityLevel = 4 // SHA-384 collision equivalent
	NISTLevel5 NISTSecurityLevel = 5 // AES-256 equivalent
)

// String returns the human-readable NIST level name.
func (n NISTSecurityLevel) String() string {
	switch n {
	case NISTLevel1:
		return "NIST Level 1"
	case NISTLevel2:
		return "NIST Level 2"
	case NISTLevel3:
		return "NIST Level 3"
	case NISTLevel4:
		return "NIST Level 4"
	case NISTLevel5:
		return "NIST Level 5"
	default:
		return "Unknown"
	}
}

// Algorithm ID constants (duplicated to avoid circular imports with pkg/crypto/pqc).
const (
	algoSLHDSA uint8 = 0x01
	algoMLDSA  uint8 = 0x02
	algoFNDSA  uint8 = 0x03
	algoMLKEM  uint8 = 0x04
	algoHQC    uint8 = 0x05
)

// Parameter set constants.
const (
	paramSet1 uint8 = 0x01
	paramSet2 uint8 = 0x02
	paramSet3 uint8 = 0x03
)

// NISTLevel returns the NIST security level for a given algorithm and
// parameter set combination.
func NISTLevel(algoID, paramSet uint8) NISTSecurityLevel {
	switch algoID {
	case algoSLHDSA:
		// SLH-DSA parameter sets map to levels 1, 3, 5.
		switch paramSet {
		case paramSet1:
			return NISTLevel1
		case paramSet2:
			return NISTLevel3
		case paramSet3:
			return NISTLevel5
		}
	case algoMLDSA:
		// ML-DSA-44 → Level 2, ML-DSA-65 → Level 3, ML-DSA-87 → Level 5.
		switch paramSet {
		case paramSet1:
			return NISTLevel2
		case paramSet2:
			return NISTLevel3
		case paramSet3:
			return NISTLevel5
		}
	case algoFNDSA:
		// FN-DSA-512 → Level 1, FN-DSA-1024 → Level 5.
		switch paramSet {
		case paramSet1:
			return NISTLevel1
		case paramSet2:
			return NISTLevel5
		}
	case algoMLKEM:
		// ML-KEM-512 → Level 1, ML-KEM-768 → Level 3, ML-KEM-1024 → Level 5.
		switch paramSet {
		case paramSet1:
			return NISTLevel1
		case paramSet2:
			return NISTLevel3
		case paramSet3:
			return NISTLevel5
		}
	case algoHQC:
		// HQC-128 → Level 1, HQC-192 → Level 3.
		switch paramSet {
		case paramSet1:
			return NISTLevel1
		case paramSet2:
			return NISTLevel3
		}
	}
	return NISTLevel0
}

// MeetsMinLevel checks if a given algorithm/parameter set meets or exceeds
// the specified minimum NIST security level.
func MeetsMinLevel(algoID, paramSet uint8, minLevel NISTSecurityLevel) bool {
	return NISTLevel(algoID, paramSet) >= minLevel
}
