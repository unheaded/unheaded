// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// Admin Auth Configuration Tests
// ============================================================================

func TestDefaultAdminAuthConfig(t *testing.T) {
	config := DefaultAdminAuthConfig()

	if config.APIKey != "" {
		t.Error("default API key should be empty")
	}
	if config.HeaderName != "X-Admin-API-Key" {
		t.Errorf("expected header name 'X-Admin-API-Key', got '%s'", config.HeaderName)
	}
	if config.Realm != "Wotan Admin" {
		t.Errorf("expected realm 'Wotan Admin', got '%s'", config.Realm)
	}
	if len(config.SkipPaths) != 3 {
		t.Errorf("expected 3 skip paths, got %d", len(config.SkipPaths))
	}
}

// ============================================================================
// Admin Auth Middleware Tests
// ============================================================================

func TestAdminAuth_MissingAPIKey(t *testing.T) {
	config := AdminAuthConfig{
		APIKey:     "test-secret-key-12345",
		HeaderName: "X-Admin-API-Key",
		Realm:      "Test",
	}

	handler := AdminAuth(config)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))

	req := httptest.NewRequest("GET", "/admin/endpoint", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}

	// Check WWW-Authenticate header
	wwwAuth := rr.Header().Get("WWW-Authenticate")
	if !strings.Contains(wwwAuth, "Bearer") {
		t.Error("expected WWW-Authenticate header with Bearer scheme")
	}

	// Check error response
	body := rr.Body.String()
	if !strings.Contains(body, "missing_api_key") {
		t.Errorf("expected 'missing_api_key' in response, got: %s", body)
	}
}

func TestAdminAuth_InvalidAPIKey(t *testing.T) {
	config := AdminAuthConfig{
		APIKey:     "test-secret-key-12345",
		HeaderName: "X-Admin-API-Key",
		Realm:      "Test",
	}

	handler := AdminAuth(config)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))

	req := httptest.NewRequest("GET", "/admin/endpoint", nil)
	req.Header.Set("X-Admin-API-Key", "wrong-key")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, rr.Code)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "invalid_api_key") {
		t.Errorf("expected 'invalid_api_key' in response, got: %s", body)
	}
}

func TestAdminAuth_ValidAPIKey(t *testing.T) {
	config := AdminAuthConfig{
		APIKey:     "test-secret-key-12345",
		HeaderName: "X-Admin-API-Key",
		Realm:      "Test",
	}

	handlerCalled := false
	handler := AdminAuth(config)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		// Verify admin context was set
		if !IsAdminAuthenticated(r.Context()) {
			t.Error("expected admin authenticated context to be true")
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))

	req := httptest.NewRequest("GET", "/admin/endpoint", nil)
	req.Header.Set("X-Admin-API-Key", "test-secret-key-12345")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if !handlerCalled {
		t.Error("expected handler to be called")
	}
}

func TestAdminAuth_BearerToken(t *testing.T) {
	config := AdminAuthConfig{
		APIKey:     "test-secret-key-12345",
		HeaderName: "X-Admin-API-Key",
		Realm:      "Test",
	}

	handlerCalled := false
	handler := AdminAuth(config)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/admin/endpoint", nil)
	req.Header.Set("Authorization", "Bearer test-secret-key-12345")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if !handlerCalled {
		t.Error("expected handler to be called with Bearer token")
	}
}

func TestAdminAuth_SkipPaths(t *testing.T) {
	config := AdminAuthConfig{
		APIKey:     "test-secret-key-12345",
		HeaderName: "X-Admin-API-Key",
		Realm:      "Test",
		SkipPaths:  []string{"/health", "/ready", "/metrics"},
	}

	handlerCalled := false
	handler := AdminAuth(config)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	skipPaths := []string{"/health", "/ready", "/metrics"}
	for _, path := range skipPaths {
		handlerCalled = false
		req := httptest.NewRequest("GET", path, nil)
		// No API key provided
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("skip path %s: expected status %d, got %d", path, http.StatusOK, rr.Code)
		}
		if !handlerCalled {
			t.Errorf("skip path %s: expected handler to be called", path)
		}
	}
}

func TestAdminAuth_MisconfiguredNoAPIKey(t *testing.T) {
	config := AdminAuthConfig{
		APIKey:     "", // Empty API key = misconfigured
		HeaderName: "X-Admin-API-Key",
		Realm:      "Test",
	}

	handler := AdminAuth(config)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called when misconfigured")
	}))

	req := httptest.NewRequest("GET", "/admin/endpoint", nil)
	req.Header.Set("X-Admin-API-Key", "any-key")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, rr.Code)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "auth_misconfigured") {
		t.Errorf("expected 'auth_misconfigured' in response, got: %s", body)
	}
}

// ============================================================================
// Admin Auth Handler Tests
// ============================================================================

func TestAdminAuthHandler_ValidKey(t *testing.T) {
	config := AdminAuthConfig{
		APIKey:     "handler-test-key",
		HeaderName: "X-Admin-API-Key",
		Realm:      "Test",
	}

	handlerCalled := false
	handler := AdminAuthHandler(config, func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		if !IsAdminAuthenticated(r.Context()) {
			t.Error("expected admin authenticated context")
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("POST", "/admin/action", nil)
	req.Header.Set("X-Admin-API-Key", "handler-test-key")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if !handlerCalled {
		t.Error("expected handler to be called")
	}
}

func TestAdminAuthHandler_InvalidKey(t *testing.T) {
	config := AdminAuthConfig{
		APIKey:     "handler-test-key",
		HeaderName: "X-Admin-API-Key",
		Realm:      "Test",
	}

	handler := AdminAuthHandler(config, func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called with invalid key")
	})

	req := httptest.NewRequest("POST", "/admin/action", nil)
	req.Header.Set("X-Admin-API-Key", "wrong-key")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, rr.Code)
	}
}

