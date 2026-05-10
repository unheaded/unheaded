// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/transport"
	wotanClient "unheaded/pkg/wotan-client"
	"unheaded/pkg/wotan-client/mock"
)

// ============================================================================
// MIDDLEWARE TESTS — corsMiddleware
// ============================================================================

func TestCorsMiddleware_AllowedOrigin_SetsHeaders(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := corsMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil)
	req.Header.Set("Origin", "http://localhost:8080")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Origin") != "http://localhost:8080" {
		t.Errorf("expected CORS origin header, got %q", w.Header().Get("Access-Control-Allow-Origin"))
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestCorsMiddleware_DisallowedOrigin_NoHeaders(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := corsMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil)
	req.Header.Set("Origin", "http://evil.example.com")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("expected no CORS origin header for disallowed origin")
	}
}

func TestCorsMiddleware_NoOrigin_NoHeaders(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := corsMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("expected no CORS origin header when no Origin header sent")
	}
}

func TestCorsMiddleware_PreflightAllowed_Returns204(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("inner handler should not be called on preflight")
	})
	handler := corsMiddleware(inner)

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/tasks", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204 for preflight, got %d", w.Code)
	}
}

func TestCorsMiddleware_PreflightDisallowed_Returns403(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("inner handler should not be called on preflight")
	})
	handler := corsMiddleware(inner)

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/tasks", nil)
	req.Header.Set("Origin", "http://evil.example.com")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for disallowed preflight, got %d", w.Code)
	}
}

func TestCorsMiddleware_PreflightNoOrigin_Returns403(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("inner handler should not be called")
	})
	handler := corsMiddleware(inner)

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/tasks", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for preflight without origin, got %d", w.Code)
	}
}

// ============================================================================
// MIDDLEWARE TESTS — isAllowedOrigin
// ============================================================================

func TestIsAllowedOrigin(t *testing.T) {
	tests := []struct {
		origin string
		want   bool
	}{
		{"http://localhost:8080", true},
		{"http://localhost:3000", true},
		{"http://127.0.0.1:8080", true},
		{"https://localhost:8080", true},
		{"https://localhost:3000", true},
		{"http://evil.com", false},
		{"", false},
	}
	for _, tc := range tests {
		t.Run(tc.origin, func(t *testing.T) {
			got := isAllowedOrigin(tc.origin)
			if got != tc.want {
				t.Errorf("isAllowedOrigin(%q) = %v, want %v", tc.origin, got, tc.want)
			}
		})
	}
}

// ============================================================================
// MIDDLEWARE TESTS — securityHeadersMiddleware
// ============================================================================

func TestSecurityHeadersMiddleware(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := securityHeadersMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	expectedHeaders := map[string]string{
		"X-Frame-Options":        "DENY",
		"X-Content-Type-Options": "nosniff",
		"X-XSS-Protection":       "1; mode=block",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
	}

	for header, expected := range expectedHeaders {
		got := w.Header().Get(header)
		if got != expected {
			t.Errorf("header %s = %q, want %q", header, got, expected)
		}
	}

	csp := w.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Error("expected Content-Security-Policy header")
	}
	hsts := w.Header().Get("Strict-Transport-Security")
	if hsts == "" {
		t.Error("expected Strict-Transport-Security header")
	}
}

// ============================================================================
// MIDDLEWARE TESTS — requestIDMiddleware
// ============================================================================

func TestRequestIDMiddleware_GeneratesID(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := getRequestID(r.Context())
		if id == "" {
			t.Error("expected request ID in context")
		}
		w.WriteHeader(http.StatusOK)
	})
	handler := requestIDMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	respID := w.Header().Get("X-Request-ID")
	if respID == "" {
		t.Error("expected X-Request-ID in response")
	}
}

func TestRequestIDMiddleware_PreservesExistingID(t *testing.T) {
	existingID := "my-custom-id-12345"
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := getRequestID(r.Context())
		if id != existingID {
			t.Errorf("expected preserved ID %q, got %q", existingID, id)
		}
		w.WriteHeader(http.StatusOK)
	})
	handler := requestIDMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", existingID)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Header().Get("X-Request-ID") != existingID {
		t.Errorf("expected preserved X-Request-ID %q in response", existingID)
	}
}

func TestGetRequestID_MissingKey(t *testing.T) {
	ctx := context.Background()
	id := getRequestID(ctx)
	if id != "" {
		t.Errorf("expected empty string for missing key, got %q", id)
	}
}

// ============================================================================
// MIDDLEWARE TESTS — requestSizeLimitMiddleware
// ============================================================================

func TestRequestSizeLimitMiddleware(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 2048)
		_, err := r.Body.Read(buf)
		if err == nil {
			t.Error("expected error reading oversized body")
		}
		w.WriteHeader(http.StatusOK)
	})
	handler := requestSizeLimitMiddleware(1024)(inner)

	// Send a body larger than 1024 bytes
	body := make([]byte, 2048)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)
}

// ============================================================================
// MIDDLEWARE TESTS — RateLimiter
// ============================================================================

func TestRateLimiter_Allow_ValidIP(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rl := NewRateLimiter(ctx, 60, 10)

	if !rl.Allow("192.168.1.1") {
		t.Error("expected first request to be allowed")
	}
}

func TestRateLimiter_Allow_EmptyIP_Denied(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rl := NewRateLimiter(ctx, 60, 10)

	if rl.Allow("") {
		t.Error("expected empty IP to be denied")
	}
}

func TestRateLimiter_Allow_ExhaustsBurst(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rl := NewRateLimiter(ctx, 60, 3) // burst of 3

	for i := 0; i < 3; i++ {
		if !rl.Allow("10.0.0.1") {
			t.Errorf("request %d should be allowed within burst", i+1)
		}
	}

	// Fourth request should be denied (burst exhausted, no time for refill)
	if rl.Allow("10.0.0.1") {
		t.Error("expected request beyond burst to be denied")
	}
}

func TestRateLimiter_Cleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rl := NewRateLimiter(ctx, 60, 10)
	rl.Allow("10.0.0.1")

	// Manually set last refill to >10 minutes ago
	rl.mu.Lock()
	bucket := rl.clients["10.0.0.1"]
	bucket.mu.Lock()
	bucket.lastRefill = time.Now().Add(-15 * time.Minute)
	bucket.mu.Unlock()
	rl.mu.Unlock()

	// Run cleanup
	rl.cleanup()

	rl.mu.RLock()
	_, exists := rl.clients["10.0.0.1"]
	rl.mu.RUnlock()

	if exists {
		t.Error("expected stale client to be cleaned up")
	}
}

func TestRateLimiter_CleanupLoop_CancelsOnContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	rl := NewRateLimiter(ctx, 60, 10)
	_ = rl

	// Cancel should stop the cleanup goroutine
	cancel()
	// Just verifying no goroutine leak; the test passing without timeout is enough.
}

func TestRateLimitMiddleware_AllowsRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	limiter := NewRateLimiter(ctx, 120, 30)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := rateLimitMiddleware(limiter)(inner)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRateLimitMiddleware_DeniesExceeded(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	limiter := NewRateLimiter(ctx, 60, 1) // burst of 1
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := rateLimitMiddleware(limiter)(inner)

	// First request
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("first request should succeed, got %d", w.Code)
	}

	// Second request — should be rate limited
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil)
	req2.RemoteAddr = "10.0.0.1:12345"
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	if w2.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", w2.Code)
	}
}

func TestRateLimitMiddleware_ExemptsStreamingEndpoints(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	limiter := NewRateLimiter(ctx, 60, 0) // zero burst = deny all
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := rateLimitMiddleware(limiter)(inner)

	for _, path := range []string{"/ws", "/api/v1/stream"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.RemoteAddr = "10.0.0.1:12345"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("streaming endpoint %s should bypass rate limit, got %d", path, w.Code)
		}
	}
}

// ============================================================================
// MIDDLEWARE TESTS — getClientIP / stripPort
// ============================================================================

