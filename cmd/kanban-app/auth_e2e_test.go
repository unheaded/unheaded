// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"unheaded/pkg/auth"
	"unheaded/pkg/auth/authtest"
)

// S-IRON-GATE Phase 6 (Steps 179-182, kanban-app): the board stays publicly
// readable (CLAUDE.md, The Meta Moment), writes need a token from an
// allowlisted caller (dashboard-backend, zhen-agentd, micromanager).
func TestAuthE2E_KanbanApp(t *testing.T) {
	f := authtest.New(t, "kanban-app", "dashboard-backend", "zhen-agentd", "micromanager", "sword")
	s := NewServer(Config{Port: "0", DataDir: t.TempDir()})
	t.Cleanup(s.cancel)
	h, err := s.buildHandler(f.Config("kanban-app", "dashboard-backend", "zhen-agentd", "micromanager"))
	if err != nil {
		t.Fatal(err)
	}
	task := func(id string) string {
		return `{"id":"` + id + `","title":"auth e2e","status":"todo"}`
	}

	for _, p := range []string{"/health", "/ready", "/metrics", "/api/v1/tasks", "/api/v1/timeline", "/"} {
		if c := authtest.Code(h, http.MethodGet, p, "", ""); !authtest.Authorized(c) {
			t.Errorf("GET %s anonymous: %d, want readable", p, c)
		}
	}
	if c := authtest.Code(h, http.MethodPost, "/api/v1/tasks", task("anon"), ""); c != http.StatusUnauthorized {
		t.Errorf("POST /api/v1/tasks anonymous: %d, want 401", c)
	}
	if c := authtest.Code(h, http.MethodDelete, "/api/v1/tasks/anon", "", ""); c != http.StatusUnauthorized {
		t.Errorf("DELETE /api/v1/tasks/anon anonymous: %d, want 401", c)
	}
	for _, caller := range []string{"dashboard-backend", "zhen-agentd", "micromanager"} {
		id := "e2e-" + caller
		if c := authtest.Code(h, http.MethodPost, "/api/v1/tasks", task(id), f.Bearer(caller, "kanban-app")); !authtest.Authorized(c) {
			t.Errorf("POST /api/v1/tasks as %s: %d, want past auth", caller, c)
		}
	}
	if c := authtest.Code(h, http.MethodPost, "/api/v1/tasks", task("stranger"), f.StrangerBearer("dashboard-backend", "kanban-app")); c != http.StatusUnauthorized {
		t.Errorf("POST with a key trust.d lacks: %d, want 401", c)
	}
	if c := authtest.Code(h, http.MethodPost, "/api/v1/tasks", task("sword"), f.Bearer("sword", "kanban-app")); c != http.StatusForbidden {
		t.Errorf("POST as trusted, not allowlisted sword: %d, want 403", c)
	}
}

func TestAuthE2E_KanbanAppRefusesBadConfig(t *testing.T) {
	s := NewServer(Config{Port: "0", DataDir: t.TempDir()})
	t.Cleanup(s.cancel)
	if _, err := s.buildHandler(auth.ServiceAuthConfig{Enabled: true, ServiceName: "kanban-app"}); err == nil {
		t.Error("auth on with nothing to verify against: no error, want refusal to start")
	}
}

// kanban-app reads Timeguru's timeline with its own service token, so the
// read still works once Timeguru requires one.
func TestAuthE2E_KanbanAppCallsTimeguruWithToken(t *testing.T) {
	f := authtest.New(t, "timeguru", "kanban-app")
	timeline := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"timeline": map[string]any{"title": "t"}})
	})
	guarded, err := auth.NewServiceHandler(timeline, f.Config("timeguru", "kanban-app"), auth.ServiceRoutes{})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(guarded)
	defer srv.Close()

	s := NewServer(Config{Port: "0", DataDir: t.TempDir(), TimeGuruAddr: strings.TrimPrefix(srv.URL, "http://")})
	t.Cleanup(s.cancel)
	if err := s.fetchTimelineFromTimeguru(); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("without a minter: %v, want a 401 from timeguru", err)
	}
	s.minter = f.Minter("kanban-app")
	if err := s.fetchTimelineFromTimeguru(); err != nil {
		t.Fatalf("with kanban-app's token: %v", err)
	}
}
