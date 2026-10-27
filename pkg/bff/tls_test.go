// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package bff_test

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"unheaded/pkg/bff"
	"unheaded/pkg/bff/bfftest"
	pqtls "unheaded/pkg/tls"
)

// S-IRON-GATE Phase 8: with UNHEADED_TLS_ENABLED=true the BFF reaches an
// https issuer signed by the deployment CA (UNHEADED_TLS_CA_CERT) with no
// AUTH_BFF_DEV_PLAINTEXT, and the session cookie keeps Secure.
func TestTLSIssuer_LoginWithDeploymentCA(t *testing.T) {
	ca, err := pqtls.NewCA()
	if err != nil {
		t.Fatal(err)
	}
	sc, err := ca.IssueCert(pqtls.CertRequest{ServiceName: "auth", TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	caFile := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caFile, ca.CertPEM(), 0o600); err != nil {
		t.Fatal(err)
	}
	srvCfg, err := pqtls.NewServerTLSConfig(pqtls.ServerConfig{CertPEM: sc.CertPEM, KeyPEM: sc.KeyPEM})
	if err != nil {
		t.Fatal(err)
	}
	is := bfftest.NewIssuerTLS(t, pqtls.ApplyStrict(srvCfg), "unheaded", svc)

	t.Setenv("UNHEADED_TLS_ENABLED", "true")
	t.Setenv("UNHEADED_TLS_CA_CERT", caFile)
	t.Setenv("UNHEADED_TLS_CERT", "")
	t.Setenv("UNHEADED_TLS_KEY_FILE", "")
	b, err := bff.New(bff.Config{ServiceName: svc, IssuerURL: is.URL})
	if err != nil {
		t.Fatalf("bff.New over https: %v", err)
	}
	t.Cleanup(b.Close)
	a := &app{t: t, is: is, b: b, h: b.Handler(), clk: &clock{t: time.Now()}}
	pw := is.AddUser(t, "dev", "operator")
	rec := a.do(http.MethodPost, "/auth/login", loginBody("dev", pw), withXRW())
	if rec.Code != http.StatusOK {
		t.Fatalf("login over https: %d %s", rec.Code, rec.Body)
	}
	if c := sessionCookie(t, rec); !c.Secure {
		t.Fatal("session cookie without Secure under TLS")
	}

	// The same issuer without the CA in the environment is untrusted.
	t.Setenv("UNHEADED_TLS_ENABLED", "")
	b2, err := bff.New(bff.Config{ServiceName: svc, IssuerURL: is.URL})
	if err != nil {
		t.Fatalf("bff.New: %v", err)
	}
	t.Cleanup(b2.Close)
	a2 := &app{t: t, is: is, b: b2, h: b2.Handler(), clk: &clock{t: time.Now()}}
	if rec := a2.do(http.MethodPost, "/auth/login", loginBody("dev", pw), withXRW()); rec.Code == http.StatusOK {
		t.Fatal("login succeeded against an issuer whose CA is not trusted")
	}
}

func TestTLS_MissingCAIsAStartupError(t *testing.T) {
	t.Setenv("UNHEADED_TLS_ENABLED", "true")
	t.Setenv("UNHEADED_TLS_CA_CERT", "")
	if _, err := bff.New(bff.Config{ServiceName: svc, IssuerURL: "https://auth:19020"}); err == nil {
		t.Fatal("TLS on without a CA: the BFF started")
	}
}
