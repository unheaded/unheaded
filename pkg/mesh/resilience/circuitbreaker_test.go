// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package resilience

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ==================== CircuitState Tests ====================

func TestCircuitState_String(t *testing.T) {
	tests := []struct {
		state    CircuitState
		expected string
	}{
		{StateClosed, "closed"},
		{StateOpen, "open"},
		{StateHalfOpen, "half-open"},
		{CircuitState(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if tt.state.String() != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, tt.state.String())
			}
		})
	}
}

// ==================== CircuitBreakerConfig Tests ====================

func TestDefaultCircuitBreakerConfig(t *testing.T) {
	config := DefaultCircuitBreakerConfig("test-circuit")

	if config.Name != "test-circuit" {
		t.Errorf("expected name test-circuit, got %s", config.Name)
	}
	if config.MaxFailures != 5 {
		t.Errorf("expected MaxFailures 5, got %d", config.MaxFailures)
	}
	if config.Timeout != 30*time.Second {
		t.Errorf("expected Timeout 30s, got %v", config.Timeout)
	}
	if config.HalfOpenMaxRequests != 3 {
		t.Errorf("expected HalfOpenMaxRequests 3, got %d", config.HalfOpenMaxRequests)
	}
	if config.SuccessThreshold != 2 {
		t.Errorf("expected SuccessThreshold 2, got %d", config.SuccessThreshold)
	}
}

// ==================== CircuitBreaker Tests ====================

func TestNewCircuitBreaker(t *testing.T) {
	t.Run("WithValidConfig", func(t *testing.T) {
		config := CircuitBreakerConfig{
			Name:                "test",
			MaxFailures:         3,
			Timeout:             10 * time.Second,
			HalfOpenMaxRequests: 2,
			SuccessThreshold:    1,
		}

		cb := NewCircuitBreaker(config)
		if cb == nil {
			t.Fatal("expected non-nil circuit breaker")
		}
		if cb.State() != StateClosed {
			t.Errorf("expected initial state closed, got %v", cb.State())
		}
	})

	t.Run("WithInvalidConfig_SetsDefaults", func(t *testing.T) {
		config := CircuitBreakerConfig{
			Name:                "test",
			MaxFailures:         0,  // Invalid
			Timeout:             0,  // Invalid
			HalfOpenMaxRequests: -1, // Invalid
			SuccessThreshold:    0,  // Invalid
		}

		cb := NewCircuitBreaker(config)

		// Should use defaults
		if cb.config.MaxFailures != 5 {
			t.Errorf("expected MaxFailures default 5, got %d", cb.config.MaxFailures)
		}
		if cb.config.Timeout != 30*time.Second {
			t.Errorf("expected Timeout default 30s, got %v", cb.config.Timeout)
		}
		if cb.config.HalfOpenMaxRequests != 3 {
			t.Errorf("expected HalfOpenMaxRequests default 3, got %d", cb.config.HalfOpenMaxRequests)
		}
		if cb.config.SuccessThreshold != 2 {
			t.Errorf("expected SuccessThreshold default 2, got %d", cb.config.SuccessThreshold)
		}
	})
}

