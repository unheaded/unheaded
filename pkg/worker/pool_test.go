// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package worker

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewPool(t *testing.T) {
	pool := NewPool(
		WithWorkers(10),
		WithQueueSize(1000),
		WithRetry(3, time.Second),
	)

	if pool.WorkerCount() != 10 {
		t.Errorf("expected 10 workers, got %d", pool.WorkerCount())
	}

	if pool.config.QueueSize != 1000 {
		t.Errorf("expected queue size 1000, got %d", pool.config.QueueSize)
	}

	if pool.config.RetryConfig.MaxRetries != 3 {
		t.Errorf("expected 3 retries, got %d", pool.config.RetryConfig.MaxRetries)
	}
}

func TestPoolStartStop(t *testing.T) {
	pool := NewPool(WithWorkers(2))

	if pool.IsRunning() {
		t.Error("pool should not be running initially")
	}

	if err := pool.Start(); err != nil {
		t.Fatalf("failed to start pool: %v", err)
	}

	if !pool.IsRunning() {
		t.Error("pool should be running after start")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := pool.Shutdown(ctx); err != nil {
		t.Fatalf("failed to shutdown pool: %v", err)
	}

	if pool.IsRunning() {
		t.Error("pool should not be running after shutdown")
	}
}

func TestPoolSubmit(t *testing.T) {
	pool := NewPool(WithWorkers(2))
	pool.Start()
	defer pool.Shutdown(context.Background())

	executed := make(chan bool, 1)
	job := NewJob("test-job", func(ctx context.Context) (interface{}, error) {
		executed <- true
		return "success", nil
	})

	future := pool.Submit(job)

	select {
	case <-executed:
		// Job executed
	case <-time.After(2 * time.Second):
		t.Fatal("job was not executed")
	}

	result, err := future.GetWithTimeout(time.Second)
	if err != nil {
		t.Fatalf("failed to get result: %v", err)
	}

	if result.Value != "success" {
		t.Errorf("expected 'success', got %v", result.Value)
	}

	if result.Status != JobStatusCompleted {
		t.Errorf("expected completed status, got %v", result.Status)
	}
}

func TestPoolSubmitFunc(t *testing.T) {
	pool := NewPool(WithWorkers(2))
	pool.Start()
	defer pool.Shutdown(context.Background())

	future := pool.SubmitFunc(func(ctx context.Context) (interface{}, error) {
		return 42, nil
	})

	result, err := future.GetWithTimeout(time.Second)
	if err != nil {
		t.Fatalf("failed to get result: %v", err)
	}

	if result.Value != 42 {
		t.Errorf("expected 42, got %v", result.Value)
	}
}

func TestPoolPriority(t *testing.T) {
	pool := NewPool(WithWorkers(1), WithQueueSize(100))
	// Don't start the pool yet - queue jobs first

	order := make([]string, 0, 3)
	var mu sync.Mutex

	lowJob := NewJob("low", func(ctx context.Context) (interface{}, error) {
		mu.Lock()
		order = append(order, "low")
		mu.Unlock()
		return nil, nil
	}).WithPriority(PriorityLow)

	highJob := NewJob("high", func(ctx context.Context) (interface{}, error) {
		mu.Lock()
		order = append(order, "high")
		mu.Unlock()
		return nil, nil
	}).WithPriority(PriorityHigh)

	normalJob := NewJob("normal", func(ctx context.Context) (interface{}, error) {
		mu.Lock()
		order = append(order, "normal")
		mu.Unlock()
		return nil, nil
	}).WithPriority(PriorityNormal)

	// Queue jobs in order: low, high, normal
	pool.queue.Enqueue(lowJob)
	pool.queue.Enqueue(highJob)
	pool.queue.Enqueue(normalJob)

	// Now start and let them execute
	pool.Start()
	defer pool.Shutdown(context.Background())

	// Wait for jobs to complete
	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(order) != 3 {
		t.Fatalf("expected 3 jobs, got %d", len(order))
	}

	// High priority should execute first
	if order[0] != "high" {
		t.Errorf("expected high priority first, got %s", order[0])
	}
}

func TestPoolJobTimeout(t *testing.T) {
	pool := NewPool(WithWorkers(2))
	pool.Start()
	defer pool.Shutdown(context.Background())

	job := NewJob("timeout-job", func(ctx context.Context) (interface{}, error) {
		select {
		case <-time.After(5 * time.Second):
			return "completed", nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}).WithTimeout(100 * time.Millisecond)

	future := pool.Submit(job)

	result, err := future.GetWithTimeout(2 * time.Second)
	if err != nil {
		t.Fatalf("failed to get result: %v", err)
	}

	if result.Status != JobStatusTimeout {
		t.Errorf("expected timeout status, got %v", result.Status)
	}
}

func TestPoolJobRetry(t *testing.T) {
	pool := NewPool(
		WithWorkers(2),
		WithRetry(2, 10*time.Millisecond),
	)
	pool.Start()
	defer pool.Shutdown(context.Background())

	attempts := int32(0)
	job := NewJob("retry-job", func(ctx context.Context) (interface{}, error) {
		count := atomic.AddInt32(&attempts, 1)
		if count < 3 {
			return nil, errors.New("temporary error")
		}
		return "success", nil
	})

	future := pool.Submit(job)

	result, err := future.GetWithTimeout(5 * time.Second)
	if err != nil {
		t.Fatalf("failed to get result: %v", err)
	}

	if result.Status != JobStatusCompleted {
		t.Errorf("expected completed status, got %v", result.Status)
	}

	if atomic.LoadInt32(&attempts) != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestPoolDeadLetterQueue(t *testing.T) {
	pool := NewPool(
		WithWorkers(2),
		WithRetry(1, 10*time.Millisecond),
		WithDeadLetterCapacity(10),
	)
	pool.Start()
	defer pool.Shutdown(context.Background())

	expectedErr := errors.New("permanent failure")
	job := NewJob("failing-job", func(ctx context.Context) (interface{}, error) {
		return nil, expectedErr
	})

	future := pool.Submit(job)

	result, err := future.GetWithTimeout(2 * time.Second)
	if err != nil {
		t.Fatalf("failed to get result: %v", err)
	}

	if result.Status != JobStatusFailed {
		t.Errorf("expected failed status, got %v", result.Status)
	}

	// Check dead letter queue
	dlq := pool.DeadLetterQueue()
	entries := dlq.Get()

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry in DLQ, got %d", len(entries))
	}

	if entries[0].Job.ID != "failing-job" {
		t.Errorf("expected job ID 'failing-job', got %s", entries[0].Job.ID)
	}
}

func TestPoolResubmitDeadLetter(t *testing.T) {
	pool := NewPool(
		WithWorkers(2),
		WithRetry(0, 10*time.Millisecond),
	)
	pool.Start()
	defer pool.Shutdown(context.Background())

	attempts := int32(0)
	job := NewJob("retry-dlq-job", func(ctx context.Context) (interface{}, error) {
		count := atomic.AddInt32(&attempts, 1)
		if count < 2 {
			return nil, errors.New("first failure")
		}
		return "success", nil
	})

	// First submission should fail
	future := pool.Submit(job)
	result, _ := future.GetWithTimeout(2 * time.Second)

	if result.Status != JobStatusFailed {
		t.Errorf("expected failed status, got %v", result.Status)
	}

	// Resubmit from dead letter queue
	newFuture, err := pool.ResubmitDeadLetter("retry-dlq-job")
	if err != nil {
		t.Fatalf("failed to resubmit: %v", err)
	}

	result, _ = newFuture.GetWithTimeout(2 * time.Second)
	if result.Status != JobStatusCompleted {
		t.Errorf("expected completed status on resubmit, got %v", result.Status)
	}
}

func TestPoolStats(t *testing.T) {
	pool := NewPool(WithWorkers(2))
	pool.Start()
	defer pool.Shutdown(context.Background())

	// Submit some jobs
	var futures []*Future
	for i := 0; i < 5; i++ {
		f := pool.SubmitFunc(func(ctx context.Context) (interface{}, error) {
			time.Sleep(10 * time.Millisecond)
			return nil, nil
		})
		futures = append(futures, f)
	}

	// Wait for completion
	for _, f := range futures {
		f.GetWithTimeout(2 * time.Second)
	}

	stats := pool.Stats()

	if stats.TotalSubmitted != 5 {
		t.Errorf("expected 5 submitted, got %d", stats.TotalSubmitted)
	}

	if stats.TotalCompleted != 5 {
		t.Errorf("expected 5 completed, got %d", stats.TotalCompleted)
	}

	if stats.TotalFailed != 0 {
		t.Errorf("expected 0 failed, got %d", stats.TotalFailed)
	}
}

func TestPoolConcurrency(t *testing.T) {
	pool := NewPool(WithWorkers(4), WithQueueSize(100))
	pool.Start()
	defer pool.Shutdown(context.Background())

	var maxConcurrent int32
	var currentConcurrent int32

	numJobs := 20
	var wg sync.WaitGroup
	wg.Add(numJobs)

	for i := 0; i < numJobs; i++ {
		pool.SubmitFunc(func(ctx context.Context) (interface{}, error) {
			defer wg.Done()
			current := atomic.AddInt32(&currentConcurrent, 1)

			// Track max concurrent
			for {
				max := atomic.LoadInt32(&maxConcurrent)
				if current <= max {
					break
				}
				if atomic.CompareAndSwapInt32(&maxConcurrent, max, current) {
					break
				}
			}

			time.Sleep(50 * time.Millisecond)
			atomic.AddInt32(&currentConcurrent, -1)
			return nil, nil
		})
	}

	wg.Wait()

	max := atomic.LoadInt32(&maxConcurrent)
	if max > 4 {
		t.Errorf("expected max 4 concurrent, got %d", max)
	}
	if max < 2 {
		t.Errorf("expected at least 2 concurrent (got %d), pool may not be utilizing workers", max)
	}
}

func TestPoolGracefulShutdown(t *testing.T) {
	pool := NewPool(WithWorkers(2))
	pool.Start()

	completed := int32(0)
	for i := 0; i < 5; i++ {
		pool.SubmitFunc(func(ctx context.Context) (interface{}, error) {
			time.Sleep(100 * time.Millisecond)
			atomic.AddInt32(&completed, 1)
			return nil, nil
		})
	}

	// Wait a bit for jobs to start
	time.Sleep(50 * time.Millisecond)

	// Shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := pool.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}

	// All running jobs should have completed
	count := atomic.LoadInt32(&completed)
	if count < 2 {
		t.Errorf("expected at least 2 completed during shutdown, got %d", count)
	}
}

func TestFuture(t *testing.T) {
	future := NewFuture("test")

	if future.IsDone() {
		t.Error("future should not be done initially")
	}

	// Try non-blocking get
	result, ok := future.TryGet()
	if ok {
		t.Error("TryGet should return false when not done")
	}
	if result != nil {
		t.Error("TryGet should return nil when not done")
	}

	// Complete the future
	expected := &Result{
		JobID:  "test",
		Value:  "hello",
		Status: JobStatusCompleted,
	}
	future.Complete(expected)

	if !future.IsDone() {
		t.Error("future should be done after complete")
	}

	// Get result
	result, err := future.Get(context.Background())
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if result.Value != "hello" {
		t.Errorf("expected 'hello', got %v", result.Value)
	}
}

func TestFutureCancel(t *testing.T) {
	future := NewFuture("test")
	future.Cancel()

	result, err := future.Get(context.Background())
	if err == nil {
		t.Error("expected error for canceled future")
	}

	if !errors.Is(err, ErrFutureCanceled) {
		t.Errorf("expected ErrFutureCanceled, got %v", err)
	}

	if result != nil {
		t.Error("result should be nil for canceled future")
	}
}

func TestFutureGroup(t *testing.T) {
	pool := NewPool(WithWorkers(4))
	pool.Start()
	defer pool.Shutdown(context.Background())

	group := NewFutureGroup()

	for i := 0; i < 5; i++ {
		val := i
		future := pool.SubmitFunc(func(ctx context.Context) (interface{}, error) {
			time.Sleep(10 * time.Millisecond)
			return val * 2, nil
		})
		group.Add(future)
	}

	if group.Size() != 5 {
		t.Errorf("expected group size 5, got %d", group.Size())
	}

	results, err := group.Wait(context.Background())
	if err != nil {
		t.Fatalf("Wait failed: %v", err)
	}

	if len(results) != 5 {
		t.Errorf("expected 5 results, got %d", len(results))
	}

	// Check all results are successful
	for i, result := range results {
		if result.Status != JobStatusCompleted {
			t.Errorf("result %d: expected completed, got %v", i, result.Status)
		}
	}
}

func TestCronParsing(t *testing.T) {
	tests := []struct {
		expr    string
		valid   bool
		testVal time.Time
		matches bool
	}{
		{"* * * * *", true, time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC), true},
		{"*/5 * * * *", true, time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC), true},
		{"*/5 * * * *", true, time.Date(2024, 1, 15, 10, 32, 0, 0, time.UTC), false},
		{"0 12 * * *", true, time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC), true},
		{"0 12 * * *", true, time.Date(2024, 1, 15, 13, 0, 0, 0, time.UTC), false},
		{"0 0 1 * *", true, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), true},
		{"1,15,30 * * * *", true, time.Date(2024, 1, 15, 10, 15, 0, 0, time.UTC), true},
		{"1-5 * * * *", true, time.Date(2024, 1, 15, 10, 3, 0, 0, time.UTC), true},
		{"invalid", false, time.Time{}, false},
		{"* * * *", false, time.Time{}, false}, // Only 4 fields
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			schedule, err := ParseCron(tt.expr)

			if tt.valid && err != nil {
				t.Fatalf("expected valid expression, got error: %v", err)
			}

			if !tt.valid && err == nil {
				t.Fatal("expected invalid expression to return error")
			}

			if !tt.valid {
				return
			}

			// Check if the test time matches the schedule
			minute := tt.testVal.Minute()
			hour := tt.testVal.Hour()
			day := tt.testVal.Day()
			month := int(tt.testVal.Month())
			weekday := int(tt.testVal.Weekday())

			matches := schedule.Minute.Contains(minute) &&
				schedule.Hour.Contains(hour) &&
				schedule.Day.Contains(day) &&
				schedule.Month.Contains(month) &&
				schedule.Weekday.Contains(weekday)

			if matches != tt.matches {
				t.Errorf("expected matches=%v, got %v", tt.matches, matches)
			}
		})
	}
}

