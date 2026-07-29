// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package worker

import (
	"container/heap"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	// ErrQueueFull is returned when the queue is at capacity.
	ErrQueueFull = errors.New("queue is full")
	// ErrQueueEmpty is returned when trying to dequeue from an empty queue.
	ErrQueueEmpty = errors.New("queue is empty")
	// ErrQueueClosed is returned when operating on a closed queue.
	ErrQueueClosed = errors.New("queue is closed")
)

// Queue defines the interface for job queues.
type Queue interface {
	// Enqueue adds a job to the queue.
	Enqueue(job *Job) error
	// Dequeue removes and returns the next job.
	Dequeue() (*Job, error)
	// Peek returns the next job without removing it.
	Peek() (*Job, error)
	// Len returns the number of jobs in the queue.
	Len() int
	// Close closes the queue.
	Close() error
}

// priorityQueueItem wraps a job for the priority queue.
type priorityQueueItem struct {
	job      *Job
	priority int       // Higher priority = higher value
	index    int       // Index in the heap
	addedAt  time.Time // For FIFO ordering within same priority
}

// priorityHeap implements heap.Interface for priority queue.
type priorityHeap []*priorityQueueItem

func (h priorityHeap) Len() int { return len(h) }

func (h priorityHeap) Less(i, j int) bool {
	// Higher priority first
	if h[i].priority != h[j].priority {
		return h[i].priority > h[j].priority
	}
	// FIFO for same priority
	return h[i].addedAt.Before(h[j].addedAt)
}

func (h priorityHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *priorityHeap) Push(x interface{}) {
	item, _ := x.(*priorityQueueItem)
	item.index = len(*h)
	*h = append(*h, item)
}

func (h *priorityHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // Avoid memory leak
	item.index = -1 // For safety
	*h = old[0 : n-1]
	return item
}

// InMemoryQueue is a priority queue stored in memory.
type InMemoryQueue struct {
	heap     priorityHeap
	capacity int
	closed   bool
	mu       sync.RWMutex
	cond     *sync.Cond
}

// NewInMemoryQueue creates a new in-memory priority queue.
func NewInMemoryQueue(capacity int) *InMemoryQueue {
	q := &InMemoryQueue{
		heap:     make(priorityHeap, 0),
		capacity: capacity,
	}
	q.cond = sync.NewCond(&q.mu)
	heap.Init(&q.heap)
	return q
}

// Enqueue adds a job to the queue with priority.
func (q *InMemoryQueue) Enqueue(job *Job) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return ErrQueueClosed
	}

	if q.capacity > 0 && len(q.heap) >= q.capacity {
		return ErrQueueFull
	}

	item := &priorityQueueItem{
		job:      job,
		priority: int(job.Priority),
		addedAt:  time.Now(),
	}
	heap.Push(&q.heap, item)
	q.cond.Signal()
	return nil
}

// Dequeue removes and returns the highest priority job.
func (q *InMemoryQueue) Dequeue() (*Job, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed && len(q.heap) == 0 {
		return nil, ErrQueueClosed
	}

	if len(q.heap) == 0 {
		return nil, ErrQueueEmpty
	}

	item, _ := heap.Pop(&q.heap).(*priorityQueueItem)
	return item.job, nil
}

// DequeueWait blocks until a job is available or the queue is closed.
func (q *InMemoryQueue) DequeueWait() (*Job, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for len(q.heap) == 0 && !q.closed {
		q.cond.Wait()
	}

	if q.closed && len(q.heap) == 0 {
		return nil, ErrQueueClosed
	}

	item, _ := heap.Pop(&q.heap).(*priorityQueueItem)
	return item.job, nil
}

// Peek returns the highest priority job without removing it.
func (q *InMemoryQueue) Peek() (*Job, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if q.closed && len(q.heap) == 0 {
		return nil, ErrQueueClosed
	}

	if len(q.heap) == 0 {
		return nil, ErrQueueEmpty
	}

	return q.heap[0].job, nil
}

// Len returns the number of jobs in the queue.
func (q *InMemoryQueue) Len() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.heap)
}

// Close closes the queue and wakes up all waiters.
func (q *InMemoryQueue) Close() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.closed = true
	q.cond.Broadcast()
	return nil
}

// IsClosed returns true if the queue is closed.
func (q *InMemoryQueue) IsClosed() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.closed
}

// DeadLetterQueue stores failed jobs for later inspection or reprocessing.
type DeadLetterQueue struct {
	entries  []*DeadLetterEntry
	capacity int
	mu       sync.RWMutex
}

// NewDeadLetterQueue creates a new dead letter queue.
func NewDeadLetterQueue(capacity int) *DeadLetterQueue {
	return &DeadLetterQueue{
		entries:  make([]*DeadLetterEntry, 0),
		capacity: capacity,
	}
}

// Add adds a failed job to the dead letter queue.
func (dlq *DeadLetterQueue) Add(job *Job, err error) {
	dlq.mu.Lock()
	defer dlq.mu.Unlock()

	entry := &DeadLetterEntry{
		Job:       job,
		Error:     err,
		FailedAt:  time.Now(),
		Attempts:  job.Attempts(),
		LastRetry: time.Now(),
	}

	// If at capacity, remove oldest entry
	if dlq.capacity > 0 && len(dlq.entries) >= dlq.capacity {
		dlq.entries = dlq.entries[1:]
	}

	dlq.entries = append(dlq.entries, entry)
}

