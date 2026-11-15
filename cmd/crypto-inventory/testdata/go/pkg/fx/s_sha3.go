// SPDX-License-Identifier: GPL-3.0-or-later

package fx

import (
	"crypto/sha3"

	xsha3 "golang.org/x/crypto/sha3"
)

// (s) SHA-3 imports, stdlib and x/crypto.
var (
	S1 = sha3.New256
	S2 = xsha3.New256
)
