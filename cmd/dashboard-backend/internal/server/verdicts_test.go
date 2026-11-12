// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"unheaded/cmd/dashboard-backend/internal/verdicts"
)

// The inventory is served as it is embedded: every section, every row,
// counts that add up, and no way to cache or write it.
func TestVerdicts_Served(t *testing.T) {
	srv := newTestServer(t)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/verdicts", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	var inv verdicts.Inventory
	if err := json.Unmarshal(rec.Body.Bytes(), &inv); err != nil {
		t.Fatalf("body is not the inventory: %v", err)
	}
	if inv.Source != verdicts.SourceFile || len(inv.Sections) == 0 {
		t.Fatalf("source %q, %d sections", inv.Source, len(inv.Sections))
	}
	rows, byClass := 0, 0
	for _, s := range inv.Sections {
		rows += len(s.Items)
		for _, it := range s.Items {
			if it.Name == "" || it.Class != verdicts.Classify(it.Verdict) {
				t.Errorf("%s: item %+v", s.Name, it)
			}
			if len(it.Fields) != len(s.Columns) {
				t.Errorf("%s / %s: %d fields for %d columns", s.Name, it.Name, len(it.Fields), len(s.Columns))
			}
		}
	}
	for _, c := range []string{verdicts.Keep, verdicts.Replace, verdicts.Cut, verdicts.Other, verdicts.Undecided} {
		byClass += inv.Counts[c]
	}
	if rows == 0 || inv.Counts["total"] != rows || byClass != rows {
		t.Errorf("%d rows, total %d, classes add to %d", rows, inv.Counts["total"], byClass)
	}
}

// Read-only: nothing but GET and HEAD, whoever asks.
func TestVerdicts_ReadOnly(t *testing.T) {
	srv := newTestServer(t)
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, httptest.NewRequest(m, "/api/v1/verdicts", strings.NewReader(`{"verdict":"keep"}`)))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s = %d, want 405", m, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodHead, "/api/v1/verdicts", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("HEAD = %d, want 200", rec.Code)
	}
}

// With auth on, the data needs a login like every other /api/ read; the
// page shell is a UI read and carries no row of the inventory.
func TestVerdicts_NeedsLogin(t *testing.T) {
	t.Setenv("AUTH_ENABLED", "true")
	t.Setenv("AUTH_API_KEYS", "k1:test-only-key")
	srv := newTestServer(t)
	h := srv.httpServer.Handler

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/verdicts", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated /api/v1/verdicts = %d, want 401", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "zerolog") {
		t.Error("the refusal carries inventory rows")
	}
}

// The page and its script are in the embedded static files and name the
// API they read; the three older pages link to it.
func TestVerdicts_PageShipped(t *testing.T) {
	config := DefaultConfig()
	config.WotanAddr = "localhost:19999"
	config.StaticFS = os.DirFS(filepath.Join("..", "..", "static")) // what main embeds
	srv, err := NewServer(config, testLogger())
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	for path, want := range map[string]string{
		"/verdicts":              `<script src="/static/js/verdicts.js"></script>`,
		"/static/js/verdicts.js": `fetch('/api/v1/verdicts')`,
		"/":                      `href="/verdicts"`,
		"/compliance":            `href="/verdicts"`,
		"/logs":                  `href="/verdicts"`,
	} {
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), want) {
			t.Errorf("GET %s = %d, lacks %q", path, rec.Code, want)
		}
	}
}
