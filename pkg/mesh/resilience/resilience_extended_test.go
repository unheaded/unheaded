// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package resilience

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ==================== Rate Limit Config Tests ====================

func TestDefaultRateLimitConfig(t *testing.T) {
	config := DefaultRateLimitConfig()

	if config.RequestsPerSecond != 100 {
		t.Errorf("expected RequestsPerSecond 100, got %f", config.RequestsPerSecond)
	}
	if config.BurstSize != 10 {
		t.Errorf("expected BurstSize 10, got %d", config.BurstSize)
	}
}

// ==================== TokenBucket Tests ====================

func TestNewTokenBucket(t *testing.T) {
	t.Run("WithValidConfig", func(t *testing.T) {
		config := RateLimitConfig{
			RequestsPerSecond: 50,
			BurstSize:         5,
		}
		tb := NewTokenBucket(config)
		if tb == nil {
			t.Fatal("expected non-nil token bucket")
		}
	})

	t.Run("WithInvalidConfig_SetsDefaults", func(t *testing.T) {
		config := RateLimitConfig{
			RequestsPerSecond: 0,  // Invalid
			BurstSize:         -1, // Invalid
		}
		tb := NewTokenBucket(config)

		if tb.config.RequestsPerSecond != 100 {
			t.Errorf("expected default RequestsPerSecond 100, got %f", tb.config.RequestsPerSecond)
		}
		if tb.config.BurstSize != 10 {
			t.Errorf("expected default BurstSize 10, got %d", tb.config.BurstSize)
		}
	})
}

func TestTokenBucket_Allow(t *testing.T) {
	t.Run("AllowsWithinBurst", func(t *testing.T) {
		config := RateLimitConfig{
			RequestsPerSecond: 100,
			BurstSize:         5,
		}
		tb := NewTokenBucket(config)

		for i := 0; i < 5; i++ {
			if !tb.Allow() {
				t.Errorf("request %d should be allowed within burst", i)
			}
		}
	})

	t.Run("DeniesExceedingBurst", func(t *testing.T) {
		config := RateLimitConfig{
			RequestsPerSecond: 1, // Very slow refill
			BurstSize:         2,
		}
		tb := NewTokenBucket(config)

		// Consume all tokens
		tb.Allow()
		tb.Allow()

		// Next should be denied
		if tb.Allow() {
			t.Error("request should be denied after burst exhausted")
		}
	})
}

func TestTokenBucket_AllowN(t *testing.T) {
	t.Run("AllowsN_WithinBurst", func(t *testing.T) {
		config := RateLimitConfig{
			RequestsPerSecond: 100,
			BurstSize:         10,
		}
		tb := NewTokenBucket(config)

		if !tb.AllowN(5) {
			t.Error("should allow 5 requests within burst of 10")
		}
	})

	t.Run("DeniesN_ExceedingTokens", func(t *testing.T) {
		config := RateLimitConfig{
			RequestsPerSecond: 1,
			BurstSize:         3,
		}
		tb := NewTokenBucket(config)

		if tb.AllowN(5) {
			t.Error("should deny 5 requests with only 3 tokens")
		}
	})
}

func TestTokenBucket_Tokens(t *testing.T) {
	config := RateLimitConfig{
		RequestsPerSecond: 100,
		BurstSize:         10,
	}
	tb := NewTokenBucket(config)

	tokens := tb.Tokens()
	if tokens < 9.9 || tokens > 10.1 {
		t.Errorf("expected ~10 tokens initially, got %f", tokens)
	}
}

func TestTokenBucket_Refill(t *testing.T) {
	config := RateLimitConfig{
		RequestsPerSecond: 1000, // High rate for fast refill
		BurstSize:         5,
	}
	tb := NewTokenBucket(config)

	// Consume all tokens
	for i := 0; i < 5; i++ {
		tb.Allow()
	}

	// Wait for some refill
	time.Sleep(10 * time.Millisecond)

	// Should have some tokens refilled
	tokens := tb.Tokens()
	if tokens <= 0 {
		t.Error("expected tokens to refill after waiting")
	}
}

