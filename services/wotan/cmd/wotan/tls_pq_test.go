// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"crypto/tls"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/peer"

	"unheaded/pkg/tls/pqtest"
	wotanClient "unheaded/pkg/wotan-client"
)

// ADR-105: Wotan's listeners negotiate a hybrid ML-KEM group. The tests read
// ConnectionState.CurveID, so re-pinning a classical curve turns them red.

// TestHTTPTLSHybrid: the HTTP listener's config (Preferred) with a default
// Go client negotiates a hybrid group over a real loopback socket.
func TestHTTPTLSHybrid(t *testing.T) {
	cert := pqtest.SelfSigned(t)
	cfg := httpTLSConfig()
	cfg.Certificates = []tls.Certificate{cert}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", cfg)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	done := make(chan error, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			done <- err
			return
		}
		defer c.Close()
		_ = c.SetDeadline(time.Now().Add(pqtest.Timeout))
		done <- c.(*tls.Conn).Handshake()
	}()

	d := &net.Dialer{Timeout: pqtest.Timeout}
	conn, err := tls.DialWithDialer(d, "tcp", ln.Addr().String(),
		&tls.Config{RootCAs: pqtest.Pool(cert), ServerName: "localhost", MinVersion: tls.VersionTLS13})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	cs := conn.ConnectionState()
	_ = conn.Close()
	if err := <-done; err != nil {
		t.Fatalf("server handshake: %v", err)
	}
	pqtest.RequireHybrid(t, cs)
}

// startGRPC serves the health service with Wotan's gRPC TLS (Strict) and
// records the server-side negotiated group of each call.
func startGRPC(t *testing.T, cert tls.Certificate) (addr string, groups chan tls.CurveID) {
	t.Helper()
	groups = make(chan tls.CurveID, 4)
	record := func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
		if p, ok := peer.FromContext(ctx); ok {
			if ti, ok := p.AuthInfo.(credentials.TLSInfo); ok {
				select {
				case groups <- ti.State.CurveID:
				default:
				}
			}
		}
		return h(ctx, req)
	}
	srv := grpc.NewServer(grpc.Creds(credentials.NewTLS(grpcTLSConfig(cert))), grpc.UnaryInterceptor(record))
	hs := health.NewServer()
	healthpb.RegisterHealthServer(srv, hs)
	hs.SetServingStatus("wotan", healthpb.HealthCheckResponse_SERVING)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(srv.Stop)
	return ln.Addr().String(), groups
}

func checkHealth(t *testing.T, addr string, cfg *tls.Config) (tls.ConnectionState, error) {
	t.Helper()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(credentials.NewTLS(cfg)))
	if err != nil {
		t.Fatalf("grpc client: %v", err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), pqtest.Timeout)
	defer cancel()
	var p peer.Peer
	_, err = healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{Service: "wotan"}, grpc.Peer(&p))
	if err != nil {
		return tls.ConnectionState{}, err
	}
	ti, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		t.Fatalf("peer auth info %T, want credentials.TLSInfo", p.AuthInfo)
	}
	return ti.State, nil
}

// TestGRPCTLSHybrid: wotan-client's TLS (Strict) against Wotan's gRPC TLS
// (Strict): both ends see a hybrid group.
func TestGRPCTLSHybrid(t *testing.T) {
	cert := pqtest.SelfSigned(t)
	addr, groups := startGRPC(t, cert)
	cfg := wotanClient.TLSConfig()
	cfg.RootCAs = pqtest.Pool(cert)
	cfg.ServerName = "localhost"
	cs, err := checkHealth(t, addr, cfg)
	if err != nil {
		t.Fatalf("Check over Strict TLS: %v", err)
	}
	pqtest.RequireHybrid(t, cs)
	select {
	case g := <-groups:
		if g != cs.CurveID {
			t.Errorf("server saw %v, client saw %v", g, cs.CurveID)
		}
	case <-time.After(pqtest.Timeout):
		t.Fatal("interceptor recorded no server-side group")
	}
}

// TestGRPCTLSRefusesClassicalPQ: an X25519-only client cannot reach Wotan gRPC.
func TestGRPCTLSRefusesClassicalPQ(t *testing.T) {
	cert := pqtest.SelfSigned(t)
	addr, _ := startGRPC(t, cert)
	cfg := &tls.Config{RootCAs: pqtest.Pool(cert), ServerName: "localhost", MinVersion: tls.VersionTLS13,
		CurvePreferences: []tls.CurveID{tls.X25519}}
	if cs, err := checkHealth(t, addr, cfg); err == nil {
		t.Fatalf("downgrade accepted: an X25519-only client negotiated %v with Wotan gRPC", cs.CurveID)
	}
}
