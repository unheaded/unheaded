// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package mtls

import (
	"crypto/tls"
	"crypto/x509"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig("test.local", "my-svc")
	if cfg.TrustDomain != "test.local" {
		t.Errorf("TrustDomain = %s", cfg.TrustDomain)
	}
	if cfg.ServiceName != "my-svc" {
		t.Errorf("ServiceName = %s", cfg.ServiceName)
	}
	if cfg.Namespace != "default" {
		t.Errorf("Namespace = %s", cfg.Namespace)
	}
	if cfg.CertValidity != DefaultCertValidity {
		t.Errorf("CertValidity = %v", cfg.CertValidity)
	}
	if cfg.RotationBuffer != DefaultRotationBuffer {
		t.Errorf("RotationBuffer = %v", cfg.RotationBuffer)
	}
	if cfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %d", cfg.MinVersion)
	}
	if cfg.MaxVersion != tls.VersionTLS13 {
		t.Errorf("MaxVersion = %d", cfg.MaxVersion)
	}
	if len(cfg.CipherSuites) != 3 {
		t.Errorf("CipherSuites count = %d, want 3", len(cfg.CipherSuites))
	}
}

func TestNewManager(t *testing.T) {
	cfg := DefaultConfig("test.local", "my-svc")
	mgr := NewManager(cfg)
	if mgr == nil {
		t.Fatal("NewManager returned nil")
	}
	if mgr.config != cfg {
		t.Error("config mismatch")
	}
	if mgr.verifier == nil {
		t.Error("verifier is nil")
	}
	if mgr.bundle == nil {
		t.Error("bundle is nil")
	}
	if mgr.initialized {
		t.Error("should not be initialized yet")
	}
}

func TestManager_Initialize(t *testing.T) {
	caCert, caKey := generateTestCA()
	cfg := DefaultConfig("example.com", "test-svc")
	mgr := NewManager(cfg)

	err := mgr.Initialize(caCert, caKey)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if !mgr.initialized {
		t.Error("should be initialized")
	}

	// ServerTLSConfig
	serverCfg := mgr.ServerTLSConfig()
	if serverCfg == nil {
		t.Fatal("ServerTLSConfig is nil")
	}
	if serverCfg.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Error("server should require client certs")
	}
	if serverCfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("server MinVersion = %d", serverCfg.MinVersion)
	}

	// ClientTLSConfig
	clientCfg := mgr.ClientTLSConfig()
	if clientCfg == nil {
		t.Fatal("ClientTLSConfig is nil")
	}

	// SPIFFEID
	id := mgr.SPIFFEID()
	if id == nil {
		t.Fatal("SPIFFEID is nil")
	}
	if id.ServiceName() != "test-svc" {
		t.Errorf("ServiceName = %s", id.ServiceName())
	}

	// Certificate
	cert, err := mgr.Certificate()
	if err != nil {
		t.Fatalf("Certificate failed: %v", err)
	}
	if cert == nil {
		t.Fatal("Certificate is nil")
	}
}

func TestManager_InitializeWithBundle(t *testing.T) {
	caCert, caKey := generateTestCA()

	// Generate a cert to use with InitializeWithBundle
	cm := NewCertificateManager(caCert, caKey)
	spiffeID := NewSPIFFEIDFromNamespace("example.com", "default", "test-svc")
	certCfg := &CertificateConfig{
		SPIFFEID:   spiffeID,
		CommonName: "test-svc",
		Validity:   time.Hour,
		KeyType:    "ecdsa-p256",
	}
	cert, key, _ := cm.GenerateCertificate(certCfg)

	rootPool := x509.NewCertPool()
	rootPool.AddCert(caCert)

	cfg := DefaultConfig("example.com", "test-svc")
	mgr := NewManager(cfg)

	err := mgr.InitializeWithBundle(cert, key, rootPool)
	if err != nil {
		t.Fatalf("InitializeWithBundle failed: %v", err)
	}

	if !mgr.initialized {
		t.Error("should be initialized")
	}

	gotCert, err := mgr.Certificate()
	if err != nil {
		t.Fatalf("Certificate failed: %v", err)
	}
	if gotCert.Subject.CommonName != "test-svc" {
		t.Errorf("CN = %s", gotCert.Subject.CommonName)
	}
}

func TestManager_InitializeWithBundle_NilRootCAs(t *testing.T) {
	caCert, caKey := generateTestCA()
	cm := NewCertificateManager(caCert, caKey)
	spiffeID := NewSPIFFEIDFromNamespace("example.com", "default", "svc")
	certCfg := &CertificateConfig{
		SPIFFEID:   spiffeID,
		CommonName: "svc",
		Validity:   time.Hour,
		KeyType:    "ecdsa-p256",
	}
	cert, key, _ := cm.GenerateCertificate(certCfg)

	cfg := DefaultConfig("example.com", "svc")
	mgr := NewManager(cfg)

	// Pass nil rootCAs - should still work
	err := mgr.InitializeWithBundle(cert, key, nil)
	if err != nil {
		t.Fatalf("InitializeWithBundle with nil rootCAs failed: %v", err)
	}
}

