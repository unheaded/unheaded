// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package healthreport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pqtls "unheaded/pkg/tls"
)

// S-IRON-GATE Phase 8: a service on TLS probes its own /health over https,
// trusting the deployment CA.

func TestHealthURL_HTTPSWhenTLSOn(t *testing.T) {
	t.Setenv("UNHEADED_TLS_ENABLED", "true")
	t.Setenv("UNHEADED_TLS_CERT", "/etc/unheaded/tls/x/x.pem")
	t.Setenv("UNHEADED_TLS_KEY_FILE", "/etc/unheaded/tls/x/x-key.pem")
	if got := HealthURL(":19000", 19000); got != "https://127.0.0.1:19000/health" {
		t.Fatalf("HealthURL with TLS on = %q", got)
	}
	// A client-only unit (TLS to others, plain listener: no cert of its
	// own) probes itself over http.
	t.Setenv("UNHEADED_TLS_CERT", "")
	if got := HealthURL(":19012", 19012); got != "http://127.0.0.1:19012/health" {
		t.Fatalf("HealthURL for a client-only unit = %q", got)
	}
}

func TestHTTPProbe_TLSTrustsDeploymentCA(t *testing.T) {
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
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	srv.TLS = pqtls.ApplyStrict(cfg)
	srv.StartTLS()
	defer srv.Close()

	t.Setenv("UNHEADED_TLS_ENABLED", "true")
	t.Setenv("UNHEADED_TLS_CA_CERT", caFile)
	t.Setenv("UNHEADED_TLS_CERT", "")
	t.Setenv("UNHEADED_TLS_KEY_FILE", "")
	url := strings.Replace(srv.URL, "http://", "https://", 1) + "/health"
	if err := HTTPProbe(url)(context.Background()); err != nil {
		t.Fatalf("probe over TLS with the deployment CA: %v", err)
	}

	// Without the CA in the environment, the same server is untrusted.
	t.Setenv("UNHEADED_TLS_ENABLED", "")
	if err := HTTPProbe(url)(context.Background()); err == nil {
		t.Fatal("a server signed by the dev CA was trusted without it")
	}
}
