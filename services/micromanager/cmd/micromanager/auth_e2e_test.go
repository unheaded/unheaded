// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"net/http"
	"testing"

	"unheaded/pkg/auth"
	"unheaded/pkg/auth/authtest"
	"unheaded/services/micromanager"
)

// S-IRON-GATE Phase 6 (Steps 179-182, micromanager): captain, architect and
// dashboard-backend may call; /health stays open.
func TestAuthE2E_Micromanager(t *testing.T) {
	f := authtest.New(t, "micromanager", "captain", "architect", "dashboard-backend", "sword")
	store := micromanager.NewStore()
	api := micromanager.NewAPI(store, micromanager.NewService(store, nil))
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	registerAPIRoutes(mux, api)
	h, err := wrapAuth(mux, f.Config("micromanager", "captain", "architect", "dashboard-backend"))
	if err != nil {
		t.Fatal(err)
	}
	const write, body = "/api/v1/tasks", `{"title":"auth e2e","priority":"P2"}`

	if c := authtest.Code(h, http.MethodGet, "/health", "", ""); c != http.StatusOK {
		t.Errorf("GET /health anonymous: %d, want 200", c)
	}
	if c := authtest.Code(h, http.MethodGet, "/api/v1/backlog", "", ""); c != http.StatusUnauthorized {
		t.Errorf("GET /api/v1/backlog anonymous: %d, want 401", c)
	}
	if c := authtest.Code(h, http.MethodPost, write, body, ""); c != http.StatusUnauthorized {
		t.Errorf("POST %s anonymous: %d, want 401", write, c)
	}
	if c := authtest.Code(h, http.MethodPut, "/api/v1/tasks/x", body, ""); c != http.StatusUnauthorized {
		t.Errorf("PUT /api/v1/tasks/x anonymous: %d, want 401", c)
	}
	for _, caller := range []string{"captain", "architect", "dashboard-backend"} {
		if c := authtest.Code(h, http.MethodPost, write, body, f.Bearer(caller, "micromanager")); !authtest.Authorized(c) {
			t.Errorf("POST %s as %s: %d, want past auth", write, caller, c)
		}
	}
	if c := authtest.Code(h, http.MethodPost, write, body, f.StrangerBearer("captain", "micromanager")); c != http.StatusUnauthorized {
		t.Errorf("POST %s with a key trust.d lacks: %d, want 401", write, c)
	}
	if c := authtest.Code(h, http.MethodPost, write, body, f.Bearer("sword", "micromanager")); c != http.StatusForbidden {
		t.Errorf("POST %s as trusted, not allowlisted sword: %d, want 403", write, c)
	}
}

func TestAuthE2E_MicromanagerRefusesBadConfig(t *testing.T) {
	if _, err := wrapAuth(http.NewServeMux(), auth.ServiceAuthConfig{Enabled: true, ServiceName: "micromanager"}); err == nil {
		t.Error("auth on with nothing to verify against: no error, want refusal to start")
	}
}
