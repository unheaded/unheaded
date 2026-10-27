// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/auth"
	"unheaded/pkg/auth/authtest"
)

// S-IRON-GATE Phase 6 (Steps 179-182, zhen-agentd): the listener's auth
// takes service tokens from unheaded-cli and dashboard-backend, and RBAC
// behind it keeps a read-only (observer) key off the gate's write routes.
func TestAuthE2E_ZhenAgentd(t *testing.T) {
	f := authtest.New(t, "zhen-agentd", "unheaded-cli", "dashboard-backend", "kanban-app")
	keys := filepath.Join(t.TempDir(), "apikeys.jsonl")
	store, err := auth.NewFileAPIKeyStore(keys)
	if err != nil {
		t.Fatal(err)
	}
	observer, _, err := store.Create(context.Background(), "watcher", auth.RoleObserver, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	cfg := f.Config("zhen-agentd", "unheaded-cli", "dashboard-backend")
	cfg.APIKeyStore = "file:" + keys
	mw, err := listenerAuth(cfg, false, "0.0.0.0")
	if err != nil {
		t.Fatal(err)
	}
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := mw(ok)
	const write, body = "/api/v1/agent/ask", `{"question":"q"}`

	for _, p := range []string{"/health", "/ready", "/metrics", "/api/v1/openapi.json"} {
		if c := authtest.Code(h, http.MethodGet, p, "", ""); c != http.StatusOK {
			t.Errorf("GET %s anonymous: %d, want 200", p, c)
		}
	}
	if c := authtest.Code(h, http.MethodPost, write, body, ""); c != http.StatusUnauthorized {
		t.Errorf("POST %s anonymous: %d, want 401", write, c)
	}
	for _, caller := range []string{"unheaded-cli", "dashboard-backend"} {
		if c := authtest.Code(h, http.MethodPost, write, body, f.Bearer(caller, "zhen-agentd")); c != http.StatusOK {
			t.Errorf("POST %s as %s: %d, want 200", write, caller, c)
		}
	}
	if c := authtest.Code(h, http.MethodPost, write, body, f.StrangerBearer("unheaded-cli", "zhen-agentd")); c != http.StatusUnauthorized {
		t.Errorf("POST %s with a key trust.d lacks: %d, want 401", write, c)
	}
	if c := authtest.Code(h, http.MethodPost, write, body, f.Bearer("kanban-app", "zhen-agentd")); c != http.StatusForbidden {
		t.Errorf("POST %s as trusted, not allowlisted kanban-app: %d, want 403", write, c)
	}

	withKey := func(method, path string) int {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("X-API-Key", observer)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec.Code
	}
	if c := withKey(http.MethodGet, "/api/v1/models"); c != http.StatusOK {
		t.Errorf("observer GET /api/v1/models: %d, want 200", c)
	}
	for _, p := range []string{write, "/api/v1/tool/exec", "/api/v1/agent/confirm", "/api/v1/memory/remember"} {
		if c := withKey(http.MethodPost, p); c != http.StatusForbidden {
			t.Errorf("observer POST %s: %d, want 403", p, c)
		}
	}
}
