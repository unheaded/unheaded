// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package eventbus

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// AsyncConfig configures async event handling.
type AsyncConfig struct {
	BufferSize  int
	WorkerCount int
	MaxRetries  int
	RetryDelay  time.Duration
}

// DefaultAsyncConfig returns sensible defaults for async handling.
func DefaultAsyncConfig() AsyncConfig {
	return AsyncConfig{
		BufferSize:  1000,
		WorkerCount: runtime.NumCPU(),
		MaxRetries:  3,
		RetryDelay:  100 * time.Millisecond,
	}
}

// startAsyncWorkers starts the async event processing workers.
func (b *Bus) startAsyncWorkers() {
	workerCount := runtime.NumCPU()
	if workerCount < 2 {
		workerCount = 2
	}

	for i := 0; i < workerCount; i++ {
		b.asyncWg.Add(1)
		go b.asyncWorker(i)
	}
}

// asyncWorker processes events from the async channel.
func (b *Bus) asyncWorker(id int) {
	defer b.asyncWg.Done()

	for {
		select {
		case <-b.asyncCtx.Done():
			// Drain remaining events before exiting
			for {
				select {
				case dispatch, ok := <-b.asyncChan:
					if !ok {
						return
					}
					b.processDispatch(dispatch)
				default:
					return
				}
			}
		case dispatch, ok := <-b.asyncChan:
			if !ok {
				return
			}
			b.processDispatch(dispatch)
		}
	}
}

// processDispatch handles a single event dispatch.
func (b *Bus) processDispatch(dispatch eventDispatch) {
	defer func() {
		if r := recover(); r != nil {
			if b.deadLetter != nil {
				b.deadLetter.Add(DeadLetter{
					Event:  dispatch.event,
					Reason: "handler panic",
					Time:   time.Now(),
				})
			}
			atomic.AddUint64(&b.errorCount, 1)
		}
	}()

	dispatch.handler(dispatch.event)
	atomic.AddUint64(&b.deliveredCount, 1)
}

// AsyncHandler wraps a handler for async execution.
type AsyncHandler struct {
	handler Handler
	timeout time.Duration
}

// NewAsyncHandler creates a new async handler wrapper.
func NewAsyncHandler(handler Handler, timeout time.Duration) *AsyncHandler {
	return &AsyncHandler{
		handler: handler,
		timeout: timeout,
	}
}

// Handle processes an event with timeout.
func (h *AsyncHandler) Handle(e Event) {
	done := make(chan struct{})

	go func() {
		defer close(done)
		h.handler(e)
	}()

	if h.timeout > 0 {
		select {
		case <-done:
			// Handler completed
		case <-time.After(h.timeout):
			// Timeout - handler still running but we move on
		}
	} else {
		<-done
	}
}

// BatchHandler collects events and processes them in batches.
type BatchHandler struct {
	mu        sync.Mutex
	events    []Event
	batchSize int
	timeout   time.Duration
	processor func([]Event)
	timer     *time.Timer
	done      chan struct{}
}

// NewBatchHandler creates a handler that batches events.
func NewBatchHandler(batchSize int, timeout time.Duration, processor func([]Event)) *BatchHandler {
	h := &BatchHandler{
		events:    make([]Event, 0, batchSize),
		batchSize: batchSize,
		timeout:   timeout,
		processor: processor,
		done:      make(chan struct{}),
	}
	return h
}

// Handle adds an event to the batch.
func (h *BatchHandler) Handle(e Event) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.events = append(h.events, e)

	// Start timeout timer on first event
	if len(h.events) == 1 && h.timeout > 0 {
		h.timer = time.AfterFunc(h.timeout, h.flush)
	}

	// Flush if batch is full
	if len(h.events) >= h.batchSize {
		h.flushLocked()
	}
}

// Flush processes all pending events.
func (h *BatchHandler) Flush() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.flushLocked()
}

// flushLocked processes pending events (caller must hold lock).
func (h *BatchHandler) flushLocked() {
	if len(h.events) == 0 {
		return
	}

	if h.timer != nil {
		h.timer.Stop()
		h.timer = nil
	}

	// Process batch
	batch := h.events
	h.events = make([]Event, 0, h.batchSize)

	// Release lock during processing
	h.mu.Unlock()
	h.processor(batch)
	h.mu.Lock()
}

