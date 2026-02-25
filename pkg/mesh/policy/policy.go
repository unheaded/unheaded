// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package policy provides traffic policy management for THE HAUBERK.
package policy

import (
	"sync"
	"time"
)

// Policy defines traffic handling rules for a service.
type Policy struct {
	// Service identification
	ServiceName string
	Namespace   string

	// Timeout configuration
	Timeout        time.Duration
	IdleTimeout    time.Duration
	ConnectTimeout time.Duration

	// Retry configuration
	Retry *RetryPolicy

	// Circuit breaker configuration
	CircuitBreaker *CircuitBreakerConfig

	// Rate limiting
	RateLimit *RateLimitConfig

	// Load balancing
	LoadBalancer string // round-robin, least-conn, weighted

	// Health check
	HealthCheck *HealthCheckConfig
}

// RetryPolicy defines retry behavior.
type RetryPolicy struct {
	MaxRetries      int
	RetryOn         []string // connection-failure, 5xx, retriable-4xx, reset
	BackoffBase     time.Duration
	BackoffMax      time.Duration
	BackoffJitter   float64
	RetryableStatus []int
}

// CircuitBreakerConfig defines circuit breaker settings.
type CircuitBreakerConfig struct {
	Enabled            bool
	FailureThreshold   int           // failures before opening
	SuccessThreshold   int           // successes before closing
	HalfOpenRequests   int           // requests allowed in half-open
	Timeout            time.Duration // time before half-open
	ConsecutiveErrors  int           // consecutive errors to trip
	FailureRatePercent float64       // failure rate threshold
	WindowDuration     time.Duration // sliding window duration
}

// RateLimitConfig defines rate limiting settings.
type RateLimitConfig struct {
	Enabled    bool
	Rate       int           // requests per interval
	Interval   time.Duration // time interval
	BurstSize  int           // maximum burst
	PerService bool          // per-service or per-endpoint
}

// HealthCheckConfig defines health check settings.
type HealthCheckConfig struct {
	Enabled      bool
	Interval     time.Duration
	Timeout      time.Duration
	Path         string // HTTP path for health checks
	Threshold    int    // consecutive failures before unhealthy
	Protocol     string // tcp, http, grpc
	ExpectedCode int    // expected HTTP status code
}

// DefaultPolicy returns a policy with sensible defaults.
func DefaultPolicy() *Policy {
	return &Policy{
		Timeout:        30 * time.Second,
		IdleTimeout:    5 * time.Minute,
		ConnectTimeout: 5 * time.Second,
		Retry: &RetryPolicy{
			MaxRetries:    3,
			RetryOn:       []string{"connection-failure", "5xx", "reset"},
			BackoffBase:   25 * time.Millisecond,
			BackoffMax:    1 * time.Second,
			BackoffJitter: 0.2,
		},
		CircuitBreaker: &CircuitBreakerConfig{
			Enabled:           true,
			FailureThreshold:  5,
			SuccessThreshold:  3,
			HalfOpenRequests:  1,
			Timeout:           30 * time.Second,
			ConsecutiveErrors: 5,
		},
		LoadBalancer: "round-robin",
		HealthCheck: &HealthCheckConfig{
			Enabled:      true,
			Interval:     10 * time.Second,
			Timeout:      5 * time.Second,
			Path:         "/health",
			Threshold:    3,
			Protocol:     "http",
			ExpectedCode: 200,
		},
	}
}

// PolicyStore manages policies for services.
type PolicyStore struct {
	mu       sync.RWMutex
	policies map[string]*Policy // key: namespace/service
	default_ *Policy
}

// NewPolicyStore creates a new policy store.
func NewPolicyStore() *PolicyStore {
	return &PolicyStore{
		policies: make(map[string]*Policy),
		default_: DefaultPolicy(),
	}
}

// Set stores a policy for a service.
func (ps *PolicyStore) Set(namespace, service string, policy *Policy) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	key := namespace + "/" + service
	ps.policies[key] = policy
}

// Get retrieves a policy for a service.
func (ps *PolicyStore) Get(namespace, service string) *Policy {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	key := namespace + "/" + service
	if policy, ok := ps.policies[key]; ok {
		return policy
	}
	return ps.default_
}

// Delete removes a policy.
func (ps *PolicyStore) Delete(namespace, service string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	key := namespace + "/" + service
	delete(ps.policies, key)
}

// SetDefault sets the default policy.
func (ps *PolicyStore) SetDefault(policy *Policy) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.default_ = policy
}

// List returns all stored policies.
func (ps *PolicyStore) List() map[string]*Policy {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	result := make(map[string]*Policy, len(ps.policies))
	for k, v := range ps.policies {
		result[k] = v
	}
	return result
}
