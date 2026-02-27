// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package main - Security middleware
package main

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	// Using package-level log from main.go (Kingdom's native logger)
)

// ============================================================================
// CORS MIDDLEWARE
// ============================================================================

// allowedOrigins contains the whitelist of permitted CORS origins.
// Add production domains here when deploying.
var allowedOrigins = []string{
	"http://localhost:8080",
	"http://localhost:3000",
	"http://127.0.0.1:8080",
	"http://127.0.0.1:3000",
	"https://localhost:8080",
	"https://localhost:3000",
}

// isAllowedOrigin checks if the given origin is in the whitelist.
func isAllowedOrigin(origin string) bool {
	for _, allowed := range allowedOrigins {
		if origin == allowed {
			return true
		}
	}
	return false
}

// corsMiddleware adds CORS headers with origin whitelist
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		// Only set CORS headers if origin is in whitelist
		if origin != "" && isAllowedOrigin(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Max-Age", "86400") // 24 hours
		}

		// Handle preflight
		if r.Method == http.MethodOptions {
			if origin == "" || !isAllowedOrigin(origin) {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// ============================================================================
// RATE LIMITING MIDDLEWARE
// ============================================================================

// RateLimiter implements token bucket rate limiting per client IP
type RateLimiter struct {
	clients map[string]*clientBucket
	mu      sync.RWMutex

	requestsPerMinute int
	burstSize         int
}

type clientBucket struct {
	tokens     int
	lastRefill time.Time
	mu         sync.Mutex
}

// NewRateLimiter creates a rate limiter. The provided context controls the
// lifetime of the background cleanup goroutine — cancelling it stops the loop.
func NewRateLimiter(ctx context.Context, requestsPerMinute, burstSize int) *RateLimiter {
	rl := &RateLimiter{
		clients:           make(map[string]*clientBucket),
		requestsPerMinute: requestsPerMinute,
		burstSize:         burstSize,
	}

	// Cleanup old clients every 5 minutes
	go rl.cleanupLoop(ctx)

	return rl
}

// Allow checks if request is allowed
func (rl *RateLimiter) Allow(clientIP string) bool {
	if clientIP == "" {
		return false // No IP = deny
	}

	rl.mu.RLock()
	bucket, exists := rl.clients[clientIP]
	rl.mu.RUnlock()

	if !exists {
		// Create new bucket — double-check after acquiring write lock
		// to prevent race where two goroutines both see !exists.
		rl.mu.Lock()
		bucket, exists = rl.clients[clientIP]
		if !exists {
			bucket = &clientBucket{
				tokens:     rl.burstSize,
				lastRefill: time.Now(),
			}
			rl.clients[clientIP] = bucket
		}
		rl.mu.Unlock()
	}

	return bucket.take(rl.requestsPerMinute, rl.burstSize)
}

// take attempts to take a token from the bucket
func (cb *clientBucket) take(refillRate, burstSize int) bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	// Refill tokens based on time elapsed
	now := time.Now()
	elapsed := now.Sub(cb.lastRefill)
	tokensToAdd := int(elapsed.Seconds() * float64(refillRate) / 60.0)

	if tokensToAdd > 0 {
		cb.tokens += tokensToAdd
		if cb.tokens > burstSize {
			cb.tokens = burstSize
		}
		cb.lastRefill = now
	}

	// Try to take a token
	if cb.tokens > 0 {
		cb.tokens--
		return true
	}

	return false
}

// cleanupLoop removes stale client buckets. It exits when ctx is cancelled.
func (rl *RateLimiter) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rl.cleanup()
		}
	}
}

func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for ip, bucket := range rl.clients {
		bucket.mu.Lock()
		idle := now.Sub(bucket.lastRefill)
		bucket.mu.Unlock()

		// Remove clients idle for >10 minutes
		if idle > 10*time.Minute {
			delete(rl.clients, ip)
		}
	}

	log.Debug().
		Int("active_clients", len(rl.clients)).
		Msg("rate limiter cleanup")
}

