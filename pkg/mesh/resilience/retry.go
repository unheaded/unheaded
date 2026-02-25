// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package resilience provides resilience patterns for the service mesh.
// THE HAUBERK - Retry logic with exponential backoff.
package resilience

import (
	"context"
	"errors"
	"math/rand"
	"time"
)

// RetryConfig configures retry behavior.
type RetryConfig struct {
	// MaxAttempts is the maximum number of attempts (including initial).
	MaxAttempts int
	// InitialBackoff is the initial backoff duration.
	InitialBackoff time.Duration
	// MaxBackoff is the maximum backoff duration.
	MaxBackoff time.Duration
	// BackoffMultiplier is the multiplier for exponential backoff.
	BackoffMultiplier float64
	// Jitter adds randomness to backoff to prevent thundering herd.
	Jitter float64
	// RetryableErrors defines which errors are retryable.
	RetryableErrors []error
	// OnRetry is called before each retry attempt.
	OnRetry func(attempt int, err error, nextBackoff time.Duration)
}

// DefaultRetryConfig returns a default retry configuration.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:       3,
		InitialBackoff:    100 * time.Millisecond,
		MaxBackoff:        10 * time.Second,
		BackoffMultiplier: 2.0,
		Jitter:            0.1,
	}
}

// Retryer handles retry logic with exponential backoff.
type Retryer struct {
	config RetryConfig
	rng    *rand.Rand
}

// NewRetryer creates a new retryer with the given configuration.
func NewRetryer(config RetryConfig) *Retryer {
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = 3
	}
	if config.InitialBackoff <= 0 {
		config.InitialBackoff = 100 * time.Millisecond
	}
	if config.MaxBackoff <= 0 {
		config.MaxBackoff = 10 * time.Second
	}
	if config.BackoffMultiplier <= 0 {
		config.BackoffMultiplier = 2.0
	}

	return &Retryer{
		config: config,
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Do executes the function with retry logic.
func (r *Retryer) Do(ctx context.Context, fn func(context.Context) error) error {
	var lastErr error
	backoff := r.config.InitialBackoff

	for attempt := 1; attempt <= r.config.MaxAttempts; attempt++ {
		err := fn(ctx)
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if !r.isRetryable(err) {
			return err
		}

		// Don't sleep after the last attempt
		if attempt == r.config.MaxAttempts {
			break
		}

		// Calculate next backoff with jitter
		nextBackoff := r.calculateBackoff(backoff)

		// Call OnRetry callback if set
		if r.config.OnRetry != nil {
			r.config.OnRetry(attempt, err, nextBackoff)
		}

		// Wait for backoff or context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(nextBackoff):
		}

		// Increase backoff for next iteration
		backoff = time.Duration(float64(backoff) * r.config.BackoffMultiplier)
		if backoff > r.config.MaxBackoff {
			backoff = r.config.MaxBackoff
		}
	}

	return &RetryError{
		Attempts: r.config.MaxAttempts,
		LastErr:  lastErr,
	}
}

// isRetryable checks if an error should trigger a retry.
func (r *Retryer) isRetryable(err error) bool {
	// Context errors are not retryable
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// If no specific retryable errors defined, retry all errors
	if len(r.config.RetryableErrors) == 0 {
		return true
	}

	// Check if error matches any retryable error
	for _, retryable := range r.config.RetryableErrors {
		if errors.Is(err, retryable) {
			return true
		}
	}

	return false
}

// calculateBackoff calculates the next backoff duration with jitter.
func (r *Retryer) calculateBackoff(base time.Duration) time.Duration {
	if r.config.Jitter <= 0 {
		return base
	}

	// Add jitter: base * (1 +/- jitter)
	jitterRange := float64(base) * r.config.Jitter
	jitter := (r.rng.Float64()*2 - 1) * jitterRange

	result := time.Duration(float64(base) + jitter)
	if result < 0 {
		result = base
	}

	return result
}

// RetryError is returned when all retry attempts are exhausted.
type RetryError struct {
	Attempts int
	LastErr  error
}

func (e *RetryError) Error() string {
	return "max retry attempts reached: " + e.LastErr.Error()
}

func (e *RetryError) Unwrap() error {
	return e.LastErr
}

// RetryWithBackoff is a convenience function for simple retry cases.
func RetryWithBackoff(ctx context.Context, maxAttempts int, fn func(context.Context) error) error {
	config := DefaultRetryConfig()
	config.MaxAttempts = maxAttempts
	return NewRetryer(config).Do(ctx, fn)
}
