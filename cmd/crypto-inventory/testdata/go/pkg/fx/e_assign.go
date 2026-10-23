// SPDX-License-Identifier: GPL-3.0-or-later

package fx

import "crypto/tls"

// (e) the pin arrives by assignment after the literal.
func E() *tls.Config {
	cfg := &tls.Config{MinVersion: tls.VersionTLS13}
	cfg.CurvePreferences = []tls.CurveID{tls.X25519}
	return cfg
}

// A MaxVersion cap at 1.2 forces a classical session.
func ECap(cfg *tls.Config) {
	cfg.MaxVersion = tls.VersionTLS12
}
