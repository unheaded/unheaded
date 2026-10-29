// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The ServeMux router that replaced gorilla/mux keeps every route GET-only
// and answers other methods with 405 + Allow, and unknown paths with 404.
func TestNewRouterMethodsAndPaths(t *testing.T) {
	r := newRouter()
	for _, path := range []string{"/health", "/ready", "/metrics", "/api/v1/timeline", "/timeline"} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("POST %s = %d, want 405", path, rec.Code)
		}
		if allow := rec.Header().Get("Allow"); allow == "" {
			t.Errorf("POST %s: no Allow header", path)
		}
		h, pattern := r.Handler(httptest.NewRequest(http.MethodGet, path, nil))
		if h == nil || pattern != "GET "+path {
			t.Errorf("GET %s routed to pattern %q", path, pattern)
		}
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/extra", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /health/extra = %d, want 404 (exact match, as gorilla had)", rec.Code)
	}
}
