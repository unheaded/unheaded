// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package auth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeIssuer is an OIDC issuer on loopback with Ed25519 keys generated at
// run time.
type fakeIssuer struct {
	srv      *httptest.Server
	requests atomic.Int64
	mu       sync.Mutex
	keys     map[string]ed25519.PrivateKey // kid -> key
	served   map[string]bool               // kids in the JWKS
	issuer   string                        // issuer the discovery document claims ("" = srv.URL)
	jwksBody []byte                        // overrides the JWKS when set
}

func newFakeIssuer(t *testing.T) *fakeIssuer {
	t.Helper()
	fi := &fakeIssuer{keys: map[string]ed25519.PrivateKey{}, served: map[string]bool{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		fi.requests.Add(1)
		fi.mu.Lock()
		iss := fi.issuer
		fi.mu.Unlock()
		if iss == "" {
			iss = fi.srv.URL
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"issuer": iss, "jwks_uri": fi.srv.URL + "/jwks"})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		fi.requests.Add(1)
		fi.mu.Lock()
		defer fi.mu.Unlock()
		if fi.jwksBody != nil {
			_, _ = w.Write(fi.jwksBody)
			return
		}
		var keys []map[string]string
		// An RSA key a real IdP might also publish: skipped, not fatal.
		keys = append(keys, map[string]string{"kty": "RSA", "kid": "rsa-1", "n": "AQAB", "e": "AQAB"})
		for kid, priv := range fi.keys {
			if !fi.served[kid] {
				continue
			}
			keys = append(keys, map[string]string{
				"kty": "OKP", "crv": "Ed25519", "kid": kid, "use": "sig", "alg": "EdDSA",
				"x": base64.RawURLEncoding.EncodeToString(priv.Public().(ed25519.PublicKey)),
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": keys})
	})
	fi.srv = httptest.NewServer(mux)
	t.Cleanup(fi.srv.Close)
	return fi
}

// addKey creates a key; served controls whether the JWKS publishes it.
func (fi *fakeIssuer) addKey(t *testing.T, served bool) string {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	kid := KeyID(pub)
	fi.mu.Lock()
	fi.keys[kid], fi.served[kid] = priv, served
	fi.mu.Unlock()
	return kid
}

func (fi *fakeIssuer) token(t *testing.T, kid, aud string, roles ...string) string {
	t.Helper()
	fi.mu.Lock()
	priv := fi.keys[kid]
	fi.mu.Unlock()
	now := time.Now()
	tok, err := SignEdDSA(priv, kid, &JWTClaims{
		Sub: "stevie", Iss: fi.srv.URL, Aud: aud, Iat: now.Unix(), Exp: now.Add(15 * time.Minute).Unix(), Roles: roles,
	})
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func newJWKS(t *testing.T, fi *fakeIssuer) *JWKSVerifier {
	t.Helper()
	v, err := NewJWKSVerifier(fi.srv.URL, "cuirass", nil, time.Hour)
	if err != nil {
		t.Fatalf("NewJWKSVerifier: %v", err)
	}
	return v
}

func TestJWKSVerifier_VerifiesAndMapsRoles(t *testing.T) {
	fi := newFakeIssuer(t)
	kid := fi.addKey(t, true)
	v := newJWKS(t, fi)
	id, err := v.Authenticate(context.Background(), bearerReq(fi.token(t, kid, "cuirass", RoleOperator)))
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if id.Subject != "stevie" || !id.HasRole(RoleOperator) || id.Extra["auth_method"] != "jwks" {
		t.Errorf("identity %+v", id)
	}
}

func TestJWKSVerifier_CacheTTL(t *testing.T) {
	fi := newFakeIssuer(t)
	kid := fi.addKey(t, true)
	v := newJWKS(t, fi)
	tok := fi.token(t, kid, "cuirass")
	if _, err := v.Authenticate(context.Background(), bearerReq(tok)); err != nil {
		t.Fatal(err)
	}
	n := fi.requests.Load()
	if n != 2 {
		t.Errorf("first verify made %d requests, want 2 (discovery + jwks)", n)
	}
	if _, err := v.Authenticate(context.Background(), bearerReq(tok)); err != nil {
		t.Fatal(err)
	}
	if fi.requests.Load() != n {
		t.Errorf("second verify inside the TTL made an HTTP call")
	}
}

func TestJWKSVerifier_UnknownKidRefetchesOnce(t *testing.T) {
	fi := newFakeIssuer(t)
	kid := fi.addKey(t, true)
	v := newJWKS(t, fi)
	if _, err := v.Authenticate(context.Background(), bearerReq(fi.token(t, kid, "cuirass"))); err != nil {
		t.Fatal(err)
	}
	before := fi.requests.Load()

	// A key the issuer rotated in after our fetch: one refetch finds it.
	rotated := fi.addKey(t, true)
	if _, err := v.Authenticate(context.Background(), bearerReq(fi.token(t, rotated, "cuirass"))); err != nil {
		t.Fatalf("rotated-in key: %v", err)
	}
	if got := fi.requests.Load() - before; got != 2 {
		t.Errorf("unknown kid made %d requests, want one refetch (2)", got)
	}

	// A kid the issuer never publishes: refused, and an immediate repeat
	// does not hammer the issuer.
	v2 := newJWKS(t, fi)
	if _, err := v2.Authenticate(context.Background(), bearerReq(fi.token(t, kid, "cuirass"))); err != nil {
		t.Fatal(err)
	}
	ghost := fi.addKey(t, false)
	before = fi.requests.Load()
	if _, err := v2.Authenticate(context.Background(), bearerReq(fi.token(t, ghost, "cuirass"))); !errors.Is(err, ErrUnknownKey) {
		t.Fatalf("unpublished kid: err %v, want ErrUnknownKey", err)
	}
	if got := fi.requests.Load() - before; got != 2 {
		t.Errorf("unpublished kid made %d requests, want one refetch (2)", got)
	}
	before = fi.requests.Load()
	if _, err := v2.Authenticate(context.Background(), bearerReq(fi.token(t, ghost, "cuirass"))); !errors.Is(err, ErrUnknownKey) {
		t.Fatalf("repeat: err %v", err)
	}
	if fi.requests.Load() != before {
		t.Error("a second unknown kid within the refetch interval refetched again")
	}
}

func TestJWKSVerifier_RefusesWrongAudienceIssuerAlg(t *testing.T) {
	fi := newFakeIssuer(t)
	kid := fi.addKey(t, true)
	v := newJWKS(t, fi)
	if _, err := v.Authenticate(context.Background(), bearerReq(fi.token(t, kid, "sophia"))); !errors.Is(err, ErrForbidden) {
		t.Errorf("wrong aud: %v", err)
	}

	fi.mu.Lock()
	priv := fi.keys[kid]
	fi.mu.Unlock()
	now := time.Now()
	c := &JWTClaims{Sub: "x", Iss: "https://evil.example", Aud: "cuirass", Iat: now.Unix(), Exp: now.Add(time.Minute).Unix()}
	if _, err := v.Authenticate(context.Background(), bearerReq(craft(t, map[string]any{"alg": "EdDSA", "kid": kid}, c,
		func(in []byte) []byte { return ed25519.Sign(priv, in) }))); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("wrong iss: %v", err)
	}
	c.Iss = fi.srv.URL
	pub := priv.Public().(ed25519.PublicKey)
	if _, err := v.Authenticate(context.Background(), bearerReq(craft(t, map[string]any{"alg": "HS256", "kid": kid}, c,
		func(in []byte) []byte { return computeHMACSHA256(pub, in) }))); !errors.Is(err, ErrAlgMismatch) {
		t.Errorf("HS256 keyed with the JWKS public key: %v", err)
	}
}

// OIDC providers may send aud as an array.
func TestJWKSVerifier_AudienceArray(t *testing.T) {
	fi := newFakeIssuer(t)
	kid := fi.addKey(t, true)
	v := newJWKS(t, fi)
	fi.mu.Lock()
	priv := fi.keys[kid]
	fi.mu.Unlock()
	now := time.Now()
	sign := func(aud any) string {
		h, _ := json.Marshal(map[string]string{"alg": "EdDSA", "kid": kid})
		p, _ := json.Marshal(map[string]any{"sub": "u", "iss": fi.srv.URL, "aud": aud, "iat": now.Unix(), "exp": now.Add(time.Minute).Unix()})
		in := base64.RawURLEncoding.EncodeToString(h) + "." + base64.RawURLEncoding.EncodeToString(p)
		return in + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(priv, []byte(in)))
	}
	if _, err := v.Authenticate(context.Background(), bearerReq(sign([]string{"dashboard", "cuirass"}))); err != nil {
		t.Errorf("aud array containing us: %v", err)
	}
	if _, err := v.Authenticate(context.Background(), bearerReq(sign([]string{"dashboard"}))); !errors.Is(err, ErrForbidden) {
		t.Errorf("aud array without us: %v", err)
	}
	if _, err := v.Authenticate(context.Background(), bearerReq(sign(nil))); err == nil {
		t.Error("no aud accepted")
	}
}

func TestJWKSVerifier_DiscoveryIssuerMismatch(t *testing.T) {
	fi := newFakeIssuer(t)
	kid := fi.addKey(t, true)
	fi.mu.Lock()
	fi.issuer = "https://someone-else.example"
	fi.mu.Unlock()
	v := newJWKS(t, fi)
	if _, err := v.Authenticate(context.Background(), bearerReq(fi.token(t, kid, "cuirass"))); err == nil {
		t.Fatal("a discovery document naming another issuer was trusted")
	}
}

func TestJWKSVerifier_OversizeDocumentRefused(t *testing.T) {
	fi := newFakeIssuer(t)
	kid := fi.addKey(t, true)
	fi.mu.Lock()
	fi.jwksBody = []byte(`{"keys":[],"pad":"` + strings.Repeat("a", maxJWKSBytes) + `"}`)
	fi.mu.Unlock()
	v := newJWKS(t, fi)
	if _, err := v.Authenticate(context.Background(), bearerReq(fi.token(t, kid, "cuirass"))); err == nil {
		t.Fatal("oversize JWKS accepted")
	}
}

func TestNewJWKSVerifier_RefusesPlainHTTPOffLoopback(t *testing.T) {
	for _, u := range []string{
		"http://auth.example.com", "http://10.10.10.5:19020", "ftp://127.0.0.1", "https://user:pw@auth.example.com",
		"", "not a url", "https://",
	} {
		if _, err := NewJWKSVerifier(u, "cuirass", nil, 0); err == nil {
			t.Errorf("%q accepted", u)
		}
	}
	for _, u := range []string{"https://auth.example.com", "http://127.0.0.1:19020", "http://localhost:19020", "http://[::1]:19020"} {
		if _, err := NewJWKSVerifier(u, "cuirass", nil, 0); err != nil {
			t.Errorf("%q refused: %v", u, err)
		}
	}
	if _, err := NewJWKSVerifier("https://auth.example.com", "", nil, 0); err == nil {
		t.Error("empty audience accepted")
	}
}

func TestJWKSVerifier_IssuerDownNoKeys(t *testing.T) {
	fi := newFakeIssuer(t)
	kid := fi.addKey(t, true)
	tok := fi.token(t, kid, "cuirass")
	v := newJWKS(t, fi)
	fi.srv.Close()
	if _, err := v.Authenticate(context.Background(), bearerReq(tok)); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("issuer down: err %v, want 401", err)
	}
}
