// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
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
	s := NewServer(Config{Port: "0"})
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
			if c.Name == bff.CookieNameFor("kanban-app", false) {
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

// S-WEST-GROUND Phase 3 (ADR-147 decision 16): with AUTH_ANONYMOUS_FROM set,
// the task list and /metrics need a credential from outside those networks;
// the page itself still loads so a person can log in.
func TestTaskListNeedsCredentialFromOutside(t *testing.T) {
	t.Setenv("AUTH_ISSUER_URL", "")
	t.Setenv("RATE_LIMIT_ENABLED", "false")
	f := authtest.New(t, "kanban-app", "dashboard-backend")
	s := NewServer(Config{Port: "0"})
	t.Cleanup(s.cancel)
	cfg := f.Config("kanban-app", "dashboard-backend")
	cfg.AnonymousFrom = []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}
	h, err := s.buildHandler(cfg)
	if err != nil {
		t.Fatal(err)
	}
	from := func(target, remote, authz string) int {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.RemoteAddr = remote
		if authz != "" {
			req.Header.Set("Authorization", authz)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	cases := []struct {
		target, remote string
		want           int
	}{
		{"/api/v1/tasks", "192.168.69.50:1", http.StatusUnauthorized},
		{"/api/v1/tasks", "127.0.0.1:1", http.StatusOK},
		{"/", "192.168.69.50:1", http.StatusOK},
		{"/metrics", "192.168.69.50:1", http.StatusUnauthorized},
		{"/metrics", "127.0.0.1:1", http.StatusOK},
		{"/health", "192.168.69.50:1", http.StatusOK},
	}
	for _, c := range cases {
		if got := from(c.target, c.remote, ""); got != c.want {
			t.Errorf("GET %s from %s: %d, want %d", c.target, c.remote, got, c.want)
		}
	}
	// A service token from outside still reads the task list.
	if got := from("/api/v1/tasks", "192.168.69.50:1", f.Bearer("dashboard-backend", "kanban-app")); got != http.StatusOK {
		t.Errorf("GET /api/v1/tasks from the LAN with a token: %d, want 200", got)
	}
}

func TestAuthBFF_KanbanAuthOff(t *testing.T) {
	is := bfftest.NewIssuer(t, "kanban-app")
	t.Setenv("AUTH_ISSUER_URL", is.URL) // set, but auth is off: no login routes
	off := NewServer(Config{Port: "0"})
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
	s := NewServer(Config{Port: "0"})
	t.Cleanup(s.cancel)
	if _, err := s.buildHandler(authtest.New(t, "kanban-app").Config("kanban-app")); err == nil {
		t.Fatal("plain http issuer accepted without AUTH_BFF_DEV_PLAINTEXT")
	}
	t.Setenv("AUTH_BFF_DEV_PLAINTEXT", "true")
	if _, err := s.buildHandler(authtest.New(t, "kanban-app").Config("kanban-app")); err != nil {
		t.Fatalf("with AUTH_BFF_DEV_PLAINTEXT: %v", err)
	}
}

// The board takes its session store from the environment (ADR-114 N5).
// "well" with no way to reach the Well stops the board from starting: it
// never keeps sessions somewhere else instead. "dev-memory" has to be
// named, and then the cookie is the server-side kind (not persistent).
func TestAuthBFF_KanbanSessionStoreFromEnv(t *testing.T) {
	is := bfftest.NewIssuer(t, "unheaded", "kanban-app")
	t.Setenv("AUTH_ISSUER_URL", is.URL)
	t.Setenv("RATE_LIMIT_ENABLED", "false")

	t.Setenv(bff.EnvSessionStore, bff.SessionStoreWell)
	t.Setenv("AUTH_SESSION_WELL_PASSWORD", "")
	s := NewServer(Config{Port: "0"})
	t.Cleanup(s.cancel)
	if _, err := s.buildHandler(authtest.New(t, "kanban-app").Config("kanban-app")); err == nil ||
		!strings.Contains(err.Error(), "AUTH_SESSION_WELL_PASSWORD") {
		t.Fatalf("AUTH_SESSION_STORE=well without a Well login: %v, want a refusal naming AUTH_SESSION_WELL_PASSWORD", err)
	}

	t.Setenv(bff.EnvSessionStore, "sqlite")
	if _, err := s.buildHandler(authtest.New(t, "kanban-app").Config("kanban-app")); err == nil {
		t.Fatal("an unknown AUTH_SESSION_STORE was accepted")
	}

	t.Setenv(bff.EnvSessionStore, bff.SessionStoreDevMemory)
	s2 := NewServer(Config{Port: "0"})
	t.Cleanup(s2.cancel)
	h, err := s2.buildHandler(authtest.New(t, "kanban-app").Config("kanban-app"))
	if err != nil {
		t.Fatalf("dev-memory: %v", err)
	}
	pw := is.AddUser(t, "dev", "operator")
	body, _ := json.Marshal(map[string]string{"username": "dev", "password": pw})
	rec := kanbanDo(h, http.MethodPost, "/auth/login", string(body), map[string]string{bff.HeaderRequestedWith: bff.RequestedWithValue}, nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "absolute_expires_at") {
		t.Fatalf("login: %d %s", rec.Code, rec.Body)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == bff.CookieNameFor("kanban-app", false) {
			if c.MaxAge != 0 || !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteStrictMode {
				t.Fatalf("server-side session cookie: %+v", c)
			}
			if code := kanbanDo(h, http.MethodGet, "/auth/me", "", nil, c).Code; code != http.StatusOK {
				t.Fatalf("/auth/me with the session: %d", code)
			}
			return
		}
	}
	t.Fatal("no session cookie")
}
