// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package tls_test

import (
	"crypto/tls"
	"testing"

	pqtls "unheaded/pkg/tls"
	"unheaded/pkg/tls/pqtest"
)

// ADR-105 handshake tests: they read the negotiated group from
// ConnectionState.CurveID, so they fail if the profiles stop being hybrid.

func profiles(t *testing.T) (cert tls.Certificate, client func(groups ...tls.CurveID) *tls.Config) {
	t.Helper()
	cert = pqtest.SelfSigned(t)
	pool := pqtest.Pool(cert)
	return cert, func(groups ...tls.CurveID) *tls.Config {
		return &tls.Config{RootCAs: pool, ServerName: "localhost", MinVersion: tls.VersionTLS13, CurvePreferences: groups}
	}
}

func TestHandshakePQ_StrictServerDefaultClientIsHybrid(t *testing.T) {
	cert, client := profiles(t)
	srv := pqtls.ApplyStrict(&tls.Config{Certificates: []tls.Certificate{cert}})
	cs, err := pqtest.Handshake(t, srv, client())
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}
	pqtest.RequireHybrid(t, cs)
	if cs.CurveID != tls.X25519MLKEM768 {
		t.Errorf("group = %v, want X25519MLKEM768", cs.CurveID)
	}
}

func TestHandshakePQ_StrictServerRefusesClassicalClient(t *testing.T) {
	cert, client := profiles(t)
	srv := pqtls.ApplyStrict(&tls.Config{Certificates: []tls.Certificate{cert}})
	if cs, err := pqtest.Handshake(t, srv, client(tls.X25519)); err == nil {
		t.Fatalf("downgrade accepted: an X25519-only client negotiated %v with a Strict server", cs.CurveID)
	}
}

func TestHandshakePQ_StrictClientRefusesClassicalServer(t *testing.T) {
	cert, client := profiles(t)
	srv := &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS13, CurvePreferences: []tls.CurveID{tls.X25519}}
	if cs, err := pqtest.Handshake(t, srv, pqtls.ApplyStrict(client())); err == nil {
		t.Fatalf("a Strict client negotiated %v with an X25519-only server", cs.CurveID)
	}
}

func TestHandshakePQ_PreferredServerDefaultClientIsHybrid(t *testing.T) {
	cert, client := profiles(t)
	srv := pqtls.ApplyPreferred(&tls.Config{Certificates: []tls.Certificate{cert}})
	cs, err := pqtest.Handshake(t, srv, client())
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}
	pqtest.RequireHybrid(t, cs)
}

// The accepted fallback: a Preferred server still serves a classical-only
// client (VictoriaMetrics, old curl), and that session is X25519.
func TestHandshakePQ_PreferredServerAcceptedClassicalFallback(t *testing.T) {
	cert, client := profiles(t)
	srv := pqtls.ApplyPreferred(&tls.Config{Certificates: []tls.Certificate{cert}})
	cs, err := pqtest.Handshake(t, srv, client(tls.X25519))
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}
	if cs.CurveID != tls.X25519 {
		t.Fatalf("group = %v, want the X25519 fallback", cs.CurveID)
	}
}

func TestHandshakePQ_NewServerTLSConfigIsHybrid(t *testing.T) {
	ca, err := pqtls.NewCA()
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}
	sc, err := ca.IssueCert(pqtls.CertRequest{ServiceName: "svc"})
	if err != nil {
		t.Fatalf("IssueCert: %v", err)
	}
	srv, err := pqtls.NewServerTLSConfig(pqtls.ServerConfig{CertPEM: sc.CertPEM, KeyPEM: sc.KeyPEM})
	if err != nil {
		t.Fatalf("NewServerTLSConfig: %v", err)
	}
	cli, err := pqtls.NewClientTLSConfig(pqtls.ClientConfig{CAPEM: sc.CAPEM})
	if err != nil {
		t.Fatalf("NewClientTLSConfig: %v", err)
	}
	cli.ServerName = "localhost"
	cs, err := pqtest.Handshake(t, srv, cli)
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}
	pqtest.RequireHybrid(t, cs)
}
