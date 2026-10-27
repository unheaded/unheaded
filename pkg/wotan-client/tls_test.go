// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package wotanClient

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"unheaded/pkg/auth/authtest"
	pqtls "unheaded/pkg/tls"
	chatpb "unheaded/services/wotan/proto"
)

// S-IRON-GATE Phase 8: with UNHEADED_TLS_ENABLED=true every Wotan client
// dials TLS (Strict, the deployment CA, server name verified), and a
// service token travels only over TLS.

// tlsEnv issues a CA and a "wotan" certificate, sets the client env to trust
// that CA, and returns the server-side TLS config.
func tlsEnv(t *testing.T) *tls.Config {
	t.Helper()
	ca, err := pqtls.NewCA()
	if err != nil {
		t.Fatal(err)
	}
	sc, err := ca.IssueCert(pqtls.CertRequest{ServiceName: "wotan", TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	caFile := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(caFile, ca.CertPEM(), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UNHEADED_TLS_ENABLED", "true")
	t.Setenv("UNHEADED_TLS_CA_CERT", caFile)
	t.Setenv("UNHEADED_TLS_CERT", "")
	t.Setenv("UNHEADED_TLS_KEY_FILE", "")
	t.Setenv("UNHEADED_GRPC_TLS", "")
	srvCfg, err := pqtls.NewServerTLSConfig(pqtls.ServerConfig{CertPEM: sc.CertPEM, KeyPEM: sc.KeyPEM})
	if err != nil {
		t.Fatal(err)
	}
	return pqtls.ApplyStrict(srvCfg)
}

// startTLSGRPC serves the fake topic server over TLS on host (a loopback IP).
func startTLSGRPC(t *testing.T, host string, cfg *tls.Config, rec *authRecorder) string {
	t.Helper()
	lis, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
	if err != nil {
		t.Skipf("listen on %s: %v", host, err)
	}
	s := grpc.NewServer(grpc.Creds(credentials.NewTLS(cfg)))
	chatpb.RegisterTopicStreamServer(s, &fakeTopicServer{rec: rec})
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(s.Stop)
	return lis.Addr().String()
}

func TestWithTLS_FromEnvGRPCClientDialsTLS(t *testing.T) {
	cfg := tlsEnv(t)
	f := authtest.New(t, "wotan", "sophia")
	v := verifier(t, f, "sophia")
	rec := &authRecorder{}
	addr := startTLSGRPC(t, "127.0.0.1", cfg, rec)

	gc, err := NewGRPCClient(addr, WithServiceToken(f.Minter("sophia")))
	if err != nil {
		t.Fatal(err)
	}
	defer gc.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := gc.Subscribe(ctx, "t.a", "sophia"); err != nil {
		t.Fatal(err)
	}
	if err := gc.Publish(ctx, "t.a", []byte("x")); err != nil {
		t.Fatalf("publish over TLS: %v", err)
	}
	got := rec.all()
	if len(got) != 1 || subjectOf(t, v, got[0]) != "sophia" {
		t.Fatalf("authorization over TLS = %q", got)
	}
}

func TestWithTLS_TopicStreamFromEnvDialsTLS(t *testing.T) {
	cfg := tlsEnv(t)
	rec := &authRecorder{}
	addr := startTLSGRPC(t, "127.0.0.1", cfg, rec)
	c, err := NewTopicStreamClient(addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := c.Subscribe(ctx, "t.a", "x"); err != nil {
		t.Fatal(err)
	}
	if err := c.Publish(ctx, "t.a", []byte("x")); err != nil {
		t.Fatalf("publish over TLS: %v", err)
	}
}

// The server name is verified: a certificate that does not name the
// dialled address is refused (127.0.0.2 is loopback on Linux but not in
// the certificate's SANs).
func TestWithTLS_VerifiesServerName(t *testing.T) {
	cfg := tlsEnv(t)
	rec := &authRecorder{}
	addr := startTLSGRPC(t, "127.0.0.2", cfg, rec)
	gc, err := NewGRPCClient(addr)
	if err != nil {
		t.Fatal(err)
	}
	defer gc.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := gc.Publish(ctx, "t.a", []byte("x")); err == nil {
		t.Fatal("a certificate without the dialled name was accepted")
	}
}

// A plaintext server is not silently accepted by a client told to use TLS.
func TestWithTLS_NoPlaintextFallback(t *testing.T) {
	tlsEnv(t)
	rec := &authRecorder{}
	addr := startFakeGRPC(t, rec) // plaintext
	gc, err := NewGRPCClient(addr)
	if err != nil {
		t.Fatal(err)
	}
	defer gc.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := gc.Publish(ctx, "t.a", []byte("x")); err == nil {
		t.Fatal("a TLS client published to a plaintext server")
	}
	if len(rec.all()) != 0 {
		t.Fatal("the plaintext server saw a call")
	}
}

func TestWithTLS_HTTPControlPlaneUsesHTTPS(t *testing.T) {
	cfg := tlsEnv(t)
	f := authtest.New(t, "wotan", "captain")
	v := verifier(t, f, "captain")
	rec := &authRecorder{}
	plain := fakeHTTPWotan(t, rec)
	srv := httptest.NewUnstartedServer(plain.Config.Handler)
	srv.TLS = cfg
	srv.StartTLS()
	t.Cleanup(srv.Close)
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())

	c, err := NewClient("localhost:"+port, WithServiceToken(f.Minter("captain")))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(c.baseURL, "https://") {
		t.Fatalf("baseURL = %q, want https", c.baseURL)
	}
	defer c.Close()
	if _, err := c.Subscribe(context.Background(), "t.a", "captain"); err != nil {
		t.Fatalf("subscribe over https: %v", err)
	}
	if err := c.Publish(context.Background(), "t.a", []byte(`{}`)); err != nil {
		t.Fatalf("publish over https: %v", err)
	}
	got := rec.all()
	if len(got) == 0 || subjectOf(t, v, got[len(got)-1]) != "captain" {
		t.Fatalf("authorization over https = %q", got)
	}
}

// Under TLS an http:// Wotan address (many callers build "http://"+addr) is
// upgraded, never used in the clear.
func TestWithTLS_ExplicitHTTPAddressUpgraded(t *testing.T) {
	tlsEnv(t)
	c, err := NewClient("http://wotan:18000")
	if err != nil {
		t.Fatal(err)
	}
	if c.baseURL != "https://wotan:18000/api/v1" {
		t.Fatalf("baseURL = %q, want the https upgrade", c.baseURL)
	}
}

func TestWithInsecureTransport_RefusedUnderTLS(t *testing.T) {
	tlsEnv(t)
	if _, err := NewGRPCClient("wotan:18001", WithInsecureTransport()); err == nil {
		t.Fatal("NewGRPCClient: WithInsecureTransport accepted with UNHEADED_TLS_ENABLED=true")
	}
	if _, err := NewClient("wotan:18000", WithInsecureTransport()); err == nil {
		t.Fatal("NewClient: WithInsecureTransport accepted with UNHEADED_TLS_ENABLED=true")
	}
	if _, err := NewClientWithGRPC("wotan:18000", "wotan:18001", WithInsecureTransport()); err == nil {
		t.Fatal("NewClientWithGRPC: WithInsecureTransport accepted with UNHEADED_TLS_ENABLED=true")
	}
}

// Tokens only over TLS: a plaintext link to another host gets no token;
// loopback and an explicit WithInsecureTransport (tests) still do.
func TestTokensOnlyOverTLS_GRPCMetadata(t *testing.T) {
	f := authtest.New(t, "wotan", "monad")
	m := f.Minter("monad")
	cases := []struct {
		name string
		ta   *tokenAuth
		want bool
	}{
		{"tls", &tokenAuth{minter: m, target: "wotan:18001"}, true},
		{"plaintext remote", &tokenAuth{minter: m, plaintext: true, target: "wotan:18001"}, false},
		{"plaintext loopback", &tokenAuth{minter: m, plaintext: true, target: "127.0.0.1:18001"}, true},
		{"plaintext localhost", &tokenAuth{minter: m, plaintext: true, target: "localhost:18001"}, true},
		{"plaintext opt-in", &tokenAuth{minter: m, plaintext: true, quiet: true, target: "wotan:18001"}, true},
	}
	for _, tc := range cases {
		md, err := tc.ta.GetRequestMetadata(context.Background())
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got := md["authorization"] != ""; got != tc.want {
			t.Errorf("%s: token sent = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestTokensOnlyOverTLS_HTTPHeader(t *testing.T) {
	f := authtest.New(t, "wotan", "monad")
	m := f.Minter("monad")
	for raw, want := range map[string]bool{
		"https://wotan:18000/api/v1/x":   true,
		"http://wotan:18000/api/v1/x":    false,
		"http://127.0.0.1:18000/api/v1":  true,
		"http://localhost:18000/api/v1":  true,
		"http://[::1]:18000/api/v1/x":    true,
		"http://10.10.10.10:18000/api/v": false,
	} {
		req, _ := http.NewRequest(http.MethodGet, raw, nil)
		if err := (&tokenAuth{minter: m}).authorize(req); err != nil {
			t.Fatal(err)
		}
		if got := req.Header.Get("Authorization") != ""; got != want {
			t.Errorf("%s: token sent = %v, want %v", raw, got, want)
		}
	}
}
