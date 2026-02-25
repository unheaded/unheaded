// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package eventbus

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// LoggingMiddleware creates middleware that logs events.
func LoggingMiddleware(logger *log.Logger) Middleware {
	if logger == nil {
		logger = log.New(os.Stdout, "[eventbus] ", log.LstdFlags)
	}

	return func(next Handler) Handler {
		return func(e Event) {
			start := time.Now()
			logger.Printf("event: topic=%s id=%s", e.Topic, e.ID)

			next(e)

			logger.Printf("event processed: topic=%s id=%s duration=%v",
				e.Topic, e.ID, time.Since(start))
		}
	}
}

// LoggingMiddlewareWithWriter creates logging middleware with custom writer.
func LoggingMiddlewareWithWriter(w io.Writer) Middleware {
	logger := log.New(w, "[eventbus] ", log.LstdFlags)
	return LoggingMiddleware(logger)
}

// TracingMiddleware creates middleware that adds trace information.
func TracingMiddleware() Middleware {
	return func(next Handler) Handler {
		return func(e Event) {
			// Ensure Meta exists
			if e.Meta == nil {
				e.Meta = make(map[string]string)
			}

			// Add trace timing if not present
			if _, ok := e.Meta["trace_start"]; !ok {
				e.Meta["trace_start"] = time.Now().Format(time.RFC3339Nano)
			}

			next(e)

			e.Meta["trace_end"] = time.Now().Format(time.RFC3339Nano)
		}
	}
}

// MetricsMiddleware creates middleware that collects metrics.
type MetricsMiddleware struct {
	mu           sync.RWMutex
	counts       map[string]uint64
	durations    map[string]time.Duration
	errors       map[string]uint64
	totalCount   uint64
	totalErrors  uint64
	totalLatency time.Duration
}

// NewMetricsMiddleware creates a new metrics middleware.
func NewMetricsMiddleware() *MetricsMiddleware {
	return &MetricsMiddleware{
		counts:    make(map[string]uint64),
		durations: make(map[string]time.Duration),
		errors:    make(map[string]uint64),
	}
}

// Middleware returns the middleware function.
func (m *MetricsMiddleware) Middleware() Middleware {
	return func(next Handler) Handler {
		return func(e Event) {
			start := time.Now()

			defer func() {
				duration := time.Since(start)

				m.mu.Lock()
				m.counts[e.Topic]++
				m.durations[e.Topic] += duration
				m.totalCount++
				m.totalLatency += duration

				if r := recover(); r != nil {
					m.errors[e.Topic]++
					m.totalErrors++
					m.mu.Unlock()
					panic(r) // Re-panic after recording
				}
				m.mu.Unlock()
			}()

			next(e)
		}
	}
}

// TopicMetrics returns metrics for a specific topic.
func (m *MetricsMiddleware) TopicMetrics(topic string) (count uint64, avgDuration time.Duration, errors uint64) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count = m.counts[topic]
	if count > 0 {
		avgDuration = m.durations[topic] / time.Duration(count)
	}
	errors = m.errors[topic]
	return
}

// TotalMetrics returns aggregate metrics.
func (m *MetricsMiddleware) TotalMetrics() (count uint64, avgDuration time.Duration, errors uint64) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count = m.totalCount
	if count > 0 {
		avgDuration = m.totalLatency / time.Duration(count)
	}
	errors = m.totalErrors
	return
}

// Reset clears all metrics.
func (m *MetricsMiddleware) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.counts = make(map[string]uint64)
	m.durations = make(map[string]time.Duration)
	m.errors = make(map[string]uint64)
	m.totalCount = 0
	m.totalErrors = 0
	m.totalLatency = 0
}

// RecoveryMiddleware creates middleware that recovers from panics.
func RecoveryMiddleware(onPanic func(Event, interface{})) Middleware {
	return func(next Handler) Handler {
		return func(e Event) {
			defer func() {
				if r := recover(); r != nil {
					if onPanic != nil {
						onPanic(e, r)
					}
				}
			}()

			next(e)
		}
	}
}

// TimeoutMiddleware creates middleware that enforces handler timeout.
func TimeoutMiddleware(timeout time.Duration) Middleware {
	return func(next Handler) Handler {
		return func(e Event) {
			done := make(chan struct{})

			go func() {
				defer close(done)
				next(e)
			}()

			select {
			case <-done:
				// Handler completed
			case <-time.After(timeout):
				// Timeout - handler may still be running
			}
		}
	}
}

// RetryMiddleware creates middleware that retries failed handlers.
func RetryMiddleware(maxRetries int, delay time.Duration) Middleware {
	return func(next Handler) Handler {
		return func(e Event) {
			var lastPanic interface{}

			for attempt := 0; attempt <= maxRetries; attempt++ {
				func() {
					defer func() {
						lastPanic = recover()
					}()
					next(e)
					lastPanic = nil
				}()

				if lastPanic == nil {
					return // Success
				}

				if attempt < maxRetries {
					time.Sleep(delay)
				}
			}

			// All retries failed
			if lastPanic != nil {
				panic(lastPanic)
			}
		}
	}
}

// CircuitBreakerMiddleware creates middleware with circuit breaker pattern.
type CircuitBreakerMiddleware struct {
	failures     uint64
	threshold    uint64
	resetTimeout time.Duration
	openUntil    int64 // Unix timestamp
	halfOpen     int32
}

