// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package mtls

import (
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestDefaultProviderConfig(t *testing.T) {
	cfg := DefaultProviderConfig("test.local", "my-svc", "staging")
	if cfg.TrustDomain != "test.local" {
		t.Errorf("TrustDomain = %s", cfg.TrustDomain)
	}
	if cfg.ServiceName != "my-svc" {
		t.Errorf("ServiceName = %s", cfg.ServiceName)
	}
	if cfg.Namespace != "staging" {
		t.Errorf("Namespace = %s", cfg.Namespace)
	}
	if !cfg.AutoRotate {
		t.Error("AutoRotate should be true")
	}
	if !cfg.VerifyClient {
		t.Error("VerifyClient should be true")
	}
	if cfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %d", cfg.MinVersion)
	}
	if cfg.MaxVersion != tls.VersionTLS13 {
		t.Errorf("MaxVersion = %d", cfg.MaxVersion)
	}
	if len(cfg.CipherSuites) != 5 {
		t.Errorf("CipherSuites count = %d, want 5", len(cfg.CipherSuites))
	}
}

func TestNewProvider(t *testing.T) {
	t.Run("with config", func(t *testing.T) {
		cfg := DefaultProviderConfig("test.local", "svc", "default")
		p := NewProvider(cfg)
		if p == nil {
			t.Fatal("nil provider")
		}
		if p.config != cfg {
			t.Error("config mismatch")
		}
		if p.verifier == nil {
			t.Error("verifier is nil")
		}
		if p.bundle == nil {
			t.Error("bundle is nil")
		}
		if p.Initialized() {
			t.Error("should not be initialized")
		}
	})

	t.Run("nil config uses default", func(t *testing.T) {
		p := NewProvider(nil)
		if p == nil {
			t.Fatal("nil provider")
		}
		if p.config.TrustDomain != DefaultTrustDomain {
			t.Errorf("TrustDomain = %s", p.config.TrustDomain)
		}
	})
}

func TestProvider_InitializeWithCA(t *testing.T) {
	caCert, caKey, err := GenerateSelfSignedCA("test.local", 10*365*24*time.Hour)
	if err != nil {
		t.Fatalf("GenerateSelfSignedCA failed: %v", err)
	}

	cfg := DefaultProviderConfig("test.local", "test-svc", "default")
	cfg.AutoRotate = false // Disable auto-rotation for tests
	p := NewProvider(cfg)

	err = p.InitializeWithCA(caCert, caKey)
	if err != nil {
		t.Fatalf("InitializeWithCA failed: %v", err)
	}

	if !p.Initialized() {
		t.Error("should be initialized")
	}

	// ServerTLSConfig
	serverCfg := p.ServerTLSConfig()
	if serverCfg == nil {
		t.Fatal("ServerTLSConfig is nil")
	}
	if serverCfg.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Error("should require client certs")
	}

	// ClientTLSConfig
	clientCfg := p.ClientTLSConfig()
	if clientCfg == nil {
		t.Fatal("ClientTLSConfig is nil")
	}

	// SPIFFEID
	spiffeID := p.SPIFFEID()
	if spiffeID == nil {
		t.Fatal("SPIFFEID is nil")
	}
	if spiffeID.ServiceName() != "test-svc" {
		t.Errorf("ServiceName = %s", spiffeID.ServiceName())
	}

	// RootCAs
	roots := p.RootCAs()
	if roots == nil {
		t.Fatal("RootCAs is nil")
	}

	// Certificate
	cert := p.Certificate()
	if cert == nil {
		t.Fatal("Certificate is nil")
	}

	// Version
	if p.CertificateVersion() != 0 {
		t.Errorf("initial version = %d", p.CertificateVersion())
	}
}

func TestProvider_InitializeWithCA_AlreadyInitialized(t *testing.T) {
	caCert, caKey, _ := GenerateSelfSignedCA("test.local", 10*365*24*time.Hour)

	cfg := DefaultProviderConfig("test.local", "svc", "default")
	cfg.AutoRotate = false
	p := NewProvider(cfg)

	p.InitializeWithCA(caCert, caKey)

	// Second call should be no-op
	err := p.InitializeWithCA(caCert, caKey)
	if err != nil {
		t.Errorf("second InitializeWithCA should return nil: %v", err)
	}
}

