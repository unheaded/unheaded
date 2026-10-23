// SPDX-License-Identifier: GPL-3.0-or-later

package fx

import "crypto/tls"

// (g) hybrid groups only: a classical-only peer cannot complete a handshake.
var G = &tls.Config{
	MinVersion:       tls.VersionTLS13,
	CurvePreferences: []tls.CurveID{tls.X25519MLKEM768},
}

// Hybrid first with a classical fallback, TLS 1.3 floor.
var GMixed = &tls.Config{
	MinVersion:       tls.VersionTLS13,
	CurvePreferences: []tls.CurveID{tls.X25519MLKEM768, tls.X25519},
}
