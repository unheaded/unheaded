// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var testServiceKey = []byte("service-mesh-shared-secret-for-unheaded-tests!")

func TestServiceTokenManager_GenerateAndValidate(t *testing.T) {
	mgr, err := NewServiceTokenManager(ServiceTokenConfig{
		SigningKey:   testServiceKey,
		ServiceName: "timeguru",
		TTL:         5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewServiceTokenManager: %v", err)
	}

	// Generate token for calling captain
	token, err := mgr.GenerateToken("captain")
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	// Validate from captain's side
	captainMgr, err := NewServiceTokenManager(ServiceTokenConfig{
		SigningKey:   testServiceKey,
		ServiceName: "captain",
	})
	if err != nil {
		t.Fatalf("NewServiceTokenManager (captain): %v", err)
	}

	validator := captainMgr.NewValidator()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/strategy", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	identity, err := validator.Authenticate(context.Background(), req)
	if err != nil {
		t.Fatalf("expected valid service token, got: %v", err)
	}
	if identity.Subject != "timeguru" {
		t.Errorf("Subject = %q, want %q", identity.Subject, "timeguru")
	}
	if !identity.HasRole(RoleService) {
		t.Error("expected service role")
	}
	if identity.Extra["source"] != "timeguru" {
		t.Errorf("Extra[source] = %q, want %q", identity.Extra["source"], "timeguru")
	}
	if identity.Extra["target"] != "captain" {
		t.Errorf("Extra[target] = %q, want %q", identity.Extra["target"], "captain")
	}
}

func TestServiceTokenManager_ExpiredRejected(t *testing.T) {
	// Create a token that is already expired by manually signing claims in the past
	claims := &JWTClaims{
		Sub:   "timeguru",
		Iss:   ServiceTokenIssuer,
		Aud:   "captain",
		Iat:   time.Now().Add(-10 * time.Minute).Unix(),
		Exp:   time.Now().Add(-5 * time.Minute).Unix(), // expired 5 minutes ago
		Roles: []string{RoleService},
		Extra: map[string]string{"auth_method": "service_token"},
	}
	token, err := SignToken(testServiceKey, claims)
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}

	captainMgr, err := NewServiceTokenManager(ServiceTokenConfig{
		SigningKey:   testServiceKey,
		ServiceName: "captain",
	})
	if err != nil {
		t.Fatalf("NewServiceTokenManager (captain): %v", err)
	}

	validator := captainMgr.NewValidator()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/strategy", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	_, err = validator.Authenticate(context.Background(), req)
	if !errors.Is(err, ErrTokenExpired) {
		t.Errorf("expected ErrTokenExpired, got: %v", err)
	}
}

func TestServiceTokenManager_WrongAudienceRejected(t *testing.T) {
	mgr, err := NewServiceTokenManager(ServiceTokenConfig{
		SigningKey:   testServiceKey,
		ServiceName: "timeguru",
		TTL:         5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewServiceTokenManager: %v", err)
	}

	// Generate token for captain
	token, err := mgr.GenerateToken("captain")
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	// Validate from architect (wrong audience)
	architectMgr, err := NewServiceTokenManager(ServiceTokenConfig{
		SigningKey:   testServiceKey,
		ServiceName: "architect",
	})
	if err != nil {
		t.Fatalf("NewServiceTokenManager (architect): %v", err)
	}

	validator := architectMgr.NewValidator()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/design", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	_, err = validator.Authenticate(context.Background(), req)
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("expected ErrForbidden for wrong audience, got: %v", err)
	}
}

func TestServiceTokenManager_WrongSigningKeyRejected(t *testing.T) {
	mgr, err := NewServiceTokenManager(ServiceTokenConfig{
		SigningKey:   []byte("key-from-service-A"),
		ServiceName: "timeguru",
		TTL:         5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewServiceTokenManager: %v", err)
	}

	token, err := mgr.GenerateToken("captain")
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	// Captain uses a different key
	captainMgr, err := NewServiceTokenManager(ServiceTokenConfig{
		SigningKey:   []byte("different-key-for-service-B"),
		ServiceName: "captain",
	})
	if err != nil {
		t.Fatalf("NewServiceTokenManager (captain): %v", err)
	}

	validator := captainMgr.NewValidator()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/strategy", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	_, err = validator.Authenticate(context.Background(), req)
	if !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("expected ErrUnauthenticated for wrong key, got: %v", err)
	}
}

