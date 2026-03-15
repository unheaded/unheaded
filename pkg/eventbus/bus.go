// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package eventbus provides an in-process event distribution system
// for the Unheaded Kingdom.
package eventbus

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Event represents an event in the system.
type Event struct {
	ID        string
	Topic     string
	Data      interface{}
	Meta      map[string]string
	Timestamp time.Time
}

// Meta is a convenience type for event metadata.
type Meta map[string]string

// Handler is a function that handles events.
type Handler func(Event)

// FilterFunc determines if an event should be processed.
type FilterFunc func(Event) bool

// Middleware processes events before they reach handlers.
type Middleware func(Handler) Handler

// Bus is the main event bus.
type Bus struct {
	mu          sync.RWMutex
	subscribers *SubscriberRegistry
	topics      *TopicRouter
	middleware  []Middleware
	store       Store
	deadLetter  *DeadLetterQueue

	// Async handling
	asyncEnabled bool
	asyncBuffer  int
	asyncChan    chan eventDispatch
	asyncWg      sync.WaitGroup
	asyncCtx     context.Context
	asyncCancel  context.CancelFunc

	// Stats
	publishCount   uint64
	deliveredCount uint64
	errorCount     uint64

	// Event ID generation
	eventIDCounter uint64

	// Shutdown state
	closed int32
}

// eventDispatch represents an event being dispatched to a handler.
type eventDispatch struct {
	event   Event
	handler Handler
	subID   string
}

// Option configures the Bus.
type Option func(*Bus)

// WithAsync enables async event handling with the given buffer size.
func WithAsync(bufferSize int) Option {
	return func(b *Bus) {
		b.asyncEnabled = true
		b.asyncBuffer = bufferSize
	}
}

// WithStore sets the event store for replay functionality.
func WithStore(store Store) Option {
	return func(b *Bus) {
		b.store = store
	}
}

// WithMiddleware adds middleware to the bus.
func WithMiddleware(mw ...Middleware) Option {
	return func(b *Bus) {
		b.middleware = append(b.middleware, mw...)
	}
}

// WithDeadLetter enables dead letter handling with the given capacity.
func WithDeadLetter(capacity int) Option {
	return func(b *Bus) {
		b.deadLetter = NewDeadLetterQueue(capacity)
	}
}

// New creates a new event bus with the given options.
func New(opts ...Option) *Bus {
	ctx, cancel := context.WithCancel(context.Background())

	b := &Bus{
		subscribers: NewSubscriberRegistry(),
		topics:      NewTopicRouter(),
		asyncCtx:    ctx,
		asyncCancel: cancel,
	}

	for _, opt := range opts {
		opt(b)
	}

	if b.asyncEnabled {
		b.asyncChan = make(chan eventDispatch, b.asyncBuffer)
		b.startAsyncWorkers()
	}

	return b
}

// Subscribe registers a handler for events on the given topic pattern.
// Returns an unsubscribe function.
func (b *Bus) Subscribe(pattern string, handler Handler) func() {
	return b.SubscribeWithFilter(pattern, nil, handler)
}

// SubscribeFunc registers a handler with a filter for events on the given topic pattern.
// This is an alias for SubscribeWithFilter with clearer naming.
func (b *Bus) SubscribeFunc(pattern string, filter FilterFunc, handler Handler) func() {
	return b.SubscribeWithFilter(pattern, filter, handler)
}

