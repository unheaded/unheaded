// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"crypto/tls"

	pqtls "unheaded/pkg/tls"
)

// httpTLSConfig is the HTTP listener's TLS (ADR-105 Preferred): TLS 1.3 and
// Go's default groups, hybrid X25519MLKEM768 first with a classical fallback,
// because VictoriaMetrics, curl and browsers reach /metrics and the API.
// The certificate is loaded by ListenAndServeTLS.
func httpTLSConfig() *tls.Config {
	return pqtls.ApplyPreferred(&tls.Config{MinVersion: tls.VersionTLS13})
}

// grpcTLSConfig is the gRPC listener's TLS (ADR-105 Strict): TLS 1.3 and
// hybrid groups only. Both ends are our Go binaries (wotan-client), so a
// classical-only peer is refused instead of downgraded.
func grpcTLSConfig(cert tls.Certificate) *tls.Config {
	return pqtls.ApplyStrict(&tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{cert},
	})
}