func TestProvider_InitializeWithCA_NilCA(t *testing.T) {
	cfg := DefaultProviderConfig("test.local", "svc", "default")
	cfg.AutoRotate = false
	p := NewProvider(cfg)

	// nil CA should still work (generates a new CA internally)
	err := p.InitializeWithCA(nil, nil)
	if err != nil {
		t.Fatalf("InitializeWithCA with nil CA failed: %v", err)
	}
	if !p.Initialized() {
		t.Error("should be initialized")
	}
}

func TestProvider_NotInitialized(t *testing.T) {
	p := NewProvider(nil)

	if p.ServerTLSConfig() != nil {
		t.Error("ServerTLSConfig should be nil when not initialized")
	}
	if p.ClientTLSConfig() != nil {
		t.Error("ClientTLSConfig should be nil when not initialized")
	}
	if p.Certificate() != nil {
		t.Error("Certificate should be nil when not initialized")
	}
	if p.SPIFFEID() != nil {
		t.Error("SPIFFEID should be nil when not initialized")
	}
	if p.RootCAs() != nil {
		t.Error("RootCAs should be nil when not initialized")
	}
}

func TestProvider_VerifyClient_False(t *testing.T) {
	caCert, caKey, _ := GenerateSelfSignedCA("test.local", 10*365*24*time.Hour)

	cfg := DefaultProviderConfig("test.local", "svc", "default")
	cfg.AutoRotate = false
	cfg.VerifyClient = false
	p := NewProvider(cfg)
	p.InitializeWithCA(caCert, caKey)

	serverCfg := p.ServerTLSConfig()
	if serverCfg.ClientAuth != tls.NoClientCert {
		t.Errorf("ClientAuth = %d, want NoClientCert", serverCfg.ClientAuth)
	}
}

func TestProvider_SkipVerify(t *testing.T) {
	caCert, caKey, _ := GenerateSelfSignedCA("test.local", 10*365*24*time.Hour)

	cfg := DefaultProviderConfig("test.local", "svc", "default")
	cfg.AutoRotate = false
	cfg.SkipVerify = true
	p := NewProvider(cfg)
	p.InitializeWithCA(caCert, caKey)

	clientCfg := p.ClientTLSConfig()
	if !clientCfg.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should be true")
	}
}

func TestProvider_OnRotation(t *testing.T) {
	p := NewProvider(nil)
	called := false
	p.OnRotation(func(c *tls.Certificate) {
		called = true
	})
	if called {
		t.Error("callback should not be called immediately")
	}
	if len(p.onRotation) != 1 {
		t.Errorf("onRotation count = %d", len(p.onRotation))
	}
}

func TestProvider_OnError(t *testing.T) {
	p := NewProvider(nil)
	p.OnError(func(err error) {})
	if len(p.onError) != 1 {
		t.Errorf("onError count = %d", len(p.onError))
	}
}

func TestProvider_Rotate_NotInitialized(t *testing.T) {
	p := NewProvider(nil)
	err := p.Rotate()
	if err != ErrProviderNotInitialized {
		t.Errorf("expected ErrProviderNotInitialized, got %v", err)
	}
}

func TestProvider_Rotate_WithCA(t *testing.T) {
	caCert, caKey, _ := GenerateSelfSignedCA("test.local", 10*365*24*time.Hour)

	cfg := DefaultProviderConfig("test.local", "svc", "default")
	cfg.AutoRotate = false
	p := NewProvider(cfg)
	p.InitializeWithCA(caCert, caKey)

	var rotated uint64
	p.OnRotation(func(c *tls.Certificate) {
		atomic.AddUint64(&rotated, 1)
	})

	oldVersion := p.CertificateVersion()
	err := p.Rotate()
	if err != nil {
		t.Fatalf("Rotate failed: %v", err)
	}

	newVersion := p.CertificateVersion()
	if newVersion <= oldVersion {
		t.Errorf("version did not increment: old=%d new=%d", oldVersion, newVersion)
	}

	// Wait a bit for async callback
	time.Sleep(50 * time.Millisecond)
	if atomic.LoadUint64(&rotated) == 0 {
		t.Error("rotation callback was not called")
	}
}

func TestProvider_Close(t *testing.T) {
	caCert, caKey, _ := GenerateSelfSignedCA("test.local", 10*365*24*time.Hour)

	cfg := DefaultProviderConfig("test.local", "svc", "default")
	cfg.AutoRotate = false
	p := NewProvider(cfg)
	p.InitializeWithCA(caCert, caKey)

	err := p.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Double close
	err = p.Close()
	if err != nil {
		t.Fatalf("second Close failed: %v", err)
	}

	// Rotate after close should fail
	err = p.Rotate()
	if err != ErrProviderNotInitialized {
		t.Errorf("Rotate after close: expected ErrProviderNotInitialized, got %v", err)
	}
}

