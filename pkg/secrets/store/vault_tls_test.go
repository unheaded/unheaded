// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package store_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"unheaded/pkg/secrets/store"
)

func writePEM(t *testing.T, path, kind string, der []byte) {
	t.Helper()
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
}

// tlsVault is a TLS Vault health endpoint and the path of its CA.
func tlsVault(t *testing.T, mod func(*http.Server)) (*httptest.Server, string) {
	t.Helper()
	hs := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	if mod != nil {
		mod(hs.Config)
		hs.TLS = hs.Config.TLSConfig
	}
	hs.StartTLS()
	t.Cleanup(hs.Close)
	ca := filepath.Join(t.TempDir(), "ca.pem")
	writePEM(t, ca, "CERTIFICATE", hs.Certificate().Raw)
	return hs, ca
}

func clientPair(t *testing.T) (certPath, keyPath string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "vault-client"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certPath, keyPath = filepath.Join(dir, "client.pem"), filepath.Join(dir, "client-key.pem")
	writePEM(t, certPath, "CERTIFICATE", der)
	writePEM(t, keyPath, "EC PRIVATE KEY", keyDER)
	return certPath, keyPath
}

func newVaultTLS(addr string, cfg *store.VaultTLSConfig) (*store.VaultStore, error) {
	return store.NewVaultStore(store.VaultStoreConfig{Address: addr, Token: "test-token", TLSConfig: cfg, PollInterval: time.Hour})
}

// L2: the configured CA is the one the connection trusts.
func TestVaultTLS_CACertIsUsed(t *testing.T) {
	hs, ca := tlsVault(t, nil)
	if _, err := newVaultTLS(hs.URL, nil); err == nil {
		t.Fatal("connected with no CA configured: the server's certificate went unchecked")
	}
	vs, err := newVaultTLS(hs.URL, &store.VaultTLSConfig{CACert: ca})
	if err != nil {
		t.Fatalf("with its CA: %v", err)
	}
	_ = vs.Close()
}

// L2: the configured client certificate is the one presented.
func TestVaultTLS_ClientCertIsPresented(t *testing.T) {
	hs, ca := tlsVault(t, func(s *http.Server) {
		s.TLSConfig = &tls.Config{ClientAuth: tls.RequireAnyClientCert, MinVersion: tls.VersionTLS13}
	})
	if _, err := newVaultTLS(hs.URL, &store.VaultTLSConfig{CACert: ca}); err == nil {
		t.Fatal("connected without the client certificate the server requires")
	}
	cert, key := clientPair(t)
	vs, err := newVaultTLS(hs.URL, &store.VaultTLSConfig{CACert: ca, ClientCert: cert, ClientKey: key})
	if err != nil {
		t.Fatalf("with a client certificate: %v", err)
	}
	_ = vs.Close()
}

// L2: settings that cannot be honoured stop the store, never pass silently.
func TestVaultTLS_UnusableSettingsAreRefused(t *testing.T) {
	hs, ca := tlsVault(t, nil)
	cert, key := clientPair(t)
	for name, cfg := range map[string]*store.VaultTLSConfig{
		"insecure":         {CACert: ca, Insecure: true},
		"missing CA":       {CACert: filepath.Join(t.TempDir(), "absent.pem")},
		"cert without key": {CACert: ca, ClientCert: cert},
		"key without cert": {CACert: ca, ClientKey: key},
	} {
		if _, err := newVaultTLS(hs.URL, cfg); err == nil {
			t.Errorf("%s: store built", name)
		}
	}
}
