// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package middleware

import (
	"context"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiter implements token bucket rate limiting per client
type RateLimiter struct {
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
	rate     rate.Limit
	burst    int
	cleanup  time.Duration
}

// NewRateLimiter creates a new rate limiter
// rate: tokens per second
// burst: maximum burst size
func NewRateLimiter(r rate.Limit, burst int, cleanup time.Duration) *RateLimiter {
	rl := &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rate:     r,
		burst:    burst,
		cleanup:  cleanup,
	}

	// Periodic cleanup of old limiters
	go rl.cleanupRoutine()

	return rl
}

// GetLimiter returns the rate limiter for a specific key
func (rl *RateLimiter) GetLimiter(key string) *rate.Limiter {
	rl.mu.RLock()
	limiter, exists := rl.limiters[key]
	rl.mu.RUnlock()

	if !exists {
		rl.mu.Lock()
		// Double-check after acquiring write lock
		limiter, exists = rl.limiters[key]
		if !exists {
			limiter = rate.NewLimiter(rl.rate, rl.burst)
			rl.limiters[key] = limiter
		}
		rl.mu.Unlock()
	}

	return limiter
}

// cleanupRoutine periodically removes inactive limiters
func (rl *RateLimiter) cleanupRoutine() {
	ticker := time.NewTicker(rl.cleanup)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		// Remove limiters with full token bucket (inactive)
		for key, limiter := range rl.limiters {
			if limiter.Tokens() == float64(rl.burst) {
				delete(rl.limiters, key)
			}
		}
		rl.mu.Unlock()
	}
}

// Middleware returns HTTP middleware for rate limiting
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Use client IP as key
		key := r.RemoteAddr
		limiter := rl.GetLimiter(key)

		if !limiter.Allow() {
			http.Error(w, `{"error":"rate_limit_exceeded","message":"Too many requests"}`, http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Wait waits until rate limit allows or context is cancelled
func (rl *RateLimiter) Wait(ctx context.Context, key string) error {
	limiter := rl.GetLimiter(key)
	return limiter.Wait(ctx)
}

// Allow checks if rate limit allows request
func (rl *RateLimiter) Allow(key string) bool {
	limiter := rl.GetLimiter(key)
	return limiter.Allow()
}

// Reserve reserves a token, returning delay until available
func (rl *RateLimiter) Reserve(key string) time.Duration {
	limiter := rl.GetLimiter(key)
	reservation := limiter.Reserve()
	if !reservation.OK() {
		return 0
	}
	return reservation.Delay()
}
