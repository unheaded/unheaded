// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package circuitbreaker

import (
	"errors"
	"time"

	"github.com/sony/gobreaker"
)

// CircuitBreaker wraps sony/gobreaker for service resilience
type CircuitBreaker struct {
	breaker *gobreaker.CircuitBreaker
}

// Config holds circuit breaker configuration
type Config struct {
	Name         string
	MaxRequests  uint32        // Max requests allowed in half-open state
	Interval     time.Duration // Interval to reset failure counter
	Timeout      time.Duration // Timeout in open state before half-open
	FailureRatio float64       // Failure ratio threshold (0.0-1.0)
	MinRequests  uint32        // Min requests before evaluating
}

var (
	ErrCircuitOpen     = errors.New("circuit breaker is open")
	ErrTooManyRequests = errors.New("too many requests in half-open state")
)

// New creates a new circuit breaker
func New(cfg Config) *CircuitBreaker {
	settings := gobreaker.Settings{
		Name:        cfg.Name,
		MaxRequests: cfg.MaxRequests,
		Interval:    cfg.Interval,
		Timeout:     cfg.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			if counts.Requests < cfg.MinRequests {
				return false
			}
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return failureRatio >= cfg.FailureRatio
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			// Log state changes
			// logger.Info(context.Background(), fmt.Sprintf("circuit breaker %s changed from %s to %s", name, from, to))
		},
	}

	return &CircuitBreaker{
		breaker: gobreaker.NewCircuitBreaker(settings),
	}
}

// Execute executes a function with circuit breaker protection
func (cb *CircuitBreaker) Execute(fn func() (interface{}, error)) (interface{}, error) {
	return cb.breaker.Execute(fn)
}

// Call executes a function with circuit breaker protection (no return value)
func (cb *CircuitBreaker) Call(fn func() error) error {
	_, err := cb.breaker.Execute(func() (interface{}, error) {
		return nil, fn()
	})
	return err
}

// State returns the current circuit breaker state
func (cb *CircuitBreaker) State() gobreaker.State {
	return cb.breaker.State()
}

// Counts returns the current counts
func (cb *CircuitBreaker) Counts() gobreaker.Counts {
	return cb.breaker.Counts()
}

// DefaultConfig returns sensible defaults for circuit breaker
func DefaultConfig(name string) Config {
	return Config{
		Name:         name,
		MaxRequests:  3,                // Allow 3 requests in half-open
		Interval:     30 * time.Second, // Reset failure counter every 30s
		Timeout:      60 * time.Second, // Try half-open after 60s
		FailureRatio: 0.6,              // Trip at 60% failure rate
		MinRequests:  10,               // Need at least 10 requests to evaluate
	}
}
