// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package policy

import (
	"context"
	"errors"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

var (
	// ErrMaxRetriesExceeded indicates all retry attempts failed.
	ErrMaxRetriesExceeded = errors.New("max retries exceeded")
	// ErrNonRetryable indicates the error is not retryable.
	ErrNonRetryable = errors.New("non-retryable error")
)

// RetryCondition determines if an error should be retried.
type RetryCondition func(err error, resp *http.Response, attempt int) bool

// Retryer executes operations with retry logic.
type Retryer struct {
	policy *RetryPolicy
	rng    *rand.Rand
	mu     sync.Mutex
}

// NewRetryer creates a new retryer with the given policy.
func NewRetryer(policy *RetryPolicy) *Retryer {
	return &Retryer{
		policy: policy,
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Do executes the operation with retry logic.
func (r *Retryer) Do(ctx context.Context, op func(ctx context.Context) error) error {
	var lastErr error

	for attempt := 0; attempt <= r.policy.MaxRetries; attempt++ {
		if attempt > 0 {
			// Calculate backoff
			backoff := r.calculateBackoff(attempt)

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		err := op(ctx)
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if !r.isRetryable(err) {
			return err
		}
	}

	return errors.Join(ErrMaxRetriesExceeded, lastErr)
}

// DoWithResponse executes the operation and checks response for retry.
func (r *Retryer) DoWithResponse(ctx context.Context, op func(ctx context.Context) (*http.Response, error)) (*http.Response, error) {
	var lastErr error
	var lastResp *http.Response

	for attempt := 0; attempt <= r.policy.MaxRetries; attempt++ {
		if attempt > 0 {
			backoff := r.calculateBackoff(attempt)

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		resp, err := op(ctx)
		if err == nil && !r.shouldRetryResponse(resp) {
			return resp, nil
		}

		if resp != nil {
			lastResp = resp
		}
		if err != nil {
			lastErr = err
		}

		// Check if should retry
		if err != nil && !r.isRetryable(err) {
			return resp, err
		}
	}

	if lastErr != nil {
		return lastResp, errors.Join(ErrMaxRetriesExceeded, lastErr)
	}
	return lastResp, ErrMaxRetriesExceeded
}

func (r *Retryer) calculateBackoff(attempt int) time.Duration {
	// Exponential backoff: base * 2^attempt
	backoff := r.policy.BackoffBase * time.Duration(1<<uint(attempt-1))

	// Cap at max
	if backoff > r.policy.BackoffMax {
		backoff = r.policy.BackoffMax
	}

	// Add jitter
	if r.policy.BackoffJitter > 0 {
		r.mu.Lock()
		jitter := time.Duration(float64(backoff) * r.policy.BackoffJitter * (r.rng.Float64()*2 - 1))
		r.mu.Unlock()
		backoff += jitter
		if backoff < 0 {
			backoff = r.policy.BackoffBase
		}
	}

	return backoff
}

func (r *Retryer) isRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Check configured retry conditions
	for _, condition := range r.policy.RetryOn {
		switch condition {
		case "connection-failure":
			if isConnectionError(err) {
				return true
			}
		case "reset":
			if isResetError(err) {
				return true
			}
		}
	}

	return false
}

func (r *Retryer) shouldRetryResponse(resp *http.Response) bool {
	if resp == nil {
		return true
	}

	for _, condition := range r.policy.RetryOn {
		switch condition {
		case "5xx":
			if resp.StatusCode >= 500 && resp.StatusCode < 600 {
				return true
			}
		case "retriable-4xx":
			if resp.StatusCode == 408 || resp.StatusCode == 429 {
				return true
			}
		}
	}

	// Check specific status codes
	for _, code := range r.policy.RetryableStatus {
		if resp.StatusCode == code {
			return true
		}
	}

	return false
}

// isConnectionError checks if the error is a connection failure.
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return containsAny(errStr, []string{
		"connection refused",
		"connection reset",
		"no such host",
		"network is unreachable",
		"i/o timeout",
		"connection timed out",
		"EOF",
	})
}

// isResetError checks if the error is a connection reset.
func isResetError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return containsAny(errStr, []string{
		"connection reset",
		"broken pipe",
	})
}

func containsAny(s string, substrs []string) bool {
	for _, sub := range substrs {
		if containsString(s, sub) {
			return true
		}
	}
	return false
}

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// BackoffCalculator provides backoff duration calculation.
type BackoffCalculator struct {
	Base   time.Duration
	Max    time.Duration
	Jitter float64
	rng    *rand.Rand
	mu     sync.Mutex
}

// NewBackoffCalculator creates a new backoff calculator.
func NewBackoffCalculator(base, max time.Duration, jitter float64) *BackoffCalculator {
	return &BackoffCalculator{
		Base:   base,
		Max:    max,
		Jitter: jitter,
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Next returns the next backoff duration.
func (b *BackoffCalculator) Next(attempt int) time.Duration {
	backoff := b.Base * time.Duration(1<<uint(attempt))
	if backoff > b.Max {
		backoff = b.Max
	}

	if b.Jitter > 0 {
		b.mu.Lock()
		jitterRange := float64(backoff) * b.Jitter
		jitter := time.Duration(jitterRange * (b.rng.Float64()*2 - 1))
		b.mu.Unlock()
		backoff += jitter
	}

	if backoff < 0 {
		backoff = b.Base
	}

	return backoff
}
