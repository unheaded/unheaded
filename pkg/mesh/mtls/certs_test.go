// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package mtls

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"testing"
	"time"
)

func TestDefaultCertificateConfig(t *testing.T) {
	spiffeID := NewSPIFFEID("example.com", "svc")
	cfg := DefaultCertificateConfig(spiffeID)

	if cfg.SPIFFEID != spiffeID {
		t.Error("SPIFFEID mismatch")
	}
	if cfg.CommonName != "svc" {
		t.Errorf("CommonName = %s, want svc", cfg.CommonName)
	}
	if cfg.Validity != DefaultCertValidity {
		t.Errorf("Validity = %v, want %v", cfg.Validity, DefaultCertValidity)
	}
	if cfg.KeyType != "ecdsa-p256" {
		t.Errorf("KeyType = %s, want ecdsa-p256", cfg.KeyType)
	}
	if cfg.IsCA {
		t.Error("IsCA should be false")
	}
	if cfg.NotBefore.IsZero() {
		t.Error("NotBefore should not be zero")
	}
}

func TestNewCertificateManager(t *testing.T) {
	caCert, caKey := generateTestCA()
	cm := NewCertificateManager(caCert, caKey)
	if cm == nil {
		t.Fatal("NewCertificateManager returned nil")
	}
	if cm.rotationBuffer != DefaultRotationBuffer {
		t.Errorf("rotationBuffer = %v, want %v", cm.rotationBuffer, DefaultRotationBuffer)
	}
}

func TestCertificateManager_GenerateCertificate(t *testing.T) {
	caCert, caKey := generateTestCA()
	cm := NewCertificateManager(caCert, caKey)

	spiffeID := NewSPIFFEIDFromNamespace("example.com", "default", "test-svc")

	t.Run("ecdsa-p256", func(t *testing.T) {
		cfg := &CertificateConfig{
			SPIFFEID:   spiffeID,
			CommonName: "test-svc",
			Validity:   time.Hour,
			KeyType:    "ecdsa-p256",
		}
		cert, key, err := cm.GenerateCertificate(cfg)
		if err != nil {
			t.Fatalf("GenerateCertificate failed: %v", err)
		}
		if cert.Subject.CommonName != "test-svc" {
			t.Errorf("CN = %s", cert.Subject.CommonName)
		}
		if key == nil {
			t.Fatal("key is nil")
		}
		// Should have SPIFFE URI
		foundSPIFFE := false
		for _, u := range cert.URIs {
			if u.Scheme == "spiffe" {
				foundSPIFFE = true
			}
		}
		if !foundSPIFFE {
			t.Error("SPIFFE URI not found in certificate")
		}
	})

	t.Run("ecdsa-p384", func(t *testing.T) {
		cfg := &CertificateConfig{
			SPIFFEID:   spiffeID,
			CommonName: "test-svc",
			Validity:   time.Hour,
			KeyType:    "ecdsa-p384",
		}
		cert, key, err := cm.GenerateCertificate(cfg)
		if err != nil {
			t.Fatalf("GenerateCertificate failed: %v", err)
		}
		ecKey := key.(*ecdsa.PrivateKey)
		if ecKey.Curve != elliptic.P384() {
			t.Error("expected P384 curve")
		}
		if cert == nil {
			t.Fatal("cert is nil")
		}
	})

	t.Run("default key type", func(t *testing.T) {
		cfg := &CertificateConfig{
			SPIFFEID:   spiffeID,
			CommonName: "test-svc",
			Validity:   time.Hour,
			KeyType:    "", // empty should default to P256
		}
		_, key, err := cm.GenerateCertificate(cfg)
		if err != nil {
			t.Fatalf("GenerateCertificate failed: %v", err)
		}
		ecKey := key.(*ecdsa.PrivateKey)
		if ecKey.Curve != elliptic.P256() {
			t.Error("expected P256 curve for empty keytype")
		}
	})

	t.Run("unknown key type defaults to P256", func(t *testing.T) {
		cfg := &CertificateConfig{
			SPIFFEID:   spiffeID,
			CommonName: "test-svc",
			Validity:   time.Hour,
			KeyType:    "rsa-4096", // unknown
		}
		_, key, err := cm.GenerateCertificate(cfg)
		if err != nil {
			t.Fatalf("GenerateCertificate failed: %v", err)
		}
		ecKey := key.(*ecdsa.PrivateKey)
		if ecKey.Curve != elliptic.P256() {
			t.Error("expected P256 curve for unknown keytype")
		}
	})

	t.Run("with DNS names and IPs", func(t *testing.T) {
		cfg := &CertificateConfig{
			SPIFFEID:    spiffeID,
			CommonName:  "test-svc",
			Validity:    time.Hour,
			KeyType:     "ecdsa-p256",
			DNSNames:    []string{"test.local", "test.example.com"},
			IPAddresses: []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("10.0.0.1")},
		}
		cert, _, err := cm.GenerateCertificate(cfg)
		if err != nil {
			t.Fatalf("GenerateCertificate failed: %v", err)
		}
		if len(cert.DNSNames) != 2 {
			t.Errorf("expected 2 DNS names, got %d", len(cert.DNSNames))
		}
		if len(cert.IPAddresses) != 2 {
			t.Errorf("expected 2 IP addresses, got %d", len(cert.IPAddresses))
		}
	})

	t.Run("CA certificate", func(t *testing.T) {
		cfg := &CertificateConfig{
			CommonName: "My CA",
			Validity:   24 * time.Hour,
			KeyType:    "ecdsa-p256",
			IsCA:       true,
			MaxPathLen: 2,
		}
		cert, _, err := cm.GenerateCertificate(cfg)
		if err != nil {
			t.Fatalf("GenerateCertificate failed: %v", err)
		}
		if !cert.IsCA {
			t.Error("expected IsCA = true")
		}
		if cert.MaxPathLen != 2 {
			t.Errorf("MaxPathLen = %d, want 2", cert.MaxPathLen)
		}
		if cert.KeyUsage&x509.KeyUsageCertSign == 0 {
			t.Error("expected KeyUsageCertSign")
		}
	})

	t.Run("self-signed when no CA", func(t *testing.T) {
		cmNoCA := NewCertificateManager(nil, nil)
		cfg := &CertificateConfig{
			SPIFFEID:   spiffeID,
			CommonName: "self-signed",
			Validity:   time.Hour,
			KeyType:    "ecdsa-p256",
		}
		cert, _, err := cmNoCA.GenerateCertificate(cfg)
		if err != nil {
			t.Fatalf("GenerateCertificate failed: %v", err)
		}
		// Self-signed: issuer == subject
		if cert.Issuer.CommonName != cert.Subject.CommonName {
			t.Errorf("expected self-signed cert, issuer=%s subject=%s",
				cert.Issuer.CommonName, cert.Subject.CommonName)
		}
	})

	t.Run("explicit NotBefore and NotAfter", func(t *testing.T) {
		nb := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		na := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		cfg := &CertificateConfig{
			SPIFFEID:   spiffeID,
			CommonName: "explicit-times",
			NotBefore:  nb,
			NotAfter:   na,
			KeyType:    "ecdsa-p256",
		}
		cert, _, err := cm.GenerateCertificate(cfg)
		if err != nil {
			t.Fatalf("GenerateCertificate failed: %v", err)
		}
		if !cert.NotBefore.Equal(nb) {
			t.Errorf("NotBefore = %v, want %v", cert.NotBefore, nb)
		}
		if !cert.NotAfter.Equal(na) {
			t.Errorf("NotAfter = %v, want %v", cert.NotAfter, na)
		}
	})
}

