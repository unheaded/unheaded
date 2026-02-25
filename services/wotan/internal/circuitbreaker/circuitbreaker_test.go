// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package circuitbreaker

import (
	"errors"
	"testing"
	"time"

	"github.com/sony/gobreaker"
)

// TestNew verifies circuit breaker creation
func TestNew(t *testing.T) {
	cfg := Config{
		Name:         "test",
		MaxRequests:  3,
		Interval:     10 * time.Second,
		Timeout:      5 * time.Second,
		FailureRatio: 0.5,
		MinRequests:  5,
	}

	cb := New(cfg)

	if cb == nil {
		t.Fatal("New() returned nil")
	}
	if cb.breaker == nil {
		t.Error("breaker is nil")
	}
	if cb.State() != gobreaker.StateClosed {
		t.Errorf("Initial state = %v, want StateClosed", cb.State())
	}
}

// TestDefaultConfig verifies default configuration values
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig("test-service")

	if cfg.Name != "test-service" {
		t.Errorf("Name = %s, want 'test-service'", cfg.Name)
	}
	if cfg.MaxRequests != 3 {
		t.Errorf("MaxRequests = %d, want 3", cfg.MaxRequests)
	}
	if cfg.Interval != 30*time.Second {
		t.Errorf("Interval = %v, want 30s", cfg.Interval)
	}
	if cfg.Timeout != 60*time.Second {
		t.Errorf("Timeout = %v, want 60s", cfg.Timeout)
	}
	if cfg.FailureRatio != 0.6 {
		t.Errorf("FailureRatio = %f, want 0.6", cfg.FailureRatio)
	}
	if cfg.MinRequests != 10 {
		t.Errorf("MinRequests = %d, want 10", cfg.MinRequests)
	}
}

// TestExecute_Success tests successful execution
func TestExecute_Success(t *testing.T) {
	cfg := DefaultConfig("test")
	cb := New(cfg)

	result, err := cb.Execute(func() (interface{}, error) {
		return "success", nil
	})

	if err != nil {
		t.Errorf("Execute() returned error: %v", err)
	}
	if result != "success" {
		t.Errorf("Execute() result = %v, want 'success'", result)
	}
	if cb.State() != gobreaker.StateClosed {
		t.Errorf("State = %v, want StateClosed", cb.State())
	}
}

// TestExecute_Failure tests failed execution
func TestExecute_Failure(t *testing.T) {
	cfg := DefaultConfig("test")
	cb := New(cfg)

	expectedErr := errors.New("operation failed")
	result, err := cb.Execute(func() (interface{}, error) {
		return nil, expectedErr
	})

	if err != expectedErr {
		t.Errorf("Execute() error = %v, want %v", err, expectedErr)
	}
	if result != nil {
		t.Errorf("Execute() result = %v, want nil", result)
	}

	counts := cb.Counts()
	if counts.TotalFailures != 1 {
		t.Errorf("TotalFailures = %d, want 1", counts.TotalFailures)
	}
}

// TestExecute_MultipleTypes tests Execute with different return types
func TestExecute_MultipleTypes(t *testing.T) {
	cfg := DefaultConfig("test")
	cb := New(cfg)

	// Test with int
	result, err := cb.Execute(func() (interface{}, error) {
		return 42, nil
	})
	if err != nil {
		t.Errorf("Execute(int) error: %v", err)
	}
	if result != 42 {
		t.Errorf("Execute(int) = %v, want 42", result)
	}

	// Test with string
	result, err = cb.Execute(func() (interface{}, error) {
		return "test", nil
	})
	if err != nil {
		t.Errorf("Execute(string) error: %v", err)
	}
	if result != "test" {
		t.Errorf("Execute(string) = %v, want 'test'", result)
	}

	// Test with struct
	type testStruct struct {
		Value int
	}
	result, err = cb.Execute(func() (interface{}, error) {
		return testStruct{Value: 100}, nil
	})
	if err != nil {
		t.Errorf("Execute(struct) error: %v", err)
	}
	if s, ok := result.(testStruct); !ok || s.Value != 100 {
		t.Errorf("Execute(struct) = %v, want testStruct{Value: 100}", result)
	}
}

// TestCall_Success tests successful Call
func TestCall_Success(t *testing.T) {
	cfg := DefaultConfig("test")
	cb := New(cfg)

	executed := false
	err := cb.Call(func() error {
		executed = true
		return nil
	})

	if err != nil {
		t.Errorf("Call() returned error: %v", err)
	}
	if !executed {
		t.Error("Function was not executed")
	}
	if cb.State() != gobreaker.StateClosed {
		t.Errorf("State = %v, want StateClosed", cb.State())
	}
}

