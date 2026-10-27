// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/auth"
)

// S-IRON-GATE Phase 5 (Steps 160-165): Cuirass with EdDSA service tokens.
// Reads stay open, writes need a token from a trusted, allowlisted caller,
// and an authenticated caller still needs a role that may write.
func TestAuthE2E_CuirassServiceTokens(t *testing.T) {
	keys, trust, stranger := t.TempDir(), t.TempDir(), t.TempDir()
	for _, n := range []string{"unheaded-daemon", "sword", "unheaded-cli", "kanban"} {
		if _, err := auth.GenerateKeyPair(keys, n); err != nil {
			t.Fatal(err)
		}
	}
	// trust.d: the callers' public keys (kanban is trusted but not allowlisted).
	for _, n := range []string{"sword", "unheaded-cli", "kanban"} {
		b, err := os.ReadFile(filepath.Join(keys, n+".pub"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(trust, n+".pub"), b, 0o444); err != nil {
			t.Fatal(err)
		}
	}
	// A key with an allowlisted name that trust.d does not hold.
	if _, err := auth.GenerateKeyPair(stranger, "sword"); err != nil {
		t.Fatal(err)
	}

	d := newTestDaemon(t)
	d.authEnabled = true
	mux := http.NewServeMux()
	d.registerHandlers(mux)
	h := d.wrapAuth(mux, auth.ServiceAuthConfig{
		Enabled:      true,
		ServiceName:  "unheaded-daemon",
		TrustDir:     trust,
		KeyFile:      filepath.Join(keys, "unheaded-daemon.key"),
		AllowCallers: []string{"sword", "unheaded-cli", "dashboard-backend"},
	})

	token := func(dir, name string) string {
		m, err := auth.LoadServiceTokenMinter(name, filepath.Join(dir, name+".key"))
		if err != nil {
			t.Fatal(err)
		}
		hdr, err := m.AuthorizationHeader("unheaded-daemon")
		if err != nil {
			t.Fatal(err)
		}
		return hdr
	}
	do := func(method, path, authz string) int {
		req := httptest.NewRequest(method, path, strings.NewReader(`{}`))
		if authz != "" {
			req.Header.Set("Authorization", authz)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	for _, p := range []string{"/health", "/ready", "/api/v1/info", "/api/v1/state"} {
		if c := do(http.MethodGet, p, ""); c != http.StatusOK {
			t.Errorf("GET %s anonymous: %d, want 200", p, c)
		}
	}
	if c := do(http.MethodPost, "/api/v1/reconcile", ""); c != http.StatusUnauthorized {
		t.Errorf("POST reconcile anonymous: %d, want 401", c)
	}
	if c := do(http.MethodPost, "/api/v1/reconcile", token(keys, "sword")); c == http.StatusUnauthorized || c == http.StatusForbidden {
		t.Errorf("POST reconcile as sword: %d, want accepted", c)
	}
	if c := do(http.MethodPost, "/api/v1/reconcile", token(keys, "unheaded-cli")); c == http.StatusUnauthorized || c == http.StatusForbidden {
		t.Errorf("POST reconcile as unheaded-cli: %d, want accepted", c)
	}
	if c := do(http.MethodPost, "/api/v1/reconcile", token(stranger, "sword")); c != http.StatusUnauthorized {
		t.Errorf("POST reconcile with a key trust.d lacks: %d, want 401", c)
	}
	if c := do(http.MethodPost, "/api/v1/reconcile", token(keys, "kanban")); c != http.StatusForbidden {
		t.Errorf("POST reconcile as trusted but not allowlisted kanban: %d, want 403", c)
	}
}

// An authenticated identity without a write role (an observer API key)
// may read the keyed endpoints but not write: RBAC, not just authentication.
func TestAuthE2E_ObserverCannotWrite(t *testing.T) {
	d := newTestDaemon(t)
	d.authEnabled = true
	mux := http.NewServeMux()
	d.registerHandlers(mux)
	path := filepath.Join(t.TempDir(), "apikeys.jsonl")
	store, err := auth.NewFileAPIKeyStore(path)
	if err != nil {
		t.Fatal(err)
	}
	key, _, err := store.Create(context.Background(), "watcher", auth.RoleObserver, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	h := d.wrapAuth(mux, auth.ServiceAuthConfig{
		Enabled: true, ServiceName: "unheaded-daemon", APIKeyStore: "file:" + path,
	})
	get := httptest.NewRequest(http.MethodGet, "/api/v1/containers", nil)
	get.Header.Set("X-API-Key", key)
	getRec := httptest.NewRecorder()
	h.ServeHTTP(getRec, get)
	if getRec.Code == http.StatusUnauthorized || getRec.Code == http.StatusForbidden {
		t.Errorf("observer GET /api/v1/containers: %d, want allowed", getRec.Code)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/reconcile", strings.NewReader(`{}`))
	req.Header.Set("X-API-Key", key)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("observer POST reconcile: %d, want 403", rec.Code)
	}
}
