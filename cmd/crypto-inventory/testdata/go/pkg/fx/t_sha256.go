// SPDX-License-Identifier: GPL-3.0-or-later

package fx

import (
	"crypto/sha256"
	"crypto/sha512"
)

// (t) SHA-2 from the standard library is a digest everywhere it appears
// (content ids, cache keys, signature pre-hashes): no row.
var (
	T1 = sha256.Size
	T2 = sha512.Size
)