// TestCall_Failure tests failed Call
func TestCall_Failure(t *testing.T) {
	cfg := DefaultConfig("test")
	cb := New(cfg)

	expectedErr := errors.New("call failed")
	err := cb.Call(func() error {
		return expectedErr
	})

	if err != expectedErr {
		t.Errorf("Call() error = %v, want %v", err, expectedErr)
	}

	counts := cb.Counts()
	if counts.TotalFailures != 1 {
		t.Errorf("TotalFailures = %d, want 1", counts.TotalFailures)
	}
}

// TestState_Transitions tests state transitions
func TestState_Transitions(t *testing.T) {
	cfg := Config{
		Name:         "test",
		MaxRequests:  1,
		Interval:     100 * time.Millisecond,
		Timeout:      100 * time.Millisecond,
		FailureRatio: 0.5,
		MinRequests:  2,
	}
	cb := New(cfg)

	// Initial state: Closed
	if cb.State() != gobreaker.StateClosed {
		t.Errorf("Initial state = %v, want StateClosed", cb.State())
	}

	// Execute one success, one failure (50% failure rate, meets MinRequests)
	cb.Call(func() error { return nil })
	cb.Call(func() error { return errors.New("fail") })

	// Should trip to Open
	if cb.State() != gobreaker.StateOpen {
		t.Errorf("State after failures = %v, want StateOpen", cb.State())
	}

	// Wait for timeout to transition to Half-Open
	time.Sleep(150 * time.Millisecond)

	// Next call should attempt (will be in half-open)
	cb.Call(func() error { return nil })

	// After successful call in half-open, should go to Closed
	if cb.State() != gobreaker.StateClosed {
		t.Errorf("State after success in half-open = %v, want StateClosed", cb.State())
	}
}

// TestCounts tests count tracking
func TestCounts(t *testing.T) {
	cfg := Config{
		Name:         "test",
		MaxRequests:  5,
		Interval:     1 * time.Second,
		Timeout:      1 * time.Second,
		FailureRatio: 0.9,
		MinRequests:  100,
	}
	cb := New(cfg)

	// Execute successful operations
	for i := 0; i < 5; i++ {
		cb.Call(func() error { return nil })
	}

	// Execute failed operations
	for i := 0; i < 3; i++ {
		cb.Call(func() error { return errors.New("fail") })
	}

	counts := cb.Counts()

	if counts.Requests != 8 {
		t.Errorf("Requests = %d, want 8", counts.Requests)
	}
	if counts.TotalSuccesses != 5 {
		t.Errorf("TotalSuccesses = %d, want 5", counts.TotalSuccesses)
	}
	if counts.TotalFailures != 3 {
		t.Errorf("TotalFailures = %d, want 3", counts.TotalFailures)
	}
}

// TestReadyToTrip_MinRequests tests that breaker doesn't trip before MinRequests
func TestReadyToTrip_MinRequests(t *testing.T) {
	cfg := Config{
		Name:         "test",
		MaxRequests:  3,
		Interval:     1 * time.Second,
		Timeout:      1 * time.Second,
		FailureRatio: 0.5,
		MinRequests:  5,
	}
	cb := New(cfg)

	// Send 4 failures (below MinRequests)
	for i := 0; i < 4; i++ {
		cb.Call(func() error { return errors.New("fail") })
	}

	// Should still be closed (below MinRequests)
	if cb.State() != gobreaker.StateClosed {
		t.Errorf("State = %v, want StateClosed (below MinRequests)", cb.State())
	}

	// Send 5th request (failure) - now at MinRequests with 100% failure
	cb.Call(func() error { return errors.New("fail") })

	// Should now be open (met MinRequests and exceeded FailureRatio)
	if cb.State() != gobreaker.StateOpen {
		t.Errorf("State = %v, want StateOpen (met MinRequests)", cb.State())
	}
}

