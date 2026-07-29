// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package auth

import (
	"net/http"
	"os"
)

// ServiceAuthConfig is the configuration for a service's auth setup.
// Read from environment variables to maintain the "configuration from env" pattern.
type ServiceAuthConfig struct {
	// Enabled controls whether auth middleware is active.
	// Read from AUTH_ENABLED env var (default: false for backward compatibility).
	Enabled bool

	// JWTSigningKey is the HMAC-SHA256 key for JWT validation.
	// Read from AUTH_JWT_KEY env var.
	JWTSigningKey []byte

	// APIKeys is the list of valid API keys.
	// Read from AUTH_API_KEYS env var (comma-separated) or AUTH_API_KEY_FILE.
	APIKeys []string // #nosec G117 -- field intentionally named APIKeys; populated at runtime, not committed.

	// APIKeyFile is the path to a file containing API keys.
	APIKeyFile string

	// ServiceName is the name of this service.
	ServiceName string

	// Issuer is the expected JWT issuer.
	Issuer string

	// Audience is the expected JWT audience.
	Audience string
}

// LoadServiceAuthConfig loads auth configuration from environment variables.
func LoadServiceAuthConfig(serviceName string) ServiceAuthConfig {
	cfg := ServiceAuthConfig{
		Enabled:     os.Getenv("AUTH_ENABLED") == "true",
		ServiceName: serviceName,
		Issuer:      getEnvDefault("AUTH_JWT_ISSUER", "https://auth.unheaded.dev"),
		Audience:    getEnvDefault("AUTH_JWT_AUDIENCE", "unheaded-api"),
		APIKeyFile:  os.Getenv("AUTH_API_KEY_FILE"),
	}

	if key := os.Getenv("AUTH_JWT_KEY"); key != "" {
		cfg.JWTSigningKey = []byte(key)
	}

	if keys := os.Getenv("AUTH_API_KEYS"); keys != "" {
		cfg.APIKeys = append(cfg.APIKeys, splitNonEmpty(keys, ",")...)
	}

	return cfg
}

// SetupMiddleware creates an auth middleware from the config, or returns nil
// if auth is disabled. Nil means "no auth middleware needed."
//
// When enabled, it creates a MultiAuthenticator that accepts:
//   - JWT tokens (Authorization: Bearer <token>)
//   - API keys (X-API-Key header)
//   - Service tokens (Authorization: Bearer <service-token>)
//
// Internal endpoints (/health, /ready, /metrics) are always skipped.
func SetupMiddleware(cfg ServiceAuthConfig) func(http.Handler) http.Handler {
	if !cfg.Enabled {
		return nil
	}

	var authenticators []Authenticator

	// JWT authenticator
	if len(cfg.JWTSigningKey) > 0 {
		authenticators = append(authenticators, NewJWTAuthenticator(JWTConfig{
			SigningKey: cfg.JWTSigningKey,
			Issuer:     cfg.Issuer,
			Audience:   cfg.Audience,
		}))
	}

	// Service token authenticator (uses same signing key, different issuer)
	if len(cfg.JWTSigningKey) > 0 && cfg.ServiceName != "" {
		authenticators = append(authenticators, NewJWTAuthenticator(JWTConfig{
			SigningKey:   cfg.JWTSigningKey,
			Issuer:       ServiceTokenIssuer,
			Audience:     cfg.ServiceName,
			AllowedRoles: []string{RoleService},
		}))
	}

	// API key authenticator
	if len(cfg.APIKeys) > 0 || cfg.APIKeyFile != "" {
		apikeyCfg := APIKeyConfig{
			Keys:    cfg.APIKeys,
			KeyFile: cfg.APIKeyFile,
		}
		if apiKeyAuth, err := NewAPIKeyAuthenticator(apikeyCfg); err == nil {
			authenticators = append(authenticators, apiKeyAuth)
		}
	}

	if len(authenticators) == 0 {
		return nil
	}

	multi := &MultiAuthenticator{Authenticators: authenticators}
	authMiddleware := Middleware(multi)

	// Skip auth for health/ready/metrics
	return SkipAuthPaths(authMiddleware, "/health", "/ready", "/metrics")
}

// WrapHandler applies the auth middleware to a handler if the middleware is non-nil.
// This is the simplest integration point for services.
func WrapHandler(handler http.Handler, authMiddleware func(http.Handler) http.Handler) http.Handler {
	if authMiddleware == nil {
		return handler
	}
	return authMiddleware(handler)
}

func getEnvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func splitNonEmpty(s, sep string) []string {
	var result []string
	for _, part := range splitString(s, sep) {
		trimmed := trimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// splitString splits s by sep without importing strings in auth.go.
func splitString(s, sep string) []string {
	var parts []string
	for {
		i := indexOf(s, sep)
		if i < 0 {
			parts = append(parts, s)
			break
		}
		parts = append(parts, s[:i])
		s = s[i+len(sep):]
	}
	return parts
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\n' || s[0] == '\r') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
