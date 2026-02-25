// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package worker

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"time"
)

var (
	// ErrMaxRetriesExceeded is returned when all retry attempts are exhausted.
	ErrMaxRetriesExceeded = errors.New("maximum retries exceeded")
)

// BackoffStrategy defines how to calculate delay between retries.
type BackoffStrategy int

const (
	// BackoffConstant uses a fixed delay between retries.
	BackoffConstant BackoffStrategy = iota
	// BackoffLinear increases delay linearly.
	BackoffLinear
	// BackoffExponential doubles delay each retry.
	BackoffExponential
	// BackoffExponentialJitter adds randomness to exponential backoff.
	BackoffExponentialJitter
)

// RetryConfig holds retry configuration.
type RetryConfig struct {
	// MaxRetries is the maximum number of retry attempts.
	MaxRetries int
	// InitialDelay is the first retry delay.
	InitialDelay time.Duration
	// MaxDelay is the maximum delay between retries.
	MaxDelay time.Duration
	// Strategy is the backoff strategy to use.
	Strategy BackoffStrategy
	// Multiplier is used for linear and exponential backoff.
	Multiplier float64
	// JitterFactor is the maximum jitter as a fraction (0-1).
	JitterFactor float64
	// RetryableErrors is a list of errors that should trigger a retry.
	// If nil, all errors are retryable.
	RetryableErrors []error
	// ShouldRetry is a custom function to determine if an error is retryable.
	ShouldRetry func(error) bool
}

// DefaultRetryConfig returns a sensible default retry configuration.
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxRetries:   3,
		InitialDelay: time.Second,
		MaxDelay:     time.Minute,
		Strategy:     BackoffExponential,
		Multiplier:   2.0,
		JitterFactor: 0.1,
	}
}

// Retryer handles retry logic for jobs.
type Retryer struct {
	config *RetryConfig
	rng    *rand.Rand
}

