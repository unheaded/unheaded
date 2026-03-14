// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package mtls

import (
	"crypto/tls"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Config path helpers — exercise the override branches
// ---------------------------------------------------------------------------

func TestConfigPathOverrides(t *testing.T) {
	cfg := Config{
		PKIDir:      "/custom/pki",
		ServiceName: "svc",
		RootCAFile:  "/override/ca.crt",
		CertFile:    "/override/svc.crt",
		KeyFile:     "/override/svc.key",
	}

	if got := cfg.rootCAPath(); got != "/override/ca.crt" {
		t.Errorf("rootCAPath override: got %s, want /override/ca.crt", got)
	}
	if got := cfg.certPath(); got != "/override/svc.crt" {
		t.Errorf("certPath override: got %s, want /override/svc.crt", got)
	}
	if got := cfg.keyPath(); got != "/override/svc.key" {
		t.Errorf("keyPath override: got %s, want /override/svc.key", got)
	}
}

func TestConfigPathDefaults(t *testing.T) {
	cfg := Config{ServiceName: "foo"}

	if got := cfg.pkiDir(); got != "/etc/unheaded/pki" {
		t.Errorf("pkiDir default: got %s, want /etc/unheaded/pki", got)
	}
	if got := cfg.rootCAPath(); got != "/etc/unheaded/pki/root-ca.crt" {
		t.Errorf("rootCAPath default: got %s", got)
	}
	if got := cfg.certPath(); got != "/etc/unheaded/pki/foo.crt" {
		t.Errorf("certPath default: got %s", got)
	}
	if got := cfg.keyPath(); got != "/etc/unheaded/pki/foo.key" {
		t.Errorf("keyPath default: got %s", got)
	}
}

func TestConfigPKIDirCustom(t *testing.T) {
	cfg := Config{PKIDir: "/my/pki", ServiceName: "bar"}

	if got := cfg.pkiDir(); got != "/my/pki" {
		t.Errorf("pkiDir custom: got %s", got)
	}
	if got := cfg.rootCAPath(); got != "/my/pki/root-ca.crt" {
		t.Errorf("rootCAPath with custom pkiDir: got %s", got)
	}
}

// ---------------------------------------------------------------------------
// integration.go — SetupServiceTLS, SetupServiceClientTLS, ValidateCertificate
// ---------------------------------------------------------------------------

func TestSetupServiceTLS(t *testing.T) {
	dir := t.TempDir()

	caCert, caKey, err := GenerateRootCA(24 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := WritePKI(dir, caCert, caKey, []string{"gateway"}, 12*time.Hour); err != nil {
		t.Fatal(err)
	}

	tlsCfg, err := SetupServiceTLS("gateway", dir)
	if err != nil {
		t.Fatalf("SetupServiceTLS: %v", err)
	}
	if tlsCfg == nil {
		t.Fatal("SetupServiceTLS returned nil config")
	}
	if tlsCfg.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Errorf("expected RequireAndVerifyClientCert, got %v", tlsCfg.ClientAuth)
	}
	if tlsCfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("expected MinVersion TLS 1.2, got %d", tlsCfg.MinVersion)
	}
}

