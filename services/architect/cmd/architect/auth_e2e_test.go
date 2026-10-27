// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"net/http"
	"testing"

	"unheaded/pkg/auth"
	"unheaded/pkg/auth/authtest"
	"unheaded/services/architect"
)

// S-IRON-GATE Phase 6 (Steps 179-182, architect): captain, micromanager and
// dashboard-backend may call; /health stays open. Its routes sit outside
// /api/v1/, so this also pins that a write there needs a token.
func TestAuthE2E_Architect(t *testing.T) {
	f := authtest.New(t, "architect", "captain", "micromanager", "dashboard-backend", "sword")
	handler := architect.NewHTTPHandler(architect.New())
	mux := http.NewServeMux()
	mux.HandleFunc("/health", handler.Health)
	registerAPIRoutes(mux, handler)
	h, err := wrapAuth(mux, f.Config("architect", "captain", "micromanager", "dashboard-backend"))
	if err != nil {
		t.Fatal(err)
	}
	const write, body = "/infrastructure/services", `{"name":"auth-e2e"}`

	if c := authtest.Code(h, http.MethodGet, "/health", "", ""); c != http.StatusOK {
		t.Errorf("GET /health anonymous: %d, want 200", c)
	}
	if c := authtest.Code(h, http.MethodGet, "/infrastructure", "", ""); c != http.StatusUnauthorized {
		t.Errorf("GET /infrastructure anonymous: %d, want 401", c)
	}
	if c := authtest.Code(h, http.MethodPost, write, body, ""); c != http.StatusUnauthorized {
		t.Errorf("POST %s anonymous: %d, want 401", write, c)
	}
	for _, caller := range []string{"captain", "micromanager", "dashboard-backend"} {
		if c := authtest.Code(h, http.MethodPost, write, body, f.Bearer(caller, "architect")); !authtest.Authorized(c) {
			t.Errorf("POST %s as %s: %d, want past auth", write, caller, c)
		}
	}
	if c := authtest.Code(h, http.MethodPost, write, body, f.StrangerBearer("captain", "architect")); c != http.StatusUnauthorized {
		t.Errorf("POST %s with a key trust.d lacks: %d, want 401", write, c)
	}
	if c := authtest.Code(h, http.MethodPost, write, body, f.Bearer("sword", "architect")); c != http.StatusForbidden {
		t.Errorf("POST %s as trusted, not allowlisted sword: %d, want 403", write, c)
	}
}

func TestAuthE2E_ArchitectRefusesBadConfig(t *testing.T) {
	if _, err := wrapAuth(http.NewServeMux(), auth.ServiceAuthConfig{Enabled: true, ServiceName: "architect"}); err == nil {
		t.Error("auth on with nothing to verify against: no error, want refusal to start")
	}
}
