// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package wotanClient

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/auth/authtest"
	pqtls "unheaded/pkg/tls"
)

// WOTAN_TLS_SERVER_NAME: a client that reaches Wotan at an address its
// certificate does not name (alert-mailer, from the monitoring network,
// over the host's overlay address) verifies the certificate against the
// name it does carry. Verification is never off: the chain must still end
// at the deployment CA and the certificate must carry that name.

// testCA is a CA made with the standard library, so a test can issue a
// certificate with exactly the names it wants (pkg/tls.IssueCert always
// adds localhost and the loopback addresses).
type testCA struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	pem  []byte
}

func newTestCA(t *testing.T) *testCA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "server-name test CA"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return &testCA{cert: cert, key: key, pem: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}
}

// serverConfig is a Strict TLS 1.3 server config whose certificate carries
// the names given (a host name, or an IP address as an IP SAN) and nothing
// else: no localhost, no loopback address.
func (ca *testCA) serverConfig(t *testing.T, names ...string) *tls.Config {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: names[0]},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	for _, n := range names {
		if ip := net.ParseIP(n); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, n)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		t.Fatal(err)
	}
	return pqtls.ApplyStrict(&tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
	})
}

// trust makes ca the deployment CA of this test's clients, TLS on.
func (ca *testCA) trust(t *testing.T) {
	t.Helper()
	caFile := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caFile, ca.pem, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UNHEADED_TLS_ENABLED", "true")
	t.Setenv("UNHEADED_TLS_CA_CERT", caFile)
	t.Setenv("UNHEADED_TLS_CERT", "")
	t.Setenv("UNHEADED_TLS_KEY_FILE", "")
	t.Setenv("UNHEADED_GRPC_TLS", "")
	t.Setenv(EnvTLSServerName, "")
}

// grpcPublish dials addr with the environment's TLS and publishes once.
func grpcPublish(t *testing.T, addr string, opts ...ClientOption) error {
	t.Helper()
	gc, err := NewGRPCClient(addr, opts...)
	if err != nil {
		return err
	}
	defer gc.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := gc.Subscribe(ctx, "t.a", "alert-mailer"); err != nil {
		return err
	}
	return gc.Publish(ctx, "t.a", []byte("x"))
}

func TestTLSServerName_GRPCVerifiesAgainstTheName(t *testing.T) {
	ca := newTestCA(t)
	ca.trust(t)
	f := authtest.New(t, "wotan", "alert-mailer")
	v := verifier(t, f, "alert-mailer")
	rec := &authRecorder{}
	// The certificate names only "wotan"; the client dials 127.0.0.1.
	addr := startTLSGRPC(t, "127.0.0.1", ca.serverConfig(t, "wotan"), rec)

	if err := grpcPublish(t, addr); err == nil {
		t.Fatal("no server name set: a certificate that does not name the dialled address was accepted")
	}
	if got := rec.all(); len(got) != 0 {
		t.Fatalf("the server saw %d call(s) from a client that must refuse its certificate", len(got))
	}

	t.Setenv(EnvTLSServerName, "wotan")
	if err := grpcPublish(t, addr, WithServiceToken(f.Minter("alert-mailer"))); err != nil {
		t.Fatalf("%s=wotan: publish: %v", EnvTLSServerName, err)
	}
	got := rec.all()
	if len(got) != 1 || subjectOf(t, v, got[0]) != "alert-mailer" {
		t.Fatalf("authorization over TLS = %q, want one token of alert-mailer", got)
	}
}