func TestSetupServiceClientTLS(t *testing.T) {
	dir := t.TempDir()

	caCert, caKey, err := GenerateRootCA(24 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := WritePKI(dir, caCert, caKey, []string{"wotan"}, 12*time.Hour); err != nil {
		t.Fatal(err)
	}

	tlsCfg, err := SetupServiceClientTLS("wotan", dir)
	if err != nil {
		t.Fatalf("SetupServiceClientTLS: %v", err)
	}
	if tlsCfg == nil {
		t.Fatal("SetupServiceClientTLS returned nil config")
	}
	if tlsCfg.RootCAs == nil {
		t.Error("expected RootCAs to be set")
	}
	if len(tlsCfg.Certificates) != 1 {
		t.Errorf("expected 1 certificate, got %d", len(tlsCfg.Certificates))
	}
}

func TestValidateCertificate(t *testing.T) {
	t.Run("empty path", func(t *testing.T) {
		err := ValidateCertificate("")
		if err == nil {
			t.Fatal("expected error for empty path")
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		err := ValidateCertificate("/no/such/file.crt")
		if err == nil {
			t.Fatal("expected error for nonexistent file")
		}
	})

	t.Run("empty file", func(t *testing.T) {
		f := filepath.Join(t.TempDir(), "empty.crt")
		if err := os.WriteFile(f, nil, 0644); err != nil {
			t.Fatal(err)
		}
		err := ValidateCertificate(f)
		if err == nil {
			t.Fatal("expected error for empty file")
		}
	})

	t.Run("valid file", func(t *testing.T) {
		caCert, _, err := GenerateRootCA(1 * time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		f := filepath.Join(t.TempDir(), "good.crt")
		if err := os.WriteFile(f, caCert, 0644); err != nil {
			t.Fatal(err)
		}
		if err := ValidateCertificate(f); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// init.go — InitPKI
// ---------------------------------------------------------------------------

func TestInitPKI(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pki")

	if err := InitPKI(dir); err != nil {
		t.Fatalf("InitPKI: %v", err)
	}

	// Verify root CA files exist.
	for _, f := range []string{"root-ca.crt", "root-ca.key"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}

	// Spot-check a few service cert files from DefaultServices.
	for _, svc := range []string{"gateway", "wotan", "monad", "sophia"} {
		crtPath := filepath.Join(dir, svc+".crt")
		keyPath := filepath.Join(dir, svc+".key")
		if _, err := os.Stat(crtPath); err != nil {
			t.Errorf("missing cert for %s: %v", svc, err)
		}
		if _, err := os.Stat(keyPath); err != nil {
			t.Errorf("missing key for %s: %v", svc, err)
		}
	}

	// Verify total count: each DefaultService gets .crt + .key, plus root-ca.crt + root-ca.key.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	expectedCount := 2 + len(DefaultServices)*2
	if len(entries) != expectedCount {
		t.Errorf("expected %d files in PKI dir, got %d", expectedCount, len(entries))
	}
}

// ---------------------------------------------------------------------------
// RotatingServerTLSConfig / RotatingClientTLSConfig
// ---------------------------------------------------------------------------

func TestRotatingServerTLSConfig(t *testing.T) {
	dir := t.TempDir()

	caCert, caKey, err := GenerateRootCA(24 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := WritePKI(dir, caCert, caKey, []string{"srv"}, 12*time.Hour); err != nil {
		t.Fatal(err)
	}

	rotator, err := NewCertRotator(Config{PKIDir: dir, ServiceName: "srv"})
	if err != nil {
		t.Fatalf("NewCertRotator: %v", err)
	}

	tlsCfg := RotatingServerTLSConfig(rotator)
	if tlsCfg == nil {
		t.Fatal("RotatingServerTLSConfig returned nil")
	}
	if tlsCfg.GetCertificate == nil {
		t.Error("GetCertificate callback not set")
	}
	if tlsCfg.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Errorf("expected RequireAndVerifyClientCert, got %v", tlsCfg.ClientAuth)
	}
	if tlsCfg.ClientCAs == nil {
		t.Error("ClientCAs not set")
	}
	if tlsCfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("expected MinVersion TLS 1.2, got %d", tlsCfg.MinVersion)
	}

	// Verify the callback returns a valid cert.
	cert, err := tlsCfg.GetCertificate(nil)
	if err != nil || cert == nil {
		t.Fatalf("GetCertificate callback: %v", err)
	}
}

func TestRotatingClientTLSConfig(t *testing.T) {
	dir := t.TempDir()

	caCert, caKey, err := GenerateRootCA(24 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := WritePKI(dir, caCert, caKey, []string{"cli"}, 12*time.Hour); err != nil {
		t.Fatal(err)
	}

	rotator, err := NewCertRotator(Config{PKIDir: dir, ServiceName: "cli"})
	if err != nil {
		t.Fatalf("NewCertRotator: %v", err)
	}

	tlsCfg := RotatingClientTLSConfig(rotator)
	if tlsCfg == nil {
		t.Fatal("RotatingClientTLSConfig returned nil")
	}
	if tlsCfg.GetClientCertificate == nil {
		t.Error("GetClientCertificate callback not set")
	}
	if tlsCfg.RootCAs == nil {
		t.Error("RootCAs not set")
	}
	if tlsCfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("expected MinVersion TLS 1.2, got %d", tlsCfg.MinVersion)
	}

	// Verify the callback returns a valid cert.
	cert, err := tlsCfg.GetClientCertificate(nil)
	if err != nil || cert == nil {
		t.Fatalf("GetClientCertificate callback: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Error paths — ServerTLSConfig, ClientTLSConfig, loadCACertPool
// ---------------------------------------------------------------------------

func TestServerTLSConfigErrors(t *testing.T) {
	t.Run("missing cert files", func(t *testing.T) {
		cfg := Config{PKIDir: "/nonexistent", ServiceName: "x"}
		_, err := ServerTLSConfig(cfg)
		if err == nil {
			t.Fatal("expected error for missing cert files")
		}
	})

	t.Run("missing CA file", func(t *testing.T) {
		dir := t.TempDir()
		caCert, caKey, err := GenerateRootCA(1 * time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		// Write service cert but not root CA.
		svcCert, svcKey, err := GenerateServiceCert(caCert, caKey, "svc", 1*time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(dir, "svc.crt"), svcCert, 0644)
		os.WriteFile(filepath.Join(dir, "svc.key"), svcKey, 0600)

		cfg := Config{PKIDir: dir, ServiceName: "svc"}
		_, err = ServerTLSConfig(cfg)
		if err == nil {
			t.Fatal("expected error for missing CA")
		}
	})
}

func TestClientTLSConfigErrors(t *testing.T) {
	t.Run("missing cert files", func(t *testing.T) {
		cfg := Config{PKIDir: "/nonexistent", ServiceName: "x"}
		_, err := ClientTLSConfig(cfg)
		if err == nil {
			t.Fatal("expected error for missing cert files")
		}
	})

	t.Run("missing CA file", func(t *testing.T) {
		dir := t.TempDir()
		caCert, caKey, err := GenerateRootCA(1 * time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		svcCert, svcKey, err := GenerateServiceCert(caCert, caKey, "svc", 1*time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(dir, "svc.crt"), svcCert, 0644)
		os.WriteFile(filepath.Join(dir, "svc.key"), svcKey, 0600)

		cfg := Config{PKIDir: dir, ServiceName: "svc"}
		_, err = ClientTLSConfig(cfg)
		if err == nil {
			t.Fatal("expected error for missing CA")
		}
	})
}

func TestLoadCACertPoolInvalidPEM(t *testing.T) {
	f := filepath.Join(t.TempDir(), "bad-ca.crt")
	os.WriteFile(f, []byte("not-a-pem"), 0644)

	cfg := Config{RootCAFile: f, ServiceName: "svc"}
	// Use ServerTLSConfig to exercise loadCACertPool with invalid PEM.
	// We need a valid cert+key first.
	dir := t.TempDir()
	caCert, caKey, err := GenerateRootCA(1 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	svcCert, svcKey, err := GenerateServiceCert(caCert, caKey, "svc", 1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	certFile := filepath.Join(dir, "svc.crt")
	keyFile := filepath.Join(dir, "svc.key")
	os.WriteFile(certFile, svcCert, 0644)
	os.WriteFile(keyFile, svcKey, 0600)

	cfg.CertFile = certFile
	cfg.KeyFile = keyFile

	_, err = ServerTLSConfig(cfg)
	if err == nil {
		t.Fatal("expected error for invalid CA PEM")
	}
}

// ---------------------------------------------------------------------------
// GenerateServiceCert error paths
// ---------------------------------------------------------------------------

func TestGenerateServiceCertBadCACertPEM(t *testing.T) {
	_, _, err := GenerateServiceCert([]byte("bad"), []byte("bad"), "svc", 1*time.Hour)
	if err == nil {
		t.Fatal("expected error for bad CA cert PEM")
	}
}

func TestGenerateServiceCertBadCAKeyPEM(t *testing.T) {
	caCert, _, err := GenerateRootCA(1 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = GenerateServiceCert(caCert, []byte("bad"), "svc", 1*time.Hour)
	if err == nil {
		t.Fatal("expected error for bad CA key PEM")
	}
}

func TestGenerateServiceCertInvalidCAKey(t *testing.T) {
	caCert, _, err := GenerateRootCA(1 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	// Valid PEM envelope but not an EC private key — use the cert PEM as if it were a key.
	_, _, err = GenerateServiceCert(caCert, caCert, "svc", 1*time.Hour)
	if err == nil {
		t.Fatal("expected error for invalid CA key type")
	}
}

// ---------------------------------------------------------------------------
// NewCertRotator error path
// ---------------------------------------------------------------------------

func TestNewCertRotatorError(t *testing.T) {
	cfg := Config{PKIDir: "/nonexistent", ServiceName: "bad"}
	_, err := NewCertRotator(cfg)
	if err == nil {
		t.Fatal("expected error from NewCertRotator with missing files")
	}
}

// ---------------------------------------------------------------------------
// CertRotator.reload error path (cert removed after initial load)
// ---------------------------------------------------------------------------

func TestCertRotatorReloadError(t *testing.T) {
	dir := t.TempDir()

	caCert, caKey, err := GenerateRootCA(24 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := WritePKI(dir, caCert, caKey, []string{"rot"}, 12*time.Hour); err != nil {
		t.Fatal(err)
	}

	rotator, err := NewCertRotator(Config{PKIDir: dir, ServiceName: "rot"})
	if err != nil {
		t.Fatalf("NewCertRotator: %v", err)
	}

	// Remove the cert file to force a reload error.
	os.Remove(filepath.Join(dir, "rot.crt"))

	err = rotator.reload()
	if err == nil {
		t.Fatal("expected error after removing cert file")
	}

	// Remove the CA file (restore cert first) to force the CA reload error path.
	svcCert, svcKey, _ := GenerateServiceCert(caCert, caKey, "rot", 12*time.Hour)
	os.WriteFile(filepath.Join(dir, "rot.crt"), svcCert, 0644)
	os.WriteFile(filepath.Join(dir, "rot.key"), svcKey, 0600)
	os.Remove(filepath.Join(dir, "root-ca.crt"))

	err = rotator.reload()
	if err == nil {
		t.Fatal("expected error after removing CA file")
	}
}

// ---------------------------------------------------------------------------
// WritePKI error path — unwritable directory
// ---------------------------------------------------------------------------

func TestWritePKIErrorBadDir(t *testing.T) {
	caCert, caKey, err := GenerateRootCA(1 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	// Use /dev/null as the PKI dir — cannot create directories under it.
	err = WritePKI("/dev/null/impossible", caCert, caKey, []string{"x"}, 1*time.Hour)
	if err == nil {
		t.Fatal("expected error for unwritable dir")
	}
}