func TestCertificateManager_SetAndGetCertificate(t *testing.T) {
	caCert, caKey := generateTestCA()
	cm := NewCertificateManager(caCert, caKey)

	spiffeID := NewSPIFFEID("example.com", "svc")
	cfg := &CertificateConfig{
		SPIFFEID:   spiffeID,
		CommonName: "svc",
		Validity:   time.Hour,
		KeyType:    "ecdsa-p256",
	}

	cert, key, _ := cm.GenerateCertificate(cfg)
	err := cm.SetCertificate(cert, key, cfg)
	if err != nil {
		t.Fatalf("SetCertificate failed: %v", err)
	}

	gotCert, gotKey, err := cm.GetCertificate()
	if err != nil {
		t.Fatalf("GetCertificate failed: %v", err)
	}
	if gotCert.SerialNumber.Cmp(cert.SerialNumber) != 0 {
		t.Error("serial number mismatch")
	}
	if gotKey == nil {
		t.Error("key is nil")
	}
}

func TestCertificateManager_GetCertificatePEM(t *testing.T) {
	caCert, caKey := generateTestCA()
	cm := NewCertificateManager(caCert, caKey)

	t.Run("no cert set", func(t *testing.T) {
		_, _, err := cm.GetCertificatePEM()
		if err != ErrNoPrivateKey {
			t.Errorf("expected ErrNoPrivateKey, got %v", err)
		}
	})

	spiffeID := NewSPIFFEID("example.com", "svc")
	cfg := &CertificateConfig{
		SPIFFEID:   spiffeID,
		CommonName: "svc",
		Validity:   time.Hour,
		KeyType:    "ecdsa-p256",
	}
	cert, key, _ := cm.GenerateCertificate(cfg)
	cm.SetCertificate(cert, key, cfg)

	t.Run("returns valid PEM", func(t *testing.T) {
		certPEM, keyPEM, err := cm.GetCertificatePEM()
		if err != nil {
			t.Fatalf("GetCertificatePEM failed: %v", err)
		}

		block, _ := pem.Decode(certPEM)
		if block == nil || block.Type != "CERTIFICATE" {
			t.Error("invalid cert PEM")
		}

		block, _ = pem.Decode(keyPEM)
		if block == nil || block.Type != "EC PRIVATE KEY" {
			t.Error("invalid key PEM")
		}
	})
}