func TestProvider_SaveToFiles(t *testing.T) {
	caCert, caKey, _ := GenerateSelfSignedCA("test.local", 10*365*24*time.Hour)

	cfg := DefaultProviderConfig("test.local", "svc", "default")
	cfg.AutoRotate = false
	p := NewProvider(cfg)
	p.InitializeWithCA(caCert, caKey)

	dir := t.TempDir()
	certDir := filepath.Join(dir, "certs")

	err := p.SaveToFiles(certDir)
	if err != nil {
		t.Fatalf("SaveToFiles failed: %v", err)
	}

	// Verify cert file
	certPath := filepath.Join(certDir, "cert.pem")
	certData, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatalf("cert.pem not found: %v", err)
	}
	block, _ := pem.Decode(certData)
	if block == nil || block.Type != "CERTIFICATE" {
		t.Error("invalid cert.pem")
	}

	// Verify key file
	keyPath := filepath.Join(certDir, "key.pem")
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("key.pem not found: %v", err)
	}
	block, _ = pem.Decode(keyData)
	if block == nil || block.Type != "PRIVATE KEY" {
		t.Error("invalid key.pem")
	}

	// Verify CA file
	caPath := filepath.Join(certDir, "ca.pem")
	caData, err := os.ReadFile(caPath)
	if err != nil {
		t.Fatalf("ca.pem not found: %v", err)
	}
	block, _ = pem.Decode(caData)
	if block == nil || block.Type != "CERTIFICATE" {
		t.Error("invalid ca.pem")
	}
}

func TestProvider_SaveToFiles_NotInitialized(t *testing.T) {
	p := NewProvider(nil)
	dir := t.TempDir()
	err := p.SaveToFiles(dir)
	if err != ErrProviderNotInitialized {
		t.Errorf("expected ErrProviderNotInitialized, got %v", err)
	}
}

func TestProvider_InitializeWithFiles(t *testing.T) {
	// Generate a self-signed cert + CA and save to files
	caCert, caKey, _ := GenerateSelfSignedCA("test.local", 10*365*24*time.Hour)

	// Generate leaf cert
	cm := NewCertificateManager(caCert, caKey.(*ecdsa.PrivateKey))
	spiffeID := NewSPIFFEIDFromNamespace("test.local", "default", "svc")
	certCfg := &CertificateConfig{
		SPIFFEID:   spiffeID,
		CommonName: "svc",
		Validity:   time.Hour,
		KeyType:    "ecdsa-p256",
	}
	leaf, leafKey, _ := cm.GenerateCertificate(certCfg)

	dir := t.TempDir()

	// Write cert
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leaf.Raw})
	certFile := filepath.Join(dir, "cert.pem")
	os.WriteFile(certFile, certPEM, 0600)

	// Write key
	keyDER, _ := x509.MarshalECPrivateKey(leafKey.(*ecdsa.PrivateKey))
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	keyFile := filepath.Join(dir, "key.pem")
	os.WriteFile(keyFile, keyPEM, 0600)

	// Write CA
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCert.Raw})
	caFile := filepath.Join(dir, "ca.pem")
	os.WriteFile(caFile, caPEM, 0600)

	cfg := DefaultProviderConfig("test.local", "svc", "default")
	cfg.AutoRotate = false
	p := NewProvider(cfg)

	err := p.InitializeWithFiles(certFile, keyFile, caFile)
	if err != nil {
		t.Fatalf("InitializeWithFiles failed: %v", err)
	}

	if !p.Initialized() {
		t.Error("should be initialized")
	}

	if p.ServerTLSConfig() == nil {
		t.Error("ServerTLSConfig is nil")
	}
	if p.ClientTLSConfig() == nil {
		t.Error("ClientTLSConfig is nil")
	}
	if p.Certificate() == nil {
		t.Error("Certificate is nil")
	}

	p.Close()
}

