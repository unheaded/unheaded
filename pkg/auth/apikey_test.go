// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestAPIKeyAuthenticator_ValidKey(t *testing.T) {
	auth, err := NewAPIKeyAuthenticator(APIKeyConfig{
		Keys:         []string{"test-api-key-12345"},
		DefaultRoles: []string{"admin"},
	})
	if err != nil {
		t.Fatalf("NewAPIKeyAuthenticator: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline", nil)
	req.Header.Set("X-API-Key", "test-api-key-12345")

	identity, err := auth.Authenticate(context.Background(), req)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if identity.Subject != "apikey" {
		t.Errorf("Subject = %q, want %q", identity.Subject, "apikey")
	}
	if !identity.HasRole("admin") {
		t.Error("expected admin role")
	}
}

func TestAPIKeyAuthenticator_InvalidKey(t *testing.T) {
	auth, err := NewAPIKeyAuthenticator(APIKeyConfig{
		Keys: []string{"valid-key"},
	})
	if err != nil {
		t.Fatalf("NewAPIKeyAuthenticator: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline", nil)
	req.Header.Set("X-API-Key", "wrong-key")

	_, err = auth.Authenticate(context.Background(), req)
	if !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("expected ErrUnauthenticated, got: %v", err)
	}
}

func TestAPIKeyAuthenticator_MissingKey(t *testing.T) {
	auth, err := NewAPIKeyAuthenticator(APIKeyConfig{
		Keys: []string{"valid-key"},
	})
	if err != nil {
		t.Fatalf("NewAPIKeyAuthenticator: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline", nil)
	// No X-API-Key header

	_, err = auth.Authenticate(context.Background(), req)
	if !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("expected ErrUnauthenticated, got: %v", err)
	}
}

func TestAPIKeyAuthenticator_RotatedKey(t *testing.T) {
	// Both current and previous key should be accepted
	auth, err := NewAPIKeyAuthenticator(APIKeyConfig{
		Keys: []string{"new-key-2026", "old-key-2025"},
	})
	if err != nil {
		t.Fatalf("NewAPIKeyAuthenticator: %v", err)
	}

	// New key works
	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/timeline", nil)
	req1.Header.Set("X-API-Key", "new-key-2026")

	identity, err := auth.Authenticate(context.Background(), req1)
	if err != nil {
		t.Fatalf("new key failed: %v", err)
	}
	if identity == nil {
		t.Fatal("expected identity for new key")
	}

	// Old (rotated) key still works
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/timeline", nil)
	req2.Header.Set("X-API-Key", "old-key-2025")

	identity, err = auth.Authenticate(context.Background(), req2)
	if err != nil {
		t.Fatalf("rotated key failed: %v", err)
	}
	if identity == nil {
		t.Fatal("expected identity for rotated key")
	}
}

func TestAPIKeyAuthenticator_RateLimited(t *testing.T) {
	auth, err := NewAPIKeyAuthenticator(APIKeyConfig{
		Keys:               []string{"test-key"},
		RateLimitPerMinute: 3,
	})
	if err != nil {
		t.Fatalf("NewAPIKeyAuthenticator: %v", err)
	}

	// Requests 1-3 should succeed
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline", nil)
		req.Header.Set("X-API-Key", "test-key")

		_, err := auth.Authenticate(context.Background(), req)
		if err != nil {
			t.Fatalf("request %d failed: %v", i+1, err)
		}
	}

	// Request 4 should be rate limited
	req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline", nil)
	req.Header.Set("X-API-Key", "test-key")

	_, err = auth.Authenticate(context.Background(), req)
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("expected ErrRateLimited, got: %v", err)
	}
}

func TestAPIKeyAuthenticator_CustomHeader(t *testing.T) {
	auth, err := NewAPIKeyAuthenticator(APIKeyConfig{
		Keys:       []string{"my-key"},
		HeaderName: "X-Custom-Key",
	})
	if err != nil {
		t.Fatalf("NewAPIKeyAuthenticator: %v", err)
	}

	// Using default header name should fail
	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/timeline", nil)
	req1.Header.Set("X-API-Key", "my-key")

	_, err = auth.Authenticate(context.Background(), req1)
	if !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("expected ErrUnauthenticated for wrong header, got: %v", err)
	}

	// Using custom header should work
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/timeline", nil)
	req2.Header.Set("X-Custom-Key", "my-key")

	identity, err := auth.Authenticate(context.Background(), req2)
	if err != nil {
		t.Fatalf("custom header failed: %v", err)
	}
	if identity == nil {
		t.Fatal("expected identity")
	}
}

func TestAPIKeyAuthenticator_KeyFromFile(t *testing.T) {
	// Write a temp key file
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "api-keys")
	content := "# API keys for Unheaded\n\ncurrent-key-2026\nprevious-key-2025\n"
	if err := os.WriteFile(keyFile, []byte(content), 0600); err != nil {
		t.Fatalf("write key file: %v", err)
	}

	auth, err := NewAPIKeyAuthenticator(APIKeyConfig{
		KeyFile: keyFile,
	})
	if err != nil {
		t.Fatalf("NewAPIKeyAuthenticator: %v", err)
	}

	// Current key works
	req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline", nil)
	req.Header.Set("X-API-Key", "current-key-2026")

	identity, err := auth.Authenticate(context.Background(), req)
	if err != nil {
		t.Fatalf("file key failed: %v", err)
	}
	if identity == nil {
		t.Fatal("expected identity from file key")
	}

	// Previous key also works
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/timeline", nil)
	req2.Header.Set("X-API-Key", "previous-key-2025")

	identity, err = auth.Authenticate(context.Background(), req2)
	if err != nil {
		t.Fatalf("rotated file key failed: %v", err)
	}
	if identity == nil {
		t.Fatal("expected identity from rotated file key")
	}
}

func TestAPIKeyAuthenticator_NoKeys(t *testing.T) {
	_, err := NewAPIKeyAuthenticator(APIKeyConfig{})
	if err == nil {
		t.Fatal("expected error for empty config")
	}
}

func TestAPIKeyAuthenticator_Middleware_RateLimited429(t *testing.T) {
	auth, err := NewAPIKeyAuthenticator(APIKeyConfig{
		Keys:               []string{"test-key"},
		RateLimitPerMinute: 1,
	})
	if err != nil {
		t.Fatalf("NewAPIKeyAuthenticator: %v", err)
	}

	handler := Middleware(auth)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request should pass
	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req1.Header.Set("X-API-Key", "test-key")
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Errorf("first request: status = %d, want 200", rec1.Code)
	}

	// Second request should be rate limited
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req2.Header.Set("X-API-Key", "test-key")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusTooManyRequests {
		t.Errorf("second request: status = %d, want 429", rec2.Code)
	}
}