func TestCronNext(t *testing.T) {
	schedule, err := ParseCron("0 12 * * *") // Every day at noon
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	from := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	next := schedule.Next(from)

	expected := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, next)
	}

	// Test when we're past the time today
	from = time.Date(2024, 1, 15, 14, 0, 0, 0, time.UTC)
	next = schedule.Next(from)

	expected = time.Date(2024, 1, 16, 12, 0, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, next)
	}
}

func TestScheduler(t *testing.T) {
	executed := int32(0)

	pool := NewPool(WithWorkers(2))
	pool.Start()
	defer pool.Shutdown(context.Background())

	// Create a scheduler that runs every second (for testing)
	// Note: In production, you'd use a proper cron expression
	job := NewJob("scheduled-job", func(ctx context.Context) (interface{}, error) {
		atomic.AddInt32(&executed, 1)
		return nil, nil
	})

	_, err := pool.Schedule("* * * * *", job)
	if err != nil {
		t.Fatalf("failed to schedule: %v", err)
	}

	entries := pool.ScheduledJobs()
	if len(entries) != 1 {
		t.Errorf("expected 1 scheduled job, got %d", len(entries))
	}
}

func TestInMemoryQueue(t *testing.T) {
	q := NewInMemoryQueue(10)

	// Test basic operations
	job1 := NewJob("job1", nil).WithPriority(PriorityNormal)
	job2 := NewJob("job2", nil).WithPriority(PriorityHigh)

	if err := q.Enqueue(job1); err != nil {
		t.Fatalf("failed to enqueue: %v", err)
	}

	if err := q.Enqueue(job2); err != nil {
		t.Fatalf("failed to enqueue: %v", err)
	}

	if q.Len() != 2 {
		t.Errorf("expected length 2, got %d", q.Len())
	}

	// High priority should come first
	job, err := q.Dequeue()
	if err != nil {
		t.Fatalf("failed to dequeue: %v", err)
	}

	if job.ID != "job2" {
		t.Errorf("expected job2 (high priority), got %s", job.ID)
	}
}