func TestManager_InitializeWithBundle_NoCertSPIFFE(t *testing.T) {
	// Cert without SPIFFE ID - should fall back to config
	caCert, caKey := generateTestCA()
	cm := NewCertificateManager(caCert, caKey)
	certCfg := &CertificateConfig{
		CommonName: "no-spiffe",
		Validity:   time.Hour,
		KeyType:    "ecdsa-p256",
		// No SPIFFEID
	}
	cert, key, _ := cm.GenerateCertificate(certCfg)

	cfg := DefaultConfig("example.com", "fallback-svc")
	mgr := NewManager(cfg)

	rootPool := x509.NewCertPool()
	rootPool.AddCert(caCert)

	err := mgr.InitializeWithBundle(cert, key, rootPool)
	if err != nil {
		t.Fatalf("InitializeWithBundle failed: %v", err)
	}
}

func TestManager_Certificate_NoInit(t *testing.T) {
	cfg := DefaultConfig("example.com", "svc")
	mgr := NewManager(cfg)

	_, err := mgr.Certificate()
	if err != ErrNoCertificate {
		t.Errorf("expected ErrNoCertificate, got %v", err)
	}
}

func TestManager_Close(t *testing.T) {
	caCert, caKey := generateTestCA()
	cfg := DefaultConfig("example.com", "svc")
	mgr := NewManager(cfg)
	mgr.Initialize(caCert, caKey)

	mgr.Close()
	// Close again should be safe
	mgr.Close()
}

func TestManager_Close_NotInitialized(t *testing.T) {
	cfg := DefaultConfig("example.com", "svc")
	mgr := NewManager(cfg)
	// Should not panic
	mgr.Close()
}

func TestTLSVersionString(t *testing.T) {
	tests := []struct {
		version uint16
		want    string
	}{
		{tls.VersionTLS10, "TLS 1.0"},
		{tls.VersionTLS11, "TLS 1.1"},
		{tls.VersionTLS12, "TLS 1.2"},
		{tls.VersionTLS13, "TLS 1.3"},
		{0, "unknown"},
		{9999, "unknown"},
	}
	for _, tt := range tests {
		if got := TLSVersionString(tt.version); got != tt.want {
			t.Errorf("TLSVersionString(%d) = %s, want %s", tt.version, got, tt.want)
		}
	}
}

func TestManager_BuildTLSConfigs_UsesBundle(t *testing.T) {
	caCert, caKey := generateTestCA()
	cfg := DefaultConfig("example.com", "svc")
	// cfg.RootCAs is nil, so buildTLSConfigs should use bundle

	mgr := NewManager(cfg)
	err := mgr.Initialize(caCert, caKey)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	serverCfg := mgr.ServerTLSConfig()
	if serverCfg.ClientCAs == nil {
		t.Error("server config ClientCAs should not be nil")
	}

	clientCfg := mgr.ClientTLSConfig()
	if clientCfg.RootCAs == nil {
		t.Error("client config RootCAs should not be nil")
	}
}

func TestManager_BuildTLSConfigs_UsesExplicitRootCAs(t *testing.T) {
	caCert, caKey := generateTestCA()

	rootPool := x509.NewCertPool()
	rootPool.AddCert(caCert)

	cfg := DefaultConfig("example.com", "svc")
	cfg.RootCAs = rootPool

	mgr := NewManager(cfg)
	err := mgr.Initialize(caCert, caKey)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	serverCfg := mgr.ServerTLSConfig()
	if serverCfg.ClientCAs == nil {
		t.Error("server config ClientCAs should not be nil")
	}
}

func TestManager_SkipVerify(t *testing.T) {
	caCert, caKey := generateTestCA()
	cfg := DefaultConfig("example.com", "svc")
	cfg.SkipVerify = true

	mgr := NewManager(cfg)
	mgr.Initialize(caCert, caKey)

	clientCfg := mgr.ClientTLSConfig()
	if !clientCfg.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should be true")
	}
}

func TestConnectionInfo(t *testing.T) {
	info := &ConnectionInfo{
		TLSVersion:  tls.VersionTLS13,
		CipherSuite: tls.TLS_AES_128_GCM_SHA256,
		ServerName:  "test.local",
	}
	if info.TLSVersion != tls.VersionTLS13 {
		t.Error("TLSVersion mismatch")
	}
	if info.ServerName != "test.local" {
		t.Error("ServerName mismatch")
	}
}
