// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package tls_test

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pqtls "unheaded/pkg/tls"
)

// S-IRON-GATE Phase 8 (FND-017): a service with UNHEADED_TLS_ENABLED=true
// serves TLS 1.3 with the Strict hybrid profile and never falls back to
// plaintext; its outgoing clients trust the deployment CA.

// devPKI writes a CA and a "svc" certificate into a temp dir and points the
// UNHEADED_TLS_* variables at them (mTLS off: identity is the service token).
func devPKI(t *testing.T) (caPEM []byte) {
	t.Helper()
	dir := t.TempDir()
	ca, err := pqtls.NewCA()
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}
	sc, err := ca.IssueCert(pqtls.CertRequest{ServiceName: "svc", TTL: time.Hour})
	if err != nil {
		t.Fatalf("IssueCert: %v", err)
	}
	write := func(name string, b []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, b, 0o600); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
		return p
	}
	t.Setenv("UNHEADED_TLS_ENABLED", "true")
	t.Setenv("UNHEADED_TLS_CA_CERT", write("ca.pem", ca.CertPEM()))
	t.Setenv("UNHEADED_TLS_CERT", write("svc.pem", sc.CertPEM))
	t.Setenv("UNHEADED_TLS_KEY_FILE", write("svc-key.pem", sc.KeyPEM))
	t.Setenv("UNHEADED_TLS_MTLS_ENABLED", "false")
	t.Setenv("UNHEADED_TLS_PROFILE", "")
	return ca.CertPEM()
}

// serve starts MaybeWrapServer's server on an ephemeral loopback port and
// returns its address.
func serve(t *testing.T) string {
	t.Helper()
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok") })
	srv, on, err := pqtls.MaybeWrapServer("127.0.0.1:0", h)
	if err != nil {
		t.Fatalf("MaybeWrapServer: %v", err)
	}
	if !on {
		t.Fatal("MaybeWrapServer: TLS not on with UNHEADED_TLS_ENABLED=true")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = pqtls.Serve(srv, ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return ln.Addr().String()
}

func dialWith(addr string, caPEM []byte, mut func(*tls.Config)) (tls.ConnectionState, error) {
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caPEM)
	cfg := &tls.Config{RootCAs: pool, ServerName: "localhost", MinVersion: tls.VersionTLS12}
	if mut != nil {
		mut(cfg)
	}
	d := &net.Dialer{Timeout: 5 * time.Second}
	c, err := tls.DialWithDialer(d, "tcp", addr, cfg)
	if err != nil {
		return tls.ConnectionState{}, err
	}
	defer func() { _ = c.Close() }()
	return c.ConnectionState(), nil
}

func TestStrictServer_HybridClientNegotiatesTLS13MLKEM(t *testing.T) {
	ca := devPKI(t)
	addr := serve(t)
	cs, err := dialWith(addr, ca, nil)
	if err != nil {
		t.Fatalf("hybrid client refused: %v", err)
	}
	if cs.Version != tls.VersionTLS13 {
		t.Errorf("version = %x, want TLS 1.3", cs.Version)
	}
	if !pqtls.IsHybrid(cs.CurveID) {
		t.Errorf("group = %v, want a hybrid ML-KEM group", cs.CurveID)
	}
}

func TestStrictServer_ClassicalOnlyClientRefused(t *testing.T) {
	ca := devPKI(t)
	addr := serve(t)
	if cs, err := dialWith(addr, ca, func(c *tls.Config) { c.CurvePreferences = []tls.CurveID{tls.X25519} }); err == nil {
		t.Fatalf("an X25519-only client negotiated %v with a Strict listener", cs.CurveID)
	}
}

func TestStrictServer_TLS12ClientRefused(t *testing.T) {
	ca := devPKI(t)
	addr := serve(t)
	if _, err := dialWith(addr, ca, func(c *tls.Config) { c.MaxVersion = tls.VersionTLS12 }); err == nil {
		t.Fatal("a TLS 1.2 client was served")
	}
}

func TestStrictServer_PreferredProfileOptIn(t *testing.T) {
	ca := devPKI(t)
	t.Setenv("UNHEADED_TLS_PROFILE", "preferred")
	addr := serve(t)
	cs, err := dialWith(addr, ca, func(c *tls.Config) { c.CurvePreferences = []tls.CurveID{tls.X25519} })
	if err != nil {
		t.Fatalf("UNHEADED_TLS_PROFILE=preferred refused a classical client: %v", err)
	}
	if cs.Version != tls.VersionTLS13 {
		t.Errorf("version = %x, want TLS 1.3", cs.Version)
	}
}

func TestStrictServer_UnknownProfileIsAnError(t *testing.T) {
	devPKI(t)
	t.Setenv("UNHEADED_TLS_PROFILE", "classical")
	if _, _, err := pqtls.MaybeWrapServer("127.0.0.1:0", http.NotFoundHandler()); err == nil {
		t.Fatal("an unknown UNHEADED_TLS_PROFILE was accepted")
	}
}

// No silent plaintext fallback: TLS asked for but not configured is a
// startup error, never a plain listener.
func TestNoSilentFallback_MissingCertFile(t *testing.T) {
	devPKI(t)
	t.Setenv("UNHEADED_TLS_CERT", filepath.Join(t.TempDir(), "absent.pem"))
	srv, on, err := pqtls.MaybeWrapServer("127.0.0.1:0", http.NotFoundHandler())
	if err == nil {
		t.Fatalf("missing cert: no error (tls=%v, srv=%v)", on, srv != nil)
	}
	if srv != nil || on {
		t.Errorf("missing cert: got a server (tls=%v); want none", on)
	}
}

func TestNoSilentFallback_UnsetPaths(t *testing.T) {
	for _, v := range []string{"UNHEADED_TLS_CERT", "UNHEADED_TLS_KEY_FILE"} {
		t.Run(v, func(t *testing.T) {
			devPKI(t)
			t.Setenv(v, "")
			_, _, err := pqtls.MaybeWrapServer("127.0.0.1:0", http.NotFoundHandler())
			if err == nil || !strings.Contains(err.Error(), v) {
				t.Fatalf("err = %v, want one naming %s", err, v)
			}
		})
	}
}

func TestNoSilentFallback_MTLSWithoutCA(t *testing.T) {
	devPKI(t)
	t.Setenv("UNHEADED_TLS_MTLS_ENABLED", "true")
	t.Setenv("UNHEADED_TLS_CA_CERT", "")
	if _, _, err := pqtls.MaybeWrapServer("127.0.0.1:0", http.NotFoundHandler()); err == nil {
		t.Fatal("mTLS without a CA started")
	}
}

// ConfigureServer is MaybeWrapServer for a server a service already built.
func TestConfigureServer_InstallsStrictTLS(t *testing.T) {
	devPKI(t)
	srv := &http.Server{Addr: ":0", ReadHeaderTimeout: time.Second}
	on, err := pqtls.ConfigureServer(srv)
	if err != nil || !on {
		t.Fatalf("ConfigureServer = %v, %v", on, err)
	}
	if srv.TLSConfig == nil || srv.TLSConfig.MinVersion != tls.VersionTLS13 {
		t.Fatalf("TLSConfig = %+v, want TLS 1.3", srv.TLSConfig)
	}
	for _, g := range srv.TLSConfig.CurvePreferences {
		if !pqtls.IsHybrid(g) {
			t.Errorf("Strict listener offers classical group %v", g)
		}
	}
	if len(srv.TLSConfig.CurvePreferences) == 0 {
		t.Error("CurvePreferences empty: that is Go's default list, which includes X25519")
	}
}

func TestConfigureServer_DisabledLeavesPlain(t *testing.T) {
	t.Setenv("UNHEADED_TLS_ENABLED", "")
	srv := &http.Server{Addr: "127.0.0.1:0", ReadHeaderTimeout: time.Second}
	on, err := pqtls.ConfigureServer(srv)
	if err != nil || on || srv.TLSConfig != nil {
		t.Fatalf("ConfigureServer with TLS off = %v, %v, TLSConfig=%v", on, err, srv.TLSConfig)
	}
}

func TestIsLoopbackListenAddr(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1:80": true, "localhost:1": true, "[::1]:9": true,
		":8080": false, "0.0.0.0:1": false, "10.10.10.2:5": false, "[::]:1": false,
	}
	for addr, want := range cases {
		if got := pqtls.IsLoopbackListenAddr(addr); got != want {
			t.Errorf("IsLoopbackListenAddr(%q) = %v, want %v", addr, got, want)
		}
	}
}

