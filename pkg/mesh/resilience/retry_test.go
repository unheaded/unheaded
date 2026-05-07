// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package resilience

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// ==================== RetryConfig Tests ====================

func TestDefaultRetryConfig(t *testing.T) {
	config := DefaultRetryConfig()

	if config.MaxAttempts != 3 {
		t.Errorf("expected MaxAttempts 3, got %d", config.MaxAttempts)
	}
	if config.InitialBackoff != 100*time.Millisecond {
		t.Errorf("expected InitialBackoff 100ms, got %v", config.InitialBackoff)
	}
	if config.MaxBackoff != 10*time.Second {
		t.Errorf("expected MaxBackoff 10s, got %v", config.MaxBackoff)
	}
	if config.BackoffMultiplier != 2.0 {
		t.Errorf("expected BackoffMultiplier 2.0, got %f", config.BackoffMultiplier)
	}
	if config.Jitter != 0.1 {
		t.Errorf("expected Jitter 0.1, got %f", config.Jitter)
	}
}

// ==================== Retryer Tests ====================

func TestNewRetryer(t *testing.T) {
	t.Run("WithValidConfig", func(t *testing.T) {
		config := RetryConfig{
			MaxAttempts:       5,
			InitialBackoff:    50 * time.Millisecond,
			MaxBackoff:        5 * time.Second,
			BackoffMultiplier: 1.5,
			Jitter:            0.2,
		}

		r := NewRetryer(config)
		if r == nil {
			t.Fatal("expected non-nil retryer")
		}
	})

	t.Run("WithInvalidConfig_SetsDefaults", func(t *testing.T) {
		config := RetryConfig{
			MaxAttempts:       0, // Invalid
			InitialBackoff:    0, // Invalid
			MaxBackoff:        0, // Invalid
			BackoffMultiplier: 0, // Invalid
		}

		r := NewRetryer(config)

		if r.config.MaxAttempts != 3 {
			t.Errorf("expected default MaxAttempts 3, got %d", r.config.MaxAttempts)
		}
		if r.config.InitialBackoff != 100*time.Millisecond {
			t.Errorf("expected default InitialBackoff 100ms, got %v", r.config.InitialBackoff)
		}
		if r.config.MaxBackoff != 10*time.Second {
			t.Errorf("expected default MaxBackoff 10s, got %v", r.config.MaxBackoff)
		}
		if r.config.BackoffMultiplier != 2.0 {
			t.Errorf("expected default BackoffMultiplier 2.0, got %f", r.config.BackoffMultiplier)
		}
	})
}

