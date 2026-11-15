// SPDX-License-Identifier: GPL-3.0-or-later

package fx

import "crypto/aes"

// (r) crypto/aes with no GCM in the file: a block cipher whose mode is
// not visible here.
var R = aes.BlockSize
