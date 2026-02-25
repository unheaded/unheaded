// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package eventbus

import (
	"bytes"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBusBasicPubSub(t *testing.T) {
	bus := New()
	defer bus.Close()

	var received Event
	var wg sync.WaitGroup
	wg.Add(1)

	unsub := bus.Subscribe("test.topic", func(e Event) {
		received = e
		wg.Done()
	})
	defer unsub()

	err := bus.Publish("test.topic", "hello")
	if err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	wg.Wait()

	if received.Topic != "test.topic" {
		t.Errorf("expected topic 'test.topic', got '%s'", received.Topic)
	}
	if received.Data != "hello" {
		t.Errorf("expected data 'hello', got '%v'", received.Data)
	}
}

func TestBusPublishWithMeta(t *testing.T) {
	bus := New()
	defer bus.Close()

	var received Event
	var wg sync.WaitGroup
	wg.Add(1)

	bus.Subscribe("test.meta", func(e Event) {
		received = e
		wg.Done()
	})

	err := bus.PublishWithMeta("test.meta", "data", Meta{
		"trace_id": "abc123",
		"source":   "test",
	})
	if err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	wg.Wait()

	if received.Meta["trace_id"] != "abc123" {
		t.Errorf("expected trace_id 'abc123', got '%s'", received.Meta["trace_id"])
	}
	if received.Meta["source"] != "test" {
		t.Errorf("expected source 'test', got '%s'", received.Meta["source"])
	}
}

func TestBusUnsubscribe(t *testing.T) {
	bus := New()
	defer bus.Close()

	var count int32

	unsub := bus.Subscribe("test.unsub", func(e Event) {
		atomic.AddInt32(&count, 1)
	})

	bus.Publish("test.unsub", "first")
	time.Sleep(10 * time.Millisecond)

	unsub()

	bus.Publish("test.unsub", "second")
	time.Sleep(10 * time.Millisecond)

	if atomic.LoadInt32(&count) != 1 {
		t.Errorf("expected count 1 after unsubscribe, got %d", count)
	}
}

func TestBusWildcardSubscription(t *testing.T) {
	bus := New()
	defer bus.Close()

	var received []string
	var mu sync.Mutex

	// Subscribe to all created events
	bus.Subscribe("*.created", func(e Event) {
		mu.Lock()
		received = append(received, e.Topic)
		mu.Unlock()
	})

	bus.Publish("users.created", "user1")
	bus.Publish("orders.created", "order1")
	bus.Publish("users.updated", "user2") // Should not match

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(received) != 2 {
		t.Errorf("expected 2 events, got %d", len(received))
	}
}

func TestBusPrefixWildcard(t *testing.T) {
	bus := New()
	defer bus.Close()

	var received []string
	var mu sync.Mutex

	// Subscribe to all user events
	bus.Subscribe("users.*", func(e Event) {
		mu.Lock()
		received = append(received, e.Topic)
		mu.Unlock()
	})

	bus.Publish("users.created", "user1")
	bus.Publish("users.updated", "user2")
	bus.Publish("orders.created", "order1") // Should not match

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(received) != 2 {
		t.Errorf("expected 2 events, got %d", len(received))
	}
}

func TestBusGlobalWildcard(t *testing.T) {
	bus := New()
	defer bus.Close()

	var count int32

	// Subscribe to all events
	bus.Subscribe("*", func(e Event) {
		atomic.AddInt32(&count, 1)
	})

	bus.Publish("users.created", "user1")
	bus.Publish("orders.created", "order1")
	bus.Publish("system.status", "ok")

	time.Sleep(50 * time.Millisecond)

	if atomic.LoadInt32(&count) != 3 {
		t.Errorf("expected 3 events, got %d", count)
	}
}

func TestBusWithFilter(t *testing.T) {
	bus := New()
	defer bus.Close()

	var received []int
	var mu sync.Mutex

	// Only receive events where data > 5
	bus.SubscribeFunc("numbers.*", func(e Event) bool {
		n, ok := e.Data.(int)
		return ok && n > 5
	}, func(e Event) {
		mu.Lock()
		received = append(received, e.Data.(int))
		mu.Unlock()
	})

	for i := 1; i <= 10; i++ {
		bus.Publish("numbers.test", i)
	}

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(received) != 5 {
		t.Errorf("expected 5 events (6-10), got %d", len(received))
	}
}

func TestBusAsync(t *testing.T) {
	bus := New(WithAsync(100))
	defer bus.Close()

	var wg sync.WaitGroup
	var count int32

	wg.Add(100)

	bus.Subscribe("async.test", func(e Event) {
		atomic.AddInt32(&count, 1)
		wg.Done()
	})

	for i := 0; i < 100; i++ {
		bus.Publish("async.test", i)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for async events")
	}

	if atomic.LoadInt32(&count) != 100 {
		t.Errorf("expected 100 events, got %d", count)
	}
}

func TestBusWithStore(t *testing.T) {
	store := NewMemoryStore(1000)
	bus := New(WithStore(store))
	defer bus.Close()

	bus.Publish("stored.event", "data1")
	bus.Publish("stored.event", "data2")
	bus.Publish("other.event", "data3")

	if store.Count() != 3 {
		t.Errorf("expected 3 stored events, got %d", store.Count())
	}

	events, err := store.Query("stored.*", time.Time{}, time.Now())
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	if len(events) != 2 {
		t.Errorf("expected 2 events matching 'stored.*', got %d", len(events))
	}
}

func TestBusReplay(t *testing.T) {
	store := NewMemoryStore(1000)
	bus := New(WithStore(store))
	defer bus.Close()

	// Publish some events
	bus.Publish("replay.test", "event1")
	bus.Publish("replay.test", "event2")
	bus.Publish("replay.test", "event3")

	// Replay events
	var replayed []string
	var mu sync.Mutex

	err := bus.Replay("replay.*", time.Hour, func(e Event) {
		mu.Lock()
		replayed = append(replayed, e.Data.(string))
		mu.Unlock()
	})

	if err != nil {
		t.Fatalf("replay failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(replayed) != 3 {
		t.Errorf("expected 3 replayed events, got %d", len(replayed))
	}
}

func TestBusMiddleware(t *testing.T) {
	var buf bytes.Buffer

	bus := New(WithMiddleware(LoggingMiddlewareWithWriter(&buf)))
	defer bus.Close()

	var wg sync.WaitGroup
	wg.Add(1)

	bus.Subscribe("middleware.test", func(e Event) {
		wg.Done()
	})

	bus.Publish("middleware.test", "data")
	wg.Wait()

	if buf.Len() == 0 {
		t.Error("expected logging output")
	}
}

func TestBusDeadLetter(t *testing.T) {
	bus := New(WithDeadLetter(100))
	defer bus.Close()

	bus.Subscribe("panic.test", func(e Event) {
		panic("test panic")
	})

	bus.Publish("panic.test", "trigger")
	time.Sleep(50 * time.Millisecond)

	dlq := bus.DeadLetters()
	if dlq == nil {
		t.Fatal("dead letter queue not configured")
	}

	if dlq.Count() != 1 {
		t.Errorf("expected 1 dead letter, got %d", dlq.Count())
	}
}

func TestBusStats(t *testing.T) {
	bus := New()
	defer bus.Close()

	var wg sync.WaitGroup
	wg.Add(3)

	bus.Subscribe("stats.test", func(e Event) {
		wg.Done()
	})

	bus.Publish("stats.test", "1")
	bus.Publish("stats.test", "2")
	bus.Publish("stats.test", "3")

	wg.Wait()

	stats := bus.Stats()

	if stats.PublishCount != 3 {
		t.Errorf("expected publish count 3, got %d", stats.PublishCount)
	}
	if stats.DeliveredCount != 3 {
		t.Errorf("expected delivered count 3, got %d", stats.DeliveredCount)
	}
	if stats.SubscriberCount != 1 {
		t.Errorf("expected subscriber count 1, got %d", stats.SubscriberCount)
	}
}

func TestBusClose(t *testing.T) {
	bus := New(WithAsync(10))
	bus.Subscribe("close.test", func(e Event) {})

	err := bus.Close()
	if err != nil {
		t.Fatalf("close failed: %v", err)
	}

	err = bus.Publish("close.test", "data")
	if err != ErrBusClosed {
		t.Errorf("expected ErrBusClosed, got %v", err)
	}
}

func TestBusMultipleSubscribers(t *testing.T) {
	bus := New()
	defer bus.Close()

	var count1, count2 int32
	var wg sync.WaitGroup
	wg.Add(2)

	bus.Subscribe("multi.test", func(e Event) {
		atomic.AddInt32(&count1, 1)
		wg.Done()
	})

	bus.Subscribe("multi.test", func(e Event) {
		atomic.AddInt32(&count2, 1)
		wg.Done()
	})

	bus.Publish("multi.test", "data")
	wg.Wait()

	if atomic.LoadInt32(&count1) != 1 || atomic.LoadInt32(&count2) != 1 {
		t.Errorf("expected both subscribers to receive event")
	}
}

func TestTopicRouter(t *testing.T) {
	router := NewTopicRouter()

	router.Register("users.created", "sub1")
	router.Register("users.*", "sub2")
	router.Register("*.created", "sub3")
	router.Register("*", "sub4")

	matches := router.Match("users.created")

	expected := map[string]bool{"sub1": false, "sub2": false, "sub3": false, "sub4": false}
	for _, m := range matches {
		if _, ok := expected[m]; ok {
			expected[m] = true
		}
	}

	for sub, found := range expected {
		if !found {
			t.Errorf("expected to match subscriber %s", sub)
		}
	}
}

func TestMemoryStore(t *testing.T) {
	store := NewMemoryStore(5)

	// Store 7 events
	for i := 0; i < 7; i++ {
		store.Store(Event{
			ID:        string(rune('a' + i)),
			Topic:     "test.topic",
			Data:      i,
			Timestamp: time.Now(),
		})
	}

	// Should only have 5 (LRU eviction)
	if store.Count() != 5 {
		t.Errorf("expected 5 events after eviction, got %d", store.Count())
	}

	// Oldest events (a, b) should be evicted
	if _, ok := store.Get("a"); ok {
		t.Error("event 'a' should have been evicted")
	}
	if _, ok := store.Get("b"); ok {
		t.Error("event 'b' should have been evicted")
	}
}

func TestDeadLetterQueue(t *testing.T) {
	dlq := NewDeadLetterQueue(3)

	dlq.Add(DeadLetter{Event: Event{ID: "1"}, Reason: "error1"})
	dlq.Add(DeadLetter{Event: Event{ID: "2"}, Reason: "error2"})
	dlq.Add(DeadLetter{Event: Event{ID: "3"}, Reason: "error3"})
	dlq.Add(DeadLetter{Event: Event{ID: "4"}, Reason: "error4"})

	if dlq.Count() != 3 {
		t.Errorf("expected 3 dead letters (oldest evicted), got %d", dlq.Count())
	}

	letters := dlq.Get()
	if letters[0].Event.ID != "2" {
		t.Errorf("expected oldest remaining to be '2', got '%s'", letters[0].Event.ID)
	}
}

func TestFilterBuilder(t *testing.T) {
	filter := NewFilterBuilder().
		Topic("users.*").
		Meta("source", "api").
		Build()

	matchingEvent := Event{
		Topic: "users.created",
		Meta:  map[string]string{"source": "api"},
	}

	nonMatchingEvent := Event{
		Topic: "orders.created",
		Meta:  map[string]string{"source": "api"},
	}

	if !filter.Apply(matchingEvent) {
		t.Error("expected filter to match event")
	}

	if filter.Apply(nonMatchingEvent) {
		t.Error("expected filter to not match event")
	}
}

func TestMetricsMiddleware(t *testing.T) {
	metrics := NewMetricsMiddleware()

	bus := New(WithMiddleware(metrics.Middleware()))
	defer bus.Close()

	var wg sync.WaitGroup
	wg.Add(3)

	bus.Subscribe("metrics.test", func(e Event) {
		time.Sleep(10 * time.Millisecond)
		wg.Done()
	})

	bus.Publish("metrics.test", "1")
	bus.Publish("metrics.test", "2")
	bus.Publish("metrics.test", "3")

	wg.Wait()

	count, avgDuration, errors := metrics.TopicMetrics("metrics.test")

	if count != 3 {
		t.Errorf("expected count 3, got %d", count)
	}
	if avgDuration < 10*time.Millisecond {
		t.Errorf("expected avg duration >= 10ms, got %v", avgDuration)
	}
	if errors != 0 {
		t.Errorf("expected 0 errors, got %d", errors)
	}
}

func TestBatchHandler(t *testing.T) {
	var processed [][]Event
	var mu sync.Mutex

	handler := NewBatchHandler(3, 100*time.Millisecond, func(events []Event) {
		mu.Lock()
		batch := make([]Event, len(events))
		copy(batch, events)
		processed = append(processed, batch)
		mu.Unlock()
	})
	defer handler.Close()

	// Send 5 events
	for i := 0; i < 5; i++ {
		handler.Handle(Event{ID: string(rune('0' + i))})
	}

	// Wait for timeout to process remaining
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(processed) != 2 {
		t.Errorf("expected 2 batches, got %d", len(processed))
	}
	if len(processed) >= 1 && len(processed[0]) != 3 {
		t.Errorf("expected first batch size 3, got %d", len(processed[0]))
	}
	if len(processed) >= 2 && len(processed[1]) != 2 {
		t.Errorf("expected second batch size 2, got %d", len(processed[1]))
	}
}

func TestCircuitBreaker(t *testing.T) {
	cb := NewCircuitBreakerMiddleware(3, 100*time.Millisecond)

	var callCount int32
	handler := func(e Event) {
		atomic.AddInt32(&callCount, 1)
		panic("test error")
	}

	wrapped := cb.Middleware()(handler)

	// Trigger 3 failures to open circuit
	for i := 0; i < 3; i++ {
		func() {
			defer func() { recover() }()
			wrapped(Event{})
		}()
	}

	if !cb.IsOpen() {
		t.Error("expected circuit to be open after 3 failures")
	}

	// Next call should be skipped
	wrapped(Event{})

	if atomic.LoadInt32(&callCount) != 3 {
		t.Errorf("expected 3 calls (circuit should be open), got %d", callCount)
	}

	// Wait for circuit to reset
	time.Sleep(150 * time.Millisecond)

	if cb.IsOpen() {
		t.Error("expected circuit to be closed after reset timeout")
	}
}

func TestTopicMatcher(t *testing.T) {
	tests := []struct {
		pattern string
		topic   string
		match   bool
	}{
		{"users.created", "users.created", true},
		{"users.created", "users.updated", false},
		{"users.*", "users.created", true},
		{"users.*", "orders.created", false},
		{"*.created", "users.created", true},
		{"*.created", "users.updated", false},
		{"*", "anything.here", true},
	}

	for _, tt := range tests {
		matcher := NewTopicMatcher(tt.pattern)
		if got := matcher.Matches(tt.topic); got != tt.match {
			t.Errorf("pattern=%s topic=%s: expected %v, got %v",
				tt.pattern, tt.topic, tt.match, got)
		}
	}
}

func BenchmarkPublish(b *testing.B) {
	bus := New()
	defer bus.Close()

	bus.Subscribe("bench.test", func(e Event) {})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.Publish("bench.test", i)
	}
}

func BenchmarkPublishAsync(b *testing.B) {
	bus := New(WithAsync(10000))
	defer bus.Close()

	bus.Subscribe("bench.async", func(e Event) {})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.Publish("bench.async", i)
	}
}

func BenchmarkWildcardMatch(b *testing.B) {
	router := NewTopicRouter()
	router.Register("users.*", "sub1")
	router.Register("*.created", "sub2")
	router.Register("*", "sub3")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		router.Match("users.created")
	}
}
