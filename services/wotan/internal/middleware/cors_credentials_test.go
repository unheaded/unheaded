// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func corsRequest(t *testing.T, allowed []string, method, origin string) *httptest.ResponseRecorder {
	t.Helper()
	h := CORS(allowed)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(method, "/api/v1/topics", nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// With "*" (Wotan's default) the answer is Allow-Origin "*" and no
// Allow-Credentials. It used to reflect any Origin and add
// Allow-Credentials: true, so any web page could make credentialed
// requests to Wotan and read the answers.
func TestCORS_WildcardNeverAllowsCredentials(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodOptions} {
		rec := corsRequest(t, []string{"*"}, method, "https://evil.example")
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
			t.Errorf("%s: Allow-Origin = %q, want *", method, got)
		}
		if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
			t.Errorf("%s: Allow-Credentials = %q, want unset", method, got)
		}
	}
	// "*" anywhere in the list wins over named entries.
	rec := corsRequest(t, []string{"https://a.example", "*"}, http.MethodGet, "https://evil.example")
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" || rec.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Errorf("mixed list with *: origin=%q creds=%q", rec.Header().Get("Access-Control-Allow-Origin"), rec.Header().Get("Access-Control-Allow-Credentials"))
	}
}

// A named list echoes only a matching origin, with credentials and Vary:
// Origin so a cache does not serve one origin's answer to another.
func TestCORS_ListEchoesMatchWithCredentials(t *testing.T) {
	allowed := []string{"https://dash.example", "https://kanban.example"}

	rec := corsRequest(t, allowed, http.MethodGet, "https://kanban.example")
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://kanban.example" {
		t.Errorf("Allow-Origin = %q, want the matching origin", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("Allow-Credentials = %q, want true", got)
	}
	if got := rec.Header().Get("Vary"); got != "Origin" {
		t.Errorf("Vary = %q, want Origin", got)
	}

	rec = corsRequest(t, allowed, http.MethodGet, "https://evil.example")
	if rec.Header().Get("Access-Control-Allow-Origin") != "" || rec.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Errorf("non-matching origin got CORS headers: origin=%q creds=%q",
			rec.Header().Get("Access-Control-Allow-Origin"), rec.Header().Get("Access-Control-Allow-Credentials"))
	}

	// No Origin header (not a browser cross-origin request): no CORS headers.
	rec = corsRequest(t, allowed, http.MethodGet, "")
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("request without Origin got Allow-Origin %q", rec.Header().Get("Access-Control-Allow-Origin"))
	}
}
