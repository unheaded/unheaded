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

	"github.com/rs/zerolog"

	"unheaded/pkg/auth"
	"unheaded/pkg/auth/authtest"
)

// S-IRON-GATE Phase 6 (Steps 179-182, sword): unheaded-cli and
// dashboard-backend may deploy with their service tokens (role service),
// which DefaultServicePolicies would refuse on /api/v1/deploy*; an observer
// may read the history but not deploy.
func TestAuthE2E_Sword(t *testing.T) {
	t.Setenv("AUTH_ENABLED", "true") // Sword's own write gate (sword.Config.AuthEnabled)
	f := authtest.New(t, "sword", "unheaded-cli", "dashboard-backend", "kanban-app")
	log := zerolog.Nop()
	s, err := build(config{cuirassURL: "http://127.0.0.1:19006", cuirassKey: "k", dataDir: t.TempDir()}, &log)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	keys := filepath.Join(t.TempDir(), "apikeys.jsonl")
	store, err := auth.NewFileAPIKeyStore(keys)
	if err != nil {
		t.Fatal(err)
	}
	observer, _, err := store.Create(context.Background(), "watcher", auth.RoleObserver, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	cfg := f.Config("sword", "unheaded-cli", "dashboard-backend")
	cfg.APIKeyStore = "file:" + keys
	h, err := handler(s, cfg)
	if err != nil {
		t.Fatal(err)
	}
	const write, body = "/api/v1/deployments", `{}`

	for _, p := range []string{"/health", "/metrics"} {
		if c := authtest.Code(h, http.MethodGet, p, "", ""); c != http.StatusOK {
			t.Errorf("GET %s anonymous: %d, want 200", p, c)
		}
	}
	if c := authtest.Code(h, http.MethodGet, write, "", ""); c != http.StatusUnauthorized {
		t.Errorf("GET %s anonymous: %d, want 401", write, c)
	}
	if c := authtest.Code(h, http.MethodPost, write, body, ""); c != http.StatusUnauthorized {
		t.Errorf("POST %s anonymous: %d, want 401", write, c)
	}
	for _, caller := range []string{"unheaded-cli", "dashboard-backend"} {
		if c := authtest.Code(h, http.MethodPost, write, body, f.Bearer(caller, "sword")); !authtest.Authorized(c) {
			t.Errorf("POST %s as %s: %d, want past auth", write, caller, c)
		}
		if c := authtest.Code(h, http.MethodGet, write, "", f.Bearer(caller, "sword")); c != http.StatusOK {
			t.Errorf("GET %s as %s: %d, want 200", write, caller, c)
		}
	}
	if c := authtest.Code(h, http.MethodPost, write, body, f.StrangerBearer("unheaded-cli", "sword")); c != http.StatusUnauthorized {
		t.Errorf("POST %s with a key trust.d lacks: %d, want 401", write, c)
	}
	if c := authtest.Code(h, http.MethodPost, write, body, f.Bearer("kanban-app", "sword")); c != http.StatusForbidden {
		t.Errorf("POST %s as trusted, not allowlisted kanban-app: %d, want 403", write, c)
	}

	withKey := func(method string) int {
		r := httptest.NewRequest(method, write, strings.NewReader(body))
		r.Header.Set("X-API-Key", observer)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec.Code
	}
	if c := withKey(http.MethodGet); c != http.StatusOK {
		t.Errorf("observer GET %s: %d, want 200", write, c)
	}
	if c := withKey(http.MethodPost); c != http.StatusForbidden {
		t.Errorf("observer POST %s: %d, want 403", write, c)
	}
}

func TestAuthE2E_SwordRefusesBadConfig(t *testing.T) {
	log := zerolog.Nop()
	s, err := build(config{cuirassURL: "http://127.0.0.1:19006", cuirassKey: "k"}, &log)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := handler(s, auth.ServiceAuthConfig{Enabled: true, ServiceName: "sword"}); err == nil {
		t.Error("auth on with nothing to verify against: no error, want refusal to start")
	}
}
