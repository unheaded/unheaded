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
	"unheaded/services/sophia"
)

// newTestServer constructs an HTTPServer wired to a fresh sophia.Service +
// io.Discard logger + nil Wotan. Returns the server (with auth disabled
// since AUTH_ENABLED is unset by default).
func newTestServer(t *testing.T) *HTTPServer {
	t.Helper()
	svc := sophia.NewService(logger.New(io.Discard), nil, nil)
	hs, err := NewHTTPServer(svc, nil, logger.New(io.Discard), ":0")
	if err != nil {
		t.Fatalf("NewHTTPServer: %v", err)
	}
	return hs
}

// ─── NewHTTPServer constructor ─────────────────────────────────────────────

func TestNewHTTPServer_RejectsNilService(t *testing.T) {
	t.Parallel()
	_, err := NewHTTPServer(nil, nil, logger.New(io.Discard), ":0")
	if err == nil {
		t.Fatal("expected error for nil service, got nil")
	}
	if !strings.Contains(err.Error(), "service") {
		t.Errorf("wrong error: %v", err)
	}
}

func TestNewHTTPServer_RejectsNilLogger(t *testing.T) {
	t.Parallel()
	svc := sophia.NewService(logger.New(io.Discard), nil, nil)
	_, err := NewHTTPServer(svc, nil, nil, ":0")
	if err == nil {
		t.Fatal("expected error for nil logger, got nil")
	}
	if !strings.Contains(err.Error(), "logger") {
		t.Errorf("wrong error: %v", err)
	}
}

func TestNewHTTPServer_DefaultsAddrFromPortsRegistry(t *testing.T) {
	t.Parallel()
	svc := sophia.NewService(logger.New(io.Discard), nil, nil)
	hs, err := NewHTTPServer(svc, nil, logger.New(io.Discard), "")
	if err != nil {
		t.Fatalf("NewHTTPServer: %v", err)
	}
	if hs.server.Addr == "" {
		t.Errorf("Addr should be defaulted from pkg/ports, got empty")
	}
}

// ─── healthHandler ──────────────────────────────────────────────────────────

func TestHealthHandler_ReturnsHealthyJSON(t *testing.T) {
	t.Parallel()
	hs := newTestServer(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/health", nil)
	hs.healthHandler(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if body["status"] != "healthy" {
		t.Errorf("status: %v", body["status"])
	}
	if body["service"] != "sophia" {
		t.Errorf("service: %v", body["service"])
	}
}

func TestHealthHandler_RejectsNonGet(t *testing.T) {
	t.Parallel()
	hs := newTestServer(t)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, "/health", nil)
		hs.healthHandler(w, r)
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s /health: status = %d, want 405", method, w.Code)
		}
	}
}

// ─── readyHandler ───────────────────────────────────────────────────────────

func TestReadyHandler_ReturnsServiceUnavailableWhenNotReady(t *testing.T) {
	t.Parallel()
	hs := newTestServer(t)
	// Default ready=false on freshly constructed server.
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ready", nil)
	hs.readyHandler(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("not-ready: status = %d, want 503", w.Code)
	}
}

func TestReadyHandler_ReturnsOKWhenReady(t *testing.T) {
	t.Parallel()
	hs := newTestServer(t)
	hs.ready.Store(true)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ready", nil)
	hs.readyHandler(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("ready: status = %d, want 200", w.Code)
	}
}

func TestReadyHandler_RejectsNonGet(t *testing.T) {
	t.Parallel()
	hs := newTestServer(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/ready", nil)
	hs.readyHandler(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}
