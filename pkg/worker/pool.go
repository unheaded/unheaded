// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package worker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

var (
	// ErrPoolClosed is returned when submitting to a closed pool.
	ErrPoolClosed = errors.New("worker pool is closed")
	// ErrPoolNotStarted is returned when the pool hasn't been started.
	ErrPoolNotStarted = errors.New("worker pool not started")
)

// PoolConfig holds configuration for the worker pool.
type PoolConfig struct {
	// Workers is the number of worker goroutines.
	Workers int
	// QueueSize is the maximum number of jobs in the queue.
	QueueSize int
	// RetryConfig is the default retry configuration.
	RetryConfig *RetryConfig
	// DeadLetterCapacity is the max size of the dead letter queue.
	DeadLetterCapacity int
	// GracefulTimeout is the maximum time to wait for graceful shutdown.
	GracefulTimeout time.Duration
}

// DefaultPoolConfig returns sensible defaults for the pool configuration.
func DefaultPoolConfig() *PoolConfig {
	return &PoolConfig{
		Workers:            4,
		QueueSize:          1000,
		RetryConfig:        DefaultRetryConfig(),
		DeadLetterCapacity: 100,
		GracefulTimeout:    30 * time.Second,
	}
}

// PoolOption is a function that configures the pool.
type PoolOption func(*PoolConfig)

// WithWorkers sets the number of workers.
func WithWorkers(n int) PoolOption {
	return func(c *PoolConfig) {
		if n > 0 {
			c.Workers = n
		}
	}
}

// WithQueueSize sets the queue capacity.
func WithQueueSize(size int) PoolOption {
	return func(c *PoolConfig) {
		if size > 0 {
			c.QueueSize = size
		}
	}
}

// WithRetry sets the retry configuration.
func WithRetry(maxRetries int, initialDelay time.Duration) PoolOption {
	return func(c *PoolConfig) {
		c.RetryConfig.MaxRetries = maxRetries
		c.RetryConfig.InitialDelay = initialDelay
	}
}

// WithRetryConfig sets a custom retry configuration.
func WithRetryConfig(config *RetryConfig) PoolOption {
	return func(c *PoolConfig) {
		c.RetryConfig = config
	}
}

// WithDeadLetterCapacity sets the dead letter queue capacity.
func WithDeadLetterCapacity(capacity int) PoolOption {
	return func(c *PoolConfig) {
		c.DeadLetterCapacity = capacity
	}
}

// WithGracefulTimeout sets the graceful shutdown timeout.
func WithGracefulTimeout(timeout time.Duration) PoolOption {
	return func(c *PoolConfig) {
		c.GracefulTimeout = timeout
	}
}

// PoolStats holds statistics about the pool.
type PoolStats struct {
	// TotalSubmitted is the total number of jobs submitted.
	TotalSubmitted uint64
	// TotalCompleted is the total number of jobs completed.
	TotalCompleted uint64
	// TotalFailed is the total number of jobs that failed.
	TotalFailed uint64
	// QueueSize is the current queue size.
	QueueSize int
	// ActiveWorkers is the number of workers currently processing.
	ActiveWorkers int
	// DeadLetterSize is the size of the dead letter queue.
	DeadLetterSize int
}

// Pool is a worker pool for executing jobs concurrently.
type Pool struct {
	config        *PoolConfig
	queue         *InMemoryQueue
	deadLetter    *DeadLetterQueue
	retryExecutor *RetryExecutor
	scheduler     *Scheduler
	futures       map[string]*Future
	futuresMu     sync.RWMutex

	workers       int
	running       bool
	stopCh        chan struct{}
	jobCh         chan *jobExecution
	wg            sync.WaitGroup
	mu            sync.RWMutex
	activeWorkers int32

	// Stats
	totalSubmitted uint64
	totalCompleted uint64
	totalFailed    uint64

	// jobCounter generates unique job IDs for SubmitFunc.
	jobCounter uint64
}

// jobExecution wraps a job with its future for worker execution.
type jobExecution struct {
	job    *Job
	future *Future
}

