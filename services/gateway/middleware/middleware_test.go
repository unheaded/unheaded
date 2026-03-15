// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package middleware

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/logger"
	"unheaded/services/gateway/config"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func testLogger() *logger.Logger {
	return logger.New(io.Discard)
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
}

// buildJWT creates a minimal HS256 JWT for testing.
func buildJWT(secret string, claims JWTClaims) string {
	headerJSON := `{"alg":"HS256","typ":"JWT"}`
	headerEnc := base64.RawURLEncoding.EncodeToString([]byte(headerJSON))

	claimsJSON, _ := json.Marshal(claims)
	claimsEnc := base64.RawURLEncoding.EncodeToString(claimsJSON)

	signingInput := headerEnc + "." + claimsEnc
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingInput))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return signingInput + "." + sig
}

// ---------------------------------------------------------------------------
// CORS middleware
// ---------------------------------------------------------------------------

func TestCORS_Disabled(t *testing.T) {
	cfg := &config.CORSConfig{Enabled: false}
	m := NewCORSMiddleware(cfg)
	handler := m.Handler(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "http://evil.com")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if rr.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("CORS header should not be set when disabled")
	}
}

func TestCORS_NoOriginHeader(t *testing.T) {
	cfg := &config.CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{"*"},
	}
	m := NewCORSMiddleware(cfg)
	handler := m.Handler(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if rr.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("no CORS header expected when Origin is absent")
	}
}

func TestCORS_AllowAllOrigins(t *testing.T) {
	cfg := &config.CORSConfig{
		Enabled:          true,
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST"},
		AllowedHeaders:   []string{"Content-Type"},
		AllowCredentials: false,
	}
	m := NewCORSMiddleware(cfg)
	handler := m.Handler(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "http://any.example.com")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("expected *, got %q", got)
	}
}

func TestCORS_AllowAllWithCredentials(t *testing.T) {
	cfg := &config.CORSConfig{
		Enabled:          true,
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET"},
		AllowCredentials: true,
	}
	m := NewCORSMiddleware(cfg)
	handler := m.Handler(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "http://foo.com")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "http://foo.com" {
		t.Fatalf("expected echoed origin, got %q", got)
	}
	if rr.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatal("expected credentials header")
	}
}

func TestCORS_SpecificOriginAllowed(t *testing.T) {
	cfg := &config.CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{"http://good.com"},
		AllowedMethods: []string{"GET"},
	}
	m := NewCORSMiddleware(cfg)
	handler := m.Handler(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "http://good.com")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "http://good.com" {
		t.Fatalf("expected http://good.com, got %q", got)
	}
}

func TestCORS_OriginNotAllowed(t *testing.T) {
	cfg := &config.CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{"http://good.com"},
		AllowedMethods: []string{"GET"},
	}
	m := NewCORSMiddleware(cfg)
	handler := m.Handler(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "http://bad.com")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("should not set CORS headers for disallowed origin")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("request should still be served, got %d", rr.Code)
	}
}

func TestCORS_ExposedHeaders(t *testing.T) {
	cfg := &config.CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{"*"},
		ExposedHeaders: []string{"X-Custom", "X-Other"},
	}
	m := NewCORSMiddleware(cfg)
	handler := m.Handler(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "http://example.com")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	exposed := rr.Header().Get("Access-Control-Expose-Headers")
	if !strings.Contains(exposed, "X-Custom") || !strings.Contains(exposed, "X-Other") {
		t.Fatalf("expected exposed headers, got %q", exposed)
	}
}

func TestCORS_PreflightSuccess(t *testing.T) {
	cfg := &config.CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "DELETE"},
		AllowedHeaders: []string{"Authorization", "X-Custom"},
		MaxAge:         3600,
	}
	m := NewCORSMiddleware(cfg)
	handler := m.Handler(okHandler())

	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "http://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Authorization, X-Custom")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rr.Code)
	}
	if rr.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Fatal("expected Allow-Methods header")
	}
	if rr.Header().Get("Access-Control-Allow-Headers") == "" {
		t.Fatal("expected Allow-Headers header")
	}
	if rr.Header().Get("Access-Control-Max-Age") != "3600" {
		t.Fatalf("expected max-age 3600, got %q", rr.Header().Get("Access-Control-Max-Age"))
	}
}

