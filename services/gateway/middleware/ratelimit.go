// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package middleware

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"unheaded/pkg/logger"
	"unheaded/pkg/metrics"
	"unheaded/services/gateway/config"
)

// TokenBucket implements the token bucket algorithm.
type TokenBucket struct {
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time
	mu         sync.Mutex
}

// NewTokenBucket creates a new token bucket.
func NewTokenBucket(rate float64, burst int) *TokenBucket {
	return &TokenBucket{
		tokens:     float64(burst),
		maxTokens:  float64(burst),
		refillRate: rate,
		lastRefill: time.Now(),
	}
}

// Allow checks if a request is allowed and consumes a token.
func (b *TokenBucket) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens += elapsed * b.refillRate
	if b.tokens > b.maxTokens {
		b.tokens = b.maxTokens
	}
	b.lastRefill = now

	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

// Tokens returns the current number of tokens.
func (b *TokenBucket) Tokens() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.tokens
}

// RateLimiter manages rate limits per client.
type RateLimiter struct {
	trustedProxies   map[string]bool
	cfg              *config.RateLimitConfig
	log              *logger.Logger
	metrics          *metrics.Registry
	rateLimitCounter *metrics.Counter
	buckets          map[string]*TokenBucket
	mu               sync.RWMutex
	stopCh           chan struct{}
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(cfg *config.RateLimitConfig, log *logger.Logger, metricsReg *metrics.Registry) *RateLimiter {
	rateLimitCounter := metrics.NewCounter(
		"gateway_rate_limit_exceeded_total",
		"Total number of rate limit exceeded responses",
		nil,
	)

	if metricsReg != nil {
		_ = metricsReg.Register(rateLimitCounter)
	}

	rl := &RateLimiter{
		cfg:              cfg,
		log:              log,
		metrics:          metricsReg,
		rateLimitCounter: rateLimitCounter,
		trustedProxies:   TrustedProxySet(cfg.TrustedProxies),
		buckets:          make(map[string]*TokenBucket),
		stopCh:           make(chan struct{}),
	}

	// Start cleanup goroutine
	go rl.cleanup()

	return rl
}

// Handler returns the middleware handler.
func (rl *RateLimiter) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.cfg.Enabled {
			next.ServeHTTP(w, r)
			return
		}

		clientIP := rl.getClientIP(r)
		bucket := rl.getBucket(clientIP)

		if !bucket.Allow() {
			rl.log.Warn().
				Str("client_ip", clientIP).
				Str("path", r.URL.Path).
				Msg("Rate limit exceeded")

			if rl.metrics != nil {
				rl.rateLimitCounter.Inc()
			}

			rl.writeRateLimitError(w, bucket)
			return
		}

		// Add rate limit headers
		w.Header().Set("X-RateLimit-Limit", formatFloat(rl.cfg.RequestsPerSec))
		w.Header().Set("X-RateLimit-Remaining", formatFloat(bucket.Tokens()))

		next.ServeHTTP(w, r)
	})
}

