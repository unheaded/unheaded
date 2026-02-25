// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package tls

import (
	"crypto/x509"
	"encoding/pem"
	"net"
	"testing"
	"time"
)

func TestNewCA(t *testing.T) {
	ca, err := NewCA()
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}

	if ca.Certificate() == nil {
		t.Fatal("CA certificate is nil")
	}
	if !ca.Certificate().IsCA {
		t.Error("CA certificate IsCA flag not set")
	}
	if ca.Certificate().Subject.CommonName != "Unheaded Internal CA" {
		t.Errorf("CA CN = %q, want %q", ca.Certificate().Subject.CommonName, "Unheaded Internal CA")
	}
	if len(ca.CertPEM()) == 0 {
		t.Error("CA CertPEM is empty")
	}
	if len(ca.KeyPEM()) == 0 {
		t.Error("CA KeyPEM is empty")
	}
}

func TestCA_CertPool(t *testing.T) {
	ca, err := NewCA()
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}

	pool := ca.CertPool()
	if pool == nil {
		t.Fatal("CertPool returned nil")
	}

	// Verify CA cert validates against its own pool.
	block, _ := pem.Decode(ca.CertPEM())
	cert, _ := x509.ParseCertificate(block.Bytes)
	opts := x509.VerifyOptions{Roots: pool}
	if _, err := cert.Verify(opts); err != nil {
		t.Errorf("CA cert does not verify against its own pool: %v", err)
	}
}

func TestCA_IssueCert(t *testing.T) {
	ca, err := NewCA()
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}

	sc, err := ca.IssueCert(CertRequest{
		ServiceName: "timeguru",
		IPAddresses: []net.IP{net.ParseIP("10.10.10.20")},
		TTL:         24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("IssueCert: %v", err)
	}

	if len(sc.CertPEM) == 0 {
		t.Error("service CertPEM is empty")
	}
	if len(sc.KeyPEM) == 0 {
		t.Error("service KeyPEM is empty")
	}
	if len(sc.CAPEM) == 0 {
		t.Error("service CAPEM is empty")
	}

	// Parse and verify the issued certificate.
	block, _ := pem.Decode(sc.CertPEM)
	if block == nil {
		t.Fatal("failed to decode service cert PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}

	// CN
	if cert.Subject.CommonName != "timeguru.unheaded.internal" {
		t.Errorf("CN = %q, want %q", cert.Subject.CommonName, "timeguru.unheaded.internal")
	}

	// SAN DNS names
	expectedDNS := []string{"timeguru", "timeguru.unheaded.internal", "localhost"}
	for _, want := range expectedDNS {
		if !containsString(cert.DNSNames, want) {
			t.Errorf("missing DNS SAN %q in %v", want, cert.DNSNames)
		}
	}

	// SAN IPs
	found1010 := false
	foundLoopback := false
	for _, ip := range cert.IPAddresses {
		if ip.Equal(net.ParseIP("10.10.10.20")) {
			found1010 = true
		}
		if ip.Equal(net.IPv4(127, 0, 0, 1)) {
			foundLoopback = true
		}
	}
	if !found1010 {
		t.Error("missing IP SAN 10.10.10.20")
	}
	if !foundLoopback {
		t.Error("missing IP SAN 127.0.0.1")
	}

	// ExtKeyUsage
	hasServer := false
	hasClient := false
	for _, usage := range cert.ExtKeyUsage {
		if usage == x509.ExtKeyUsageServerAuth {
			hasServer = true
		}
		if usage == x509.ExtKeyUsageClientAuth {
			hasClient = true
		}
	}
	if !hasServer {
		t.Error("missing ExtKeyUsageServerAuth")
	}
	if !hasClient {
		t.Error("missing ExtKeyUsageClientAuth")
	}

	// Verify against CA
	if err := ca.VerifyCert(sc.CertPEM); err != nil {
		t.Errorf("VerifyCert failed for valid cert: %v", err)
	}
}

func TestCA_IssueCert_EmptyServiceName(t *testing.T) {
	ca, err := NewCA()
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}

	_, err = ca.IssueCert(CertRequest{})
	if err != ErrInvalidCertRequest {
		t.Errorf("expected ErrInvalidCertRequest, got %v", err)
	}
}

func TestCA_IssueCert_DefaultTTL(t *testing.T) {
	ca, err := NewCA()
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}

	sc, err := ca.IssueCert(CertRequest{ServiceName: "captain"})
	if err != nil {
		t.Fatalf("IssueCert: %v", err)
	}

	block, _ := pem.Decode(sc.CertPEM)
	cert, _ := x509.ParseCertificate(block.Bytes)

	// Default TTL is 24h. Allow some tolerance for clock skew buffer.
	validity := cert.NotAfter.Sub(cert.NotBefore)
	if validity < 24*time.Hour || validity > 25*time.Hour {
		t.Errorf("validity = %v, want ~24h", validity)
	}
}

