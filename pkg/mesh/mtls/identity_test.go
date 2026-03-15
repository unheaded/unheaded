// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package mtls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"math/big"
	"testing"
	"time"
)

func TestWorkloadIdentity_IsValid(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	cert := generateTestCertWithSPIFFE("example.com", "default", "svc")

	t.Run("valid", func(t *testing.T) {
		wi := &WorkloadIdentity{
			Certificate: cert,
			PrivateKey:  key,
			ExpiresAt:   cert.NotAfter,
		}
		if !wi.IsValid() {
			t.Error("should be valid")
		}
	})

	t.Run("expired", func(t *testing.T) {
		expiredCert := generateTestCertWithTimeBounds(
			time.Now().Add(-2*time.Hour),
			time.Now().Add(-1*time.Hour),
		)
		wi := &WorkloadIdentity{
			Certificate: expiredCert,
			PrivateKey:  key,
			ExpiresAt:   expiredCert.NotAfter,
		}
		if wi.IsValid() {
			t.Error("should not be valid (expired)")
		}
	})
}

func TestWorkloadIdentity_TimeToExpiry(t *testing.T) {
	expires := time.Now().Add(1 * time.Hour)
	wi := &WorkloadIdentity{
		ExpiresAt: expires,
	}
	ttl := wi.TimeToExpiry()
	if ttl < 59*time.Minute || ttl > 61*time.Minute {
		t.Errorf("TimeToExpiry = %v, expected ~1h", ttl)
	}
}

func TestWorkloadIdentity_TLSCertificate(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	cert := generateTestCertWithSPIFFE("example.com", "default", "svc")

	wi := &WorkloadIdentity{
		Certificate: cert,
		PrivateKey:  key,
	}

	tlsCert := wi.TLSCertificate()
	if len(tlsCert.Certificate) != 1 {
		t.Errorf("expected 1 cert, got %d", len(tlsCert.Certificate))
	}
	if tlsCert.Leaf != cert {
		t.Error("Leaf mismatch")
	}
	if tlsCert.PrivateKey != key {
		t.Error("PrivateKey mismatch")
	}
}