// SubscribeWithFilter registers a handler with an optional filter.
func (b *Bus) SubscribeWithFilter(pattern string, filter FilterFunc, handler Handler) func() {
	if atomic.LoadInt32(&b.closed) == 1 {
		return func() {}
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// Apply middleware chain
	wrappedHandler := b.applyMiddleware(handler)

	sub := &Subscription{
		ID:      b.generateSubscriptionID(),
		Pattern: pattern,
		Handler: wrappedHandler,
		Filter:  filter,
		Created: time.Now(),
	}

	b.subscribers.Add(sub)
	b.topics.Register(pattern, sub.ID)

	return func() {
		b.unsubscribe(sub.ID, pattern)
	}
}

// Publish sends an event to all matching subscribers.
func (b *Bus) Publish(topic string, data interface{}) error {
	return b.PublishWithMeta(topic, data, nil)
}

// PublishWithMeta sends an event with metadata to all matching subscribers.
func (b *Bus) PublishWithMeta(topic string, data interface{}, meta Meta) error {
	if atomic.LoadInt32(&b.closed) == 1 {
		return ErrBusClosed
	}

	event := Event{
		ID:        b.generateEventID(),
		Topic:     topic,
		Data:      data,
		Meta:      meta,
		Timestamp: time.Now(),
	}

	// Store event if store is configured
	if b.store != nil {
		if err := b.store.Store(event); err != nil {
			// Log error but continue publishing
			atomic.AddUint64(&b.errorCount, 1)
		}
	}

	atomic.AddUint64(&b.publishCount, 1)

	return b.dispatch(event)
}

// PublishSync sends an event synchronously, waiting for all handlers.
func (b *Bus) PublishSync(topic string, data interface{}) error {
	return b.PublishSyncWithMeta(topic, data, nil)
}

// PublishSyncWithMeta sends an event synchronously with metadata.
func (b *Bus) PublishSyncWithMeta(topic string, data interface{}, meta Meta) error {
	if atomic.LoadInt32(&b.closed) == 1 {
		return ErrBusClosed
	}

	event := Event{
		ID:        b.generateEventID(),
		Topic:     topic,
		Data:      data,
		Meta:      meta,
		Timestamp: time.Now(),
	}

	if b.store != nil {
		if err := b.store.Store(event); err != nil {
			atomic.AddUint64(&b.errorCount, 1)
		}
	}

	atomic.AddUint64(&b.publishCount, 1)

	return b.dispatchSync(event)
}

// Replay replays events from the store matching the pattern within the duration.
func (b *Bus) Replay(pattern string, duration time.Duration, handler Handler) error {
	if b.store == nil {
		return ErrNoStore
	}

	since := time.Now().Add(-duration)
	events, err := b.store.Query(pattern, since, time.Now())
	if err != nil {
		return fmt.Errorf("query store: %w", err)
	}

	wrappedHandler := b.applyMiddleware(handler)

	for _, event := range events {
		wrappedHandler(event)
	}

	return nil
}

// ReplayAll replays all stored events matching the pattern.
func (b *Bus) ReplayAll(pattern string, handler Handler) error {
	if b.store == nil {
		return ErrNoStore
	}

	events, err := b.store.Query(pattern, time.Time{}, time.Now())
	if err != nil {
		return fmt.Errorf("query store: %w", err)
	}

	wrappedHandler := b.applyMiddleware(handler)

	for _, event := range events {
		wrappedHandler(event)
	}

	return nil
}

// Stats returns current bus statistics.
func (b *Bus) Stats() BusStats {
	return BusStats{
		PublishCount:    atomic.LoadUint64(&b.publishCount),
		DeliveredCount:  atomic.LoadUint64(&b.deliveredCount),
		ErrorCount:      atomic.LoadUint64(&b.errorCount),
		SubscriberCount: b.subscribers.Count(),
		TopicCount:      b.topics.Count(),
	}
}

// Close shuts down the event bus gracefully.
func (b *Bus) Close() error {
	if !atomic.CompareAndSwapInt32(&b.closed, 0, 1) {
		return ErrBusClosed
	}

	b.asyncCancel()

	if b.asyncChan != nil {
		close(b.asyncChan)
		b.asyncWg.Wait()
	}

	return nil
}

// DeadLetters returns the dead letter queue if configured.
func (b *Bus) DeadLetters() *DeadLetterQueue {
	return b.deadLetter
}

// dispatch sends an event to all matching subscribers.
func (b *Bus) dispatch(event Event) error {
	b.mu.RLock()
	subIDs := b.topics.Match(event.Topic)
	b.mu.RUnlock()

	for _, subID := range subIDs {
		sub := b.subscribers.Get(subID)
		if sub == nil {
			continue
		}

		// Apply filter
		if sub.Filter != nil && !sub.Filter(event) {
			continue
		}

		if b.asyncEnabled {
			b.dispatchAsync(event, sub)
		} else {
			b.deliverEvent(event, sub)
		}
	}

	return nil
}

// dispatchSync sends an event synchronously to all matching subscribers.
func (b *Bus) dispatchSync(event Event) error {
	b.mu.RLock()
	subIDs := b.topics.Match(event.Topic)
	b.mu.RUnlock()

	for _, subID := range subIDs {
		sub := b.subscribers.Get(subID)
		if sub == nil {
			continue
		}

		if sub.Filter != nil && !sub.Filter(event) {
			continue
		}

		b.deliverEvent(event, sub)
	}

	return nil
}

// dispatchAsync queues an event for async delivery.
func (b *Bus) dispatchAsync(event Event, sub *Subscription) {
	select {
	case b.asyncChan <- eventDispatch{
		event:   event,
		handler: sub.Handler,
		subID:   sub.ID,
	}:
	default:
		// Channel full, handle as dead letter
		if b.deadLetter != nil {
			b.deadLetter.Add(DeadLetter{
				Event:  event,
				Reason: "async buffer full",
				Time:   time.Now(),
			})
		}
		atomic.AddUint64(&b.errorCount, 1)
	}
}

// deliverEvent safely delivers an event to a handler.
func (b *Bus) deliverEvent(event Event, sub *Subscription) {
	defer func() {
		if r := recover(); r != nil {
			if b.deadLetter != nil {
				b.deadLetter.Add(DeadLetter{
					Event:  event,
					Reason: fmt.Sprintf("panic: %v", r),
					Time:   time.Now(),
				})
			}
			atomic.AddUint64(&b.errorCount, 1)
		}
	}()

	sub.Handler(event)
	atomic.AddUint64(&b.deliveredCount, 1)
}

// unsubscribe removes a subscription.
func (b *Bus) unsubscribe(subID, pattern string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.subscribers.Remove(subID)
	b.topics.Unregister(pattern, subID)
}

// applyMiddleware wraps a handler with all configured middleware.
func (b *Bus) applyMiddleware(handler Handler) Handler {
	wrapped := handler
	// Apply in reverse order so first middleware is outermost
	for i := len(b.middleware) - 1; i >= 0; i-- {
		wrapped = b.middleware[i](wrapped)
	}
	return wrapped
}

// generateEventID generates a unique event ID.
func (b *Bus) generateEventID() string {
	id := atomic.AddUint64(&b.eventIDCounter, 1)
	return fmt.Sprintf("evt_%d_%d", time.Now().UnixNano(), id)
}

// generateSubscriptionID generates a unique subscription ID.
func (b *Bus) generateSubscriptionID() string {
	id := atomic.AddUint64(&b.eventIDCounter, 1)
	return fmt.Sprintf("sub_%d_%d", time.Now().UnixNano(), id)
}

// BusStats contains bus statistics.
type BusStats struct {
	PublishCount    uint64
	DeliveredCount  uint64
	ErrorCount      uint64
	SubscriberCount int
	TopicCount      int
}

// Errors
var (
	ErrBusClosed = fmt.Errorf("event bus is closed")
	ErrNoStore   = fmt.Errorf("no event store configured")
)