func TestTLSServerName_TopicStreamClient(t *testing.T) {
	ca := newTestCA(t)
	ca.trust(t)
	rec := &authRecorder{}
	addr := startTLSGRPC(t, "127.0.0.1", ca.serverConfig(t, "wotan"), rec)
	t.Setenv(EnvTLSServerName, "wotan")

	c, err := NewTopicStreamClient(addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := c.Subscribe(ctx, "t.a", "x"); err != nil {
		t.Fatal(err)
	}
	if err := c.Publish(ctx, "t.a", []byte("x")); err != nil {
		t.Fatalf("publish: %v", err)
	}
}

// The name may be an address the certificate carries as an IP SAN (the dev
// certificate carries the bridge address), IPv4 or IPv6, on both sides.
func TestTLSServerName_IPAddress(t *testing.T) {
	for _, ip := range []string{"10.10.10.10", "fd00:dead:beef::1"} {
		t.Run(ip, func(t *testing.T) {
			ca := newTestCA(t)
			ca.trust(t)
			rec := &authRecorder{}
			cfg := ca.serverConfig(t, ip)
			grpcAddr := startTLSGRPC(t, "127.0.0.1", cfg, rec)
			httpAddr := startTLSHTTP(t, cfg, rec)

			if err := grpcPublish(t, grpcAddr); err == nil {
				t.Fatal("gRPC, no server name set: accepted")
			}
			if err := httpJoin(t, httpAddr); err == nil {
				t.Fatal("HTTP, no server name set: accepted")
			}
			t.Setenv(EnvTLSServerName, ip)
			if err := grpcPublish(t, grpcAddr); err != nil {
				t.Fatalf("gRPC with %s=%s: %v", EnvTLSServerName, ip, err)
			}
			if err := httpJoin(t, httpAddr); err != nil {
				t.Fatalf("HTTP with %s=%s: %v", EnvTLSServerName, ip, err)
			}
			t.Setenv(EnvTLSServerName, "10.10.10.11")
			if err := grpcPublish(t, grpcAddr); err == nil {
				t.Fatal("gRPC: an address the certificate does not carry was accepted")
			}
			if err := httpJoin(t, httpAddr); err == nil {
				t.Fatal("HTTP: an address the certificate does not carry was accepted")
			}
		})
	}
}

// The name replaces the dialled host in verification; it does not relax it.
func TestTLSServerName_GRPCStillVerifies(t *testing.T) {
	t.Run("a name the certificate does not carry", func(t *testing.T) {
		ca := newTestCA(t)
		ca.trust(t)
		rec := &authRecorder{}
		addr := startTLSGRPC(t, "127.0.0.1", ca.serverConfig(t, "wotan"), rec)
		t.Setenv(EnvTLSServerName, "odin")
		if err := grpcPublish(t, addr); err == nil {
			t.Fatal("a certificate for wotan was accepted as odin")
		}
		if len(rec.all()) != 0 {
			t.Fatal("the server saw a call")
		}
	})
	t.Run("the right name from another CA", func(t *testing.T) {
		newTestCA(t).trust(t) // the client's deployment CA
		rec := &authRecorder{}
		addr := startTLSGRPC(t, "127.0.0.1", newTestCA(t).serverConfig(t, "wotan"), rec)
		t.Setenv(EnvTLSServerName, "wotan")
		if err := grpcPublish(t, addr); err == nil {
			t.Fatal("a certificate from a CA the client does not trust was accepted")
		}
		if len(rec.all()) != 0 {
			t.Fatal("the server saw a call")
		}
	})
	t.Run("the dialled address is no longer enough", func(t *testing.T) {
		// A certificate that names the dialled address but not the
		// configured name is refused: the setting is the one name checked.
		ca := newTestCA(t)
		ca.trust(t)
		rec := &authRecorder{}
		addr := startTLSGRPC(t, "127.0.0.1", ca.serverConfig(t, "localhost"), rec)
		_, port, _ := net.SplitHostPort(addr)
		if err := grpcPublish(t, "localhost:"+port); err != nil {
			t.Fatalf("no server name set, dialled by the certificate's name: %v", err)
		}
		t.Setenv(EnvTLSServerName, "wotan")
		if err := grpcPublish(t, "localhost:"+port); err == nil {
			t.Fatal("a certificate for localhost was accepted as wotan")
		}
	})
}

// httpJoin builds a control-plane client for addr and joins a topic.
func httpJoin(t *testing.T, addr string, opts ...ClientOption) error {
	t.Helper()
	c, err := NewClient(addr, opts...)
	if err != nil {
		return err
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err = c.Subscribe(ctx, "t.a", "alert-mailer")
	return err
}

// startTLSHTTP serves the fake REST API over TLS with cfg on 127.0.0.1.
func startTLSHTTP(t *testing.T, cfg *tls.Config, rec *authRecorder) string {
	t.Helper()
	srv := httptest.NewUnstartedServer(fakeHTTPWotan(t, rec).Config.Handler)
	srv.TLS = cfg
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv.Listener.Addr().String()
}

func TestTLSServerName_HTTPVerifiesAgainstTheName(t *testing.T) {
	ca := newTestCA(t)
	ca.trust(t)
	f := authtest.New(t, "wotan", "alert-mailer")
	v := verifier(t, f, "alert-mailer")
	rec := &authRecorder{}
	addr := startTLSHTTP(t, ca.serverConfig(t, "wotan"), rec)

	if err := httpJoin(t, addr); err == nil {
		t.Fatal("no server name set: a certificate that does not name the dialled address was accepted")
	}
	if got := rec.all(); len(got) != 0 {
		t.Fatalf("the server saw %d request(s) from a client that must refuse its certificate", len(got))
	}

	t.Setenv(EnvTLSServerName, "wotan")
	// The address as alert-mailer's compose entry writes it: a URL.
	if err := httpJoin(t, "https://"+addr, WithServiceToken(f.Minter("alert-mailer"))); err != nil {
		t.Fatalf("%s=wotan: join: %v", EnvTLSServerName, err)
	}
	got := rec.all()
	if len(got) != 1 || subjectOf(t, v, got[0]) != "alert-mailer" {
		t.Fatalf("authorization over https = %q, want one token of alert-mailer", got)
	}
}

func TestTLSServerName_HTTPStillVerifies(t *testing.T) {
	t.Run("a name the certificate does not carry", func(t *testing.T) {
		ca := newTestCA(t)
		ca.trust(t)
		rec := &authRecorder{}
		addr := startTLSHTTP(t, ca.serverConfig(t, "wotan"), rec)
		t.Setenv(EnvTLSServerName, "odin")
		if err := httpJoin(t, addr); err == nil {
			t.Fatal("a certificate for wotan was accepted as odin")
		}
		if len(rec.all()) != 0 {
			t.Fatal("the server saw a request")
		}
	})
	t.Run("the right name from another CA", func(t *testing.T) {
		newTestCA(t).trust(t)
		rec := &authRecorder{}
		addr := startTLSHTTP(t, newTestCA(t).serverConfig(t, "wotan"), rec)
		t.Setenv(EnvTLSServerName, "wotan")
		if err := httpJoin(t, addr); err == nil {
			t.Fatal("a certificate from a CA the client does not trust was accepted")
		}
		if len(rec.all()) != 0 {
			t.Fatal("the server saw a request")
		}
	})
}

// A value that cannot be a certificate name stops the constructor: it is
// never dropped in favour of the dialled host, and never sent as written.
func TestTLSServerName_MalformedIsAConstructorError(t *testing.T) {
	newTestCA(t).trust(t)
	for _, bad := range []string{
		"wotan:18001", "https://wotan", "wotan/", "wo tan", "[fd00:dead:beef::1]", "*.wotan", "-wotan", "wotan.", "a..b", strings.Repeat("a", 254),
	} {
		t.Setenv(EnvTLSServerName, bad)
		if _, err := NewGRPCClient("127.0.0.1:18001"); err == nil || !strings.Contains(err.Error(), EnvTLSServerName) {
			t.Errorf("NewGRPCClient with %s=%q: err = %v, want an error naming the variable", EnvTLSServerName, bad, err)
		}
		if _, err := NewClient("127.0.0.1:18000"); err == nil || !strings.Contains(err.Error(), EnvTLSServerName) {
			t.Errorf("NewClient with %s=%q: err = %v, want an error naming the variable", EnvTLSServerName, bad, err)
		}
		if _, err := NewClientWithGRPC("127.0.0.1:18000", "127.0.0.1:18001"); err == nil || !strings.Contains(err.Error(), EnvTLSServerName) {
			t.Errorf("NewClientWithGRPC with %s=%q: err = %v, want an error naming the variable", EnvTLSServerName, bad, err)
		}
		if _, err := NewTopicStreamClient("127.0.0.1:18001"); err == nil || !strings.Contains(err.Error(), EnvTLSServerName) {
			t.Errorf("NewTopicStreamClient with %s=%q: err = %v, want an error naming the variable", EnvTLSServerName, bad, err)
		}
	}
}

func TestTLSServerName_Accepted(t *testing.T) {
	for raw, want := range map[string]string{
		"":                  "",
		"  wotan ":          "wotan",
		"wotan":             "wotan",
		"Wotan.Example.net": "Wotan.Example.net",
		"wotan-1.internal":  "wotan-1.internal",
		"10.10.10.10":       "10.10.10.10",
		"fd00:dead:beef::1": "fd00:dead:beef::1",
	} {
		t.Setenv(EnvTLSServerName, raw)
		got, err := tlsServerName()
		if err != nil || got != want {
			t.Errorf("%s=%q: got %q, %v; want %q", EnvTLSServerName, raw, got, err, want)
		}
	}
}

// The older gRPC-only switch (system roots) honours the name too, and a
// plaintext client has nothing to verify: the name changes nothing there.
func TestTLSServerName_OtherTransportModes(t *testing.T) {
	t.Setenv("UNHEADED_TLS_ENABLED", "")
	t.Setenv("UNHEADED_TLS_CA_CERT", "")
	t.Setenv(EnvTLSServerName, "wotan")

	t.Setenv("UNHEADED_GRPC_TLS", "1")
	creds, err := grpcCredentials(nil)
	if err != nil {
		t.Fatal(err)
	}
	if isPlaintext(creds) || len(grpcServerNameOption(creds)) != 1 {
		t.Fatalf("UNHEADED_GRPC_TLS=1: plaintext=%v, %d server name option(s); want TLS and one", isPlaintext(creds), len(grpcServerNameOption(creds)))
	}

	t.Setenv("UNHEADED_GRPC_TLS", "")
	creds, err = grpcCredentials(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !isPlaintext(creds) {
		t.Fatal("TLS off: the server name turned TLS on")
	}
	if n := len(grpcServerNameOption(creds)); n != 0 {
		t.Fatalf("TLS off: %d server name option(s) on a plaintext link, want none", n)
	}
	base, tr, err := httpBase("wotan:18000", nil)
	if err != nil || base != "http://wotan:18000/api/v1" || tr != nil {
		t.Fatalf("TLS off: httpBase = %q, %v, %v; want plain http and the default transport", base, tr, err)
	}
}

// An https:// address without UNHEADED_TLS_ENABLED verifies against the
// system roots; with a server name it still does, for that name.
func TestTLSServerName_HTTPSWithSystemRoots(t *testing.T) {
	t.Setenv("UNHEADED_TLS_ENABLED", "")
	t.Setenv("UNHEADED_TLS_CA_CERT", "")
	t.Setenv(EnvTLSServerName, "wotan")
	ca := newTestCA(t)
	rec := &authRecorder{}
	addr := startTLSHTTP(t, ca.serverConfig(t, "wotan"), rec)
	if err := httpJoin(t, "https://"+addr); err == nil {
		t.Fatal("a certificate from a CA outside the system roots was accepted")
	}
	if len(rec.all()) != 0 {
		t.Fatal("the server saw a request")
	}
}
