// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package resilience provides resilience patterns for the service mesh.
// THE HAUBERK - Rate limiting for service protection.
package resilience

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	// ErrRateLimitExceeded is returned when rate limit is exceeded.
	ErrRateLimitExceeded = errors.New("rate limit exceeded")
)

// RateLimitConfig configures rate limiting behavior.
type RateLimitConfig struct {
	// RequestsPerSecond is the allowed requests per second.
	RequestsPerSecond float64
	// BurstSize is the maximum burst size.
	BurstSize int
	// OnLimitReached is called when rate limit is reached.
	OnLimitReached func(service string)
}

// DefaultRateLimitConfig returns a default rate limit configuration.
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		RequestsPerSecond: 100,
		BurstSize:         10,
	}
}

// TokenBucket implements token bucket rate limiting.
type TokenBucket struct {
	config RateLimitConfig

	mu         sync.Mutex
	tokens     float64
	lastRefill time.Time
}

// NewTokenBucket creates a new token bucket rate limiter.
func NewTokenBucket(config RateLimitConfig) *TokenBucket {
	if config.RequestsPerSecond <= 0 {
		config.RequestsPerSecond = 100
	}
	if config.BurstSize <= 0 {
		config.BurstSize = 10
	}

	return &TokenBucket{
		config:     config,
		tokens:     float64(config.BurstSize),
		lastRefill: time.Now(),
	}
}

// Allow checks if a request is allowed.
func (tb *TokenBucket) Allow() bool {
	return tb.AllowN(1)
}

// AllowN checks if n requests are allowed.
func (tb *TokenBucket) AllowN(n int) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()

	if tb.tokens >= float64(n) {
		tb.tokens -= float64(n)
		return true
	}

	return false
}

// refill adds tokens based on elapsed time.
func (tb *TokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.lastRefill = now

	tb.tokens += elapsed * tb.config.RequestsPerSecond
	if tb.tokens > float64(tb.config.BurstSize) {
		tb.tokens = float64(tb.config.BurstSize)
	}
}

// Tokens returns the current number of tokens.
func (tb *TokenBucket) Tokens() float64 {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.refill()
	return tb.tokens
}

// RateLimiter manages rate limits for multiple services.
type RateLimiter struct {
	defaultConfig RateLimitConfig

	mu       sync.RWMutex
	limiters map[string]*TokenBucket
	configs  map[string]RateLimitConfig
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(defaultConfig RateLimitConfig) *RateLimiter {
	return &RateLimiter{
		defaultConfig: defaultConfig,
		limiters:      make(map[string]*TokenBucket),
		configs:       make(map[string]RateLimitConfig),
	}
}

// SetServiceLimit sets rate limit for a specific service.
func (rl *RateLimiter) SetServiceLimit(service string, config RateLimitConfig) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.configs[service] = config
	rl.limiters[service] = NewTokenBucket(config)
}

// getLimiter returns the limiter for a service, creating if needed.
func (rl *RateLimiter) getLimiter(service string) *TokenBucket {
	rl.mu.RLock()
	if limiter, ok := rl.limiters[service]; ok {
		rl.mu.RUnlock()
		return limiter
	}
	rl.mu.RUnlock()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Double-check after acquiring write lock
	if limiter, ok := rl.limiters[service]; ok {
		return limiter
	}

	config := rl.defaultConfig
	if cfg, ok := rl.configs[service]; ok {
		config = cfg
	}

	limiter := NewTokenBucket(config)
	rl.limiters[service] = limiter
	return limiter
}

// Allow checks if a request to a service is allowed.
func (rl *RateLimiter) Allow(service string) bool {
	limiter := rl.getLimiter(service)
	allowed := limiter.Allow()

	if !allowed && rl.defaultConfig.OnLimitReached != nil {
		rl.defaultConfig.OnLimitReached(service)
	}

	return allowed
}

// AllowN checks if n requests to a service are allowed.
func (rl *RateLimiter) AllowN(service string, n int) bool {
	limiter := rl.getLimiter(service)
	allowed := limiter.AllowN(n)

	if !allowed && rl.defaultConfig.OnLimitReached != nil {
		rl.defaultConfig.OnLimitReached(service)
	}

	return allowed
}

// Wait waits until a request is allowed or context is cancelled.
func (rl *RateLimiter) Wait(ctx context.Context, service string) error {
	limiter := rl.getLimiter(service)

	for {
		if limiter.Allow() {
			return nil
		}

		// Calculate wait time based on token refill rate
		waitTime := time.Duration(float64(time.Second) / rl.defaultConfig.RequestsPerSecond)
		if waitTime < time.Millisecond {
			waitTime = time.Millisecond
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitTime):
			continue
		}
	}
}

// Execute executes a function if rate limit allows.
func (rl *RateLimiter) Execute(ctx context.Context, service string, fn func(context.Context) error) error {
	if !rl.Allow(service) {
		return ErrRateLimitExceeded
	}
	return fn(ctx)
}

// ExecuteWithWait executes a function, waiting for rate limit if needed.
func (rl *RateLimiter) ExecuteWithWait(ctx context.Context, service string, fn func(context.Context) error) error {
	if err := rl.Wait(ctx, service); err != nil {
		return err
	}
	return fn(ctx)
}

// Metrics returns rate limit metrics for a service.
func (rl *RateLimiter) Metrics(service string) RateLimitMetrics {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	config := rl.defaultConfig
	if cfg, ok := rl.configs[service]; ok {
		config = cfg
	}

	var tokens float64
	if limiter, ok := rl.limiters[service]; ok {
		tokens = limiter.Tokens()
	}

	return RateLimitMetrics{
		Service:           service,
		RequestsPerSecond: config.RequestsPerSecond,
		BurstSize:         config.BurstSize,
		AvailableTokens:   tokens,
	}
}

// RateLimitMetrics holds metrics for rate limiting.
type RateLimitMetrics struct {
	Service           string
	RequestsPerSecond float64
	BurstSize         int
	AvailableTokens   float64
}
