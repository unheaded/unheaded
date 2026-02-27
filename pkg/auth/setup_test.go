// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package auth

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// setup.go — LoadServiceAuthConfig
// ---------------------------------------------------------------------------

func TestLoadServiceAuthConfig_Defaults(t *testing.T) {
	// No env vars set — everything should be at default values.
	cfg := LoadServiceAuthConfig("test-svc")

	if cfg.Enabled {
		t.Error("expected Enabled=false when AUTH_ENABLED is not set")
	}
	if cfg.ServiceName != "test-svc" {
		t.Errorf("expected ServiceName=test-svc, got %s", cfg.ServiceName)
	}
	if cfg.Issuer != "https://auth.unheaded.dev" {
		t.Errorf("expected default Issuer, got %s", cfg.Issuer)
	}
	if cfg.Audience != "unheaded-api" {
		t.Errorf("expected default Audience, got %s", cfg.Audience)
	}
	if len(cfg.JWTSigningKey) != 0 {
		t.Error("expected empty JWTSigningKey when AUTH_JWT_KEY is not set")
	}
	if len(cfg.APIKeys) != 0 {
		t.Error("expected empty APIKeys when AUTH_API_KEYS is not set")
	}
	if cfg.APIKeyFile != "" {
		t.Error("expected empty APIKeyFile when AUTH_API_KEY_FILE is not set")
	}
}

func TestLoadServiceAuthConfig_WithEnvVars(t *testing.T) {
	t.Setenv("AUTH_ENABLED", "true")
	t.Setenv("AUTH_JWT_KEY", "my-secret-key")
	t.Setenv("AUTH_API_KEYS", "key1,key2, key3 ")
	t.Setenv("AUTH_JWT_ISSUER", "https://custom-issuer.dev")
	t.Setenv("AUTH_JWT_AUDIENCE", "custom-audience")
	t.Setenv("AUTH_API_KEY_FILE", "/tmp/keys.txt")

	cfg := LoadServiceAuthConfig("daemon")

	if !cfg.Enabled {
		t.Error("expected Enabled=true")
	}
	if string(cfg.JWTSigningKey) != "my-secret-key" {
		t.Errorf("expected JWTSigningKey=my-secret-key, got %s", cfg.JWTSigningKey)
	}
	if len(cfg.APIKeys) != 3 {
		t.Errorf("expected 3 API keys, got %d: %v", len(cfg.APIKeys), cfg.APIKeys)
	}
	if cfg.APIKeys[0] != "key1" || cfg.APIKeys[1] != "key2" || cfg.APIKeys[2] != "key3" {
		t.Errorf("unexpected API keys: %v", cfg.APIKeys)
	}
	if cfg.Issuer != "https://custom-issuer.dev" {
		t.Errorf("expected custom Issuer, got %s", cfg.Issuer)
	}
	if cfg.Audience != "custom-audience" {
		t.Errorf("expected custom Audience, got %s", cfg.Audience)
	}
	if cfg.APIKeyFile != "/tmp/keys.txt" {
		t.Errorf("expected APIKeyFile=/tmp/keys.txt, got %s", cfg.APIKeyFile)
	}
}

func TestLoadServiceAuthConfig_AuthEnabledFalseExplicit(t *testing.T) {
	t.Setenv("AUTH_ENABLED", "false")

	cfg := LoadServiceAuthConfig("svc")
	if cfg.Enabled {
		t.Error("expected Enabled=false when AUTH_ENABLED=false")
	}
}

// ---------------------------------------------------------------------------
// setup.go — SetupMiddleware
// ---------------------------------------------------------------------------

func TestSetupMiddleware_Disabled(t *testing.T) {
	cfg := ServiceAuthConfig{Enabled: false}
	mw := SetupMiddleware(cfg)
	if mw != nil {
		t.Error("expected nil middleware when auth is disabled")
	}
}

func TestSetupMiddleware_EnabledWithJWTKey(t *testing.T) {
	cfg := ServiceAuthConfig{
		Enabled:       true,
		JWTSigningKey: []byte("test-key-for-setup-middleware"),
		ServiceName:   "wotan",
		Issuer:        "https://auth.unheaded.dev",
		Audience:      "unheaded-api",
	}
	mw := SetupMiddleware(cfg)
	if mw == nil {
		t.Fatal("expected non-nil middleware when JWT key is set")
	}

	// The returned middleware should skip /health.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	wrapped := mw(handler)

	// /health should pass without auth.
	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for /health, got %d", rec.Code)
	}

	// An authenticated endpoint without a token should be rejected.
	req2 := httptest.NewRequest("GET", "/api/v1/data", nil)
	rec2 := httptest.NewRecorder()
	wrapped.ServeHTTP(rec2, req2)
	if rec2.Code == http.StatusOK {
		t.Error("expected non-200 for unauthenticated /api/v1/data request")
	}
}

func TestSetupMiddleware_EnabledWithAPIKeys(t *testing.T) {
	cfg := ServiceAuthConfig{
		Enabled:     true,
		APIKeys:     []string{"my-test-api-key-12345"},
		ServiceName: "timeguru",
	}
	mw := SetupMiddleware(cfg)
	if mw == nil {
		t.Fatal("expected non-nil middleware when API keys are provided")
	}
}

