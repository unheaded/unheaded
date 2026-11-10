// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package database

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testAuthority is a throwaway CA; its key never leaves memory.
type testAuthority struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	der  []byte
}

func newAuthority(t *testing.T) testAuthority {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test Well CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return testAuthority{cert: cert, key: key, der: der}
}

// writeCA writes the public certificate and returns its path.
func (a testAuthority) writeCA(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "ca.crt")
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: a.der})
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// serverCert issues a certificate for the given IP addresses and DNS names.
func (a testAuthority) serverCert(t *testing.T, ips []net.IP, names ...string) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "unheaded-postgres"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  ips,
		DNSNames:     names,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, a.cert, &key.PublicKey, a.key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// tlsOnlyPostgres answers the driver's SSLRequest and completes the TLS
// handshake, then hangs up. It reports whether a client finished the handshake.
func tlsOnlyPostgres(t *testing.T, cert tls.Certificate) (port string, finished <-chan error) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	done := make(chan error, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			done <- err
			return
		}
		defer c.Close()
		_ = c.SetDeadline(time.Now().Add(5 * time.Second))
		if _, err := io.ReadFull(c, make([]byte, 8)); err != nil {
			done <- err
			return
		}
		if _, err := c.Write([]byte{'S'}); err != nil {
			done <- err
			return
		}
		srv := tls.Server(c, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS13})
		err = srv.Handshake()
		if err == nil {
			// TLS 1.3: the client's verdict arrives with its first bytes.
			_, err = srv.Read(make([]byte, 1))
		}
		done <- err
	}()
	_, port, err = net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return port, done
}

func wellConfig(port, mode, ca string) Config {
	return Config{Host: "127.0.0.1", Port: port, User: "ops_writer", Password: "p", DBName: "unheaded_ops", SSLMode: mode, SSLRootCert: ca}
}

// A client told to verify proves the server holds a certificate from the
// deployment's CA, under the name it dialled. Only then does the session go on.
func TestConnect_VerifiesTheServer(t *testing.T) {
	theWell, impostor := newAuthority(t), newAuthority(t)
	loopback := []net.IP{net.IPv4(127, 0, 0, 1)}

	for name, tc := range map[string]struct {
		cert     tls.Certificate
		mode, ca string
		accepted bool
	}{
		"verify-full, the Well's certificate": {theWell.serverCert(t, loopback), "verify-full", theWell.writeCA(t), true},
		"verify-ca, the Well's certificate":   {theWell.serverCert(t, loopback), "verify-ca", theWell.writeCA(t), true},
		"verify-full, another CA":             {impostor.serverCert(t, loopback), "verify-full", theWell.writeCA(t), false},
		"verify-ca, another CA":               {impostor.serverCert(t, loopback), "verify-ca", theWell.writeCA(t), false},
		"verify-full, another name":           {theWell.serverCert(t, nil, "elsewhere"), "verify-full", theWell.writeCA(t), false},
		"verify-ca, another name":             {theWell.serverCert(t, nil, "elsewhere"), "verify-ca", theWell.writeCA(t), true},
		"require with a CA acts as verify-ca": {impostor.serverCert(t, loopback), "require", theWell.writeCA(t), false},
		"require without a CA checks nothing": {impostor.serverCert(t, loopback), "require", "", true},
	} {
		t.Run(name, func(t *testing.T) {
			port, finished := tlsOnlyPostgres(t, tc.cert)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, err := Connect(ctx, wellConfig(port, tc.mode, tc.ca))
			if err == nil {
				t.Fatal("Connect returned no error against a server that hangs up")
			}
			var got error
			select {
			case got = <-finished:
			case <-time.After(5 * time.Second):
				t.Fatal("server never saw the handshake end")
			}
			if accepted := got == nil; accepted != tc.accepted {
				t.Fatalf("accepted=%v (server: %v, client: %v), want %v", accepted, got, err, tc.accepted)
			}
			if !tc.accepted && !strings.Contains(err.Error(), "certificate") {
				t.Fatalf("client error does not name the certificate: %v", err)
			}
		})
	}
}

