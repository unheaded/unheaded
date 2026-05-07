// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package sophia

import "fmt"

// KEMParamSet describes a KEM parameter set's properties.
type KEMParamSet struct {
	AlgoID        uint8  // 0x04 (ML-KEM) or 0x05 (HQC)
	ParamSet      uint8  // Parameter set identifier
	Name          string // Human-readable name
	Standard      string // FIPS standard number
	SharedKeyLen  int    // Shared secret length in bytes
	CiphertextLen int    // Ciphertext length in bytes
	PublicKeyLen  int    // Public key length in bytes
	SecretKeyLen  int    // Secret key length in bytes
	Available     bool   // Is circl implementation available?
}

// KEM parameter registry
var kemRegistry = map[string]KEMParamSet{
	// FIPS 203 — ML-KEM
	"ML-KEM-512": {
		AlgoID: 0x04, ParamSet: 0x01, Name: "ML-KEM-512", Standard: "FIPS 203",
		SharedKeyLen: 32, CiphertextLen: 768, PublicKeyLen: 800, SecretKeyLen: 1632,
		Available: true,
	},
	"ML-KEM-768": {
		AlgoID: 0x04, ParamSet: 0x02, Name: "ML-KEM-768", Standard: "FIPS 203",
		SharedKeyLen: 32, CiphertextLen: 1088, PublicKeyLen: 1184, SecretKeyLen: 2400,
		Available: true,
	},
	"ML-KEM-1024": {
		AlgoID: 0x04, ParamSet: 0x03, Name: "ML-KEM-1024", Standard: "FIPS 203",
		SharedKeyLen: 32, CiphertextLen: 1568, PublicKeyLen: 1568, SecretKeyLen: 3168,
		Available: true,
	},
	// FIPS 207 — HQC (stub)
	"HQC-128": {
		AlgoID: 0x05, ParamSet: 0x01, Name: "HQC-128", Standard: "FIPS 207",
		SharedKeyLen: 32, CiphertextLen: 4481, PublicKeyLen: 2249, SecretKeyLen: 2289,
		Available: false,
	},
	"HQC-192": {
		AlgoID: 0x05, ParamSet: 0x02, Name: "HQC-192", Standard: "FIPS 207",
		SharedKeyLen: 32, CiphertextLen: 6882, PublicKeyLen: 4522, SecretKeyLen: 4562,
		Available: false,
	},
}

// LookupKEMParam returns the KEM parameter set by name.
func LookupKEMParam(name string) (KEMParamSet, bool) {
	p, ok := kemRegistry[name]
	return p, ok
}

// LookupKEMByID returns the KEM parameter set by algoID and paramSet.
func LookupKEMByID(algoID, paramSet uint8) (KEMParamSet, bool) {
	for _, p := range kemRegistry {
		if p.AlgoID == algoID && p.ParamSet == paramSet {
			return p, true
		}
	}
	return KEMParamSet{}, false
}

// AllKEMParams returns all KEM parameter sets.
func AllKEMParams() []KEMParamSet {
	result := make([]KEMParamSet, 0, len(kemRegistry))
	for _, p := range kemRegistry {
		result = append(result, p)
	}
	return result
}

// AvailableKEMParams returns only KEM parameter sets with circl implementations.
func AvailableKEMParams() []KEMParamSet {
	var result []KEMParamSet
	for _, p := range kemRegistry {
		if p.Available {
			result = append(result, p)
		}
	}
	return result
}

// ValidateKEMParam checks if a KEM parameter combination is valid.
func ValidateKEMParam(algoID, paramSet uint8) error {
	_, ok := LookupKEMByID(algoID, paramSet)
	if !ok {
		return fmt.Errorf("sophia: unknown KEM parameter: algo=0x%02x param=0x%02x", algoID, paramSet)
	}
	return nil
}