func TestSetupMiddleware_EnabledNoKeys(t *testing.T) {
	cfg := ServiceAuthConfig{
		Enabled:     true,
		ServiceName: "empty-svc",
		// No JWTSigningKey, no APIKeys, no APIKeyFile
	}
	mw := SetupMiddleware(cfg)
	if mw != nil {
		t.Error("expected nil middleware when enabled but no authenticators available")
	}
}

func TestSetupMiddleware_EnabledWithAPIKeyFile(t *testing.T) {
	// Write a temporary key file.
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "keys.txt")
	if err := os.WriteFile(keyFile, []byte("file-api-key-abc\nfile-api-key-def\n"), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := ServiceAuthConfig{
		Enabled:    true,
		APIKeyFile: keyFile,
	}
	mw := SetupMiddleware(cfg)
	if mw == nil {
		t.Fatal("expected non-nil middleware when API key file is provided")
	}
}

// ---------------------------------------------------------------------------
// setup.go — WrapHandler
// ---------------------------------------------------------------------------

func TestWrapHandler_NilMiddleware(t *testing.T) {
	original := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	result := WrapHandler(original, nil)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	result.ServeHTTP(rec, req)

	if rec.Code != http.StatusTeapot {
		t.Errorf("expected %d, got %d", http.StatusTeapot, rec.Code)
	}
}

