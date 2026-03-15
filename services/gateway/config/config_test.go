// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg == nil {
		t.Fatal("DefaultConfig returned nil")
	}

	if cfg.Server.HTTPPort != 21000 {
		t.Errorf("expected HTTP port 21000, got %d", cfg.Server.HTTPPort)
	}
	if cfg.Server.HTTP3Port != 21443 {
		t.Errorf("expected HTTP3 port 21443, got %d", cfg.Server.HTTP3Port)
	}
	if cfg.Server.ReadTimeout != 30*time.Second {
		t.Errorf("expected read timeout 30s, got %v", cfg.Server.ReadTimeout)
	}
	if !cfg.RateLimit.Enabled {
		t.Error("expected rate limit enabled by default")
	}
	if cfg.RateLimit.RequestsPerSec != 100 {
		t.Errorf("expected 100 req/s, got %f", cfg.RateLimit.RequestsPerSec)
	}
	if !cfg.CORS.Enabled {
		t.Error("expected CORS enabled by default")
	}
	if !cfg.Circuit.Enabled {
		t.Error("expected circuit breaker enabled by default")
	}
	if cfg.Circuit.FailureThreshold != 5 {
		t.Errorf("expected failure threshold 5, got %d", cfg.Circuit.FailureThreshold)
	}
	if !cfg.Metrics.Enabled {
		t.Error("expected metrics enabled by default")
	}
}

func TestDefaultConfig_AlphaServices(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.AlphaServices.TimeGuruHost != "timeguru:19000" {
		t.Errorf("expected timeguru:19000, got %q", cfg.AlphaServices.TimeGuruHost)
	}
	if cfg.AlphaServices.CaptainHost != "captain:19002" {
		t.Errorf("expected captain:19002, got %q", cfg.AlphaServices.CaptainHost)
	}
	if cfg.AlphaServices.ArchitectHost != "architect:19001" {
		t.Errorf("expected architect:19001, got %q", cfg.AlphaServices.ArchitectHost)
	}
	if cfg.AlphaServices.MicromanagerHost != "micromanager:19003" {
		t.Errorf("expected micromanager:19003, got %q", cfg.AlphaServices.MicromanagerHost)
	}
	if cfg.AlphaServices.MonadHost != "monad:19004" {
		t.Errorf("expected monad:19004, got %q", cfg.AlphaServices.MonadHost)
	}
	if cfg.AlphaServices.SophiaHost != "sophia:19005" {
		t.Errorf("expected sophia:19005, got %q", cfg.AlphaServices.SophiaHost)
	}
}

func TestValidate_ValidConfig(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("DefaultConfig should validate: %v", err)
	}
}

func TestValidate_InvalidPort(t *testing.T) {
	tests := []struct {
		name  string
		port  int
		valid bool
	}{
		{"valid_8080", 8080, true},
		{"valid_zero", 0, true},
		{"valid_65535", 65535, true},
		{"negative", -1, false},
		{"too_high", 65536, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Server.HTTPPort = tt.port
			err := cfg.Validate()
			if tt.valid && err != nil {
				t.Errorf("expected valid for port %d, got error: %v", tt.port, err)
			}
			if !tt.valid && err == nil {
				t.Errorf("expected error for port %d", tt.port)
			}
		})
	}
}

func TestValidate_AuthEnabled_NoSecret(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = ""
	cfg.Auth.APIKeys = nil

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error when auth enabled without JWT secret or API keys")
	}

	// Check it's a ConfigError
	var cfgErr *ConfigError
	if _, ok := err.(*ConfigError); !ok {
		t.Errorf("expected *ConfigError, got %T: %v", err, cfgErr)
	}
}

func TestValidate_AuthEnabled_WithAPIKeys(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = ""
	cfg.Auth.APIKeys = []string{"key-123"}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("auth with API keys should be valid: %v", err)
	}
}

func TestValidate_AuthEnabled_WithJWTSecret(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = "mysecret"

	if err := cfg.Validate(); err != nil {
		t.Fatalf("auth with JWT secret should be valid: %v", err)
	}
}

func TestConfigError_Error(t *testing.T) {
	err := &ConfigError{
		Field:   "server.http_port",
		Message: "invalid port number",
	}

	expected := "config error: server.http_port: invalid port number"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gateway.json")

	cfg := DefaultConfig()
	cfg.Server.HTTPPort = 9090
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = "test-secret"

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	loaded, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}

	if loaded.Server.HTTPPort != 9090 {
		t.Errorf("expected port 9090, got %d", loaded.Server.HTTPPort)
	}
	if loaded.Auth.JWTSecret != "test-secret" {
		t.Errorf("expected JWT secret, got %q", loaded.Auth.JWTSecret)
	}
}

