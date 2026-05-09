// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"unheaded/pkg/logger"
	"unheaded/services/shield"
)

func newTestMux(t *testing.T) (*http.ServeMux, *shield.Service) {
	t.Helper()
	svc := shield.NewService(logger.New(io.Discard), nil, nil)
	mux := http.NewServeMux()
	registerControlAPI(mux, svc, logger.New(io.Discard))
	return mux, svc
}

// ─── /api/v1/rules GET (empty) ──────────────────────────────────────────────

func TestRulesGET_EmptyReturnsZeroCount(t *testing.T) {
	t.Parallel()
	mux, _ := newTestMux(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/rules", nil)
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", w.Code)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["count"] != float64(0) {
		t.Errorf("count = %v, want 0", body["count"])
	}
}

// ─── /api/v1/rules POST ─────────────────────────────────────────────────────

func TestRulesPOST_RejectsInvalidJSON(t *testing.T) {
	t.Parallel()
	mux, _ := newTestMux(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/rules", strings.NewReader("not-json"))
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid JSON") {
		t.Errorf("missing 'invalid JSON' in body: %s", w.Body.String())
	}
}

// ─── /api/v1/rules unsupported methods ──────────────────────────────────────

func TestRules_RejectsUnsupportedMethod(t *testing.T) {
	t.Parallel()
	mux, _ := newTestMux(t)
	for _, m := range []string{http.MethodPut, http.MethodPatch} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(m, "/api/v1/rules", nil)
		mux.ServeHTTP(w, r)
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: status %d, want 405", m, w.Code)
		}
		if w.Header().Get("Allow") != "GET, POST" {
			t.Errorf("%s: Allow header = %q, want 'GET, POST'", m, w.Header().Get("Allow"))
		}
	}
}

// ─── /api/v1/rules/<id> DELETE path ─────────────────────────────────────────

func TestRulesDELETE_RequiresID(t *testing.T) {
	t.Parallel()
	mux, _ := newTestMux(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodDelete, "/api/v1/rules/", nil)
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("empty-id status %d, want 400", w.Code)
	}
}

func TestRulesDELETE_RejectsSlashInID(t *testing.T) {
	t.Parallel()
	mux, _ := newTestMux(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodDelete, "/api/v1/rules/foo/bar", nil)
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("nested-id status %d, want 400", w.Code)
	}
}

func TestRulesIDPath_RejectsNonDelete(t *testing.T) {
	t.Parallel()
	mux, _ := newTestMux(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/rules/some-id", nil)
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status %d, want 405", w.Code)
	}
	if w.Header().Get("Allow") != "DELETE" {
		t.Errorf("Allow = %q, want DELETE", w.Header().Get("Allow"))
	}
}

// ─── /api/v1/evaluate ───────────────────────────────────────────────────────

func TestEvaluate_RejectsNonPost(t *testing.T) {
	t.Parallel()
	mux, _ := newTestMux(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/evaluate", nil)
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status %d, want 405", w.Code)
	}
}

func TestEvaluate_RejectsInvalidJSON(t *testing.T) {
	t.Parallel()
	mux, _ := newTestMux(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/evaluate", strings.NewReader("{not-json"))
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status %d, want 400", w.Code)
	}
}

// ─── writeJSON helper ───────────────────────────────────────────────────────

func TestWriteJSON_SetsContentTypeAndStatus(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusTeapot, map[string]string{"k": "v"})
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if w.Code != http.StatusTeapot {
		t.Errorf("status = %d, want 418", w.Code)
	}
	body, _ := io.ReadAll(w.Body)
	if !bytes.Contains(body, []byte(`"k":"v"`)) {
		t.Errorf("body missing JSON: %s", body)
	}
}