func TestCORS_PreflightNoRequestMethod(t *testing.T) {
	cfg := &config.CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET"},
	}
	m := NewCORSMiddleware(cfg)
	handler := m.Handler(okHandler())

	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "http://example.com")
	// No Access-Control-Request-Method
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestCORS_PreflightMethodNotAllowed(t *testing.T) {
	cfg := &config.CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET"},
	}
	m := NewCORSMiddleware(cfg)
	handler := m.Handler(okHandler())

	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "http://example.com")
	req.Header.Set("Access-Control-Request-Method", "DELETE")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}

func TestCORS_PreflightHeaderNotAllowed(t *testing.T) {
	cfg := &config.CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET"},
		AllowedHeaders: []string{},
	}
	m := NewCORSMiddleware(cfg)
	handler := m.Handler(okHandler())

	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "http://example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Headers", "X-Not-Allowed")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestCORS_SimpleHeadersAlwaysAllowed(t *testing.T) {
	cfg := &config.CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET"},
		AllowedHeaders: []string{}, // no extra headers
	}
	m := NewCORSMiddleware(cfg)
	handler := m.Handler(okHandler())

	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "http://example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Headers", "Accept, Content-Type")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("simple headers should be allowed, got %d", rr.Code)
	}
}

func TestIsMethodAllowed_CaseInsensitive(t *testing.T) {
	cfg := &config.CORSConfig{
		AllowedMethods: []string{"GET", "post"},
	}
	m := NewCORSMiddleware(cfg)

	if !m.isMethodAllowed("get") {
		t.Fatal("expected GET to match (case insensitive)")
	}
	if !m.isMethodAllowed("POST") {
		t.Fatal("expected POST to match (case insensitive)")
	}
	if m.isMethodAllowed("DELETE") {
		t.Fatal("DELETE should not be allowed")
	}
}

// ---------------------------------------------------------------------------
// Auth middleware
// ---------------------------------------------------------------------------

func TestAuth_Disabled(t *testing.T) {
	cfg := &config.AuthConfig{Enabled: false}
	m := NewAuthMiddleware(cfg, testLogger())
	handler := m.Handler(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/secret", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 when auth disabled, got %d", rr.Code)
	}
}

func TestAuth_ExcludedPath(t *testing.T) {
	cfg := &config.AuthConfig{
		Enabled:       true,
		JWTSecret:     "secret",
		ExcludedPaths: []string{"/health", "/public/"},
	}
	m := NewAuthMiddleware(cfg, testLogger())
	handler := m.Handler(okHandler())

	for _, path := range []string{"/health", "/public/anything"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("path %q should be excluded, got %d", path, rr.Code)
		}
	}
}

