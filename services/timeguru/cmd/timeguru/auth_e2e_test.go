// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"net/http"
	"path/filepath"
	"testing"

	"unheaded/pkg/auth"
	"unheaded/pkg/auth/authtest"
	"unheaded/services/timeguru/internal/api"
	"unheaded/services/timeguru/internal/storage"
)

// S-IRON-GATE Phase 6 (Steps 179-182, timeguru): kanban-app reads the
// timeline with its token; micromanager and kanban-app may write;
// /health stays open.
func TestAuthE2E_Timeguru(t *testing.T) {
	f := authtest.New(t, "timeguru", "kanban-app", "micromanager", "sword")
	st, err := storage.NewStore(filepath.Join(t.TempDir(), "timeguru.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	handler := api.NewHandler(st)
	mux := http.NewServeMux()
	mux.HandleFunc("/health", handler.HandleHealth)
	registerAPIRoutes(mux, handler)
	h, err := wrapAuth(mux, f.Config("timeguru", "micromanager", "kanban-app"))
	if err != nil {
		t.Fatal(err)
	}
	const write, body = "/api/v1/milestones/m1/update", `{"status":"done"}`

	if c := authtest.Code(h, http.MethodGet, "/health", "", ""); c != http.StatusOK {
		t.Errorf("GET /health anonymous: %d, want 200", c)
	}
	if c := authtest.Code(h, http.MethodGet, "/timeline", "", ""); c != http.StatusUnauthorized {
		t.Errorf("GET /timeline anonymous: %d, want 401", c)
	}
	if c := authtest.Code(h, http.MethodGet, "/timeline", "", f.Bearer("kanban-app", "timeguru")); !authtest.Authorized(c) {
		t.Errorf("GET /timeline as kanban-app: %d, want past auth", c)
	}
	if c := authtest.Code(h, http.MethodPost, write, body, ""); c != http.StatusUnauthorized {
		t.Errorf("POST %s anonymous: %d, want 401", write, c)
	}
	for _, caller := range []string{"micromanager", "kanban-app"} {
		if c := authtest.Code(h, http.MethodPost, write, body, f.Bearer(caller, "timeguru")); !authtest.Authorized(c) {
			t.Errorf("POST %s as %s: %d, want past auth", write, caller, c)
		}
	}
	if c := authtest.Code(h, http.MethodPost, write, body, f.StrangerBearer("micromanager", "timeguru")); c != http.StatusUnauthorized {
		t.Errorf("POST %s with a key trust.d lacks: %d, want 401", write, c)
	}
	if c := authtest.Code(h, http.MethodPost, write, body, f.Bearer("sword", "timeguru")); c != http.StatusForbidden {
		t.Errorf("POST %s as trusted, not allowlisted sword: %d, want 403", write, c)
	}
}

func TestAuthE2E_TimeguruRefusesBadConfig(t *testing.T) {
	if _, err := wrapAuth(http.NewServeMux(), auth.ServiceAuthConfig{Enabled: true, ServiceName: "timeguru"}); err == nil {
		t.Error("auth on with nothing to verify against: no error, want refusal to start")
	}
}
