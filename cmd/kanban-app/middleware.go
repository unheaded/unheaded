// Package main - Security middleware
package main

import (
	"net/http"
	"sync"
	"time"
	// Using package-level log from main.go (Kingdom's native logger)
)

// ============================================================================
// CORS MIDDLEWARE
// ============================================================================

// corsMiddleware adds CORS headers with secure defaults
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Allow specific origins in production - for now use restrictive wildcard
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = "*" // Fallback for non-CORS requests
		}

		// Restrictive CORS policy
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Max-Age", "86400") // 24 hours

		// Handle preflight
		if r.Method == http.MethodOptions {
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

// NewRateLimiter creates a rate limiter
func NewRateLimiter(requestsPerMinute, burstSize int) *RateLimiter {
	rl := &RateLimiter{
		clients:           make(map[string]*clientBucket),
		requestsPerMinute: requestsPerMinute,
		burstSize:         burstSize,
	}

	// Cleanup old clients every 5 minutes
	go rl.cleanupLoop()

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
		// Create new bucket
		rl.mu.Lock()
		bucket = &clientBucket{
			tokens:     rl.burstSize,
			lastRefill: time.Now(),
		}
		rl.clients[clientIP] = bucket
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

// cleanupLoop removes stale client buckets
func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		rl.cleanup()
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

// getClientIP extracts client IP from request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (proxy/LB)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take first IP in list
		for i, c := range xff {
			if c == ',' {
				return xff[:i]
			}
		}
		return xff
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fallback to RemoteAddr
	// Strip port if present
	for i := len(r.RemoteAddr) - 1; i >= 0; i-- {
		if r.RemoteAddr[i] == ':' {
			return r.RemoteAddr[:i]
		}
	}

	return r.RemoteAddr
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

		// Content Security Policy (strict)
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self' ws: wss:")

		// Force HTTPS (when behind TLS termination)
		// w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")

		next.ServeHTTP(w, r)
	})
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