func TestTokenBucket_RefillCap(t *testing.T) {
	config := RateLimitConfig{
		RequestsPerSecond: 10000, // Very high refill
		BurstSize:         5,
	}
	tb := NewTokenBucket(config)

	// Wait to allow excess refill time
	time.Sleep(10 * time.Millisecond)

	// Tokens should not exceed burst size
	tokens := tb.Tokens()
	if tokens > float64(config.BurstSize)+0.1 {
		t.Errorf("tokens %f should not exceed burst size %d", tokens, config.BurstSize)
	}
}

// ==================== RateLimiter Tests ====================

func TestNewRateLimiter(t *testing.T) {
	config := DefaultRateLimitConfig()
	rl := NewRateLimiter(config)
	if rl == nil {
		t.Fatal("expected non-nil rate limiter")
	}
}

func TestRateLimiter_SetServiceLimit(t *testing.T) {
	rl := NewRateLimiter(DefaultRateLimitConfig())

	custom := RateLimitConfig{
		RequestsPerSecond: 50,
		BurstSize:         3,
	}
	rl.SetServiceLimit("api-service", custom)

	// Verify by using Allow — burst should be 3
	for i := 0; i < 3; i++ {
		if !rl.Allow("api-service") {
			t.Errorf("request %d should be allowed within custom burst", i)
		}
	}
}

func TestRateLimiter_Allow(t *testing.T) {
	t.Run("AllowsRequests", func(t *testing.T) {
		config := RateLimitConfig{
			RequestsPerSecond: 100,
			BurstSize:         5,
		}
		rl := NewRateLimiter(config)

		if !rl.Allow("test-service") {
			t.Error("first request should be allowed")
		}
	})

	t.Run("DeniesExceedingLimit", func(t *testing.T) {
		config := RateLimitConfig{
			RequestsPerSecond: 1, // Slow refill
			BurstSize:         2,
		}
		rl := NewRateLimiter(config)

		rl.Allow("test-service")
		rl.Allow("test-service")

		if rl.Allow("test-service") {
			t.Error("should deny after burst exhausted")
		}
	})

	t.Run("OnLimitReachedCallback", func(t *testing.T) {
		var callbackService string
		var callbackCalled bool

		config := RateLimitConfig{
			RequestsPerSecond: 1,
			BurstSize:         1,
			OnLimitReached: func(service string) {
				callbackCalled = true
				callbackService = service
			},
		}
		rl := NewRateLimiter(config)

		rl.Allow("my-service") // Consume the token
		rl.Allow("my-service") // Should trigger callback

		if !callbackCalled {
			t.Error("expected OnLimitReached callback to be called")
		}
		if callbackService != "my-service" {
			t.Errorf("expected service 'my-service', got '%s'", callbackService)
		}
	})

	t.Run("CreatesLimiterPerService", func(t *testing.T) {
		config := RateLimitConfig{
			RequestsPerSecond: 1,
			BurstSize:         1,
		}
		rl := NewRateLimiter(config)

		// Exhaust service-a
		rl.Allow("service-a")
		if rl.Allow("service-a") {
			t.Error("service-a should be rate limited")
		}

		// service-b should have its own bucket
		if !rl.Allow("service-b") {
			t.Error("service-b should be allowed (separate bucket)")
		}
	})
}

func TestRateLimiter_AllowN(t *testing.T) {
	t.Run("AllowsN", func(t *testing.T) {
		config := RateLimitConfig{
			RequestsPerSecond: 100,
			BurstSize:         10,
		}
		rl := NewRateLimiter(config)

		if !rl.AllowN("test-service", 5) {
			t.Error("should allow 5 requests within burst")
		}
	})

	t.Run("DeniesN_Exceeding", func(t *testing.T) {
		config := RateLimitConfig{
			RequestsPerSecond: 1,
			BurstSize:         3,
		}
		rl := NewRateLimiter(config)

		if rl.AllowN("test-service", 5) {
			t.Error("should deny 5 requests with burst of 3")
		}
	})

	t.Run("OnLimitReachedCallback_AllowN", func(t *testing.T) {
		var called bool
		config := RateLimitConfig{
			RequestsPerSecond: 1,
			BurstSize:         1,
			OnLimitReached: func(service string) {
				called = true
			},
		}
		rl := NewRateLimiter(config)

		rl.AllowN("svc", 10) // Exceeds burst, should call OnLimitReached

		if !called {
			t.Error("expected OnLimitReached callback for AllowN")
		}
	})
}

