// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package worker

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestJobLastError(t *testing.T) {
	job := NewJob("test", func(ctx context.Context) (any, error) {
		return nil, nil
	})

	if job.LastError() != nil {
		t.Error("expected nil initial error")
	}

	testErr := errors.New("test error")
	job.SetLastError(testErr)

	if job.LastError() != testErr {
		t.Errorf("LastError = %v, want %v", job.LastError(), testErr)
	}

	job.SetLastError(nil)
	if job.LastError() != nil {
		t.Error("expected nil after clearing error")
	}
}

func TestJobCanRetry(t *testing.T) {
	job := NewJob("test", func(ctx context.Context) (any, error) {
		return nil, nil
	})
	job.MaxRetries = 2

	if !job.CanRetry() {
		t.Error("expected CanRetry true with 0 attempts")
	}

	job.IncrementAttempts() // 1
	if !job.CanRetry() {
		t.Error("expected CanRetry true with 1 attempt")
	}

	job.IncrementAttempts() // 2
	if !job.CanRetry() {
		t.Error("expected CanRetry true with 2 attempts (== MaxRetries)")
	}

	job.IncrementAttempts() // 3
	if job.CanRetry() {
		t.Error("expected CanRetry false with 3 attempts (> MaxRetries)")
	}
}

func TestWithRetryConfig(t *testing.T) {
	rc := &RetryConfig{
		MaxRetries:   5,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     10 * time.Second,
		Multiplier:   3.0,
	}

	pool := NewPool(WithWorkers(2), WithRetryConfig(rc))
	if pool.config.RetryConfig.MaxRetries != 5 {
		t.Errorf("MaxRetries = %d, want 5", pool.config.RetryConfig.MaxRetries)
	}
	if pool.config.RetryConfig.Multiplier != 3.0 {
		t.Errorf("Multiplier = %f, want 3.0", pool.config.RetryConfig.Multiplier)
	}
}

func TestWithGracefulTimeout(t *testing.T) {
	pool := NewPool(WithWorkers(2), WithGracefulTimeout(30*time.Second))
	if pool.config.GracefulTimeout != 30*time.Second {
		t.Errorf("GracefulTimeout = %v, want 30s", pool.config.GracefulTimeout)
	}
}

func TestPoolSubmitWithPriority(t *testing.T) {
	pool := NewPool(WithWorkers(2))
	pool.Start()
	defer pool.Shutdown(context.Background())

	job := NewJob("priority-job", func(ctx context.Context) (any, error) {
		return "high-priority-result", nil
	})

	future := pool.SubmitWithPriority(job, PriorityHigh)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := future.Get(ctx)
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}

	if result.Error != nil {
		t.Fatalf("result error: %v", result.Error)
	}
	if result.Value != "high-priority-result" {
		t.Errorf("value = %v, want high-priority-result", result.Value)
	}
	if job.Priority != PriorityHigh {
		t.Errorf("priority = %v, want High", job.Priority)
	}
}

func TestPoolScheduleFunc(t *testing.T) {
	pool := NewPool(WithWorkers(2))
	pool.Start()
	defer pool.Shutdown(context.Background())

	called := make(chan struct{}, 1)
	id, err := pool.ScheduleFunc("* * * * *", func(ctx context.Context) (any, error) {
		select {
		case called <- struct{}{}:
		default:
		}
		return nil, nil
	})

	if err != nil {
		t.Fatalf("ScheduleFunc: %v", err)
	}
	if id == "" {
		t.Error("expected non-empty schedule ID")
	}

	// Unschedule
	if !pool.Unschedule(id) {
		t.Error("Unschedule returned false")
	}
	if pool.Unschedule(id) {
		t.Error("double Unschedule should return false")
	}
}

func TestPoolShutdownNow(t *testing.T) {
	pool := NewPool(WithWorkers(1))
	pool.Start()

	// Submit a slow job
	pool.Submit(NewJob("slow", func(ctx context.Context) (any, error) {
		time.Sleep(10 * time.Second)
		return nil, nil
	}))

	// Give it a moment to start
	time.Sleep(50 * time.Millisecond)

	// ShutdownNow should return immediately
	pool.ShutdownNow()

	if pool.IsRunning() {
		t.Error("pool should not be running after ShutdownNow")
	}
}

func TestPoolShutdownNow_NotRunning(t *testing.T) {
	pool := NewPool(WithWorkers(2))
	// ShutdownNow on a pool that was never started should be safe
	pool.ShutdownNow()
	if pool.IsRunning() {
		t.Error("pool should not be running")
	}
}

func TestPoolQueueSize(t *testing.T) {
	pool := NewPool(WithWorkers(2))
	// Not started, so jobs will queue but not be processed
	if pool.QueueSize() != 0 {
		t.Errorf("initial QueueSize = %d, want 0", pool.QueueSize())
	}
}

func TestPoolActiveWorkerCount(t *testing.T) {
	pool := NewPool(WithWorkers(2))
	if pool.ActiveWorkerCount() != 0 {
		t.Errorf("initial ActiveWorkerCount = %d, want 0", pool.ActiveWorkerCount())
	}
}

func TestPoolResize(t *testing.T) {
	pool := NewPool(WithWorkers(2))

	// Resize when not running should work
	if err := pool.Resize(4); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	if pool.WorkerCount() != 4 {
		t.Errorf("WorkerCount = %d, want 4", pool.WorkerCount())
	}

	// Invalid size
	if err := pool.Resize(0); err == nil {
		t.Error("expected error for zero workers")
	}
	if err := pool.Resize(-1); err == nil {
		t.Error("expected error for negative workers")
	}

	// Resize while running should fail
	pool.Start()
	defer pool.Shutdown(context.Background())

	if err := pool.Resize(8); err == nil {
		t.Error("expected error when resizing running pool")
	}
}

func TestJobStatusString(t *testing.T) {
	tests := []struct {
		status   JobStatus
		expected string
	}{
		{JobStatusPending, "pending"},
		{JobStatusRunning, "running"},
		{JobStatusCompleted, "completed"},
		{JobStatusFailed, "failed"},
		{JobStatus(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.status.String(); got != tt.expected {
			t.Errorf("JobStatus(%d).String() = %q, want %q", tt.status, got, tt.expected)
		}
	}
}

func TestPriorityString_Extra(t *testing.T) {
	// Test unknown priority
	p := Priority(99)
	s := p.String()
	if s != "unknown" {
		t.Errorf("Priority(99).String() = %q, want 'unknown'", s)
	}
}
