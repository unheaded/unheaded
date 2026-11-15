// SPDX-License-Identifier: GPL-3.0-or-later

package fx

import (
	"crypto/aes"
	ciph "crypto/cipher"
)

// (q) crypto/aes wrapped in GCM in the same file is AES-GCM.
func Q(key []byte) (ciph.AEAD, error) {
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return ciph.NewGCM(b)
}
