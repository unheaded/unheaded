// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// SetupMiddleware returns nil when AUTH_ENABLED is not true, and WrapHandler
// is documented to pass a nil middleware through. SkipAuthPaths wrapped that
// nil into a middleware that called it, so Cuirass (unheaded-daemon), which
// composes the two, panicked at start in its default configuration
// (found live 2026-10-27, S-CLEAN-SLATE Step 153).
func TestSkipAuthPaths_NilMiddlewareStaysNil(t *testing.T) {
	if mw := SkipAuthPaths(nil, "/api/v1/info"); mw != nil {
		t.Fatal("SkipAuthPaths(nil) returned a middleware; WrapHandler will call it")
	}
	var served bool
	h := WrapHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { served = true }),
		SkipAuthPaths(nil, "/api/v1/info"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/containers", nil))
	if !served || rec.Code != http.StatusOK {
		t.Fatalf("auth off: request not served (served=%v code=%d)", served, rec.Code)
	}
}

func TestSkipAuthPaths_SetupMiddlewareOffComposes(t *testing.T) {
	t.Setenv("AUTH_ENABLED", "false")
	mw := SkipAuthPaths(SetupMiddleware(ServiceAuthConfig{ServiceName: "t"}), "/api/v1/state")
	h := WrapHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), mw)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil)) // must not panic
	if rec.Code != http.StatusOK {
		t.Fatalf("code %d", rec.Code)
	}
}
