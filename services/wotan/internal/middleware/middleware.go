// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"unheaded/services/wotan/internal/logger"
	"unheaded/services/wotan/internal/metrics"
)

// responseWriter wraps http.ResponseWriter to capture status code and size
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	size, err := rw.ResponseWriter.Write(b)
	rw.size += size
	return size, err
}

// Logging middleware logs HTTP requests with structured logging
func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Generate request ID
		requestID := uuid.New().String()
		ctx := r.Context()
		ctx, reqLogger := logger.WithRequestID(ctx, requestID)
		r = r.WithContext(ctx)

		// Add request ID to response headers
		w.Header().Set("X-Request-ID", requestID)

		// Wrap response writer
		wrapped := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
			size:           0,
		}

		// Log request start
		reqLogger.Info().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Str("remote_addr", r.RemoteAddr).
			Str("user_agent", r.UserAgent()).
			Msg("http_request_start")

		// Call next handler
		next.ServeHTTP(wrapped, r)

		// Log request completion
		duration := time.Since(start)
		reqLogger.Info().
			Int("status_code", wrapped.statusCode).
			Int("response_size", wrapped.size).
			Dur("duration_ms", duration).
			Msg("http_request_complete")
	})
}

// Metrics middleware records Prometheus metrics
func Metrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer
		wrapped := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		// Call next handler
		next.ServeHTTP(wrapped, r)

		// Record metrics
		duration := time.Since(start)
		m := metrics.Get()
		if m != nil {
			m.RecordHTTPRequest(r.Method, r.URL.Path, wrapped.statusCode, duration)
			m.HTTPRequestSize.WithLabelValues(r.Method, r.URL.Path).Observe(float64(r.ContentLength))
			m.HTTPResponseSize.WithLabelValues(r.Method, r.URL.Path).Observe(float64(wrapped.size))
		}
	})
}

// SecurityHeaders adds security headers to responses
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Security headers
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		w.Header().Set("Content-Security-Policy", "default-src 'self'")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

		next.ServeHTTP(w, r)
	})
}

// CORS middleware adds CORS headers
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Check if origin is allowed
			allowed := false
			for _, allowedOrigin := range allowedOrigins {
				if allowedOrigin == "*" || allowedOrigin == origin {
					allowed = true
					break
				}
			}

			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")
				w.Header().Set("Access-Control-Max-Age", "86400")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}

			// Handle preflight
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// Timeout middleware adds request timeout
func Timeout(duration time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), duration)
			defer cancel()

			r = r.WithContext(ctx)

			done := make(chan struct{})
			go func() {
				next.ServeHTTP(w, r)
				close(done)
			}()

			select {
			case <-done:
				return
			case <-ctx.Done():
				log.Warn().
					Str("path", r.URL.Path).
					Msg("request_timeout")
				http.Error(w, `{"error":"request_timeout","message":"Request exceeded timeout"}`, http.StatusGatewayTimeout)
			}
		})
	}
}

// Recovery middleware recovers from panics
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Error().
					Interface("panic", err).
					Str("path", r.URL.Path).
					Msg("panic_recovered")

				http.Error(w, `{"error":"internal_error","message":"Internal server error"}`, http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// Chain applies multiple middleware functions
func Chain(h http.Handler, middleware ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middleware) - 1; i >= 0; i-- {
		h = middleware[i](h)
	}
	return h
}

// ============================================================================
// Admin Authentication Middleware
// ============================================================================

// AdminAuthConfig holds configuration for admin authentication
type AdminAuthConfig struct {
	// APIKey is the expected admin API key (should be loaded from env/secrets)
	APIKey string
	// HeaderName is the header to check for the API key
	HeaderName string
	// Realm is the authentication realm for WWW-Authenticate header
	Realm string
	// SkipPaths are paths that don't require authentication
	SkipPaths []string
}

// DefaultAdminAuthConfig returns a default admin auth configuration
func DefaultAdminAuthConfig() AdminAuthConfig {
	return AdminAuthConfig{
		APIKey:     "", // Must be set by caller
		HeaderName: "X-Admin-API-Key",
		Realm:      "Wotan Admin",
		SkipPaths:  []string{"/health", "/ready", "/metrics"},
	}
}

