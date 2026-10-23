// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package pqtest is test support for ADR-105 handshake tests: a throwaway
// certificate, an in-memory TLS handshake, and an assertion that the
// negotiated key exchange is a hybrid ML-KEM group. Import it from _test.go
// files only.
package pqtest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"testing"
	"time"

	pqtls "unheaded/pkg/tls"
)

// Timeout bounds every Handshake.
const Timeout = 5 * time.Second

// SelfSigned returns a one-hour ECDSA P-256 certificate for localhost and
// 127.0.0.1, with Leaf set.
func SelfSigned(t testing.TB) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("pqtest: generate key: %v", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 62))
	if err != nil {
		t.Fatalf("pqtest: serial: %v", err)
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "localhost"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true, // self-signed: it is its own root
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("pqtest: create certificate: %v", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("pqtest: parse certificate: %v", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}
}

// Pool returns a CertPool that trusts cert (made by SelfSigned).
func Pool(cert tls.Certificate) *x509.CertPool {
	p := x509.NewCertPool()
	if cert.Leaf != nil {
		p.AddCert(cert.Leaf)
	}
	return p
}

// Handshake runs a TLS handshake between srv and cli over net.Pipe, both
// sides concurrently, within Timeout. It returns the client's connection
// state and the first error from either side.
func Handshake(t testing.TB, srv, cli *tls.Config) (tls.ConnectionState, error) {
	t.Helper()
	if srv == nil || cli == nil {
		return tls.ConnectionState{}, errors.New("pqtest: nil tls.Config")
	}
	sc, cc := net.Pipe()
	defer func() { _ = sc.Close() }()
	defer func() { _ = cc.Close() }()
	deadline := time.Now().Add(Timeout)
	_ = sc.SetDeadline(deadline)
	_ = cc.SetDeadline(deadline)

	server := tls.Server(sc, srv)
	client := tls.Client(cc, cli)
	srvErr := make(chan error, 1)
	go func() {
		err := server.Handshake()
		if err != nil {
			_ = sc.Close() // unblock the client
		}
		srvErr <- err
	}()
	cliErr := client.Handshake()
	if cliErr != nil {
		_ = cc.Close() // unblock the server
	}
	sErr := <-srvErr
	if cliErr != nil {
		return tls.ConnectionState{}, cliErr
	}
	if sErr != nil {
		return tls.ConnectionState{}, sErr
	}
	return client.ConnectionState(), nil
}

// RequireHybrid fails t unless cs negotiated TLS 1.3 with a hybrid ML-KEM
// group, naming the group it got.
func RequireHybrid(t testing.TB, cs tls.ConnectionState) {
	t.Helper()
	if !cs.HandshakeComplete {
		t.Fatalf("pqtest: no completed handshake (group %v)", cs.CurveID)
		return
	}
	if cs.Version != tls.VersionTLS13 {
		t.Fatalf("pqtest: negotiated %s, want TLS 1.3 (group %v)", tls.VersionName(cs.Version), cs.CurveID)
		return
	}
	if !pqtls.IsHybrid(cs.CurveID) {
		t.Fatalf("pqtest: negotiated classical or non-hybrid group %v, want a hybrid ML-KEM group", cs.CurveID)
	}
}
