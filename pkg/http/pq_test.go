// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package http

import (
	"crypto/tls"
	"io"
	"log"
	"net"
	"testing"
	"time"

	"unheaded/pkg/tls/pqtest"
)

// ADR-105 Preferred: ListenAndServeTLS without a caller-supplied TLSConfig
// serves TLS 1.3 with hybrid ML-KEM first.
func TestHandshakePQ_ListenAndServeTLSIsHybrid(t *testing.T) {
	cert := pqtest.SelfSigned(t)
	certFile, keyFile := pqtest.WritePEM(t, t.TempDir(), cert)
	s := NewServerWithConfig(NewRouter(), ServerConfig{Logger: log.New(io.Discard, "", 0)})
	started := make(chan struct{})
	s.OnStart(func() { close(started) })
	errc := make(chan error, 1)
	go func() { errc <- s.ListenAndServeTLS("127.0.0.1:0", certFile, keyFile) }()
	select {
	case <-started:
	case err := <-errc:
		t.Fatalf("ListenAndServeTLS: %v", err)
	case <-time.After(pqtest.Timeout):
		t.Fatal("server did not start")
	}
	t.Cleanup(func() { _ = s.ForceShutdown() })

	d := &net.Dialer{Timeout: pqtest.Timeout}
	conn, err := tls.DialWithDialer(d, "tcp", s.Addr(),
		&tls.Config{RootCAs: pqtest.Pool(cert), ServerName: "localhost", MinVersion: tls.VersionTLS12})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	cs := conn.ConnectionState()
	_ = conn.Close()
	pqtest.RequireHybrid(t, cs)
}

// EnableHTTP2 builds the same fallback config.
func TestHandshakePQ_EnableHTTP2ConfigIsHybrid(t *testing.T) {
	s := NewServerWithConfig(NewRouter(), ServerConfig{Logger: log.New(io.Discard, "", 0)})
	s.EnableHTTP2()
	cert := pqtest.SelfSigned(t)
	srv := s.server.TLSConfig.Clone()
	srv.Certificates = []tls.Certificate{cert}
	srv.NextProtos = nil
	cs, err := pqtest.Handshake(t, srv, &tls.Config{RootCAs: pqtest.Pool(cert), ServerName: "localhost", MinVersion: tls.VersionTLS12})
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}
	pqtest.RequireHybrid(t, cs)
}
