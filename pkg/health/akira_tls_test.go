// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package health

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	pqtls "unheaded/pkg/tls"
)

// S-IRON-GATE Phase 8: with UNHEADED_TLS_ENABLED=true Akira checks
// first-party targets over https with the deployment CA; a target marked
// scheme http (a third-party service on the host) stays plain.

func TestCheckService_TLSFromEnv(t *testing.T) {
	ca, err := pqtls.NewCA()
	if err != nil {
		t.Fatal(err)
	}
	sc, err := ca.IssueCert(pqtls.CertRequest{ServiceName: "svc", TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	caFile := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caFile, ca.CertPEM(), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := pqtls.NewServerTLSConfig(pqtls.ServerConfig{CertPEM: sc.CertPEM, KeyPEM: sc.KeyPEM})
	if err != nil {
		t.Fatal(err)
	}
	tlsSrv := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	tlsSrv.TLS = pqtls.ApplyStrict(cfg)
	tlsSrv.StartTLS()
	defer tlsSrv.Close()
	plainSrv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer plainSrv.Close()

	t.Setenv("UNHEADED_TLS_ENABLED", "true")
	t.Setenv("UNHEADED_TLS_CA_CERT", caFile)
	t.Setenv("UNHEADED_TLS_CERT", "")
	t.Setenv("UNHEADED_TLS_KEY_FILE", "")

	port := func(s *httptest.Server) int {
		_, p, _ := net.SplitHostPort(s.Listener.Addr().String())
		n, _ := strconv.Atoi(p)
		return n
	}
	a := NewAkira("n", nil)
	c, err := pqtls.ClientFromEnv(5 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	a.SetHTTPClient(c)

	if r := a.CheckService(ServiceTarget{Name: "first-party", Host: "localhost", Port: port(tlsSrv)}); !r.Healthy {
		t.Fatalf("TLS target unhealthy: %s", r.Error)
	}
	if r := a.CheckService(ServiceTarget{Name: "llama", Host: "127.0.0.1", Port: port(plainSrv), Scheme: "http"}); !r.Healthy {
		t.Fatalf("plain target marked http unhealthy: %s", r.Error)
	}
	// A plaintext first-party target is not trusted as healthy over TLS.
	if r := a.CheckService(ServiceTarget{Name: "plain", Host: "127.0.0.1", Port: port(plainSrv)}); r.Healthy {
		t.Fatal("a plaintext server passed an https check")
	}
}
