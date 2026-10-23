// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package mtls

import (
	"crypto/tls"
	"testing"
	"time"

	"unheaded/pkg/tls/pqtest"
)

// ADR-105 Strict: first-party mTLS negotiates a hybrid ML-KEM group, and a
// classical-only peer gets no session at all.

func pqPKI(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	caCert, caKey, err := GenerateRootCA(time.Hour)
	if err != nil {
		t.Fatalf("GenerateRootCA: %v", err)
	}
	if err := WritePKI(dir, caCert, caKey, []string{"svc-a", "svc-b"}, time.Hour); err != nil {
		t.Fatalf("WritePKI: %v", err)
	}
	return dir
}

func TestHandshakePQ_MTLSIsHybrid(t *testing.T) {
	dir := pqPKI(t)
	srv, err := ServerTLSConfig(Config{PKIDir: dir, ServiceName: "svc-a"})
	if err != nil {
		t.Fatalf("ServerTLSConfig: %v", err)
	}
	cli, err := ClientTLSConfig(Config{PKIDir: dir, ServiceName: "svc-b"})
	if err != nil {
		t.Fatalf("ClientTLSConfig: %v", err)
	}
	cli.ServerName = "localhost"
	cs, err := pqtest.Handshake(t, srv, cli)
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}
	pqtest.RequireHybrid(t, cs)
}

func TestHandshakePQ_RotatingMTLSIsHybrid(t *testing.T) {
	dir := pqPKI(t)
	srvRot, err := NewCertRotator(Config{PKIDir: dir, ServiceName: "svc-a"})
	if err != nil {
		t.Fatalf("NewCertRotator: %v", err)
	}
	cliRot, err := NewCertRotator(Config{PKIDir: dir, ServiceName: "svc-b"})
	if err != nil {
		t.Fatalf("NewCertRotator: %v", err)
	}
	cli := RotatingClientTLSConfig(cliRot)
	cli.ServerName = "localhost"
	cs, err := pqtest.Handshake(t, RotatingServerTLSConfig(srvRot), cli)
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}
	pqtest.RequireHybrid(t, cs)
}

func TestHandshakePQ_MTLSRefusesClassicalPeers(t *testing.T) {
	dir := pqPKI(t)
	srv, err := ServerTLSConfig(Config{PKIDir: dir, ServiceName: "svc-a"})
	if err != nil {
		t.Fatalf("ServerTLSConfig: %v", err)
	}
	cli, err := ClientTLSConfig(Config{PKIDir: dir, ServiceName: "svc-b"})
	if err != nil {
		t.Fatalf("ClientTLSConfig: %v", err)
	}
	cli.ServerName = "localhost"

	classicalCli := cli.Clone()
	classicalCli.CurvePreferences = []tls.CurveID{tls.X25519}
	if cs, err := pqtest.Handshake(t, srv, classicalCli); err == nil {
		t.Errorf("server accepted an X25519-only client (group %v)", cs.CurveID)
	}

	classicalSrv := srv.Clone()
	classicalSrv.CurvePreferences = []tls.CurveID{tls.X25519}
	if cs, err := pqtest.Handshake(t, classicalSrv, cli); err == nil {
		t.Errorf("client accepted an X25519-only server (group %v)", cs.CurveID)
	}
}
