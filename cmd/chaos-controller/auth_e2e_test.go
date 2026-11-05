// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/auth"
)

// M8: with auth on, an observer key reads the controller and starts nothing.
func TestHandler_WritesNeedAWriteRole(t *testing.T) {
	keys := filepath.Join(t.TempDir(), "apikeys.jsonl")
	store, err := auth.NewFileAPIKeyStore(keys)
	if err != nil {
		t.Fatal(err)
	}
	observer, _, err := store.Create(context.Background(), "watcher", auth.RoleObserver, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	operator, _, err := store.Create(context.Background(), "hand", auth.RoleOperator, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, true)
	guarded, err := handler(h.a, auth.ServiceAuthConfig{Enabled: true, ServiceName: serviceName, APIKeyStore: "file:" + keys})
	if err != nil {
		t.Fatal(err)
	}
	do := func(method, path, body, key string) int {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		r.Header.Set("X-API-Key", key)
		rec := httptest.NewRecorder()
		guarded.ServeHTTP(rec, r)
		return rec.Code
	}

	if got := do(http.MethodGet, "/api/v1/chaos/experiments", "", observer); got != http.StatusOK {
		t.Errorf("observer read = %d, want 200", got)
	}
	for _, path := range []string{"/api/v1/chaos/experiments", "/api/v1/chaos/inject", "/api/v1/chaos/gameday", "/api/v1/chaos/abort"} {
		if got := do(http.MethodPost, path, body("1s"), observer); got != http.StatusForbidden {
			t.Errorf("observer POST %s = %d, want 403", path, got)
		}
	}
	if got := do(http.MethodPost, "/api/v1/chaos/experiments", body("1s"), operator); got != http.StatusCreated {
		t.Errorf("operator start = %d, want 201", got)
	}
}

// Auth on with no way to authenticate anyone must stop the start.
func TestHandler_UnusableAuthIsAnError(t *testing.T) {
	h := newHarness(t, true)
	if _, err := handler(h.a, auth.ServiceAuthConfig{Enabled: true, ServiceName: serviceName}); err == nil {
		t.Fatal("handler built with auth on and no credentials")
	}
}
