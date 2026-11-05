// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	utls "unheaded/pkg/tls"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
}

// serveOnLoopback runs srv and returns its address.
func serveOnLoopback(t *testing.T, srv *http.Server) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- utls.Serve(srv, ln) }()
	t.Cleanup(func() {
		_ = srv.Close()
		if err := <-done; !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("Serve: %v", err)
		}
	})
	return ln.Addr().String()
}

func status(t *testing.T, c *http.Client, url string) (int, error) {
	t.Helper()
	resp, err := c.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

// The shipped unit turns TLS on to dial Wotan and Cuirass and has no
// certificate of its own: that must keep starting, on a plain listener.
func TestNewHTTPServer_ClientOnlyUnitStaysPlain(t *testing.T) {
	t.Setenv(utls.EnvEnabled, "true")
	t.Setenv(utls.EnvCACert, "/etc/unheaded/tls/ca.pem")
	t.Setenv(utls.EnvCert, "")
	t.Setenv(utls.EnvKeyFile, "")
	srv, tlsOn, err := newHTTPServer("127.0.0.1:0", okHandler())
	if err != nil {
		t.Fatalf("a client-only unit must start: %v", err)
	}
	if tlsOn || srv.TLSConfig != nil {
		t.Fatal("no certificate, yet the listener is TLS")
	}
	addr := serveOnLoopback(t, srv)
	if got, err := status(t, &http.Client{}, "http://"+addr+"/"); err != nil || got != http.StatusOK {
		t.Fatalf("plain GET = %d, %v", got, err)
	}
}

// M8: with a certificate and key the API keys do not cross in clear.
func TestNewHTTPServer_CertificateMeansTLS(t *testing.T) {
	ca, err := utls.NewCA()
	if err != nil {
		t.Fatal(err)
	}
	issued, err := ca.IssueCert(utls.CertRequest{ServiceName: "chaos-controller", IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)}})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	write := func(name string, raw []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	t.Setenv(utls.EnvEnabled, "true")
	t.Setenv(utls.EnvCACert, write("ca.pem", ca.CertPEM()))
	t.Setenv(utls.EnvCert, write("cert.pem", issued.CertPEM))
	t.Setenv(utls.EnvKeyFile, write("key.pem", issued.KeyPEM))
	t.Setenv(utls.EnvMTLS, "false")

	srv, tlsOn, err := newHTTPServer("127.0.0.1:0", okHandler())
	if err != nil {
		t.Fatal(err)
	}
	if !tlsOn {
		t.Fatal("certificate and key set, yet the listener is plain")
	}
	addr := serveOnLoopback(t, srv)

	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(ca.CertPEM())
	secure := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS13}}}
	if got, err := status(t, secure, "https://"+addr+"/"); err != nil || got != http.StatusOK {
		t.Fatalf("TLS GET = %d, %v", got, err)
	}
	if got, err := status(t, &http.Client{}, "http://"+addr+"/"); err == nil && got == http.StatusOK {
		t.Fatal("a plain GET was answered on the TLS listener")
	}
}

// Half a pair is a mistake, never a quiet fallback to plain HTTP.
func TestNewHTTPServer_HalfAPairRefuses(t *testing.T) {
	for _, env := range []string{utls.EnvCert, utls.EnvKeyFile} {
		t.Run(env, func(t *testing.T) {
			t.Setenv(utls.EnvEnabled, "true")
			t.Setenv(utls.EnvCert, "")
			t.Setenv(utls.EnvKeyFile, "")
			t.Setenv(env, filepath.Join(t.TempDir(), "only-one.pem"))
			if _, _, err := newHTTPServer("127.0.0.1:0", okHandler()); err == nil {
				t.Fatal("half a certificate pair must stop the controller")
			}
		})
	}
}