func TestQueueCapacity(t *testing.T) {
	q := NewInMemoryQueue(2)

	q.Enqueue(NewJob("job1", nil))
	q.Enqueue(NewJob("job2", nil))

	err := q.Enqueue(NewJob("job3", nil))
	if err == nil {
		t.Error("expected queue full error")
	}

	if !errors.Is(err, ErrQueueFull) {
		t.Errorf("expected ErrQueueFull, got %v", err)
	}
}

func TestDeadLetterQueue(t *testing.T) {
	dlq := NewDeadLetterQueue(5)

	job := NewJob("failed-job", nil)
	err := errors.New("test error")

	dlq.Add(job, err)

	entries := dlq.Get()
	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}

	if entries[0].Job.ID != "failed-job" {
		t.Errorf("expected job ID 'failed-job', got %s", entries[0].Job.ID)
	}

	// Test capacity
	for i := 0; i < 10; i++ {
		dlq.Add(NewJob("job"+string(rune(i)), nil), errors.New("error"))
	}

	if dlq.Len() != 5 {
		t.Errorf("expected DLQ capped at 5, got %d", dlq.Len())
	}

	// Test remove
	dlq.Clear()
	if dlq.Len() != 0 {
		t.Errorf("expected empty DLQ after clear, got %d", dlq.Len())
	}
}