func TestRateLimiter_Wait(t *testing.T) {
	t.Run("WaitsForToken", func(t *testing.T) {
		config := RateLimitConfig{
			RequestsPerSecond: 1000, // Fast refill
			BurstSize:         1,
		}
		rl := NewRateLimiter(config)

		// Consume the token
		rl.Allow("test-service")

		ctx := context.Background()
		start := time.Now()
		err := rl.Wait(ctx, "test-service")
		elapsed := time.Since(start)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		// Should return quickly with high refill rate
		if elapsed > 100*time.Millisecond {
			t.Errorf("wait took too long: %v", elapsed)
		}
	})

	t.Run("RespectsContextCancellation", func(t *testing.T) {
		config := RateLimitConfig{
			RequestsPerSecond: 0.1, // Very slow refill
			BurstSize:         1,
		}
		rl := NewRateLimiter(config)

		// Consume the token
		rl.Allow("test-service")

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		err := rl.Wait(ctx, "test-service")
		if err == nil {
			t.Error("expected context error")
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("expected DeadlineExceeded, got %v", err)
		}
	})
}

func TestRateLimiter_Execute(t *testing.T) {
	t.Run("ExecutesWhenAllowed", func(t *testing.T) {
		config := RateLimitConfig{
			RequestsPerSecond: 100,
			BurstSize:         5,
		}
		rl := NewRateLimiter(config)

		ctx := context.Background()
		executed := false
		err := rl.Execute(ctx, "test-service", func(ctx context.Context) error {
			executed = true
			return nil
		})

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !executed {
			t.Error("function should have been executed")
		}
	})

	t.Run("ReturnsRateLimitError", func(t *testing.T) {
		config := RateLimitConfig{
			RequestsPerSecond: 1,
			BurstSize:         1,
		}
		rl := NewRateLimiter(config)

		ctx := context.Background()

		// Exhaust rate limit
		rl.Allow("test-service")

		err := rl.Execute(ctx, "test-service", func(ctx context.Context) error {
			t.Error("function should not be executed when rate limited")
			return nil
		})

		if !errors.Is(err, ErrRateLimitExceeded) {
			t.Errorf("expected ErrRateLimitExceeded, got %v", err)
		}
	})

	t.Run("PropagatesFunctionError", func(t *testing.T) {
		config := RateLimitConfig{
			RequestsPerSecond: 100,
			BurstSize:         5,
		}
		rl := NewRateLimiter(config)

		ctx := context.Background()
		expectedErr := errors.New("function error")

		err := rl.Execute(ctx, "test-service", func(ctx context.Context) error {
			return expectedErr
		})

		if err != expectedErr {
			t.Errorf("expected function error, got %v", err)
		}
	})
}

func TestRateLimiter_ExecuteWithWait(t *testing.T) {
	t.Run("ExecutesAfterWaiting", func(t *testing.T) {
		config := RateLimitConfig{
			RequestsPerSecond: 1000,
			BurstSize:         1,
		}
		rl := NewRateLimiter(config)

		// Consume the token
		rl.Allow("test-service")

		ctx := context.Background()
		executed := false
		err := rl.ExecuteWithWait(ctx, "test-service", func(ctx context.Context) error {
			executed = true
			return nil
		})

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !executed {
			t.Error("function should have been executed")
		}
	})

	t.Run("RespectsContextCancellation", func(t *testing.T) {
		config := RateLimitConfig{
			RequestsPerSecond: 0.1, // Very slow refill
			BurstSize:         1,
		}
		rl := NewRateLimiter(config)

		// Consume the token
		rl.Allow("test-service")

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		err := rl.ExecuteWithWait(ctx, "test-service", func(ctx context.Context) error {
			t.Error("function should not be executed after context timeout")
			return nil
		})

		if err == nil {
			t.Error("expected context error")
		}
	})
}

