// SPDX-License-Identifier: GPL-3.0-or-later

package main

import "crypto/tls"

// The red-team tool probes weak TLS on purpose; the lint exempts it.
var weak = &tls.Config{ // #nosec G402 -- scanner test fixture, never built into a binary: the weak config the lint must exempt for cmd/lich-security
	MaxVersion:       tls.VersionTLS12,
	CurvePreferences: []tls.CurveID{tls.CurveP256},
}