func TestRetryBackoff(t *testing.T) {
	config := &RetryConfig{
		MaxRetries:   5,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     2 * time.Second,
		Strategy:     BackoffExponential,
		Multiplier:   2.0,
	}

	retryer := NewRetryer(config)

	delays := []time.Duration{
		retryer.CalculateDelay(1),
		retryer.CalculateDelay(2),
		retryer.CalculateDelay(3),
		retryer.CalculateDelay(4),
	}

	// Check exponential increase
	for i := 1; i < len(delays); i++ {
		if delays[i] <= delays[i-1] {
			t.Errorf("delay %d (%v) should be greater than delay %d (%v)",
				i+1, delays[i], i, delays[i-1])
		}
	}

	// Check max delay cap
	delay := retryer.CalculateDelay(100)
	if delay > config.MaxDelay {
		t.Errorf("delay %v exceeds max %v", delay, config.MaxDelay)
	}
}

func TestJobMetadata(t *testing.T) {
	job := NewJob("test", nil).
		WithName("Test Job").
		WithPriority(PriorityHigh).
		WithTimeout(5 * time.Second).
		WithRetry(3, time.Second).
		WithMetadata("key1", "value1").
		WithMetadata("key2", 42)

	if job.Name != "Test Job" {
		t.Errorf("expected name 'Test Job', got %s", job.Name)
	}

	if job.Priority != PriorityHigh {
		t.Errorf("expected high priority, got %v", job.Priority)
	}

	if job.Timeout != 5*time.Second {
		t.Errorf("expected 5s timeout, got %v", job.Timeout)
	}

	if job.MaxRetries != 3 {
		t.Errorf("expected 3 retries, got %d", job.MaxRetries)
	}

	if job.Metadata["key1"] != "value1" {
		t.Errorf("expected metadata key1='value1', got %v", job.Metadata["key1"])
	}
}

