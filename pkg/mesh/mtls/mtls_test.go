// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package mtls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/url"
	"testing"
	"time"
)

func TestSPIFFEID(t *testing.T) {
	t.Run("NewSPIFFEID", func(t *testing.T) {
		id := NewSPIFFEID("example.com", "my-service")
		if id.TrustDomain != "example.com" {
			t.Errorf("expected trust domain example.com, got %s", id.TrustDomain)
		}
		if id.Path != "/service/my-service" {
			t.Errorf("expected path /service/my-service, got %s", id.Path)
		}
		if id.String() != "spiffe://example.com/service/my-service" {
			t.Errorf("unexpected string representation: %s", id.String())
		}
	})

	t.Run("NewSPIFFEIDFromNamespace", func(t *testing.T) {
		id := NewSPIFFEIDFromNamespace("example.com", "production", "api-server")
		if id.String() != "spiffe://example.com/ns/production/sa/api-server" {
			t.Errorf("unexpected string representation: %s", id.String())
		}
		if id.ServiceName() != "api-server" {
			t.Errorf("expected service name api-server, got %s", id.ServiceName())
		}
		if id.Namespace() != "production" {
			t.Errorf("expected namespace production, got %s", id.Namespace())
		}
	})

	t.Run("ParseSPIFFEID", func(t *testing.T) {
		id, err := ParseSPIFFEID("spiffe://example.com/ns/default/sa/frontend")
		if err != nil {
			t.Fatalf("failed to parse SPIFFE ID: %v", err)
		}
		if id.TrustDomain != "example.com" {
			t.Errorf("expected trust domain example.com, got %s", id.TrustDomain)
		}
		if id.Namespace() != "default" {
			t.Errorf("expected namespace default, got %s", id.Namespace())
		}
	})

	t.Run("ParseSPIFFEID_Invalid", func(t *testing.T) {
		invalidIDs := []string{
			"",
			"http://example.com/service/test",
			"spiffe://", // no host
			"spiffe://example.com", // no path
			"spiffe://example.com?query=value", // has query
			"spiffe://example.com#fragment", // has fragment
		}

		for _, idStr := range invalidIDs {
			_, err := ParseSPIFFEID(idStr)
			if err == nil {
				t.Errorf("expected error for invalid SPIFFE ID: %s", idStr)
			}
		}
	})

	t.Run("Matches", func(t *testing.T) {
		id := NewSPIFFEIDFromNamespace("example.com", "default", "api")

		tests := []struct {
			pattern string
			match   bool
		}{
			{"spiffe://example.com/*", true},
			{"spiffe://example.com/ns/default/*", true},
			{"spiffe://other.com/*", false},
			{"*", true},
		}

		for _, tt := range tests {
			if id.Matches(tt.pattern) != tt.match {
				t.Errorf("pattern %s: expected match=%v", tt.pattern, tt.match)
			}
		}
	})
}

func TestSPIFFEVerifier(t *testing.T) {
	t.Run("VerifyCertificate", func(t *testing.T) {
		verifier := NewSPIFFEVerifier([]string{"example.com"})

		// Create a certificate with SPIFFE ID
		cert := generateTestCertWithSPIFFE("example.com", "default", "test-service")

		id, err := verifier.VerifyCertificate(cert)
		if err != nil {
			t.Fatalf("verification failed: %v", err)
		}

		if id.ServiceName() != "test-service" {
			t.Errorf("expected service name test-service, got %s", id.ServiceName())
		}
	})

	t.Run("VerifyCertificate_WrongDomain", func(t *testing.T) {
		verifier := NewSPIFFEVerifier([]string{"other.com"})

		cert := generateTestCertWithSPIFFE("example.com", "default", "test-service")

		_, err := verifier.VerifyCertificate(cert)
		if err != ErrTrustDomainMismatch {
			t.Errorf("expected ErrTrustDomainMismatch, got %v", err)
		}
	})

	t.Run("AllowedPatterns", func(t *testing.T) {
		verifier := NewSPIFFEVerifier([]string{"example.com"})
		// Use a pattern that matches the exact SPIFFE ID format
		verifier.AllowID("spiffe://example.com/ns/production/sa/allowed-service")

		allowedCert := generateTestCertWithSPIFFE("example.com", "production", "allowed-service")
		_, err := verifier.VerifyCertificate(allowedCert)
		if err != nil {
			t.Errorf("expected allowed certificate to pass: %v", err)
		}
	})
}

