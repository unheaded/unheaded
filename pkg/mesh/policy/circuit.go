// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package policy

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

var (
	// ErrCircuitOpen indicates the circuit breaker is open.
	ErrCircuitOpen = errors.New("circuit breaker is open")
	// ErrTooManyRequests indicates too many requests in half-open state.
	ErrTooManyRequests = errors.New("too many requests in half-open state")
)

// CircuitState represents the state of a circuit breaker.
type CircuitState int

const (
	StateClosed CircuitState = iota
	StateOpen
	StateHalfOpen
)

func (s CircuitState) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreaker implements the circuit breaker pattern.
type CircuitBreaker struct {
	config *CircuitBreakerConfig

	mu               sync.RWMutex
	state            CircuitState
	failures         int
	successes        int
	consecutiveErrs  int
	lastFailure      time.Time
	halfOpenRequests int32

	// Sliding window for failure rate calculation
	window      []bool // true = success, false = failure
	windowIdx   int
	windowCount int

	// Callbacks
	onStateChange func(from, to CircuitState)
}

// NewCircuitBreaker creates a new circuit breaker.
func NewCircuitBreaker(config *CircuitBreakerConfig) *CircuitBreaker {
	if config == nil {
		config = &CircuitBreakerConfig{
			Enabled:          true,
			FailureThreshold: 5,
			SuccessThreshold: 3,
			HalfOpenRequests: 1,
			Timeout:          30 * time.Second,
			WindowDuration:   time.Minute,
		}
	}

	windowSize := 100 // default window size
	if config.WindowDuration > 0 {
		// Approximate window size based on expected request rate
		windowSize = 100
	}

	return &CircuitBreaker{
		config: config,
		state:  StateClosed,
		window: make([]bool, windowSize),
	}
}

// Execute runs the given function with circuit breaker protection.
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func(ctx context.Context) error) error {
	if !cb.config.Enabled {
		return fn(ctx)
	}

	// Check if request is allowed
	if err := cb.allowRequest(); err != nil {
		return err
	}

	// Execute the function
	err := fn(ctx)

	// Record the result
	cb.recordResult(err == nil)

	return err
}

// allowRequest checks if a request should be allowed.
func (cb *CircuitBreaker) allowRequest() error {
	cb.mu.RLock()
	state := cb.state
	cb.mu.RUnlock()

	switch state {
	case StateClosed:
		return nil

	case StateOpen:
		// Check if timeout has elapsed
		cb.mu.Lock()
		defer cb.mu.Unlock()

		if time.Since(cb.lastFailure) > cb.config.Timeout {
			cb.setState(StateHalfOpen)
			cb.halfOpenRequests = 0
			return nil
		}
		return ErrCircuitOpen

	case StateHalfOpen:
		// Allow limited requests
		current := atomic.AddInt32(&cb.halfOpenRequests, 1)
		if int(current) > cb.config.HalfOpenRequests {
			atomic.AddInt32(&cb.halfOpenRequests, -1)
			return ErrTooManyRequests
		}
		return nil
	}

	return nil
}

// recordResult records the success or failure of a request.
func (cb *CircuitBreaker) recordResult(success bool) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	// Update sliding window
	cb.window[cb.windowIdx] = success
	cb.windowIdx = (cb.windowIdx + 1) % len(cb.window)
	if cb.windowCount < len(cb.window) {
		cb.windowCount++
	}

	if success {
		cb.successes++
		cb.consecutiveErrs = 0

		switch cb.state {
		case StateHalfOpen:
			if cb.successes >= cb.config.SuccessThreshold {
				cb.setState(StateClosed)
				cb.reset()
			}
		case StateClosed:
			// Reset failure count on success in closed state
			cb.failures = 0
		}
	} else {
		cb.failures++
		cb.consecutiveErrs++
		cb.lastFailure = time.Now()
		cb.successes = 0

		switch cb.state {
		case StateClosed:
			if cb.shouldTrip() {
				cb.setState(StateOpen)
			}
		case StateHalfOpen:
			cb.setState(StateOpen)
		}
	}
}

