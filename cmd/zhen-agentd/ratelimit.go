// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

package main

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// rateLimiter implements per-IP token-bucket rate limiting. Same shape
// as services/wotan/internal/middleware/ratelimit.go but inlined here
// to avoid a cmd/→services/internal/ import.
type rateLimiter struct {
	mu       sync.RWMutex
	limiters map[string]*rate.Limiter
	rate     rate.Limit
	burst    int
}

// newRateLimiter constructs a per-IP token-bucket limiter and starts
// a background sweeper that evicts inactive limiters every minute.
// rps=0 disables rate limiting (returns nil).
func newRateLimiter(rps float64, burst int) *rateLimiter {
	if rps <= 0 {
		return nil
	}
	rl := &rateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rate:     rate.Limit(rps),
		burst:    burst,
	}
	go rl.sweep()
	return rl
}

func (rl *rateLimiter) sweep() {
	t := time.NewTicker(60 * time.Second)
	defer t.Stop()
	for range t.C {
		rl.mu.Lock()
		for ip, l := range rl.limiters {
			// Inactive == bucket full → safe to drop, callers will
			// re-create on next hit.
			if l.Tokens() == float64(rl.burst) {
				delete(rl.limiters, ip)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *rateLimiter) get(ip string) *rate.Limiter {
	rl.mu.RLock()
	l, ok := rl.limiters[ip]
	rl.mu.RUnlock()
	if ok {
		return l
	}
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if l, ok := rl.limiters[ip]; ok {
		return l
	}
	l = rate.NewLimiter(rl.rate, rl.burst)
	rl.limiters[ip] = l
	return l
}

// Middleware returns the rate-limit middleware. Requests over the
// per-IP token-bucket get HTTP 429 with a Retry-After header.
//
// The /health, /ready, /metrics endpoints are NEVER rate-limited so
// orchestrators can probe at any cadence.
func (rl *rateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health", "/ready", "/metrics":
			next.ServeHTTP(w, r)
			return
		}
		ip := clientIP(r)
		if !rl.get(ip).Allow() {
			rateLimitedRequests.Inc()
			w.Header().Set("Retry-After", "1")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"rate limit exceeded"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// clientIP extracts the best-available client identifier from the
// request. Prefers the right-most IP in X-Forwarded-For (the closest
// trusted proxy's view of the client) when present; otherwise falls
// back to RemoteAddr. Trust X-Forwarded-For ONLY when the daemon is
// behind a known reverse proxy that injects it.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Last entry is the most-trusted proxy's view of the source.
		parts := strings.Split(xff, ",")
		ip := strings.TrimSpace(parts[len(parts)-1])
		if ip != "" {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
