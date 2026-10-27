// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package wotanClient

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"google.golang.org/grpc/credentials"

	pqtls "unheaded/pkg/tls"
)

// Transport security for Wotan clients (S-IRON-GATE Phase 8, FND-017).
//
// With UNHEADED_TLS_ENABLED=true every client dials Wotan's gRPC and HTTP
// ports over TLS 1.3 with the Strict hybrid profile, trusting only the
// deployment CA (UNHEADED_TLS_CA_CERT) and verifying the server name
// against the dialled host; a missing or unreadable CA is a constructor
// error, never a plaintext fallback. Without it a client dials plaintext
// (pkg/tls.PlaintextGRPCCredentials logs that once) and sends its service
// token only to loopback. UNHEADED_GRPC_TLS=1 is the older gRPC-only switch
// (system roots) and still works when UNHEADED_TLS_ENABLED is unset.

// errInsecureUnderTLS: WithInsecureTransport on a service told to use TLS.
var errInsecureUnderTLS = errors.New("wotan client: WithInsecureTransport refused: UNHEADED_TLS_ENABLED=true")

// grpcCredentials picks the gRPC transport credentials for a client built
// with opts.
func grpcCredentials(opts []ClientOption) (credentials.TransportCredentials, error) {
	o := collectOptions(opts)
	if o.insecureOK {
		if pqtls.Enabled() {
			return nil, errInsecureUnderTLS
		}
		return pqtls.PlaintextGRPCCredentials("wotan-client WithInsecureTransport")
	}
	creds, on, err := pqtls.GRPCClientCredentialsFromEnv()
	if err != nil {
		return nil, fmt.Errorf("wotan client TLS: %w", err)
	}
	if on {
		return creds, nil
	}
	if os.Getenv("UNHEADED_GRPC_TLS") == "1" {
		return credentials.NewTLS(TLSConfig()), nil
	}
	return pqtls.PlaintextGRPCCredentials("wotan-client")
}

// httpBase returns the REST base URL (".../api/v1") for addr and the
// transport to reach it: https and the deployment CA when
// UNHEADED_TLS_ENABLED=true, else plain http and the default transport
// (nil). Under TLS an http:// address is upgraded to https:// (Wotan's
// HTTP and gRPC ports serve TLS together; many callers still build
// "http://"+WOTAN_ADDR), never the other way round.
func httpBase(addr string, opts []ClientOption) (string, http.RoundTripper, error) {
	o := collectOptions(opts)
	tlsOn := pqtls.Enabled()
	if o.insecureOK && tlsOn {
		return "", nil, errInsecureUnderTLS
	}
	addr = strings.TrimRight(strings.TrimSpace(addr), "/")
	if tlsOn && strings.HasPrefix(addr, "http://") {
		addr = "https://" + strings.TrimPrefix(addr, "http://")
	}
	base := addr
	if !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") {
		base = pqtls.Scheme() + "://" + addr
	}
	if !strings.HasPrefix(base, "https://") {
		return base + "/api/v1", nil, nil
	}
	tr, err := pqtls.TransportFromEnv()
	if err != nil {
		return "", nil, fmt.Errorf("wotan client TLS: %w", err)
	}
	if !tlsOn {
		// An https:// address without UNHEADED_TLS_ENABLED: system roots.
		return base + "/api/v1", nil, nil
	}
	return base + "/api/v1", tr, nil
}
