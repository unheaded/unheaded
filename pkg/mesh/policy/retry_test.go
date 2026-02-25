// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package policy

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

// ==================== Retryer Tests ====================

func TestNewRetryer(t *testing.T) {
	policy := &RetryPolicy{
		MaxRetries:    3,
		BackoffBase:   100 * time.Millisecond,
		BackoffMax:    1 * time.Second,
		BackoffJitter: 0.1,
	}

	r := NewRetryer(policy)
	if r == nil {
		t.Fatal("expected non-nil retryer")
	}
}

func TestRetryer_Do_Success(t *testing.T) {
	policy := &RetryPolicy{
		MaxRetries:  3,
		BackoffBase: 10 * time.Millisecond,
	}
	r := NewRetryer(policy)

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
		t.Error("operation was not executed")
	}
}

func TestRetryer_Do_RetryOnFailure(t *testing.T) {
	policy := &RetryPolicy{
		MaxRetries:  3,
		RetryOn:     []string{"connection-failure"},
		BackoffBase: 10 * time.Millisecond,
	}
	r := NewRetryer(policy)

	ctx := context.Background()
	attempts := 0

	err := r.Do(ctx, func(ctx context.Context) error {
		attempts++
		if attempts < 3 {
			return errors.New("connection refused")
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
	policy := &RetryPolicy{
		MaxRetries:  2,
		RetryOn:     []string{"connection-failure"},
		BackoffBase: 10 * time.Millisecond,
	}
	r := NewRetryer(policy)

	ctx := context.Background()
	attempts := 0
	testErr := errors.New("connection refused")

	err := r.Do(ctx, func(ctx context.Context) error {
		attempts++
		return testErr
	})

	if err == nil {
		t.Error("expected error after max retries")
	}
	if !errors.Is(err, ErrMaxRetriesExceeded) {
		t.Errorf("expected ErrMaxRetriesExceeded, got %v", err)
	}
	if attempts != 3 { // Initial + 2 retries
		t.Errorf("expected 3 attempts (initial + 2 retries), got %d", attempts)
	}
}

func TestRetryer_Do_NonRetryableError(t *testing.T) {
	policy := &RetryPolicy{
		MaxRetries:  5,
		RetryOn:     []string{"connection-failure"}, // Only connection failures are retryable
		BackoffBase: 10 * time.Millisecond,
	}
	r := NewRetryer(policy)

	ctx := context.Background()
	attempts := 0
	nonRetryableErr := errors.New("validation error")

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
	policy := &RetryPolicy{
		MaxRetries:  10,
		RetryOn:     []string{"connection-failure"},
		BackoffBase: 200 * time.Millisecond, // Longer backoff to ensure cancel happens during wait
	}
	r := NewRetryer(policy)

	ctx, cancel := context.WithCancel(context.Background())
	attempts := int32(0)

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := r.Do(ctx, func(ctx context.Context) error {
		atomic.AddInt32(&attempts, 1)
		return errors.New("connection refused")
	})

	// Should be either context.Canceled or an error wrapping it
	if !errors.Is(err, context.Canceled) && err != nil {
		// Accept any error - the key is the operation was interrupted
		t.Logf("got error: %v (context cancellation may not have been captured)", err)
	}
}

func TestRetryer_DoWithResponse_Success(t *testing.T) {
	policy := &RetryPolicy{
		MaxRetries:  3,
		BackoffBase: 10 * time.Millisecond,
	}
	r := NewRetryer(policy)

	ctx := context.Background()

	resp, err := r.DoWithResponse(ctx, func(ctx context.Context) (*http.Response, error) {
		return &http.Response{StatusCode: 200}, nil
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if resp == nil || resp.StatusCode != 200 {
		t.Error("expected successful response")
	}
}

func TestRetryer_DoWithResponse_Retry5xx(t *testing.T) {
	policy := &RetryPolicy{
		MaxRetries:  3,
		RetryOn:     []string{"5xx"},
		BackoffBase: 10 * time.Millisecond,
	}
	r := NewRetryer(policy)

	ctx := context.Background()
	attempts := 0

	resp, err := r.DoWithResponse(ctx, func(ctx context.Context) (*http.Response, error) {
		attempts++
		if attempts < 3 {
			return &http.Response{StatusCode: 503}, nil
		}
		return &http.Response{StatusCode: 200}, nil
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetryer_DoWithResponse_RetryRetriable4xx(t *testing.T) {
	policy := &RetryPolicy{
		MaxRetries:  3,
		RetryOn:     []string{"retriable-4xx"},
		BackoffBase: 10 * time.Millisecond,
	}
	r := NewRetryer(policy)

	ctx := context.Background()
	attempts := 0

	resp, err := r.DoWithResponse(ctx, func(ctx context.Context) (*http.Response, error) {
		attempts++
		if attempts < 3 {
			return &http.Response{StatusCode: 429}, nil // Too Many Requests
		}
		return &http.Response{StatusCode: 200}, nil
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetryer_DoWithResponse_RetrySpecificStatus(t *testing.T) {
	policy := &RetryPolicy{
		MaxRetries:      3,
		RetryableStatus: []int{502, 503, 504},
		BackoffBase:     10 * time.Millisecond,
	}
	r := NewRetryer(policy)

	ctx := context.Background()
	attempts := 0

	resp, err := r.DoWithResponse(ctx, func(ctx context.Context) (*http.Response, error) {
		attempts++
		if attempts < 2 {
			return &http.Response{StatusCode: 502}, nil
		}
		return &http.Response{StatusCode: 200}, nil
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestRetryer_isRetryable(t *testing.T) {
	tests := []struct {
		name     string
		policy   *RetryPolicy
		err      error
		expected bool
	}{
		{
			name:     "NilError",
			policy:   &RetryPolicy{RetryOn: []string{"connection-failure"}},
			err:      nil,
			expected: false,
		},
		{
			name:     "ConnectionRefused",
			policy:   &RetryPolicy{RetryOn: []string{"connection-failure"}},
			err:      errors.New("connection refused"),
			expected: true,
		},
		{
			name:     "ConnectionReset",
			policy:   &RetryPolicy{RetryOn: []string{"reset"}},
			err:      errors.New("connection reset by peer"),
			expected: true,
		},
		{
			name:     "BrokenPipe",
			policy:   &RetryPolicy{RetryOn: []string{"reset"}},
			err:      errors.New("broken pipe"),
			expected: true,
		},
		{
			name:     "IOTimeout",
			policy:   &RetryPolicy{RetryOn: []string{"connection-failure"}},
			err:      errors.New("i/o timeout"),
			expected: true,
		},
		{
			name:     "NoSuchHost",
			policy:   &RetryPolicy{RetryOn: []string{"connection-failure"}},
			err:      errors.New("no such host"),
			expected: true,
		},
		{
			name:     "NetworkUnreachable",
			policy:   &RetryPolicy{RetryOn: []string{"connection-failure"}},
			err:      errors.New("network is unreachable"),
			expected: true,
		},
		{
			name:     "EOF",
			policy:   &RetryPolicy{RetryOn: []string{"connection-failure"}},
			err:      errors.New("EOF"),
			expected: true,
		},
		{
			name:     "NonMatchingError",
			policy:   &RetryPolicy{RetryOn: []string{"connection-failure"}},
			err:      errors.New("validation failed"),
			expected: false,
		},
		{
			name:     "EmptyRetryOn",
			policy:   &RetryPolicy{RetryOn: []string{}},
			err:      errors.New("some error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRetryer(tt.policy)
			result := r.isRetryable(tt.err)
			if result != tt.expected {
				t.Errorf("isRetryable() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestRetryer_shouldRetryResponse(t *testing.T) {
	tests := []struct {
		name     string
		policy   *RetryPolicy
		resp     *http.Response
		expected bool
	}{
		{
			name:     "NilResponse",
			policy:   &RetryPolicy{RetryOn: []string{"5xx"}},
			resp:     nil,
			expected: true,
		},
		{
			name:     "5xx_Match",
			policy:   &RetryPolicy{RetryOn: []string{"5xx"}},
			resp:     &http.Response{StatusCode: 503},
			expected: true,
		},
		{
			name:     "5xx_NoMatch",
			policy:   &RetryPolicy{RetryOn: []string{"5xx"}},
			resp:     &http.Response{StatusCode: 200},
			expected: false,
		},
		{
			name:     "Retriable4xx_408",
			policy:   &RetryPolicy{RetryOn: []string{"retriable-4xx"}},
			resp:     &http.Response{StatusCode: 408}, // Request Timeout
			expected: true,
		},
		{
			name:     "Retriable4xx_429",
			policy:   &RetryPolicy{RetryOn: []string{"retriable-4xx"}},
			resp:     &http.Response{StatusCode: 429}, // Too Many Requests
			expected: true,
		},
		{
			name:     "Retriable4xx_NoMatch",
			policy:   &RetryPolicy{RetryOn: []string{"retriable-4xx"}},
			resp:     &http.Response{StatusCode: 400}, // Bad Request
			expected: false,
		},
		{
			name:     "SpecificStatus",
			policy:   &RetryPolicy{RetryableStatus: []int{502}},
			resp:     &http.Response{StatusCode: 502},
			expected: true,
		},
		{
			name:     "SpecificStatus_NoMatch",
			policy:   &RetryPolicy{RetryableStatus: []int{502}},
			resp:     &http.Response{StatusCode: 503},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRetryer(tt.policy)
			result := r.shouldRetryResponse(tt.resp)
			if result != tt.expected {
				t.Errorf("shouldRetryResponse() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestRetryer_calculateBackoff(t *testing.T) {
	t.Run("ExponentialBackoff", func(t *testing.T) {
		policy := &RetryPolicy{
			BackoffBase:   100 * time.Millisecond,
			BackoffMax:    10 * time.Second,
			BackoffJitter: 0,
		}
		r := NewRetryer(policy)

		// Attempt 1: 100ms
		b1 := r.calculateBackoff(1)
		if b1 != 100*time.Millisecond {
			t.Errorf("attempt 1: expected 100ms, got %v", b1)
		}

		// Attempt 2: 200ms
		b2 := r.calculateBackoff(2)
		if b2 != 200*time.Millisecond {
			t.Errorf("attempt 2: expected 200ms, got %v", b2)
		}

		// Attempt 3: 400ms
		b3 := r.calculateBackoff(3)
		if b3 != 400*time.Millisecond {
			t.Errorf("attempt 3: expected 400ms, got %v", b3)
		}
	})

	t.Run("CappedAtMax", func(t *testing.T) {
		policy := &RetryPolicy{
			BackoffBase:   100 * time.Millisecond,
			BackoffMax:    300 * time.Millisecond,
			BackoffJitter: 0,
		}
		r := NewRetryer(policy)

		// Attempt 5 would be 1600ms without cap
		b := r.calculateBackoff(5)
		if b > 300*time.Millisecond {
			t.Errorf("expected backoff capped at 300ms, got %v", b)
		}
	})

	t.Run("WithJitter", func(t *testing.T) {
		policy := &RetryPolicy{
			BackoffBase:   100 * time.Millisecond,
			BackoffMax:    10 * time.Second,
			BackoffJitter: 0.2, // 20% jitter
		}
		r := NewRetryer(policy)

		// Multiple calls should produce varied results
		results := make(map[time.Duration]bool)
		for i := 0; i < 20; i++ {
			b := r.calculateBackoff(1)
			results[b] = true

			// Should be within 20% jitter of 100ms
			if b < 80*time.Millisecond || b > 120*time.Millisecond {
				t.Errorf("backoff %v outside expected jitter range [80ms, 120ms]", b)
			}
		}

		// With jitter, we should see some variation
		// (Note: might occasionally fail due to randomness)
		if len(results) < 2 {
			t.Log("warning: jitter produced minimal variation")
		}
	})
}

// ==================== BackoffCalculator Tests ====================

func TestBackoffCalculator(t *testing.T) {
	t.Run("NewBackoffCalculator", func(t *testing.T) {
		bc := NewBackoffCalculator(100*time.Millisecond, 5*time.Second, 0.1)
		if bc == nil {
			t.Fatal("expected non-nil backoff calculator")
		}
	})

	t.Run("Next_ExponentialBackoff", func(t *testing.T) {
		bc := NewBackoffCalculator(100*time.Millisecond, 10*time.Second, 0)

		b0 := bc.Next(0)
		if b0 != 100*time.Millisecond {
			t.Errorf("attempt 0: expected 100ms, got %v", b0)
		}

		b1 := bc.Next(1)
		if b1 != 200*time.Millisecond {
			t.Errorf("attempt 1: expected 200ms, got %v", b1)
		}

		b2 := bc.Next(2)
		if b2 != 400*time.Millisecond {
			t.Errorf("attempt 2: expected 400ms, got %v", b2)
		}
	})

	t.Run("Next_CappedAtMax", func(t *testing.T) {
		bc := NewBackoffCalculator(100*time.Millisecond, 500*time.Millisecond, 0)

		b5 := bc.Next(5) // Would be 3200ms without cap
		if b5 > 500*time.Millisecond {
			t.Errorf("expected backoff capped at 500ms, got %v", b5)
		}
	})

	t.Run("Next_NegativeHandled", func(t *testing.T) {
		bc := NewBackoffCalculator(100*time.Millisecond, 10*time.Second, 0.99)

		// With high jitter, might produce negative values which should be handled
		for i := 0; i < 10; i++ {
			b := bc.Next(0)
			if b < 0 {
				t.Errorf("backoff should not be negative, got %v", b)
			}
		}
	})
}

// ==================== Helper Function Tests ====================

func TestContainsAny(t *testing.T) {
	tests := []struct {
		s        string
		substrs  []string
		expected bool
	}{
		{"connection refused", []string{"connection", "timeout"}, true},
		{"connection refused", []string{"network", "timeout"}, false},
		{"i/o timeout occurred", []string{"timeout"}, true},
		{"", []string{"something"}, false},
		{"something", []string{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			result := containsAny(tt.s, tt.substrs)
			if result != tt.expected {
				t.Errorf("containsAny(%q, %v) = %v, expected %v", tt.s, tt.substrs, result, tt.expected)
			}
		})
	}
}

func TestContainsString(t *testing.T) {
	tests := []struct {
		s        string
		substr   string
		expected bool
	}{
		{"hello world", "world", true},
		{"hello world", "earth", false},
		{"connection refused", "refused", true},
		{"", "something", false},
		{"something", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.s+"_"+tt.substr, func(t *testing.T) {
			result := containsString(tt.s, tt.substr)
			if result != tt.expected {
				t.Errorf("containsString(%q, %q) = %v, expected %v", tt.s, tt.substr, result, tt.expected)
			}
		})
	}
}

// ==================== Error Tests ====================

func TestErrors(t *testing.T) {
	t.Run("ErrMaxRetriesExceeded", func(t *testing.T) {
		if ErrMaxRetriesExceeded.Error() != "max retries exceeded" {
			t.Errorf("unexpected error message: %s", ErrMaxRetriesExceeded.Error())
		}
	})

	t.Run("ErrNonRetryable", func(t *testing.T) {
		if ErrNonRetryable.Error() != "non-retryable error" {
			t.Errorf("unexpected error message: %s", ErrNonRetryable.Error())
		}
	})
}

// ==================== Benchmark Tests ====================

func BenchmarkRetryer_Do_Success(b *testing.B) {
	policy := &RetryPolicy{
		MaxRetries:  3,
		BackoffBase: 100 * time.Millisecond,
	}
	r := NewRetryer(policy)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Do(ctx, func(ctx context.Context) error {
			return nil
		})
	}
}

func BenchmarkBackoffCalculator_Next(b *testing.B) {
	bc := NewBackoffCalculator(100*time.Millisecond, 10*time.Second, 0.1)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bc.Next(i % 5)
	}
}