func TestRateLimiter_Metrics(t *testing.T) {
	t.Run("ExistingService", func(t *testing.T) {
		config := RateLimitConfig{
			RequestsPerSecond: 50,
			BurstSize:         5,
		}
		rl := NewRateLimiter(config)

		custom := RateLimitConfig{
			RequestsPerSecond: 200,
			BurstSize:         20,
		}
		rl.SetServiceLimit("api-service", custom)

		metrics := rl.Metrics("api-service")

		if metrics.Service != "api-service" {
			t.Errorf("expected service 'api-service', got '%s'", metrics.Service)
		}
		if metrics.RequestsPerSecond != 200 {
			t.Errorf("expected RequestsPerSecond 200, got %f", metrics.RequestsPerSecond)
		}
		if metrics.BurstSize != 20 {
			t.Errorf("expected BurstSize 20, got %d", metrics.BurstSize)
		}
		if metrics.AvailableTokens < 19.9 {
			t.Errorf("expected ~20 available tokens, got %f", metrics.AvailableTokens)
		}
	})

	t.Run("NonExistingService_UsesDefaults", func(t *testing.T) {
		config := RateLimitConfig{
			RequestsPerSecond: 50,
			BurstSize:         5,
		}
		rl := NewRateLimiter(config)

		metrics := rl.Metrics("unknown-service")

		if metrics.Service != "unknown-service" {
			t.Errorf("expected service 'unknown-service', got '%s'", metrics.Service)
		}
		if metrics.RequestsPerSecond != 50 {
			t.Errorf("expected default RequestsPerSecond 50, got %f", metrics.RequestsPerSecond)
		}
		if metrics.BurstSize != 5 {
			t.Errorf("expected default BurstSize 5, got %d", metrics.BurstSize)
		}
		if metrics.AvailableTokens != 0 {
			t.Errorf("expected 0 tokens for non-existing service, got %f", metrics.AvailableTokens)
		}
	})
}

func TestRateLimiter_GetLimiter_DoubleCheck(t *testing.T) {
	// Test the double-check locking in getLimiter by concurrent access
	config := RateLimitConfig{
		RequestsPerSecond: 100,
		BurstSize:         10,
	}
	rl := NewRateLimiter(config)

	var wg sync.WaitGroup
	var allowed int64

	// Many goroutines requesting the same service simultaneously
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if rl.Allow("concurrent-service") {
				atomic.AddInt64(&allowed, 1)
			}
		}()
	}

	wg.Wait()

	if allowed == 0 {
		t.Error("expected at least some requests to be allowed")
	}
	if allowed > 10 {
		t.Errorf("allowed %d requests with burst of 10", allowed)
	}
}

func TestRateLimiter_ConcurrentAccess(t *testing.T) {
	config := RateLimitConfig{
		RequestsPerSecond: 10000,
		BurstSize:         100,
	}
	rl := NewRateLimiter(config)

	var wg sync.WaitGroup

	// Concurrent Allow, SetServiceLimit, and Metrics
	for i := 0; i < 50; i++ {
		wg.Add(3)
		go func(idx int) {
			defer wg.Done()
			rl.Allow("service-a")
		}(i)
		go func(idx int) {
			defer wg.Done()
			rl.SetServiceLimit("service-b", RateLimitConfig{
				RequestsPerSecond: float64(idx + 1),
				BurstSize:         idx + 1,
			})
		}(i)
		go func(idx int) {
			defer wg.Done()
			rl.Metrics("service-a")
		}(i)
	}

	wg.Wait()
}

// ==================== ErrRateLimitExceeded Tests ====================

func TestErrRateLimitExceeded(t *testing.T) {
	if ErrRateLimitExceeded.Error() != "rate limit exceeded" {
		t.Errorf("unexpected error message: %s", ErrRateLimitExceeded.Error())
	}
}

