// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package proxy

import (
	"sync"
	"time"

	"unheaded/pkg/logger"
	"unheaded/services/gateway/config"
)

// CircuitState represents the state of a circuit breaker.
type CircuitState int

const (
	// CircuitClosed allows requests through.
	CircuitClosed CircuitState = iota
	// CircuitOpen blocks requests.
	CircuitOpen
	// CircuitHalfOpen allows limited requests through.
	CircuitHalfOpen
)

// String returns the string representation of the state.
func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreaker implements the circuit breaker pattern.
type CircuitBreaker struct {
	cfg              *config.CircuitConfig
	log              *logger.Logger
	state            CircuitState
	failures         int
	successes        int
	lastStateChange  time.Time
	halfOpenRequests int
	mu               sync.RWMutex
}

// NewCircuitBreaker creates a new circuit breaker.
func NewCircuitBreaker(cfg *config.CircuitConfig, log *logger.Logger) *CircuitBreaker {
	return &CircuitBreaker{
		cfg:             cfg,
		log:             log,
		state:           CircuitClosed,
		lastStateChange: time.Now(),
	}
}

// Allow checks if a request is allowed.
func (cb *CircuitBreaker) Allow() bool {
	if !cb.cfg.Enabled {
		return true
	}

	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true

	case CircuitOpen:
		// Check if timeout has passed
		if time.Since(cb.lastStateChange) > cb.cfg.Timeout {
			cb.transitionTo(CircuitHalfOpen)
			cb.halfOpenRequests = 1
			return true
		}
		return false

	case CircuitHalfOpen:
		// Allow limited requests in half-open state
		if cb.halfOpenRequests < cb.cfg.HalfOpenRequests {
			cb.halfOpenRequests++
			return true
		}
		return false
	}

	return false
}

// RecordSuccess records a successful request.
func (cb *CircuitBreaker) RecordSuccess() {
	if !cb.cfg.Enabled {
		return
	}

	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		// Reset failure count on success
		cb.failures = 0

	case CircuitHalfOpen:
		cb.successes++
		if cb.successes >= cb.cfg.SuccessThreshold {
			cb.transitionTo(CircuitClosed)
		}
	}
}

// RecordFailure records a failed request.
func (cb *CircuitBreaker) RecordFailure() {
	if !cb.cfg.Enabled {
		return
	}

	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		cb.failures++
		if cb.failures >= cb.cfg.FailureThreshold {
			cb.transitionTo(CircuitOpen)
		}

	case CircuitHalfOpen:
		// Any failure in half-open state opens the circuit
		cb.transitionTo(CircuitOpen)
	}
}

// transitionTo changes the circuit state.
func (cb *CircuitBreaker) transitionTo(state CircuitState) {
	if cb.state == state {
		return
	}

	cb.log.Info().
		Str("from", cb.state.String()).
		Str("to", state.String()).
		Msg("Circuit breaker state change")

	cb.state = state
	cb.lastStateChange = time.Now()
	cb.failures = 0
	cb.successes = 0
	cb.halfOpenRequests = 0
}

// State returns the current circuit state.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Stats returns circuit breaker statistics.
func (cb *CircuitBreaker) Stats() CircuitStats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return CircuitStats{
		State:           cb.state.String(),
		Failures:        cb.failures,
		Successes:       cb.successes,
		LastStateChange: cb.lastStateChange,
	}
}

// CircuitStats holds circuit breaker statistics.
type CircuitStats struct {
	State           string    `json:"state"`
	Failures        int       `json:"failures"`
	Successes       int       `json:"successes"`
	LastStateChange time.Time `json:"last_state_change"`
}

// Reset resets the circuit breaker to closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.transitionTo(CircuitClosed)
}

// CircuitBreakerRegistry manages circuit breakers for multiple backends.
type CircuitBreakerRegistry struct {
	breakers map[string]*CircuitBreaker
	cfg      *config.CircuitConfig
	log      *logger.Logger
	mu       sync.RWMutex
}

// NewCircuitBreakerRegistry creates a new circuit breaker registry.
func NewCircuitBreakerRegistry(cfg *config.CircuitConfig, log *logger.Logger) *CircuitBreakerRegistry {
	return &CircuitBreakerRegistry{
		breakers: make(map[string]*CircuitBreaker),
		cfg:      cfg,
		log:      log,
	}
}

// Get returns or creates a circuit breaker for a backend.
func (r *CircuitBreakerRegistry) Get(backend string) *CircuitBreaker {
	r.mu.RLock()
	cb, ok := r.breakers[backend]
	r.mu.RUnlock()

	if ok {
		return cb
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check after acquiring write lock
	if cb, ok = r.breakers[backend]; ok {
		return cb
	}

	cb = NewCircuitBreaker(r.cfg, r.log)
	r.breakers[backend] = cb
	return cb
}

// Stats returns statistics for all circuit breakers.
func (r *CircuitBreakerRegistry) Stats() map[string]CircuitStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := make(map[string]CircuitStats)
	for backend, cb := range r.breakers {
		stats[backend] = cb.Stats()
	}
	return stats
}

// ResetAll resets all circuit breakers.
func (r *CircuitBreakerRegistry) ResetAll() {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, cb := range r.breakers {
		cb.Reset()
	}
}
