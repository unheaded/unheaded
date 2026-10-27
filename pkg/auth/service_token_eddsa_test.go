// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeClock is a settable clock for cache-expiry tests.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *fakeClock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newMinter(t *testing.T, f *eddsaFixture, name string, clock func() time.Time) *ServiceTokenMinter {
	t.Helper()
	m, err := NewServiceTokenMinter(name, f.priv[name], f.kid[name], clock)
	if err != nil {
		t.Fatalf("NewServiceTokenMinter: %v", err)
	}
	return m
}

func TestServiceTokenMinter_PerTargetCache(t *testing.T) {
	f := newEdDSAFixture(t, "cuirass")
	clk := &fakeClock{t: time.Unix(1_800_000_000, 0)}
	m := newMinter(t, f, "cuirass", clk.now)

	a, err := m.Mint("wotan")
	if err != nil {
		t.Fatal(err)
	}
	clk.add(4 * time.Minute)
	b, err := m.Mint("wotan")
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Error("second Mint within 4 min minted a new token; want the cached one")
	}
	other, err := m.Mint("sophia")
	if err != nil {
		t.Fatal(err)
	}
	if other == a {
		t.Error("one token served two audiences")
	}
	// exp is iat+5m; at exp-30s (4m30s after minting) the cache refreshes.
	clk.add(30 * time.Second)
	c, err := m.Mint("wotan")
	if err != nil {
		t.Fatal(err)
	}
	if c == a {
		t.Error("Mint at exp-30s returned the old token")
	}

	h, err := m.AuthorizationHeader("wotan")
	if err != nil || h != "Bearer "+c {
		t.Errorf("AuthorizationHeader = %q, %v", h, err)
	}
	if _, err := m.Mint(""); err == nil {
		t.Error("Mint with no target succeeded")
	}
}

func TestServiceTokenMinter_Claims(t *testing.T) {
	f := newEdDSAFixture(t, "cuirass")
	now := time.Now()
	m := newMinter(t, f, "cuirass", func() time.Time { return now })
	tok, err := m.Mint("wotan")
	if err != nil {
		t.Fatal(err)
	}
	payload, kid, owner, err := verifyEdDSA(tok, f.trust.LookupName)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if kid != f.kid["cuirass"] || owner != "cuirass" {
		t.Errorf("kid %q owner %q", kid, owner)
	}
	c := decodeClaims(t, payload)
	if c.Sub != "cuirass" || c.Aud != "wotan" || c.Iss != EdDSAServiceIssuer {
		t.Errorf("claims %+v", c)
	}
	if c.Iat != now.Unix() || c.Exp-c.Iat != int64((5*time.Minute).Seconds()) {
		t.Errorf("iat %d exp %d, want exp-iat = 300", c.Iat, c.Exp)
	}

	// What the minter makes, the verifier at the target takes.
	v := f.verifier(t, "wotan", "cuirass")
	if _, err := v.Authenticate(context.Background(), bearerReq(tok)); err != nil {
		t.Fatalf("minted token refused at its target: %v", err)
	}
}

