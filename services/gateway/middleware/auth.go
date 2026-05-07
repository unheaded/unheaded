// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package middleware provides HTTP middleware for the API Gateway.
package middleware

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"unheaded/pkg/logger"
	"unheaded/services/gateway/config"
)

// ContextKey is used for context values.
type ContextKey string

const (
	// UserIDKey is the context key for user ID.
	UserIDKey ContextKey = "user_id"
	// ClaimsKey is the context key for JWT claims.
	ClaimsKey ContextKey = "claims"
	// APIKeyKey is the context key for API key identifier.
	APIKeyKey ContextKey = "api_key"
)

// JWTClaims represents JWT claims.
type JWTClaims struct {
	Subject   string   `json:"sub"`
	Issuer    string   `json:"iss"`
	Audience  string   `json:"aud"`
	ExpiresAt int64    `json:"exp"`
	IssuedAt  int64    `json:"iat"`
	NotBefore int64    `json:"nbf"`
	Scopes    []string `json:"scopes,omitempty"`
}

// AuthMiddleware handles authentication.
type AuthMiddleware struct {
	cfg     *config.AuthConfig
	log     *logger.Logger
	apiKeys map[string]bool
}

// NewAuthMiddleware creates a new authentication middleware.
func NewAuthMiddleware(cfg *config.AuthConfig, log *logger.Logger) *AuthMiddleware {
	apiKeys := make(map[string]bool)
	for _, key := range cfg.APIKeys {
		apiKeys[key] = true
	}

	return &AuthMiddleware{
		cfg:     cfg,
		log:     log,
		apiKeys: apiKeys,
	}
}

// Handler returns the middleware handler.
func (m *AuthMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !m.cfg.Enabled {
			next.ServeHTTP(w, r)
			return
		}

		// Check excluded paths
		for _, path := range m.cfg.ExcludedPaths {
			if strings.HasPrefix(r.URL.Path, path) {
				next.ServeHTTP(w, r)
				return
			}
		}

		// Try JWT auth first
		if authHeader := r.Header.Get("Authorization"); authHeader != "" {
			if strings.HasPrefix(authHeader, "Bearer ") {
				token := strings.TrimPrefix(authHeader, "Bearer ")
				claims, err := m.validateJWT(token)
				if err != nil {
					m.log.Warn().
						Str("error", err.Error()).
						Str("path", r.URL.Path).
						Msg("JWT validation failed")
					writeAuthError(w, "invalid token", http.StatusUnauthorized)
					return
				}

				ctx := context.WithValue(r.Context(), ClaimsKey, claims)
				ctx = context.WithValue(ctx, UserIDKey, claims.Subject)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		// Try API key auth
		if apiKey := r.Header.Get(m.cfg.APIKeyHeader); apiKey != "" {
			if m.validateAPIKey(apiKey) {
				ctx := context.WithValue(r.Context(), APIKeyKey, hashAPIKey(apiKey))
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			m.log.Warn().
				Str("path", r.URL.Path).
				Msg("Invalid API key")
			writeAuthError(w, "invalid API key", http.StatusUnauthorized)
			return
		}

		writeAuthError(w, "authentication required", http.StatusUnauthorized)
	})
}

// validateJWT validates a JWT token and returns claims.
func (m *AuthMiddleware) validateJWT(token string) (*JWTClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, &AuthError{Message: "invalid token format"}
	}

	// Verify signature
	signingInput := parts[0] + "." + parts[1]
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, &AuthError{Message: "invalid signature encoding"}
	}

	expectedSig := m.signHS256(signingInput)
	if !hmac.Equal(signature, expectedSig) {
		return nil, &AuthError{Message: "signature verification failed"}
	}

	// Decode claims
	claimsData, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, &AuthError{Message: "invalid claims encoding"}
	}

	var claims JWTClaims
	if err := json.Unmarshal(claimsData, &claims); err != nil {
		return nil, &AuthError{Message: "invalid claims JSON"}
	}

	// Validate claims
	now := time.Now().Unix()
	if claims.ExpiresAt != 0 && claims.ExpiresAt < now {
		return nil, &AuthError{Message: "token expired"}
	}

	if claims.NotBefore != 0 && claims.NotBefore > now {
		return nil, &AuthError{Message: "token not yet valid"}
	}

	if m.cfg.JWTIssuer != "" && claims.Issuer != m.cfg.JWTIssuer {
		return nil, &AuthError{Message: "invalid issuer"}
	}

	return &claims, nil
}

// signHS256 signs data using HMAC-SHA256.
func (m *AuthMiddleware) signHS256(data string) []byte {
	h := hmac.New(sha256.New, []byte(m.cfg.JWTSecret))
	h.Write([]byte(data))
	return h.Sum(nil)
}

// validateAPIKey checks if an API key is valid.
func (m *AuthMiddleware) validateAPIKey(key string) bool {
	return m.apiKeys[key]
}

// hashAPIKey creates a hash of an API key for logging.
func hashAPIKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return base64.RawURLEncoding.EncodeToString(h[:8])
}

// writeAuthError writes an authentication error response.
func writeAuthError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("WWW-Authenticate", "Bearer")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"error":   "authentication_error",
		"message": message,
	})
}

// AuthError represents an authentication error.
type AuthError struct {
	Message string
}

func (e *AuthError) Error() string {
	return e.Message
}

// GetUserID retrieves the user ID from context.
func GetUserID(ctx context.Context) string {
	if v := ctx.Value(UserIDKey); v != nil {
		return v.(string)
	}
	return ""
}

// GetClaims retrieves JWT claims from context.
func GetClaims(ctx context.Context) *JWTClaims {
	if v := ctx.Value(ClaimsKey); v != nil {
		return v.(*JWTClaims)
	}
	return nil
}

// HasScope checks if the claims contain a specific scope.
func HasScope(ctx context.Context, scope string) bool {
	claims := GetClaims(ctx)
	if claims == nil {
		return false
	}
	for _, s := range claims.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}