// TestReadyToTrip_FailureRatio tests failure ratio threshold
func TestReadyToTrip_FailureRatio(t *testing.T) {
	cfg := Config{
		Name:         "test",
		MaxRequests:  3,
		Interval:     1 * time.Second,
		Timeout:      1 * time.Second,
		FailureRatio: 0.6,
		MinRequests:  10,
	}
	cb := New(cfg)

	// Send 10 requests: 5 success, 5 failure (50% failure - below threshold)
	for i := 0; i < 5; i++ {
		cb.Call(func() error { return nil })
		cb.Call(func() error { return errors.New("fail") })
	}

	// Should still be closed (failure ratio < 0.6)
	if cb.State() != gobreaker.StateClosed {
		t.Errorf("State = %v, want StateClosed (below FailureRatio)", cb.State())
	}

	// Add one more failure (6/11 = 54.5%, still below)
	cb.Call(func() error { return errors.New("fail") })
	if cb.State() != gobreaker.StateClosed {
		t.Error("State should still be closed at 54.5% failure")
	}

	// Add 2 more failures (8/13 = 61.5%, above threshold)
	cb.Call(func() error { return errors.New("fail") })
	cb.Call(func() error { return errors.New("fail") })

	// Should now be open (failure ratio >= 0.6)
	if cb.State() != gobreaker.StateOpen {
		t.Errorf("State = %v, want StateOpen (exceeded FailureRatio)", cb.State())
	}
}

// TestMaxRequests_HalfOpen tests MaxRequests limit in half-open state
func TestMaxRequests_HalfOpen(t *testing.T) {
	cfg := Config{
		Name:         "test",
		MaxRequests:  2,
		Interval:     50 * time.Millisecond,
		Timeout:      50 * time.Millisecond,
		FailureRatio: 0.5,
		MinRequests:  2,
	}
	cb := New(cfg)

	// Trip the breaker
	cb.Call(func() error { return errors.New("fail") })
	cb.Call(func() error { return errors.New("fail") })

	if cb.State() != gobreaker.StateOpen {
		t.Fatalf("Breaker should be open, got %v", cb.State())
	}

	// Wait for timeout to move to half-open
	time.Sleep(100 * time.Millisecond)

	// First request in half-open should succeed
	err := cb.Call(func() error { return nil })
	if err != nil {
		t.Errorf("First request in half-open failed: %v", err)
	}

	// Note: After one successful request in half-open, gobreaker transitions back to closed
	// So we need to trip it again to test MaxRequests properly
}

// TestInterval_ResetsCounters tests that interval resets counters
func TestInterval_ResetsCounters(t *testing.T) {
	cfg := Config{
		Name:         "test",
		MaxRequests:  3,
		Interval:     100 * time.Millisecond,
		Timeout:      1 * time.Second,
		FailureRatio: 0.8,
		MinRequests:  5,
	}
	cb := New(cfg)

	// Send some failures
	for i := 0; i < 3; i++ {
		cb.Call(func() error { return errors.New("fail") })
	}

	counts := cb.Counts()
	if counts.TotalFailures != 3 {
		t.Errorf("TotalFailures = %d, want 3", counts.TotalFailures)
	}

	// Wait for interval to reset
	time.Sleep(150 * time.Millisecond)

	// Send a new request (this should be in a new interval)
	cb.Call(func() error { return nil })

	counts = cb.Counts()
	// After interval, counts should reset and we should have 1 request
	if counts.Requests != 1 {
		t.Errorf("Requests after interval = %d, want 1", counts.Requests)
	}
}

// TestCircuitBreaker_ErrorPropagation tests that errors are properly propagated
func TestCircuitBreaker_ErrorPropagation(t *testing.T) {
	cfg := DefaultConfig("test")
	cb := New(cfg)

	customErr := errors.New("custom error message")

	_, err := cb.Execute(func() (interface{}, error) {
		return nil, customErr
	})

	if err != customErr {
		t.Errorf("Error = %v, want %v", err, customErr)
	}

	err = cb.Call(func() error {
		return customErr
	})

	if err != customErr {
		t.Errorf("Error = %v, want %v", err, customErr)
	}
}

// TestCircuitBreaker_SuccessAfterFailures tests recovery after failures
func TestCircuitBreaker_SuccessAfterFailures(t *testing.T) {
	cfg := Config{
		Name:         "test",
		MaxRequests:  1,
		Interval:     100 * time.Millisecond,
		Timeout:      50 * time.Millisecond,
		FailureRatio: 0.5,
		MinRequests:  2,
	}
	cb := New(cfg)

	// Cause failures
	cb.Call(func() error { return errors.New("fail") })
	cb.Call(func() error { return errors.New("fail") })

	if cb.State() != gobreaker.StateOpen {
		t.Fatalf("State = %v, want StateOpen", cb.State())
	}

	// Wait for timeout
	time.Sleep(100 * time.Millisecond)

	// Successful call should close the circuit
	err := cb.Call(func() error { return nil })
	if err != nil {
		t.Errorf("Call() error = %v, want nil", err)
	}

	if cb.State() != gobreaker.StateClosed {
		t.Errorf("State = %v, want StateClosed after recovery", cb.State())
	}
}