func TestNewServiceTokenMinter_RefusesBadInput(t *testing.T) {
	f := newEdDSAFixture(t, "a", "b")
	for name, fn := range map[string]func() (*ServiceTokenMinter, error){
		"bad name": func() (*ServiceTokenMinter, error) { return NewServiceTokenMinter("A/B", f.priv["a"], f.kid["a"], nil) },
		"kid of b": func() (*ServiceTokenMinter, error) { return NewServiceTokenMinter("a", f.priv["a"], f.kid["b"], nil) },
		"short key": func() (*ServiceTokenMinter, error) {
			return NewServiceTokenMinter("a", f.priv["a"][:5], f.kid["a"], nil)
		},
		"empty name": func() (*ServiceTokenMinter, error) { return NewServiceTokenMinter("", f.priv["a"], f.kid["a"], nil) },
	} {
		if _, err := fn(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestLoadServiceTokenMinter_FromKeyFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := GenerateKeyPair(dir, "sword"); err != nil {
		t.Fatal(err)
	}
	m, err := LoadServiceTokenMinter("sword", filepath.Join(dir, "sword.key"))
	if err != nil {
		t.Fatal(err)
	}
	if tok, err := m.Mint("cuirass"); err != nil || strings.Count(tok, ".") != 2 {
		t.Fatalf("Mint: %q %v", tok, err)
	}
	if _, err := LoadServiceTokenMinter("sword", filepath.Join(dir, "missing.key")); err == nil {
		t.Error("missing key file accepted")
	}
}

func TestServiceTokenMinter_ConcurrentMint(t *testing.T) {
	f := newEdDSAFixture(t, "cuirass")
	m := newMinter(t, f, "cuirass", nil)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := m.Mint([]string{"wotan", "sophia", "monad"}[i%3]); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
}

func TestCallerAllowlist(t *testing.T) {
	f := newEdDSAFixture(t, "cuirass", "unheaded-cli", "sword")
	v := f.verifier(t, "wotan", "cuirass", "unheaded-cli")

	if _, err := v.Authenticate(context.Background(), bearerReq(f.sign(t, "cuirass", svcClaims("cuirass", "wotan")))); err != nil {
		t.Errorf("cuirass refused: %v", err)
	}
	_, err := v.Authenticate(context.Background(), bearerReq(f.sign(t, "sword", svcClaims("sword", "wotan"))))
	if !errors.Is(err, ErrCallerNotAllowed) || !errors.Is(err, ErrForbidden) {
		t.Errorf("sword: err %v, want ErrCallerNotAllowed (403)", err)
	}

	// Empty allowlist: deny all, even with a valid token.
	empty := f.verifier(t, "wotan")
	if _, err := empty.Authenticate(context.Background(), bearerReq(f.sign(t, "cuirass", svcClaims("cuirass", "wotan")))); !errors.Is(err, ErrCallerNotAllowed) {
		t.Errorf("empty allowlist: err %v, want ErrCallerNotAllowed", err)
	}
}

func TestCallerAllowlist_MiddlewareStatus(t *testing.T) {
	f := newEdDSAFixture(t, "cuirass", "sword")
	v := f.verifier(t, "wotan", "cuirass")
	// Behind a MultiAuthenticator with an API-key authenticator after it,
	// as SetupMiddleware builds it: the 403 must survive.
	ak, err := NewAPIKeyAuthenticator(APIKeyConfig{Keys: []string{"k-" + strings.Repeat("x", 8)}})
	if err != nil {
		t.Fatal(err)
	}
	h := Middleware(&MultiAuthenticator{Authenticators: []Authenticator{v, ak}})(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))

	for name, tc := range map[string]struct {
		tok  string
		want int
	}{
		"allowed":     {f.sign(t, "cuirass", svcClaims("cuirass", "wotan")), http.StatusOK},
		"not allowed": {f.sign(t, "sword", svcClaims("sword", "wotan")), http.StatusForbidden},
		"garbage":     {"x.y.z", http.StatusUnauthorized},
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, bearerReq(tc.tok))
		if rec.Code != tc.want {
			t.Errorf("%s: %d, want %d", name, rec.Code, tc.want)
		}
	}
}

func TestMultiAuthenticator_PrefersForbiddenOverUnauthenticated(t *testing.T) {
	m := &MultiAuthenticator{Authenticators: []Authenticator{
		&mockAuthenticator{err: ErrCallerNotAllowed},
		&mockAuthenticator{err: ErrUnauthenticated},
	}}
	_, err := m.Authenticate(context.Background(), httptest.NewRequest(http.MethodGet, "/", nil))
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("err %v, want the 403 to win over a later 401", err)
	}
	m = &MultiAuthenticator{Authenticators: []Authenticator{
		&mockAuthenticator{err: ErrForbidden},
		&mockAuthenticator{err: ErrRateLimited},
	}}
	if _, err := m.Authenticate(context.Background(), httptest.NewRequest(http.MethodGet, "/", nil)); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err %v, want 429 to win", err)
	}
}

func decodeClaims(t *testing.T, payload []byte) *JWTClaims {
	t.Helper()
	var c JWTClaims
	if err := json.Unmarshal(payload, &c); err != nil {
		t.Fatal(err)
	}
	return &c
}