// shouldTrip checks if the circuit should trip to open state.
func (cb *CircuitBreaker) shouldTrip() bool {
	// Check consecutive errors
	if cb.config.ConsecutiveErrors > 0 && cb.consecutiveErrs >= cb.config.ConsecutiveErrors {
		return true
	}

	// Check failure threshold
	if cb.failures >= cb.config.FailureThreshold {
		return true
	}

	// Check failure rate
	if cb.config.FailureRatePercent > 0 && cb.windowCount >= 10 {
		failures := 0
		for i := 0; i < cb.windowCount; i++ {
			if !cb.window[i] {
				failures++
			}
		}
		rate := float64(failures) / float64(cb.windowCount) * 100
		if rate >= cb.config.FailureRatePercent {
			return true
		}
	}

	return false
}

// setState changes the circuit state.
func (cb *CircuitBreaker) setState(state CircuitState) {
	if cb.state == state {
		return
	}
	old := cb.state
	cb.state = state

	if cb.onStateChange != nil {
		cb.onStateChange(old, state)
	}
}

// reset resets the circuit breaker counters.
func (cb *CircuitBreaker) reset() {
	cb.failures = 0
	cb.successes = 0
	cb.consecutiveErrs = 0
	cb.halfOpenRequests = 0
	cb.windowIdx = 0
	cb.windowCount = 0
	for i := range cb.window {
		cb.window[i] = true
	}
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

	failures := 0
	for i := 0; i < cb.windowCount; i++ {
		if !cb.window[i] {
			failures++
		}
	}

	var failureRate float64
	if cb.windowCount > 0 {
		failureRate = float64(failures) / float64(cb.windowCount) * 100
	}

	return CircuitStats{
		State:           cb.state,
		Failures:        cb.failures,
		Successes:       cb.successes,
		ConsecutiveErrs: cb.consecutiveErrs,
		FailureRate:     failureRate,
		LastFailure:     cb.lastFailure,
	}
}

// OnStateChange sets the state change callback.
func (cb *CircuitBreaker) OnStateChange(fn func(from, to CircuitState)) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.onStateChange = fn
}

// ForceOpen forces the circuit to open state.
func (cb *CircuitBreaker) ForceOpen() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.setState(StateOpen)
	cb.lastFailure = time.Now()
}

// ForceClose forces the circuit to closed state.
func (cb *CircuitBreaker) ForceClose() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.setState(StateClosed)
	cb.reset()
}

// CircuitStats holds circuit breaker statistics.
type CircuitStats struct {
	State           CircuitState
	Failures        int
	Successes       int
	ConsecutiveErrs int
	FailureRate     float64
	LastFailure     time.Time
}

// CircuitBreakerRegistry manages circuit breakers for multiple services.
type CircuitBreakerRegistry struct {
	mu       sync.RWMutex
	breakers map[string]*CircuitBreaker
	config   *CircuitBreakerConfig
}

// NewCircuitBreakerRegistry creates a new registry.
func NewCircuitBreakerRegistry(defaultConfig *CircuitBreakerConfig) *CircuitBreakerRegistry {
	return &CircuitBreakerRegistry{
		breakers: make(map[string]*CircuitBreaker),
		config:   defaultConfig,
	}
}

// Get returns the circuit breaker for a service, creating one if needed.
func (r *CircuitBreakerRegistry) Get(service string) *CircuitBreaker {
	r.mu.RLock()
	if cb, ok := r.breakers[service]; ok {
		r.mu.RUnlock()
		return cb
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check after acquiring write lock
	if cb, ok := r.breakers[service]; ok {
		return cb
	}

	cb := NewCircuitBreaker(r.config)
	r.breakers[service] = cb
	return cb
}

// Remove removes a circuit breaker.
func (r *CircuitBreakerRegistry) Remove(service string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.breakers, service)
}

// Stats returns stats for all circuit breakers.
func (r *CircuitBreakerRegistry) Stats() map[string]CircuitStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := make(map[string]CircuitStats, len(r.breakers))
	for name, cb := range r.breakers {
		stats[name] = cb.Stats()
	}
	return stats
}