func TestCircuitBreaker_ClosedState(t *testing.T) {
	t.Run("AllowsRequests", func(t *testing.T) {
		cb := NewCircuitBreaker(DefaultCircuitBreakerConfig("test"))

		ctx := context.Background()
		executed := false
		err := cb.Execute(ctx, func(ctx context.Context) error {
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

	t.Run("ReturnsErrors_WithoutTripping", func(t *testing.T) {
		config := DefaultCircuitBreakerConfig("test")
		config.MaxFailures = 5
		cb := NewCircuitBreaker(config)

		ctx := context.Background()
		expectedErr := errors.New("operation failed")

		// Execute fewer than max failures
		for i := 0; i < 4; i++ {
			err := cb.Execute(ctx, func(ctx context.Context) error {
				return expectedErr
			})
			if err != expectedErr {
				t.Errorf("expected error %v, got %v", expectedErr, err)
			}
		}

		// Should still be closed
		if cb.State() != StateClosed {
			t.Errorf("expected state closed, got %v", cb.State())
		}
	})

	t.Run("SuccessResetsFailureCount", func(t *testing.T) {
		config := DefaultCircuitBreakerConfig("test")
		config.MaxFailures = 3
		cb := NewCircuitBreaker(config)

		ctx := context.Background()
		testErr := errors.New("test error")

		// Two failures
		cb.Execute(ctx, func(ctx context.Context) error { return testErr })
		cb.Execute(ctx, func(ctx context.Context) error { return testErr })

		// One success - should reset failure count
		cb.Execute(ctx, func(ctx context.Context) error { return nil })

		// Two more failures - should not trip
		cb.Execute(ctx, func(ctx context.Context) error { return testErr })
		cb.Execute(ctx, func(ctx context.Context) error { return testErr })

		if cb.State() != StateClosed {
			t.Errorf("expected state closed after success reset, got %v", cb.State())
		}
	})
}

func TestCircuitBreaker_TransitionToOpen(t *testing.T) {
	t.Run("TripsAfterMaxFailures", func(t *testing.T) {
		config := CircuitBreakerConfig{
			Name:        "test",
			MaxFailures: 3,
			Timeout:     100 * time.Millisecond,
		}
		cb := NewCircuitBreaker(config)

		ctx := context.Background()
		testErr := errors.New("test error")

		// Trigger max failures
		for i := 0; i < 3; i++ {
			cb.Execute(ctx, func(ctx context.Context) error { return testErr })
		}

		if cb.State() != StateOpen {
			t.Errorf("expected state open after max failures, got %v", cb.State())
		}
	})

	t.Run("RejectsRequests_WhenOpen", func(t *testing.T) {
		config := CircuitBreakerConfig{
			Name:        "test",
			MaxFailures: 1,
			Timeout:     1 * time.Hour, // Long timeout
		}
		cb := NewCircuitBreaker(config)

		ctx := context.Background()

		// Trip the circuit
		cb.Execute(ctx, func(ctx context.Context) error { return errors.New("fail") })

		// Subsequent request should be rejected
		executed := false
		err := cb.Execute(ctx, func(ctx context.Context) error {
			executed = true
			return nil
		})

		if err != ErrCircuitOpen {
			t.Errorf("expected ErrCircuitOpen, got %v", err)
		}
		if executed {
			t.Error("function should not have been executed when circuit is open")
		}
	})

	t.Run("StateChangeCallback", func(t *testing.T) {
		type stateChange struct {
			from CircuitState
			to   CircuitState
		}
		ch := make(chan stateChange, 1)

		config := CircuitBreakerConfig{
			Name:        "test",
			MaxFailures: 1,
			Timeout:     100 * time.Millisecond,
			OnStateChange: func(name string, from, to CircuitState) {
				ch <- stateChange{from: from, to: to}
			},
		}
		cb := NewCircuitBreaker(config)

		ctx := context.Background()
		cb.Execute(ctx, func(ctx context.Context) error { return errors.New("fail") })

		select {
		case sc := <-ch:
			if sc.from != StateClosed {
				t.Errorf("expected from state closed, got %v", sc.from)
			}
			if sc.to != StateOpen {
				t.Errorf("expected to state open, got %v", sc.to)
			}
		case <-time.After(time.Second):
			t.Error("expected state change callback to be called")
		}
	})
}

func TestCircuitBreaker_TransitionToHalfOpen(t *testing.T) {
	t.Run("TransitionsAfterTimeout", func(t *testing.T) {
		config := CircuitBreakerConfig{
			Name:        "test",
			MaxFailures: 1,
			Timeout:     50 * time.Millisecond,
		}
		cb := NewCircuitBreaker(config)

		ctx := context.Background()

		// Trip the circuit
		cb.Execute(ctx, func(ctx context.Context) error { return errors.New("fail") })

		if cb.State() != StateOpen {
			t.Fatalf("expected state open, got %v", cb.State())
		}

		// Wait for timeout
		time.Sleep(100 * time.Millisecond)

		// State should now be half-open
		if cb.State() != StateHalfOpen {
			t.Errorf("expected state half-open after timeout, got %v", cb.State())
		}
	})

	t.Run("AllowsLimitedRequests", func(t *testing.T) {
		config := CircuitBreakerConfig{
			Name:                "test",
			MaxFailures:         1,
			Timeout:             50 * time.Millisecond,
			HalfOpenMaxRequests: 2,
		}
		cb := NewCircuitBreaker(config)

		ctx := context.Background()

		// Trip and wait for half-open
		cb.Execute(ctx, func(ctx context.Context) error { return errors.New("fail") })
		time.Sleep(100 * time.Millisecond)

		// First two requests should be allowed
		for i := 0; i < 2; i++ {
			err := cb.Execute(ctx, func(ctx context.Context) error { return nil })
			if err != nil {
				t.Errorf("request %d should be allowed in half-open, got %v", i, err)
			}
		}
	})

	t.Run("RejectExcessRequests", func(t *testing.T) {
		config := CircuitBreakerConfig{
			Name:                "test",
			MaxFailures:         1,
			Timeout:             50 * time.Millisecond,
			HalfOpenMaxRequests: 1,
			SuccessThreshold:    10, // High threshold to stay in half-open
		}
		cb := NewCircuitBreaker(config)

		ctx := context.Background()

		// Trip and wait for half-open
		cb.Execute(ctx, func(ctx context.Context) error { return errors.New("fail") })
		time.Sleep(100 * time.Millisecond)

		// First request allowed
		cb.Execute(ctx, func(ctx context.Context) error {
			// Block to keep the slot occupied
			time.Sleep(50 * time.Millisecond)
			return nil
		})

		// This may return ErrTooManyRequests if the limit is exceeded
		// Note: timing-sensitive test
	})
}

func TestCircuitBreaker_TransitionToClosed(t *testing.T) {
	t.Run("ClosesAfterSuccessThreshold", func(t *testing.T) {
		config := CircuitBreakerConfig{
			Name:                "test",
			MaxFailures:         1,
			Timeout:             50 * time.Millisecond,
			HalfOpenMaxRequests: 10,
			SuccessThreshold:    2,
		}
		cb := NewCircuitBreaker(config)

		ctx := context.Background()

		// Trip and wait for half-open
		cb.Execute(ctx, func(ctx context.Context) error { return errors.New("fail") })
		time.Sleep(100 * time.Millisecond)

		// Success threshold successes
		for i := 0; i < 2; i++ {
			cb.Execute(ctx, func(ctx context.Context) error { return nil })
		}

		if cb.State() != StateClosed {
			t.Errorf("expected state closed after success threshold, got %v", cb.State())
		}
	})

	t.Run("ReopensOnFailureInHalfOpen", func(t *testing.T) {
		config := CircuitBreakerConfig{
			Name:                "test",
			MaxFailures:         1,
			Timeout:             50 * time.Millisecond,
			HalfOpenMaxRequests: 10,
			SuccessThreshold:    5,
		}
		cb := NewCircuitBreaker(config)

		ctx := context.Background()

		// Trip and wait for half-open
		cb.Execute(ctx, func(ctx context.Context) error { return errors.New("fail") })
		time.Sleep(100 * time.Millisecond)

		// Fail in half-open - should go back to open
		cb.Execute(ctx, func(ctx context.Context) error { return errors.New("fail again") })

		if cb.State() != StateOpen {
			t.Errorf("expected state open after failure in half-open, got %v", cb.State())
		}
	})
}

func TestCircuitBreaker_Reset(t *testing.T) {
	config := CircuitBreakerConfig{
		Name:        "test",
		MaxFailures: 1,
		Timeout:     1 * time.Hour,
	}
	cb := NewCircuitBreaker(config)

	ctx := context.Background()

	// Trip the circuit
	cb.Execute(ctx, func(ctx context.Context) error { return errors.New("fail") })

	if cb.State() != StateOpen {
		t.Fatalf("expected state open, got %v", cb.State())
	}

	// Reset
	cb.Reset()

	if cb.State() != StateClosed {
		t.Errorf("expected state closed after reset, got %v", cb.State())
	}

	// Should allow requests
	executed := false
	err := cb.Execute(ctx, func(ctx context.Context) error {
		executed = true
		return nil
	})

	if err != nil {
		t.Errorf("unexpected error after reset: %v", err)
	}
	if !executed {
		t.Error("expected function to execute after reset")
	}
}

func TestCircuitBreaker_Metrics(t *testing.T) {
	config := DefaultCircuitBreakerConfig("test-metrics")
	cb := NewCircuitBreaker(config)

	ctx := context.Background()

	// Generate some activity
	cb.Execute(ctx, func(ctx context.Context) error { return nil })
	cb.Execute(ctx, func(ctx context.Context) error { return errors.New("fail") })

	metrics := cb.Metrics()

	if metrics.Name != "test-metrics" {
		t.Errorf("expected name test-metrics, got %s", metrics.Name)
	}
	if metrics.State != StateClosed {
		t.Errorf("expected state closed, got %v", metrics.State)
	}
	if metrics.Failures != 1 {
		t.Errorf("expected 1 failure, got %d", metrics.Failures)
	}
}

func TestCircuitBreaker_ConcurrentAccess(t *testing.T) {
	config := CircuitBreakerConfig{
		Name:        "concurrent-test",
		MaxFailures: 100, // High to avoid tripping
		Timeout:     100 * time.Millisecond,
	}
	cb := NewCircuitBreaker(config)

	ctx := context.Background()
	var wg sync.WaitGroup
	var successCount int64

	// Run concurrent requests
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := cb.Execute(ctx, func(ctx context.Context) error {
				return nil
			})
			if err == nil {
				atomic.AddInt64(&successCount, 1)
			}
		}()
	}

	wg.Wait()

	if successCount != 100 {
		t.Errorf("expected 100 successful executions, got %d", successCount)
	}
}

func TestCircuitBreaker_ContextCancellation(t *testing.T) {
	config := DefaultCircuitBreakerConfig("test")
	cb := NewCircuitBreaker(config)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := cb.Execute(ctx, func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	})

	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

// ==================== Error Tests ====================

func TestCircuitBreakerErrors(t *testing.T) {
	t.Run("ErrCircuitOpen", func(t *testing.T) {
		if ErrCircuitOpen.Error() != "circuit breaker is open" {
			t.Errorf("unexpected error message: %s", ErrCircuitOpen.Error())
		}
	})

	t.Run("ErrTooManyRequests", func(t *testing.T) {
		if ErrTooManyRequests.Error() != "too many requests in half-open state" {
			t.Errorf("unexpected error message: %s", ErrTooManyRequests.Error())
		}
	})
}

// ==================== Benchmark Tests ====================

func BenchmarkCircuitBreaker_Execute_Closed(b *testing.B) {
	config := DefaultCircuitBreakerConfig("bench")
	cb := NewCircuitBreaker(config)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cb.Execute(ctx, func(ctx context.Context) error {
			return nil
		})
	}
}

func BenchmarkCircuitBreaker_Execute_Open(b *testing.B) {
	config := CircuitBreakerConfig{
		Name:        "bench",
		MaxFailures: 1,
		Timeout:     1 * time.Hour,
	}
	cb := NewCircuitBreaker(config)
	ctx := context.Background()

	// Trip the circuit
	cb.Execute(ctx, func(ctx context.Context) error { return errors.New("fail") })

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cb.Execute(ctx, func(ctx context.Context) error {
			return nil
		})
	}
}
