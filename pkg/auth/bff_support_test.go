// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// What the backend-for-frontend (ADR-103 §4, S-IRON-GATE Steps 186-216)
// needs from pkg/auth.

func TestJWKSVerifier_PlainHTTPIssuerNeedsOptIn(t *testing.T) {
	const compose = "http://auth:19020"
	if _, err := NewJWKSVerifier(compose, "dashboard-backend", nil, 0); err == nil {
		t.Fatal("plain http to a non-loopback issuer accepted without the dev opt-in")
	}
	v, err := NewJWKSVerifier(compose, "dashboard-backend", nil, 0, WithPlainHTTPIssuer())
	if err != nil {
		t.Fatalf("with WithPlainHTTPIssuer: %v", err)
	}
	if err := v.checkURL("http://auth:19020/jwks.json"); err != nil {
		t.Fatalf("jwks_uri on the same plaintext issuer refused: %v", err)
	}
	if err := v.checkURL("ftp://auth/jwks"); err == nil {
		t.Fatal("the opt-in must still refuse other schemes")
	}
	if err := v.checkURL("http://user:pw@auth/jwks"); err == nil {
		t.Fatal("the opt-in must still refuse credentials in the URL")
	}
}

func TestDefaultServicePolicies_ViewerReadsOnly(t *testing.T) {
	h := RBACMiddleware(DefaultServicePolicies("svc"))(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	viewer := &Identity{Subject: "v", Roles: []string{RoleViewer}}
	for _, tc := range []struct {
		method string
		want   int
	}{{http.MethodGet, http.StatusOK}, {http.MethodPost, http.StatusForbidden}, {http.MethodDelete, http.StatusForbidden}} {
		req := httptest.NewRequest(tc.method, "/api/v1/things", nil)
		req = req.WithContext(context.WithValue(req.Context(), identityKey, viewer))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Errorf("viewer %s: got %d, want %d", tc.method, rec.Code, tc.want)
		}
	}
}

// staticAuthenticator accepts requests carrying header X-Test-User.
type staticAuthenticator struct{ roles []string }

func (s staticAuthenticator) Authenticate(_ context.Context, r *http.Request) (*Identity, error) {
	u := r.Header.Get("X-Test-User")
	if u == "" {
		return nil, ErrUnauthenticated
	}
	return &Identity{Subject: u, Roles: s.roles}, nil
}

func TestNewServiceChain_ExtraAuthenticators(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id := IdentityFromContext(r.Context()); id != nil {
			w.Header().Set("X-Sub", id.Subject)
		}
		w.WriteHeader(http.StatusOK)
	})
	// Auth on with nothing else configured: the extra authenticator alone
	// is enough credentials to start.
	cfg := ServiceAuthConfig{Enabled: true, ServiceName: "dash"}
	if _, err := NewServiceChain(cfg, ServiceRoutes{}); !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("no credentials and no extra: got %v, want ErrNoCredentials", err)
	}
	mw, err := NewServiceChain(cfg, ServiceRoutes{Authenticators: []Authenticator{staticAuthenticator{roles: []string{RoleOperator}}}})
	if err != nil || mw == nil {
		t.Fatalf("NewServiceChain with an extra authenticator: %v", err)
	}
	h := mw(ok)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/x", nil)
	req.Header.Set("X-Test-User", "dev")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Header().Get("X-Sub") != "dev" {
		t.Fatalf("extra authenticator's identity: code %d sub %q", rec.Code, rec.Header().Get("X-Sub"))
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/x", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no credential: got %d, want 401", rec.Code)
	}
	// A broken configuration still refuses even with an extra.
	bad := cfg
	bad.JWTSigningKey = []byte("k")
	if _, err := NewServiceChain(bad, ServiceRoutes{Authenticators: []Authenticator{staticAuthenticator{}}}); !errors.Is(err, ErrSharedKeyForbidden) {
		t.Fatalf("AUTH_JWT_KEY with an extra: got %v, want ErrSharedKeyForbidden", err)
	}
	// Auth off: no chain, extra or not.
	if mw, err := NewServiceChain(ServiceAuthConfig{}, ServiceRoutes{Authenticators: []Authenticator{staticAuthenticator{}}}); mw != nil || err != nil {
		t.Fatalf("auth off: got (%v, %v), want (nil, nil)", mw != nil, err)
	}
}

func TestMint_WithActor(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"dashboard-backend", "sword"} {
		if _, err := GenerateKeyPair(dir, n); err != nil {
			t.Fatal(err)
		}
	}
	trust := t.TempDir()
	copyPub(t, filepath.Join(dir, "dashboard-backend.pub"), filepath.Join(trust, "dashboard-backend.pub"))
	m, err := LoadServiceTokenMinter("dashboard-backend", filepath.Join(dir, "dashboard-backend.key"))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := m.Mint("sword")
	if err != nil {
		t.Fatal(err)
	}
	acted, err := m.Mint("sword", WithActor("dev"))
	if err != nil {
		t.Fatal(err)
	}
	if plain == acted {
		t.Fatal("a token on behalf of a user came from the plain token's cache entry")
	}
	other, err := m.Mint("sword", WithActor("stevie"))
	if err != nil {
		t.Fatal(err)
	}
	if other == acted {
		t.Fatal("two actors share one cached token")
	}
	again, _ := m.Mint("sword", WithActor("dev"))
	if again != acted {
		t.Fatal("the per-actor token is not cached")
	}
	if _, err := m.Mint("sword", WithActor("bad\nname")); err == nil {
		t.Fatal("an unprintable actor was signed")
	}
	if _, err := m.Mint("sword", WithActor(strings.Repeat("a", 300))); err == nil {
		t.Fatal("an over-long actor was signed")
	}

	td, err := NewTrustDir(trust)
	if err != nil {
		t.Fatal(err)
	}
	v, err := NewEdDSAVerifier(EdDSAVerifierConfig{Trust: td, Issuer: EdDSAServiceIssuer, Audience: "sword", AllowCallers: []string{"dashboard-backend"}})
	if err != nil {
		t.Fatal(err)
	}
	for tok, want := range map[string]string{acted: "dev", plain: ""} {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		id, err := v.Authenticate(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		if id.Subject != "dashboard-backend" || id.Extra["act"] != want {
			t.Fatalf("identity %q act %q, want dashboard-backend act %q", id.Subject, id.Extra["act"], want)
		}
	}
	hdr, err := m.AuthorizationHeader("sword", WithActor("dev"))
	if err != nil || hdr != "Bearer "+acted {
		t.Fatalf("AuthorizationHeader with actor: %v", err)
	}
}

func copyPub(t *testing.T, from, to string) {
	t.Helper()
	b, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(to, b, 0o444); err != nil {
		t.Fatal(err)
	}
}