// flush is called by the timeout timer.
func (h *BatchHandler) flush() {
	h.Flush()
}

// Close flushes remaining events and stops the handler.
func (h *BatchHandler) Close() {
	h.Flush()
	close(h.done)
}

// ThrottledHandler limits event processing rate.
type ThrottledHandler struct {
	handler  Handler
	ticker   *time.Ticker
	events   chan Event
	done     chan struct{}
	dropping int32
}

// NewThrottledHandler creates a rate-limited handler.
func NewThrottledHandler(handler Handler, rate time.Duration, bufferSize int) *ThrottledHandler {
	h := &ThrottledHandler{
		handler: handler,
		ticker:  time.NewTicker(rate),
		events:  make(chan Event, bufferSize),
		done:    make(chan struct{}),
	}

	go h.run()
	return h
}

// Handle queues an event for throttled processing.
func (h *ThrottledHandler) Handle(e Event) {
	select {
	case h.events <- e:
		atomic.StoreInt32(&h.dropping, 0)
	default:
		// Buffer full, drop event
		atomic.StoreInt32(&h.dropping, 1)
	}
}

// IsDropping returns true if events are being dropped.
func (h *ThrottledHandler) IsDropping() bool {
	return atomic.LoadInt32(&h.dropping) == 1
}

// run processes events at the configured rate.
func (h *ThrottledHandler) run() {
	for {
		select {
		case <-h.done:
			h.ticker.Stop()
			return
		case <-h.ticker.C:
			select {
			case e := <-h.events:
				h.handler(e)
			default:
				// No events waiting
			}
		}
	}
}

// Close stops the throttled handler.
func (h *ThrottledHandler) Close() {
	close(h.done)
}

// DebounceHandler delays processing until events stop arriving.
type DebounceHandler struct {
	handler Handler
	delay   time.Duration
	mu      sync.Mutex
	timer   *time.Timer
	latest  *Event
	done    chan struct{}
}

// NewDebounceHandler creates a debounced handler.
func NewDebounceHandler(handler Handler, delay time.Duration) *DebounceHandler {
	return &DebounceHandler{
		handler: handler,
		delay:   delay,
		done:    make(chan struct{}),
	}
}

// Handle processes the event after the delay, resetting on each call.
func (h *DebounceHandler) Handle(e Event) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Store latest event
	h.latest = &e

	// Reset timer
	if h.timer != nil {
		h.timer.Stop()
	}

	h.timer = time.AfterFunc(h.delay, func() {
		h.mu.Lock()
		event := h.latest
		h.latest = nil
		h.mu.Unlock()

		if event != nil {
			h.handler(*event)
		}
	})
}

// Close stops the debounce handler.
func (h *DebounceHandler) Close() {
	h.mu.Lock()
	if h.timer != nil {
		h.timer.Stop()
	}
	h.mu.Unlock()
	close(h.done)
}

// ParallelHandler processes events in parallel with a worker pool.
type ParallelHandler struct {
	handler     Handler
	workerCount int
	events      chan Event
	wg          sync.WaitGroup
	done        chan struct{}
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewParallelHandler creates a handler with parallel processing.
func NewParallelHandler(handler Handler, workerCount, bufferSize int) *ParallelHandler {
	ctx, cancel := context.WithCancel(context.Background())

	h := &ParallelHandler{
		handler:     handler,
		workerCount: workerCount,
		events:      make(chan Event, bufferSize),
		done:        make(chan struct{}),
		ctx:         ctx,
		cancel:      cancel,
	}

	for i := 0; i < workerCount; i++ {
		h.wg.Add(1)
		go h.worker()
	}

	return h
}

// Handle queues an event for parallel processing.
func (h *ParallelHandler) Handle(e Event) {
	select {
	case <-h.ctx.Done():
		return
	case h.events <- e:
	}
}

// worker processes events from the queue.
func (h *ParallelHandler) worker() {
	defer h.wg.Done()

	for {
		select {
		case <-h.ctx.Done():
			return
		case e, ok := <-h.events:
			if !ok {
				return
			}
			h.handler(e)
		}
	}
}

// Close stops all workers and waits for completion.
func (h *ParallelHandler) Close() {
	h.cancel()
	close(h.events)
	h.wg.Wait()
	close(h.done)
}
