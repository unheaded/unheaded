// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Stevie Bellis. All rights reserved.

package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"unheaded/pkg/auth"
)

// With AUTH_ENABLED=true the read-only state endpoints stay open (Sword's
// Ping and steel-echo's discover read them without a key, S-REAL-STEEL
// Phase 5 live), while every container write still needs the key.
func TestAuthWrap_ReadsOpenWritesKeyed(t *testing.T) {
	d := newTestDaemon(t)
	d.authEnabled = true
	mux := http.NewServeMux()
	d.registerHandlers(mux)
	h := d.wrapAuth(mux, auth.ServiceAuthConfig{Enabled: true, APIKeys: []string{"k-test"}, ServiceName: "unheaded-daemon"})

	get := func(path string) int {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec.Code
	}
	for _, p := range []string{"/health", "/api/v1/info", "/api/v1/state", "/api/v1/state/actual"} {
		if c := get(p); c != http.StatusOK {
			t.Errorf("GET %s without a key: %d, want 200 (reads stay open)", p, c)
		}
	}
	if c := get("/api/v1/state/../containers/web"); c == http.StatusOK {
		t.Error("a non-canonical path under a skipped prefix bypassed auth")
	}

	put := func(key string) int {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/containers/web", strings.NewReader(`{"name":"web"}`))
		if key != "" {
			req.Header.Set("X-API-Key", key)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	if c := put(""); c != http.StatusUnauthorized {
		t.Errorf("PUT without a key: %d, want 401", c)
	}
	if c := put("wrong"); c != http.StatusUnauthorized {
		t.Errorf("PUT with a wrong key: %d, want 401", c)
	}
	if c := put("k-test"); c == http.StatusUnauthorized || c == http.StatusForbidden {
		t.Errorf("PUT with the key: %d, want accepted", c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/containers/web", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("GET /api/v1/containers/web without a key: %d, want 401 (only /info and /state are public)", rec.Code)
	}
}
