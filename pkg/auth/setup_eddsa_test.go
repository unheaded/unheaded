// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package auth

import (
	"crypto/rand"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
}

func codeOf(h http.Handler, r *http.Request) int {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec.Code
}

func TestLoadServiceAuthConfig_EdDSAEnv(t *testing.T) {
	cfg := LoadServiceAuthConfig("cuirass")
	if cfg.KeyFile != "/etc/unheaded/keys/cuirass.key" {
		t.Errorf("default KeyFile %q", cfg.KeyFile)
	}
	if cfg.TrustDir != "/etc/unheaded/trust.d" {
		t.Errorf("default TrustDir %q", cfg.TrustDir)
	}
	if cfg.Mode != "" || cfg.OIDCIssuerURL != "" || len(cfg.AllowCallers) != 0 {
		t.Errorf("unexpected defaults %+v", cfg)
	}

	t.Setenv("AUTH_MODE", "eddsa")
	t.Setenv("AUTH_KEY_FILE", "/run/secrets/cuirass.key")
	t.Setenv("AUTH_TRUST_DIR", "/srv/trust.d")
	t.Setenv("AUTH_ALLOW_CALLERS", "unheaded-cli, sword,,")
	t.Setenv("AUTH_OIDC_ISSUER_URL", "https://auth.example")
	cfg = LoadServiceAuthConfig("cuirass")
	if cfg.Mode != ModeEdDSA || cfg.KeyFile != "/run/secrets/cuirass.key" || cfg.TrustDir != "/srv/trust.d" ||
		cfg.OIDCIssuerURL != "https://auth.example" || strings.Join(cfg.AllowCallers, "|") != "unheaded-cli|sword" {
		t.Errorf("env not read: %+v", cfg)
	}
}

