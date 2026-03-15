// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package mtls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net/url"
	"testing"
	"time"
)

func TestNewSPIFFEID(t *testing.T) {
	id := NewSPIFFEID("example.com", "my-service")
	if id.TrustDomain != "example.com" {
		t.Errorf("expected trust domain example.com, got %s", id.TrustDomain)
	}
	if id.Path != "/service/my-service" {
		t.Errorf("expected path /service/my-service, got %s", id.Path)
	}
	if got := id.String(); got != "spiffe://example.com/service/my-service" {
		t.Errorf("String() = %s", got)
	}
}

func TestNewSPIFFEIDFromNamespace(t *testing.T) {
	id := NewSPIFFEIDFromNamespace("example.com", "production", "api-server")
	if got := id.String(); got != "spiffe://example.com/ns/production/sa/api-server" {
		t.Errorf("String() = %s", got)
	}
	if got := id.ServiceName(); got != "api-server" {
		t.Errorf("ServiceName() = %s, want api-server", got)
	}
	if got := id.Namespace(); got != "production" {
		t.Errorf("Namespace() = %s, want production", got)
	}
}

func TestSPIFFEID_ServiceName(t *testing.T) {
	tests := []struct {
		name string
		id   *SPIFFEID
		want string
	}{
		{
			name: "service format",
			id:   &SPIFFEID{TrustDomain: "d", Path: "/service/my-svc"},
			want: "my-svc",
		},
		{
			name: "namespace format",
			id:   &SPIFFEID{TrustDomain: "d", Path: "/ns/default/sa/my-svc"},
			want: "my-svc",
		},
		{
			name: "unknown format falls back to last segment",
			id:   &SPIFFEID{TrustDomain: "d", Path: "/custom/path/thing"},
			want: "thing",
		},
		{
			name: "empty path",
			id:   &SPIFFEID{TrustDomain: "d", Path: ""},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.id.ServiceName(); got != tt.want {
				t.Errorf("ServiceName() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestSPIFFEID_Namespace(t *testing.T) {
	tests := []struct {
		name string
		id   *SPIFFEID
		want string
	}{
		{
			name: "has namespace",
			id:   &SPIFFEID{TrustDomain: "d", Path: "/ns/production/sa/svc"},
			want: "production",
		},
		{
			name: "no namespace defaults",
			id:   &SPIFFEID{TrustDomain: "d", Path: "/service/svc"},
			want: "default",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.id.Namespace(); got != tt.want {
				t.Errorf("Namespace() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestParseSPIFFEID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		wantTD  string
		wantP   string
	}{
		{
			name:   "valid simple",
			input:  "spiffe://example.com/service/test",
			wantTD: "example.com",
			wantP:  "/service/test",
		},
		{
			name:   "valid namespaced",
			input:  "spiffe://example.com/ns/default/sa/frontend",
			wantTD: "example.com",
			wantP:  "/ns/default/sa/frontend",
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "wrong scheme",
			input:   "http://example.com/service/test",
			wantErr: true,
		},
		{
			name:    "no host",
			input:   "spiffe:///service/test",
			wantErr: true,
		},
		{
			name:    "no path",
			input:   "spiffe://example.com",
			wantErr: true,
		},
		{
			name:    "has query",
			input:   "spiffe://example.com/service/test?q=1",
			wantErr: true,
		},
		{
			name:    "has fragment",
			input:   "spiffe://example.com/service/test#frag",
			wantErr: true,
		},
		{
			name:    "has port",
			input:   "spiffe://example.com:8080/service/test",
			wantErr: true,
		},
		{
			name:    "has userinfo",
			input:   "spiffe://user@example.com/service/test",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := ParseSPIFFEID(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if id.TrustDomain != tt.wantTD {
				t.Errorf("TrustDomain = %s, want %s", id.TrustDomain, tt.wantTD)
			}
			if id.Path != tt.wantP {
				t.Errorf("Path = %s, want %s", id.Path, tt.wantP)
			}
		})
	}
}

func TestSPIFFEID_Matches(t *testing.T) {
	id := NewSPIFFEIDFromNamespace("example.com", "default", "api")

	tests := []struct {
		pattern string
		want    bool
	}{
		{"spiffe://example.com/*", true},
		{"spiffe://example.com/ns/default/*", true},
		{"spiffe://example.com/ns/default/sa/api", true},
		{"spiffe://other.com/*", false},
		{"spiffe://example.com/ns/staging/*", false},
		{"*", true},
	}
	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			if got := id.Matches(tt.pattern); got != tt.want {
				t.Errorf("Matches(%q) = %v, want %v", tt.pattern, got, tt.want)
			}
		})
	}
}

func TestMatchWildcard(t *testing.T) {
	tests := []struct {
		pattern string
		s       string
		want    bool
	}{
		{"*", "anything", true},
		{"*suffix", "has-suffix", true},
		{"*suffix", "no-match", false},
		{"prefix*", "prefix-and-more", true},
		{"prefix*", "no-match", false},
		{"exact", "exact", true},
		{"exact", "not-exact", false},
	}
	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.s, func(t *testing.T) {
			if got := matchWildcard(tt.pattern, tt.s); got != tt.want {
				t.Errorf("matchWildcard(%q, %q) = %v, want %v", tt.pattern, tt.s, got, tt.want)
			}
		})
	}
}

func TestSPIFFEVerifier_ExtractSPIFFEID(t *testing.T) {
	verifier := NewSPIFFEVerifier([]string{"example.com"})

	t.Run("cert with SPIFFE URI", func(t *testing.T) {
		cert := generateTestCertWithSPIFFE("example.com", "default", "svc")
		id, err := verifier.ExtractSPIFFEID(cert)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id.ServiceName() != "svc" {
			t.Errorf("ServiceName() = %s, want svc", id.ServiceName())
		}
	})

	t.Run("cert without SPIFFE URI", func(t *testing.T) {
		cert := generateTestCertNoSPIFFE()
		_, err := verifier.ExtractSPIFFEID(cert)
		if err != ErrSPIFFENotFound {
			t.Errorf("expected ErrSPIFFENotFound, got %v", err)
		}
	})
}

func TestSPIFFEVerifier_VerifyCertificate(t *testing.T) {
	t.Run("trusted domain", func(t *testing.T) {
		verifier := NewSPIFFEVerifier([]string{"example.com"})
		cert := generateTestCertWithSPIFFE("example.com", "default", "svc")
		id, err := verifier.VerifyCertificate(cert)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id.TrustDomain != "example.com" {
			t.Errorf("TrustDomain = %s", id.TrustDomain)
		}
	})

	t.Run("untrusted domain", func(t *testing.T) {
		verifier := NewSPIFFEVerifier([]string{"trusted.com"})
		cert := generateTestCertWithSPIFFE("untrusted.com", "default", "svc")
		_, err := verifier.VerifyCertificate(cert)
		if err != ErrTrustDomainMismatch {
			t.Errorf("expected ErrTrustDomainMismatch, got %v", err)
		}
	})

	t.Run("wildcard trust domain", func(t *testing.T) {
		verifier := NewSPIFFEVerifier([]string{"*"})
		cert := generateTestCertWithSPIFFE("any.com", "default", "svc")
		_, err := verifier.VerifyCertificate(cert)
		if err != nil {
			t.Fatalf("wildcard should accept any domain: %v", err)
		}
	})

	t.Run("allowed patterns", func(t *testing.T) {
		verifier := NewSPIFFEVerifier([]string{"example.com"})
		verifier.AllowID("spiffe://example.com/ns/production/sa/allowed")

		allowed := generateTestCertWithSPIFFE("example.com", "production", "allowed")
		_, err := verifier.VerifyCertificate(allowed)
		if err != nil {
			t.Errorf("expected allowed cert to pass: %v", err)
		}

		denied := generateTestCertWithSPIFFE("example.com", "production", "denied")
		_, err = verifier.VerifyCertificate(denied)
		if err != ErrInvalidSPIFFEID {
			t.Errorf("expected ErrInvalidSPIFFEID for denied cert, got %v", err)
		}
	})
}

func TestSPIFFEVerifier_VerifyPeerCertificate(t *testing.T) {
	verifier := NewSPIFFEVerifier([]string{"example.com"})

	t.Run("empty verified chains", func(t *testing.T) {
		err := verifier.VerifyPeerCertificate(nil, nil)
		if err != ErrSPIFFENotFound {
			t.Errorf("expected ErrSPIFFENotFound, got %v", err)
		}
	})

	t.Run("valid chain", func(t *testing.T) {
		cert := generateTestCertWithSPIFFE("example.com", "default", "svc")
		chains := [][]*x509.Certificate{{cert}}
		err := verifier.VerifyPeerCertificate(nil, chains)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestSPIFFEBundle_Extended(t *testing.T) {
	bundle := NewSPIFFEBundle("example.com")
	if bundle.TrustDomain != "example.com" {
		t.Errorf("TrustDomain = %s", bundle.TrustDomain)
	}
	if len(bundle.Roots) != 0 {
		t.Errorf("expected empty roots, got %d", len(bundle.Roots))
	}

	ca, _ := generateTestCA()
	bundle.AddRoot(ca)
	if len(bundle.Roots) != 1 {
		t.Errorf("expected 1 root, got %d", len(bundle.Roots))
	}

	pool := bundle.CertPool()
	if pool == nil {
		t.Fatal("CertPool() returned nil")
	}

	// Add another
	ca2, _ := generateTestCA()
	bundle.AddRoot(ca2)
	if len(bundle.Roots) != 2 {
		t.Errorf("expected 2 roots, got %d", len(bundle.Roots))
	}
}

func TestAuthorizationPolicy_IsAuthorized(t *testing.T) {
	tests := []struct {
		name   string
		policy AuthorizationPolicy
		id     *SPIFFEID
		want   bool
	}{
		{
			name:   "no lists allows all",
			policy: AuthorizationPolicy{Service: "svc"},
			id:     NewSPIFFEID("example.com", "any"),
			want:   true,
		},
		{
			name: "allowed list permits match",
			policy: AuthorizationPolicy{
				AllowedIDs: []string{"spiffe://example.com/service/frontend"},
			},
			id:   NewSPIFFEID("example.com", "frontend"),
			want: true,
		},
		{
			name: "allowed list blocks non-match",
			policy: AuthorizationPolicy{
				AllowedIDs: []string{"spiffe://example.com/service/frontend"},
			},
			id:   NewSPIFFEID("example.com", "backend"),
			want: false,
		},
		{
			name: "denied list blocks match",
			policy: AuthorizationPolicy{
				DeniedIDs: []string{"spiffe://example.com/service/blocked"},
			},
			id:   NewSPIFFEID("example.com", "blocked"),
			want: false,
		},
		{
			name: "denied list allows non-match",
			policy: AuthorizationPolicy{
				DeniedIDs: []string{"spiffe://example.com/service/blocked"},
			},
			id:   NewSPIFFEID("example.com", "ok"),
			want: true,
		},
		{
			name: "denied takes precedence over allowed",
			policy: AuthorizationPolicy{
				AllowedIDs: []string{"spiffe://example.com/*"},
				DeniedIDs:  []string{"spiffe://example.com/service/blocked"},
			},
			id:   NewSPIFFEID("example.com", "blocked"),
			want: false,
		},
		{
			name: "wildcard allowed",
			policy: AuthorizationPolicy{
				AllowedIDs: []string{"spiffe://example.com/*"},
			},
			id:   NewSPIFFEIDFromNamespace("example.com", "prod", "any"),
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.policy.IsAuthorized(tt.id); got != tt.want {
				t.Errorf("IsAuthorized() = %v, want %v", got, tt.want)
			}
		})
	}
}

// helper: cert with no SPIFFE URI SAN
func generateTestCertNoSPIFFE() *x509.Certificate {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "no-spiffe"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	cert, _ := x509.ParseCertificate(certDER)
	return cert
}

// helper: cert with a non-SPIFFE URI SAN (to ensure we skip non-spiffe URIs)
func generateTestCertWithNonSPIFFEURI() *x509.Certificate {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	u, _ := url.Parse("https://example.com/not-spiffe")
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "non-spiffe-uri"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		URIs:         []*url.URL{u},
	}
	certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	cert, _ := x509.ParseCertificate(certDER)
	return cert
}

func TestSPIFFEVerifier_ExtractSPIFFEID_NonSPIFFEURI(t *testing.T) {
	verifier := NewSPIFFEVerifier([]string{"*"})
	cert := generateTestCertWithNonSPIFFEURI()
	_, err := verifier.ExtractSPIFFEID(cert)
	if err != ErrSPIFFENotFound {
		t.Errorf("expected ErrSPIFFENotFound for non-SPIFFE URI cert, got %v", err)
	}
}
