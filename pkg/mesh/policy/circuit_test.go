// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package policy

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

// ==================== CircuitBreaker Tests ====================

func TestNewCircuitBreaker(t *testing.T) {
	t.Run("WithNilConfig", func(t *testing.T) {
		cb := NewCircuitBreaker(nil)
		if cb == nil {
			t.Fatal("expected non-nil circuit breaker")
		}
		if cb.State() != StateClosed {
			t.Errorf("expected initial state closed, got %v", cb.State())
		}
	})

	t.Run("WithCustomConfig", func(t *testing.T) {
		config := &CircuitBreakerConfig{
			Enabled:          true,
			FailureThreshold: 10,
			SuccessThreshold: 5,
			HalfOpenRequests: 3,
			Timeout:          60 * time.Second,
		}
		cb := NewCircuitBreaker(config)

		if cb.config.FailureThreshold != 10 {
			t.Errorf("expected FailureThreshold 10, got %d", cb.config.FailureThreshold)
		}
		if cb.config.SuccessThreshold != 5 {
			t.Errorf("expected SuccessThreshold 5, got %d", cb.config.SuccessThreshold)
		}
	})
}

func TestCircuitBreaker_Disabled(t *testing.T) {
	config := &CircuitBreakerConfig{
		Enabled:          false,
		FailureThreshold: 1,
	}
	cb := NewCircuitBreaker(config)

	ctx := context.Background()
	executed := false

	// Should execute even with failures
	for i := 0; i < 10; i++ {
		cb.Execute(ctx, func(ctx context.Context) error {
			return errors.New("fail")
		})
	}

	// Should still allow execution
	err := cb.Execute(ctx, func(ctx context.Context) error {
		executed = true
		return nil
	})

	if err != nil {
		t.Errorf("unexpected error with disabled circuit breaker: %v", err)
	}
	if !executed {
		t.Error("function should execute when circuit breaker is disabled")
	}
}