func TestWrapHandler_WithMiddleware(t *testing.T) {
	original := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// A middleware that adds a header.
	mw := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Wrapped", "true")
			next.ServeHTTP(w, r)
		})
	}

	result := WrapHandler(original, mw)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	result.ServeHTTP(rec, req)

	if rec.Header().Get("X-Wrapped") != "true" {
		t.Error("expected X-Wrapped header from middleware")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// setup.go — helper functions
// ---------------------------------------------------------------------------

func TestGetEnvDefault_Set(t *testing.T) {
	t.Setenv("TEST_SETUP_ENV_KEY", "custom-value")

	got := getEnvDefault("TEST_SETUP_ENV_KEY", "fallback")
	if got != "custom-value" {
		t.Errorf("expected custom-value, got %s", got)
	}
}

func TestGetEnvDefault_NotSet(t *testing.T) {
	got := getEnvDefault("TEST_NONEXISTENT_KEY_XYZ_ABC", "fallback")
	if got != "fallback" {
		t.Errorf("expected fallback, got %s", got)
	}
}

func TestSplitNonEmpty(t *testing.T) {
	tests := []struct {
		input    string
		sep      string
		expected []string
	}{
		{"a,b,c", ",", []string{"a", "b", "c"}},
		{" a , b , c ", ",", []string{"a", "b", "c"}},
		{",,a,,b,,", ",", []string{"a", "b"}},
		{"single", ",", []string{"single"}},
		{"", ",", nil},
		{"  ,  ,  ", ",", nil},
	}

	for _, tc := range tests {
		result := splitNonEmpty(tc.input, tc.sep)
		if len(result) != len(tc.expected) {
			t.Errorf("splitNonEmpty(%q, %q): got %v, want %v", tc.input, tc.sep, result, tc.expected)
			continue
		}
		for i := range result {
			if result[i] != tc.expected[i] {
				t.Errorf("splitNonEmpty(%q, %q)[%d]: got %q, want %q", tc.input, tc.sep, i, result[i], tc.expected[i])
			}
		}
	}
}

func TestSplitString(t *testing.T) {
	tests := []struct {
		input    string
		sep      string
		expected []string
	}{
		{"a,b,c", ",", []string{"a", "b", "c"}},
		{"hello", ",", []string{"hello"}},
		{"a::b::c", "::", []string{"a", "b", "c"}},
		{"", ",", []string{""}},
		{",", ",", []string{"", ""}},
	}

	for _, tc := range tests {
		result := splitString(tc.input, tc.sep)
		if len(result) != len(tc.expected) {
			t.Errorf("splitString(%q, %q): got %v, want %v", tc.input, tc.sep, result, tc.expected)
			continue
		}
		for i := range result {
			if result[i] != tc.expected[i] {
				t.Errorf("splitString(%q, %q)[%d]: got %q, want %q", tc.input, tc.sep, i, result[i], tc.expected[i])
			}
		}
	}
}

func TestIndexOf(t *testing.T) {
	tests := []struct {
		s, sub string
		want   int
	}{
		{"hello world", "world", 6},
		{"hello world", "hello", 0},
		{"hello world", "xyz", -1},
		{"", "a", -1},
		{"abc", "", 0},
		{"aaa", "aa", 0},
	}

	for _, tc := range tests {
		got := indexOf(tc.s, tc.sub)
		if got != tc.want {
			t.Errorf("indexOf(%q, %q) = %d, want %d", tc.s, tc.sub, got, tc.want)
		}
	}
}

func TestTrimSpace(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"  hello  ", "hello"},
		{"\t\nhello\r\n", "hello"},
		{"hello", "hello"},
		{"", ""},
		{"   ", ""},
		{"\t\n\r", ""},
		{" a b c ", "a b c"},
	}

	for _, tc := range tests {
		got := trimSpace(tc.input)
		if got != tc.expected {
			t.Errorf("trimSpace(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

// ---------------------------------------------------------------------------
// testutil.go — token generation functions
// ---------------------------------------------------------------------------

func TestAdminToken(t *testing.T) {
	token := AdminToken()
	if token == "" {
		t.Fatal("AdminToken() returned empty string")
	}
	if !strings.HasPrefix(token, "eyJ") {
		t.Error("AdminToken() should return a JWT starting with eyJ")
	}
}

func TestObserverToken(t *testing.T) {
	token := ObserverToken()
	if token == "" {
		t.Fatal("ObserverToken() returned empty string")
	}
	if !strings.HasPrefix(token, "eyJ") {
		t.Error("ObserverToken() should return a JWT starting with eyJ")
	}
}

func TestOperatorToken(t *testing.T) {
	token := OperatorToken()
	if token == "" {
		t.Fatal("OperatorToken() returned empty string")
	}
	if !strings.HasPrefix(token, "eyJ") {
		t.Error("OperatorToken() should return a JWT starting with eyJ")
	}
}

func TestServiceToken(t *testing.T) {
	token := ServiceToken("wotan")
	if token == "" {
		t.Fatal("ServiceToken(wotan) returned empty string")
	}
	if !strings.HasPrefix(token, "eyJ") {
		t.Error("ServiceToken() should return a JWT starting with eyJ")
	}
}

func TestInvalidToken(t *testing.T) {
	token := InvalidToken()
	if token == "" {
		t.Fatal("InvalidToken() returned empty string")
	}
	// The token is well-formed JWT, just expired.
	if !strings.HasPrefix(token, "eyJ") {
		t.Error("InvalidToken() should return a JWT starting with eyJ")
	}
}

func TestMalformedToken(t *testing.T) {
	token := MalformedToken()
	if token == "" {
		t.Fatal("MalformedToken() returned empty string")
	}
	if !strings.HasPrefix(token, "eyJ") {
		t.Error("MalformedToken() should start with eyJ")
	}
}

func TestWrongSignatureToken(t *testing.T) {
	token := WrongSignatureToken()
	if token == "" {
		t.Fatal("WrongSignatureToken() returned empty string")
	}
	if !strings.HasPrefix(token, "eyJ") {
		t.Error("WrongSignatureToken() should return a JWT starting with eyJ")
	}
}

// ---------------------------------------------------------------------------
// audit.go — NewFileAuditLogger and Close
// ---------------------------------------------------------------------------

func TestNewFileAuditLogger(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.log")

	logger, err := NewFileAuditLogger(logPath, "test-svc")
	if err != nil {
		t.Fatalf("NewFileAuditLogger failed: %v", err)
	}

	// Log an event.
	logger.Log(AuditEvent{
		EventType: AuditAPIKeySuccess,
		Subject:   "api-user",
		Result:    "success",
	})

	// Close the logger.
	if err := logger.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Verify the file has content.
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty audit log file")
	}
	if !strings.Contains(string(data), "apikey_validation_success") {
		t.Errorf("expected event_type in file content: %s", data)
	}
}

func TestNewFileAuditLogger_InvalidPath(t *testing.T) {
	_, err := NewFileAuditLogger("/nonexistent/dir/audit.log", "svc")
	if err == nil {
		t.Error("expected error for invalid path")
	}
}

func TestAuditLogger_Close_NonCloser(t *testing.T) {
	// bytes.Buffer does not implement io.Closer.
	var buf bytes.Buffer
	logger := NewAuditLogger(&buf, "test")

	err := logger.Close()
	if err != nil {
		t.Errorf("Close on non-Closer writer should return nil, got: %v", err)
	}
}

func TestAuditLogger_LogWithTimestamp(t *testing.T) {
	var buf bytes.Buffer
	logger := NewAuditLogger(&buf, "svc")

	// When Timestamp is pre-set, it should be preserved.
	logger.Log(AuditEvent{
		Timestamp: "2026-01-01T00:00:00Z",
		EventType: AuditRateLimited,
		Result:    "limited",
	})

	output := buf.String()
	if !strings.Contains(output, "2026-01-01T00:00:00Z") {
		t.Errorf("expected pre-set timestamp to be preserved: %s", output)
	}
}

func TestAuditLogger_LogWithService(t *testing.T) {
	var buf bytes.Buffer
	logger := NewAuditLogger(&buf, "default-svc")

	// When Service is pre-set, it should be preserved.
	logger.Log(AuditEvent{
		EventType: AuditUnauthorized,
		Service:   "override-svc",
		Result:    "denied",
	})

	output := buf.String()
	if !strings.Contains(output, "override-svc") {
		t.Errorf("expected pre-set service to be preserved: %s", output)
	}
	if strings.Contains(output, "default-svc") {
		t.Errorf("default service should NOT appear when Service is pre-set: %s", output)
	}
}
