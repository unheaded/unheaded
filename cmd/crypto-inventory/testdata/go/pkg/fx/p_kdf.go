// SPDX-License-Identifier: GPL-3.0-or-later

package fx

import (
	"crypto/hkdf"
	"crypto/sha256"
	"golang.org/x/crypto/argon2"
	xhkdf "golang.org/x/crypto/hkdf"
)

// (p) key-derivation imports: the stdlib HKDF, x/crypto's, and Argon2.
// crypto/sha256 is here as HKDF's hash and gives no row of its own.
var (
	P2 = xhkdf.New
	P3 = argon2.IDKey
)

// P1 extracts with the standard library's HKDF.
func P1(secret, salt []byte) ([]byte, error) {
	return hkdf.Extract(sha256.New, secret, salt)
}
