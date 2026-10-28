// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gws "github.com/gorilla/websocket"

	"unheaded/pkg/auth/authtest"
	"unheaded/pkg/bff"
	"unheaded/pkg/bff/bfftest"
)

// dashboard-backend as the browser's backend-for-frontend (ADR-103 §4,
// S-IRON-GATE Steps 186-192): login through the issuer, an HttpOnly session
// cookie, role checks on writes, WebSockets by cookie after the Origin check.

type bffEnv struct {
	is  *bfftest.Issuer
	f   *authtest.Fixture
	srv *Server
	h   http.Handler
}

func newBFFEnv(t *testing.T, mod func(*Config)) *bffEnv {
	t.Helper()
	is := bfftest.NewIssuer(t, "unheaded", "dashboard-backend")
	f := authtest.New(t, "dashboard-backend", "unheaded-cli")
	t.Setenv("AUTH_ENABLED", "true")
	t.Setenv("AUTH_TRUST_DIR", f.Trust)
	t.Setenv("AUTH_KEY_FILE", filepath.Join(f.Keys, "dashboard-backend.key"))
	t.Setenv("AUTH_ALLOW_CALLERS", "unheaded-cli")
	t.Setenv("AUTH_ISSUER_URL", is.URL)
	config := DefaultConfig()
	config.WotanAddr = "localhost:19999"
	config.ComplianceRoot = t.TempDir()
	if mod != nil {
		mod(config)
	}
	srv, err := NewServer(config, testLogger())
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	return &bffEnv{is: is, f: f, srv: srv, h: srv.httpServer.Handler}
}

