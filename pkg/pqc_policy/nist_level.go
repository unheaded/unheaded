// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

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
	algoMLKEM  uint8 = 0x04
	algoHQC    uint8 = 0x05
)

// Parameter set constants (ML-DSA, ML-KEM, HQC).
const (
	paramSet1 uint8 = 0x01
	paramSet2 uint8 = 0x02
	paramSet3 uint8 = 0x03
)

// SLH-DSA parameter set ids, copied from pkg/crypto/pqc (slh_dsa.go).
// nist_level_slhdsa_test.go fails if the two tables drift apart.
const (
	slhdsaSHAKE128s uint8 = 0x07
	slhdsaSHAKE128f uint8 = 0x08
	slhdsaSHAKE192s uint8 = 0x09
	slhdsaSHAKE192f uint8 = 0x0A
	slhdsaSHAKE256s uint8 = 0x0B
	slhdsaSHAKE256f uint8 = 0x0C
	slhdsaSHA2128s  uint8 = 0x10
	slhdsaSHA2128f  uint8 = 0x11
	slhdsaSHA2192s  uint8 = 0x12
	slhdsaSHA2192f  uint8 = 0x13
	slhdsaSHA2256s  uint8 = 0x14
	slhdsaSHA2256f  uint8 = 0x15
)

// NISTLevel returns the NIST security level for a given algorithm and
// parameter set combination.
func NISTLevel(algoID, paramSet uint8) NISTSecurityLevel {
	switch algoID {
	case algoSLHDSA:
		// SLH-DSA ids are pkg/crypto/pqc's own (the ids it signs with):
		// SHAKE sets 0x07-0x0C, SHA2 sets 0x10-0x15, each family in the
		// order 128s, 128f, 192s, 192f, 256s, 256f. FIPS 205 puts n=16 at
		// category 1, n=24 at 3, n=32 at 5. Any other id is unknown.
		switch paramSet {
		case slhdsaSHAKE128s, slhdsaSHAKE128f, slhdsaSHA2128s, slhdsaSHA2128f:
			return NISTLevel1
		case slhdsaSHAKE192s, slhdsaSHAKE192f, slhdsaSHA2192s, slhdsaSHA2192f:
			return NISTLevel3
		case slhdsaSHAKE256s, slhdsaSHAKE256f, slhdsaSHA2256s, slhdsaSHA2256f:
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