// rateLimitMiddleware wraps handler with rate limiting
func rateLimitMiddleware(limiter *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Exempt long-lived streaming endpoints from rate limiting.
			// SSE and WebSocket connections are persistent — consuming a
			// token per reconnect attempt causes cascading rate limit failures.
			if r.URL.Path == "/ws" || r.URL.Path == "/api/v1/stream" {
				next.ServeHTTP(w, r)
				return
			}

			// Extract client IP
			clientIP := getClientIP(r)

			if !limiter.Allow(clientIP) {
				log.Warn().
					Str("client_ip", clientIP).
					Str("path", r.URL.Path).
					Msg("rate limit exceeded")

				http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// trustedProxies holds the set of proxy IPs whose X-Forwarded-For /
// X-Real-IP headers are trusted.  Empty by default — meaning the rate
// limiter always keys on RemoteAddr, which cannot be spoofed by the
// client.  Populate this from configuration when running behind a
// known load-balancer or reverse proxy (e.g. the Unheaded gateway on
// localhost or the LXD bridge).
var trustedProxies = map[string]bool{}

// getClientIP extracts the client IP from the request.
//
// It uses RemoteAddr (with port stripped) by default.  X-Forwarded-For
// and X-Real-IP are only consulted when RemoteAddr belongs to a
// trusted proxy — preventing spoofing by arbitrary clients.
func getClientIP(r *http.Request) string {
	remoteIP := stripPort(r.RemoteAddr)

	// Only trust forwarded headers when the direct peer is a known proxy.
	if trustedProxies[remoteIP] {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			// Take the first (client-supplied) IP in the chain.
			for i, c := range xff {
				if c == ',' {
					return strings.TrimSpace(xff[:i])
				}
			}
			return strings.TrimSpace(xff)
		}
		if xri := r.Header.Get("X-Real-IP"); xri != "" {
			return strings.TrimSpace(xri)
		}
	}

	return remoteIP
}

// stripPort removes the port suffix from an address string.
// Handles both IPv4 ("1.2.3.4:8080") and IPv6 ("[::1]:8080") forms.
func stripPort(addr string) string {
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[:i]
		}
	}
	return addr
}

// ============================================================================
// SECURITY HEADERS MIDDLEWARE
// ============================================================================

// securityHeadersMiddleware adds security headers
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Prevent clickjacking
		w.Header().Set("X-Frame-Options", "DENY")

		// Prevent MIME sniffing
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// XSS protection (legacy but still useful)
		w.Header().Set("X-XSS-Protection", "1; mode=block")

		// Referrer policy
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Content Security Policy — no unsafe-inline for either script-src or style-src.
		// Inline styles were replaced with CSSOM assignments (element.style) which CSP permits.
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; font-src 'self'; connect-src 'self' ws: wss:")

		// Force HTTPS (when behind TLS termination)
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")

		next.ServeHTTP(w, r)
	})
}

// ============================================================================
// REQUEST ID MIDDLEWARE
// ============================================================================

// requestIDMiddleware injects an X-Request-ID header into every request and
// response. If the client already sent an X-Request-ID, it is preserved;
// otherwise a new UUID is generated. The ID is logged in the request context.
func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}
		// Echo back on response for client correlation
		w.Header().Set("X-Request-ID", requestID)

		// Store in request context for downstream handlers and logging
		ctx := context.WithValue(r.Context(), requestIDKey, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requestIDKeyType is an unexported type for the context key, preventing collisions.
type requestIDKeyType struct{}

var requestIDKey = requestIDKeyType{}

// getRequestID extracts the X-Request-ID from a request context.
func getRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

// ============================================================================
// REQUEST SIZE LIMITING MIDDLEWARE
// ============================================================================

// requestSizeLimitMiddleware limits request body size
func requestSizeLimitMiddleware(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Limit request body size
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)

			next.ServeHTTP(w, r)
		})
	}
}
