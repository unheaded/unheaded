// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

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

func wikiDo(h http.Handler, method, target, body string, hdr map[string]string, c *http.Cookie) *httptest.ResponseRecorder {
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

// With an issuer the wiki has the dashboard's login (Stevie 2026-11-11):
// a browser without a session gets the page shell with the login panel at
// 401, scripts and services get auth's own 401, a session reads the pages,
// and the probes and the login routes stay open.
func TestWikiLogin(t *testing.T) {
	is := bfftest.NewIssuer(t, "unheaded", "wiki-server")
	t.Setenv("AUTH_ISSUER_URL", is.URL)
	f := authtest.New(t, "wiki-server", "unheaded-cli")
	ws := setupTestWiki(t)
	h, err := wrapAuth(ws, setupMux(ws), f.Config("wiki-server", "unheaded-cli"))
	if err != nil {
		t.Fatal(err)
	}
	html := map[string]string{"Accept": "text/html,application/xhtml+xml"}

	// Open without a session: the probes, the login routes, the two scripts.
	for _, p := range []string{"/health", "/ready", "/metrics", "/auth/auth.js", "/auth/nav.js"} {
		if rec := wikiDo(h, http.MethodGet, p, "", nil, nil); rec.Code != http.StatusOK {
			t.Errorf("GET %s anonymous: %d, want 200", p, rec.Code)
		}
	}
	if rec := wikiDo(h, http.MethodGet, "/auth/nav.js", "", nil, nil); !strings.Contains(rec.Body.String(), `"id":"wiki"`) {
		t.Error("nav.js is not the menu")
	}

	// A browser's page request: the shell, 401, the sign-in line, no page content.
	for _, p := range []string{"/wiki/", "/wiki/test-page", "/search?q=x", "/sheets"} {
		rec := wikiDo(h, http.MethodGet, p, "", html, nil)
		body := rec.Body.String()
		if rec.Code != http.StatusUnauthorized || !strings.Contains(body, "<h1>Sign in</h1>") ||
			!strings.Contains(body, `src="/auth/auth.js"`) || !strings.Contains(body, `data-kingdom-nav="wiki"`) {
			t.Errorf("GET %s anonymous browser: %d, want 401 with the sign-in shell:\n%.300s", p, rec.Code, body)
		}
		if strings.Contains(body, "This is the homepage.") || strings.Contains(body, "Some content with") || strings.Contains(body, `href="/wiki/test-page"`) {
			t.Errorf("GET %s anonymous: page content or the sidebar shown before login", p)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Errorf("GET %s anonymous: Content-Type %q", p, ct)
		}
		if rec.Header().Get("WWW-Authenticate") != "" {
			t.Errorf("GET %s anonymous: WWW-Authenticate left on the page", p)
		}
	}
	// Not a browser: auth's own 401, not a page.
	rec := wikiDo(h, http.MethodGet, "/wiki/test-page", "", nil, nil)
	if rec.Code != http.StatusUnauthorized || strings.Contains(rec.Body.String(), "<h1>Sign in</h1>") {
		t.Errorf("GET /wiki/test-page without Accept html: %d %.80s", rec.Code, rec.Body.String())
	}

	// Log in, then read.
	pw := is.AddUser(t, "stevie", "admin")
	body, _ := json.Marshal(map[string]string{"username": "stevie", "password": pw})
	rec = wikiDo(h, http.MethodPost, "/auth/login", string(body), map[string]string{bff.HeaderRequestedWith: bff.RequestedWithValue}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("login: %d %s", rec.Code, rec.Body.String())
	}
	var cookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == bff.CookieNameFor("wiki-server", false) {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("no session cookie")
	}
	for _, p := range []string{"/wiki/", "/wiki/test-page"} {
		rec := wikiDo(h, http.MethodGet, p, "", html, cookie)
		if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "<h1>Sign in</h1>") || !strings.Contains(rec.Body.String(), `href="/wiki/test-page"`) {
			t.Errorf("GET %s with a session: %d, want the page with its sidebar", p, rec.Code)
		}
	}
	// An allowlisted service's token reads too; a stranger's does not.
	if rec := wikiDo(h, http.MethodGet, "/wiki/test-page", "", map[string]string{"Authorization": f.Bearer("unheaded-cli", "wiki-server")}, nil); rec.Code != http.StatusOK {
		t.Errorf("GET /wiki/test-page with a service token: %d", rec.Code)
	}
	if rec := wikiDo(h, http.MethodGet, "/wiki/test-page", "", map[string]string{"Authorization": f.StrangerBearer("unheaded-cli", "wiki-server")}, nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("GET /wiki/test-page with a key trust.d lacks: %d, want 401", rec.Code)
	}
	// A 404 for a missing page passes through the gate untouched.
	if rec := wikiDo(h, http.MethodGet, "/wiki/NoSuchPage", "", html, cookie); rec.Code != http.StatusNotFound {
		t.Errorf("GET /wiki/NoSuchPage with a session: %d, want 404", rec.Code)
	}
}
