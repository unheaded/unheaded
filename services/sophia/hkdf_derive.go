// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package sophia

import (
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"io"
)

// HKDFDeriveKey derives a key using HKDF-SHA256 (RFC 5869).
// Used to derive tunnel keys from KEM shared secrets.
func HKDFDeriveKey(sharedSecret, salt, info []byte, keyLen int) ([]byte, error) {
	if keyLen <= 0 || keyLen > 255*sha256.Size {
		return nil, fmt.Errorf("sophia: hkdf: invalid key length %d", keyLen)
	}

	// HKDF-Extract
	prk := hkdfExtract(sharedSecret, salt)

	// HKDF-Expand
	return hkdfExpand(prk, info, keyLen)
}

// hkdfExtract performs HKDF-Extract (RFC 5869 §2.2).
func hkdfExtract(ikm, salt []byte) []byte {
	if len(salt) == 0 {
		salt = make([]byte, sha256.Size)
	}
	mac := hmac.New(sha256.New, salt)
	mac.Write(ikm)
	return mac.Sum(nil)
}

// hkdfExpand performs HKDF-Expand (RFC 5869 §2.3).
func hkdfExpand(prk, info []byte, length int) ([]byte, error) {
	hashLen := sha256.Size
	n := (length + hashLen - 1) / hashLen
	if n > 255 {
		return nil, fmt.Errorf("sophia: hkdf: output too long")
	}

	okm := make([]byte, 0, n*hashLen)
	var prev []byte

	for i := 1; i <= n; i++ {
		mac := hmac.New(sha256.New, prk)
		mac.Write(prev)
		mac.Write(info)
		mac.Write([]byte{byte(i)})
		prev = mac.Sum(nil)
		okm = append(okm, prev...)
	}

	return okm[:length], nil
}

// DeriveTunnelKey derives a symmetric key for a KEM tunnel.
// Uses HKDF-SHA256 with tunnel-specific context.
func DeriveTunnelKey(sharedSecret []byte, tunnelID string, localSvc, remoteSvc uint16) ([]byte, error) {
	salt := []byte("unheaded-pqc-tunnel-v1")
	info := fmt.Sprintf("tunnel=%s local=%d remote=%d", tunnelID, localSvc, remoteSvc)
	return HKDFDeriveKey(sharedSecret, salt, []byte(info), 32)
}

// DeriveNonce derives a nonce for a KEM tunnel from the shared secret and a counter.
func DeriveNonce(sharedSecret []byte, counter uint64) ([]byte, error) {
	salt := []byte("unheaded-pqc-nonce-v1")
	info := make([]byte, 8)
	info[0] = byte(counter >> 56)
	info[1] = byte(counter >> 48)
	info[2] = byte(counter >> 40)
	info[3] = byte(counter >> 32)
	info[4] = byte(counter >> 24)
	info[5] = byte(counter >> 16)
	info[6] = byte(counter >> 8)
	info[7] = byte(counter)
	return HKDFDeriveKey(sharedSecret, salt, info, 12)
}

// Ensure io import is used (for future streaming HKDF if needed)
var _ = io.EOF