// Get returns all entries in the dead letter queue.
func (dlq *DeadLetterQueue) Get() []*DeadLetterEntry {
	dlq.mu.RLock()
	defer dlq.mu.RUnlock()

	entries := make([]*DeadLetterEntry, len(dlq.entries))
	copy(entries, dlq.entries)
	return entries
}

// Len returns the number of entries in the dead letter queue.
func (dlq *DeadLetterQueue) Len() int {
	dlq.mu.RLock()
	defer dlq.mu.RUnlock()
	return len(dlq.entries)
}

// Clear removes all entries from the dead letter queue.
func (dlq *DeadLetterQueue) Clear() {
	dlq.mu.Lock()
	defer dlq.mu.Unlock()
	dlq.entries = make([]*DeadLetterEntry, 0)
}

// Remove removes and returns an entry by job ID.
func (dlq *DeadLetterQueue) Remove(jobID string) *DeadLetterEntry {
	dlq.mu.Lock()
	defer dlq.mu.Unlock()

	for i, entry := range dlq.entries {
		if entry.Job.ID == jobID {
			dlq.entries = append(dlq.entries[:i], dlq.entries[i+1:]...)
			return entry
		}
	}
	return nil
}

// PersistentQueue wraps an in-memory queue with file persistence.
type PersistentQueue struct {
	*InMemoryQueue
	path     string
	saveOnOp bool
}

// persistedJob is a serializable version of Job for persistence.
type persistedJob struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Priority   Priority               `json:"priority"`
	Timeout    time.Duration          `json:"timeout"`
	MaxRetries int                    `json:"max_retries"`
	RetryDelay time.Duration          `json:"retry_delay"`
	CreatedAt  time.Time              `json:"created_at"`
	Metadata   map[string]interface{} `json:"metadata"`
}

// NewPersistentQueue creates a new queue with file persistence.
func NewPersistentQueue(path string, capacity int) (*PersistentQueue, error) {
	pq := &PersistentQueue{
		InMemoryQueue: NewInMemoryQueue(capacity),
		path:          path,
		saveOnOp:      true,
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, err
	}

	// Load existing queue if file exists
	if _, err := os.Stat(path); err == nil {
		if err := pq.load(); err != nil {
			return nil, err
		}
	}

	return pq, nil
}

// Enqueue adds a job and persists the queue.
func (pq *PersistentQueue) Enqueue(job *Job) error {
	if err := pq.InMemoryQueue.Enqueue(job); err != nil {
		return err
	}
	if pq.saveOnOp {
		return pq.save()
	}
	return nil
}

// Dequeue removes a job and persists the queue.
func (pq *PersistentQueue) Dequeue() (*Job, error) {
	job, err := pq.InMemoryQueue.Dequeue()
	if err != nil {
		return nil, err
	}
	if pq.saveOnOp {
		if saveErr := pq.save(); saveErr != nil {
			// Re-enqueue the job if save fails
			_ = pq.InMemoryQueue.Enqueue(job)
			return nil, saveErr
		}
	}
	return job, nil
}

// save persists the queue to disk.
func (pq *PersistentQueue) save() error {
	pq.mu.RLock()
	jobs := make([]persistedJob, len(pq.heap))
	for i, item := range pq.heap {
		jobs[i] = persistedJob{
			ID:         item.job.ID,
			Name:       item.job.Name,
			Priority:   item.job.Priority,
			Timeout:    item.job.Timeout,
			MaxRetries: item.job.MaxRetries,
			RetryDelay: item.job.RetryDelay,
			CreatedAt:  item.job.CreatedAt,
			Metadata:   item.job.Metadata,
		}
	}
	pq.mu.RUnlock()

	data, err := json.MarshalIndent(jobs, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(pq.path, data, 0640) // #nosec G306 -- 0640 — group-readable only, no world access
}

// load restores the queue from disk.
// Note: This only restores job metadata, not the actual functions.
// Jobs without functions will need to be re-registered.
func (pq *PersistentQueue) load() error {
	data, err := os.ReadFile(pq.path)
	if err != nil {
		return err
	}

	var jobs []persistedJob
	if err := json.Unmarshal(data, &jobs); err != nil {
		return err
	}

	// Note: We can only restore job metadata, not the actual functions
	// In a real system, you'd have a job registry to look up functions by ID
	for _, pj := range jobs {
		job := &Job{
			ID:         pj.ID,
			Name:       pj.Name,
			Priority:   pj.Priority,
			Timeout:    pj.Timeout,
			MaxRetries: pj.MaxRetries,
			RetryDelay: pj.RetryDelay,
			CreatedAt:  pj.CreatedAt,
			Metadata:   pj.Metadata,
			// Func will need to be set by the application
		}
		_ = pq.InMemoryQueue.Enqueue(job)
	}

	return nil
}

// Sync forces a save to disk.
func (pq *PersistentQueue) Sync() error {
	return pq.save()
}
