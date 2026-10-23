// SPDX-License-Identifier: GPL-3.0-or-later

package fx

import "crypto/tls"

// (c) a server with a TLS 1.2 floor: a 1.2 session is always classical.
func C(cert tls.Certificate) *tls.Config {
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}
}
