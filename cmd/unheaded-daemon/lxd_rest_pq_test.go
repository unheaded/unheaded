// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	pqtls "unheaded/pkg/tls"
	"unheaded/pkg/tls/pqtest"
)

// ADR-105 Preferred: the LXD REST client negotiates a hybrid ML-KEM group
// with a server that offers one (LXD 5.21 is a Go binary with Go defaults).
func TestHandshakePQ_LXDClientIsHybrid(t *testing.T) {
	dir := t.TempDir()
	serverCrt, _, serverPair := selfSigned(t, dir, "server")
	clientCrt, clientKey, _ := selfSigned(t, dir, "client")

	var mu sync.Mutex
	var seen tls.ConnectionState
	f := newFakeLXD()
	inner := f.handler()
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS != nil {
			mu.Lock()
			seen = *r.TLS
			mu.Unlock()
		}
		inner.ServeHTTP(w, r)
	}))
	srv.TLS = pqtls.ApplyPreferred(&tls.Config{Certificates: []tls.Certificate{serverPair}, ClientAuth: tls.RequireAnyClientCert})
	srv.StartTLS()
	t.Cleanup(srv.Close)

	c, err := newLXDREST(LXDRESTConfig{URL: srv.URL, ClientCert: clientCrt, ClientKey: clientKey, ServerCert: serverCrt})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.serverInfo(t.Context()); err != nil {
		t.Fatalf("serverInfo: %v", err)
	}
	mu.Lock()
	cs := seen
	mu.Unlock()
	pqtest.RequireHybrid(t, cs)
}