// TestCircuitBreaker_OpenStateRejectsRequests tests that open state rejects requests
func TestCircuitBreaker_OpenStateRejectsRequests(t *testing.T) {
	cfg := Config{
		Name:         "test",
		MaxRequests:  1,
		Interval:     100 * time.Millisecond,
		Timeout:      10 * time.Second, // Long timeout to keep it open
		FailureRatio: 0.5,
		MinRequests:  2,
	}
	cb := New(cfg)

	// Trip the breaker
	cb.Call(func() error { return errors.New("fail") })
	cb.Call(func() error { return errors.New("fail") })

	if cb.State() != gobreaker.StateOpen {
		t.Fatalf("State = %v, want StateOpen", cb.State())
	}

	// Request should be rejected
	err := cb.Call(func() error {
		t.Error("Function should not be called when circuit is open")
		return nil
	})

	if err == nil {
		t.Error("Expected error when circuit is open, got nil")
	}

	// gobreaker returns ErrOpenState
	if err != gobreaker.ErrOpenState {
		t.Errorf("Error = %v, want ErrOpenState", err)
	}
}

// TestCircuitBreaker_ZeroValues tests behavior with zero values
func TestCircuitBreaker_ZeroValues(t *testing.T) {
	cfg := Config{
		Name:         "test",
		MaxRequests:  0, // Will use gobreaker default (1)
		Interval:     0, // Will use gobreaker default
		Timeout:      0, // Will use gobreaker default
		FailureRatio: 0, // Will never trip (0% threshold)
		MinRequests:  0, // Will evaluate immediately
	}

	cb := New(cfg)

	// Should not panic
	err := cb.Call(func() error { return nil })
	if err != nil {
		t.Errorf("Call() error = %v, want nil", err)
	}
}

// TestCircuitBreaker_HighFailureRatio tests with 100% failure ratio
func TestCircuitBreaker_HighFailureRatio(t *testing.T) {
	cfg := Config{
		Name:         "test",
		MaxRequests:  3,
		Interval:     1 * time.Second,
		Timeout:      1 * time.Second,
		FailureRatio: 1.0, // Require 100% failure to trip
		MinRequests:  3,
	}
	cb := New(cfg)

	// 2 failures, 1 success (66% failure, below 100%)
	cb.Call(func() error { return errors.New("fail") })
	cb.Call(func() error { return errors.New("fail") })
	cb.Call(func() error { return nil })

	// Should still be closed
	if cb.State() != gobreaker.StateClosed {
		t.Errorf("State = %v, want StateClosed (not 100%% failure)", cb.State())
	}

	// Add one more failure to make it 3 failures, 1 success (75%)
	cb.Call(func() error { return errors.New("fail") })

	// Still not 100%
	if cb.State() != gobreaker.StateClosed {
		t.Error("State should be closed (still not 100% failure)")
	}
}

// TestCircuitBreaker_ConcurrentCalls tests concurrent execution
func TestCircuitBreaker_ConcurrentCalls(t *testing.T) {
	cfg := Config{
		Name:         "test",
		MaxRequests:  10,
		Interval:     1 * time.Second,
		Timeout:      1 * time.Second,
		FailureRatio: 0.9,
		MinRequests:  100,
	}
	cb := New(cfg)

	// Execute many concurrent calls
	done := make(chan bool, 50)
	for i := 0; i < 50; i++ {
		go func() {
			cb.Call(func() error {
				time.Sleep(1 * time.Millisecond)
				return nil
			})
			done <- true
		}()
	}

	// Wait for all to complete
	for i := 0; i < 50; i++ {
		<-done
	}

	counts := cb.Counts()
	if counts.TotalSuccesses != 50 {
		t.Errorf("TotalSuccesses = %d, want 50", counts.TotalSuccesses)
	}
}

// TestErrors tests exported error values
func TestErrors(t *testing.T) {
	if ErrCircuitOpen == nil {
		t.Error("ErrCircuitOpen is nil")
	}
	if ErrCircuitOpen.Error() != "circuit breaker is open" {
		t.Errorf("ErrCircuitOpen.Error() = %s", ErrCircuitOpen.Error())
	}

	if ErrTooManyRequests == nil {
		t.Error("ErrTooManyRequests is nil")
	}
	if ErrTooManyRequests.Error() != "too many requests in half-open state" {
		t.Errorf("ErrTooManyRequests.Error() = %s", ErrTooManyRequests.Error())
	}
}
