// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

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

func TestNewServerTLSConfig(t *testing.T) {
	ca, err := NewCA()
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}
	sc, err := ca.IssueCert(CertRequest{ServiceName: "test-svc"})
	if err != nil {
		t.Fatalf("IssueCert: %v", err)
	}

	cfg, err := NewServerTLSConfig(ServerConfig{
		CertPEM:           sc.CertPEM,
		KeyPEM:            sc.KeyPEM,
		CAPEM:             ca.CertPEM(),
		RequireClientCert: true,
	})
	if err != nil {
		t.Fatalf("NewServerTLSConfig: %v", err)
	}
	if cfg.MinVersion != 0x0304 { // TLS 1.3
		t.Errorf("MinVersion = %x, want TLS 1.3", cfg.MinVersion)
	}
	if cfg.ClientAuth != 4 { // RequireAndVerifyClientCert
		t.Errorf("ClientAuth = %d, want RequireAndVerifyClientCert", cfg.ClientAuth)
	}
}

func TestNewServerTLSConfig_NoMTLS(t *testing.T) {
	ca, err := NewCA()
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}
	sc, err := ca.IssueCert(CertRequest{ServiceName: "test-svc"})
	if err != nil {
		t.Fatalf("IssueCert: %v", err)
	}

	cfg, err := NewServerTLSConfig(ServerConfig{
		CertPEM:           sc.CertPEM,
		KeyPEM:            sc.KeyPEM,
		RequireClientCert: false,
	})
	if err != nil {
		t.Fatalf("NewServerTLSConfig: %v", err)
	}
	if cfg.ClientAuth != 0 { // NoClientCert
		t.Errorf("ClientAuth = %d, want NoClientCert", cfg.ClientAuth)
	}
}

func TestNewServerTLSConfig_InvalidCert(t *testing.T) {
	_, err := NewServerTLSConfig(ServerConfig{
		CertPEM: []byte("bad"),
		KeyPEM:  []byte("bad"),
	})
	if err == nil {
		t.Error("expected error for invalid cert/key")
	}
}

func TestNewServerTLSConfig_InvalidCA(t *testing.T) {
	ca, err := NewCA()
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}
	sc, err := ca.IssueCert(CertRequest{ServiceName: "test-svc"})
	if err != nil {
		t.Fatalf("IssueCert: %v", err)
	}

	_, err = NewServerTLSConfig(ServerConfig{
		CertPEM:           sc.CertPEM,
		KeyPEM:            sc.KeyPEM,
		CAPEM:             []byte("not a cert"),
		RequireClientCert: true,
	})
	if err == nil {
		t.Error("expected error for invalid CA PEM")
	}
}

func TestNewTLSServer(t *testing.T) {
	ca, err := NewCA()
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}
	sc, err := ca.IssueCert(CertRequest{ServiceName: "test-svc"})
	if err != nil {
		t.Fatalf("IssueCert: %v", err)
	}

	srv, err := NewTLSServer(":0", nil, ServerConfig{
		CertPEM: sc.CertPEM,
		KeyPEM:  sc.KeyPEM,
		CAPEM:   ca.CertPEM(),
	})
	if err != nil {
		t.Fatalf("NewTLSServer: %v", err)
	}
	if srv.TLSConfig == nil {
		t.Error("expected TLSConfig to be set")
	}
	if srv.ReadHeaderTimeout == 0 {
		t.Error("expected ReadHeaderTimeout to be set")
	}
}

func TestNewClientTLSConfig(t *testing.T) {
	ca, err := NewCA()
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}
	sc, err := ca.IssueCert(CertRequest{ServiceName: "test-client"})
	if err != nil {
		t.Fatalf("IssueCert: %v", err)
	}

	cfg, err := NewClientTLSConfig(ClientConfig{
		CertPEM: sc.CertPEM,
		KeyPEM:  sc.KeyPEM,
		CAPEM:   ca.CertPEM(),
	})
	if err != nil {
		t.Fatalf("NewClientTLSConfig: %v", err)
	}
	if cfg.MinVersion != 0x0304 {
		t.Errorf("MinVersion = %x, want TLS 1.3", cfg.MinVersion)
	}
	if len(cfg.Certificates) != 1 {
		t.Errorf("expected 1 client cert, got %d", len(cfg.Certificates))
	}
	if cfg.RootCAs == nil {
		t.Error("expected RootCAs to be set")
	}
}

func TestNewClientTLSConfig_NoCert(t *testing.T) {
	ca, err := NewCA()
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}

	cfg, err := NewClientTLSConfig(ClientConfig{
		CAPEM: ca.CertPEM(),
	})
	if err != nil {
		t.Fatalf("NewClientTLSConfig: %v", err)
	}
	if len(cfg.Certificates) != 0 {
		t.Errorf("expected no client certs, got %d", len(cfg.Certificates))
	}
}

func TestNewClientTLSConfig_InvalidCert(t *testing.T) {
	_, err := NewClientTLSConfig(ClientConfig{
		CertPEM: []byte("bad"),
		KeyPEM:  []byte("bad"),
	})
	if err == nil {
		t.Error("expected error for invalid client cert")
	}
}

