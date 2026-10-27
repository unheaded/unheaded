// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

// Package authtest builds the key material a service's auth end-to-end test
// needs (S-IRON-GATE Phase 6): per-identity Ed25519 keys in a temp dir, a
// trust.d holding the public halves of the trusted ones, and a second key
// for any name that trust.d does not hold. Tests only; nothing here is
// used by a binary.
package authtest

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"unheaded/pkg/auth"
)

// Fixture is one test's keys and trust directory.
type Fixture struct {
	t        testing.TB
	Keys     string // <name>.key / <name>.pub for every identity
	Trust    string // trust.d: <name>.pub of the trusted identities
	stranger string // keys with trusted names that trust.d does not hold
}

// New generates a key pair for service and each of trusted, and files the
// public halves of trusted (not of service: nobody calls a service as
// itself) in the trust directory.
func New(t testing.TB, service string, trusted ...string) *Fixture {
	t.Helper()
	f := &Fixture{t: t, Keys: t.TempDir(), Trust: t.TempDir(), stranger: t.TempDir()}
	if _, err := auth.GenerateKeyPair(f.Keys, service); err != nil {
		t.Fatal(err)
	}
	for _, n := range trusted {
		if _, err := auth.GenerateKeyPair(f.Keys, n); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(f.Keys, n+".pub"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(f.Trust, n+".pub"), b, 0o444); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

// Config is the ServiceAuthConfig of service with auth on, this fixture's
// key and trust.d, and the caller allowlist.
func (f *Fixture) Config(service string, allow ...string) auth.ServiceAuthConfig {
	return auth.ServiceAuthConfig{
		Enabled:      true,
		ServiceName:  service,
		KeyFile:      filepath.Join(f.Keys, service+".key"),
		TrustDir:     f.Trust,
		AllowCallers: allow,
	}
}

// Minter is caller's token minter from this fixture's key.
func (f *Fixture) Minter(caller string) *auth.ServiceTokenMinter {
	f.t.Helper()
	m, err := auth.LoadServiceTokenMinter(caller, filepath.Join(f.Keys, caller+".key"))
	if err != nil {
		f.t.Fatal(err)
	}
	return m
}

// Bearer is an Authorization header value: a token minted by caller for aud.
func (f *Fixture) Bearer(caller, aud string) string {
	f.t.Helper()
	h, err := f.Minter(caller).AuthorizationHeader(aud)
	if err != nil {
		f.t.Fatal(err)
	}
	return h
}

// StrangerBearer is a token for aud that claims to be caller but is signed
// by a key trust.d does not hold.
func (f *Fixture) StrangerBearer(caller, aud string) string {
	f.t.Helper()
	keyFile := filepath.Join(f.stranger, caller+".key")
	if _, err := os.Stat(keyFile); err != nil {
		if _, err := auth.GenerateKeyPair(f.stranger, caller); err != nil {
			f.t.Fatal(err)
		}
	}
	m, err := auth.LoadServiceTokenMinter(caller, keyFile)
	if err != nil {
		f.t.Fatal(err)
	}
	h, err := m.AuthorizationHeader(aud)
	if err != nil {
		f.t.Fatal(err)
	}
	return h
}

// Code sends method target (body may be empty) with the Authorization
// header authz (none when empty) and returns the status.
func Code(h http.Handler, method, target, body, authz string) int {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if authz != "" {
		req.Header.Set("Authorization", authz)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

// Authorized reports whether status is a response that got past auth: not
// 401 and not 403.
func Authorized(status int) bool {
	return status != http.StatusUnauthorized && status != http.StatusForbidden
}