// AdminAuth middleware provides API key authentication for admin endpoints
// This implements constant-time comparison to prevent timing attacks
func AdminAuth(config AdminAuthConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Validate config
			if config.APIKey == "" {
				log.Error().Msg("admin_auth_misconfigured: API key not set")
				writeAuthError(w, http.StatusInternalServerError, "auth_misconfigured", "Admin authentication not configured")
				return
			}

			// Check if path should skip authentication
			for _, path := range config.SkipPaths {
				if r.URL.Path == path {
					next.ServeHTTP(w, r)
					return
				}
			}

			// Get API key from header
			headerName := config.HeaderName
			if headerName == "" {
				headerName = "X-Admin-API-Key"
			}
			providedKey := r.Header.Get(headerName)

			// Also check Authorization header with Bearer token
			if providedKey == "" {
				authHeader := r.Header.Get("Authorization")
				if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
					providedKey = authHeader[7:]
				}
			}

			// Validate API key
			if providedKey == "" {
				log.Warn().
					Str("path", r.URL.Path).
					Str("remote_addr", r.RemoteAddr).
					Msg("admin_auth_missing_key")
				writeAuthError(w, http.StatusUnauthorized, "missing_api_key", "Admin API key required")
				w.Header().Set("WWW-Authenticate", `Bearer realm="`+config.Realm+`"`)
				return
			}

			// Constant-time comparison to prevent timing attacks
			if !secureCompare(providedKey, config.APIKey) {
				log.Warn().
					Str("path", r.URL.Path).
					Str("remote_addr", r.RemoteAddr).
					Msg("admin_auth_invalid_key")
				writeAuthError(w, http.StatusForbidden, "invalid_api_key", "Invalid admin API key")
				return
			}

			// Log successful authentication
			log.Info().
				Str("path", r.URL.Path).
				Str("remote_addr", r.RemoteAddr).
				Msg("admin_auth_success")

			// Add admin context
			ctx := context.WithValue(r.Context(), adminAuthContextKey, true)
			r = r.WithContext(ctx)

			next.ServeHTTP(w, r)
		})
	}
}

// AdminAuthHandler wraps a single handler function with admin authentication
func AdminAuthHandler(config AdminAuthConfig, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Validate config
		if config.APIKey == "" {
			log.Error().Msg("admin_auth_misconfigured: API key not set")
			writeAuthError(w, http.StatusInternalServerError, "auth_misconfigured", "Admin authentication not configured")
			return
		}

		// Get API key from header
		headerName := config.HeaderName
		if headerName == "" {
			headerName = "X-Admin-API-Key"
		}
		providedKey := r.Header.Get(headerName)

		// Also check Authorization header with Bearer token
		if providedKey == "" {
			authHeader := r.Header.Get("Authorization")
			if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
				providedKey = authHeader[7:]
			}
		}

		// Validate API key
		if providedKey == "" {
			log.Warn().
				Str("path", r.URL.Path).
				Str("remote_addr", r.RemoteAddr).
				Msg("admin_auth_missing_key")
			w.Header().Set("WWW-Authenticate", `Bearer realm="`+config.Realm+`"`)
			writeAuthError(w, http.StatusUnauthorized, "missing_api_key", "Admin API key required")
			return
		}

		// Constant-time comparison to prevent timing attacks
		if !secureCompare(providedKey, config.APIKey) {
			log.Warn().
				Str("path", r.URL.Path).
				Str("remote_addr", r.RemoteAddr).
				Msg("admin_auth_invalid_key")
			writeAuthError(w, http.StatusForbidden, "invalid_api_key", "Invalid admin API key")
			return
		}

		// Log successful authentication
		log.Info().
			Str("path", r.URL.Path).
			Str("remote_addr", r.RemoteAddr).
			Msg("admin_auth_success")

		// Add admin context
		ctx := context.WithValue(r.Context(), adminAuthContextKey, true)
		r = r.WithContext(ctx)

		handler(w, r)
	}
}

// secureCompare performs constant-time string comparison to prevent timing attacks
func secureCompare(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var result byte
	for i := 0; i < len(a); i++ {
		result |= a[i] ^ b[i]
	}
	return result == 0
}

// writeAuthError writes a JSON error response for authentication failures
func writeAuthError(w http.ResponseWriter, status int, errCode, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// Manual JSON to avoid import cycle
	_, _ = w.Write([]byte(`{"error":"` + errCode + `","code":` + statusCodeString(status) + `,"message":"` + message + `"}`))
}

func statusCodeString(code int) string {
	switch code {
	case 401:
		return "401"
	case 403:
		return "403"
	case 500:
		return "500"
	default:
		return "500"
	}
}

// Context key for admin authentication
type contextKey string

const adminAuthContextKey contextKey = "adminAuthenticated"

// IsAdminAuthenticated checks if the request has been authenticated as admin
func IsAdminAuthenticated(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	authenticated, ok := ctx.Value(adminAuthContextKey).(bool)
	return ok && authenticated
}