// ADR-103: the shared HMAC key retires. With auth on, AUTH_JWT_KEY is a
// configuration error, not a second way in.
func TestNewServiceMiddleware_SharedKeyForbidden(t *testing.T) {
	t.Setenv("AUTH_ENABLED", "true")
	t.Setenv("AUTH_JWT_KEY", "a-shared-secret-for-this-test")
	cfg := LoadServiceAuthConfig("wotan")
	if _, err := NewServiceMiddleware(cfg); !errors.Is(err, ErrSharedKeyForbidden) {
		t.Fatalf("err %v, want ErrSharedKeyForbidden", err)
	}
	// SetupMiddleware cannot return an error: it fails closed instead.
	h := WrapHandler(okHandler(), SetupMiddleware(cfg))
	tok, err := SignToken(cfg.JWTSigningKey, &JWTClaims{Sub: "x", Iss: cfg.Issuer, Aud: cfg.Audience, Exp: time.Now().Add(time.Minute).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	if got := codeOf(h, bearerReq(tok)); got != http.StatusUnauthorized {
		t.Errorf("HS256 token with AUTH_JWT_KEY forbidden: %d, want 401", got)
	}
	if got := codeOf(h, httptest.NewRequest(http.MethodGet, "/health", nil)); got != http.StatusOK {
		t.Errorf("/health %d", got)
	}
}

func TestNewServiceMiddleware_HS256TestModeOnlyInTests(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	cfg := ServiceAuthConfig{Enabled: true, Mode: ModeHS256Test, JWTSigningKey: key, ServiceName: "wotan", Issuer: "i", Audience: "a"}

	prev := hs256Allowed.Swap(false)
	_, err := NewServiceMiddleware(cfg)
	hs256Allowed.Store(prev)
	if err == nil {
		t.Fatal("AUTH_MODE=hs256-test accepted outside tests")
	}

	mw, err := NewServiceMiddleware(cfg)
	if err != nil {
		t.Fatalf("hs256-test under the test opt-in: %v", err)
	}
	tok, err := SignToken(key, &JWTClaims{Sub: "x", Iss: "i", Aud: "a", Exp: time.Now().Add(time.Minute).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	if got := codeOf(mw(okHandler()), bearerReq(tok)); got != http.StatusOK {
		t.Errorf("HS256 token in hs256-test mode: %d", got)
	}
}

func TestNewServiceMiddleware_UnknownMode(t *testing.T) {
	if _, err := NewServiceMiddleware(ServiceAuthConfig{Enabled: true, Mode: "rot13", APIKeys: []string{"k"}}); err == nil {
		t.Fatal("unknown AUTH_MODE accepted")
	}
}

func TestNewServiceMiddleware_Disabled(t *testing.T) {
	mw, err := NewServiceMiddleware(ServiceAuthConfig{})
	if mw != nil || err != nil {
		t.Fatalf("auth off: mw %v err %v, want nil, nil", mw != nil, err)
	}
}

func TestNewServiceMiddleware_NothingConfigured(t *testing.T) {
	_, err := NewServiceMiddleware(ServiceAuthConfig{Enabled: true, ServiceName: "x", TrustDir: filepath.Join(t.TempDir(), "absent")})
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("err %v, want ErrNoCredentials", err)
	}
}

func TestNewServiceMiddleware_EdDSAEndToEnd(t *testing.T) {
	f := newEdDSAFixture(t, "unheaded-cli", "sword")
	cfg := ServiceAuthConfig{
		Enabled:      true,
		ServiceName:  "cuirass",
		TrustDir:     f.trust.Path(),
		AllowCallers: []string{"unheaded-cli"},
	}
	mw, err := NewServiceMiddleware(cfg)
	if err != nil {
		t.Fatal(err)
	}
	h := mw(okHandler())
	post := func(tok string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/containers/web/stop", nil)
		if tok != "" {
			r.Header.Set("Authorization", "Bearer "+tok)
		}
		return r
	}
	for name, tc := range map[string]struct {
		r    *http.Request
		want int
	}{
		"no token":         {post(""), http.StatusUnauthorized},
		"allowed caller":   {post(f.sign(t, "unheaded-cli", svcClaims("unheaded-cli", "cuirass"))), http.StatusOK},
		"caller not on it": {post(f.sign(t, "sword", svcClaims("sword", "cuirass"))), http.StatusForbidden},
		"token for wotan":  {post(f.sign(t, "unheaded-cli", svcClaims("unheaded-cli", "wotan"))), http.StatusForbidden},
		"probe":            {httptest.NewRequest(http.MethodGet, "/ready", nil), http.StatusOK},
	} {
		if got := codeOf(h, tc.r); got != tc.want {
			t.Errorf("%s: %d, want %d", name, got, tc.want)
		}
	}
}

// Hosts without trust.d yet (before unheaded-keygen) keep their API keys.
func TestNewServiceMiddleware_MissingTrustDirKeepsAPIKeys(t *testing.T) {
	cfg := ServiceAuthConfig{Enabled: true, ServiceName: "sword", TrustDir: filepath.Join(t.TempDir(), "absent"), APIKeys: []string{"k-sword-test"}}
	mw, err := NewServiceMiddleware(cfg)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/v1/deploy", nil)
	r.Header.Set("X-API-Key", "k-sword-test")
	if got := codeOf(mw(okHandler()), r); got != http.StatusOK {
		t.Errorf("API key with no trust dir: %d", got)
	}
}

func TestNewServiceMiddleware_BrokenTrustDirRefuses(t *testing.T) {
	trust := t.TempDir()
	if _, err := GenerateKeyPair(trust, "oops"); err != nil { // leaves oops.key in trust.d
		t.Fatal(err)
	}
	cfg := ServiceAuthConfig{Enabled: true, ServiceName: "cuirass", TrustDir: trust, APIKeys: []string{"k"}}
	if _, err := NewServiceMiddleware(cfg); err == nil {
		t.Fatal("a trust dir holding a private key was accepted")
	}
	h := WrapHandler(okHandler(), SetupMiddleware(cfg))
	r := httptest.NewRequest(http.MethodPost, "/api/v1/x", nil)
	r.Header.Set("X-API-Key", "k")
	if got := codeOf(h, r); got != http.StatusUnauthorized {
		t.Errorf("SetupMiddleware with a broken trust dir: %d, want 401 (fail closed)", got)
	}
}

func TestNewServiceMiddleware_TrustDirNeedsServiceName(t *testing.T) {
	f := newEdDSAFixture(t, "a")
	if _, err := NewServiceMiddleware(ServiceAuthConfig{Enabled: true, TrustDir: f.trust.Path()}); err == nil {
		t.Fatal("EdDSA verifier built with no audience (service name)")
	}
}

func TestNewServiceMiddleware_UnusableAPIKeyFile(t *testing.T) {
	_, err := NewServiceMiddleware(ServiceAuthConfig{Enabled: true, ServiceName: "x", APIKeyFile: filepath.Join(t.TempDir(), "nope")})
	if err == nil || errors.Is(err, ErrNoCredentials) {
		t.Fatalf("err %v, want the key-file error", err)
	}
}

func TestNewServiceMiddleware_OIDC(t *testing.T) {
	fi := newFakeIssuer(t)
	kid := fi.addKey(t, true)
	mw, err := NewServiceMiddleware(ServiceAuthConfig{
		Enabled: true, ServiceName: "cuirass", TrustDir: filepath.Join(t.TempDir(), "absent"), OIDCIssuerURL: fi.srv.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := codeOf(mw(okHandler()), bearerReq(fi.token(t, kid, "cuirass", RoleOperator))); got != http.StatusOK {
		t.Errorf("user token: %d", got)
	}
	if _, err := NewServiceMiddleware(ServiceAuthConfig{Enabled: true, ServiceName: "c", OIDCIssuerURL: "http://auth.example.com"}); err == nil {
		t.Error("plain-http OIDC issuer off loopback accepted")
	}
}

func TestServiceAuthConfig_ServiceTokenMinter(t *testing.T) {
	dir := t.TempDir()
	if _, err := GenerateKeyPair(dir, "cuirass"); err != nil {
		t.Fatal(err)
	}
	cfg := ServiceAuthConfig{ServiceName: "cuirass", KeyFile: filepath.Join(dir, "cuirass.key")}
	m, err := cfg.ServiceTokenMinter()
	if err != nil {
		t.Fatal(err)
	}
	if m.Name() != "cuirass" {
		t.Errorf("minter name %q", m.Name())
	}
	if err := os.Chmod(cfg.KeyFile, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.ServiceTokenMinter(); !errors.Is(err, ErrKeyPermissions) {
		t.Errorf("world-readable key: %v", err)
	}
}
