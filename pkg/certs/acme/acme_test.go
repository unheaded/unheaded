// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package acme

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.DirectoryURL != "https://acme-v02.api.letsencrypt.org/directory" {
		t.Errorf("DirectoryURL = %q", cfg.DirectoryURL)
	}
	if cfg.ChallengePort != 80 {
		t.Errorf("ChallengePort = %d, want 80", cfg.ChallengePort)
	}
	if cfg.AcceptTOS {
		t.Error("AcceptTOS should default to false")
	}
}

func TestStagingConfig(t *testing.T) {
	cfg := StagingConfig()
	if cfg.DirectoryURL != "https://acme-staging-v02.api.letsencrypt.org/directory" {
		t.Errorf("DirectoryURL = %q", cfg.DirectoryURL)
	}
}

func TestNewClient_NilConfig(t *testing.T) {
	client, err := NewClient(nil)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if client.config.DirectoryURL != "https://acme-v02.api.letsencrypt.org/directory" {
		t.Error("expected default config when nil passed")
	}
}

func TestNewClient(t *testing.T) {
	cfg := &Config{DirectoryURL: "https://custom.example.com"}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if client.config.DirectoryURL != "https://custom.example.com" {
		t.Error("expected custom config")
	}
}

func TestInitialize_GeneratesKey(t *testing.T) {
	client, _ := NewClient(nil)
	ctx := context.Background()

	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	if client.accountKey == nil {
		t.Error("expected accountKey to be set")
	}
	if !client.initialized {
		t.Error("expected initialized = true")
	}

	// Second init should be no-op
	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("second Initialize: %v", err)
	}
}

func TestInitialize_LoadsKeyFromFile(t *testing.T) {
	// Generate a test key
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	// Write PEM file
	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("MarshalECPrivateKey: %v", err)
	}
	pemBlock := &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes}
	keyPath := filepath.Join(t.TempDir(), "acme.key")
	f, _ := os.Create(keyPath)
	pem.Encode(f, pemBlock)
	f.Close()

	client, _ := NewClient(&Config{KeyFile: keyPath})
	if err := client.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize with key file: %v", err)
	}

	if client.accountKey == nil {
		t.Error("expected accountKey to be loaded from file")
	}
}

func TestInitialize_KeyFileNotFound(t *testing.T) {
	client, _ := NewClient(&Config{KeyFile: "/nonexistent/key.pem"})
	err := client.Initialize(context.Background())
	if err == nil {
		t.Error("expected error for missing key file")
	}
}

func TestLoadKeyFromFile_NoPEMBlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.pem")
	os.WriteFile(path, []byte("not a PEM file"), 0600)

	_, err := loadKeyFromFile(path)
	if err == nil {
		t.Error("expected error for non-PEM data")
	}
}

func TestLoadKeyFromFile_InvalidKeyData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.pem")
	block := &pem.Block{Type: "EC PRIVATE KEY", Bytes: []byte("invalid")}
	data := pem.EncodeToMemory(block)
	os.WriteFile(path, data, 0600)

	_, err := loadKeyFromFile(path)
	if err == nil {
		t.Error("expected error for invalid key data")
	}
}

func TestLoadKeyFromFile_PKCS8(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	keyBytes, _ := x509.MarshalPKCS8PrivateKey(key)
	block := &pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes}
	path := filepath.Join(t.TempDir(), "pkcs8.pem")
	os.WriteFile(path, pem.EncodeToMemory(block), 0600)

	loaded, err := loadKeyFromFile(path)
	if err != nil {
		t.Fatalf("loadKeyFromFile PKCS8: %v", err)
	}
	if loaded == nil {
		t.Error("expected non-nil key")
	}
}

func TestRequestCertificate(t *testing.T) {
	client, _ := NewClient(nil)
	ctx := context.Background()

	cert, err := client.RequestCertificate(ctx, []string{"example.com"})
	if err != nil {
		t.Fatalf("RequestCertificate: %v", err)
	}
	if cert == nil {
		t.Fatal("expected non-nil certificate")
	}
	if len(cert.Domains) != 1 || cert.Domains[0] != "example.com" {
		t.Errorf("Domains = %v, want [example.com]", cert.Domains)
	}
}