// ClientFromEnv is every service's outgoing client for internal links.
func TestClientFromEnv_TrustsDeploymentCA(t *testing.T) {
	devPKI(t)
	addr := serve(t)
	c, err := pqtls.ClientFromEnv(5 * time.Second)
	if err != nil {
		t.Fatalf("ClientFromEnv: %v", err)
	}
	_, port, _ := net.SplitHostPort(addr)
	resp, err := c.Get("https://localhost:" + port + "/")
	if err != nil {
		t.Fatalf("GET over TLS: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if resp.TLS == nil || !pqtls.IsHybrid(resp.TLS.CurveID) {
		t.Errorf("client session not hybrid: %+v", resp.TLS)
	}
}

func TestClientFromEnv_RefusesUnknownCA(t *testing.T) {
	devPKI(t)
	addr := serve(t)
	// A second deployment's CA: the client must not trust the first's server.
	devPKI(t)
	c, err := pqtls.ClientFromEnv(5 * time.Second)
	if err != nil {
		t.Fatalf("ClientFromEnv: %v", err)
	}
	_, port, _ := net.SplitHostPort(addr)
	if resp, err := c.Get("https://localhost:" + port + "/"); err == nil {
		_ = resp.Body.Close()
		t.Fatal("a server signed by another CA was trusted")
	}
}

func TestClientFromEnv_MissingCAIsAnError(t *testing.T) {
	devPKI(t)
	t.Setenv("UNHEADED_TLS_CA_CERT", "")
	if _, err := pqtls.ClientFromEnv(time.Second); err == nil {
		t.Fatal("TLS on without a CA: no error")
	}
}

func TestClientFromEnv_DisabledIsPlain(t *testing.T) {
	t.Setenv("UNHEADED_TLS_ENABLED", "")
	c, err := pqtls.ClientFromEnv(time.Second)
	if err != nil || c == nil {
		t.Fatalf("ClientFromEnv with TLS off = %v, %v", c, err)
	}
	if c == http.DefaultClient {
		t.Error("returned http.DefaultClient (shared, no timeout)")
	}
}

func TestLoadCA_RoundTripAndMismatch(t *testing.T) {
	a, err := pqtls.NewCA()
	if err != nil {
		t.Fatal(err)
	}
	b, err := pqtls.NewCA()
	if err != nil {
		t.Fatal(err)
	}
	re, err := pqtls.LoadCA(a.CertPEM(), a.KeyPEM())
	if err != nil {
		t.Fatalf("LoadCA: %v", err)
	}
	sc, err := re.IssueCert(pqtls.CertRequest{ServiceName: "svc"})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.VerifyCert(sc.CertPEM); err != nil {
		t.Fatalf("a cert from the reloaded CA does not verify against the original: %v", err)
	}
	if _, err := pqtls.LoadCA(a.CertPEM(), b.KeyPEM()); err == nil {
		t.Fatal("a mismatched key was accepted")
	}
	if _, err := pqtls.LoadCA(sc.CertPEM, sc.KeyPEM); err == nil {
		t.Fatal("a leaf certificate was accepted as a CA")
	}
}

func TestEndpointURL(t *testing.T) {
	t.Setenv("UNHEADED_TLS_ENABLED", "true")
	cases := []struct {
		in    string
		first bool
		want  string
	}{
		{"timeguru:19000", true, "https://timeguru:19000"},
		{"172.28.1.1:8081", false, "http://172.28.1.1:8081"},
		{"http://legacy:1/", true, "http://legacy:1"},
		{"https://x:2", false, "https://x:2"},
	}
	for _, c := range cases {
		if got := pqtls.EndpointURL(c.in, c.first); got != c.want {
			t.Errorf("EndpointURL(%q, %v) = %q, want %q", c.in, c.first, got, c.want)
		}
	}
	t.Setenv("UNHEADED_TLS_ENABLED", "")
	if got := pqtls.EndpointURL("timeguru:19000", true); got != "http://timeguru:19000" {
		t.Errorf("TLS off: %q", got)
	}
}

// An OIDC issuer may be internal or public: system roots plus the CA.
func TestTransportTrustingDeploymentCA(t *testing.T) {
	devPKI(t)
	addr := serve(t)
	tr, err := pqtls.TransportTrustingDeploymentCA()
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(addr)
	resp, err := (&http.Client{Transport: tr, Timeout: 5 * time.Second}).Get("https://localhost:" + port + "/")
	if err != nil {
		t.Fatalf("GET with system roots + deployment CA: %v", err)
	}
	_ = resp.Body.Close()

	t.Setenv("UNHEADED_TLS_CA_CERT", "")
	if _, err := pqtls.TransportTrustingDeploymentCA(); err == nil {
		t.Fatal("TLS on without a CA: no error")
	}
	t.Setenv("UNHEADED_TLS_ENABLED", "")
	if tr, err := pqtls.TransportTrustingDeploymentCA(); err != nil || (tr.TLSClientConfig != nil && tr.TLSClientConfig.RootCAs != nil) {
		t.Fatalf("TLS off: err %v; want the default transport (system roots)", err)
	}
}

func TestSchemeAndBaseURL(t *testing.T) {
	t.Setenv("UNHEADED_TLS_ENABLED", "")
	if got := pqtls.BaseURL("wotan:18000"); got != "http://wotan:18000" {
		t.Errorf("TLS off: %q", got)
	}
	t.Setenv("UNHEADED_TLS_ENABLED", "true")
	if pqtls.Scheme() != "https" {
		t.Errorf("Scheme() = %q", pqtls.Scheme())
	}
	for in, want := range map[string]string{
		"wotan:18000":          "https://wotan:18000",
		"http://victoria:8428": "http://victoria:8428",
		"https://auth:19020/":  "https://auth:19020",
		"  timeguru:19000  ":   "https://timeguru:19000",
	} {
		if got := pqtls.BaseURL(in); got != want {
			t.Errorf("BaseURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPlaintextGRPCCredentials_RefusedUnderTLS(t *testing.T) {
	t.Setenv("UNHEADED_TLS_ENABLED", "true")
	if _, err := pqtls.PlaintextGRPCCredentials("test"); err == nil {
		t.Fatal("plaintext gRPC credentials handed out with UNHEADED_TLS_ENABLED=true")
	}
	t.Setenv("UNHEADED_TLS_ENABLED", "")
	creds, err := pqtls.PlaintextGRPCCredentials("test")
	if err != nil || creds == nil || creds.Info().SecurityProtocol != "insecure" {
		t.Fatalf("PlaintextGRPCCredentials = %v, %v", creds, err)
	}
}
