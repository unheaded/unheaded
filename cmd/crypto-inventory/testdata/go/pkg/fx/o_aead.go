// SPDX-License-Identifier: GPL-3.0-or-later

package fx

import "golang.org/x/crypto/chacha20poly1305"

// (o) an AEAD package import is an authenticated-encryption site.
var O = chacha20poly1305.KeySize