func TestDefaultIdentityConfig(t *testing.T) {
	cfg := DefaultIdentityConfig("my-svc", "staging")
	if cfg.TrustDomain != DefaultTrustDomain {
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
	if !cfg.ValidatePeers {
		t.Error("ValidatePeers should be true")
	}
	if !cfg.RequireSPIFFEID {
		t.Error("RequireSPIFFEID should be true")
	}
	if len(cfg.AllowedTrustDomains) != 1 || cfg.AllowedTrustDomains[0] != DefaultTrustDomain {
		t.Errorf("AllowedTrustDomains = %v", cfg.AllowedTrustDomains)
	}
}

func TestNewIdentityManager(t *testing.T) {
	t.Run("with config", func(t *testing.T) {
		cfg := DefaultIdentityConfig("svc", "default")
		mgr := NewIdentityManager(cfg)
		if mgr == nil {
			t.Fatal("nil manager")
		}
		if mgr.config != cfg {
			t.Error("config mismatch")
		}
	})

	t.Run("nil config uses default", func(t *testing.T) {
		mgr := NewIdentityManager(nil)
		if mgr == nil {
			t.Fatal("nil manager")
		}
		if mgr.config.ServiceName != "unknown" {
			t.Errorf("ServiceName = %s, want unknown", mgr.config.ServiceName)
		}
	})
}

func TestIdentityManager_SetAndGetIdentity(t *testing.T) {
	mgr := NewIdentityManager(DefaultIdentityConfig("svc", "default"))

	if mgr.GetIdentity() != nil {
		t.Error("initial identity should be nil")
	}

	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	cert := generateTestCertWithSPIFFE("example.com", "default", "svc")
	id := NewSPIFFEID("example.com", "svc")

	identity := &WorkloadIdentity{
		ID:          id,
		Certificate: cert,
		PrivateKey:  key,
		ServiceName: "svc",
		Namespace:   "default",
		ExpiresAt:   cert.NotAfter,
	}

	mgr.SetIdentity(identity)
	got := mgr.GetIdentity()
	if got != identity {
		t.Error("GetIdentity returned wrong identity")
	}
}

func TestIdentityManager_Watch(t *testing.T) {
	mgr := NewIdentityManager(DefaultIdentityConfig("svc", "default"))
	ch := make(chan *WorkloadIdentity, 1)
	mgr.Watch(ch)

	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	cert := generateTestCertWithSPIFFE("example.com", "default", "svc")
	identity := &WorkloadIdentity{
		Certificate: cert,
		PrivateKey:  key,
	}

	mgr.SetIdentity(identity)

	select {
	case got := <-ch:
		if got != identity {
			t.Error("watcher received wrong identity")
		}
	case <-time.After(time.Second):
		t.Error("watcher did not receive identity")
	}
}

func TestIdentityManager_Watch_FullChannel(t *testing.T) {
	mgr := NewIdentityManager(DefaultIdentityConfig("svc", "default"))
	ch := make(chan *WorkloadIdentity) // unbuffered
	mgr.Watch(ch)

	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	cert := generateTestCertWithSPIFFE("example.com", "default", "svc")
	identity := &WorkloadIdentity{
		Certificate: cert,
		PrivateKey:  key,
	}

	// Should not block even though channel is full
	done := make(chan struct{})
	go func() {
		mgr.SetIdentity(identity)
		close(done)
	}()

	select {
	case <-done:
		// OK, did not block
	case <-time.After(time.Second):
		t.Error("SetIdentity blocked on full watcher channel")
	}
}

func TestIdentityManager_Authorize(t *testing.T) {
	mgr := NewIdentityManager(DefaultIdentityConfig("svc", "default"))

	t.Run("no policy allows all", func(t *testing.T) {
		id := NewSPIFFEID("example.com", "any")
		err := mgr.Authorize(id, "target-svc")
		if err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	})

	t.Run("policy blocks unauthorized", func(t *testing.T) {
		policy := &AuthorizationPolicy{
			Service:    "target-svc",
			AllowedIDs: []string{"spiffe://example.com/service/allowed"},
		}
		mgr.SetPolicy("target-svc", policy)

		allowed := NewSPIFFEID("example.com", "allowed")
		if err := mgr.Authorize(allowed, "target-svc"); err != nil {
			t.Errorf("expected authorized, got %v", err)
		}

		denied := NewSPIFFEID("example.com", "denied")
		if err := mgr.Authorize(denied, "target-svc"); err != ErrIdentityDenied {
			t.Errorf("expected ErrIdentityDenied, got %v", err)
		}
	})
}

func TestIdentityManager_GetPolicy(t *testing.T) {
	mgr := NewIdentityManager(DefaultIdentityConfig("svc", "default"))

	if mgr.GetPolicy("nonexistent") != nil {
		t.Error("expected nil for nonexistent policy")
	}

	policy := &AuthorizationPolicy{Service: "svc"}
	mgr.SetPolicy("svc", policy)
	if got := mgr.GetPolicy("svc"); got != policy {
		t.Error("GetPolicy mismatch")
	}
}

func TestIdentityManager_Close(t *testing.T) {
	mgr := NewIdentityManager(DefaultIdentityConfig("svc", "default"))
	ch := make(chan *WorkloadIdentity, 1)
	mgr.Watch(ch)

	err := mgr.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Double close should be safe
	err = mgr.Close()
	if err != nil {
		t.Fatalf("second Close failed: %v", err)
	}
}

func TestPolicyStore(t *testing.T) {
	store := NewPolicyStore()

	t.Run("empty store", func(t *testing.T) {
		if store.GetPolicy("svc") != nil {
			t.Error("expected nil for empty store")
		}
		policies := store.ListPolicies()
		if len(policies) != 0 {
			t.Errorf("expected empty list, got %d", len(policies))
		}
	})

	t.Run("set and get", func(t *testing.T) {
		policy := &AuthorizationPolicy{Service: "svc-a"}
		store.SetPolicy("svc-a", policy)
		if got := store.GetPolicy("svc-a"); got != policy {
			t.Error("GetPolicy mismatch")
		}
	})

	t.Run("list", func(t *testing.T) {
		store.SetPolicy("svc-b", &AuthorizationPolicy{Service: "svc-b"})
		policies := store.ListPolicies()
		if len(policies) < 2 {
			t.Errorf("expected at least 2 policies, got %d", len(policies))
		}
	})

	t.Run("delete", func(t *testing.T) {
		store.DeletePolicy("svc-a")
		if store.GetPolicy("svc-a") != nil {
			t.Error("policy should be deleted")
		}
	})
}

func TestServiceMatcher(t *testing.T) {
	t.Run("no patterns matches all", func(t *testing.T) {
		m := NewServiceMatcher()
		id := NewSPIFFEID("example.com", "any")
		if !m.Matches(id) {
			t.Error("empty matcher should match all")
		}
	})

	t.Run("matching pattern", func(t *testing.T) {
		m := NewServiceMatcher("spiffe://example.com/*")
		id := NewSPIFFEIDFromNamespace("example.com", "default", "svc")
		if !m.Matches(id) {
			t.Error("should match")
		}
	})

	t.Run("non-matching pattern", func(t *testing.T) {
		m := NewServiceMatcher("spiffe://other.com/*")
		id := NewSPIFFEIDFromNamespace("example.com", "default", "svc")
		if m.Matches(id) {
			t.Error("should not match")
		}
	})

	t.Run("add pattern", func(t *testing.T) {
		m := NewServiceMatcher("spiffe://other.com/*")
		id := NewSPIFFEIDFromNamespace("example.com", "default", "svc")
		if m.Matches(id) {
			t.Error("should not match initially")
		}
		m.AddPattern("spiffe://example.com/*")
		if !m.Matches(id) {
			t.Error("should match after adding pattern")
		}
	})
}

func TestNewIdentityValidator(t *testing.T) {
	v := NewIdentityValidator([]string{"example.com"})
	if v == nil {
		t.Fatal("nil validator")
	}
	if !v.requireSPIFFE {
		t.Error("requireSPIFFE should default to true")
	}
	if len(v.allowedServices) != 0 {
		t.Error("allowedServices should be empty")
	}
	if len(v.deniedServices) != 0 {
		t.Error("deniedServices should be empty")
	}
}

func TestIdentityValidator_AllowDenyService(t *testing.T) {
	v := NewIdentityValidator([]string{"example.com"})
	v.AllowService("spiffe://example.com/service/frontend")
	v.DenyService("spiffe://example.com/service/blocked")

	if len(v.allowedServices) != 1 {
		t.Errorf("allowedServices = %d, want 1", len(v.allowedServices))
	}
	if len(v.deniedServices) != 1 {
		t.Errorf("deniedServices = %d, want 1", len(v.deniedServices))
	}
}

func TestIdentityValidator_SetRequireSPIFFE(t *testing.T) {
	v := NewIdentityValidator([]string{"example.com"})
	v.SetRequireSPIFFE(false)
	if v.requireSPIFFE {
		t.Error("requireSPIFFE should be false")
	}
}

func TestIdentityValidator_ValidateCertificate(t *testing.T) {
	v := NewIdentityValidator([]string{"example.com"})

	t.Run("valid cert", func(t *testing.T) {
		cert := generateTestCertWithSPIFFE("example.com", "default", "svc")
		id, err := v.ValidateCertificate(cert)
		if err != nil {
			t.Fatalf("ValidateCertificate failed: %v", err)
		}
		if id.ServiceName() != "svc" {
			t.Errorf("ServiceName = %s", id.ServiceName())
		}
	})

	t.Run("wrong domain", func(t *testing.T) {
		cert := generateTestCertWithSPIFFE("other.com", "default", "svc")
		_, err := v.ValidateCertificate(cert)
		if err == nil {
			t.Error("expected error for wrong domain")
		}
	})
}

func TestZeroTrustEnforcer(t *testing.T) {
	enforcer := NewZeroTrustEnforcer([]string{"example.com"})
	if enforcer == nil {
		t.Fatal("nil enforcer")
	}

	t.Run("set policy", func(t *testing.T) {
		policy := &AuthorizationPolicy{
			Service:    "backend",
			AllowedIDs: []string{"spiffe://example.com/service/frontend"},
		}
		enforcer.SetPolicy("backend", policy)
	})

	t.Run("stats", func(t *testing.T) {
		allowed, denied := enforcer.Stats()
		// Initially should be zero
		if allowed != 0 || denied != 0 {
			t.Errorf("initial stats: allowed=%d denied=%d", allowed, denied)
		}
	})
}

func TestPeerInfo(t *testing.T) {
	info := &PeerInfo{
		CommonName:  "test",
		ServiceName: "svc",
		Namespace:   "default",
		TLSVersion:  tls.VersionTLS13,
	}
	if info.CommonName != "test" {
		t.Error("CommonName mismatch")
	}
}

// helper: generate a cert with explicit time bounds
func generateTestCertWithTimeBounds(notBefore, notAfter time.Time) *x509.Certificate {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	serial, _ := rand.Int(rand.Reader, big128())
	template := &x509.Certificate{
		SerialNumber:          serial,
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	cert, _ := x509.ParseCertificate(certDER)
	return cert
}

func big128() *big.Int {
	return new(big.Int).Lsh(big.NewInt(1), 128)
}
