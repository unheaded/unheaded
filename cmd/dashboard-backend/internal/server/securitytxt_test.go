// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package server

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestSecurityTxt_Served: RFC 9116 file at /.well-known/security.txt, plain
// text, with a Contact field.
func TestSecurityTxt_Served(t *testing.T) {
	srv := newTestServer(t)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.well-known/security.txt", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/plain; charset=utf-8", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{"Contact: mailto:", "Expires: ", "Canonical: ", "Policy: "} {
		if !strings.Contains(body, want) {
			t.Errorf("security.txt lacks %q", want)
		}
	}
}

// TestSecurityTxt_MethodsAndAuth: only GET/HEAD, and reachable without
// credentials when AUTH_ENABLED=true (RFC 9116: the file is public).
func TestSecurityTxt_MethodsAndAuth(t *testing.T) {
	t.Setenv("AUTH_ENABLED", "true")
	t.Setenv("AUTH_API_KEYS", "k1:test-only-key")
	srv := newTestServer(t)
	h := srv.httpServer.Handler

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.well-known/security.txt", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("unauthenticated GET status = %d, want 200", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodHead, "/.well-known/security.txt", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("HEAD status = %d, want 200", rec.Code)
	}
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/.well-known/security.txt", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST status = %d, want 405", rec.Code)
	}
	// The skip must not widen: an API path still needs auth.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/services", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated /api/v1/services = %d, want 401", rec.Code)
	}
}

// TestSecurityTxt_NotExpired keeps the file from rotting silently: RFC 9116
// requires Expires, and a past date tells reporters the contact is stale.
// When this fails, bump Expires (at most a year ahead).
func TestSecurityTxt_NotExpired(t *testing.T) {
	var expires string
	sc := bufio.NewScanner(strings.NewReader(securityTxt))
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), "Expires: "); ok {
			expires = v
		}
	}
	if expires == "" {
		t.Fatal("security.txt has no Expires field")
	}
	ts, err := time.Parse(time.RFC3339, expires)
	if err != nil {
		t.Fatalf("Expires %q is not RFC 3339: %v", expires, err)
	}
	if !ts.After(time.Now()) {
		t.Errorf("security.txt expired at %s; bump Expires", expires)
	}
	if ts.After(time.Now().AddDate(1, 1, 0)) {
		t.Errorf("Expires %s is more than a year out (RFC 9116 recommends under a year)", expires)
	}
}