// NewCircuitBreakerMiddleware creates a circuit breaker.
func NewCircuitBreakerMiddleware(threshold uint64, resetTimeout time.Duration) *CircuitBreakerMiddleware {
	return &CircuitBreakerMiddleware{
		threshold:    threshold,
		resetTimeout: resetTimeout,
	}
}

// Middleware returns the middleware function.
func (cb *CircuitBreakerMiddleware) Middleware() Middleware {
	return func(next Handler) Handler {
		return func(e Event) {
			// Check if circuit is open
			openUntil := atomic.LoadInt64(&cb.openUntil)
			if openUntil > 0 {
				if time.Now().UnixNano() < openUntil {
					// Circuit is open, skip handler
					return
				}
				// Try half-open
				if !atomic.CompareAndSwapInt32(&cb.halfOpen, 0, 1) {
					return // Another goroutine is trying
				}
			}

			// Execute handler
			var handlerPanic interface{}
			func() {
				defer func() {
					handlerPanic = recover()
				}()
				next(e)
			}()

			if handlerPanic != nil {
				failures := atomic.AddUint64(&cb.failures, 1)
				if failures >= cb.threshold {
					// Open circuit
					atomic.StoreInt64(&cb.openUntil,
						time.Now().Add(cb.resetTimeout).UnixNano())
				}
				atomic.StoreInt32(&cb.halfOpen, 0)
				panic(handlerPanic)
			}

			// Success - reset
			atomic.StoreUint64(&cb.failures, 0)
			atomic.StoreInt64(&cb.openUntil, 0)
			atomic.StoreInt32(&cb.halfOpen, 0)
		}
	}
}

// IsOpen returns true if the circuit is open.
func (cb *CircuitBreakerMiddleware) IsOpen() bool {
	openUntil := atomic.LoadInt64(&cb.openUntil)
	return openUntil > 0 && time.Now().UnixNano() < openUntil
}

// Reset manually resets the circuit breaker.
func (cb *CircuitBreakerMiddleware) Reset() {
	atomic.StoreUint64(&cb.failures, 0)
	atomic.StoreInt64(&cb.openUntil, 0)
	atomic.StoreInt32(&cb.halfOpen, 0)
}

// ValidationMiddleware creates middleware that validates events.
func ValidationMiddleware(validate func(Event) error) Middleware {
	return func(next Handler) Handler {
		return func(e Event) {
			if err := validate(e); err != nil {
				panic(fmt.Sprintf("event validation failed: %v", err))
			}
			next(e)
		}
	}
}

// EnrichmentMiddleware creates middleware that enriches events.
func EnrichmentMiddleware(enrich func(Event) Event) Middleware {
	return func(next Handler) Handler {
		return func(e Event) {
			enriched := enrich(e)
			next(enriched)
		}
	}
}

// DeduplicationMiddleware creates middleware that prevents duplicate processing.
type DeduplicationMiddleware struct {
	mu      sync.RWMutex
	seen    map[string]time.Time
	ttl     time.Duration
	maxSize int
}

// NewDeduplicationMiddleware creates a deduplication middleware.
func NewDeduplicationMiddleware(ttl time.Duration, maxSize int) *DeduplicationMiddleware {
	d := &DeduplicationMiddleware{
		seen:    make(map[string]time.Time),
		ttl:     ttl,
		maxSize: maxSize,
	}

	// Start cleanup goroutine
	go d.cleanup()

	return d
}

// Middleware returns the middleware function.
func (d *DeduplicationMiddleware) Middleware() Middleware {
	return func(next Handler) Handler {
		return func(e Event) {
			d.mu.Lock()

			// Check if seen
			if seenAt, ok := d.seen[e.ID]; ok {
				if time.Since(seenAt) < d.ttl {
					d.mu.Unlock()
					return // Duplicate, skip
				}
			}

			// Mark as seen
			d.seen[e.ID] = time.Now()

			// Enforce max size
			if len(d.seen) > d.maxSize {
				d.evictOldest()
			}

			d.mu.Unlock()

			next(e)
		}
	}
}

// evictOldest removes the oldest entry (caller must hold lock).
func (d *DeduplicationMiddleware) evictOldest() {
	var oldestID string
	var oldestTime time.Time

	for id, t := range d.seen {
		if oldestID == "" || t.Before(oldestTime) {
			oldestID = id
			oldestTime = t
		}
	}

	if oldestID != "" {
		delete(d.seen, oldestID)
	}
}

// cleanup periodically removes expired entries.
func (d *DeduplicationMiddleware) cleanup() {
	ticker := time.NewTicker(d.ttl)
	defer ticker.Stop()

	for range ticker.C {
		d.mu.Lock()
		now := time.Now()
		for id, t := range d.seen {
			if now.Sub(t) > d.ttl {
				delete(d.seen, id)
			}
		}
		d.mu.Unlock()
	}
}

// ChainMiddleware combines multiple middleware into one.
func ChainMiddleware(middlewares ...Middleware) Middleware {
	return func(next Handler) Handler {
		for i := len(middlewares) - 1; i >= 0; i-- {
			next = middlewares[i](next)
		}
		return next
	}
}

// ConditionalMiddleware applies middleware only if condition is met.
func ConditionalMiddleware(condition func(Event) bool, mw Middleware) Middleware {
	return func(next Handler) Handler {
		wrapped := mw(next)
		return func(e Event) {
			if condition(e) {
				wrapped(e)
			} else {
				next(e)
			}
		}
	}
}