func TestJobStatus(t *testing.T) {
	job := NewJob("test", nil)

	if job.Status() != JobStatusPending {
		t.Errorf("expected pending status, got %v", job.Status())
	}

	job.SetStatus(JobStatusRunning)
	if job.Status() != JobStatusRunning {
		t.Errorf("expected running status, got %v", job.Status())
	}

	// Test string representation
	statuses := map[JobStatus]string{
		JobStatusPending:   "pending",
		JobStatusRunning:   "running",
		JobStatusCompleted: "completed",
		JobStatusFailed:    "failed",
		JobStatusCanceled:  "canceled",
		JobStatusTimeout:   "timeout",
	}

	for status, expected := range statuses {
		if status.String() != expected {
			t.Errorf("expected status string '%s', got '%s'", expected, status.String())
		}
	}
}

func TestPriorityString(t *testing.T) {
	priorities := map[Priority]string{
		PriorityLow:      "low",
		PriorityNormal:   "normal",
		PriorityHigh:     "high",
		PriorityCritical: "critical",
	}

	for priority, expected := range priorities {
		if priority.String() != expected {
			t.Errorf("expected priority string '%s', got '%s'", expected, priority.String())
		}
	}
}

func TestResultSuccess(t *testing.T) {
	success := &Result{
		Status: JobStatusCompleted,
		Error:  nil,
	}

	failure := &Result{
		Status: JobStatusFailed,
		Error:  errors.New("failed"),
	}

	if !success.Success() {
		t.Error("expected success result to return true")
	}

	if failure.Success() {
		t.Error("expected failure result to return false")
	}
}

func TestPoolSubmitNotStarted(t *testing.T) {
	pool := NewPool(WithWorkers(2))
	// Don't start the pool

	job := NewJob("test", func(ctx context.Context) (interface{}, error) {
		return nil, nil
	})

	future := pool.Submit(job)

	result, err := future.GetWithTimeout(time.Second)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if result.Status != JobStatusFailed {
		t.Errorf("expected failed status, got %v", result.Status)
	}

	if !errors.Is(result.Error, ErrPoolNotStarted) {
		t.Errorf("expected ErrPoolNotStarted, got %v", result.Error)
	}
}

func BenchmarkPoolSubmit(b *testing.B) {
	pool := NewPool(WithWorkers(8), WithQueueSize(10000))
	pool.Start()
	defer pool.Shutdown(context.Background())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.SubmitFunc(func(ctx context.Context) (interface{}, error) {
			return nil, nil
		})
	}
}

func BenchmarkQueueEnqueue(b *testing.B) {
	q := NewInMemoryQueue(b.N + 1)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Enqueue(NewJob("test", nil))
	}
}

func BenchmarkCronParse(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ParseCron("*/5 * * * *")
	}
}

func BenchmarkCronNext(b *testing.B) {
	schedule, _ := ParseCron("*/5 * * * *")
	from := time.Now()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		schedule.Next(from)
	}
}
