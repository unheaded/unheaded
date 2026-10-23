// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package mesh

import (
	"crypto/tls"
	"testing"

	"unheaded/pkg/tls/pqtest"
)

// ADR-105 Strict: mesh peers (both ends ours) negotiate a hybrid ML-KEM
// group, and a classical-only peer gets no session.
func TestHandshakePQ_MeshIsHybrid(t *testing.T) {
	c := DefaultConfig()
	c.DiscoveryType = DiscoveryTypeStatic
	c.InboundPort = 0
	c.OutboundPort = 0
	m, err := NewMesh(c)
	if err != nil {
		t.Fatalf("NewMesh: %v", err)
	}
	cert := pqtest.SelfSigned(t)
	if err := m.ConfigureTLS(cert, pqtest.Pool(cert)); err != nil {
		t.Fatalf("ConfigureTLS: %v", err)
	}
	srv := m.ServerTLSConfig()
	cli := m.ClientTLSConfig().Clone()
	cli.ServerName = "localhost"

	cs, err := pqtest.Handshake(t, srv, cli)
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}
	pqtest.RequireHybrid(t, cs)

	classical := cli.Clone()
	classical.CurvePreferences = []tls.CurveID{tls.X25519}
	if cs, err := pqtest.Handshake(t, srv, classical); err == nil {
		t.Errorf("mesh server accepted an X25519-only client (group %v)", cs.CurveID)
	}
}