func TestServiceTokenManager_KeyFromFile(t *testing.T) {
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "service-signing-key")
	if err := os.WriteFile(keyFile, []byte("file-based-signing-key-256\n"), 0600); err != nil {
		t.Fatalf("write key file: %v", err)
	}

	mgr, err := NewServiceTokenManager(ServiceTokenConfig{
		SigningKeyFile: keyFile,
		ServiceName:   "monad",
	})
	if err != nil {
		t.Fatalf("NewServiceTokenManager: %v", err)
	}

	token, err := mgr.GenerateToken("sophia")
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	// Validate
	sophiaMgr, err := NewServiceTokenManager(ServiceTokenConfig{
		SigningKeyFile: keyFile,
		ServiceName:   "sophia",
	})
	if err != nil {
		t.Fatalf("NewServiceTokenManager (sophia): %v", err)
	}

	validator := sophiaMgr.NewValidator()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/knowledge", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	identity, err := validator.Authenticate(context.Background(), req)
	if err != nil {
		t.Fatalf("expected valid token, got: %v", err)
	}
	if identity.Subject != "monad" {
		t.Errorf("Subject = %q, want %q", identity.Subject, "monad")
	}
}

func TestServiceTokenManager_FromEnvVar(t *testing.T) {
	// Test that UNHEADED_SERVICE_NAME env is used as fallback
	t.Setenv("UNHEADED_SERVICE_NAME", "dashboard-backend")

	mgr, err := NewServiceTokenManager(ServiceTokenConfig{
		SigningKey: testServiceKey,
		// ServiceName deliberately empty — should use env
	})
	if err != nil {
		t.Fatalf("NewServiceTokenManager: %v", err)
	}

	token, err := mgr.GenerateToken("timeguru")
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	// Validate
	timeGuruMgr, err := NewServiceTokenManager(ServiceTokenConfig{
		SigningKey:   testServiceKey,
		ServiceName: "timeguru",
	})
	if err != nil {
		t.Fatalf("NewServiceTokenManager (timeguru): %v", err)
	}

	validator := timeGuruMgr.NewValidator()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	identity, err := validator.Authenticate(context.Background(), req)
	if err != nil {
		t.Fatalf("expected valid token, got: %v", err)
	}
	if identity.Subject != "dashboard-backend" {
		t.Errorf("Subject = %q, want %q", identity.Subject, "dashboard-backend")
	}
}

func TestServiceTokenManager_NoKeyError(t *testing.T) {
	_, err := NewServiceTokenManager(ServiceTokenConfig{
		ServiceName: "test",
	})
	if err == nil {
		t.Fatal("expected error for empty key")
	}
}

func TestServiceTokenManager_NoServiceNameError(t *testing.T) {
	// Ensure env is clear
	t.Setenv("UNHEADED_SERVICE_NAME", "")

	_, err := NewServiceTokenManager(ServiceTokenConfig{
		SigningKey: testServiceKey,
	})
	if err == nil {
		t.Fatal("expected error for empty service name")
	}
}

func TestServiceTokenManager_DefaultTTL(t *testing.T) {
	mgr, err := NewServiceTokenManager(ServiceTokenConfig{
		SigningKey:   testServiceKey,
		ServiceName: "test",
		// TTL deliberately 0 — should use default
	})
	if err != nil {
		t.Fatalf("NewServiceTokenManager: %v", err)
	}

	if mgr.ttl != DefaultServiceTokenTTL {
		t.Errorf("TTL = %v, want %v", mgr.ttl, DefaultServiceTokenTTL)
	}
}
