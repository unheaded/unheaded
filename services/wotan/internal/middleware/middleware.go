// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package middleware

import (
	"context"
	"net/http"
	"sync"
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

// failureLogEvery is how often a run of failures with the same status is
// logged: the first one at once, then one line per interval carrying how
// many were not logged in between.
const failureLogEvery = 10 * time.Second

// failureLog rate-limits failure lines per status code (a bounded key: the
// path is client input and would not be).
type failureLog struct {
	mu   sync.Mutex
	last map[int]time.Time
	held map[int]int
}

var failures = &failureLog{last: map[int]time.Time{}, held: map[int]int{}}

// allow reports whether a failure with this status is logged now, and how
// many with it were held back since the last line.
func (f *failureLog) allow(status int, now time.Time) (bool, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if t, ok := f.last[status]; ok && now.Sub(t) < failureLogEvery {
		f.held[status]++
		return false, 0
	}
	n := f.held[status]
	f.last[status], f.held[status] = now, 0
	return true, n
}

// Logging middleware logs HTTP requests with structured logging.
//
// Successful requests are logged at debug. Wotan serves hundreds a second
// (topic polls, publishes, scrapes): two info lines each filled the 3 x 10
// MB container log in about four minutes, so when Wotan was OOM-killed
// (2026-10-18) everything before was gone, and a client sending ~150 bad
// requests a second hid among them. Failures (4xx warn, 5xx error) are
// logged with method and path, at most once per status every
// failureLogEvery, with the count held back in between.
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
		reqLogger.Debug().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Str("remote_addr", r.RemoteAddr).
			Str("user_agent", r.UserAgent()).
			Msg("http_request_start")

		// Call next handler
		next.ServeHTTP(wrapped, r)

		// Log request completion
		duration := time.Since(start)
		status := wrapped.statusCode
		if status < 400 {
			reqLogger.Debug().
				Int("status_code", status).
				Int("response_size", wrapped.size).
				Dur("duration_ms", duration).
				Msg("http_request_complete")
			return
		}
		ok, held := failures.allow(status, time.Now())
		if !ok {
			return
		}
		ev := reqLogger.Warn()
		if status >= 500 {
			ev = reqLogger.Error()
		}
		ev.Str("method", r.Method).
			Str("path", r.URL.Path).
			Str("remote_addr", r.RemoteAddr).
			Int("status_code", status).
			Int("response_size", wrapped.size).
			Dur("duration_ms", duration).
			Int("same_status_not_logged", held).
			Msg("http_request_failed")
	})
}

// MuxRoute returns a route function reporting the ServeMux pattern that
// serves a request, or "unmatched". For a handler that is not a ServeMux,
// every request is "unmatched".
func MuxRoute(h http.Handler) func(*http.Request) string {
	mux, ok := h.(*http.ServeMux)
	return func(r *http.Request) string {
		if ok {
			if _, pattern := mux.Handler(r); pattern != "" {
				return pattern
			}
		}
		return "unmatched"
	}
}

// MetricsFor records the unheaded_chat_http_* metrics with path = route(r).
// It used to label by r.URL.Path, topic name included, so the family grew
// by ~27 series with every topic ever used.
func MetricsFor(route func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return metricsHandler(route, next)
	}
}

func metricsHandler(route func(*http.Request) string, next http.Handler) http.Handler {
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
			path := route(r)
			m.RecordHTTPRequest(r.Method, path, wrapped.statusCode, duration)
			m.HTTPRequestSize.WithLabelValues(r.Method, path).Observe(float64(r.ContentLength))
			m.HTTPResponseSize.WithLabelValues(r.Method, path).Observe(float64(wrapped.size))
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

// CORS middleware adds CORS headers.
//
// A "*" anywhere in allowedOrigins answers Access-Control-Allow-Origin: *
// and never allows credentials. A named list echoes only a matching origin,
// allows credentials for it, and sets Vary: Origin. It used to reflect any
// Origin with Allow-Credentials: true under "*" (Wotan's default), which let
// any web page make credentialed requests and read the answers.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	wildcard := false
	named := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if o == "*" {
			wildcard = true
		} else if o != "" {
			named[o] = true
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			if origin != "" && (wildcard || named[origin]) {
				h := w.Header()
				if wildcard {
					h.Set("Access-Control-Allow-Origin", "*")
				} else {
					h.Set("Access-Control-Allow-Origin", origin)
					h.Set("Access-Control-Allow-Credentials", "true")
					h.Add("Vary", "Origin")
				}
				h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")
				h.Set("Access-Control-Max-Age", "86400")
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

// Timeout bounds each request with http.TimeoutHandler: the response is
// buffered, and on timeout the client gets 503 with a JSON body while the
// handler's later writes are dropped.
//
// This used to run the handler in a goroutine and answer 504 itself on
// timeout, while that goroutine went on writing to the same ResponseWriter.
// Once the outer handler returned, net/http recycled the writer: a /metrics
// scrape slowed by load wrote into a nil bufio.Writer and the panic killed
// Wotan (2026-10-18), out of Recovery's reach on its own goroutine.
// TimeoutHandler re-raises a handler panic on the request's goroutine.
func Timeout(duration time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.TimeoutHandler(next, duration, `{"error":"request_timeout","message":"Request exceeded timeout"}`)
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
