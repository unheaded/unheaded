// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package worker provides a worker pool and job queue system for the Unheaded Kingdom.
package worker

import (
	"context"
	"sync"
	"time"
)

// Priority represents job priority levels.
type Priority int

const (
	// PriorityLow is for background tasks.
	PriorityLow Priority = iota
	// PriorityNormal is the default priority.
	PriorityNormal
	// PriorityHigh is for urgent tasks.
	PriorityHigh
	// PriorityCritical is for system-critical tasks.
	PriorityCritical
)

// String returns the string representation of a priority.
func (p Priority) String() string {
	switch p {
	case PriorityLow:
		return "low"
	case PriorityNormal:
		return "normal"
	case PriorityHigh:
		return "high"
	case PriorityCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// JobStatus represents the current state of a job.
type JobStatus int

const (
	// JobStatusPending means the job is waiting to be executed.
	JobStatusPending JobStatus = iota
	// JobStatusRunning means the job is currently executing.
	JobStatusRunning
	// JobStatusCompleted means the job finished successfully.
	JobStatusCompleted
	// JobStatusFailed means the job failed after all retries.
	JobStatusFailed
	// JobStatusCanceled means the job was canceled.
	JobStatusCanceled
	// JobStatusTimeout means the job exceeded its timeout.
	JobStatusTimeout
)

// String returns the string representation of a job status.
func (s JobStatus) String() string {
	switch s {
	case JobStatusPending:
		return "pending"
	case JobStatusRunning:
		return "running"
	case JobStatusCompleted:
		return "completed"
	case JobStatusFailed:
		return "failed"
	case JobStatusCanceled:
		return "canceled"
	case JobStatusTimeout:
		return "timeout"
	default:
		return "unknown"
	}
}

// JobFunc is the function signature for executable jobs.
type JobFunc func(ctx context.Context) (interface{}, error)

// Job represents a unit of work to be executed by the worker pool.
type Job struct {
	// ID is a unique identifier for the job.
	ID string
	// Name is a human-readable name for the job.
	Name string
	// Func is the function to execute.
	Func JobFunc
	// Priority determines execution order.
	Priority Priority
	// Timeout is the maximum execution time (0 means no timeout).
	Timeout time.Duration
	// MaxRetries is the maximum number of retry attempts.
	MaxRetries int
	// RetryDelay is the initial delay between retries.
	RetryDelay time.Duration
	// CreatedAt is when the job was created.
	CreatedAt time.Time
	// Metadata holds additional job information.
	Metadata map[string]interface{}

	// Internal fields
	attempts  int
	status    JobStatus
	lastError error
	mu        sync.RWMutex
}

// NewJob creates a new job with the given function.
func NewJob(id string, fn JobFunc) *Job {
	return &Job{
		ID:         id,
		Func:       fn,
		Priority:   PriorityNormal,
		CreatedAt:  time.Now(),
		Metadata:   make(map[string]interface{}),
		MaxRetries: 0,
		RetryDelay: time.Second,
	}
}

// WithName sets the job name.
func (j *Job) WithName(name string) *Job {
	j.Name = name
	return j
}

// WithPriority sets the job priority.
func (j *Job) WithPriority(p Priority) *Job {
	j.Priority = p
	return j
}

// WithTimeout sets the job timeout.
func (j *Job) WithTimeout(d time.Duration) *Job {
	j.Timeout = d
	return j
}

// WithRetry sets the retry configuration.
func (j *Job) WithRetry(maxRetries int, delay time.Duration) *Job {
	j.MaxRetries = maxRetries
	j.RetryDelay = delay
	return j
}

// WithMetadata adds metadata to the job.
func (j *Job) WithMetadata(key string, value interface{}) *Job {
	j.Metadata[key] = value
	return j
}

// Status returns the current job status.
func (j *Job) Status() JobStatus {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.status
}

// SetStatus updates the job status.
func (j *Job) SetStatus(s JobStatus) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.status = s
}

// Attempts returns the number of execution attempts.
func (j *Job) Attempts() int {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.attempts
}

// IncrementAttempts increments the attempt counter.
func (j *Job) IncrementAttempts() int {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.attempts++
	return j.attempts
}

// LastError returns the last error encountered.
func (j *Job) LastError() error {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.lastError
}

// SetLastError sets the last error.
func (j *Job) SetLastError(err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.lastError = err
}

// CanRetry returns true if the job can be retried.
func (j *Job) CanRetry() bool {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.attempts <= j.MaxRetries
}

// ScheduledJob represents a job that runs on a schedule.
type ScheduledJob struct {
	*Job
	// Schedule is the cron expression.
	Schedule string
	// NextRun is the next scheduled execution time.
	NextRun time.Time
	// LastRun is the last execution time.
	LastRun time.Time
}

// DeadLetterEntry represents a failed job in the dead letter queue.
type DeadLetterEntry struct {
	Job       *Job
	Error     error
	FailedAt  time.Time
	Attempts  int
	LastRetry time.Time
}