func TestNewClientTLSConfig_InvalidCA(t *testing.T) {
	_, err := NewClientTLSConfig(ClientConfig{
		CAPEM: []byte("not a cert"),
	})
	if err == nil {
		t.Error("expected error for invalid CA PEM")
	}
}

func TestNewTLSHTTPClient(t *testing.T) {
	ca, err := NewCA()
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}
	sc, err := ca.IssueCert(CertRequest{ServiceName: "test-client"})
	if err != nil {
		t.Fatalf("IssueCert: %v", err)
	}

	client, err := NewTLSHTTPClient(ClientConfig{
		CertPEM: sc.CertPEM,
		KeyPEM:  sc.KeyPEM,
		CAPEM:   ca.CertPEM(),
	})
	if err != nil {
		t.Fatalf("NewTLSHTTPClient: %v", err)
	}
	if client.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", client.Timeout)
	}
}

func TestNewTLSHTTPClient_CustomTimeout(t *testing.T) {
	ca, err := NewCA()
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}
	sc, err := ca.IssueCert(CertRequest{ServiceName: "test-client"})
	if err != nil {
		t.Fatalf("IssueCert: %v", err)
	}

	client, err := NewTLSHTTPClient(ClientConfig{
		CertPEM: sc.CertPEM,
		KeyPEM:  sc.KeyPEM,
		CAPEM:   ca.CertPEM(),
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewTLSHTTPClient: %v", err)
	}
	if client.Timeout != 5*time.Second {
		t.Errorf("Timeout = %v, want 5s", client.Timeout)
	}
}

func TestNewServerTransportCredentials(t *testing.T) {
	ca, err := NewCA()
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}
	sc, err := ca.IssueCert(CertRequest{ServiceName: "grpc-svc"})
	if err != nil {
		t.Fatalf("IssueCert: %v", err)
	}

	creds, err := NewServerTransportCredentials(ServerConfig{
		CertPEM: sc.CertPEM,
		KeyPEM:  sc.KeyPEM,
		CAPEM:   ca.CertPEM(),
	})
	if err != nil {
		t.Fatalf("NewServerTransportCredentials: %v", err)
	}
	if creds == nil {
		t.Error("expected non-nil credentials")
	}
	info := creds.Info()
	if info.SecurityProtocol != "tls" {
		t.Errorf("SecurityProtocol = %q, want tls", info.SecurityProtocol)
	}
}

func TestNewClientTransportCredentials(t *testing.T) {
	ca, err := NewCA()
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}
	sc, err := ca.IssueCert(CertRequest{ServiceName: "grpc-client"})
	if err != nil {
		t.Fatalf("IssueCert: %v", err)
	}

	creds, err := NewClientTransportCredentials(ClientConfig{
		CertPEM: sc.CertPEM,
		KeyPEM:  sc.KeyPEM,
		CAPEM:   ca.CertPEM(),
	})
	if err != nil {
		t.Fatalf("NewClientTransportCredentials: %v", err)
	}
	if creds == nil {
		t.Error("expected non-nil credentials")
	}
}

func TestServerTLSConfigToCredentials(t *testing.T) {
	ca, err := NewCA()
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}
	sc, err := ca.IssueCert(CertRequest{ServiceName: "svc"})
	if err != nil {
		t.Fatalf("IssueCert: %v", err)
	}
	cfg, err := NewServerTLSConfig(ServerConfig{
		CertPEM: sc.CertPEM,
		KeyPEM:  sc.KeyPEM,
	})
	if err != nil {
		t.Fatalf("NewServerTLSConfig: %v", err)
	}

	creds := ServerTLSConfigToCredentials(cfg)
	if creds == nil {
		t.Error("expected non-nil credentials")
	}
}

func TestClientTLSConfigToCredentials(t *testing.T) {
	ca, err := NewCA()
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}

	cfg, err := NewClientTLSConfig(ClientConfig{
		CAPEM: ca.CertPEM(),
	})
	if err != nil {
		t.Fatalf("NewClientTLSConfig: %v", err)
	}

	creds := ClientTLSConfigToCredentials(cfg)
	if creds == nil {
		t.Error("expected non-nil credentials")
	}
}

func TestLoadServiceTLSConfig_Disabled(t *testing.T) {
	// Without env vars, TLS should be disabled
	cfg := LoadServiceTLSConfig()
	if cfg.Enabled {
		t.Error("expected Enabled=false when env not set")
	}
	if !cfg.RequireClientCert {
		t.Error("expected RequireClientCert=true by default")
	}
}

func TestMaybeWrapServer_Disabled(t *testing.T) {
	srv, tlsEnabled, err := MaybeWrapServer(":0", nil)
	if err != nil {
		t.Fatalf("MaybeWrapServer: %v", err)
	}
	if tlsEnabled {
		t.Error("expected tlsEnabled=false")
	}
	if srv == nil {
		t.Error("expected non-nil server")
	}
}

func TestMaybeCreateClient_Disabled(t *testing.T) {
	client, err := MaybeCreateClient()
	if err != nil {
		t.Fatalf("MaybeCreateClient: %v", err)
	}
	if client == nil {
		t.Error("expected non-nil client")
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