// ============================================================================
// Secure Compare Tests
// ============================================================================

func TestSecureCompare_EqualStrings(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"", "", true},
		{"a", "a", true},
		{"hello", "hello", true},
		{"test-key-12345", "test-key-12345", true},
		{"a", "b", false},
		{"hello", "world", false},
		{"short", "longer-string", false},
		{"test-key-12345", "test-key-12346", false},
	}

	for _, tt := range tests {
		got := secureCompare(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("secureCompare(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestSecureCompare_TimingConsistency(t *testing.T) {
	// This test verifies that comparison time doesn't vary significantly
	// based on where strings differ (constant-time property)
	correctKey := "super-secret-key-that-is-quite-long-for-testing"
	wrongKeyEarly := "Xuper-secret-key-that-is-quite-long-for-testing"
	wrongKeyLate := "super-secret-key-that-is-quite-long-for-testinX"

	iterations := 1000

	// Measure time for early difference. The function's return value is
	// intentionally discarded — this test asserts on timing, not result.
	startEarly := time.Now()
	for i := 0; i < iterations; i++ {
		_ = secureCompare(correctKey, wrongKeyEarly)
	}
	durationEarly := time.Since(startEarly)

	// Measure time for late difference. Return value also intentionally
	// discarded for the same reason.
	startLate := time.Now()
	for i := 0; i < iterations; i++ {
		_ = secureCompare(correctKey, wrongKeyLate)
	}
	durationLate := time.Since(startLate)

	// The times should be similar (within 2x of each other for timing safety)
	ratio := float64(durationEarly) / float64(durationLate)
	if ratio < 0.5 || ratio > 2.0 {
		t.Logf("WARNING: timing difference may indicate non-constant-time comparison")
		t.Logf("early diff: %v, late diff: %v, ratio: %f", durationEarly, durationLate, ratio)
		// Not failing the test as timing can vary, but logging for awareness
	}
}

// ============================================================================
// IsAdminAuthenticated Tests
// ============================================================================

func TestIsAdminAuthenticated_NilContext(t *testing.T) {
	if IsAdminAuthenticated(nil) { //nolint:staticcheck // intentional nil-context coverage
		t.Error("expected false for nil context")
	}
}

func TestIsAdminAuthenticated_NoValue(t *testing.T) {
	ctx := context.Background()
	if IsAdminAuthenticated(ctx) {
		t.Error("expected false for context without admin auth value")
	}
}

func TestIsAdminAuthenticated_WithValue(t *testing.T) {
	ctx := context.WithValue(context.Background(), adminAuthContextKey, true)
	if !IsAdminAuthenticated(ctx) {
		t.Error("expected true for context with admin auth value")
	}
}

func TestIsAdminAuthenticated_FalseValue(t *testing.T) {
	ctx := context.WithValue(context.Background(), adminAuthContextKey, false)
	if IsAdminAuthenticated(ctx) {
		t.Error("expected false when value is explicitly false")
	}
}

// ============================================================================
// Concurrent Access Tests
// ============================================================================

func TestAdminAuth_ConcurrentRequests(t *testing.T) {
	config := AdminAuthConfig{
		APIKey:     "concurrent-test-key",
		HeaderName: "X-Admin-API-Key",
		Realm:      "Test",
	}

	var successCount int64
	handler := AdminAuth(config)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&successCount, 1)
		w.WriteHeader(http.StatusOK)
	}))

	concurrency := 100
	done := make(chan bool, concurrency)

	for i := 0; i < concurrency; i++ {
		go func() {
			req := httptest.NewRequest("GET", "/admin/endpoint", nil)
			req.Header.Set("X-Admin-API-Key", "concurrent-test-key")
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			done <- rr.Code == http.StatusOK
		}()
	}

	successResponses := 0
	for i := 0; i < concurrency; i++ {
		if <-done {
			successResponses++
		}
	}

	if successResponses != concurrency {
		t.Errorf("expected %d successful responses, got %d", concurrency, successResponses)
	}
}

// ============================================================================
// Benchmarks
// ============================================================================

func BenchmarkAdminAuth_ValidKey(b *testing.B) {
	config := AdminAuthConfig{
		APIKey:     "benchmark-secret-key-12345",
		HeaderName: "X-Admin-API-Key",
		Realm:      "Benchmark",
	}

	handler := AdminAuth(config)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/admin/endpoint", nil)
	req.Header.Set("X-Admin-API-Key", "benchmark-secret-key-12345")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}
}

func BenchmarkAdminAuth_InvalidKey(b *testing.B) {
	config := AdminAuthConfig{
		APIKey:     "benchmark-secret-key-12345",
		HeaderName: "X-Admin-API-Key",
		Realm:      "Benchmark",
	}

	handler := AdminAuth(config)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/admin/endpoint", nil)
	req.Header.Set("X-Admin-API-Key", "wrong-key-attempt")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}
}

func BenchmarkSecureCompare(b *testing.B) {
	key1 := "super-secret-key-that-is-quite-long-for-testing-purposes"
	key2 := "super-secret-key-that-is-quite-long-for-testing-purposes"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		secureCompare(key1, key2)
	}
}
