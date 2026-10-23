// SPDX-License-Identifier: GPL-3.0-or-later

package main

import "crypto/tls"

// The red-team tool probes weak TLS on purpose; the lint exempts it.
var weak = &tls.Config{
	MaxVersion:       tls.VersionTLS12,
	CurvePreferences: []tls.CurveID{tls.CurveP256},
}