func TestCA_IssueCert_CustomDNSAndIPs(t *testing.T) {
	ca, err := NewCA()
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}

	sc, err := ca.IssueCert(CertRequest{
		ServiceName: "architect",
		DNSNames:    []string{"architect.custom.local"},
		IPAddresses: []net.IP{net.ParseIP("10.10.10.22")},
	})
	if err != nil {
		t.Fatalf("IssueCert: %v", err)
	}

	block, _ := pem.Decode(sc.CertPEM)
	cert, _ := x509.ParseCertificate(block.Bytes)

	if !containsString(cert.DNSNames, "architect.custom.local") {
		t.Error("missing custom DNS SAN")
	}

	foundCustomIP := false
	for _, ip := range cert.IPAddresses {
		if ip.Equal(net.ParseIP("10.10.10.22")) {
			foundCustomIP = true
		}
	}
	if !foundCustomIP {
		t.Error("missing custom IP SAN 10.10.10.22")
	}
}

func TestCA_VerifyCert_WrongCA(t *testing.T) {
	ca1, err := NewCA()
	if err != nil {
		t.Fatalf("NewCA(1): %v", err)
	}

	ca2, err := NewCA()
	if err != nil {
		t.Fatalf("NewCA(2): %v", err)
	}

	// Issue from ca1, verify against ca2 — should fail.
	sc, err := ca1.IssueCert(CertRequest{ServiceName: "monad"})
	if err != nil {
		t.Fatalf("IssueCert: %v", err)
	}

	if err := ca2.VerifyCert(sc.CertPEM); err == nil {
		t.Error("VerifyCert should fail for cert from different CA")
	}
}

func TestCA_VerifyCert_ExpiredCert(t *testing.T) {
	ca, err := NewCA()
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}

	// Issue a cert with a very short TTL that is already expired.
	// We issue with TTL=1ms; the clock skew tolerance of -5min means NotBefore
	// is in the past, and NotAfter is essentially now. This cert will fail
	// standard x509 verification because NotAfter < now.
	sc, err := ca.IssueCert(CertRequest{
		ServiceName: "expired-svc",
		TTL:         1 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("IssueCert: %v", err)
	}

	// Wait briefly to ensure we are past NotAfter.
	time.Sleep(10 * time.Millisecond)

	err = ca.VerifyCert(sc.CertPEM)
	if err == nil {
		t.Error("VerifyCert should fail for expired cert")
	}
}

func TestCA_VerifyCert_BadPEM(t *testing.T) {
	ca, err := NewCA()
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}

	if err := ca.VerifyCert([]byte("not-a-pem")); err == nil {
		t.Error("VerifyCert should fail for bad PEM")
	}
}

func TestCA_IssueCert_DuplicateSANs(t *testing.T) {
	ca, err := NewCA()
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}

	// Pass duplicates of auto-added values.
	sc, err := ca.IssueCert(CertRequest{
		ServiceName: "sophia",
		DNSNames:    []string{"localhost", "sophia"},
		IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)},
	})
	if err != nil {
		t.Fatalf("IssueCert: %v", err)
	}

	block, _ := pem.Decode(sc.CertPEM)
	cert, _ := x509.ParseCertificate(block.Bytes)

	// Count occurrences of "localhost" — should be exactly 1.
	count := 0
	for _, name := range cert.DNSNames {
		if name == "localhost" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("localhost appears %d times in DNS SANs, want 1", count)
	}
}
