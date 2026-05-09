// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"unheaded/pkg/logger"
	"unheaded/services/monad"
)

func newTestServer(t *testing.T) *HTTPServer {
	t.Helper()
	svc := monad.NewService(logger.New(io.Discard), nil)
	hs, err := NewHTTPServer(svc, logger.New(io.Discard), ":0")
	if err != nil {
		t.Fatalf("NewHTTPServer: %v", err)
	}
	return hs
}

// ─── NewHTTPServer constructor ─────────────────────────────────────────────

func TestNewHTTPServer_RejectsNilService(t *testing.T) {
	t.Parallel()
	_, err := NewHTTPServer(nil, logger.New(io.Discard), ":0")
	if err == nil || !strings.Contains(err.Error(), "service") {
		t.Fatalf("expected service-nil error, got %v", err)
	}
}

func TestNewHTTPServer_RejectsNilLogger(t *testing.T) {
	t.Parallel()
	svc := monad.NewService(logger.New(io.Discard), nil)
	_, err := NewHTTPServer(svc, nil, ":0")
	if err == nil || !strings.Contains(err.Error(), "logger") {
		t.Fatalf("expected logger-nil error, got %v", err)
	}
}

func TestNewHTTPServer_RejectsEmptyAddr(t *testing.T) {
	t.Parallel()
	svc := monad.NewService(logger.New(io.Discard), nil)
	_, err := NewHTTPServer(svc, logger.New(io.Discard), "")
	if err == nil || !strings.Contains(err.Error(), "address") {
		t.Fatalf("expected address-empty error, got %v", err)
	}
}

func TestNewHTTPServer_DefaultsReadyTrue(t *testing.T) {
	t.Parallel()
	hs := newTestServer(t)
	if !hs.ready {
		t.Errorf("ready should default to true on construction")
	}
}

// ─── healthHandler ──────────────────────────────────────────────────────────

func TestHealthHandler_Returns200WithJSONHealthy(t *testing.T) {
	t.Parallel()
	hs := newTestServer(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/health", nil)
	hs.healthHandler(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", w.Code)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if body["status"] != "healthy" {
		t.Errorf("status: %v", body["status"])
	}
	if body["service"] != "monad" {
		t.Errorf("service: %v", body["service"])
	}
}

func TestHealthHandler_RejectsNonGet(t *testing.T) {
	t.Parallel()
	hs := newTestServer(t)
	for _, m := range []string{http.MethodPost, http.MethodDelete} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(m, "/health", nil)
		hs.healthHandler(w, r)
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: status %d, want 405", m, w.Code)
		}
	}
}

// ─── readyHandler ───────────────────────────────────────────────────────────

func TestReadyHandler_Returns200WhenReady(t *testing.T) {
	t.Parallel()
	hs := newTestServer(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ready", nil)
	hs.readyHandler(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", w.Code)
	}
}

func TestReadyHandler_Returns503WhenNotReady(t *testing.T) {
	t.Parallel()
	hs := newTestServer(t)
	hs.mu.Lock()
	hs.ready = false
	hs.mu.Unlock()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ready", nil)
	hs.readyHandler(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("not-ready status = %d, want 503", w.Code)
	}
}

func TestReadyHandler_RejectsNonGet(t *testing.T) {
	t.Parallel()
	hs := newTestServer(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/ready", nil)
	hs.readyHandler(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status %d, want 405", w.Code)
	}
}
