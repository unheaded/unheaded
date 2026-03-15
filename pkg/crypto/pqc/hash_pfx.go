// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package pqc

import (
	"crypto/sha256"
	"crypto/subtle"
)

// HashPrefix computes SHA-256(signature)[0:4] for the 4-byte HashPfx field
// in the Monad PQC value. This prefix provides a fast, fixed-size fingerprint
// of a signature for use in BPF map lookups and wire-format headers.
func HashPrefix(signature []byte) [4]byte {
	h := sha256.Sum256(signature)
	var pfx [4]byte
	copy(pfx[:], h[:4])
	return pfx
}

// VerifyHashPrefix checks if a hash prefix matches a signature using
// constant-time comparison to prevent timing side-channel attacks.
func VerifyHashPrefix(prefix [4]byte, signature []byte) bool {
	computed := HashPrefix(signature)
	return subtle.ConstantTimeCompare(prefix[:], computed[:]) == 1
}