// ==================== CircuitBreaker Extended Tests ====================

func TestCircuitBreaker_SetState_SameState(t *testing.T) {
	// Cover the early return in setState when state == newState
	config := CircuitBreakerConfig{
		Name:        "test",
		MaxFailures: 1,
		Timeout:     1 * time.Hour,
		OnStateChange: func(name string, from, to CircuitState) {
			t.Error("OnStateChange should not be called for same-state transition")
		},
	}
	cb := NewCircuitBreaker(config)

	// State is already Closed; calling setState(Closed) internally
	// We trigger this through Reset (which sets state to Closed when already Closed)
	cb.Reset()
	// No panic, no callback — test passes
}

func TestCircuitBreaker_HalfOpen_TooManyRequests(t *testing.T) {
	// Explicitly cover the ErrTooManyRequests branch.
	// The first request in half-open triggers setState(HalfOpen) which resets
	// halfOpenRequests to 0, so it doesn't count. With HalfOpenMaxRequests=1,
	// the second request sets halfOpenRequests to 1, and the third is rejected.
	config := CircuitBreakerConfig{
		Name:                "test",
		MaxFailures:         1,
		Timeout:             50 * time.Millisecond,
		HalfOpenMaxRequests: 1,
		SuccessThreshold:    10, // Keep in half-open for a while
	}
	cb := NewCircuitBreaker(config)

	ctx := context.Background()

	// Trip the circuit
	cb.Execute(ctx, func(ctx context.Context) error { return errors.New("fail") })

	// Wait for half-open
	time.Sleep(100 * time.Millisecond)

	// First request: triggers Open->HalfOpen transition, resets halfOpenRequests
	cb.Execute(ctx, func(ctx context.Context) error { return nil })
	// Second request: halfOpenRequests becomes 1
	cb.Execute(ctx, func(ctx context.Context) error { return nil })
	// Third request: halfOpenRequests(1) >= HalfOpenMaxRequests(1) -> rejected
	err := cb.Execute(ctx, func(ctx context.Context) error { return nil })
	if err != ErrTooManyRequests {
		t.Errorf("expected ErrTooManyRequests, got %v", err)
	}
}

// ==================== Retry Extended Tests ====================

func TestRetryer_CalculateBackoff_NegativeResult(t *testing.T) {
	// Cover the branch where jitter causes a negative result
	// Use a very high jitter to make this likely
	config := RetryConfig{
		MaxAttempts:       3,
		InitialBackoff:    1 * time.Nanosecond, // Very small base
		MaxBackoff:        10 * time.Second,
		BackoffMultiplier: 2.0,
		Jitter:            10.0, // Huge jitter to potentially go negative
	}
	r := NewRetryer(config)

	// Run multiple times to exercise both branches
	for i := 0; i < 100; i++ {
		backoff := r.calculateBackoff(1 * time.Nanosecond)
		if backoff < 0 {
			t.Errorf("backoff should never be negative, got %v", backoff)
		}
	}
}

func TestRetryer_NegativeJitter(t *testing.T) {
	// Cover the jitter <= 0 early return with negative jitter
	config := RetryConfig{
		MaxAttempts:       3,
		InitialBackoff:    100 * time.Millisecond,
		MaxBackoff:        10 * time.Second,
		BackoffMultiplier: 2.0,
		Jitter:            -0.5, // Negative jitter
	}
	r := NewRetryer(config)

	backoff := r.calculateBackoff(100 * time.Millisecond)
	if backoff != 100*time.Millisecond {
		t.Errorf("expected 100ms with negative jitter, got %v", backoff)
	}
}

// ==================== Wait MinWaitTime Tests ====================

func TestRateLimiter_Wait_MinWaitTime(t *testing.T) {
	// Cover the waitTime < time.Millisecond branch by using a very high rate
	config := RateLimitConfig{
		RequestsPerSecond: 100000, // Very high rate, waitTime would be < 1ms
		BurstSize:         1,
	}
	rl := NewRateLimiter(config)

	// Consume the token
	rl.Allow("test-service")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := rl.Wait(ctx, "test-service")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