func TestCertificateManager(t *testing.T) {
	t.Run("GenerateCertificate", func(t *testing.T) {
		caCert, caKey := generateTestCA()
		cm := NewCertificateManager(caCert, caKey)

		spiffeID := NewSPIFFEIDFromNamespace("example.com", "default", "test")
		config := &CertificateConfig{
			SPIFFEID:   spiffeID,
			CommonName: "test-service",
			Validity:   time.Hour,
			KeyType:    "ecdsa-p256",
		}

		cert, key, err := cm.GenerateCertificate(config)
		if err != nil {
			t.Fatalf("failed to generate certificate: %v", err)
		}

		if cert == nil {
			t.Fatal("certificate is nil")
		}
		if key == nil {
			t.Fatal("key is nil")
		}

		// Verify certificate properties
		if cert.Subject.CommonName != "test-service" {
			t.Errorf("expected CN=test-service, got %s", cert.Subject.CommonName)
		}

		// Verify SPIFFE ID in URIs
		found := false
		for _, uri := range cert.URIs {
			if uri.Scheme == "spiffe" {
				found = true
				break
			}
		}
		if !found {
			t.Error("SPIFFE ID not found in certificate URIs")
		}
	})

	t.Run("SetCertificate_and_GetCertificate", func(t *testing.T) {
		caCert, caKey := generateTestCA()
		cm := NewCertificateManager(caCert, caKey)

		spiffeID := NewSPIFFEIDFromNamespace("example.com", "default", "test")
		config := &CertificateConfig{
			SPIFFEID:   spiffeID,
			CommonName: "test-service",
			Validity:   time.Hour,
			KeyType:    "ecdsa-p256",
		}

		cert, key, _ := cm.GenerateCertificate(config)
		err := cm.SetCertificate(cert, key, config)
		if err != nil {
			t.Fatalf("failed to set certificate: %v", err)
		}

		gotCert, gotKey, err := cm.GetCertificate()
		if err != nil {
			t.Fatalf("failed to get certificate: %v", err)
		}

		if gotCert.SerialNumber.Cmp(cert.SerialNumber) != 0 {
			t.Error("retrieved certificate serial doesn't match")
		}
		if gotKey == nil {
			t.Error("retrieved key is nil")
		}
	})

	t.Run("NeedsRotation", func(t *testing.T) {
		caCert, caKey := generateTestCA()
		cm := NewCertificateManager(caCert, caKey)
		cm.rotationBuffer = 30 * time.Minute

		// Certificate that expires soon
		spiffeID := NewSPIFFEIDFromNamespace("example.com", "default", "test")
		config := &CertificateConfig{
			SPIFFEID:   spiffeID,
			CommonName: "test-service",
			Validity:   20 * time.Minute, // Less than rotation buffer
			KeyType:    "ecdsa-p256",
		}

		cert, key, _ := cm.GenerateCertificate(config)
		cm.SetCertificate(cert, key, config)

		if !cm.NeedsRotation() {
			t.Error("expected certificate to need rotation")
		}
	})
}

func TestAuthorizationPolicy(t *testing.T) {
	t.Run("AllowedID", func(t *testing.T) {
		policy := &AuthorizationPolicy{
			Service:    "backend",
			AllowedIDs: []string{"spiffe://example.com/ns/production/sa/frontend"},
		}

		frontendID := NewSPIFFEIDFromNamespace("example.com", "production", "frontend")
		if !policy.IsAuthorized(frontendID) {
			t.Error("expected frontend to be authorized")
		}

		backendID := NewSPIFFEIDFromNamespace("example.com", "production", "other-backend")
		if policy.IsAuthorized(backendID) {
			t.Error("expected other-backend to not be authorized")
		}
	})

	t.Run("DeniedID", func(t *testing.T) {
		policy := &AuthorizationPolicy{
			Service:   "backend",
			DeniedIDs: []string{"spiffe://example.com/ns/production/sa/blocked-service"},
		}

		blockedID := NewSPIFFEIDFromNamespace("example.com", "production", "blocked-service")
		if policy.IsAuthorized(blockedID) {
			t.Error("expected blocked-service to be denied")
		}

		allowedID := NewSPIFFEIDFromNamespace("example.com", "production", "allowed-service")
		if !policy.IsAuthorized(allowedID) {
			t.Error("expected allowed-service to be authorized")
		}
	})

	t.Run("WildcardAllowed", func(t *testing.T) {
		policy := &AuthorizationPolicy{
			Service:    "backend",
			AllowedIDs: []string{"spiffe://example.com/*"},
		}

		anyID := NewSPIFFEIDFromNamespace("example.com", "production", "any-service")
		if !policy.IsAuthorized(anyID) {
			t.Error("expected any-service to be authorized with wildcard")
		}
	})

	t.Run("NoAllowedList", func(t *testing.T) {
		// When no allowed list is specified, all non-denied IDs are allowed
		policy := &AuthorizationPolicy{
			Service: "backend",
		}

		anyID := NewSPIFFEIDFromNamespace("example.com", "production", "any-service")
		if !policy.IsAuthorized(anyID) {
			t.Error("expected any-service to be authorized when no allow list")
		}
	})
}

