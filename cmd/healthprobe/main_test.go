// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	utls "unheaded/pkg/tls"
)

func TestTarget(t *testing.T) {
	t.Setenv("UNHEADED_TLS_ENABLED", "")
	if got, _ := target("19000"); got != "http://127.0.0.1:19000/health" {
		t.Errorf("TLS off: %q", got)
	}
	t.Setenv("UNHEADED_TLS_ENABLED", "true")
	if got, _ := target("19000"); got != "https://127.0.0.1:19000/health" {
		t.Errorf("TLS on: %q", got)
	}
	if got, _ := target("https://localhost:1/ready"); got != "https://localhost:1/ready" {
		t.Errorf("URL: %q", got)
	}
	for _, bad := range []string{"0", "70000", "abc", "ftp://x/"} {
		if _, err := target(bad); err == nil {
			t.Errorf("target(%q) accepted", bad)
		}
	}
}

func TestRun_TLSHealthyAndPlainRefused(t *testing.T) {
	ca, err := utls.NewCA()
	if err != nil {
		t.Fatal(err)
	}
	sc, err := ca.IssueCert(utls.CertRequest{ServiceName: "svc", TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	caFile := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caFile, ca.CertPEM(), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := utls.NewServerTLSConfig(utls.ServerConfig{CertPEM: sc.CertPEM, KeyPEM: sc.KeyPEM})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	srv.TLS = utls.ApplyStrict(cfg)
	srv.StartTLS()
	defer srv.Close()
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())

	t.Setenv("UNHEADED_TLS_ENABLED", "true")
	t.Setenv("UNHEADED_TLS_CA_CERT", caFile)
	t.Setenv("UNHEADED_TLS_CERT", "")
	t.Setenv("UNHEADED_TLS_KEY_FILE", "")
	if code := run([]string{port}, io.Discard); code != 0 {
		t.Fatalf("healthy TLS service: exit %d", code)
	}
	if code := run([]string{"https://127.0.0.1:" + port + "/nope"}, io.Discard); code != 1 {
		t.Fatalf("404: exit %d, want 1", code)
	}
	t.Setenv("UNHEADED_TLS_ENABLED", "")
	if code := run([]string{port}, io.Discard); code != 1 {
		t.Fatalf("plain http to a TLS port: exit %d, want 1", code)
	}
	if code := run(nil, io.Discard); code != 2 {
		t.Fatalf("no args: exit %d, want 2", code)
	}
}