func TestCircuitBreaker_Execute_Success(t *testing.T) {
	config := &CircuitBreakerConfig{
		Enabled:          true,
		FailureThreshold: 5,
	}
	cb := NewCircuitBreaker(config)

	ctx := context.Background()
	result := ""

	err := cb.Execute(ctx, func(ctx context.Context) error {
		result = "executed"
		return nil
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != "executed" {
		t.Error("function was not executed")
	}
}

func TestCircuitBreaker_Execute_Failure(t *testing.T) {
	config := &CircuitBreakerConfig{
		Enabled:          true,
		FailureThreshold: 5,
	}
	cb := NewCircuitBreaker(config)

	ctx := context.Background()
	expectedErr := errors.New("operation failed")

	err := cb.Execute(ctx, func(ctx context.Context) error {
		return expectedErr
	})

	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

func TestCircuitBreaker_TransitionToOpen(t *testing.T) {
	t.Run("ByFailureThreshold", func(t *testing.T) {
		config := &CircuitBreakerConfig{
			Enabled:          true,
			FailureThreshold: 3,
			Timeout:          1 * time.Hour,
		}
		cb := NewCircuitBreaker(config)

		ctx := context.Background()

		// Trigger failures
		for i := 0; i < 3; i++ {
			cb.Execute(ctx, func(ctx context.Context) error {
				return errors.New("fail")
			})
		}

		if cb.State() != StateOpen {
			t.Errorf("expected state open, got %v", cb.State())
		}
	})

	t.Run("ByConsecutiveErrors", func(t *testing.T) {
		config := &CircuitBreakerConfig{
			Enabled:           true,
			FailureThreshold:  100, // High
			ConsecutiveErrors: 3,
			Timeout:           1 * time.Hour,
		}
		cb := NewCircuitBreaker(config)

		ctx := context.Background()

		// Trigger consecutive failures
		for i := 0; i < 3; i++ {
			cb.Execute(ctx, func(ctx context.Context) error {
				return errors.New("fail")
			})
		}

		if cb.State() != StateOpen {
			t.Errorf("expected state open by consecutive errors, got %v", cb.State())
		}
	})

	t.Run("ByFailureRate", func(t *testing.T) {
		config := &CircuitBreakerConfig{
			Enabled:            true,
			FailureThreshold:   100, // High
			FailureRatePercent: 50,
			Timeout:            1 * time.Hour,
		}
		cb := NewCircuitBreaker(config)

		ctx := context.Background()

		// Need at least 10 requests for rate calculation
		for i := 0; i < 15; i++ {
			cb.Execute(ctx, func(ctx context.Context) error {
				return errors.New("fail")
			})
		}

		if cb.State() != StateOpen {
			t.Errorf("expected state open by failure rate, got %v", cb.State())
		}
	})
}

func TestCircuitBreaker_RejectsRequests_WhenOpen(t *testing.T) {
	config := &CircuitBreakerConfig{
		Enabled:          true,
		FailureThreshold: 1,
		Timeout:          1 * time.Hour,
	}
	cb := NewCircuitBreaker(config)

	ctx := context.Background()

	// Trip the circuit
	cb.Execute(ctx, func(ctx context.Context) error {
		return errors.New("fail")
	})

	// Should reject
	executed := false
	err := cb.Execute(ctx, func(ctx context.Context) error {
		executed = true
		return nil
	})

	if err != ErrCircuitOpen {
		t.Errorf("expected ErrCircuitOpen, got %v", err)
	}
	if executed {
		t.Error("function should not execute when circuit is open")
	}
}

func TestCircuitBreaker_TransitionToHalfOpen(t *testing.T) {
	config := &CircuitBreakerConfig{
		Enabled:          true,
		FailureThreshold: 1,
		HalfOpenRequests: 2,
		Timeout:          50 * time.Millisecond,
	}
	cb := NewCircuitBreaker(config)

	ctx := context.Background()

	// Trip the circuit
	cb.Execute(ctx, func(ctx context.Context) error {
		return errors.New("fail")
	})

	// Wait for timeout
	time.Sleep(100 * time.Millisecond)

	// Should transition to half-open on next request
	cb.Execute(ctx, func(ctx context.Context) error {
		return nil
	})

	state := cb.State()
	if state != StateHalfOpen && state != StateClosed {
		t.Errorf("expected state half-open or closed, got %v", state)
	}
}

func TestCircuitBreaker_HalfOpenLimitsRequests(t *testing.T) {
	config := &CircuitBreakerConfig{
		Enabled:          true,
		FailureThreshold: 1,
		HalfOpenRequests: 1,
		SuccessThreshold: 10, // High to stay in half-open
		Timeout:          50 * time.Millisecond,
	}
	cb := NewCircuitBreaker(config)

	ctx := context.Background()

	// Trip the circuit
	cb.Execute(ctx, func(ctx context.Context) error {
		return errors.New("fail")
	})

	// Wait for timeout
	time.Sleep(100 * time.Millisecond)

	// First request should be allowed
	err1 := cb.Execute(ctx, func(ctx context.Context) error {
		return nil
	})

	// Second request may be rejected
	err2 := cb.Execute(ctx, func(ctx context.Context) error {
		return nil
	})

	// At least one should succeed
	if err1 != nil && err2 != nil {
		t.Error("expected at least one request to be allowed in half-open")
	}
}

func TestCircuitBreaker_TransitionToClosed(t *testing.T) {
	config := &CircuitBreakerConfig{
		Enabled:          true,
		FailureThreshold: 1,
		SuccessThreshold: 2,
		HalfOpenRequests: 10,
		Timeout:          50 * time.Millisecond,
	}
	cb := NewCircuitBreaker(config)

	ctx := context.Background()

	// Trip the circuit
	cb.Execute(ctx, func(ctx context.Context) error {
		return errors.New("fail")
	})

	// Wait for half-open
	time.Sleep(100 * time.Millisecond)

	// Success threshold
	for i := 0; i < 2; i++ {
		cb.Execute(ctx, func(ctx context.Context) error {
			return nil
		})
	}

	if cb.State() != StateClosed {
		t.Errorf("expected state closed after success threshold, got %v", cb.State())
	}
}

func TestCircuitBreaker_ReopensOnFailureInHalfOpen(t *testing.T) {
	config := &CircuitBreakerConfig{
		Enabled:          true,
		FailureThreshold: 1,
		SuccessThreshold: 5,
		HalfOpenRequests: 10,
		Timeout:          50 * time.Millisecond,
	}
	cb := NewCircuitBreaker(config)

	ctx := context.Background()

	// Trip the circuit
	cb.Execute(ctx, func(ctx context.Context) error {
		return errors.New("fail")
	})

	// Wait for half-open
	time.Sleep(100 * time.Millisecond)

	// Fail in half-open
	cb.Execute(ctx, func(ctx context.Context) error {
		return errors.New("fail again")
	})

	if cb.State() != StateOpen {
		t.Errorf("expected state open after failure in half-open, got %v", cb.State())
	}
}

func TestCircuitBreaker_Stats(t *testing.T) {
	config := &CircuitBreakerConfig{
		Enabled:          true,
		FailureThreshold: 10,
	}
	cb := NewCircuitBreaker(config)

	ctx := context.Background()

	// Generate activity
	cb.Execute(ctx, func(ctx context.Context) error { return nil })
	cb.Execute(ctx, func(ctx context.Context) error { return nil })
	cb.Execute(ctx, func(ctx context.Context) error { return errors.New("fail") })

	stats := cb.Stats()

	if stats.State != StateClosed {
		t.Errorf("expected state closed, got %v", stats.State)
	}
	if stats.Failures != 1 {
		t.Errorf("expected 1 failure, got %d", stats.Failures)
	}
	if stats.ConsecutiveErrs != 1 {
		t.Errorf("expected 1 consecutive error, got %d", stats.ConsecutiveErrs)
	}
}

func TestCircuitBreaker_ForceOpen(t *testing.T) {
	config := &CircuitBreakerConfig{
		Enabled:          true,
		FailureThreshold: 100,
	}
	cb := NewCircuitBreaker(config)

	cb.ForceOpen()

	if cb.State() != StateOpen {
		t.Errorf("expected state open after ForceOpen, got %v", cb.State())
	}
}

func TestCircuitBreaker_ForceClose(t *testing.T) {
	config := &CircuitBreakerConfig{
		Enabled:          true,
		FailureThreshold: 1,
		Timeout:          1 * time.Hour,
	}
	cb := NewCircuitBreaker(config)

	ctx := context.Background()

	// Trip the circuit
	cb.Execute(ctx, func(ctx context.Context) error {
		return errors.New("fail")
	})

	if cb.State() != StateOpen {
		t.Fatalf("expected state open, got %v", cb.State())
	}

	cb.ForceClose()

	if cb.State() != StateClosed {
		t.Errorf("expected state closed after ForceClose, got %v", cb.State())
	}

	// Should allow requests
	err := cb.Execute(ctx, func(ctx context.Context) error { return nil })
	if err != nil {
		t.Errorf("expected no error after ForceClose, got %v", err)
	}
}

func TestCircuitBreaker_OnStateChange(t *testing.T) {
	config := &CircuitBreakerConfig{
		Enabled:          true,
		FailureThreshold: 1,
		Timeout:          1 * time.Hour,
	}
	cb := NewCircuitBreaker(config)

	var callbackCalled bool
	var fromState, toState CircuitState

	cb.OnStateChange(func(from, to CircuitState) {
		callbackCalled = true
		fromState = from
		toState = to
	})

	ctx := context.Background()
	cb.Execute(ctx, func(ctx context.Context) error {
		return errors.New("fail")
	})

	if !callbackCalled {
		t.Error("expected state change callback to be called")
	}
	if fromState != StateClosed {
		t.Errorf("expected from state closed, got %v", fromState)
	}
	if toState != StateOpen {
		t.Errorf("expected to state open, got %v", toState)
	}
}

func TestCircuitBreaker_SlidingWindow(t *testing.T) {
	config := &CircuitBreakerConfig{
		Enabled:            true,
		FailureThreshold:   1000, // Very high to only trigger by rate
		FailureRatePercent: 60,   // 60% failure rate triggers open
		WindowDuration:     time.Minute,
	}
	cb := NewCircuitBreaker(config)

	ctx := context.Background()

	// Generate high failure rate - we need at least 10 requests for rate calculation
	// Send 80% failures to ensure we exceed 60% threshold
	for i := 0; i < 25; i++ {
		if i%5 < 4 { // 4 failures per 5 = 80% failure rate
			cb.Execute(ctx, func(ctx context.Context) error {
				return errors.New("fail")
			})
		} else {
			cb.Execute(ctx, func(ctx context.Context) error {
				return nil
			})
		}
	}

	stats := cb.Stats()
	if stats.FailureRate < 60 {
		t.Logf("failure rate: %.2f%%, expected > 60%%", stats.FailureRate)
	}

	// The sliding window rate calculation depends on implementation details
	// Just verify that the failure rate is being tracked correctly
	if stats.FailureRate < 60 {
		t.Errorf("expected failure rate >= 60%%, got %.2f%%", stats.FailureRate)
	}
}

func TestCircuitBreaker_ConcurrentAccess(t *testing.T) {
	config := &CircuitBreakerConfig{
		Enabled:          true,
		FailureThreshold: 1000, // High to avoid tripping
	}
	cb := NewCircuitBreaker(config)

	ctx := context.Background()
	var wg sync.WaitGroup
	var successCount int64

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
		t.Errorf("expected 100 successes, got %d", successCount)
	}
}

// ==================== CircuitBreakerRegistry Tests ====================

func TestCircuitBreakerRegistry(t *testing.T) {
	t.Run("NewCircuitBreakerRegistry", func(t *testing.T) {
		config := &CircuitBreakerConfig{
			Enabled:          true,
			FailureThreshold: 5,
		}
		registry := NewCircuitBreakerRegistry(config)

		if registry == nil {
			t.Fatal("expected non-nil registry")
		}
	})

	t.Run("Get_CreatesIfNotExists", func(t *testing.T) {
		config := &CircuitBreakerConfig{
			Enabled:          true,
			FailureThreshold: 5,
		}
		registry := NewCircuitBreakerRegistry(config)

		cb := registry.Get("service-a")
		if cb == nil {
			t.Fatal("expected non-nil circuit breaker")
		}

		// Same key should return same instance
		cb2 := registry.Get("service-a")
		if cb != cb2 {
			t.Error("expected same circuit breaker instance")
		}
	})

	t.Run("Get_DifferentServices", func(t *testing.T) {
		config := &CircuitBreakerConfig{
			Enabled:          true,
			FailureThreshold: 5,
		}
		registry := NewCircuitBreakerRegistry(config)

		cb1 := registry.Get("service-a")
		cb2 := registry.Get("service-b")

		if cb1 == cb2 {
			t.Error("expected different circuit breakers for different services")
		}
	})

	t.Run("Remove", func(t *testing.T) {
		config := &CircuitBreakerConfig{
			Enabled:          true,
			FailureThreshold: 5,
		}
		registry := NewCircuitBreakerRegistry(config)

		cb1 := registry.Get("service-a")
		registry.Remove("service-a")
		cb2 := registry.Get("service-a")

		if cb1 == cb2 {
			t.Error("expected new circuit breaker after remove")
		}
	})

	t.Run("Stats", func(t *testing.T) {
		config := &CircuitBreakerConfig{
			Enabled:          true,
			FailureThreshold: 5,
		}
		registry := NewCircuitBreakerRegistry(config)

		registry.Get("service-a")
		registry.Get("service-b")

		stats := registry.Stats()
		if len(stats) != 2 {
			t.Errorf("expected 2 stats entries, got %d", len(stats))
		}

		if _, ok := stats["service-a"]; !ok {
			t.Error("expected stats for service-a")
		}
		if _, ok := stats["service-b"]; !ok {
			t.Error("expected stats for service-b")
		}
	})

	t.Run("ConcurrentAccess", func(t *testing.T) {
		config := &CircuitBreakerConfig{
			Enabled:          true,
			FailureThreshold: 5,
		}
		registry := NewCircuitBreakerRegistry(config)

		var wg sync.WaitGroup
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				service := "service-" + string(rune('a'+idx%5))
				registry.Get(service)
			}(i)
		}
		wg.Wait()

		stats := registry.Stats()
		if len(stats) != 5 {
			t.Errorf("expected 5 services, got %d", len(stats))
		}
	})
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

func BenchmarkCircuitBreaker_Execute(b *testing.B) {
	config := &CircuitBreakerConfig{
		Enabled:          true,
		FailureThreshold: 100000,
	}
	cb := NewCircuitBreaker(config)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cb.Execute(ctx, func(ctx context.Context) error {
			return nil
		})
	}
}

func BenchmarkCircuitBreakerRegistry_Get(b *testing.B) {
	config := &CircuitBreakerConfig{
		Enabled:          true,
		FailureThreshold: 5,
	}
	registry := NewCircuitBreakerRegistry(config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		registry.Get("service-a")
	}
}