func TestManager(t *testing.T) {
	t.Run("Initialize", func(t *testing.T) {
		caCert, caKey := generateTestCA()

		config := DefaultConfig("example.com", "test-service")
		config.Namespace = "default"
		mgr := NewManager(config)

		err := mgr.Initialize(caCert, caKey)
		if err != nil {
			t.Fatalf("failed to initialize manager: %v", err)
		}

		// Verify we can get TLS configs
		serverConfig := mgr.ServerTLSConfig()
		if serverConfig == nil {
			t.Error("server TLS config is nil")
		}

		clientConfig := mgr.ClientTLSConfig()
		if clientConfig == nil {
			t.Error("client TLS config is nil")
		}

		// Verify SPIFFE ID
		spiffeID := mgr.SPIFFEID()
		if spiffeID == nil {
			t.Error("SPIFFE ID is nil")
		}
		if spiffeID.ServiceName() != "test-service" {
			t.Errorf("expected service name test-service, got %s", spiffeID.ServiceName())
		}
	})

	t.Run("ServerTLSConfig_RequiresClientCert", func(t *testing.T) {
		caCert, caKey := generateTestCA()

		config := DefaultConfig("example.com", "test-service")
		mgr := NewManager(config)
		mgr.Initialize(caCert, caKey)

		serverConfig := mgr.ServerTLSConfig()
		if serverConfig.ClientAuth != tls.RequireAndVerifyClientCert {
			t.Error("server config should require client certificates")
		}
	})
}

func TestSPIFFEBundle(t *testing.T) {
	t.Run("AddRoot_and_CertPool", func(t *testing.T) {
		bundle := NewSPIFFEBundle("example.com")

		caCert, _ := generateTestCA()
		bundle.AddRoot(caCert)

		pool := bundle.CertPool()
		if pool == nil {
			t.Fatal("cert pool is nil")
		}

		if len(bundle.Roots) != 1 {
			t.Errorf("expected 1 root, got %d", len(bundle.Roots))
		}
	})
}

func TestGenerateSelfSignedCA(t *testing.T) {
	cert, key, err := GenerateSelfSignedCA("test.local", 365*24*time.Hour)
	if err != nil {
		t.Fatalf("failed to generate CA: %v", err)
	}

	if cert == nil {
		t.Fatal("certificate is nil")
	}
	if key == nil {
		t.Fatal("key is nil")
	}

	if !cert.IsCA {
		t.Error("certificate should be a CA")
	}

	if cert.Subject.CommonName != "test.local CA" {
		t.Errorf("expected CN=test.local CA, got %s", cert.Subject.CommonName)
	}
}

// Helper functions

func generateTestCA() (*x509.Certificate, *ecdsa.PrivateKey) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "Test CA",
			Organization: []string{"Test Org"},
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
	}

	certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	cert, _ := x509.ParseCertificate(certDER)

	return cert, key
}

func generateTestCertWithSPIFFE(trustDomain, namespace, serviceName string) *x509.Certificate {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))

	spiffeID := NewSPIFFEIDFromNamespace(trustDomain, namespace, serviceName)
	spiffeURI, _ := url.Parse(spiffeID.String())

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: serviceName,
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		URIs:                  []*url.URL{spiffeURI},
		DNSNames:              []string{serviceName},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	cert, _ := x509.ParseCertificate(certDER)

	return cert
}
