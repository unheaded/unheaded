// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package worker

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	// ErrFutureCanceled is returned when a future is canceled.
	ErrFutureCanceled = errors.New("future was canceled")
	// ErrFutureTimeout is returned when waiting for a future times out.
	ErrFutureTimeout = errors.New("future timed out")
)

// Result represents the outcome of a job execution.
type Result struct {
	// JobID is the ID of the job that produced this result.
	JobID string
	// Value is the return value from the job.
	Value interface{}
	// Error is any error that occurred during execution.
	Error error
	// Status is the final job status.
	Status JobStatus
	// Duration is how long the job took to execute.
	Duration time.Duration
	// Attempts is the number of execution attempts.
	Attempts int
	// CompletedAt is when the job completed.
	CompletedAt time.Time
}

// Success returns true if the job completed successfully.
func (r *Result) Success() bool {
	return r.Error == nil && r.Status == JobStatusCompleted
}

// Future represents a pending result that will be available in the future.
type Future struct {
	jobID    string
	result   *Result
	done     chan struct{}
	once     sync.Once
	canceled bool
	mu       sync.RWMutex
}

// NewFuture creates a new future for the given job ID.
func NewFuture(jobID string) *Future {
	return &Future{
		jobID: jobID,
		done:  make(chan struct{}),
	}
}

// JobID returns the ID of the job this future is tracking.
func (f *Future) JobID() string {
	return f.jobID
}

// Get waits for the result and returns it.
// The context can be used to cancel the wait.
func (f *Future) Get(ctx context.Context) (*Result, error) {
	select {
	case <-f.done:
		f.mu.RLock()
		defer f.mu.RUnlock()
		if f.canceled {
			return nil, ErrFutureCanceled
		}
		return f.result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// GetWithTimeout waits for the result with a timeout.
func (f *Future) GetWithTimeout(timeout time.Duration) (*Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return f.Get(ctx)
}

// TryGet attempts to get the result without blocking.
// Returns nil, false if the result is not yet available.
func (f *Future) TryGet() (*Result, bool) {
	select {
	case <-f.done:
		f.mu.RLock()
		defer f.mu.RUnlock()
		if f.canceled {
			return nil, false
		}
		return f.result, true
	default:
		return nil, false
	}
}

// Done returns a channel that is closed when the result is available.
func (f *Future) Done() <-chan struct{} {
	return f.done
}

// IsDone returns true if the result is available.
func (f *Future) IsDone() bool {
	select {
	case <-f.done:
		return true
	default:
		return false
	}
}

// Complete sets the result and signals that the future is done.
func (f *Future) Complete(result *Result) {
	f.once.Do(func() {
		f.mu.Lock()
		f.result = result
		f.mu.Unlock()
		close(f.done)
	})
}

// Cancel cancels the future.
func (f *Future) Cancel() {
	f.once.Do(func() {
		f.mu.Lock()
		f.canceled = true
		f.result = &Result{
			JobID:       f.jobID,
			Status:      JobStatusCanceled,
			Error:       ErrFutureCanceled,
			CompletedAt: time.Now(),
		}
		f.mu.Unlock()
		close(f.done)
	})
}

// FutureGroup manages multiple futures and waits for all to complete.
type FutureGroup struct {
	futures []*Future
	mu      sync.Mutex
}

// NewFutureGroup creates a new future group.
func NewFutureGroup() *FutureGroup {
	return &FutureGroup{
		futures: make([]*Future, 0),
	}
}

// Add adds a future to the group.
func (fg *FutureGroup) Add(f *Future) {
	fg.mu.Lock()
	defer fg.mu.Unlock()
	fg.futures = append(fg.futures, f)
}

// Wait waits for all futures in the group to complete.
func (fg *FutureGroup) Wait(ctx context.Context) ([]*Result, error) {
	fg.mu.Lock()
	futures := make([]*Future, len(fg.futures))
	copy(futures, fg.futures)
	fg.mu.Unlock()

	results := make([]*Result, len(futures))
	for i, f := range futures {
		result, err := f.Get(ctx)
		if err != nil {
			return results, err
		}
		results[i] = result
	}
	return results, nil
}

// WaitAny waits for any future in the group to complete.
func (fg *FutureGroup) WaitAny(ctx context.Context) (*Result, error) {
	fg.mu.Lock()
	futures := make([]*Future, len(fg.futures))
	copy(futures, fg.futures)
	fg.mu.Unlock()

	if len(futures) == 0 {
		return nil, nil
	}

	// Create a combined done channel
	done := make(chan *Result, 1)
	for _, f := range futures {
		go func(fut *Future) {
			select {
			case <-fut.Done():
				result, _ := fut.TryGet()
				select {
				case done <- result:
				default:
				}
			case <-ctx.Done():
			}
		}(f)
	}

	select {
	case result := <-done:
		return result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Size returns the number of futures in the group.
func (fg *FutureGroup) Size() int {
	fg.mu.Lock()
	defer fg.mu.Unlock()
	return len(fg.futures)
}