func TestLoadFromFile_NonexistentFile(t *testing.T) {
	_, err := LoadFromFile("/nonexistent/config.json")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLoadFromFile_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	os.WriteFile(path, []byte("{invalid"), 0644)

	_, err := LoadFromFile(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("GATEWAY_HTTP_PORT", "9999")
	t.Setenv("GATEWAY_JWT_SECRET", "env-secret")
	t.Setenv("WOTAN_URL", "http://wotan:8081")
	t.Setenv("GATEWAY_TLS_CERT", "/certs/cert.pem")
	t.Setenv("GATEWAY_TLS_KEY", "/certs/key.pem")

	cfg := LoadFromEnv()

	if cfg.Server.HTTPPort != 9999 {
		t.Errorf("expected port 9999 from env, got %d", cfg.Server.HTTPPort)
	}
	if cfg.Auth.JWTSecret != "env-secret" {
		t.Errorf("expected JWT secret from env, got %q", cfg.Auth.JWTSecret)
	}
	if !cfg.Auth.Enabled {
		t.Error("expected auth enabled when JWT secret set via env")
	}
	if cfg.Wotan.URL != "http://wotan:8081" {
		t.Errorf("expected wotan URL from env, got %q", cfg.Wotan.URL)
	}
	if cfg.Server.TLSCertFile != "/certs/cert.pem" {
		t.Errorf("expected TLS cert from env, got %q", cfg.Server.TLSCertFile)
	}
	if cfg.Server.TLSKeyFile != "/certs/key.pem" {
		t.Errorf("expected TLS key from env, got %q", cfg.Server.TLSKeyFile)
	}
}

func TestLoadFromEnv_CORSConfig(t *testing.T) {
	t.Setenv("GATEWAY_CORS_ORIGINS", "https://example.com, https://api.example.com")

	cfg := LoadFromEnv()
	if len(cfg.CORS.AllowedOrigins) != 2 {
		t.Fatalf("expected 2 CORS origins, got %d", len(cfg.CORS.AllowedOrigins))
	}
	if cfg.CORS.AllowedOrigins[0] != "https://example.com" {
		t.Errorf("expected first origin 'https://example.com', got %q", cfg.CORS.AllowedOrigins[0])
	}
	if cfg.CORS.AllowedOrigins[1] != "https://api.example.com" {
		t.Errorf("expected second origin 'https://api.example.com', got %q", cfg.CORS.AllowedOrigins[1])
	}
}

func TestLoadFromEnv_CORSDisabled(t *testing.T) {
	t.Setenv("GATEWAY_CORS_ENABLED", "false")

	cfg := LoadFromEnv()
	if cfg.CORS.Enabled {
		t.Error("expected CORS disabled")
	}
}

func TestLoadFromEnv_AlphaServices(t *testing.T) {
	t.Setenv("TIMEGURU_HOST", "timeguru.local:8000")
	t.Setenv("CAPTAIN_HOST", "captain.local:8001")
	t.Setenv("ARCHITECT_HOST", "architect.local:8002")
	t.Setenv("MICROMANAGER_HOST", "micromanager.local:8003")
	t.Setenv("MONAD_HOST", "monad.local:8004")
	t.Setenv("SOPHIA_HOST", "sophia.local:8005")

	cfg := LoadFromEnv()

	if cfg.AlphaServices.TimeGuruHost != "timeguru.local:8000" {
		t.Errorf("expected timeguru host from env, got %q", cfg.AlphaServices.TimeGuruHost)
	}
	if cfg.AlphaServices.CaptainHost != "captain.local:8001" {
		t.Errorf("expected captain host from env, got %q", cfg.AlphaServices.CaptainHost)
	}
	if cfg.AlphaServices.MonadHost != "monad.local:8004" {
		t.Errorf("expected monad host from env, got %q", cfg.AlphaServices.MonadHost)
	}
	if cfg.AlphaServices.SophiaHost != "sophia.local:8005" {
		t.Errorf("expected sophia host from env, got %q", cfg.AlphaServices.SophiaHost)
	}
}

func TestSplitAndTrim(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		delim    string
		expected []string
	}{
		{"simple", "a,b,c", ",", []string{"a", "b", "c"}},
		{"with_spaces", " a , b , c ", ",", []string{"a", "b", "c"}},
		{"empty_parts", "a,,b", ",", []string{"a", "b"}},
		{"single", "only", ",", []string{"only"}},
		{"empty", "", ",", nil},
		{"all_spaces", " , , ", ",", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitAndTrim(tt.input, tt.delim)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d results, got %d: %v", len(tt.expected), len(result), result)
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("index %d: expected %q, got %q", i, tt.expected[i], result[i])
				}
			}
		})
	}
}