func TestStripPort(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"192.168.1.1:8080", "192.168.1.1"},
		{"[::1]:8080", "[::1]"},
		{"no-port", "no-port"},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := stripPort(tc.input)
			if got != tc.want {
				t.Errorf("stripPort(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestGetClientIP_UsesRemoteAddr(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.100:54321"

	ip := getClientIP(req)
	if ip != "192.168.1.100" {
		t.Errorf("expected 192.168.1.100, got %q", ip)
	}
}

func TestGetClientIP_IgnoresXFFWhenNotTrusted(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.100:54321"
	req.Header.Set("X-Forwarded-For", "10.0.0.1, 10.0.0.2")

	ip := getClientIP(req)
	if ip != "192.168.1.100" {
		t.Errorf("expected RemoteAddr IP 192.168.1.100, got %q", ip)
	}
}

func TestGetClientIP_TrustsXFFFromProxy(t *testing.T) {
	// Temporarily add a trusted proxy
	trustedProxies["10.10.10.1"] = true
	defer func() { delete(trustedProxies, "10.10.10.1") }()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.10.10.1:54321"
	req.Header.Set("X-Forwarded-For", "203.0.113.5, 10.10.10.1")

	ip := getClientIP(req)
	if ip != "203.0.113.5" {
		t.Errorf("expected first XFF IP 203.0.113.5, got %q", ip)
	}
}

func TestGetClientIP_TrustsXRealIPFromProxy(t *testing.T) {
	trustedProxies["10.10.10.2"] = true
	defer func() { delete(trustedProxies, "10.10.10.2") }()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.10.10.2:54321"
	req.Header.Set("X-Real-IP", "203.0.113.10")

	ip := getClientIP(req)
	if ip != "203.0.113.10" {
		t.Errorf("expected X-Real-IP 203.0.113.10, got %q", ip)
	}
}

func TestGetClientIP_TrustedProxy_SingleXFF(t *testing.T) {
	trustedProxies["10.10.10.3"] = true
	defer func() { delete(trustedProxies, "10.10.10.3") }()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.10.10.3:54321"
	req.Header.Set("X-Forwarded-For", "203.0.113.20")

	ip := getClientIP(req)
	if ip != "203.0.113.20" {
		t.Errorf("expected single XFF IP 203.0.113.20, got %q", ip)
	}
}

// ============================================================================
// HANDLER TESTS — handleTaskByID
// ============================================================================

func TestHandleTaskByID_Get_WithStore(t *testing.T) {
	server := NewServer(Config{Port: "0", DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	// Save a task
	task := &Task{ID: "byid-1", Title: "ByID Task", Status: "todo", Type: "task"}
	server.store.SaveTask(task)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/byid-1", nil)
	w := httptest.NewRecorder()
	server.handleTaskByID(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp Task
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ID != "byid-1" {
		t.Errorf("expected task ID byid-1, got %q", resp.ID)
	}
}

func TestHandleTaskByID_Get_NotFound(t *testing.T) {
	server := NewServer(Config{Port: "0", DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/nonexistent", nil)
	w := httptest.NewRecorder()
	server.handleTaskByID(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleTaskByID_InvalidPath(t *testing.T) {
	server := NewServer(Config{Port: "0", DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/wrong/path/", nil)
	w := httptest.NewRecorder()
	server.handleTaskByID(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid path, got %d", w.Code)
	}
}

func TestHandleTaskByID_EmptyID(t *testing.T) {
	server := NewServer(Config{Port: "0", DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/", nil)
	w := httptest.NewRecorder()
	server.handleTaskByID(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty ID, got %d", w.Code)
	}
}

func TestHandleTaskByID_Put_WithStore(t *testing.T) {
	server := NewServer(Config{Port: "0", DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	// Create task first
	task := &Task{ID: "byid-put-1", Title: "Original", Status: "todo", Type: "task", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	server.store.SaveTask(task)

	// Update via PUT
	update := Task{Title: "Updated Title", Status: "in-progress"}
	payload, _ := json.Marshal(update)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/tasks/byid-put-1", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.handleTaskByID(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleTaskByID_Put_NotFound(t *testing.T) {
	server := NewServer(Config{Port: "0", DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	update := Task{Title: "Orphan", Status: "todo"}
	payload, _ := json.Marshal(update)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/tasks/nonexistent", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.handleTaskByID(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleTaskByID_Put_InvalidJSON(t *testing.T) {
	server := NewServer(Config{Port: "0", DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	req := httptest.NewRequest(http.MethodPut, "/api/v1/tasks/some-id", bytes.NewReader([]byte("bad json")))
	w := httptest.NewRecorder()
	server.handleTaskByID(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTaskByID_Delete_WithStore(t *testing.T) {
	server := NewServer(Config{Port: "0", DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	// Create task first
	task := &Task{ID: "byid-del-1", Title: "Delete Me", Status: "todo", Type: "task"}
	server.store.SaveTask(task)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tasks/byid-del-1", nil)
	w := httptest.NewRecorder()
	server.handleTaskByID(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleTaskByID_Delete_NotFound(t *testing.T) {
	server := NewServer(Config{Port: "0", DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tasks/ghost", nil)
	w := httptest.NewRecorder()
	server.handleTaskByID(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleTaskByID_MethodNotAllowed(t *testing.T) {
	server := NewServer(Config{Port: "0", DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/tasks/some-id", nil)
	w := httptest.NewRecorder()
	server.handleTaskByID(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// ============================================================================
// HANDLER TESTS — handleTaskByID with in-memory fallback
// ============================================================================

func TestHandleGetTaskByID_InMemoryFallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := &Server{
		config:     Config{},
		sseClients: make(map[chan []byte]bool),
		tasks: []Task{
			{ID: "mem-1", Title: "Memory Task", Status: "todo"},
		},
		ctx:    ctx,
		cancel: cancel,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/mem-1", nil)
	w := httptest.NewRecorder()
	server.handleTaskByID(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleGetTaskByID_InMemoryFallback_NotFound(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := &Server{
		config:     Config{},
		sseClients: make(map[chan []byte]bool),
		tasks:      []Task{},
		ctx:        ctx,
		cancel:     cancel,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/missing", nil)
	w := httptest.NewRecorder()
	server.handleTaskByID(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleUpdateTaskByID_InMemoryFallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := &Server{
		config:     Config{},
		sseClients: make(map[chan []byte]bool),
		tasks: []Task{
			{ID: "mem-upd-1", Title: "Original", Status: "todo", CreatedAt: time.Now()},
		},
		ctx:    ctx,
		cancel: cancel,
	}

	update := Task{Title: "Updated", Status: "done"}
	payload, _ := json.Marshal(update)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/tasks/mem-upd-1", bytes.NewReader(payload))
	w := httptest.NewRecorder()
	server.handleTaskByID(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleUpdateTaskByID_InMemoryFallback_NotFound(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := &Server{
		config:     Config{},
		sseClients: make(map[chan []byte]bool),
		tasks:      []Task{},
		ctx:        ctx,
		cancel:     cancel,
	}

	update := Task{Title: "Orphan", Status: "todo"}
	payload, _ := json.Marshal(update)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/tasks/nope", bytes.NewReader(payload))
	w := httptest.NewRecorder()
	server.handleTaskByID(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleDeleteTaskByID_InMemoryFallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := &Server{
		config:     Config{},
		sseClients: make(map[chan []byte]bool),
		tasks: []Task{
			{ID: "mem-del-1", Title: "Delete Me", Status: "todo"},
		},
		ctx:    ctx,
		cancel: cancel,
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tasks/mem-del-1", nil)
	w := httptest.NewRecorder()
	server.handleTaskByID(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleDeleteTaskByID_InMemoryFallback_NotFound(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := &Server{
		config:     Config{},
		sseClients: make(map[chan []byte]bool),
		tasks:      []Task{},
		ctx:        ctx,
		cancel:     cancel,
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tasks/ghost", nil)
	w := httptest.NewRecorder()
	server.handleTaskByID(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ============================================================================
// HANDLER TESTS — handleTimeline
// ============================================================================

func TestHandleTimeline_MethodNotAllowed(t *testing.T) {
	server := NewServer(Config{Port: "0", DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/timeline", nil)
	w := httptest.NewRecorder()
	server.handleTimeline(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleTimeline_NoTimeline_ReturnsNilMessage(t *testing.T) {
	server := NewServer(Config{Port: "0", DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline", nil)
	w := httptest.NewRecorder()
	server.handleTimeline(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if _, ok := resp["message"]; !ok {
		t.Error("expected message field when no timeline data")
	}
}

func TestHandleTimeline_WithTimeline(t *testing.T) {
	server := NewServer(Config{Port: "0", DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	// Set timeline data
	tl := &Timeline{
		Version: "1.0.0",
		Status:  "alpha",
		Phases:  []*Phase{{ID: "p1", Name: "Phase 1"}},
	}
	server.timelineManager.UpdateTimeline(tl)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline", nil)
	w := httptest.NewRecorder()
	server.handleTimeline(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["timeline"] == nil {
		t.Error("expected timeline data in response")
	}
}

func TestHandleTimeline_NoTimelineManager(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := &Server{
		config:     Config{},
		sseClients: make(map[chan []byte]bool),
		ctx:        ctx,
		cancel:     cancel,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline", nil)
	w := httptest.NewRecorder()
	server.handleTimeline(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when no timeline manager, got %d", w.Code)
	}
}

// ============================================================================
// HANDLER TESTS — handleTimelineCards
// ============================================================================

func TestHandleTimelineCards_MethodNotAllowed(t *testing.T) {
	server := NewServer(Config{Port: "0", DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/timeline/cards", nil)
	w := httptest.NewRecorder()
	server.handleTimelineCards(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleTimelineCards_FallbackToTasks(t *testing.T) {
	server := NewServer(Config{Port: "0", DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline/cards", nil)
	w := httptest.NewRecorder()
	server.handleTimelineCards(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["source"] == nil {
		t.Error("expected source field in response")
	}
}

func TestHandleTimelineCards_WithTimelineTasks(t *testing.T) {
	server := NewServer(Config{Port: "0", DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	// Load timeline data
	tl := &Timeline{
		Version: "1.0.0",
		Status:  "alpha",
		Milestones: []*Milestone{
			{ID: "ms-1", Name: "Test Milestone", Status: "in_progress"},
		},
	}
	server.timelineManager.UpdateTimeline(tl)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline/cards", nil)
	w := httptest.NewRecorder()
	server.handleTimelineCards(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	source := resp["source"].(string)
	if source != "timeline-http" {
		t.Errorf("expected source 'timeline-http', got %q", source)
	}
}

func TestHandleTimelineCards_InMemoryFallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := &Server{
		config:     Config{},
		sseClients: make(map[chan []byte]bool),
		tasks:      getInitialTasks(),
		ctx:        ctx,
		cancel:     cancel,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline/cards", nil)
	w := httptest.NewRecorder()
	server.handleTimelineCards(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["source"] != "fallback" {
		t.Errorf("expected source 'fallback', got %v", resp["source"])
	}
}

// ============================================================================
// HANDLER TESTS — handleReady
// ============================================================================

func TestHandleReady_StandaloneMode(t *testing.T) {
	server := NewServer(Config{Port: "0", DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()
	server.handleReady(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["ready"] != true {
		t.Error("expected ready=true")
	}
	if resp["reason"] != "standalone mode" {
		t.Errorf("expected reason 'standalone mode', got %v", resp["reason"])
	}
}

// ============================================================================
// HANDLER TESTS — handleMetrics
// ============================================================================

func TestHandleMetrics_StandaloneMode(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := &Server{
		config:     Config{},
		sseClients: make(map[chan []byte]bool),
		tasks:      getInitialTasks(),
		ctx:        ctx,
		cancel:     cancel,
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	server.handleMetrics(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "kanban_tasks_total") {
		t.Error("expected kanban_tasks_total metric")
	}
	if !strings.Contains(body, "kanban_sse_clients_active") {
		t.Error("expected kanban_sse_clients_active metric")
	}
	if !strings.Contains(body, "kanban_wotan_enabled 0") {
		t.Error("expected kanban_wotan_enabled 0 in standalone mode")
	}
}

func TestHandleMetrics_WithTaskManager(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := &Server{
		config:      Config{},
		sseClients:  make(map[chan []byte]bool),
		taskManager: tm,
		ctx:         ctx,
		cancel:      cancel,
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	server.handleMetrics(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "kanban_wotan_enabled 1") {
		t.Error("expected kanban_wotan_enabled 1 with task manager")
	}
}

// ============================================================================
// HANDLER TESTS — handleHealth (additional paths)
// ============================================================================

func TestHandleHealth_WithTimelineData(t *testing.T) {
	server := NewServer(Config{Port: "0", DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	// Set timeline data
	tl := &Timeline{Version: "1.0.0", Status: "alpha"}
	server.timelineManager.UpdateTimeline(tl)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()
	server.handleHealth(w, req)

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["timeline_http"] != true {
		t.Error("expected timeline_http=true when timeline data exists")
	}
}

// ============================================================================
// HANDLER TESTS — handleSSE
// ============================================================================

func TestHandleSSE_NonFlusher_Returns500(t *testing.T) {
	server := NewServer(Config{Port: "0", DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	// httptest.NewRecorder implements Flusher, so we use a custom writer
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stream", nil)
	w := &nonFlusherWriter{header: http.Header{}}

	server.handleSSE(w, req)

	if w.code != http.StatusInternalServerError {
		t.Errorf("expected 500 for non-flusher, got %d", w.code)
	}
}

// nonFlusherWriter is an http.ResponseWriter that does NOT implement http.Flusher
type nonFlusherWriter struct {
	header http.Header
	code   int
	body   bytes.Buffer
}

func (w *nonFlusherWriter) Header() http.Header         { return w.header }
func (w *nonFlusherWriter) Write(b []byte) (int, error) { return w.body.Write(b) }
func (w *nonFlusherWriter) WriteHeader(code int)        { w.code = code }

func TestHandleWebSocket_DelegatesToSSE(t *testing.T) {
	server := NewServer(Config{Port: "0", DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	w := &nonFlusherWriter{header: http.Header{}}

	server.handleWebSocket(w, req)

	// Since it delegates to handleSSE and nonFlusherWriter is used, it should return 500
	if w.code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.code)
	}
}

// ============================================================================
// HANDLER TESTS — standalone (store) Create/Update/Delete paths
// ============================================================================

func TestHandleCreateTask_StoreMode_DuplicateID_Returns409(t *testing.T) {
	server := NewServer(Config{Port: "0", DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	task := &Task{ID: "dup-store-1", Title: "First", Status: "todo", Type: "task"}
	server.store.SaveTask(task)

	payload, _ := json.Marshal(Task{ID: "dup-store-1", Title: "Duplicate", Status: "todo"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewReader(payload))
	w := httptest.NewRecorder()
	server.handleCreateTask(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func TestHandleUpdateTask_StoreMode_HappyPath(t *testing.T) {
	server := NewServer(Config{Port: "0", DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	task := &Task{ID: "upd-store-1", Title: "Original", Status: "todo", Type: "task", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	server.store.SaveTask(task)

	update := Task{ID: "upd-store-1", Title: "Updated", Status: "done"}
	payload, _ := json.Marshal(update)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/tasks", bytes.NewReader(payload))
	w := httptest.NewRecorder()
	server.handleUpdateTask(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleUpdateTask_StoreMode_NotFound(t *testing.T) {
	server := NewServer(Config{Port: "0", DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	update := Task{ID: "ghost", Title: "Ghost", Status: "todo"}
	payload, _ := json.Marshal(update)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/tasks", bytes.NewReader(payload))
	w := httptest.NewRecorder()
	server.handleUpdateTask(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleDeleteTask_StoreMode_HappyPath(t *testing.T) {
	server := NewServer(Config{Port: "0", DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	task := &Task{ID: "del-store-1", Title: "Delete Me", Status: "todo", Type: "task"}
	server.store.SaveTask(task)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tasks?id=del-store-1", nil)
	w := httptest.NewRecorder()
	server.handleDeleteTask(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleDeleteTask_StoreMode_NotFound(t *testing.T) {
	server := NewServer(Config{Port: "0", DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tasks?id=nonexistent", nil)
	w := httptest.NewRecorder()
	server.handleDeleteTask(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ============================================================================
// HANDLER TESTS — in-memory fallback Create/Update/Delete
// ============================================================================

func TestHandleCreateTask_InMemory(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := &Server{
		config:     Config{},
		sseClients: make(map[chan []byte]bool),
		tasks:      []Task{},
		ctx:        ctx,
		cancel:     cancel,
	}

	task := Task{ID: "inmem-1", Title: "Memory Task", Status: "todo"}
	payload, _ := json.Marshal(task)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewReader(payload))
	w := httptest.NewRecorder()
	server.handleCreateTask(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
}

func TestHandleCreateTask_InMemory_Duplicate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := &Server{
		config:     Config{},
		sseClients: make(map[chan []byte]bool),
		tasks:      []Task{{ID: "inmem-dup", Title: "Existing", Status: "todo"}},
		ctx:        ctx,
		cancel:     cancel,
	}

	task := Task{ID: "inmem-dup", Title: "Duplicate", Status: "todo"}
	payload, _ := json.Marshal(task)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewReader(payload))
	w := httptest.NewRecorder()
	server.handleCreateTask(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func TestHandleUpdateTask_InMemory(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := &Server{
		config:     Config{},
		sseClients: make(map[chan []byte]bool),
		tasks:      []Task{{ID: "inmem-upd", Title: "Original", Status: "todo", CreatedAt: time.Now()}},
		ctx:        ctx,
		cancel:     cancel,
	}

	task := Task{ID: "inmem-upd", Title: "Updated", Status: "done"}
	payload, _ := json.Marshal(task)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/tasks", bytes.NewReader(payload))
	w := httptest.NewRecorder()
	server.handleUpdateTask(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleUpdateTask_InMemory_NotFound(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := &Server{
		config:     Config{},
		sseClients: make(map[chan []byte]bool),
		tasks:      []Task{},
		ctx:        ctx,
		cancel:     cancel,
	}

	task := Task{ID: "ghost", Title: "Ghost", Status: "todo"}
	payload, _ := json.Marshal(task)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/tasks", bytes.NewReader(payload))
	w := httptest.NewRecorder()
	server.handleUpdateTask(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleDeleteTask_InMemory(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := &Server{
		config:     Config{},
		sseClients: make(map[chan []byte]bool),
		tasks:      []Task{{ID: "inmem-del", Title: "Delete Me", Status: "todo"}},
		ctx:        ctx,
		cancel:     cancel,
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tasks?id=inmem-del", nil)
	w := httptest.NewRecorder()
	server.handleDeleteTask(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleDeleteTask_InMemory_NotFound(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := &Server{
		config:     Config{},
		sseClients: make(map[chan []byte]bool),
		tasks:      []Task{},
		ctx:        ctx,
		cancel:     cancel,
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tasks?id=ghost", nil)
	w := httptest.NewRecorder()
	server.handleDeleteTask(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ============================================================================
// HANDLER TESTS — handleGetTasks with store fallback
// ============================================================================

func TestHandleGetTasks_StoreOnly(t *testing.T) {
	server := NewServer(Config{Port: "0", DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil)
	w := httptest.NewRecorder()
	server.handleGetTasks(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	count := int(resp["count"].(float64))
	if count == 0 {
		t.Error("expected tasks from seeded store")
	}
}

// ============================================================================
// SERVER LIFECYCLE TESTS
// ============================================================================

func TestNewServerWithTaskManager(t *testing.T) {
	mockClient := mock.NewMockClient(mock.WithAutoApprove())
	broadcast := func(eventType string, data interface{}) {}
	tm, err := NewTaskManager(mockClient, broadcast, nil)
	if err != nil {
		t.Fatalf("NewTaskManager: %v", err)
	}

	cfg := Config{Port: "0", DataDir: t.TempDir()}
	server := NewServerWithTaskManager(cfg, tm, nil)

	if server == nil {
		t.Fatal("expected non-nil server")
	}
	if server.taskManager != tm {
		t.Error("expected task manager to be set")
	}
	if server.sseClients == nil {
		t.Error("expected sseClients map to be initialized")
	}
}

func TestGetTimelineManager_WithTaskManager(t *testing.T) {
	mockClient := mock.NewMockClient(mock.WithAutoApprove())
	broadcast := func(eventType string, data interface{}) {}
	tm, _ := NewTaskManager(mockClient, broadcast, nil)

	cfg := Config{Port: "0", DataDir: t.TempDir()}
	server := NewServerWithTaskManager(cfg, tm, nil)

	got := server.getTimelineManager()
	if got == nil {
		t.Error("expected timeline manager from task manager")
	}
	if got != tm.timelineManager {
		t.Error("expected task manager's timeline manager")
	}
}

func TestGetTimelineManager_Standalone(t *testing.T) {
	server := NewServer(Config{Port: "0", DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	got := server.getTimelineManager()
	if got == nil {
		t.Error("expected standalone timeline manager")
	}
	if got != server.timelineManager {
		t.Error("expected server's standalone timeline manager")
	}
}

// ============================================================================
// LOGGING MIDDLEWARE / statusWriter TESTS
// ============================================================================

func TestLoggingMiddleware(t *testing.T) {
	server := NewServer(Config{Port: "0", DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	handler := server.loggingMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusTeapot {
		t.Errorf("expected 418, got %d", w.Code)
	}
}

func TestStatusWriter_DefaultStatus(t *testing.T) {
	w := httptest.NewRecorder()
	sw := &statusWriter{ResponseWriter: w, status: 200}

	// Write body without explicit WriteHeader — status should remain 200
	sw.Write([]byte("hello"))
	if sw.status != 200 {
		t.Errorf("expected default status 200, got %d", sw.status)
	}
}

func TestStatusWriter_Flush(t *testing.T) {
	w := httptest.NewRecorder()
	sw := &statusWriter{ResponseWriter: w, status: 200}

	// httptest.ResponseRecorder implements Flusher, so Flush should not panic
	sw.Flush()
}

// ============================================================================
// FetchTimelineFromTimeguru TESTS
// ============================================================================

func TestFetchTimelineFromTimeguru_Success(t *testing.T) {
	timeline := &Timeline{
		Version: "1.0.0",
		Status:  "alpha",
		Phases:  []*Phase{{ID: "p1", Name: "Phase 1", Status: "in_progress"}},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"timeline": timeline,
		})
	}))
	defer ts.Close()

	// Parse ts.URL to get host:port
	addr := strings.TrimPrefix(ts.URL, "http://")
	server := NewServer(Config{Port: "0", TimeGuruAddr: addr, DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	err := server.fetchTimelineFromTimeguru()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tl := server.timelineManager.GetTimeline()
	if tl == nil {
		t.Error("expected timeline to be set")
	}
}

func TestFetchTimelineFromTimeguru_ErrorResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	addr := strings.TrimPrefix(ts.URL, "http://")
	server := NewServer(Config{Port: "0", TimeGuruAddr: addr, DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	err := server.fetchTimelineFromTimeguru()
	if err == nil {
		t.Error("expected error for non-200 response")
	}
}

func TestFetchTimelineFromTimeguru_NilTimeline(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"timeline": nil,
		})
	}))
	defer ts.Close()

	addr := strings.TrimPrefix(ts.URL, "http://")
	server := NewServer(Config{Port: "0", TimeGuruAddr: addr, DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	err := server.fetchTimelineFromTimeguru()
	if err == nil {
		t.Error("expected error for nil timeline in response")
	}
}

func TestFetchTimelineFromTimeguru_InvalidJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer ts.Close()

	addr := strings.TrimPrefix(ts.URL, "http://")
	server := NewServer(Config{Port: "0", TimeGuruAddr: addr, DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	err := server.fetchTimelineFromTimeguru()
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestFetchTimelineFromTimeguru_NoTimelineManager(t *testing.T) {
	timeline := &Timeline{Version: "1.0.0", Status: "alpha"}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"timeline": timeline,
		})
	}))
	defer ts.Close()

	addr := strings.TrimPrefix(ts.URL, "http://")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := &Server{
		config:     Config{TimeGuruAddr: addr},
		sseClients: make(map[chan []byte]bool),
		ctx:        ctx,
		cancel:     cancel,
	}

	err := server.fetchTimelineFromTimeguru()
	if err == nil {
		t.Error("expected error when no timeline manager")
	}
}

// ============================================================================
// STORE TESTS — Close + operations on closed store
// ============================================================================

func TestStore_Close_DoubleClose(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(filepath.Join(dir, "double-close.db"))
	if err != nil {
		t.Fatal(err)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("second close should be no-op: %v", err)
	}
}

func TestStore_OperationsOnClosedStore(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(filepath.Join(dir, "closed-ops.db"))
	store.Close()

	if _, err := store.GetAllTasks(); err != ErrStoreClosed {
		t.Errorf("GetAllTasks: expected ErrStoreClosed, got %v", err)
	}
	if _, err := store.GetTask("any"); err != ErrStoreClosed {
		t.Errorf("GetTask: expected ErrStoreClosed, got %v", err)
	}
	if err := store.SaveTask(&Task{ID: "t", Title: "t"}); err != ErrStoreClosed {
		t.Errorf("SaveTask: expected ErrStoreClosed, got %v", err)
	}
	if err := store.DeleteTask("t"); err != ErrStoreClosed {
		t.Errorf("DeleteTask: expected ErrStoreClosed, got %v", err)
	}
	if _, err := store.TaskCount(); err != ErrStoreClosed {
		t.Errorf("TaskCount: expected ErrStoreClosed, got %v", err)
	}
	if _, err := store.SeedIfEmpty(); err != ErrStoreClosed {
		t.Errorf("SeedIfEmpty: expected ErrStoreClosed, got %v", err)
	}
}

// ============================================================================
// WOTAN TESTS — TaskManager.Close
// ============================================================================

func TestTaskManager_Close_NilClient(t *testing.T) {
	tm := &TaskManager{}
	err := tm.Close()
	if err != ErrNilClient {
		t.Errorf("expected ErrNilClient, got %v", err)
	}
}

func TestTaskManager_Close_HappyPath(t *testing.T) {
	mockClient := mock.NewMockClient(mock.WithAutoApprove())
	broadcast := func(eventType string, data interface{}) {}
	tm, _ := NewTaskManager(mockClient, broadcast, nil)

	err := tm.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

// ============================================================================
// WOTAN TESTS — publishTask
// ============================================================================

func TestPublishTask_EmptyTopic(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	err := tm.publishTask(context.Background(), "", createTestTask("t"))
	if err != ErrEmptyTopic {
		t.Errorf("expected ErrEmptyTopic, got %v", err)
	}
}

func TestPublishTask_NilTask(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	err := tm.publishTask(context.Background(), "test.topic", nil)
	if err != ErrInvalidTask {
		t.Errorf("expected ErrInvalidTask, got %v", err)
	}
}

// ============================================================================
// WOTAN TESTS — handleMessage default topic branch
// ============================================================================

func TestHandleMessage_DefaultTopic_NewTask(t *testing.T) {
	tm, _ := createTestTaskManager(t)

	task := createTestTask("default-new")
	payload, _ := json.Marshal(task)

	msg := &wotanClient.Message{
		MessageID: "test-default-1",
		Topic:     "tasks.other", // Not a known specific topic
		Payload:   string(payload),
	}

	if err := tm.handleMessage(msg); err != nil {
		t.Fatalf("handleMessage failed: %v", err)
	}

	_, err := tm.GetTask("default-new")
	if err != nil {
		t.Error("expected task to be created via default branch")
	}
}

func TestHandleMessage_DefaultTopic_ExistingTask(t *testing.T) {
	tm, _ := createTestTaskManager(t)

	task := createTestTask("default-existing")
	tm.addTask(task)

	task.Title = "Updated via default"
	payload, _ := json.Marshal(task)

	msg := &wotanClient.Message{
		MessageID: "test-default-2",
		Topic:     "tasks.other",
		Payload:   string(payload),
	}

	if err := tm.handleMessage(msg); err != nil {
		t.Fatalf("handleMessage failed: %v", err)
	}

	retrieved, _ := tm.GetTask("default-existing")
	if retrieved.Title != "Updated via default" {
		t.Error("expected task to be updated via default branch")
	}
}

func TestHandleMessage_TaskDeleted(t *testing.T) {
	tm, _ := createTestTaskManager(t)

	task := createTestTask("msg-delete")
	tm.addTask(task)

	payload, _ := json.Marshal(task)
	msg := &wotanClient.Message{
		MessageID: "test-del-1",
		Topic:     TopicTasksDeleted,
		Payload:   string(payload),
	}

	if err := tm.handleMessage(msg); err != nil {
		t.Fatalf("handleMessage failed: %v", err)
	}

	_, err := tm.GetTask("msg-delete")
	if err != ErrTaskNotFound {
		t.Error("expected task to be deleted")
	}
}

// ============================================================================
// WOTAN TESTS — handleTimelineMessage
// ============================================================================

func TestHandleTimelineMessage_NilMessage(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	err := tm.handleTimelineMessage(nil)
	if err == nil {
		t.Error("expected error for nil message")
	}
}

func TestHandleTimelineMessage_ValidPayload(t *testing.T) {
	tm, _ := createTestTaskManager(t)

	timeline := Timeline{
		Version: "1.0.0",
		Status:  "alpha",
		Phases:  []*Phase{{ID: "p1", Name: "Phase1", Status: "in_progress"}},
	}
	payload, _ := json.Marshal(timeline)

	msg := &wotanClient.Message{
		MessageID: "tl-msg-1",
		Topic:     TopicTimelineUpdates,
		Seq:       1,
		Payload:   string(payload),
	}

	err := tm.handleTimelineMessage(msg)
	if err != nil {
		t.Fatalf("handleTimelineMessage failed: %v", err)
	}
}

func TestHandleTimelineMessage_NilTimelineManager(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	tm.timelineManager = nil

	msg := &wotanClient.Message{
		MessageID: "tl-msg-nil",
		Topic:     TopicTimelineUpdates,
		Payload:   "{}",
	}

	err := tm.handleTimelineMessage(msg)
	if err != nil {
		t.Error("expected no error when timelineManager is nil")
	}
}

// ============================================================================
// WOTAN TESTS — TaskManager timeline delegation
// ============================================================================

func TestTaskManager_GetTimelineTasks_NilManager(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	tm.timelineManager = nil

	tasks := tm.GetTimelineTasks()
	if tasks != nil {
		t.Error("expected nil when timelineManager is nil")
	}
}

func TestTaskManager_GetTimeline_NilManager(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	tm.timelineManager = nil

	tl := tm.GetTimeline()
	if tl != nil {
		t.Error("expected nil when timelineManager is nil")
	}
}

func TestTaskManager_IsTimelineSubscribed_Default(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	if tm.IsTimelineSubscribed() {
		t.Error("expected false before timeline subscription")
	}
}

// ============================================================================
// WOTAN TESTS — handleMessage with Store persistence
// ============================================================================

func TestHandleMessage_WithStore_CreatedTopic(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(filepath.Join(dir, "msg-store.db"))
	defer store.Close()

	mockClient := mock.NewMockClient(mock.WithAutoApprove())
	broadcast := func(eventType string, data interface{}) {}
	tm, _ := NewTaskManager(mockClient, broadcast, store)

	task := createTestTask("store-msg-create")
	payload, _ := json.Marshal(task)
	msg := &wotanClient.Message{
		MessageID: "store-msg-1",
		Topic:     TopicTasksCreated,
		Payload:   string(payload),
	}

	if err := tm.handleMessage(msg); err != nil {
		t.Fatalf("handleMessage failed: %v", err)
	}

	// Verify persisted to store
	stored, err := store.GetTask("store-msg-create")
	if err != nil {
		t.Fatalf("expected task in store: %v", err)
	}
	if stored.Title != task.Title {
		t.Errorf("store title = %q, want %q", stored.Title, task.Title)
	}
}

// ============================================================================
// TIMELINE TESTS — additional conversion paths
// ============================================================================

func TestConvertMilestones_WithETA(t *testing.T) {
	broadcast := func(eventType string, data interface{}) {}
	tm := NewTimelineManager(broadcast)

	eta := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	timeline := &Timeline{
		Version: "1.0.0",
		Milestones: []*Milestone{
			{ID: "ms-eta", Name: "ETA Test", Status: "in_progress", ETA: eta},
		},
	}

	if err := tm.UpdateTimeline(timeline); err != nil {
		t.Fatalf("UpdateTimeline: %v", err)
	}

	tasks := tm.GetTimelineTasks()
	found := false
	for _, task := range tasks {
		if task.ID == "tl-ms-eta" {
			found = true
			if !strings.Contains(task.Description, "ETA:") {
				t.Error("expected ETA in task description")
			}
		}
	}
	if !found {
		t.Error("expected milestone task tl-ms-eta")
	}
}

func TestConvertMilestones_WithRisk(t *testing.T) {
	broadcast := func(eventType string, data interface{}) {}
	tm := NewTimelineManager(broadcast)

	timeline := &Timeline{
		Version: "1.0.0",
		Milestones: []*Milestone{
			{ID: "ms-risk", Name: "Risk Test", Status: "blocked", Risk: "high"},
		},
	}

	if err := tm.UpdateTimeline(timeline); err != nil {
		t.Fatalf("UpdateTimeline: %v", err)
	}

	tasks := tm.GetTimelineTasks()
	for _, task := range tasks {
		if task.ID == "tl-ms-risk" {
			if !strings.Contains(task.Description, "Risk: High") {
				t.Error("expected Risk in task description")
			}
		}
	}
}

func TestConvertMilestones_SubTasksCompletedParent(t *testing.T) {
	broadcast := func(eventType string, data interface{}) {}
	tm := NewTimelineManager(broadcast)

	timeline := &Timeline{
		Version: "1.0.0",
		Milestones: []*Milestone{
			{
				ID:       "ms-done",
				Name:     "Completed MS",
				Status:   "completed",
				Progress: 100,
				Tasks:    []string{"Sub 1", "Sub 2"},
			},
		},
	}

	if err := tm.UpdateTimeline(timeline); err != nil {
		t.Fatalf("UpdateTimeline: %v", err)
	}

	tasks := tm.GetTimelineTasks()
	for _, task := range tasks {
		if strings.HasPrefix(task.ID, "tl-ms-done-task-") {
			if task.Status != "done" || task.Progress != 100 {
				t.Errorf("sub-task %s: expected done/100, got %s/%d", task.ID, task.Status, task.Progress)
			}
		}
	}
}

func TestConvertMilestones_PhaseParent(t *testing.T) {
	broadcast := func(eventType string, data interface{}) {}
	tm := NewTimelineManager(broadcast)

	timeline := &Timeline{
		Version: "1.0.0",
		Phases: []*Phase{
			{ID: "phase-a", Name: "Alpha Phase", Status: "in_progress", Milestones: []string{"ms-child"}},
		},
		Milestones: []*Milestone{
			{ID: "ms-child", Name: "Child Milestone", Status: "in_progress"},
		},
	}

	if err := tm.UpdateTimeline(timeline); err != nil {
		t.Fatalf("UpdateTimeline: %v", err)
	}

	tasks := tm.GetTimelineTasks()
	for _, task := range tasks {
		if task.ID == "tl-ms-child" {
			if !strings.Contains(task.Description, "Phase: Alpha Phase") {
				t.Errorf("expected phase info in description, got: %s", task.Description)
			}
		}
	}
}

func TestConvertMilestones_PhaseParent_WithDescription(t *testing.T) {
	broadcast := func(eventType string, data interface{}) {}
	tm := NewTimelineManager(broadcast)

	timeline := &Timeline{
		Version: "1.0.0",
		Phases: []*Phase{
			{ID: "phase-b", Name: "Beta Phase", Status: "planned", Milestones: []string{"ms-desc"}},
		},
		Milestones: []*Milestone{
			{ID: "ms-desc", Name: "MS With Desc", Status: "pending", Description: "Existing description"},
		},
	}

	if err := tm.UpdateTimeline(timeline); err != nil {
		t.Fatalf("UpdateTimeline: %v", err)
	}

	tasks := tm.GetTimelineTasks()
	for _, task := range tasks {
		if task.ID == "tl-ms-desc" {
			if !strings.Contains(task.Description, "Phase: Beta Phase") {
				t.Errorf("expected phase prefix in description, got: %s", task.Description)
			}
			if !strings.Contains(task.Description, "Existing description") {
				t.Errorf("expected original description preserved, got: %s", task.Description)
			}
		}
	}
}

// ============================================================================
// TIMELINE TESTS — UpdateTimeline change detection
// ============================================================================

func TestUpdateTimeline_DetectsRemovedTasks(t *testing.T) {
	removedCount := 0
	broadcast := func(eventType string, data interface{}) {
		if eventType == "timeline.task.removed" {
			removedCount++
		}
	}
	tm := NewTimelineManager(broadcast)

	// First update with 2 milestones
	tl1 := &Timeline{
		Version: "1.0.0",
		Milestones: []*Milestone{
			{ID: "ms-a", Name: "A", Status: "in_progress"},
			{ID: "ms-b", Name: "B", Status: "pending"},
		},
	}
	tm.UpdateTimeline(tl1)

	// Second update with only 1 milestone — ms-b removed
	tl2 := &Timeline{
		Version: "1.0.1",
		Milestones: []*Milestone{
			{ID: "ms-a", Name: "A", Status: "completed"},
		},
	}
	tm.UpdateTimeline(tl2)

	if removedCount == 0 {
		t.Error("expected removed task broadcast")
	}
}

func TestUpdateTimeline_DetectsUpdatedTasks(t *testing.T) {
	updatedCount := 0
	broadcast := func(eventType string, data interface{}) {
		if eventType == "timeline.task.updated" {
			updatedCount++
		}
	}
	tm := NewTimelineManager(broadcast)

	tl1 := &Timeline{
		Version:    "1.0.0",
		Milestones: []*Milestone{{ID: "ms-u", Name: "Original", Status: "pending", Progress: 0}},
	}
	tm.UpdateTimeline(tl1)

	tl2 := &Timeline{
		Version:    "1.0.1",
		Milestones: []*Milestone{{ID: "ms-u", Name: "Updated", Status: "in_progress", Progress: 50}},
	}
	tm.UpdateTimeline(tl2)

	if updatedCount == 0 {
		t.Error("expected updated task broadcast")
	}
}

// ============================================================================
// TIMELINE TESTS — HandleTimelineUpdate invalid JSON
// ============================================================================

func TestHandleTimelineUpdate_InvalidJSON(t *testing.T) {
	tm := NewTimelineManager(nil)
	err := tm.HandleTimelineUpdate([]byte("{invalid"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// ============================================================================
// WOTAN TESTS — UpdateTask with nil context
// ============================================================================

func TestTaskManager_UpdateTask_NilContext(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	task := createTestTask("nil-ctx-task")
	tm.addTask(task)

	task.Title = "Updated"
	err := tm.UpdateTask(nil, task) //nolint:staticcheck // intentional nil-context coverage
	if err != nil {
		t.Fatalf("expected no error with nil context, got %v", err)
	}
}

func TestTaskManager_DeleteTask_NilContext(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	task := createTestTask("nil-ctx-del")
	tm.addTask(task)

	err := tm.DeleteTask(nil, "nil-ctx-del") //nolint:staticcheck // intentional nil-context coverage
	if err != nil {
		t.Fatalf("expected no error with nil context, got %v", err)
	}
}

func TestTaskManager_CreateTask_NilContext(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	task := createTestTask("nil-ctx-create")

	err := tm.CreateTask(nil, task) //nolint:staticcheck // intentional nil-context coverage
	if err != nil {
		t.Fatalf("expected no error with nil context, got %v", err)
	}
}

// ============================================================================
// TASK CHANGED — additional branches
// ============================================================================

func TestTaskChanged_DifferentDescription(t *testing.T) {
	a := &Task{ID: "1", Title: "T", Description: "old", Status: "todo"}
	b := &Task{ID: "1", Title: "T", Description: "new", Status: "todo"}
	if !taskChanged(a, b) {
		t.Error("expected changed for different descriptions")
	}
}

func TestTaskChanged_DifferentStatus(t *testing.T) {
	a := &Task{ID: "1", Title: "T", Status: "todo"}
	b := &Task{ID: "1", Title: "T", Status: "done"}
	if !taskChanged(a, b) {
		t.Error("expected changed for different statuses")
	}
}

func TestTaskChanged_DifferentProgress(t *testing.T) {
	a := &Task{ID: "1", Title: "T", Status: "todo", Progress: 0}
	b := &Task{ID: "1", Title: "T", Status: "todo", Progress: 50}
	if !taskChanged(a, b) {
		t.Error("expected changed for different progress")
	}
}

func TestTaskChanged_DifferentOwner(t *testing.T) {
	a := &Task{ID: "1", Title: "T", Status: "todo", Owner: "alice"}
	b := &Task{ID: "1", Title: "T", Status: "todo", Owner: "bob"}
	if !taskChanged(a, b) {
		t.Error("expected changed for different owners")
	}
}

// ============================================================================
// TASK MAP TO SLICE
// ============================================================================

func TestTaskMapToSlice_Empty(t *testing.T) {
	result := taskMapToSlice(map[string]*Task{})
	if len(result) != 0 {
		t.Errorf("expected empty slice, got %d", len(result))
	}
}

func TestTaskMapToSlice_Multiple(t *testing.T) {
	tasks := map[string]*Task{
		"a": {ID: "a"},
		"b": {ID: "b"},
		"c": {ID: "c"},
	}
	result := taskMapToSlice(tasks)
	if len(result) != 3 {
		t.Errorf("expected 3 tasks, got %d", len(result))
	}
}

// ============================================================================
// WOTAN TESTS — internal helpers
// ============================================================================

func TestAddTask_NilTask(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	// Should not panic
	tm.addTask(nil)
}

func TestAddTask_EmptyID(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	tm.addTask(&Task{})
	if len(tm.tasks) != 0 {
		t.Error("expected empty map after adding task with empty ID")
	}
}

func TestUpdateTask_NilTask(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	// Should not panic
	tm.updateTask(nil)
}

func TestDeleteTask_EmptyID_Helper(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	// Should not panic
	tm.deleteTask("")
}

func TestGetTask_EmptyID_Helper(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	task := tm.getTask("")
	if task != nil {
		t.Error("expected nil for empty ID")
	}
}

func TestTaskExists_EmptyID(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	if tm.taskExists("") {
		t.Error("expected false for empty ID")
	}
}

// ============================================================================
// WOTAN TESTS — validate task additional cases
// ============================================================================

func TestValidateTask_ReviewStatus(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	task := &Task{ID: "review-1", Title: "Review Task", Status: "review"}
	if err := tm.validateTask(task); err != nil {
		t.Errorf("review status should be valid, got: %v", err)
	}
}

// ============================================================================
// INITIALIZE WITH STORE TESTS
// ============================================================================

func TestTaskManager_Initialize_WithStore(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(filepath.Join(dir, "init-store.db"))
	defer store.Close()

	mockClient := mock.NewMockClient(mock.WithAutoApprove())
	broadcast := func(eventType string, data interface{}) {}
	tm, _ := NewTaskManager(mockClient, broadcast, store)

	if err := tm.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize with store: %v", err)
	}

	tasks := tm.GetAllTasks()
	if len(tasks) == 0 {
		t.Error("expected tasks loaded from store")
	}
}

// ============================================================================
// HANDLER — handleCreateTask with TaskManager internal error
// ============================================================================

func TestHandleUpdateTask_InvalidStatus(t *testing.T) {
	tm, mockClient := createTestTaskManager(t)
	mockClient.ApproveSubscription(TopicTasks)
	server := &Server{taskManager: tm}

	ctx := context.Background()
	tm.Initialize(ctx)

	task := Task{ID: "bad-status", Title: "Task", Status: "invalid-status"}
	payload, _ := json.Marshal(task)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/tasks", bytes.NewReader(payload))
	w := httptest.NewRecorder()

	server.handleUpdateTask(w, req)

	// Should get 404 since the task doesn't exist, or 400 if validation fails first
	if w.Code == http.StatusOK {
		t.Error("expected non-200 status for invalid task")
	}
}

// ============================================================================
// NewServer — with empty DataDir
// ============================================================================

func TestNewServer_EmptyDataDir(t *testing.T) {
	// Clean up default ./data dir if it gets created
	server := NewServer(Config{Port: "0", DataDir: ""})
	if server == nil {
		t.Fatal("expected non-nil server")
	}
	// Cleanup: close store if it was created
	if server.store != nil {
		server.store.Close()
	}
}

// ============================================================================
// CONCURRENCY — RateLimiter double-check locking
// ============================================================================

func TestRateLimiter_ConcurrentNewClients(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rl := NewRateLimiter(ctx, 120, 30)

	done := make(chan bool, 50)
	for i := 0; i < 50; i++ {
		go func(n int) {
			ip := fmt.Sprintf("10.0.%d.%d", n/256, n%256)
			rl.Allow(ip)
			done <- true
		}(i)
	}

	for i := 0; i < 50; i++ {
		<-done
	}
}

// ============================================================================
// getInitialTasks
// ============================================================================

func TestGetInitialTasks_ReturnsNonEmpty(t *testing.T) {
	tasks := getInitialTasks()
	if len(tasks) == 0 {
		t.Error("expected non-empty initial tasks")
	}

	// Check a few expected tasks exist
	ids := make(map[string]bool)
	for _, task := range tasks {
		ids[task.ID] = true
	}
	if !ids["ms-phase0"] {
		t.Error("expected ms-phase0 in initial tasks")
	}
	if !ids["svc-wotan"] {
		t.Error("expected svc-wotan in initial tasks")
	}
}

// ============================================================================
// HANDLER — handleUpdateTask with MissingID
// ============================================================================

func TestHandleUpdateTask_MissingID_Returns400(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	server := &Server{taskManager: tm}

	task := Task{Title: "No ID", Status: "todo"}
	payload, _ := json.Marshal(task)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/tasks", bytes.NewReader(payload))
	w := httptest.NewRecorder()

	server.handleUpdateTask(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ============================================================================
// HANDLER — handleDeleteTask with empty query param
// ============================================================================

func TestHandleDeleteTask_EmptyQueryParam_Returns400(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	server := &Server{taskManager: tm}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tasks?id=", nil)
	w := httptest.NewRecorder()

	server.handleDeleteTask(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ============================================================================
// HANDLER TESTS — handleSSE with real flusher (SSE streaming)
// ============================================================================

func TestHandleSSE_SendsInitialData_AndPing(t *testing.T) {
	server := NewServer(Config{Port: "0", DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	// Create a request with cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stream", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	done := make(chan bool)
	go func() {
		server.handleSSE(w, req)
		done <- true
	}()

	// Give SSE handler time to write initial data
	time.Sleep(50 * time.Millisecond)
	cancel() // Cancel to stop SSE loop
	<-done

	body := w.Body.String()
	if !strings.Contains(body, "event: tasks") {
		t.Error("expected initial 'event: tasks' in SSE output")
	}
}

func TestHandleSSE_WithTaskManager(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	ctx := context.Background()
	tm.Initialize(ctx)

	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()
	server := &Server{
		config:      Config{},
		sseClients:  make(map[chan []byte]bool),
		taskManager: tm,
		ctx:         serverCtx,
		cancel:      serverCancel,
	}

	reqCtx, reqCancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stream", nil).WithContext(reqCtx)
	w := httptest.NewRecorder()

	done := make(chan bool)
	go func() {
		server.handleSSE(w, req)
		done <- true
	}()

	time.Sleep(50 * time.Millisecond)
	reqCancel()
	<-done

	body := w.Body.String()
	if !strings.Contains(body, "event: tasks") {
		t.Error("expected initial 'event: tasks' in SSE output with task manager")
	}
}

func TestHandleSSE_ReceivesBroadcast(t *testing.T) {
	server := NewServer(Config{Port: "0", DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	reqCtx, reqCancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stream", nil).WithContext(reqCtx)
	w := httptest.NewRecorder()

	done := make(chan bool)
	go func() {
		server.handleSSE(w, req)
		done <- true
	}()

	// Wait for SSE handler to register client
	time.Sleep(50 * time.Millisecond)

	// Broadcast an update
	server.broadcastUpdate("test.event", map[string]string{"key": "value"})

	// Give time for message delivery
	time.Sleep(50 * time.Millisecond)
	reqCancel()
	<-done

	body := w.Body.String()
	if !strings.Contains(body, "test.event") {
		t.Error("expected broadcast message in SSE output")
	}
}

func TestHandleSSE_ChannelClosed(t *testing.T) {
	server := NewServer(Config{Port: "0", DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	reqCtx, reqCancel := context.WithCancel(context.Background())
	defer reqCancel()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stream", nil).WithContext(reqCtx)
	w := httptest.NewRecorder()

	done := make(chan bool)
	go func() {
		server.handleSSE(w, req)
		done <- true
	}()

	// Wait for handler to register
	time.Sleep(50 * time.Millisecond)

	// Find and close the client channel
	server.sseMu.Lock()
	for ch := range server.sseClients {
		close(ch)
		delete(server.sseClients, ch)
	}
	server.sseMu.Unlock()

	// SSE handler should exit when channel is closed
	select {
	case <-done:
		// Good - handler exited
	case <-time.After(1 * time.Second):
		t.Error("SSE handler did not exit after channel closed")
		reqCancel()
	}
}

// ============================================================================
// HANDLER TESTS — handleReady with HealthServer
// ============================================================================

func TestHandleReady_WithHealthServer_Up(t *testing.T) {
	server := NewServer(Config{Port: "0", DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	// Wire a health server
	hs := transport.NewHealthServer("kanban-app")
	server.healthSrv = hs

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()
	server.handleReady(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["ready"] != true {
		t.Error("expected ready=true")
	}
}

func TestHandleReady_WithHealthServer_Down(t *testing.T) {
	server := NewServer(Config{Port: "0", DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	hs := transport.NewHealthServer("kanban-app")
	hs.SetStatus(transport.StatusDown)
	server.healthSrv = hs

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()
	server.handleReady(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["ready"] != false {
		t.Error("expected ready=false")
	}
}

func TestHandleReady_WithTaskManager(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := &Server{
		config:      Config{},
		sseClients:  make(map[chan []byte]bool),
		taskManager: tm,
		ctx:         ctx,
		cancel:      cancel,
	}

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()
	server.handleReady(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["reason"] != "ready" {
		t.Errorf("expected reason 'ready', got %v", resp["reason"])
	}
}

// ============================================================================
// HANDLER TESTS — handleTaskByID with TaskManager
// ============================================================================

func TestHandleGetTaskByID_WithTaskManager(t *testing.T) {
	tm, mockClient := createTestTaskManager(t)
	mockClient.ApproveSubscription(TopicTasks)
	ctx := context.Background()
	tm.Initialize(ctx)

	mockClient.Subscribe(ctx, TopicTasksCreated, "test-pub")
	mockClient.ApproveSubscription(TopicTasksCreated)

	task := createTestTask("tm-byid-get")
	tm.CreateTask(ctx, task)
	tm.inflight.Wait()

	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()
	server := &Server{
		config:      Config{},
		sseClients:  make(map[chan []byte]bool),
		taskManager: tm,
		ctx:         serverCtx,
		cancel:      serverCancel,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/tm-byid-get", nil)
	w := httptest.NewRecorder()
	server.handleTaskByID(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleGetTaskByID_WithTaskManager_NotFound(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := &Server{
		config:      Config{},
		sseClients:  make(map[chan []byte]bool),
		taskManager: tm,
		ctx:         ctx,
		cancel:      cancel,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/nonexistent", nil)
	w := httptest.NewRecorder()
	server.handleTaskByID(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleUpdateTaskByID_WithTaskManager(t *testing.T) {
	tm, mockClient := createTestTaskManager(t)
	mockClient.ApproveSubscription(TopicTasks)
	ctx := context.Background()
	tm.Initialize(ctx)

	mockClient.Subscribe(ctx, TopicTasksCreated, "test-pub")
	mockClient.ApproveSubscription(TopicTasksCreated)
	mockClient.Subscribe(ctx, TopicTasksUpdated, "test-pub")
	mockClient.ApproveSubscription(TopicTasksUpdated)

	task := createTestTask("tm-byid-upd")
	tm.CreateTask(ctx, task)
	tm.inflight.Wait()

	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()
	server := &Server{
		config:      Config{},
		sseClients:  make(map[chan []byte]bool),
		taskManager: tm,
		ctx:         serverCtx,
		cancel:      serverCancel,
	}

	update := Task{Title: "Updated By ID", Status: "done"}
	payload, _ := json.Marshal(update)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/tasks/tm-byid-upd", bytes.NewReader(payload))
	w := httptest.NewRecorder()
	server.handleTaskByID(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleUpdateTaskByID_WithTaskManager_NotFound(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := &Server{
		config:      Config{},
		sseClients:  make(map[chan []byte]bool),
		taskManager: tm,
		ctx:         ctx,
		cancel:      cancel,
	}

	update := Task{Title: "Ghost", Status: "todo"}
	payload, _ := json.Marshal(update)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/tasks/nonexistent", bytes.NewReader(payload))
	w := httptest.NewRecorder()
	server.handleTaskByID(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleDeleteTaskByID_WithTaskManager(t *testing.T) {
	tm, mockClient := createTestTaskManager(t)
	mockClient.ApproveSubscription(TopicTasks)
	ctx := context.Background()
	tm.Initialize(ctx)

	mockClient.Subscribe(ctx, TopicTasksCreated, "test-pub")
	mockClient.ApproveSubscription(TopicTasksCreated)
	mockClient.Subscribe(ctx, TopicTasksDeleted, "test-pub")
	mockClient.ApproveSubscription(TopicTasksDeleted)

	task := createTestTask("tm-byid-del")
	tm.CreateTask(ctx, task)
	tm.inflight.Wait()

	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()
	server := &Server{
		config:      Config{},
		sseClients:  make(map[chan []byte]bool),
		taskManager: tm,
		ctx:         serverCtx,
		cancel:      serverCancel,
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tasks/tm-byid-del", nil)
	w := httptest.NewRecorder()
	server.handleTaskByID(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleDeleteTaskByID_WithTaskManager_NotFound(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := &Server{
		config:      Config{},
		sseClients:  make(map[chan []byte]bool),
		taskManager: tm,
		ctx:         ctx,
		cancel:      cancel,
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tasks/nonexistent", nil)
	w := httptest.NewRecorder()
	server.handleTaskByID(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ============================================================================
// HANDLER TESTS — handleTimelineCards with TaskManager
// ============================================================================

func TestHandleTimelineCards_WithTaskManager_NoTimeline(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	ctx := context.Background()
	tm.Initialize(ctx)

	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()
	server := &Server{
		config:      Config{},
		sseClients:  make(map[chan []byte]bool),
		taskManager: tm,
		ctx:         serverCtx,
		cancel:      serverCancel,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline/cards", nil)
	w := httptest.NewRecorder()
	server.handleTimelineCards(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	// Should fall back to regular tasks from TaskManager
	source := resp["source"].(string)
	if source != "fallback" {
		t.Errorf("expected source 'fallback', got %q", source)
	}
}

func TestHandleTimelineCards_WithTaskManager_WithTimeline(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	ctx := context.Background()
	tm.Initialize(ctx)

	// Load timeline into TaskManager's timeline manager
	tl := &Timeline{
		Version:    "1.0.0",
		Milestones: []*Milestone{{ID: "tm-ms-1", Name: "TM Milestone", Status: "pending"}},
	}
	tm.timelineManager.UpdateTimeline(tl)

	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()
	server := &Server{
		config:      Config{},
		sseClients:  make(map[chan []byte]bool),
		taskManager: tm,
		ctx:         serverCtx,
		cancel:      serverCancel,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline/cards", nil)
	w := httptest.NewRecorder()
	server.handleTimelineCards(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	source := resp["source"].(string)
	if source != "timeline-wotan" {
		t.Errorf("expected source 'timeline-wotan', got %q", source)
	}
}

// ============================================================================
// PollTimeguru TEST
// ============================================================================

func TestPollTimeguru_InitialFetch_Success(t *testing.T) {
	timeline := &Timeline{
		Version: "1.0.0",
		Status:  "alpha",
		Phases:  []*Phase{{ID: "p1", Name: "Phase 1", Status: "in_progress"}},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"timeline": timeline})
	}))
	defer ts.Close()

	addr := strings.TrimPrefix(ts.URL, "http://")
	server := NewServer(Config{Port: "0", TimeGuruAddr: addr, DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan bool)
	go func() {
		server.pollTimeguru(ctx, 1*time.Hour) // Long interval - we'll cancel before tick
		done <- true
	}()

	// Give time for initial fetch
	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done

	tl := server.timelineManager.GetTimeline()
	if tl == nil {
		t.Error("expected timeline to be set after initial poll")
	}
}

func TestPollTimeguru_InitialFetch_Failure(t *testing.T) {
	// Server that always fails
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	defer ts.Close()

	addr := strings.TrimPrefix(ts.URL, "http://")
	server := NewServer(Config{Port: "0", TimeGuruAddr: addr, DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan bool)
	go func() {
		server.pollTimeguru(ctx, 1*time.Hour)
		done <- true
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done
	// Just verify it doesn't panic
}

// ============================================================================
// HANDLER TESTS — handleHealth with TaskManager
// ============================================================================

func TestHandleHealth_WithTaskManager(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := &Server{
		config:      Config{TimeGuruAddr: "localhost:19000"},
		sseClients:  make(map[chan []byte]bool),
		taskManager: tm,
		ctx:         ctx,
		cancel:      cancel,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()
	server.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["wotan_enabled"] != true {
		t.Error("expected wotan_enabled=true")
	}
}

// ============================================================================
// WOTAN TESTS — CreateTask/UpdateTask/DeleteTask with Store
// ============================================================================

func TestTaskManager_CreateTask_WithStore(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(filepath.Join(dir, "create-store.db"))
	defer store.Close()

	mockClient := mock.NewMockClient(mock.WithAutoApprove())
	broadcast := func(eventType string, data interface{}) {}
	tm, _ := NewTaskManager(mockClient, broadcast, store)
	tm.Initialize(context.Background())

	task := createTestTask("store-create")
	if err := tm.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	tm.inflight.Wait()

	// Verify in store
	stored, err := store.GetTask("store-create")
	if err != nil {
		t.Fatalf("expected task in store: %v", err)
	}
	if stored.Title != task.Title {
		t.Errorf("title mismatch")
	}
}

func TestTaskManager_UpdateTask_WithStore(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(filepath.Join(dir, "update-store.db"))
	defer store.Close()

	mockClient := mock.NewMockClient(mock.WithAutoApprove())
	broadcast := func(eventType string, data interface{}) {}
	tm, _ := NewTaskManager(mockClient, broadcast, store)
	tm.Initialize(context.Background())

	task := createTestTask("store-update")
	tm.CreateTask(context.Background(), task)
	tm.inflight.Wait()

	task.Title = "Updated in Store"
	task.Progress = 77
	tm.UpdateTask(context.Background(), task)
	tm.inflight.Wait()

	stored, _ := store.GetTask("store-update")
	if stored.Title != "Updated in Store" {
		t.Errorf("expected updated title in store")
	}
}

func TestTaskManager_DeleteTask_WithStore(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(filepath.Join(dir, "delete-store.db"))
	defer store.Close()

	mockClient := mock.NewMockClient(mock.WithAutoApprove())
	broadcast := func(eventType string, data interface{}) {}
	tm, _ := NewTaskManager(mockClient, broadcast, store)
	tm.Initialize(context.Background())

	task := createTestTask("store-delete")
	tm.CreateTask(context.Background(), task)
	tm.inflight.Wait()

	tm.DeleteTask(context.Background(), "store-delete")
	tm.inflight.Wait()

	_, err := store.GetTask("store-delete")
	if err != ErrStoreNotFound {
		t.Error("expected task deleted from store")
	}
}

// ============================================================================
// HANDLER TESTS — handleTasks POST/PUT/DELETE dispatch
// ============================================================================

func TestHandleTasks_Post_Dispatch(t *testing.T) {
	server := NewServer(Config{Port: "0", DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	task := Task{ID: "dispatch-post", Title: "Dispatch", Status: "todo"}
	payload, _ := json.Marshal(task)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewReader(payload))
	w := httptest.NewRecorder()
	server.handleTasks(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
}

func TestHandleTasks_Delete_Dispatch(t *testing.T) {
	server := NewServer(Config{Port: "0", DataDir: t.TempDir()})
	t.Cleanup(func() {
		if server.store != nil {
			server.store.Close()
		}
	})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tasks?id=nonexistent", nil)
	w := httptest.NewRecorder()
	server.handleTasks(w, req)

	// 404 because task doesn't exist, but the dispatch worked
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 (task not found), got %d", w.Code)
	}
}
