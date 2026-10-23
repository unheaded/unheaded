// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package loadbalancer

import (
	"crypto/tls"
	"testing"

	"unheaded/pkg/tls/pqtest"
)

// ADR-105 Preferred on load-balancer frontends: TLS 1.3 by default, hybrid
// ML-KEM with any modern client. min_version / max_version "1.2" are
// operator opt-ins (accepted in docs/policy/pq-kex-baseline.txt).

func lbFrontend(t *testing.T, minV, maxV string) (*tls.Config, *tls.Config) {
	t.Helper()
	cert := pqtest.SelfSigned(t)
	certFile, keyFile := pqtest.WritePEM(t, t.TempDir(), cert)
	srv, err := (&TLSConfig{CertFile: certFile, KeyFile: keyFile, MinVersion: minV, MaxVersion: maxV}).BuildTLSConfig()
	if err != nil {
		t.Fatalf("BuildTLSConfig: %v", err)
	}
	return srv, &tls.Config{RootCAs: pqtest.Pool(cert), ServerName: "localhost", MinVersion: tls.VersionTLS12}
}

func TestHandshakePQ_FrontendDefaultIsTLS13Hybrid(t *testing.T) {
	srv, cli := lbFrontend(t, "", "")
	if srv.MinVersion != tls.VersionTLS13 {
		t.Errorf("unset min_version built MinVersion %#x, want TLS 1.3", srv.MinVersion)
	}
	cs, err := pqtest.Handshake(t, srv, cli)
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}
	pqtest.RequireHybrid(t, cs)

	// A TLS 1.2-only client is refused by the default frontend.
	old := cli.Clone()
	old.MaxVersion = tls.VersionTLS12
	if cs, err := pqtest.Handshake(t, srv, old); err == nil {
		t.Errorf("default frontend accepted a TLS 1.2-only client (group %v)", cs.CurveID)
	}
}

func TestHandshakePQ_FrontendOptIn12StillHybridForModernClients(t *testing.T) {
	srv, cli := lbFrontend(t, "1.2", "")
	if srv.MinVersion != tls.VersionTLS12 {
		t.Errorf("min_version 1.2 built MinVersion %#x, want the opt-in TLS 1.2", srv.MinVersion)
	}
	cs, err := pqtest.Handshake(t, srv, cli)
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}
	pqtest.RequireHybrid(t, cs)
}

// The accepted opt-in: max_version "1.2" makes every session classical.
func TestHandshakePQ_FrontendMax12OptInIsClassical(t *testing.T) {
	srv, cli := lbFrontend(t, "1.2", "1.2")
	cs, err := pqtest.Handshake(t, srv, cli)
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}
	if cs.Version != tls.VersionTLS12 {
		t.Fatalf("version %s, want the TLS 1.2 cap", tls.VersionName(cs.Version))
	}
}
