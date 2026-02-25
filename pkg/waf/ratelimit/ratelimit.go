// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package ratelimit provides rate limiting functionality
package ratelimit

import (
	"context"
	"sync"
	"time"
)

// Limiter is the interface for rate limiters
type Limiter interface {
	Allow(key string) bool
	AllowN(key string, n int) bool
	Reset(key string)
	Close() error
}

// Config holds rate limiter configuration
type Config struct {
	Rate     int           // Requests per window
	Window   time.Duration // Time window
	BurstMax int           // Maximum burst size
}

// Result represents the result of a rate limit check
type Result struct {
	Allowed   bool
	Remaining int
	ResetAt   time.Time
	RetryAt   time.Time
}

// MultiLimiter combines multiple rate limiters
type MultiLimiter struct {
	limiters []Limiter
	mu       sync.RWMutex
}

// NewMultiLimiter creates a new multi-limiter
func NewMultiLimiter(limiters ...Limiter) *MultiLimiter {
	return &MultiLimiter{
		limiters: limiters,
	}
}

// Allow checks all limiters and returns true only if all allow
func (m *MultiLimiter) Allow(key string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, limiter := range m.limiters {
		if !limiter.Allow(key) {
			return false
		}
	}
	return true
}

// AllowN checks all limiters for N requests
func (m *MultiLimiter) AllowN(key string, n int) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, limiter := range m.limiters {
		if !limiter.AllowN(key, n) {
			return false
		}
	}
	return true
}

// Reset resets all limiters for a key
func (m *MultiLimiter) Reset(key string) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, limiter := range m.limiters {
		limiter.Reset(key)
	}
}

// Close closes all limiters
func (m *MultiLimiter) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, limiter := range m.limiters {
		if err := limiter.Close(); err != nil {
			return err
		}
	}
	return nil
}

// AddLimiter adds a limiter
func (m *MultiLimiter) AddLimiter(limiter Limiter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.limiters = append(m.limiters, limiter)
}

// EndpointLimiter provides per-endpoint rate limiting
type EndpointLimiter struct {
	limiters map[string]Limiter
	factory  func() Limiter
	mu       sync.RWMutex
}

// NewEndpointLimiter creates a new endpoint-based limiter
func NewEndpointLimiter(factory func() Limiter) *EndpointLimiter {
	return &EndpointLimiter{
		limiters: make(map[string]Limiter),
		factory:  factory,
	}
}

// Allow checks rate limit for an endpoint
func (e *EndpointLimiter) Allow(endpoint, key string) bool {
	limiter := e.getLimiter(endpoint)
	return limiter.Allow(key)
}

// AllowN checks rate limit for N requests
func (e *EndpointLimiter) AllowN(endpoint, key string, n int) bool {
	limiter := e.getLimiter(endpoint)
	return limiter.AllowN(key, n)
}

// getLimiter gets or creates a limiter for an endpoint
func (e *EndpointLimiter) getLimiter(endpoint string) Limiter {
	e.mu.RLock()
	limiter, ok := e.limiters[endpoint]
	e.mu.RUnlock()

	if ok {
		return limiter
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// Double-check
	if limiter, ok = e.limiters[endpoint]; ok {
		return limiter
	}

	limiter = e.factory()
	e.limiters[endpoint] = limiter
	return limiter
}

// Reset resets the limiter for an endpoint
func (e *EndpointLimiter) Reset(endpoint, key string) {
	e.mu.RLock()
	limiter, ok := e.limiters[endpoint]
	e.mu.RUnlock()

	if ok {
		limiter.Reset(key)
	}
}

// Close closes all endpoint limiters
func (e *EndpointLimiter) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, limiter := range e.limiters {
		if err := limiter.Close(); err != nil {
			return err
		}
	}
	return nil
}

// RateLimitMiddleware provides context-aware rate limiting
type RateLimitMiddleware struct {
	limiter        Limiter
	keyExtractor   func(ctx context.Context) string
	onLimitReached func(ctx context.Context, key string)
}

// NewRateLimitMiddleware creates a new rate limit middleware
func NewRateLimitMiddleware(limiter Limiter, keyExtractor func(ctx context.Context) string) *RateLimitMiddleware {
	return &RateLimitMiddleware{
		limiter:      limiter,
		keyExtractor: keyExtractor,
	}
}

// SetLimitReachedHandler sets the handler for when limit is reached
func (m *RateLimitMiddleware) SetLimitReachedHandler(handler func(ctx context.Context, key string)) {
	m.onLimitReached = handler
}

// Check checks if the request should be allowed
func (m *RateLimitMiddleware) Check(ctx context.Context) bool {
	key := m.keyExtractor(ctx)
	allowed := m.limiter.Allow(key)

	if !allowed && m.onLimitReached != nil {
		m.onLimitReached(ctx, key)
	}

	return allowed
}