func (e *bffEnv) do(method, target, body string, hdr map[string]string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

var xrw = map[string]string{bff.HeaderRequestedWith: bff.RequestedWithValue}

func (e *bffEnv) login(t *testing.T, user string, roles ...string) *http.Cookie {
	t.Helper()
	pw := e.is.AddUser(t, user, roles...)
	body, _ := json.Marshal(map[string]string{"username": user, "password": pw})
	rec := e.do(http.MethodPost, "/auth/login", string(body), xrw)
	if rec.Code != http.StatusOK {
		t.Fatalf("login %s: %d %s", user, rec.Code, rec.Body)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == bff.DefaultCookieName {
			if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteStrictMode || c.Path != "/" {
				t.Fatalf("session cookie attributes: %+v", c)
			}
			return c
		}
	}
	t.Fatalf("login set no %s cookie", bff.DefaultCookieName)
	return nil
}

func TestBFF_LoginMeLogout(t *testing.T) {
	e := newBFFEnv(t, nil)
	if c := e.do(http.MethodGet, "/auth/me", "", nil).Code; c != http.StatusUnauthorized {
		t.Fatalf("/auth/me without a session: %d", c)
	}
	c := e.login(t, "dev", "operator")
	rec := e.do(http.MethodGet, "/auth/me", "", nil, c)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"sub":"dev"`) || !strings.Contains(rec.Body.String(), "operator") {
		t.Fatalf("/auth/me: %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "eyJ") {
		t.Fatal("/auth/me leaks the token")
	}
	if code := e.do(http.MethodPost, "/auth/logout", "", xrw, c).Code; code != http.StatusNoContent {
		t.Fatalf("logout: %d", code)
	}
	if code := e.do(http.MethodGet, "/api/v1/services", "", nil, c).Code; code != http.StatusUnauthorized {
		t.Fatalf("API after logout: %d, want 401", code)
	}
}

func TestBFF_WritesNeedAnOperatorSession(t *testing.T) {
	e := newBFFEnv(t, nil)
	const write = "/api/v1/doom/input"
	if code := e.do(http.MethodPost, write, "{}", xrw).Code; code != http.StatusUnauthorized {
		t.Fatalf("write without a session: %d, want 401", code)
	}
	op := e.login(t, "dev", "operator")
	if code := e.do(http.MethodPost, write, "{}", nil, op).Code; code != http.StatusForbidden {
		t.Fatalf("session write without X-Requested-With: %d, want 403 (CSRF)", code)
	}
	if code := e.do(http.MethodPost, write, "{}", xrw, op).Code; !authtest.Authorized(code) {
		t.Fatalf("operator session write: %d, want past auth", code)
	}
	viewer := e.login(t, "watcher", "viewer")
	if code := e.do(http.MethodPost, write, "{}", xrw, viewer).Code; code != http.StatusForbidden {
		t.Fatalf("viewer session write: %d, want 403", code)
	}
	// A read-shaped POST stays open to readers.
	if code := e.do(http.MethodPost, "/api/v1/metrics/query", `{"name":"x"}`, xrw, viewer).Code; !authtest.Authorized(code) {
		t.Fatalf("viewer metrics query: %d, want past auth", code)
	}
}

func TestBFF_ReadsNeedASession(t *testing.T) {
	e := newBFFEnv(t, nil)
	if code := e.do(http.MethodGet, "/api/v1/services", "", nil).Code; code != http.StatusUnauthorized {
		t.Fatalf("anonymous read: %d, want 401", code)
	}
	viewer := e.login(t, "watcher", "viewer")
	if code := e.do(http.MethodGet, "/api/v1/services", "", nil, viewer).Code; code != http.StatusOK {
		t.Fatalf("viewer read: %d, want 200", code)
	}
	guest := e.login(t, "passerby", "guest")
	if code := e.do(http.MethodGet, "/api/v1/services", "", nil, guest).Code; code != http.StatusForbidden {
		t.Fatalf("guest read: %d, want 403", code)
	}
}

func TestBFF_ServiceTokensStillWork(t *testing.T) {
	e := newBFFEnv(t, nil)
	tok := e.f.Bearer("unheaded-cli", "dashboard-backend")
	if code := e.do(http.MethodPost, "/api/v1/doom/input", "{}", map[string]string{"Authorization": tok}).Code; !authtest.Authorized(code) {
		t.Fatalf("unheaded-cli service token write: %d", code)
	}
	if code := e.do(http.MethodPost, "/api/v1/doom/input", "{}", map[string]string{"Authorization": e.f.StrangerBearer("unheaded-cli", "dashboard-backend")}).Code; code != http.StatusUnauthorized {
		t.Fatalf("untrusted key: %d, want 401", code)
	}
}

func TestBFF_OpenPaths(t *testing.T) {
	e := newBFFEnv(t, nil)
	for _, p := range []string{"/health", "/ready", "/metrics", "/", "/index.html", "/logs", "/compliance", "/auth/auth.js", "/.well-known/security.txt"} {
		if code := e.do(http.MethodGet, p, "", nil).Code; code == http.StatusUnauthorized || code == http.StatusForbidden {
			t.Errorf("GET %s: %d, want open", p, code)
		}
	}
	// Non-canonical paths never ride the static exemption into the API.
	for _, p := range []string{"/static/../api/v1/services", "/x/%2e%2e/api/v1/services"} {
		if code := e.do(http.MethodGet, p, "", nil).Code; code == http.StatusOK {
			t.Errorf("GET %s: 200, want refused", p)
		}
	}
	// A write to a static path still needs credentials.
	if code := e.do(http.MethodPost, "/", "{}", nil).Code; code != http.StatusUnauthorized {
		t.Errorf("POST /: %d, want 401", code)
	}
}

func TestBFF_WebSocketNeedsTheCookieAfterTheOriginCheck(t *testing.T) {
	e := newBFFEnv(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := e.srv.wsServer.Start(ctx); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(e.h)
	defer ts.Close()
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	dial := func(origin string, c *http.Cookie) int {
		h := http.Header{}
		if origin != "" {
			h.Set("Origin", origin)
		}
		if c != nil {
			h.Set("Cookie", c.Name+"="+c.Value)
		}
		conn, resp, err := gws.DefaultDialer.Dial(wsURL, h)
		if conn != nil {
			conn.Close()
		}
		if resp == nil {
			t.Fatalf("dial: %v", err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	good := ts.URL
	if code := dial("http://evil.example", nil); code != http.StatusForbidden {
		t.Fatalf("foreign origin, no cookie: %d, want 403 (origin before auth)", code)
	}
	if code := dial(good, nil); code != http.StatusUnauthorized {
		t.Fatalf("same origin, no cookie: %d, want 401", code)
	}
	c := e.login(t, "watcher", "viewer")
	if code := dial("http://evil.example", c); code != http.StatusForbidden {
		t.Fatalf("foreign origin with a cookie: %d, want 403", code)
	}
	if code := dial(good, c); code != http.StatusSwitchingProtocols {
		t.Fatalf("same origin with a session: %d, want 101", code)
	}
}

func TestBFF_RefusesToStartMisconfigured(t *testing.T) {
	f := authtest.New(t, "dashboard-backend")
	t.Setenv("AUTH_ENABLED", "true")
	t.Setenv("AUTH_TRUST_DIR", f.Trust)
	t.Setenv("AUTH_KEY_FILE", filepath.Join(f.Keys, "dashboard-backend.key"))
	config := DefaultConfig()
	config.WotanAddr = "localhost:19999"

	t.Setenv("AUTH_ISSUER_URL", "http://auth:19020")
	if _, err := NewServer(config, testLogger()); err == nil {
		t.Fatal("plain http issuer on a non-loopback host accepted without -dev-plaintext")
	}
	config.DevPlaintext = true
	srv, err := NewServer(config, testLogger())
	if err != nil {
		t.Fatalf("with -dev-plaintext: %v", err)
	}
	_ = srv.Shutdown(context.Background())
	config.DevPlaintext = false

	t.Setenv("AUTH_ISSUER_URL", "")
	t.Setenv("AUTH_JWT_KEY", "shared-secret-for-the-test")
	if _, err := NewServer(config, testLogger()); !errors.Is(err, errAuthConfig) {
		t.Fatalf("AUTH_JWT_KEY with auth on: %v, want a refusal", err)
	}
}

func TestBFF_AuthOffServesAsBefore(t *testing.T) {
	t.Setenv("AUTH_ENABLED", "")
	t.Setenv("AUTH_ISSUER_URL", "")
	srv := newTestServer(t)
	h := srv.httpServer.Handler
	if code := authtest.Code(h, http.MethodGet, "/api/v1/services", "", ""); code != http.StatusOK {
		t.Fatalf("auth off read: %d", code)
	}
	if code := authtest.Code(h, http.MethodGet, "/auth/auth.js", "", ""); code != http.StatusOK {
		t.Fatalf("auth.js with auth off: %d, want 200 (the page loads it either way)", code)
	}
	if code := authtest.Code(h, http.MethodGet, "/auth/me", "", ""); code != http.StatusNotFound {
		t.Fatalf("/auth/me with auth off: %d, want 404 (auth.js shows nothing)", code)
	}
}