func TestProvider_InitializeWithFiles_AlreadyInitialized(t *testing.T) {
	caCert, caKey, _ := GenerateSelfSignedCA("test.local", 10*365*24*time.Hour)
	cm := NewCertificateManager(caCert, caKey.(*ecdsa.PrivateKey))
	spiffeID := NewSPIFFEIDFromNamespace("test.local", "default", "svc")
	leaf, leafKey, _ := cm.GenerateCertificate(&CertificateConfig{
		SPIFFEID: spiffeID, CommonName: "svc", Validity: time.Hour, KeyType: "ecdsa-p256",
	})

	dir := t.TempDir()
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leaf.Raw})
	os.WriteFile(filepath.Join(dir, "cert.pem"), certPEM, 0600)
	keyDER, _ := x509.MarshalECPrivateKey(leafKey.(*ecdsa.PrivateKey))
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	os.WriteFile(filepath.Join(dir, "key.pem"), keyPEM, 0600)
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCert.Raw})
	os.WriteFile(filepath.Join(dir, "ca.pem"), caPEM, 0600)

	cfg := DefaultProviderConfig("test.local", "svc", "default")
	cfg.AutoRotate = false
	p := NewProvider(cfg)

	p.InitializeWithFiles(filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem"), filepath.Join(dir, "ca.pem"))

	// Second call should no-op
	err := p.InitializeWithFiles(filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem"), filepath.Join(dir, "ca.pem"))
	if err != nil {
		t.Errorf("second init should be nil: %v", err)
	}
}

func TestProvider_InitializeWithFiles_BadCert(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "cert.pem"), []byte("bad"), 0600)
	os.WriteFile(filepath.Join(dir, "key.pem"), []byte("bad"), 0600)
	os.WriteFile(filepath.Join(dir, "ca.pem"), []byte("bad"), 0600)

	cfg := DefaultProviderConfig("test.local", "svc", "default")
	cfg.AutoRotate = false
	p := NewProvider(cfg)

	err := p.InitializeWithFiles(
		filepath.Join(dir, "cert.pem"),
		filepath.Join(dir, "key.pem"),
		filepath.Join(dir, "ca.pem"),
	)
	if err == nil {
		t.Error("expected error for bad cert files")
	}
}

func TestProvider_InitializeWithFiles_BadCA(t *testing.T) {
	caCert, caKey, _ := GenerateSelfSignedCA("test.local", 10*365*24*time.Hour)
	cm := NewCertificateManager(caCert, caKey.(*ecdsa.PrivateKey))
	leaf, leafKey, _ := cm.GenerateCertificate(&CertificateConfig{
		CommonName: "svc", Validity: time.Hour, KeyType: "ecdsa-p256",
	})

	dir := t.TempDir()
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leaf.Raw})
	os.WriteFile(filepath.Join(dir, "cert.pem"), certPEM, 0600)
	keyDER, _ := x509.MarshalECPrivateKey(leafKey.(*ecdsa.PrivateKey))
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	os.WriteFile(filepath.Join(dir, "key.pem"), keyPEM, 0600)
	os.WriteFile(filepath.Join(dir, "ca.pem"), []byte("not a pem"), 0600)

	cfg := DefaultProviderConfig("test.local", "svc", "default")
	cfg.AutoRotate = false
	p := NewProvider(cfg)

	err := p.InitializeWithFiles(
		filepath.Join(dir, "cert.pem"),
		filepath.Join(dir, "key.pem"),
		filepath.Join(dir, "ca.pem"),
	)
	if err == nil {
		t.Error("expected error for bad CA file")
	}
}

func TestProvider_CertificateVersion(t *testing.T) {
	p := NewProvider(nil)
	if p.CertificateVersion() != 0 {
		t.Errorf("initial version = %d", p.CertificateVersion())
	}

	atomic.AddUint64(&p.version, 1)
	if p.CertificateVersion() != 1 {
		t.Errorf("version after increment = %d", p.CertificateVersion())
	}
}

func TestProvider_NotifyCallbacks(t *testing.T) {
	caCert, caKey, _ := GenerateSelfSignedCA("test.local", 10*365*24*time.Hour)

	cfg := DefaultProviderConfig("test.local", "svc", "default")
	cfg.AutoRotate = false
	p := NewProvider(cfg)

	var errorCalled uint64
	p.OnError(func(err error) {
		atomic.AddUint64(&errorCalled, 1)
	})

	p.InitializeWithCA(caCert, caKey)

	// Rotate should trigger onRotation
	var rotationCalled uint64
	p.OnRotation(func(c *tls.Certificate) {
		atomic.AddUint64(&rotationCalled, 1)
	})

	p.Rotate()

	time.Sleep(50 * time.Millisecond)
	if atomic.LoadUint64(&rotationCalled) == 0 {
		t.Error("rotation callback not called")
	}
}
