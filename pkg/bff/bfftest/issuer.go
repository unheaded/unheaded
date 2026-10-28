// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

// Package bfftest is an OIDC issuer on loopback shaped like unheaded-auth
// (discovery, JWKS, POST /login), for tests of a backend-for-frontend.
// Keys and passwords are generated at run time. Tests only.
package bfftest

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/subtle"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"unheaded/pkg/auth"
)

// Issuer is a test issuer. URL is its issuer URL (the tokens' iss).
type Issuer struct {
	URL string

	// Logins counts POST /login requests.
	Logins atomic.Int64
	// ForceStatus, when non-zero, answers every login with that status.
	ForceStatus atomic.Int64
	// Lifetime is the minted token lifetime (default 15 minutes).
	Lifetime time.Duration

	audiences []string

	mu    sync.Mutex
	priv  ed25519.PrivateKey
	kid   string
	users map[string]user
}

type user struct {
	password string
	roles    []string
}

// NewIssuer starts an issuer that mints tokens for the given audiences.
func NewIssuer(t testing.TB, audiences ...string) *Issuer {
	t.Helper()
	return NewIssuerTLS(t, nil, audiences...)
}

// NewIssuerTLS is NewIssuer served over TLS with cfg (nil: plain http).
func NewIssuerTLS(t testing.TB, cfg *tls.Config, audiences ...string) *Issuer {
	t.Helper()
	is := &Issuer{audiences: audiences, users: map[string]user{}, Lifetime: 15 * time.Minute}
	is.RotateKey(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"issuer": is.URL, "jwks_uri": is.URL + "/jwks.json"})
	})
	mux.HandleFunc("GET /jwks.json", func(w http.ResponseWriter, _ *http.Request) {
		is.mu.Lock()
		pub, ok := is.priv.Public().(ed25519.PublicKey)
		if !ok {
			is.mu.Unlock()
			http.Error(w, "signing key is not Ed25519", http.StatusInternalServerError)
			return
		}
		kid := is.kid
		is.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "OKP", "crv": "Ed25519", "kid": kid, "use": "sig", "alg": auth.AlgEdDSA,
			"x": base64.RawURLEncoding.EncodeToString(pub),
		}}})
	})
	mux.HandleFunc("POST /login", is.login)
	srv := httptest.NewUnstartedServer(mux)
	if cfg != nil {
		srv.TLS = cfg
		srv.StartTLS()
	} else {
		srv.Start()
	}
	t.Cleanup(srv.Close)
	is.URL = srv.URL
	return is
}

// RotateKey replaces the signing key; tokens signed by the old one no
// longer verify once a verifier refetches the JWKS.
func (is *Issuer) RotateKey(t testing.TB) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	is.mu.Lock()
	is.priv, is.kid = priv, auth.KeyID(pub)
	is.mu.Unlock()
}

// AddUser creates a user with roles and returns its random password.
func (is *Issuer) AddUser(t testing.TB, name string, roles ...string) string {
	t.Helper()
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	pw := base64.RawURLEncoding.EncodeToString(b)
	is.mu.Lock()
	is.users[name] = user{password: pw, roles: roles}
	is.mu.Unlock()
	return pw
}

// Token mints a token as the issuer would at login.
func (is *Issuer) Token(t testing.TB, sub, aud string, life time.Duration, roles ...string) string {
	t.Helper()
	tok, err := is.sign(sub, aud, life, roles)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func (is *Issuer) sign(sub, aud string, life time.Duration, roles []string) (string, error) {
	is.mu.Lock()
	priv, kid := is.priv, is.kid
	is.mu.Unlock()
	now := time.Now()
	return auth.SignEdDSA(priv, kid, &auth.JWTClaims{
		Sub: sub, Iss: is.URL, Aud: aud, Iat: now.Unix(), Nbf: now.Unix(), Exp: now.Add(life).Unix(), Roles: roles,
	})
}

func (is *Issuer) login(w http.ResponseWriter, r *http.Request) {
	is.Logins.Add(1)
	if st := is.ForceStatus.Load(); st != 0 {
		w.WriteHeader(int(st))
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Audience string `json:"audience"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	dec.DisallowUnknownFields()
	if r.Header.Get("Content-Type") != "application/json" || dec.Decode(&req) != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	aud := ""
	for _, a := range is.audiences {
		if a == req.Audience {
			aud = a
		}
	}
	if aud == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	is.mu.Lock()
	u, ok := is.users[req.Username]
	is.mu.Unlock()
	if !ok || subtle.ConstantTimeCompare([]byte(u.password), []byte(req.Password)) != 1 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_credentials"}`))
		return
	}
	tok, err := is.sign(req.Username, aud, is.Lifetime, u.roles)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"access_token": tok, "token_type": "Bearer", "expires_in": int(is.Lifetime.Seconds())})
}
