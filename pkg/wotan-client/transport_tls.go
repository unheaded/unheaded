// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package wotanClient

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"

	"google.golang.org/grpc"
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
//
// WOTAN_TLS_SERVER_NAME (EnvTLSServerName) is for a client that reaches
// Wotan at an address its certificate does not name, such as the host's
// overlay address from another compose project (alert-mailer): the
// certificate is then verified against that name instead of the dialled
// host, on the gRPC and the HTTP side. Nothing else changes: the chain must
// still end at the trusted CA and the certificate must carry the name.

// EnvTLSServerName is the environment variable holding the name a Wotan
// client verifies the server certificate against, when that is not the
// host it dials. Empty or unset: the dialled host.
const EnvTLSServerName = "WOTAN_TLS_SERVER_NAME"

// tlsServerName returns EnvTLSServerName's value, "" when it is not set.
// A value that cannot be a certificate name (a port, a scheme, a path, a
// wildcard, brackets) is an error for the constructor: it is never dropped
// in favour of the dialled host.
func tlsServerName() (string, error) {
	name := strings.TrimSpace(os.Getenv(EnvTLSServerName))
	if name == "" {
		return "", nil
	}
	if net.ParseIP(name) == nil && !validDNSName(name) {
		return "", fmt.Errorf("wotan client TLS: %s=%q: want a host name or an IP address, without port, scheme or brackets", EnvTLSServerName, name)
	}
	return name, nil
}

// validDNSName reports whether s is a host name as a certificate carries
// one: labels of letters, digits and inner hyphens, 253 bytes at most.
func validDNSName(s string) bool {
	if s == "" || len(s) > 253 {
		return false
	}
	for _, label := range strings.Split(s, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' {
				return false
			}
		}
	}
	return true
}

// errInsecureUnderTLS: WithInsecureTransport on a service told to use TLS.
var errInsecureUnderTLS = errors.New("wotan client: WithInsecureTransport refused: UNHEADED_TLS_ENABLED=true")

// grpcCredentials picks the gRPC transport credentials for a client built
// with opts. A malformed WOTAN_TLS_SERVER_NAME is an error here, so every
// constructor refuses it; the name itself is applied by
// grpcServerNameOption.
func grpcCredentials(opts []ClientOption) (credentials.TransportCredentials, error) {
	o := collectOptions(opts)
	if _, err := tlsServerName(); err != nil {
		return nil, err
	}
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

// grpcServerNameOption is the dial option that makes a gRPC client built
// from the environment's credentials (grpcCredentials) verify Wotan's
// certificate against WOTAN_TLS_SERVER_NAME: gRPC's TLS handshake takes the
// server name from the channel's authority, so the name is set there
// (grpc.WithAuthority). An IPv6 address goes in brackets, as an authority
// writes one. Nothing for a plaintext link or when the variable is unset.
func grpcServerNameOption(creds credentials.TransportCredentials) []grpc.DialOption {
	name, err := tlsServerName()
	if err != nil || name == "" || isPlaintext(creds) {
		return nil
	}
	if ip := net.ParseIP(name); ip != nil && ip.To4() == nil {
		name = "[" + name + "]"
	}
	return []grpc.DialOption{grpc.WithAuthority(name)}
}

// httpBase returns the REST base URL (".../api/v1") for addr and the
// transport to reach it: https and the deployment CA when
// UNHEADED_TLS_ENABLED=true, else plain http and the default transport
// (nil). Under TLS an http:// address is upgraded to https:// (Wotan's
// HTTP and gRPC ports serve TLS together; many callers still build
// "http://"+WOTAN_ADDR), never the other way round.
func httpBase(addr string, opts []ClientOption) (string, http.RoundTripper, error) {
	o := collectOptions(opts)
	name, err := tlsServerName()
	if err != nil {
		return "", nil, err
	}
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
		if name == "" {
			return base + "/api/v1", nil, nil
		}
		tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS13, ServerName: name}
		return base + "/api/v1", tr, nil
	}
	// "" leaves the name to net/http: the dialled host.
	tr.TLSClientConfig.ServerName = name
	return base + "/api/v1", tr, nil
}