// NewRetryer creates a new retryer with the given configuration.
func NewRetryer(config *RetryConfig) *Retryer {
	if config == nil {
		config = DefaultRetryConfig()
	}
	return &Retryer{
		config: config,
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// CalculateDelay returns the delay for the given attempt number.
func (r *Retryer) CalculateDelay(attempt int) time.Duration {
	if attempt <= 0 {
		attempt = 1
	}

	var delay time.Duration

	switch r.config.Strategy {
	case BackoffConstant:
		delay = r.config.InitialDelay

	case BackoffLinear:
		delay = time.Duration(float64(r.config.InitialDelay) * r.config.Multiplier * float64(attempt))

	case BackoffExponential:
		delay = time.Duration(float64(r.config.InitialDelay) * math.Pow(r.config.Multiplier, float64(attempt-1)))

	case BackoffExponentialJitter:
		baseDelay := float64(r.config.InitialDelay) * math.Pow(r.config.Multiplier, float64(attempt-1))
		jitter := baseDelay * r.config.JitterFactor * (r.rng.Float64()*2 - 1) // [-jitter, +jitter]
		delay = time.Duration(baseDelay + jitter)

	default:
		delay = r.config.InitialDelay
	}

	// Cap at max delay
	if r.config.MaxDelay > 0 && delay > r.config.MaxDelay {
		delay = r.config.MaxDelay
	}

	return delay
}

// IsRetryable determines if an error should trigger a retry.
func (r *Retryer) IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Use custom function if provided
	if r.config.ShouldRetry != nil {
		return r.config.ShouldRetry(err)
	}

	// If no specific errors are defined, all errors are retryable
	if len(r.config.RetryableErrors) == 0 {
		return true
	}

	// Check if error matches any retryable error
	for _, retryableErr := range r.config.RetryableErrors {
		if errors.Is(err, retryableErr) {
			return true
		}
	}

	return false
}

// ShouldRetry returns true if another attempt should be made.
func (r *Retryer) ShouldRetry(attempt int, err error) bool {
	if attempt >= r.config.MaxRetries {
		return false
	}
	return r.IsRetryable(err)
}

// Execute runs a function with retry logic.
func (r *Retryer) Execute(ctx context.Context, fn func(context.Context) (interface{}, error)) (interface{}, error) {
	var lastErr error

	for attempt := 0; attempt <= r.config.MaxRetries; attempt++ {
		// Check context before attempting
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		// Execute the function
		result, err := fn(ctx)
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Check if we should retry
		if !r.ShouldRetry(attempt+1, err) {
			break
		}

		// Calculate and wait for delay
		delay := r.CalculateDelay(attempt + 1)
		select {
		case <-time.After(delay):
			// Continue to next attempt
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return nil, lastErr
}

// RetryExecutor wraps job execution with retry capabilities.
type RetryExecutor struct {
	retryer *Retryer
}

// NewRetryExecutor creates a new retry executor.
func NewRetryExecutor(config *RetryConfig) *RetryExecutor {
	return &RetryExecutor{
		retryer: NewRetryer(config),
	}
}

// ExecuteJob executes a job with retry logic.
func (re *RetryExecutor) ExecuteJob(ctx context.Context, job *Job) *Result {
	startTime := time.Now()
	var result interface{}
	var err error
	attempt := 0

	// Use job-specific retry settings if available
	maxRetries := re.retryer.config.MaxRetries
	if job.MaxRetries > 0 {
		maxRetries = job.MaxRetries
	}

	initialDelay := re.retryer.config.InitialDelay
	if job.RetryDelay > 0 {
		initialDelay = job.RetryDelay
	}

	for attempt = 0; attempt <= maxRetries; attempt++ {
		job.IncrementAttempts()

		// Check context before attempting
		if ctx.Err() != nil {
			return &Result{
				JobID:       job.ID,
				Error:       ctx.Err(),
				Status:      JobStatusCanceled,
				Duration:    time.Since(startTime),
				Attempts:    attempt + 1,
				CompletedAt: time.Now(),
			}
		}

		// Create job context with timeout if specified
		jobCtx := ctx
		var cancel context.CancelFunc
		if job.Timeout > 0 {
			jobCtx, cancel = context.WithTimeout(ctx, job.Timeout)
		}

		// Execute the job
		job.SetStatus(JobStatusRunning)
		result, err = job.Func(jobCtx)

		if cancel != nil {
			cancel()
		}

		if err == nil {
			job.SetStatus(JobStatusCompleted)
			return &Result{
				JobID:       job.ID,
				Value:       result,
				Status:      JobStatusCompleted,
				Duration:    time.Since(startTime),
				Attempts:    attempt + 1,
				CompletedAt: time.Now(),
			}
		}

		job.SetLastError(err)

		// Check for timeout
		if errors.Is(err, context.DeadlineExceeded) && job.Timeout > 0 {
			job.SetStatus(JobStatusTimeout)
			return &Result{
				JobID:       job.ID,
				Error:       err,
				Status:      JobStatusTimeout,
				Duration:    time.Since(startTime),
				Attempts:    attempt + 1,
				CompletedAt: time.Now(),
			}
		}

		// Check if we should retry
		if attempt >= maxRetries || !re.retryer.IsRetryable(err) {
			break
		}

		// Calculate delay with exponential backoff
		delay := time.Duration(float64(initialDelay) * math.Pow(2, float64(attempt)))
		if re.retryer.config.MaxDelay > 0 && delay > re.retryer.config.MaxDelay {
			delay = re.retryer.config.MaxDelay
		}

		// Wait before retry
		select {
		case <-time.After(delay):
			// Continue to next attempt
		case <-ctx.Done():
			return &Result{
				JobID:       job.ID,
				Error:       ctx.Err(),
				Status:      JobStatusCanceled,
				Duration:    time.Since(startTime),
				Attempts:    attempt + 1,
				CompletedAt: time.Now(),
			}
		}
	}

	job.SetStatus(JobStatusFailed)
	return &Result{
		JobID:       job.ID,
		Error:       err,
		Status:      JobStatusFailed,
		Duration:    time.Since(startTime),
		Attempts:    attempt + 1,
		CompletedAt: time.Now(),
	}
}