// NewPool creates a new worker pool with the given options.
func NewPool(opts ...PoolOption) *Pool {
	config := DefaultPoolConfig()
	for _, opt := range opts {
		opt(config)
	}

	p := &Pool{
		config:        config,
		queue:         NewInMemoryQueue(config.QueueSize),
		deadLetter:    NewDeadLetterQueue(config.DeadLetterCapacity),
		retryExecutor: NewRetryExecutor(config.RetryConfig),
		futures:       make(map[string]*Future),
		workers:       config.Workers,
		stopCh:        make(chan struct{}),
		jobCh:         make(chan *jobExecution, config.Workers*2),
	}

	p.scheduler = NewScheduler(p.Submit)

	return p
}

// Start starts the worker pool.
func (p *Pool) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return nil
	}

	p.running = true
	p.stopCh = make(chan struct{})

	// Start workers
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}

	// Start dispatcher
	p.wg.Add(1)
	go p.dispatcher()

	// Start scheduler
	p.scheduler.Start()

	return nil
}

// worker is the main worker loop.
func (p *Pool) worker(id int) {
	defer p.wg.Done()

	for {
		select {
		case <-p.stopCh:
			return
		case exec, ok := <-p.jobCh:
			if !ok {
				return
			}
			p.executeJob(exec)
		}
	}
}

// dispatcher fetches jobs from the queue and sends them to workers.
func (p *Pool) dispatcher() {
	defer p.wg.Done()

	for {
		select {
		case <-p.stopCh:
			return
		default:
			job, err := p.queue.DequeueWait()
			if err != nil {
				if errors.Is(err, ErrQueueClosed) {
					return
				}
				continue
			}

			p.futuresMu.RLock()
			future, ok := p.futures[job.ID]
			p.futuresMu.RUnlock()

			if !ok {
				// Create future if not found (shouldn't happen normally)
				future = NewFuture(job.ID)
				p.futuresMu.Lock()
				p.futures[job.ID] = future
				p.futuresMu.Unlock()
			}

			exec := &jobExecution{
				job:    job,
				future: future,
			}

			select {
			case p.jobCh <- exec:
			case <-p.stopCh:
				return
			}
		}
	}
}

// executeJob executes a single job with retries.
func (p *Pool) executeJob(exec *jobExecution) {
	atomic.AddInt32(&p.activeWorkers, 1)
	defer atomic.AddInt32(&p.activeWorkers, -1)

	// Create context for job execution
	ctx := context.Background()

	// Execute with retry
	result := p.retryExecutor.ExecuteJob(ctx, exec.job)

	// Update stats
	if result.Success() {
		atomic.AddUint64(&p.totalCompleted, 1)
	} else {
		atomic.AddUint64(&p.totalFailed, 1)
		// Add to dead letter queue
		p.deadLetter.Add(exec.job, result.Error)
	}

	// Complete the future
	exec.future.Complete(result)

	// Clean up future from map
	p.futuresMu.Lock()
	delete(p.futures, exec.job.ID)
	p.futuresMu.Unlock()
}

// Submit submits a job function for execution.
func (p *Pool) Submit(job *Job) *Future {
	p.mu.RLock()
	if !p.running {
		p.mu.RUnlock()
		future := NewFuture(job.ID)
		future.Complete(&Result{
			JobID:       job.ID,
			Error:       ErrPoolNotStarted,
			Status:      JobStatusFailed,
			CompletedAt: time.Now(),
		})
		return future
	}
	p.mu.RUnlock()

	// Create future
	future := NewFuture(job.ID)

	// Store future
	p.futuresMu.Lock()
	p.futures[job.ID] = future
	p.futuresMu.Unlock()

	// Apply default retry from pool config if job doesn't have its own
	if job.MaxRetries == 0 && p.config.RetryConfig.MaxRetries > 0 {
		job.MaxRetries = p.config.RetryConfig.MaxRetries
		job.RetryDelay = p.config.RetryConfig.InitialDelay
	}

	// Enqueue job
	if err := p.queue.Enqueue(job); err != nil {
		p.futuresMu.Lock()
		delete(p.futures, job.ID)
		p.futuresMu.Unlock()

		future.Complete(&Result{
			JobID:       job.ID,
			Error:       err,
			Status:      JobStatusFailed,
			CompletedAt: time.Now(),
		})
		return future
	}

	atomic.AddUint64(&p.totalSubmitted, 1)
	return future
}

