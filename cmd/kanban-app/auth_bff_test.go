// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"unheaded/pkg/auth/authtest"
	"unheaded/pkg/bff"
	"unheaded/pkg/bff/bfftest"
)

// S-IRON-GATE Steps 211-216: the board's own login (pkg/bff, ADR-103 §4).
// Reads stay public (The Meta Moment); a person writes with an operator
// session cookie, services with their tokens as before.

func kanbanDo(h http.Handler, method, target, body string, hdr map[string]string, c *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	if c != nil {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAuthBFF_KanbanBoardLogin(t *testing.T) {
	is := bfftest.NewIssuer(t, "unheaded", "kanban-app")
	t.Setenv("AUTH_ISSUER_URL", is.URL)
	t.Setenv("RATE_LIMIT_ENABLED", "false")
	f := authtest.New(t, "kanban-app", "dashboard-backend")
	s := NewServer(Config{Port: "0", DataDir: t.TempDir()})
	t.Cleanup(s.cancel)
	h, err := s.buildHandler(f.Config("kanban-app", "dashboard-backend"))
	if err != nil {
		t.Fatal(err)
	}
	xrw := map[string]string{bff.HeaderRequestedWith: bff.RequestedWithValue}
	login := func(user string, roles ...string) *http.Cookie {
		t.Helper()
		pw := is.AddUser(t, user, roles...)
		body, _ := json.Marshal(map[string]string{"username": user, "password": pw})
		rec := kanbanDo(h, http.MethodPost, "/auth/login", string(body), xrw, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("login %s: %d %s", user, rec.Code, rec.Body)
		}
		for _, c := range rec.Result().Cookies() {
			if c.Name == bff.DefaultCookieName {
				if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteStrictMode {
					t.Fatalf("cookie attributes: %+v", c)
				}
				return c
			}
		}
		t.Fatal("no session cookie")
		return nil
	}
	task := func(id string) string { return `{"id":"` + id + `","title":"board login","status":"todo"}` }

	for _, p := range []string{"/", "/api/v1/tasks", "/auth/auth.js"} {
		if c := kanbanDo(h, http.MethodGet, p, "", nil, nil).Code; c != http.StatusOK {
			t.Errorf("GET %s anonymous: %d, want 200 (the board is public)", p, c)
		}
	}
	if c := kanbanDo(h, http.MethodPost, "/api/v1/tasks", task("anon"), xrw, nil).Code; c != http.StatusUnauthorized {
		t.Fatalf("anonymous write: %d, want 401", c)
	}
	op := login("dev", "operator")
	if rec := kanbanDo(h, http.MethodGet, "/auth/me", "", nil, op); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"sub":"dev"`) {
		t.Fatalf("/auth/me: %d %s", rec.Code, rec.Body)
	}
	if c := kanbanDo(h, http.MethodPost, "/api/v1/tasks", task("csrf"), nil, op).Code; c != http.StatusForbidden {
		t.Fatalf("session write without X-Requested-With: %d, want 403", c)
	}
	if c := kanbanDo(h, http.MethodPost, "/api/v1/tasks", task("by-dev"), xrw, op).Code; !authtest.Authorized(c) {
		t.Fatalf("operator session write: %d, want past auth", c)
	}
	viewer := login("watcher", "viewer")
	if c := kanbanDo(h, http.MethodPost, "/api/v1/tasks", task("by-viewer"), xrw, viewer).Code; c != http.StatusForbidden {
		t.Fatalf("viewer session write: %d, want 403", c)
	}
	// Services keep their tokens.
	if c := kanbanDo(h, http.MethodPost, "/api/v1/tasks", task("by-dash"),
		map[string]string{"Authorization": f.Bearer("dashboard-backend", "kanban-app")}, nil).Code; !authtest.Authorized(c) {
		t.Fatalf("dashboard-backend token write: %d", c)
	}
}

func TestAuthBFF_KanbanAuthOff(t *testing.T) {
	is := bfftest.NewIssuer(t, "kanban-app")
	t.Setenv("AUTH_ISSUER_URL", is.URL) // set, but auth is off: no login routes
	off := NewServer(Config{Port: "0", DataDir: t.TempDir()})
	t.Cleanup(off.cancel)
	cfg := authtest.New(t, "kanban-app").Config("kanban-app")
	cfg.Enabled = false
	h, err := off.buildHandler(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if c := kanbanDo(h, http.MethodGet, "/auth/auth.js", "", nil, nil).Code; c != http.StatusOK {
		t.Fatalf("auth.js with auth off: %d, want 200", c)
	}
	if c := kanbanDo(h, http.MethodGet, "/auth/me", "", nil, nil).Code; c != http.StatusNotFound {
		t.Fatalf("/auth/me with auth off: %d, want 404", c)
	}
}

func TestAuthBFF_KanbanRefusesPlainHTTPIssuer(t *testing.T) {
	t.Setenv("AUTH_ISSUER_URL", "http://auth:19020")
	t.Setenv("AUTH_BFF_DEV_PLAINTEXT", "")
	s := NewServer(Config{Port: "0", DataDir: t.TempDir()})
	t.Cleanup(s.cancel)
	if _, err := s.buildHandler(authtest.New(t, "kanban-app").Config("kanban-app")); err == nil {
		t.Fatal("plain http issuer accepted without AUTH_BFF_DEV_PLAINTEXT")
	}
	t.Setenv("AUTH_BFF_DEV_PLAINTEXT", "true")
	if _, err := s.buildHandler(authtest.New(t, "kanban-app").Config("kanban-app")); err != nil {
		t.Fatalf("with AUTH_BFF_DEV_PLAINTEXT: %v", err)
	}
}