func TestCertificateManager_GetCertificate_NoCert(t *testing.T) {
	cm := NewCertificateManager(nil, nil)
	_, _, err := cm.GetCertificate()
	if err != ErrNoPrivateKey {
		t.Errorf("expected ErrNoPrivateKey, got %v", err)
	}
}

func TestCertificateManager_GetCertificate_Expired(t *testing.T) {
	caCert, caKey := generateTestCA()
	cm := NewCertificateManager(caCert, caKey)

	spiffeID := NewSPIFFEID("example.com", "svc")
	cfg := &CertificateConfig{
		SPIFFEID:   spiffeID,
		CommonName: "svc",
		NotBefore:  time.Now().Add(-2 * time.Hour),
		NotAfter:   time.Now().Add(-1 * time.Hour), // expired
		KeyType:    "ecdsa-p256",
	}
	cert, key, _ := cm.GenerateCertificate(cfg)
	cm.SetCertificate(cert, key, cfg)

	_, _, err := cm.GetCertificate()
	if err != ErrCertExpired {
		t.Errorf("expected ErrCertExpired, got %v", err)
	}
}

func TestCertificateManager_GetCertificate_NotYetValid(t *testing.T) {
	caCert, caKey := generateTestCA()
	cm := NewCertificateManager(caCert, caKey)

	spiffeID := NewSPIFFEID("example.com", "svc")
	cfg := &CertificateConfig{
		SPIFFEID:   spiffeID,
		CommonName: "svc",
		NotBefore:  time.Now().Add(1 * time.Hour),
		NotAfter:   time.Now().Add(2 * time.Hour),
		KeyType:    "ecdsa-p256",
	}
	cert, key, _ := cm.GenerateCertificate(cfg)
	cm.SetCertificate(cert, key, cfg)

	_, _, err := cm.GetCertificate()
	if err != ErrCertNotYetValid {
		t.Errorf("expected ErrCertNotYetValid, got %v", err)
	}
}

func TestCertificateManager_NeedsRotation(t *testing.T) {
	caCert, caKey := generateTestCA()

	t.Run("nil cert needs rotation", func(t *testing.T) {
		cm := NewCertificateManager(caCert, caKey)
		if !cm.NeedsRotation() {
			t.Error("nil cert should need rotation")
		}
	})

	t.Run("fresh cert does not need rotation", func(t *testing.T) {
		cm := NewCertificateManager(caCert, caKey)
		cm.rotationBuffer = 10 * time.Minute

		spiffeID := NewSPIFFEID("example.com", "svc")
		cfg := &CertificateConfig{
			SPIFFEID:   spiffeID,
			CommonName: "svc",
			Validity:   24 * time.Hour, // Long validity
			KeyType:    "ecdsa-p256",
		}
		cert, key, _ := cm.GenerateCertificate(cfg)
		cm.SetCertificate(cert, key, cfg)

		if cm.NeedsRotation() {
			t.Error("fresh cert should not need rotation")
		}
	})

	t.Run("expiring cert needs rotation", func(t *testing.T) {
		cm := NewCertificateManager(caCert, caKey)
		cm.rotationBuffer = 30 * time.Minute

		spiffeID := NewSPIFFEID("example.com", "svc")
		cfg := &CertificateConfig{
			SPIFFEID:   spiffeID,
			CommonName: "svc",
			Validity:   20 * time.Minute, // Less than rotation buffer
			KeyType:    "ecdsa-p256",
		}
		cert, key, _ := cm.GenerateCertificate(cfg)
		cm.SetCertificate(cert, key, cfg)

		if !cm.NeedsRotation() {
			t.Error("expiring cert should need rotation")
		}
	})
}

