// SPDX-License-Identifier: GPL-3.0-or-later

package fx

import "crypto/tls"

// (a) a classical-only curve pin: turns Go's hybrid default off.
var A = &tls.Config{
	MinVersion:       tls.VersionTLS13,
	CurvePreferences: []tls.CurveID{tls.X25519, tls.CurveP256},
}