func TestRetryer_Do_Success(t *testing.T) {
	config := DefaultRetryConfig()
	r := NewRetryer(config)

	ctx := context.Background()
	executed := false

	err := r.Do(ctx, func(ctx context.Context) error {
		executed = true
		return nil
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !executed {
		t.Error("function was not executed")
	}
}

func TestRetryer_Do_RetryOnFailure(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:       3,
		InitialBackoff:    10 * time.Millisecond,
		MaxBackoff:        100 * time.Millisecond,
		BackoffMultiplier: 2.0,
	}
	r := NewRetryer(config)

	ctx := context.Background()
	attempts := 0

	err := r.Do(ctx, func(ctx context.Context) error {
		attempts++
		if attempts < 3 {
			return errors.New("temporary failure")
		}
		return nil
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetryer_Do_MaxRetriesExceeded(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:       3,
		InitialBackoff:    10 * time.Millisecond,
		MaxBackoff:        100 * time.Millisecond,
		BackoffMultiplier: 2.0,
	}
	r := NewRetryer(config)

	ctx := context.Background()
	attempts := 0
	testErr := errors.New("persistent failure")

	err := r.Do(ctx, func(ctx context.Context) error {
		attempts++
		return testErr
	})

	if err == nil {
		t.Error("expected error after max retries")
	}

	var retryErr *RetryError
	if !errors.As(err, &retryErr) {
		t.Errorf("expected RetryError, got %T", err)
	}
	if retryErr.Attempts != 3 {
		t.Errorf("expected 3 attempts in error, got %d", retryErr.Attempts)
	}
	if !errors.Is(retryErr.LastErr, testErr) {
		t.Errorf("expected last error to be testErr")
	}

	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetryer_Do_NonRetryableError(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:     5,
		InitialBackoff:  10 * time.Millisecond,
		RetryableErrors: []error{errors.New("retryable")}, // Only this error is retryable
	}
	r := NewRetryer(config)

	ctx := context.Background()
	attempts := 0
	nonRetryableErr := errors.New("non-retryable")

	err := r.Do(ctx, func(ctx context.Context) error {
		attempts++
		return nonRetryableErr
	})

	// Should return immediately without retrying
	if attempts != 1 {
		t.Errorf("expected 1 attempt for non-retryable error, got %d", attempts)
	}
	if err != nonRetryableErr {
		t.Errorf("expected nonRetryableErr, got %v", err)
	}
}

func TestRetryer_Do_ContextCanceled(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:    10,
		InitialBackoff: 100 * time.Millisecond, // Long backoff
	}
	r := NewRetryer(config)

	ctx, cancel := context.WithCancel(context.Background())
	attempts := int32(0)

	// Cancel after first attempt
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := r.Do(ctx, func(ctx context.Context) error {
		atomic.AddInt32(&attempts, 1)
		return errors.New("fail")
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestRetryer_Do_ContextDeadlineExceeded(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:    10,
		InitialBackoff: 100 * time.Millisecond,
	}
	r := NewRetryer(config)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := r.Do(ctx, func(ctx context.Context) error {
		return errors.New("fail")
	})

	// Context errors are not retryable
	if err == nil {
		t.Error("expected error due to timeout")
	}
}

func TestRetryer_Do_OnRetryCallback(t *testing.T) {
	var callbackCalls []int
	var lastErrors []error
	var backoffs []time.Duration

	config := RetryConfig{
		MaxAttempts:       4,
		InitialBackoff:    10 * time.Millisecond,
		MaxBackoff:        100 * time.Millisecond,
		BackoffMultiplier: 2.0,
		OnRetry: func(attempt int, err error, nextBackoff time.Duration) {
			callbackCalls = append(callbackCalls, attempt)
			lastErrors = append(lastErrors, err)
			backoffs = append(backoffs, nextBackoff)
		},
	}
	r := NewRetryer(config)

	ctx := context.Background()
	testErr := errors.New("fail")

	r.Do(ctx, func(ctx context.Context) error {
		return testErr
	})

	// OnRetry should be called for each retry (not the final attempt)
	if len(callbackCalls) != 3 {
		t.Errorf("expected 3 callback calls, got %d", len(callbackCalls))
	}

	// Check attempt numbers
	expected := []int{1, 2, 3}
	for i, call := range callbackCalls {
		if call != expected[i] {
			t.Errorf("callback %d: expected attempt %d, got %d", i, expected[i], call)
		}
	}
}

func TestRetryer_Do_ExponentialBackoff(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:       4,
		InitialBackoff:    10 * time.Millisecond,
		MaxBackoff:        1 * time.Second,
		BackoffMultiplier: 2.0,
		Jitter:            0, // No jitter for predictable timing
	}
	r := NewRetryer(config)

	ctx := context.Background()
	var timestamps []time.Time

	start := time.Now()
	r.Do(ctx, func(ctx context.Context) error {
		timestamps = append(timestamps, time.Now())
		return errors.New("fail")
	})

	// Check that delays roughly match exponential backoff
	// Expected: 0ms, 10ms, 20ms, 40ms
	if len(timestamps) != 4 {
		t.Fatalf("expected 4 timestamps, got %d", len(timestamps))
	}

	// First attempt should be immediate
	if timestamps[0].Sub(start) > 5*time.Millisecond {
		t.Error("first attempt should be immediate")
	}

	// Check delays are increasing
	for i := 1; i < len(timestamps); i++ {
		delay := timestamps[i].Sub(timestamps[i-1])
		// Allow some tolerance
		if delay < 5*time.Millisecond {
			t.Errorf("delay %d too short: %v", i, delay)
		}
	}
}

func TestRetryer_Do_MaxBackoffCap(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:       5,
		InitialBackoff:    50 * time.Millisecond,
		MaxBackoff:        60 * time.Millisecond, // Low cap
		BackoffMultiplier: 10.0,                  // Aggressive multiplier
		Jitter:            0,
	}
	r := NewRetryer(config)

	ctx := context.Background()
	var timestamps []time.Time

	r.Do(ctx, func(ctx context.Context) error {
		timestamps = append(timestamps, time.Now())
		return errors.New("fail")
	})

	// Later delays should not exceed MaxBackoff by much
	for i := 2; i < len(timestamps); i++ {
		delay := timestamps[i].Sub(timestamps[i-1])
		if delay > 80*time.Millisecond { // Allow some tolerance
			t.Errorf("delay %d exceeds max backoff: %v", i, delay)
		}
	}
}

func TestRetryer_isRetryable(t *testing.T) {
	t.Run("NoRetryableErrors_AllRetryable", func(t *testing.T) {
		config := RetryConfig{MaxAttempts: 3}
		r := NewRetryer(config)

		err := errors.New("any error")
		if !r.isRetryable(err) {
			t.Error("expected error to be retryable when no specific retryable errors defined")
		}
	})

	t.Run("ContextErrors_NotRetryable", func(t *testing.T) {
		config := RetryConfig{MaxAttempts: 3}
		r := NewRetryer(config)

		if r.isRetryable(context.Canceled) {
			t.Error("context.Canceled should not be retryable")
		}
		if r.isRetryable(context.DeadlineExceeded) {
			t.Error("context.DeadlineExceeded should not be retryable")
		}
	})

	t.Run("SpecificRetryableErrors", func(t *testing.T) {
		retryableErr := errors.New("retryable")
		nonRetryableErr := errors.New("non-retryable")

		config := RetryConfig{
			MaxAttempts:     3,
			RetryableErrors: []error{retryableErr},
		}
		r := NewRetryer(config)

		if !r.isRetryable(retryableErr) {
			t.Error("expected retryableErr to be retryable")
		}
		if r.isRetryable(nonRetryableErr) {
			t.Error("expected nonRetryableErr to not be retryable")
		}
	})
}

func TestRetryer_calculateBackoff(t *testing.T) {
	t.Run("NoJitter", func(t *testing.T) {
		config := RetryConfig{
			InitialBackoff: 100 * time.Millisecond,
			Jitter:         0,
		}
		r := NewRetryer(config)

		backoff := r.calculateBackoff(100 * time.Millisecond)
		if backoff != 100*time.Millisecond {
			t.Errorf("expected 100ms without jitter, got %v", backoff)
		}
	})

	t.Run("WithJitter", func(t *testing.T) {
		config := RetryConfig{
			InitialBackoff: 100 * time.Millisecond,
			Jitter:         0.1,
		}
		r := NewRetryer(config)

		// Run multiple times to test jitter variability
		var hasVariation bool
		lastBackoff := time.Duration(0)
		for i := 0; i < 10; i++ {
			backoff := r.calculateBackoff(100 * time.Millisecond)
			if lastBackoff != 0 && backoff != lastBackoff {
				hasVariation = true
			}
			lastBackoff = backoff

			// Should be within 10% jitter range
			if backoff < 90*time.Millisecond || backoff > 110*time.Millisecond {
				t.Errorf("backoff %v outside expected jitter range", backoff)
			}
		}

		if !hasVariation {
			t.Log("warning: no variation detected in jitter (may be OK for random seeds)")
		}
	})
}

// ==================== RetryError Tests ====================

func TestRetryError(t *testing.T) {
	t.Run("Error", func(t *testing.T) {
		lastErr := errors.New("final failure")
		err := &RetryError{
			Attempts: 3,
			LastErr:  lastErr,
		}

		expected := "max retry attempts reached: final failure"
		if err.Error() != expected {
			t.Errorf("expected error message '%s', got '%s'", expected, err.Error())
		}
	})

	t.Run("Unwrap", func(t *testing.T) {
		lastErr := errors.New("final failure")
		err := &RetryError{
			Attempts: 3,
			LastErr:  lastErr,
		}

		unwrapped := err.Unwrap()
		if unwrapped != lastErr {
			t.Errorf("expected unwrapped error to be lastErr")
		}

		if !errors.Is(err, lastErr) {
			t.Error("errors.Is should match lastErr")
		}
	})
}

// ==================== RetryWithBackoff Tests ====================

func TestRetryWithBackoff(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		ctx := context.Background()
		attempts := 0

		err := RetryWithBackoff(ctx, 3, func(ctx context.Context) error {
			attempts++
			if attempts < 2 {
				return errors.New("fail")
			}
			return nil
		})

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if attempts != 2 {
			t.Errorf("expected 2 attempts, got %d", attempts)
		}
	})

	t.Run("MaxRetriesExceeded", func(t *testing.T) {
		ctx := context.Background()
		attempts := 0

		err := RetryWithBackoff(ctx, 3, func(ctx context.Context) error {
			attempts++
			return errors.New("persistent failure")
		})

		if err == nil {
			t.Error("expected error after max retries")
		}
		if attempts != 3 {
			t.Errorf("expected 3 attempts, got %d", attempts)
		}
	})
}

// ==================== Benchmark Tests ====================

func BenchmarkRetryer_Do_Success(b *testing.B) {
	config := DefaultRetryConfig()
	r := NewRetryer(config)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Do(ctx, func(ctx context.Context) error {
			return nil
		})
	}
}

func BenchmarkRetryer_Do_WithRetries(b *testing.B) {
	config := RetryConfig{
		MaxAttempts:       3,
		InitialBackoff:    1 * time.Microsecond,
		MaxBackoff:        10 * time.Microsecond,
		BackoffMultiplier: 2.0,
	}
	r := NewRetryer(config)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		attempts := 0
		r.Do(ctx, func(ctx context.Context) error {
			attempts++
			if attempts < 3 {
				return errors.New("fail")
			}
			return nil
		})
	}
}