func TestCertificateManager_Rotate(t *testing.T) {
	caCert, caKey := generateTestCA()
	cm := NewCertificateManager(caCert, caKey)

	t.Run("no config", func(t *testing.T) {
		err := cm.Rotate()
		if err == nil {
			t.Error("expected error when no config set")
		}
	})

	t.Run("successful rotation", func(t *testing.T) {
		spiffeID := NewSPIFFEID("example.com", "svc")
		cfg := &CertificateConfig{
			SPIFFEID:   spiffeID,
			CommonName: "svc",
			Validity:   time.Hour,
			KeyType:    "ecdsa-p256",
		}
		cert, key, _ := cm.GenerateCertificate(cfg)
		cm.SetCertificate(cert, key, cfg)

		oldSerial := cert.SerialNumber

		var rotated bool
		cm.OnRotation(func(c *x509.Certificate, k crypto.Signer) {
			rotated = true
		})

		err := cm.Rotate()
		if err != nil {
			t.Fatalf("Rotate failed: %v", err)
		}

		newCert, _, err := cm.GetCertificate()
		if err != nil {
			t.Fatalf("GetCertificate after rotate failed: %v", err)
		}
		if newCert.SerialNumber.Cmp(oldSerial) == 0 {
			t.Error("serial should change after rotation")
		}
		if !rotated {
			t.Error("rotation callback was not called")
		}
	})
}

func TestCertificateManager_OnRotation(t *testing.T) {
	cm := NewCertificateManager(nil, nil)
	called := false
	cm.OnRotation(func(c *x509.Certificate, k crypto.Signer) {
		called = true
	})
	if called {
		t.Error("callback should not be called immediately")
	}
}

func TestCertificateManager_Close(t *testing.T) {
	caCert, caKey := generateTestCA()
	cm := NewCertificateManager(caCert, caKey)

	spiffeID := NewSPIFFEID("example.com", "svc")
	cfg := &CertificateConfig{
		SPIFFEID:   spiffeID,
		CommonName: "svc",
		Validity:   time.Hour,
		KeyType:    "ecdsa-p256",
	}
	cert, key, _ := cm.GenerateCertificate(cfg)
	cm.SetCertificate(cert, key, cfg)

	cm.Close()
	if !cm.closed {
		t.Error("expected closed = true")
	}

	// Close again should be safe
	cm.Close()
}

func TestGenerateSelfSignedCA_Extended(t *testing.T) {
	cert, key, err := GenerateSelfSignedCA("test.local", 365*24*time.Hour)
	if err != nil {
		t.Fatalf("GenerateSelfSignedCA failed: %v", err)
	}
	if !cert.IsCA {
		t.Error("expected IsCA = true")
	}
	if cert.Subject.CommonName != "test.local CA" {
		t.Errorf("CN = %s, want test.local CA", cert.Subject.CommonName)
	}
	if cert.Subject.Organization[0] != "Unheaded Kingdom" {
		t.Errorf("Organization = %v", cert.Subject.Organization)
	}
	if key == nil {
		t.Fatal("key is nil")
	}
	if cert.MaxPathLen != 1 {
		t.Errorf("MaxPathLen = %d, want 1", cert.MaxPathLen)
	}
	// Should be self-signed
	if cert.Issuer.CommonName != cert.Subject.CommonName {
		t.Error("expected self-signed certificate")
	}
}

func TestCertificateChain_PEM(t *testing.T) {
	caCert, caKey := generateTestCA()

	cm := NewCertificateManager(caCert, caKey)
	spiffeID := NewSPIFFEID("example.com", "svc")
	cfg := &CertificateConfig{
		SPIFFEID:   spiffeID,
		CommonName: "svc",
		Validity:   time.Hour,
		KeyType:    "ecdsa-p256",
	}
	leaf, key, _ := cm.GenerateCertificate(cfg)

	chain := &CertificateChain{
		Leaf:          leaf,
		Intermediates: []*x509.Certificate{caCert},
		Key:           key,
	}

	pemData := chain.PEM()
	if len(pemData) == 0 {
		t.Fatal("PEM returned empty data")
	}

	// Should contain two CERTIFICATE blocks
	var count int
	rest := pemData
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type == "CERTIFICATE" {
			count++
		}
	}
	if count != 2 {
		t.Errorf("expected 2 CERTIFICATE blocks, got %d", count)
	}
}