// getBucket returns or creates a token bucket for a client.
func (rl *RateLimiter) getBucket(clientIP string) *TokenBucket {
	rl.mu.RLock()
	bucket, ok := rl.buckets[clientIP]
	rl.mu.RUnlock()

	if ok {
		return bucket
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Double-check after acquiring write lock
	if bucket, ok = rl.buckets[clientIP]; ok {
		return bucket
	}

	bucket = NewTokenBucket(rl.cfg.RequestsPerSec, rl.cfg.BurstSize)
	rl.buckets[clientIP] = bucket
	return bucket
}

// getClientIP is the client address this limiter keys on.
func (rl *RateLimiter) getClientIP(r *http.Request) string {
	return ClientIP(r, rl.trustedProxies)
}

// TrustedProxySet is addrs as ClientIP takes them.
func TrustedProxySet(addrs []string) map[string]bool {
	set := make(map[string]bool, len(addrs))
	for _, a := range addrs {
		set[a] = true
	}
	return set
}

// ClientIP is the address of the client behind r. The direct peer is the
// answer unless it is a trusted proxy. Then X-Forwarded-For is read from the
// nearest hop outward: the first entry that is not itself a trusted proxy is
// the client, since each proxy appends and only entries left of that one are
// the client's own. With no X-Forwarded-For, X-Real-IP stands. Anything that
// does not parse ends the walk at the last address known good.
func ClientIP(r *http.Request, trusted map[string]bool) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if !trusted[host] {
		return host
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		hops := strings.Split(xff, ",")
		for i := len(hops) - 1; i >= 0; i-- {
			ip := net.ParseIP(strings.TrimSpace(hops[i]))
			if ip == nil {
				return host
			}
			if !trusted[ip.String()] {
				return ip.String()
			}
			host = ip.String()
		}
		return host
	}
	if ip := net.ParseIP(r.Header.Get("X-Real-IP")); ip != nil {
		return ip.String()
	}
	return host
}

// cleanup periodically removes stale buckets.
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.cfg.CleanupPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.cleanupBuckets()
		case <-rl.stopCh:
			return
		}
	}
}

// cleanupBuckets removes buckets that are full (idle clients).
func (rl *RateLimiter) cleanupBuckets() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	for ip, bucket := range rl.buckets {
		if bucket.Tokens() >= bucket.maxTokens*0.99 {
			delete(rl.buckets, ip)
		}
	}

	rl.log.Debug().
		Int("remaining_buckets", len(rl.buckets)).
		Msg("Rate limiter cleanup")
}

// Stop stops the rate limiter cleanup goroutine.
func (rl *RateLimiter) Stop() {
	close(rl.stopCh)
}

// writeRateLimitError writes a rate limit exceeded response.
func (rl *RateLimiter) writeRateLimitError(w http.ResponseWriter, bucket *TokenBucket) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", "1")
	w.Header().Set("X-RateLimit-Limit", formatFloat(rl.cfg.RequestsPerSec))
	w.Header().Set("X-RateLimit-Remaining", "0")
	w.WriteHeader(http.StatusTooManyRequests)

	json.NewEncoder(w).Encode(map[string]interface{}{ // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
		"error":       "rate_limit_exceeded",
		"message":     "Too many requests. Please try again later.",
		"retry_after": 1,
	})
}

// formatFloat formats a float for headers.
func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// RouteRateLimiter provides per-route rate limiting.
type RouteRateLimiter struct {
	limiters map[string]*RateLimiter
	mu       sync.RWMutex
}

// NewRouteRateLimiter creates a new route-specific rate limiter.
func NewRouteRateLimiter() *RouteRateLimiter {
	return &RouteRateLimiter{
		limiters: make(map[string]*RateLimiter),
	}
}

// AddRoute adds a rate limiter for a specific route.
func (rrl *RouteRateLimiter) AddRoute(path string, cfg *config.RateLimitConfig, log *logger.Logger, metricsReg *metrics.Registry) {
	rrl.mu.Lock()
	defer rrl.mu.Unlock()
	rrl.limiters[path] = NewRateLimiter(cfg, log, metricsReg)
}

// GetLimiter returns the rate limiter for a path.
func (rrl *RouteRateLimiter) GetLimiter(path string) *RateLimiter {
	rrl.mu.RLock()
	defer rrl.mu.RUnlock()

	for prefix, limiter := range rrl.limiters {
		if len(path) >= len(prefix) && path[:len(prefix)] == prefix {
			return limiter
		}
	}
	return nil
}

// Stop stops all rate limiters.
func (rrl *RouteRateLimiter) Stop() {
	rrl.mu.Lock()
	defer rrl.mu.Unlock()

	for _, limiter := range rrl.limiters {
		limiter.Stop()
	}
}