// SubmitFunc is a convenience method to submit a function as a job.
func (p *Pool) SubmitFunc(fn JobFunc) *Future {
	id := atomic.AddUint64(&p.jobCounter, 1)
	job := NewJob(fmt.Sprintf("job-%d", id), fn)
	return p.Submit(job)
}

// SubmitWithPriority submits a job with a specific priority.
func (p *Pool) SubmitWithPriority(job *Job, priority Priority) *Future {
	job.Priority = priority
	return p.Submit(job)
}

// Schedule adds a recurring job with a cron expression.
func (p *Pool) Schedule(cronExpr string, job *Job) (string, error) {
	return p.scheduler.AddJob(cronExpr, job)
}

// ScheduleFunc schedules a function with a cron expression.
func (p *Pool) ScheduleFunc(cronExpr string, fn JobFunc) (string, error) {
	return p.scheduler.AddFunc(cronExpr, fn)
}

// Unschedule removes a scheduled job.
func (p *Pool) Unschedule(id string) bool {
	return p.scheduler.RemoveJob(id)
}

// Shutdown gracefully shuts down the pool.
func (p *Pool) Shutdown(ctx context.Context) error {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return nil
	}
	p.running = false
	p.mu.Unlock()

	// Stop accepting new jobs
	p.queue.Close()

	// Stop scheduler
	p.scheduler.Stop()

	// Signal workers to stop
	close(p.stopCh)

	// Wait for completion with timeout
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ShutdownNow immediately shuts down the pool without waiting.
func (p *Pool) ShutdownNow() {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return
	}
	p.running = false
	p.mu.Unlock()

	p.queue.Close()
	p.scheduler.Stop()
	close(p.stopCh)

	// Cancel all pending futures
	p.futuresMu.Lock()
	for _, future := range p.futures {
		future.Cancel()
	}
	p.futures = make(map[string]*Future)
	p.futuresMu.Unlock()
}

// Stats returns current pool statistics.
func (p *Pool) Stats() PoolStats {
	return PoolStats{
		TotalSubmitted: atomic.LoadUint64(&p.totalSubmitted),
		TotalCompleted: atomic.LoadUint64(&p.totalCompleted),
		TotalFailed:    atomic.LoadUint64(&p.totalFailed),
		QueueSize:      p.queue.Len(),
		ActiveWorkers:  int(atomic.LoadInt32(&p.activeWorkers)),
		DeadLetterSize: p.deadLetter.Len(),
	}
}

// DeadLetterQueue returns the dead letter queue.
func (p *Pool) DeadLetterQueue() *DeadLetterQueue {
	return p.deadLetter
}

// ResubmitDeadLetter moves a job from dead letter queue back to main queue.
func (p *Pool) ResubmitDeadLetter(jobID string) (*Future, error) {
	entry := p.deadLetter.Remove(jobID)
	if entry == nil {
		return nil, errors.New("job not found in dead letter queue")
	}

	// Reset job state
	entry.Job.SetStatus(JobStatusPending)
	entry.Job.SetLastError(nil)

	return p.Submit(entry.Job), nil
}

// IsRunning returns true if the pool is running.
func (p *Pool) IsRunning() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.running
}

// QueueSize returns the current queue size.
func (p *Pool) QueueSize() int {
	return p.queue.Len()
}

// WorkerCount returns the number of workers.
func (p *Pool) WorkerCount() int {
	return p.workers
}

// ActiveWorkerCount returns the number of workers currently processing jobs.
func (p *Pool) ActiveWorkerCount() int {
	return int(atomic.LoadInt32(&p.activeWorkers))
}

// PendingFutures returns the number of futures currently tracked by the pool.
func (p *Pool) PendingFutures() int {
	p.futuresMu.RLock()
	defer p.futuresMu.RUnlock()
	return len(p.futures)
}

// Resize changes the number of workers (only when stopped).
func (p *Pool) Resize(workers int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return errors.New("cannot resize while running")
	}

	if workers <= 0 {
		return errors.New("workers must be positive")
	}

	p.workers = workers
	p.config.Workers = workers
	return nil
}

// ScheduledJobs returns all scheduled job entries.
func (p *Pool) ScheduledJobs() []*ScheduledEntry {
	return p.scheduler.Entries()
}