func TestAuth_NoCredentials(t *testing.T) {
	cfg := &config.AuthConfig{
		Enabled:      true,
		JWTSecret:    "secret",
		APIKeyHeader: "X-API-Key",
	}
	m := NewAuthMiddleware(cfg, testLogger())
	handler := m.Handler(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/secret", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
	if rr.Header().Get("WWW-Authenticate") != "Bearer" {
		t.Fatal("expected WWW-Authenticate header")
	}
}

func TestAuth_ValidJWT(t *testing.T) {
	secret := "test-secret"
	cfg := &config.AuthConfig{
		Enabled:      true,
		JWTSecret:    secret,
		APIKeyHeader: "X-API-Key",
	}
	m := NewAuthMiddleware(cfg, testLogger())

	var capturedUserID string
	var capturedClaims *JWTClaims
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUserID = GetUserID(r.Context())
		capturedClaims = GetClaims(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	handler := m.Handler(inner)

	claims := JWTClaims{
		Subject:   "user-42",
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
		IssuedAt:  time.Now().Unix(),
		Scopes:    []string{"read", "write"},
	}
	token := buildJWT(secret, claims)

	req := httptest.NewRequest(http.MethodGet, "/secret", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if capturedUserID != "user-42" {
		t.Fatalf("expected user-42, got %q", capturedUserID)
	}
	if capturedClaims == nil || len(capturedClaims.Scopes) != 2 {
		t.Fatal("expected claims with 2 scopes")
	}
}

func TestAuth_ExpiredJWT(t *testing.T) {
	secret := "test-secret"
	cfg := &config.AuthConfig{
		Enabled:      true,
		JWTSecret:    secret,
		APIKeyHeader: "X-API-Key",
	}
	m := NewAuthMiddleware(cfg, testLogger())
	handler := m.Handler(okHandler())

	claims := JWTClaims{
		Subject:   "user-42",
		ExpiresAt: time.Now().Add(-time.Hour).Unix(),
	}
	token := buildJWT(secret, claims)

	req := httptest.NewRequest(http.MethodGet, "/secret", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for expired token, got %d", rr.Code)
	}
}

func TestAuth_InvalidJWTSignature(t *testing.T) {
	cfg := &config.AuthConfig{
		Enabled:      true,
		JWTSecret:    "real-secret",
		APIKeyHeader: "X-API-Key",
	}
	m := NewAuthMiddleware(cfg, testLogger())
	handler := m.Handler(okHandler())

	claims := JWTClaims{
		Subject:   "user-42",
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}
	// Sign with a different secret
	token := buildJWT("wrong-secret", claims)

	req := httptest.NewRequest(http.MethodGet, "/secret", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for bad signature, got %d", rr.Code)
	}
}

func TestAuth_MalformedJWT(t *testing.T) {
	cfg := &config.AuthConfig{
		Enabled:      true,
		JWTSecret:    "secret",
		APIKeyHeader: "X-API-Key",
	}
	m := NewAuthMiddleware(cfg, testLogger())
	handler := m.Handler(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/secret", nil)
	req.Header.Set("Authorization", "Bearer not.a.valid.token")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestAuth_JWTIssuerValidation(t *testing.T) {
	secret := "test-secret"
	cfg := &config.AuthConfig{
		Enabled:      true,
		JWTSecret:    secret,
		JWTIssuer:    "trusted-issuer",
		APIKeyHeader: "X-API-Key",
	}
	m := NewAuthMiddleware(cfg, testLogger())
	handler := m.Handler(okHandler())

	claims := JWTClaims{
		Subject:   "user-42",
		Issuer:    "untrusted-issuer",
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}
	token := buildJWT(secret, claims)

	req := httptest.NewRequest(http.MethodGet, "/secret", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong issuer, got %d", rr.Code)
	}
}

func TestAuth_NotBeforeValidation(t *testing.T) {
	secret := "test-secret"
	cfg := &config.AuthConfig{
		Enabled:      true,
		JWTSecret:    secret,
		APIKeyHeader: "X-API-Key",
	}
	m := NewAuthMiddleware(cfg, testLogger())
	handler := m.Handler(okHandler())

	claims := JWTClaims{
		Subject:   "user-42",
		NotBefore: time.Now().Add(time.Hour).Unix(),
		ExpiresAt: time.Now().Add(2 * time.Hour).Unix(),
	}
	token := buildJWT(secret, claims)

	req := httptest.NewRequest(http.MethodGet, "/secret", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for not-yet-valid token, got %d", rr.Code)
	}
}

func TestAuth_ValidAPIKey(t *testing.T) {
	cfg := &config.AuthConfig{
		Enabled:      true,
		JWTSecret:    "secret",
		APIKeyHeader: "X-API-Key",
		APIKeys:      []string{"key-abc-123"},
	}
	m := NewAuthMiddleware(cfg, testLogger())

	var capturedAPIKey string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v := r.Context().Value(APIKeyKey); v != nil {
			capturedAPIKey = v.(string)
		}
		w.WriteHeader(http.StatusOK)
	})
	handler := m.Handler(inner)

	req := httptest.NewRequest(http.MethodGet, "/secret", nil)
	req.Header.Set("X-API-Key", "key-abc-123")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if capturedAPIKey == "" {
		t.Fatal("expected API key context value to be set")
	}
}

func TestAuth_InvalidAPIKey(t *testing.T) {
	cfg := &config.AuthConfig{
		Enabled:      true,
		JWTSecret:    "secret",
		APIKeyHeader: "X-API-Key",
		APIKeys:      []string{"key-abc-123"},
	}
	m := NewAuthMiddleware(cfg, testLogger())
	handler := m.Handler(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/secret", nil)
	req.Header.Set("X-API-Key", "wrong-key")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// Context helper functions
// ---------------------------------------------------------------------------

func TestGetUserID_Empty(t *testing.T) {
	ctx := context.Background()
	if got := GetUserID(ctx); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestGetClaims_Nil(t *testing.T) {
	ctx := context.Background()
	if got := GetClaims(ctx); got != nil {
		t.Fatal("expected nil")
	}
}

func TestHasScope(t *testing.T) {
	claims := &JWTClaims{Scopes: []string{"read", "admin"}}
	ctx := context.WithValue(context.Background(), ClaimsKey, claims)

	if !HasScope(ctx, "read") {
		t.Fatal("expected read scope")
	}
	if !HasScope(ctx, "admin") {
		t.Fatal("expected admin scope")
	}
	if HasScope(ctx, "write") {
		t.Fatal("write scope should not exist")
	}
}

func TestHasScope_NoClaims(t *testing.T) {
	if HasScope(context.Background(), "read") {
		t.Fatal("expected false with no claims")
	}
}

func TestAuthError(t *testing.T) {
	err := &AuthError{Message: "bad token"}
	if err.Error() != "bad token" {
		t.Fatalf("expected 'bad token', got %q", err.Error())
	}
}

func TestHashAPIKey(t *testing.T) {
	h1 := hashAPIKey("key1")
	h2 := hashAPIKey("key2")
	if h1 == h2 {
		t.Fatal("different keys should produce different hashes")
	}
	h1a := hashAPIKey("key1")
	if h1 != h1a {
		t.Fatal("same key should produce same hash")
	}
}

// ---------------------------------------------------------------------------
// Tracing middleware
// ---------------------------------------------------------------------------

func TestTracing_GeneratesIDs(t *testing.T) {
	m := NewTracingMiddleware("test-svc")

	var traceID, spanID string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID = GetTraceID(r.Context())
		spanID = GetSpanID(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	handler := m.Handler(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if traceID == "" {
		t.Fatal("expected auto-generated trace ID")
	}
	if spanID == "" {
		t.Fatal("expected auto-generated span ID")
	}
	if rr.Header().Get(TraceIDHeader) != traceID {
		t.Fatal("response should contain trace ID header")
	}
	if rr.Header().Get(SpanIDHeader) != spanID {
		t.Fatal("response should contain span ID header")
	}
}

func TestTracing_PropagatesExistingTraceID(t *testing.T) {
	m := NewTracingMiddleware("test-svc")

	var capturedTraceID string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedTraceID = GetTraceID(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	handler := m.Handler(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(TraceIDHeader, "existing-trace-123")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if capturedTraceID != "existing-trace-123" {
		t.Fatalf("expected propagated trace ID, got %q", capturedTraceID)
	}
}

func TestTracing_UsesRequestIDFallback(t *testing.T) {
	m := NewTracingMiddleware("test-svc")

	var capturedTraceID string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedTraceID = GetTraceID(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	handler := m.Handler(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(RequestIDHeader, "req-456")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if capturedTraceID != "req-456" {
		t.Fatalf("expected Request-ID fallback, got %q", capturedTraceID)
	}
}

func TestTracing_ParentSpanIDSet(t *testing.T) {
	m := NewTracingMiddleware("test-svc")
	handler := m.Handler(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(SpanIDHeader, "parent-span-789")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Header().Get(ParentSpanIDHeader) != "parent-span-789" {
		t.Fatal("expected parent span ID in response")
	}
}

func TestTracing_NoParentSpan(t *testing.T) {
	m := NewTracingMiddleware("test-svc")
	handler := m.Handler(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Header().Get(ParentSpanIDHeader) != "" {
		t.Fatal("no parent span header expected when none set")
	}
}

func TestGetRequestStart(t *testing.T) {
	m := NewTracingMiddleware("test-svc")

	var start time.Time
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start = GetRequestStart(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	handler := m.Handler(inner)

	before := time.Now()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	after := time.Now()

	if start.Before(before) || start.After(after) {
		t.Fatalf("request start %v should be between %v and %v", start, before, after)
	}
}

func TestGetRequestStart_Empty(t *testing.T) {
	start := GetRequestStart(context.Background())
	if !start.IsZero() {
		t.Fatal("expected zero time")
	}
}

func TestGetTraceID_Empty(t *testing.T) {
	if got := GetTraceID(context.Background()); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestGetSpanID_Empty(t *testing.T) {
	if got := GetSpanID(context.Background()); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestInjectHeaders(t *testing.T) {
	ctx := context.WithValue(context.Background(), TraceIDKey, "trace-abc")
	ctx = context.WithValue(ctx, SpanIDKey, "span-def")

	header := http.Header{}
	InjectHeaders(ctx, header)

	if header.Get(TraceIDHeader) != "trace-abc" {
		t.Fatal("expected trace ID in injected headers")
	}
	if header.Get(ParentSpanIDHeader) != "span-def" {
		t.Fatal("expected parent span ID in injected headers")
	}
	if header.Get(SpanIDHeader) == "" {
		t.Fatal("expected new span ID in injected headers")
	}
}

func TestSpanBuilder(t *testing.T) {
	sb := NewSpan("my-service", "handle-request").
		WithTraceID("trace-1").
		WithParentSpanID("parent-1").
		WithTag("key", "value")

	time.Sleep(time.Millisecond)
	span := sb.End("ok")

	if span.ServiceName != "my-service" {
		t.Fatalf("expected my-service, got %q", span.ServiceName)
	}
	if span.OperationName != "handle-request" {
		t.Fatalf("expected handle-request, got %q", span.OperationName)
	}
	if span.TraceID != "trace-1" {
		t.Fatalf("expected trace-1, got %q", span.TraceID)
	}
	if span.ParentSpanID != "parent-1" {
		t.Fatalf("expected parent-1, got %q", span.ParentSpanID)
	}
	if span.Tags["key"] != "value" {
		t.Fatal("expected tag key=value")
	}
	if span.Status != "ok" {
		t.Fatalf("expected status ok, got %q", span.Status)
	}
	if span.Duration <= 0 {
		t.Fatal("expected positive duration")
	}
	if span.SpanID == "" {
		t.Fatal("expected auto-generated span ID")
	}
}

// ---------------------------------------------------------------------------
// Logging helpers
// ---------------------------------------------------------------------------

func TestResponseWriter_DefaultStatus(t *testing.T) {
	rw := newResponseWriter(httptest.NewRecorder())
	if rw.status != http.StatusOK {
		t.Fatalf("expected default 200, got %d", rw.status)
	}
}

func TestResponseWriter_WriteHeader(t *testing.T) {
	rw := newResponseWriter(httptest.NewRecorder())
	rw.WriteHeader(http.StatusNotFound)
	if rw.status != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rw.status)
	}
}

func TestResponseWriter_WriteHeaderOnlyOnce(t *testing.T) {
	rw := newResponseWriter(httptest.NewRecorder())
	rw.WriteHeader(http.StatusNotFound)
	rw.WriteHeader(http.StatusOK) // second call ignored
	if rw.status != http.StatusNotFound {
		t.Fatalf("expected first status 404, got %d", rw.status)
	}
}

func TestResponseWriter_WriteImplicitHeader(t *testing.T) {
	rw := newResponseWriter(httptest.NewRecorder())
	rw.Write([]byte("hello"))
	if !rw.wroteHeader {
		t.Fatal("Write should set wroteHeader")
	}
	if rw.size != 5 {
		t.Fatalf("expected size 5, got %d", rw.size)
	}
}

func TestResponseWriter_Flush(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := newResponseWriter(rec)
	// Should not panic
	rw.Flush()
}

func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name       string
		xff        string
		xri        string
		remoteAddr string
		want       string
	}{
		{"X-Forwarded-For single", "1.2.3.4", "", "5.6.7.8:1234", "1.2.3.4"},
		{"X-Forwarded-For multiple", "1.2.3.4, 5.6.7.8", "", "9.0.0.1:1234", "1.2.3.4"},
		{"X-Real-IP", "", "10.0.0.1", "5.6.7.8:1234", "10.0.0.1"},
		{"RemoteAddr with port", "", "", "192.168.1.1:4321", "192.168.1.1"},
		{"RemoteAddr no port", "", "", "192.168.1.1", "192.168.1.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.xff != "" {
				req.Header.Set("X-Forwarded-For", tt.xff)
			}
			if tt.xri != "" {
				req.Header.Set("X-Real-IP", tt.xri)
			}
			req.RemoteAddr = tt.remoteAddr
			got := getClientIP(req)
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/users/123", "/users/:id"},
		{"/users/42/posts/7", "/users/:id/posts/:id"},
		{"/health", "/health"},
		{"/api/v1/items", "/api/v1/items"},
		{"/", "/"},
		{"", "/"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizePath(tt.input)
			if got != tt.want {
				t.Fatalf("normalizePath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStatusCodeClass(t *testing.T) {
	tests := []struct {
		code int
		want string
	}{
		{200, "2xx"},
		{201, "2xx"},
		{301, "3xx"},
		{404, "4xx"},
		{500, "5xx"},
		{100, "unknown"},
	}
	for _, tt := range tests {
		got := statusCodeClass(tt.code)
		if got != tt.want {
			t.Fatalf("statusCodeClass(%d) = %q, want %q", tt.code, got, tt.want)
		}
	}
}

func TestLoggingMiddleware_Handler(t *testing.T) {
	log := testLogger()
	lm := NewLoggingMiddleware(log, nil)
	handler := lm.Handler(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/test?q=1", nil)
	req.RemoteAddr = "127.0.0.1:9999"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// Rate limiter
// ---------------------------------------------------------------------------

func TestTokenBucket_Allow(t *testing.T) {
	tb := NewTokenBucket(10, 5) // 10/s, burst 5
	for i := 0; i < 5; i++ {
		if !tb.Allow() {
			t.Fatalf("expected allow on iteration %d", i)
		}
	}
	// Bucket should be empty now
	if tb.Allow() {
		t.Fatal("expected deny when bucket is empty")
	}
}

func TestTokenBucket_Refill(t *testing.T) {
	tb := NewTokenBucket(1000, 1) // high rate so refill is fast
	tb.Allow()                    // drain the single token
	time.Sleep(5 * time.Millisecond)
	if !tb.Allow() {
		t.Fatal("expected token to refill")
	}
}

func TestTokenBucket_Tokens(t *testing.T) {
	tb := NewTokenBucket(10, 5)
	initial := tb.Tokens()
	if initial != 5 {
		t.Fatalf("expected 5 tokens, got %f", initial)
	}
	tb.Allow()
	after := tb.Tokens()
	if after >= initial {
		t.Fatal("expected fewer tokens after Allow()")
	}
}

func TestRateLimiter_Disabled(t *testing.T) {
	cfg := &config.RateLimitConfig{
		Enabled:       false,
		CleanupPeriod: time.Hour,
	}
	rl := NewRateLimiter(cfg, testLogger(), nil)
	defer rl.Stop()

	handler := rl.Handler(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "1.2.3.4:5678"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 when disabled, got %d", rr.Code)
	}
}

func TestRateLimiter_AllowsWithinLimit(t *testing.T) {
	cfg := &config.RateLimitConfig{
		Enabled:        true,
		RequestsPerSec: 100,
		BurstSize:      5,
		CleanupPeriod:  time.Hour,
	}
	rl := NewRateLimiter(cfg, testLogger(), nil)
	defer rl.Stop()

	handler := rl.Handler(okHandler())

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "10.0.0.1:1234"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i, rr.Code)
		}
		if rr.Header().Get("X-RateLimit-Limit") == "" {
			t.Fatal("expected rate limit headers")
		}
	}
}

func TestRateLimiter_DeniesOverLimit(t *testing.T) {
	cfg := &config.RateLimitConfig{
		Enabled:        true,
		RequestsPerSec: 1,
		BurstSize:      2,
		CleanupPeriod:  time.Hour,
	}
	rl := NewRateLimiter(cfg, testLogger(), nil)
	defer rl.Stop()

	handler := rl.Handler(okHandler())

	// Exhaust burst
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "10.0.0.1:1234"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}

	// This should be rate limited
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rr.Code)
	}
	if rr.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header")
	}
}

func TestRateLimiter_PerClientBuckets(t *testing.T) {
	cfg := &config.RateLimitConfig{
		Enabled:        true,
		RequestsPerSec: 1,
		BurstSize:      1,
		CleanupPeriod:  time.Hour,
	}
	rl := NewRateLimiter(cfg, testLogger(), nil)
	defer rl.Stop()

	handler := rl.Handler(okHandler())

	// First client uses its token
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	req1.RemoteAddr = "10.0.0.1:1234"
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("client 1 first request should succeed, got %d", rr1.Code)
	}

	// Second client should have its own bucket
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "10.0.0.2:1234"
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("client 2 should have own bucket, got %d", rr2.Code)
	}
}

func TestRateLimiter_XForwardedFor(t *testing.T) {
	cfg := &config.RateLimitConfig{
		Enabled:        true,
		RequestsPerSec: 1,
		BurstSize:      1,
		CleanupPeriod:  time.Hour,
	}
	rl := NewRateLimiter(cfg, testLogger(), nil)
	defer rl.Stop()

	handler := rl.Handler(okHandler())

	// Use X-Forwarded-For so same RemoteAddr but different real client
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "proxy:8080"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestRouteRateLimiter(t *testing.T) {
	rrl := NewRouteRateLimiter()
	defer rrl.Stop()

	cfg := &config.RateLimitConfig{
		Enabled:        true,
		RequestsPerSec: 100,
		BurstSize:      10,
		CleanupPeriod:  time.Hour,
	}
	rrl.AddRoute("/api/v1", cfg, testLogger(), nil)

	if limiter := rrl.GetLimiter("/api/v1/users"); limiter == nil {
		t.Fatal("expected limiter for /api/v1/users")
	}
	if limiter := rrl.GetLimiter("/other"); limiter != nil {
		t.Fatal("expected no limiter for /other")
	}
}

func TestFormatFloat(t *testing.T) {
	tests := []struct {
		input float64
		want  string
	}{
		{100.0, "100"},
		{1.5, "1.5"},
		{0, "0"},
	}
	for _, tt := range tests {
		got := formatFloat(tt.input)
		if got != tt.want {
			t.Fatalf("formatFloat(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
