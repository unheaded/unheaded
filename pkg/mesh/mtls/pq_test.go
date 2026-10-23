// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package mtls

import (
	"crypto/tls"
	"testing"

	"unheaded/pkg/tls/pqtest"
)

// ADR-105 Strict for the mesh mTLS Manager and Provider: hybrid ML-KEM or
// no session.

func TestHandshakePQ_ManagerIsHybrid(t *testing.T) {
	srv, cli, cleanup := helperSetupMTLSPair(t)
	defer cleanup()
	cs, err := pqtest.Handshake(t, srv, cli)
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}
	pqtest.RequireHybrid(t, cs)

	classical := cli.Clone()
	classical.CurvePreferences = []tls.CurveID{tls.X25519}
	if cs, err := pqtest.Handshake(t, srv, classical); err == nil {
		t.Errorf("Manager server accepted an X25519-only client (group %v)", cs.CurveID)
	}
}

func TestHandshakePQ_ProviderIsHybrid(t *testing.T) {
	caCert, caKey := generateTestCA()
	// One Provider on both ends: each Provider mints its own intermediate,
	// so two Providers do not trust each other's client certificates.
	cfg := DefaultProviderConfig("example.com", "svc", "default")
	cfg.AutoRotate = false
	p := NewProvider(cfg)
	if err := p.InitializeWithCA(caCert, caKey); err != nil {
		t.Fatalf("InitializeWithCA: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })
	srv := p.ServerTLSConfig()
	cli := p.ClientTLSConfig()
	// Same accommodation as helperSetupMTLSPair: the SPIFFE certificates carry
	// no DNS name, so the chain check is skipped on the client.
	cli.InsecureSkipVerify = true
	cli.VerifyPeerCertificate = nil

	cs, err := pqtest.Handshake(t, srv, cli)
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}
	pqtest.RequireHybrid(t, cs)

	classical := cli.Clone()
	classical.CurvePreferences = []tls.CurveID{tls.X25519}
	if cs, err := pqtest.Handshake(t, srv, classical); err == nil {
		t.Errorf("Provider server accepted an X25519-only client (group %v)", cs.CurveID)
	}
}
