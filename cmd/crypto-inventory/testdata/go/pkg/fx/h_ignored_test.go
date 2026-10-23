// SPDX-License-Identifier: GPL-3.0-or-later

package fx

import "crypto/tls"

// (h) _test.go files are not inventoried.
var H = &tls.Config{CurvePreferences: []tls.CurveID{tls.X25519}}