// Told to verify, a client with no usable CA stops before any connection. A
// path that is set counts as being told, whatever the sslmode: lib/pq drops a
// missing sslrootcert under require and carries on unverified.
func TestCheckTLS_FailsLoudlyWithoutTheCA(t *testing.T) {
	dir := t.TempDir()
	notPEM := filepath.Join(dir, "not-a-cert")
	if err := os.WriteFile(notPEM, []byte("not a certificate\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gone := filepath.Join(dir, "gone.crt")
	good := newAuthority(t).writeCA(t)

	for name, cfg := range map[string]Config{
		"verify-full, no path":     {Host: "postgres", SSLMode: "verify-full"},
		"verify-ca, no path":       {Host: "postgres", SSLMode: "verify-ca"},
		"verify-full, file absent": {Host: "postgres", SSLMode: "verify-full", SSLRootCert: gone},
		"verify-full, a directory": {Host: "postgres", SSLMode: "verify-full", SSLRootCert: dir},
		"verify-full, not PEM":     {Host: "postgres", SSLMode: "verify-full", SSLRootCert: notPEM},
		"require, file absent":     {Host: "postgres", SSLMode: "require", SSLRootCert: gone},
		"verify-full, system":      {Host: "postgres", SSLMode: "verify-full", SSLRootCert: "system"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := CheckTLS(cfg); !errors.Is(err, ErrMissingCA) {
				t.Fatalf("CheckTLS = %v, want ErrMissingCA", err)
			}
			cfg.Port, cfg.User, cfg.Password, cfg.DBName = "1", "ops_writer", "p", "unheaded_ops"
			if _, err := Connect(context.Background(), cfg); !errors.Is(err, ErrMissingCA) {
				t.Fatalf("Connect = %v, want ErrMissingCA before any connection attempt", err)
			}
		})
	}

	for name, cfg := range map[string]Config{
		"verify-full with the CA": {Host: "postgres", SSLMode: "verify-full", SSLRootCert: good},
		"verify-ca with the CA":   {Host: "postgres", SSLMode: "verify-ca", SSLRootCert: good},
		"require with the CA":     {Host: "postgres", SSLMode: "require", SSLRootCert: good},
		"require, no path":        {Host: "postgres", SSLMode: "require"},
	} {
		t.Run("allowed "+name, func(t *testing.T) {
			if err := CheckTLS(cfg); err != nil {
				t.Fatalf("CheckTLS refused %+v: %v", cfg, err)
			}
		})
	}
}

// The CA path reaches the driver as a quoted value and stays out of a DSN that
// has none, so a deployment with no CA connects exactly as before.
func TestDSN_SSLRootCert(t *testing.T) {
	c := Config{Host: "h", Port: "1", User: "u", Password: "p", DBName: "d", SSLMode: "verify-full", SSLRootCert: "/etc/o'neil dir/ca.crt"}
	if got, want := c.DSN(), `sslrootcert='/etc/o\'neil dir/ca.crt'`; !strings.Contains(got, want) {
		t.Fatalf("DSN %q lacks %q", got, want)
	}
	c.SSLRootCert = ""
	if strings.Contains(c.DSN(), "sslrootcert") {
		t.Fatalf("DSN names sslrootcert with none set: %q", c.DSN())
	}
}

func TestConfigs_ReadWellSSLRootCert(t *testing.T) {
	builders := map[string]func() Config{"ops": OpsWriterConfig, "kanban": AppKanbanConfig, "auth": AppAuthConfig, "log": LogWriterConfig}
	t.Setenv("WELL_SSLROOTCERT", "x")
	unsetenv(t, "WELL_SSLROOTCERT")
	for name, build := range builders {
		if got := build().SSLRootCert; got != "" {
			t.Fatalf("%s: SSLRootCert = %q with the variable unset", name, got)
		}
	}
	t.Setenv("WELL_SSLROOTCERT", "/etc/unheaded/well-tls/ca.crt")
	for name, build := range builders {
		if got := build().SSLRootCert; got != "/etc/unheaded/well-tls/ca.crt" {
			t.Fatalf("%s: SSLRootCert = %q", name, got)
		}
	}
}
