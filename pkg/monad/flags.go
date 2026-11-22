// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package monad holds Monad wire helpers for PQC authentication: the flags
// byte, the 12-byte PQC value, the 52-byte pseudo-header that gets signed, and
// a per-flow sliding window against replayed sequence numbers. Parked
// (docs/sweeps/REAL-STEEL-UNREACHED.tsv): no binary imports it; the
// pkg/crypto/pqc tests do.
package monad

// Monad flags byte (offset 0x07 in wire format).
const (
	FlagChaos    = 0x80 // Yaldabaoth chaos injection
	FlagCanary   = 0x40 // Canary deployment
	FlagTraced   = 0x20 // Full distributed trace
	FlagEncrypt  = 0x10 // Intra-Kingdom TLS
	FlagSampled  = 0x08 // Statistical sampling
	FlagMirror   = 0x04 // Mirror copy
	FlagCustom   = 0x02 // Exponent-encoded scratch/checksum
	FlagReserved = 0x01 // Reserved
)

// FlagPQCActive is the PQC active flag combination: S+CUSTOM (SAMPLED and CUSTOM both set).
// When both flags are set, the scratch bytes carry the PQC value
// and the checksum covers the PQC-extended pseudo-header.
const FlagPQCActive = FlagSampled | FlagCustom // 0x0A

// IsPQCActive checks if PQC authentication is active in the given flags byte.
func IsPQCActive(flags byte) bool {
	return flags&FlagPQCActive == FlagPQCActive
}

// SetPQCActive sets the PQC active flags.
func SetPQCActive(flags byte) byte {
	return flags | FlagPQCActive
}

// ClearPQCActive clears the PQC active flags.
func ClearPQCActive(flags byte) byte {
	return flags &^ byte(FlagPQCActive)
}