func TestCertificateChain_Verify(t *testing.T) {
	caCert, caKey := generateTestCA()
	roots := x509.NewCertPool()
	roots.AddCert(caCert)

	cm := NewCertificateManager(caCert, caKey)
	spiffeID := NewSPIFFEID("example.com", "svc")
	cfg := &CertificateConfig{
		SPIFFEID:   spiffeID,
		CommonName: "svc",
		Validity:   time.Hour,
		KeyType:    "ecdsa-p256",
	}
	leaf, key, _ := cm.GenerateCertificate(cfg)

	t.Run("valid chain", func(t *testing.T) {
		chain := &CertificateChain{
			Leaf: leaf,
			Key:  key,
		}
		err := chain.Verify(roots)
		if err != nil {
			t.Errorf("Verify failed: %v", err)
		}
	})

	t.Run("wrong root", func(t *testing.T) {
		otherCA, _ := generateTestCA()
		wrongRoots := x509.NewCertPool()
		wrongRoots.AddCert(otherCA)

		chain := &CertificateChain{
			Leaf: leaf,
			Key:  key,
		}
		err := chain.Verify(wrongRoots)
		if err == nil {
			t.Error("expected verification to fail with wrong roots")
		}
	})
}

func TestCertificateManager_BuildTemplate_ZeroNotBefore(t *testing.T) {
	caCert, caKey := generateTestCA()
	cm := NewCertificateManager(caCert, caKey)

	cfg := &CertificateConfig{
		CommonName: "zero-times",
		Validity:   time.Hour,
		KeyType:    "ecdsa-p256",
		// NotBefore and NotAfter both zero
	}
	cert, _, err := cm.GenerateCertificate(cfg)
	if err != nil {
		t.Fatalf("GenerateCertificate failed: %v", err)
	}
	// NotBefore should be auto-set (within the last 10 minutes)
	if time.Since(cert.NotBefore) > 10*time.Minute {
		t.Errorf("NotBefore too old: %v", cert.NotBefore)
	}
}

func TestCertificateManager_BuildTemplate_ZeroValidity(t *testing.T) {
	caCert, caKey := generateTestCA()
	cm := NewCertificateManager(caCert, caKey)

	cfg := &CertificateConfig{
		CommonName: "zero-validity",
		KeyType:    "ecdsa-p256",
		// Validity = 0, NotAfter = zero
	}
	cert, _, err := cm.GenerateCertificate(cfg)
	if err != nil {
		t.Fatalf("GenerateCertificate failed: %v", err)
	}
	// Should default to DefaultCertValidity
	duration := cert.NotAfter.Sub(cert.NotBefore)
	// Allow some tolerance
	if duration < DefaultCertValidity-time.Minute || duration > DefaultCertValidity+time.Minute {
		t.Errorf("duration = %v, expected near %v", duration, DefaultCertValidity)
	}
}

func TestCertificateManager_ScheduleRotation_ClosedManager(t *testing.T) {
	caCert, caKey := generateTestCA()
	cm := NewCertificateManager(caCert, caKey)
	cm.Close()

	spiffeID := NewSPIFFEID("example.com", "svc")
	cfg := &CertificateConfig{
		SPIFFEID:   spiffeID,
		CommonName: "svc",
		Validity:   time.Hour,
		KeyType:    "ecdsa-p256",
	}
	// Even though closed, SetCertificate should work (scheduleRotation
	// will not schedule because cm.closed is true)
	cert, key, _ := cm.GenerateCertificate(cfg)
	err := cm.SetCertificate(cert, key, cfg)
	if err != nil {
		t.Fatalf("SetCertificate after close failed: %v", err)
	}
}

// helper: generate an intermediate CA signed by a root
func generateIntermediateCA(rootCert *x509.Certificate, rootKey *ecdsa.PrivateKey) (*x509.Certificate, *ecdsa.PrivateKey) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "Intermediate CA",
			Organization: []string{"Test Org"},
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
	}
	certDER, _ := x509.CreateCertificate(rand.Reader, template, rootCert, &key.PublicKey, rootKey)
	cert, _ := x509.ParseCertificate(certDER)
	return cert, key
}

func TestCertificateChain_VerifyWithIntermediate(t *testing.T) {
	rootCert, rootKey := generateTestCA()
	intermCert, intermKey := generateIntermediateCA(rootCert, rootKey)

	// Generate leaf signed by intermediate
	cm := NewCertificateManager(intermCert, intermKey)
	cfg := &CertificateConfig{
		CommonName: "leaf",
		Validity:   time.Hour,
		KeyType:    "ecdsa-p256",
	}
	leaf, key, _ := cm.GenerateCertificate(cfg)

	roots := x509.NewCertPool()
	roots.AddCert(rootCert)

	chain := &CertificateChain{
		Leaf:          leaf,
		Intermediates: []*x509.Certificate{intermCert},
		Key:           key,
	}
	err := chain.Verify(roots)
	if err != nil {
		t.Errorf("Verify with intermediate failed: %v", err)
	}
}
