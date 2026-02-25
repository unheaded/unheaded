// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package resilience provides resilience patterns for the service mesh.
// THE HAUBERK - Circuit breaker implementation for the Unheaded Kingdom.
package resilience

import (
	"context"
	"errors"
	"sync"
	"time"
)

// CircuitState represents the state of a circuit breaker.
type CircuitState int

const (
	// StateClosed allows requests through normally.
	StateClosed CircuitState = iota
	// StateOpen blocks all requests.
	StateOpen
	// StateHalfOpen allows a limited number of test requests.
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

var (
	// ErrCircuitOpen is returned when the circuit is open.
	ErrCircuitOpen = errors.New("circuit breaker is open")
	// ErrTooManyRequests is returned when too many requests in half-open state.
	ErrTooManyRequests = errors.New("too many requests in half-open state")
)

// CircuitBreakerConfig holds configuration for a circuit breaker.
type CircuitBreakerConfig struct {
	// Name identifies this circuit breaker.
	Name string
	// MaxFailures is the number of failures before opening the circuit.
	MaxFailures int
	// Timeout is how long the circuit stays open before transitioning to half-open.
	Timeout time.Duration
	// HalfOpenMaxRequests is the max requests allowed in half-open state.
	HalfOpenMaxRequests int
	// SuccessThreshold is the number of successes needed to close from half-open.
	SuccessThreshold int
	// OnStateChange is called when the state changes.
	OnStateChange func(name string, from, to CircuitState)
}

// DefaultCircuitBreakerConfig returns a default configuration.
func DefaultCircuitBreakerConfig(name string) CircuitBreakerConfig {
	return CircuitBreakerConfig{
		Name:                name,
		MaxFailures:         5,
		Timeout:             30 * time.Second,
		HalfOpenMaxRequests: 3,
		SuccessThreshold:    2,
	}
}

// CircuitBreaker implements the circuit breaker pattern.
type CircuitBreaker struct {
	config CircuitBreakerConfig

	mu               sync.RWMutex
	state            CircuitState
	failures         int
	successes        int
	halfOpenRequests int
	lastFailureTime  time.Time
}

// NewCircuitBreaker creates a new circuit breaker with the given configuration.
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	if config.MaxFailures <= 0 {
		config.MaxFailures = 5
	}
	if config.Timeout <= 0 {
		config.Timeout = 30 * time.Second
	}
	if config.HalfOpenMaxRequests <= 0 {
		config.HalfOpenMaxRequests = 3
	}
	if config.SuccessThreshold <= 0 {
		config.SuccessThreshold = 2
	}

	return &CircuitBreaker{
		config: config,
		state:  StateClosed,
	}
}

// State returns the current state of the circuit breaker.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.currentState()
}

// currentState returns the state, potentially transitioning from open to half-open.
// Must be called with at least a read lock held.
func (cb *CircuitBreaker) currentState() CircuitState {
	if cb.state == StateOpen {
		if time.Since(cb.lastFailureTime) >= cb.config.Timeout {
			return StateHalfOpen
		}
	}
	return cb.state
}

// Execute runs the given function if the circuit allows it.
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func(context.Context) error) error {
	if err := cb.beforeRequest(); err != nil {
		return err
	}

	err := fn(ctx)
	cb.afterRequest(err)
	return err
}

// beforeRequest checks if the request should be allowed.
func (cb *CircuitBreaker) beforeRequest() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	state := cb.currentState()

	switch state {
	case StateClosed:
		return nil
	case StateOpen:
		return ErrCircuitOpen
	case StateHalfOpen:
		if cb.halfOpenRequests >= cb.config.HalfOpenMaxRequests {
			return ErrTooManyRequests
		}
		cb.halfOpenRequests++
		// Transition state if needed
		if cb.state == StateOpen {
			cb.setState(StateHalfOpen)
		}
		return nil
	default:
		return ErrCircuitOpen
	}
}

// afterRequest records the result of a request.
func (cb *CircuitBreaker) afterRequest(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.onFailure()
	} else {
		cb.onSuccess()
	}
}

// onSuccess handles a successful request.
func (cb *CircuitBreaker) onSuccess() {
	switch cb.currentState() {
	case StateClosed:
		cb.failures = 0
	case StateHalfOpen:
		cb.successes++
		if cb.successes >= cb.config.SuccessThreshold {
			cb.setState(StateClosed)
		}
	}
}

// onFailure handles a failed request.
func (cb *CircuitBreaker) onFailure() {
	cb.lastFailureTime = time.Now()

	switch cb.currentState() {
	case StateClosed:
		cb.failures++
		if cb.failures >= cb.config.MaxFailures {
			cb.setState(StateOpen)
		}
	case StateHalfOpen:
		cb.setState(StateOpen)
	}
}

// setState transitions to a new state.
func (cb *CircuitBreaker) setState(newState CircuitState) {
	if cb.state == newState {
		return
	}

	oldState := cb.state
	cb.state = newState

	// Reset counters on state change
	cb.failures = 0
	cb.successes = 0
	cb.halfOpenRequests = 0

	if cb.config.OnStateChange != nil {
		go cb.config.OnStateChange(cb.config.Name, oldState, newState)
	}
}

// Reset resets the circuit breaker to closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = StateClosed
	cb.failures = 0
	cb.successes = 0
	cb.halfOpenRequests = 0
	cb.lastFailureTime = time.Time{}
}

// Metrics returns current metrics for the circuit breaker.
func (cb *CircuitBreaker) Metrics() CircuitBreakerMetrics {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return CircuitBreakerMetrics{
		Name:             cb.config.Name,
		State:            cb.currentState(),
		Failures:         cb.failures,
		Successes:        cb.successes,
		LastFailureTime:  cb.lastFailureTime,
		HalfOpenRequests: cb.halfOpenRequests,
	}
}

// CircuitBreakerMetrics holds metrics for a circuit breaker.
type CircuitBreakerMetrics struct {
	Name             string
	State            CircuitState
	Failures         int
	Successes        int
	LastFailureTime  time.Time
	HalfOpenRequests int
}
