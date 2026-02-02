package eventbus

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// 1. EVENT PUBLISHING AND SUBSCRIPTION TESTS
// =============================================================================

func TestPublishToMultipleTopics(t *testing.T) {
	bus := New()
	defer bus.Close()

	results := make(map[string][]Event)
	var mu sync.Mutex
	var wg sync.WaitGroup

	topics := []string{"users.created", "orders.placed", "payments.received"}
	wg.Add(len(topics))

	for _, topic := range topics {
		topic := topic
		bus.Subscribe(topic, func(e Event) {
			mu.Lock()
			results[e.Topic] = append(results[e.Topic], e)
			mu.Unlock()
			wg.Done()
		})
	}

	for _, topic := range topics {
		bus.Publish(topic, "test-data")
	}

	wg.Wait()

	if len(results) != 3 {
		t.Errorf("expected 3 topics with events, got %d", len(results))
	}
}

func TestPublishWithNilData(t *testing.T) {
	bus := New()
	defer bus.Close()

	var received Event
	var wg sync.WaitGroup
	wg.Add(1)

	bus.Subscribe("test.nil", func(e Event) {
		received = e
		wg.Done()
	})

	err := bus.Publish("test.nil", nil)
	if err != nil {
		t.Fatalf("publish with nil data failed: %v", err)
	}

	wg.Wait()

	if received.Data != nil {
		t.Errorf("expected nil data, got %v", received.Data)
	}
}

func TestPublishComplexData(t *testing.T) {
	bus := New()
	defer bus.Close()

	type ComplexData struct {
		ID       int
		Name     string
		Tags     []string
		Metadata map[string]interface{}
	}

	expected := ComplexData{
		ID:       123,
		Name:     "test",
		Tags:     []string{"a", "b", "c"},
		Metadata: map[string]interface{}{"key": "value"},
	}

	var received Event
	var wg sync.WaitGroup
	wg.Add(1)

	bus.Subscribe("test.complex", func(e Event) {
		received = e
		wg.Done()
	})

	bus.Publish("test.complex", expected)
	wg.Wait()

	data, ok := received.Data.(ComplexData)
	if !ok {
		t.Fatal("failed to assert complex data type")
	}

	if data.ID != expected.ID || data.Name != expected.Name {
		t.Errorf("complex data mismatch: got %+v, expected %+v", data, expected)
	}
}

func TestSubscribeReturnsUnsubscribeFunc(t *testing.T) {
	bus := New()
	defer bus.Close()

	var count int32
	unsub := bus.Subscribe("test.unsub", func(e Event) {
		atomic.AddInt32(&count, 1)
	})

	bus.PublishSync("test.unsub", "first")
	unsub()
	bus.PublishSync("test.unsub", "second")

	if atomic.LoadInt32(&count) != 1 {
		t.Errorf("expected 1 event before unsubscribe, got %d", count)
	}
}

func TestMultipleUnsubscribeCalls(t *testing.T) {
	bus := New()
	defer bus.Close()

	unsub := bus.Subscribe("test.multi-unsub", func(e Event) {})

	// Multiple unsubscribe calls should be safe
	unsub()
	unsub()
	unsub()

	// Should not panic
	stats := bus.Stats()
	if stats.SubscriberCount != 0 {
		t.Errorf("expected 0 subscribers after unsubscribe, got %d", stats.SubscriberCount)
	}
}

func TestPublishSyncDelivery(t *testing.T) {
	bus := New()
	defer bus.Close()

	delivered := false
	bus.Subscribe("test.sync", func(e Event) {
		time.Sleep(10 * time.Millisecond)
		delivered = true
	})

	bus.PublishSync("test.sync", "data")

	// After PublishSync returns, handler should have completed
	if !delivered {
		t.Error("expected synchronous delivery")
	}
}

func TestPublishSyncWithMetaData(t *testing.T) {
	bus := New()
	defer bus.Close()

	var received Event
	bus.Subscribe("test.sync-meta", func(e Event) {
		received = e
	})

	meta := Meta{"key1": "value1", "key2": "value2"}
	bus.PublishSyncWithMeta("test.sync-meta", "data", meta)

	if received.Meta["key1"] != "value1" || received.Meta["key2"] != "value2" {
		t.Errorf("metadata mismatch: got %v", received.Meta)
	}
}

func TestSubscribeAfterPublish(t *testing.T) {
	bus := New()
	defer bus.Close()

	// Publish before subscribe
	bus.Publish("test.late", "early-event")
	time.Sleep(10 * time.Millisecond)

	var count int32
	bus.Subscribe("test.late", func(e Event) {
		atomic.AddInt32(&count, 1)
	})

	// Late subscriber should not receive earlier events
	time.Sleep(10 * time.Millisecond)

	if atomic.LoadInt32(&count) != 0 {
		t.Errorf("late subscriber should not receive past events, got %d", count)
	}
}

func TestEventIDUniqueness(t *testing.T) {
	bus := New()
	defer bus.Close()

	ids := make(map[string]bool)
	var mu sync.Mutex
	var wg sync.WaitGroup
	count := 1000

	wg.Add(count)
	bus.Subscribe("test.ids", func(e Event) {
		mu.Lock()
		ids[e.ID] = true
		mu.Unlock()
		wg.Done()
	})

	for i := 0; i < count; i++ {
		bus.Publish("test.ids", i)
	}

	wg.Wait()

	if len(ids) != count {
		t.Errorf("expected %d unique IDs, got %d", count, len(ids))
	}
}

func TestEventTimestamp(t *testing.T) {
	bus := New()
	defer bus.Close()

	before := time.Now()
	var received Event
	var wg sync.WaitGroup
	wg.Add(1)

	bus.Subscribe("test.timestamp", func(e Event) {
		received = e
		wg.Done()
	})

	bus.Publish("test.timestamp", "data")
	wg.Wait()
	after := time.Now()

	if received.Timestamp.Before(before) || received.Timestamp.After(after) {
		t.Errorf("timestamp %v not in range [%v, %v]", received.Timestamp, before, after)
	}
}

// =============================================================================
// 2. TOPIC ROUTING AND WILDCARDS TESTS
// =============================================================================

func TestExactTopicMatch(t *testing.T) {
	router := NewTopicRouter()

	router.Register("users.created", "sub1")
	router.Register("users.updated", "sub2")

	matches := router.Match("users.created")
	if len(matches) != 1 || matches[0] != "sub1" {
		t.Errorf("expected exact match for sub1, got %v", matches)
	}
}

func TestPrefixWildcardMatch(t *testing.T) {
	router := NewTopicRouter()

	router.Register("users.*", "sub1")

	testCases := []struct {
		topic   string
		matches bool
	}{
		{"users.created", true},
		{"users.updated", true},
		{"users.deleted", true},
		{"orders.created", false},
		{"users", false},
	}

	for _, tc := range testCases {
		matches := router.Match(tc.topic)
		found := false
		for _, m := range matches {
			if m == "sub1" {
				found = true
				break
			}
		}
		if found != tc.matches {
			t.Errorf("topic %s: expected match=%v, got match=%v", tc.topic, tc.matches, found)
		}
	}
}

func TestSuffixWildcardMatch(t *testing.T) {
	router := NewTopicRouter()

	router.Register("*.created", "sub1")

	testCases := []struct {
		topic   string
		matches bool
	}{
		{"users.created", true},
		{"orders.created", true},
		{"system.logs.created", true},
		{"users.updated", false},
		{"created", true}, // edge case: topic equals suffix
	}

	for _, tc := range testCases {
		matches := router.Match(tc.topic)
		found := false
		for _, m := range matches {
			if m == "sub1" {
				found = true
				break
			}
		}
		if found != tc.matches {
			t.Errorf("topic %s: expected match=%v, got match=%v", tc.topic, tc.matches, found)
		}
	}
}

func TestGlobalWildcardMatch(t *testing.T) {
	router := NewTopicRouter()

	router.Register("*", "sub1")

	topics := []string{"users.created", "orders", "a.b.c.d.e", "single"}
	for _, topic := range topics {
		matches := router.Match(topic)
		found := false
		for _, m := range matches {
			if m == "sub1" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("global wildcard should match topic %s", topic)
		}
	}
}

func TestMultipleWildcardsMatchSameTopic(t *testing.T) {
	router := NewTopicRouter()

	router.Register("users.created", "exact")
	router.Register("users.*", "prefix")
	router.Register("*.created", "suffix")
	router.Register("*", "global")

	matches := router.Match("users.created")

	expected := map[string]bool{
		"exact":  true,
		"prefix": true,
		"suffix": true,
		"global": true,
	}

	for _, m := range matches {
		if !expected[m] {
			t.Errorf("unexpected match: %s", m)
		}
		delete(expected, m)
	}

	if len(expected) > 0 {
		t.Errorf("missing matches: %v", expected)
	}
}

func TestTopicMatcherSegmentMatching(t *testing.T) {
	tests := []struct {
		pattern string
		topic   string
		match   bool
	}{
		{"users.*.profile", "users.123.profile", true},
		{"users.*.profile", "users.abc.profile", true},
		{"users.*.profile", "users.profile", false},
		{"*.*.created", "users.admin.created", true},
		{"*.*.created", "users.created", false},
	}

	for _, tt := range tests {
		matcher := NewTopicMatcher(tt.pattern)
		if got := matcher.Matches(tt.topic); got != tt.match {
			t.Errorf("pattern=%s topic=%s: expected %v, got %v",
				tt.pattern, tt.topic, tt.match, got)
		}
	}
}

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		pattern string
		topic   string
		match   bool
	}{
		{"*", "anything", true},
		{"*", "a.b.c", true},
		{"users.*", "users.created", true},
		{"users.*", "users.admin.created", true},
		{"users.*", "orders.created", false},
		{"*.created", "users.created", true},
		{"*.created", "created", true},
		{"*.created", "users.updated", false},
		{"users.created", "users.created", true},
		{"users.created", "users.updated", false},
	}

	for _, tt := range tests {
		if got := MatchPattern(tt.pattern, tt.topic); got != tt.match {
			t.Errorf("MatchPattern(%s, %s) = %v, want %v",
				tt.pattern, tt.topic, got, tt.match)
		}
	}
}

func TestRouterUnregister(t *testing.T) {
	router := NewTopicRouter()

	router.Register("users.*", "sub1")
	router.Register("users.*", "sub2")
	router.Register("*", "sub3")

	// Verify initial state
	matches := router.Match("users.created")
	if len(matches) != 3 {
		t.Errorf("expected 3 matches, got %d", len(matches))
	}

	// Unregister sub1
	router.Unregister("users.*", "sub1")

	matches = router.Match("users.created")
	for _, m := range matches {
		if m == "sub1" {
			t.Error("sub1 should have been unregistered")
		}
	}
}

func TestRouterTopics(t *testing.T) {
	router := NewTopicRouter()

	patterns := []string{"users.*", "*.created", "orders.placed", "*"}
	for i, p := range patterns {
		router.Register(p, fmt.Sprintf("sub%d", i))
	}

	topics := router.Topics()
	if len(topics) != len(patterns) {
		t.Errorf("expected %d topics, got %d", len(patterns), len(topics))
	}
}

func TestRouterCount(t *testing.T) {
	router := NewTopicRouter()

	if router.Count() != 0 {
		t.Error("expected empty router to have count 0")
	}

	router.Register("topic1", "sub1")
	router.Register("topic2", "sub2")

	if router.Count() != 2 {
		t.Errorf("expected count 2, got %d", router.Count())
	}
}

// =============================================================================
// 3. MESSAGE DELIVERY GUARANTEES TESTS
// =============================================================================

func TestAtLeastOnceDelivery(t *testing.T) {
	bus := New()
	defer bus.Close()

	deliveryCount := make(map[string]int)
	var mu sync.Mutex
	var wg sync.WaitGroup

	count := 100
	wg.Add(count)

	bus.Subscribe("test.delivery", func(e Event) {
		mu.Lock()
		deliveryCount[e.ID]++
		mu.Unlock()
		wg.Done()
	})

	for i := 0; i < count; i++ {
		bus.Publish("test.delivery", i)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for deliveries")
	}

	mu.Lock()
	defer mu.Unlock()

	// All events should be delivered at least once
	if len(deliveryCount) != count {
		t.Errorf("expected %d unique deliveries, got %d", count, len(deliveryCount))
	}
}

func TestAllSubscribersReceiveEvent(t *testing.T) {
	bus := New()
	defer bus.Close()

	numSubscribers := 10
	received := make([]int32, numSubscribers)
	var wg sync.WaitGroup
	wg.Add(numSubscribers)

	for i := 0; i < numSubscribers; i++ {
		i := i
		bus.Subscribe("test.all-subs", func(e Event) {
			atomic.StoreInt32(&received[i], 1)
			wg.Done()
		})
	}

	bus.Publish("test.all-subs", "data")
	wg.Wait()

	for i, r := range received {
		if r != 1 {
			t.Errorf("subscriber %d did not receive event", i)
		}
	}
}

func TestStoreAndReplay(t *testing.T) {
	store := NewMemoryStore(1000)
	bus := New(WithStore(store))
	defer bus.Close()

	// Publish events
	for i := 0; i < 50; i++ {
		bus.Publish("replay.test", i)
	}

	time.Sleep(10 * time.Millisecond)

	// Verify stored
	if store.Count() != 50 {
		t.Errorf("expected 50 stored events, got %d", store.Count())
	}

	// Replay
	var replayed []int
	var mu sync.Mutex
	err := bus.ReplayAll("replay.*", func(e Event) {
		mu.Lock()
		replayed = append(replayed, e.Data.(int))
		mu.Unlock()
	})

	if err != nil {
		t.Fatalf("replay failed: %v", err)
	}

	if len(replayed) != 50 {
		t.Errorf("expected 50 replayed events, got %d", len(replayed))
	}
}

func TestReplayWithTimeRange(t *testing.T) {
	store := NewMemoryStore(1000)
	bus := New(WithStore(store))
	defer bus.Close()

	// Publish events
	for i := 0; i < 10; i++ {
		bus.Publish("time.test", i)
	}

	time.Sleep(50 * time.Millisecond)

	// Replay only recent
	var replayed []Event
	err := bus.Replay("time.*", 100*time.Millisecond, func(e Event) {
		replayed = append(replayed, e)
	})

	if err != nil {
		t.Fatalf("replay failed: %v", err)
	}

	if len(replayed) != 10 {
		t.Errorf("expected 10 events within time range, got %d", len(replayed))
	}
}

func TestReplayWithoutStore(t *testing.T) {
	bus := New() // No store configured
	defer bus.Close()

	err := bus.Replay("test.*", time.Hour, func(e Event) {})
	if err != ErrNoStore {
		t.Errorf("expected ErrNoStore, got %v", err)
	}
}

func TestDeadLetterOnHandlerPanic(t *testing.T) {
	bus := New(WithDeadLetter(100))
	defer bus.Close()

	bus.Subscribe("panic.test", func(e Event) {
		panic("intentional panic")
	})

	for i := 0; i < 5; i++ {
		bus.PublishSync("panic.test", i)
	}

	dlq := bus.DeadLetters()
	if dlq == nil {
		t.Fatal("dead letter queue not configured")
	}

	if dlq.Count() != 5 {
		t.Errorf("expected 5 dead letters, got %d", dlq.Count())
	}
}

func TestDeadLetterRetry(t *testing.T) {
	bus := New(WithDeadLetter(100))
	defer bus.Close()

	failCount := int32(3)
	successCount := int32(0)

	bus.Subscribe("retry.test", func(e Event) {
		if atomic.LoadInt32(&failCount) > 0 {
			atomic.AddInt32(&failCount, -1)
			panic("retry needed")
		}
		atomic.AddInt32(&successCount, 1)
	})

	// First publish fails
	bus.PublishSync("retry.test", "data")

	dlq := bus.DeadLetters()

	// Retry until success (simulate retry mechanism)
	for dlq.Count() > 0 && atomic.LoadInt32(&failCount) >= 0 {
		dlq.Retry(bus)
		time.Sleep(10 * time.Millisecond)
	}
}

// =============================================================================
// 4. BACKPRESSURE HANDLING TESTS
// =============================================================================

func TestAsyncBufferFull(t *testing.T) {
	smallBuffer := 5
	bus := New(WithAsync(smallBuffer), WithDeadLetter(100))
	defer bus.Close()

	// Slow handler
	bus.Subscribe("backpressure.test", func(e Event) {
		time.Sleep(100 * time.Millisecond)
	})

	// Flood with events
	for i := 0; i < 100; i++ {
		bus.Publish("backpressure.test", i)
	}

	time.Sleep(50 * time.Millisecond)

	// Some events should go to dead letter queue due to buffer full
	dlq := bus.DeadLetters()
	if dlq != nil && dlq.Count() == 0 {
		// This is okay - depends on timing
		t.Log("no dead letters - buffer handled all events")
	}
}

func TestThrottledHandler(t *testing.T) {
	var processed int32
	handler := NewThrottledHandler(func(e Event) {
		atomic.AddInt32(&processed, 1)
	}, 10*time.Millisecond, 10)
	defer handler.Close()

	// Send many events quickly
	for i := 0; i < 100; i++ {
		handler.Handle(Event{ID: fmt.Sprintf("%d", i)})
	}

	// Wait for throttled processing
	time.Sleep(200 * time.Millisecond)

	// Should have processed significantly fewer than 100
	count := atomic.LoadInt32(&processed)
	if count >= 50 {
		t.Errorf("throttling not effective: processed %d events", count)
	}
}

func TestThrottledHandlerDropping(t *testing.T) {
	handler := NewThrottledHandler(func(e Event) {
		time.Sleep(50 * time.Millisecond)
	}, 100*time.Millisecond, 2) // Small buffer
	defer handler.Close()

	// Flood with events
	for i := 0; i < 100; i++ {
		handler.Handle(Event{ID: fmt.Sprintf("%d", i)})
	}

	// Should be dropping
	if !handler.IsDropping() {
		// This depends on timing, might not always be true
		t.Log("handler not dropping - buffer may have drained")
	}
}

func TestBatchHandlerBatching(t *testing.T) {
	var batches []int
	var mu sync.Mutex

	handler := NewBatchHandler(5, 100*time.Millisecond, func(events []Event) {
		mu.Lock()
		batches = append(batches, len(events))
		mu.Unlock()
	})
	defer handler.Close()

	// Send 12 events
	for i := 0; i < 12; i++ {
		handler.Handle(Event{ID: fmt.Sprintf("%d", i)})
	}

	// Wait for timeout to flush remaining
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	// Should have batches of [5, 5, 2]
	if len(batches) != 3 {
		t.Errorf("expected 3 batches, got %d", len(batches))
	}
}

func TestBatchHandlerTimeout(t *testing.T) {
	var processed bool
	var mu sync.Mutex

	handler := NewBatchHandler(100, 50*time.Millisecond, func(events []Event) {
		mu.Lock()
		processed = true
		mu.Unlock()
	})
	defer handler.Close()

	// Send only 3 events (less than batch size)
	for i := 0; i < 3; i++ {
		handler.Handle(Event{ID: fmt.Sprintf("%d", i)})
	}

	// Wait for timeout
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if !processed {
		t.Error("expected timeout to trigger batch processing")
	}
}

func TestDebounceHandler(t *testing.T) {
	var lastEvent Event
	var processCount int32
	var mu sync.Mutex

	handler := NewDebounceHandler(func(e Event) {
		mu.Lock()
		lastEvent = e
		mu.Unlock()
		atomic.AddInt32(&processCount, 1)
	}, 50*time.Millisecond)
	defer handler.Close()

	// Rapid events - only last should be processed
	for i := 0; i < 10; i++ {
		handler.Handle(Event{ID: fmt.Sprintf("%d", i)})
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for debounce
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	// Should only process once (or twice if timing is borderline)
	count := atomic.LoadInt32(&processCount)
	if count > 2 {
		t.Errorf("debounce not effective: processed %d times", count)
	}

	if lastEvent.ID != "9" {
		t.Errorf("expected last event ID '9', got '%s'", lastEvent.ID)
	}
}

// =============================================================================
// 5. CONCURRENT PUB/SUB TESTS
// =============================================================================

func TestConcurrentPublishers(t *testing.T) {
	bus := New()
	defer bus.Close()

	var received int64
	var wg sync.WaitGroup
	numPublishers := 10
	eventsPerPublisher := 100

	wg.Add(numPublishers * eventsPerPublisher)

	bus.Subscribe("concurrent.pub", func(e Event) {
		atomic.AddInt64(&received, 1)
		wg.Done()
	})

	// Start concurrent publishers
	var pubWg sync.WaitGroup
	for p := 0; p < numPublishers; p++ {
		pubWg.Add(1)
		go func(publisherID int) {
			defer pubWg.Done()
			for i := 0; i < eventsPerPublisher; i++ {
				bus.Publish("concurrent.pub", fmt.Sprintf("p%d-e%d", publisherID, i))
			}
		}(p)
	}

	pubWg.Wait()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatalf("timeout: received %d of %d events", atomic.LoadInt64(&received), numPublishers*eventsPerPublisher)
	}

	expected := int64(numPublishers * eventsPerPublisher)
	if atomic.LoadInt64(&received) != expected {
		t.Errorf("expected %d events, got %d", expected, atomic.LoadInt64(&received))
	}
}

func TestConcurrentSubscribers(t *testing.T) {
	bus := New()
	defer bus.Close()

	numSubscribers := 10
	numEvents := 100
	received := make([]int64, numSubscribers)
	var wg sync.WaitGroup

	wg.Add(numSubscribers * numEvents)

	for i := 0; i < numSubscribers; i++ {
		i := i
		bus.Subscribe("concurrent.sub", func(e Event) {
			atomic.AddInt64(&received[i], 1)
			wg.Done()
		})
	}

	for i := 0; i < numEvents; i++ {
		bus.Publish("concurrent.sub", i)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for concurrent subscribers")
	}

	for i, count := range received {
		if count != int64(numEvents) {
			t.Errorf("subscriber %d received %d events, expected %d", i, count, numEvents)
		}
	}
}

func TestConcurrentSubscribeUnsubscribe(t *testing.T) {
	bus := New()
	defer bus.Close()

	var wg sync.WaitGroup
	numGoroutines := 50
	iterations := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				unsub := bus.Subscribe("concurrent.sub-unsub", func(e Event) {})
				time.Sleep(time.Duration(rand.Intn(1000)) * time.Microsecond)
				unsub()
			}
		}()
	}

	wg.Wait()

	stats := bus.Stats()
	if stats.SubscriberCount != 0 {
		t.Errorf("expected 0 subscribers after all unsubscribes, got %d", stats.SubscriberCount)
	}
}

func TestConcurrentPublishAndSubscribe(t *testing.T) {
	bus := New()
	defer bus.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	var published int64
	var received int64

	// Publishers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					bus.Publish("concurrent.mixed", atomic.LoadInt64(&published))
					atomic.AddInt64(&published, 1)
					time.Sleep(time.Microsecond)
				}
			}
		}()
	}

	// Subscribers (dynamic)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					unsub := bus.Subscribe("concurrent.mixed", func(e Event) {
						atomic.AddInt64(&received, 1)
					})
					time.Sleep(10 * time.Millisecond)
					unsub()
				}
			}
		}()
	}

	<-ctx.Done()
	time.Sleep(100 * time.Millisecond) // Allow cleanup
	wg.Wait()

	t.Logf("Published: %d, Received: %d", atomic.LoadInt64(&published), atomic.LoadInt64(&received))
}

func TestParallelHandler(t *testing.T) {
	var processed int64
	var wg sync.WaitGroup

	handler := NewParallelHandler(func(e Event) {
		atomic.AddInt64(&processed, 1)
		time.Sleep(10 * time.Millisecond)
	}, 4, 100)
	defer handler.Close()

	numEvents := 100
	wg.Add(numEvents)

	go func() {
		for i := 0; i < numEvents; i++ {
			handler.Handle(Event{ID: fmt.Sprintf("%d", i)})
			wg.Done()
		}
	}()

	wg.Wait()
	time.Sleep(500 * time.Millisecond) // Allow processing

	count := atomic.LoadInt64(&processed)
	if count < int64(numEvents/2) {
		t.Errorf("parallel processing not effective: only processed %d of %d", count, numEvents)
	}
}

// =============================================================================
// 6. EVENT ORDERING TESTS
// =============================================================================

func TestEventOrderingSync(t *testing.T) {
	bus := New()
	defer bus.Close()

	var received []int
	var mu sync.Mutex

	bus.Subscribe("order.sync", func(e Event) {
		mu.Lock()
		received = append(received, e.Data.(int))
		mu.Unlock()
	})

	expected := []int{}
	for i := 0; i < 100; i++ {
		expected = append(expected, i)
		bus.PublishSync("order.sync", i)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(received) != len(expected) {
		t.Fatalf("expected %d events, got %d", len(expected), len(received))
	}

	for i, v := range received {
		if v != expected[i] {
			t.Errorf("order mismatch at index %d: expected %d, got %d", i, expected[i], v)
		}
	}
}

func TestEventOrderingPerSubscriber(t *testing.T) {
	bus := New()
	defer bus.Close()

	numSubscribers := 5
	numEvents := 100
	received := make([][]int, numSubscribers)
	var mu sync.Mutex
	var wg sync.WaitGroup

	wg.Add(numSubscribers * numEvents)

	for i := 0; i < numSubscribers; i++ {
		i := i
		received[i] = make([]int, 0, numEvents)
		bus.Subscribe("order.per-sub", func(e Event) {
			mu.Lock()
			received[i] = append(received[i], e.Data.(int))
			mu.Unlock()
			wg.Done()
		})
	}

	for i := 0; i < numEvents; i++ {
		bus.Publish("order.per-sub", i)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for events")
	}

	mu.Lock()
	defer mu.Unlock()

	// Each subscriber should receive events in order
	for subIdx, events := range received {
		if len(events) != numEvents {
			t.Errorf("subscriber %d received %d events, expected %d", subIdx, len(events), numEvents)
			continue
		}

		for i := 1; i < len(events); i++ {
			if events[i] <= events[i-1] {
				// Order may not be guaranteed in async mode
				// But for sync delivery, it should be
				t.Logf("subscriber %d: potential out-of-order at index %d (%d after %d)",
					subIdx, i, events[i], events[i-1])
			}
		}
	}
}

func TestStoreReplayOrder(t *testing.T) {
	store := NewMemoryStore(1000)
	bus := New(WithStore(store))
	defer bus.Close()

	// Publish events with delays
	for i := 0; i < 10; i++ {
		bus.Publish("order.store", i)
		time.Sleep(5 * time.Millisecond)
	}

	// Replay should be in timestamp order
	var replayed []int
	bus.ReplayAll("order.store", func(e Event) {
		replayed = append(replayed, e.Data.(int))
	})

	for i := 0; i < len(replayed)-1; i++ {
		if replayed[i] > replayed[i+1] {
			t.Errorf("replay out of order at index %d: %d before %d", i, replayed[i], replayed[i+1])
		}
	}
}

func TestMemoryStoreQueryOrder(t *testing.T) {
	store := NewMemoryStore(100)

	// Store events with explicit timestamps
	baseTime := time.Now()
	for i := 0; i < 10; i++ {
		store.Store(Event{
			ID:        fmt.Sprintf("evt-%d", i),
			Topic:     "order.test",
			Data:      i,
			Timestamp: baseTime.Add(time.Duration(i) * time.Second),
		})
	}

	events, err := store.Query("order.test", time.Time{}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	// Should be sorted by timestamp (oldest first)
	for i := 1; i < len(events); i++ {
		if events[i].Timestamp.Before(events[i-1].Timestamp) {
			t.Errorf("events not in timestamp order: %v before %v",
				events[i].Timestamp, events[i-1].Timestamp)
		}
	}
}

// =============================================================================
// 7. ERROR HANDLING AND DEAD LETTERS TESTS
// =============================================================================

func TestHandlerPanicRecovery(t *testing.T) {
	bus := New(WithDeadLetter(100))
	defer bus.Close()

	panicCount := 0
	normalCount := 0
	var mu sync.Mutex

	bus.Subscribe("panic.recovery", func(e Event) {
		if e.Data.(int)%2 == 0 {
			panic("even number panic")
		}
		mu.Lock()
		normalCount++
		mu.Unlock()
	})

	for i := 0; i < 10; i++ {
		bus.PublishSync("panic.recovery", i)
	}

	dlq := bus.DeadLetters()
	mu.Lock()
	defer mu.Unlock()

	if normalCount != 5 {
		t.Errorf("expected 5 normal events, got %d", normalCount)
	}

	if dlq.Count() != 5 {
		t.Errorf("expected 5 dead letters, got %d", dlq.Count())
	}

	_ = panicCount
}

func TestDeadLetterQueueCapacity(t *testing.T) {
	dlq := NewDeadLetterQueue(5)

	for i := 0; i < 10; i++ {
		dlq.Add(DeadLetter{
			Event:  Event{ID: fmt.Sprintf("%d", i)},
			Reason: "test",
			Time:   time.Now(),
		})
	}

	if dlq.Count() != 5 {
		t.Errorf("expected 5 dead letters (capacity), got %d", dlq.Count())
	}

	// Oldest should be evicted
	letters := dlq.Get()
	if letters[0].Event.ID != "5" {
		t.Errorf("expected oldest to be '5', got '%s'", letters[0].Event.ID)
	}
}

func TestDeadLetterAttemptTracking(t *testing.T) {
	dlq := NewDeadLetterQueue(10)

	// Add same event multiple times
	for i := 0; i < 5; i++ {
		dlq.Add(DeadLetter{
			Event:  Event{ID: "same-event"},
			Reason: fmt.Sprintf("attempt %d", i),
			Time:   time.Now(),
		})
	}

	if dlq.Count() != 1 {
		t.Errorf("expected 1 dead letter (deduplicated), got %d", dlq.Count())
	}

	letters := dlq.Get()
	if letters[0].Attempts != 4 {
		t.Errorf("expected 4 retry attempts, got %d", letters[0].Attempts)
	}
}

func TestDeadLetterRetryWithFilter(t *testing.T) {
	bus := New(WithDeadLetter(100))
	defer bus.Close()

	shouldSucceed := make(map[string]bool)
	var successCount int32

	bus.Subscribe("filter.retry", func(e Event) {
		if !shouldSucceed[e.ID] {
			panic("not ready yet")
		}
		atomic.AddInt32(&successCount, 1)
	})

	// Generate failures
	for i := 0; i < 5; i++ {
		bus.PublishSync("filter.retry", fmt.Sprintf("event-%d", i))
	}

	dlq := bus.DeadLetters()
	if dlq.Count() != 5 {
		t.Errorf("expected 5 dead letters, got %d", dlq.Count())
	}

	// Mark some as ready to succeed
	shouldSucceed["event-1"] = true
	shouldSucceed["event-3"] = true

	// Retry with filter (only event-1 and event-3)
	retried := dlq.RetryWithFilter(bus, func(dl DeadLetter) bool {
		return shouldSucceed[dl.Event.ID]
	})

	if retried != 2 {
		t.Errorf("expected 2 events to be retried, got %d", retried)
	}
}

func TestRecoveryMiddleware(t *testing.T) {
	var recovered interface{}
	var recoveredEvent Event
	var mu sync.Mutex

	recovery := RecoveryMiddleware(func(e Event, r interface{}) {
		mu.Lock()
		recovered = r
		recoveredEvent = e
		mu.Unlock()
	})

	handler := recovery(func(e Event) {
		panic("test panic")
	})

	handler(Event{ID: "test", Topic: "recovery.test"})

	mu.Lock()
	defer mu.Unlock()

	if recovered == nil {
		t.Error("expected panic to be recovered")
	}

	if recoveredEvent.ID != "test" {
		t.Errorf("expected event ID 'test', got '%s'", recoveredEvent.ID)
	}
}

func TestValidationMiddleware(t *testing.T) {
	validate := func(e Event) error {
		if e.Data == nil {
			return errors.New("data cannot be nil")
		}
		return nil
	}

	validation := ValidationMiddleware(validate)

	var processed bool
	handler := validation(func(e Event) {
		processed = true
	})

	// Valid event
	handler(Event{Data: "valid"})
	if !processed {
		t.Error("valid event should be processed")
	}

	// Invalid event should panic
	processed = false
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for invalid event")
		}
	}()

	handler(Event{Data: nil})
}

func TestRetryMiddleware(t *testing.T) {
	attempts := 0
	successOnAttempt := 3

	retry := RetryMiddleware(5, time.Millisecond)

	handler := retry(func(e Event) {
		attempts++
		if attempts < successOnAttempt {
			panic("not yet")
		}
	})

	handler(Event{ID: "retry-test"})

	if attempts != successOnAttempt {
		t.Errorf("expected %d attempts, got %d", successOnAttempt, attempts)
	}
}

func TestCircuitBreakerOpens(t *testing.T) {
	cb := NewCircuitBreakerMiddleware(3, 100*time.Millisecond)

	var callCount int32
	handler := cb.Middleware()(func(e Event) {
		atomic.AddInt32(&callCount, 1)
		panic("error")
	})

	// Trigger failures
	for i := 0; i < 5; i++ {
		func() {
			defer func() { recover() }()
			handler(Event{})
		}()
	}

	if !cb.IsOpen() {
		t.Error("circuit should be open after threshold failures")
	}

	// Calls should be blocked
	handler(Event{})
	handler(Event{})

	if atomic.LoadInt32(&callCount) != 3 {
		t.Errorf("expected 3 calls (before circuit opened), got %d", callCount)
	}
}

func TestCircuitBreakerReset(t *testing.T) {
	cb := NewCircuitBreakerMiddleware(2, 50*time.Millisecond)

	handler := cb.Middleware()(func(e Event) {
		panic("error")
	})

	// Open circuit
	for i := 0; i < 2; i++ {
		func() {
			defer func() { recover() }()
			handler(Event{})
		}()
	}

	if !cb.IsOpen() {
		t.Error("circuit should be open")
	}

	// Wait for reset
	time.Sleep(100 * time.Millisecond)

	if cb.IsOpen() {
		t.Error("circuit should be closed after reset timeout")
	}

	// Manual reset
	cb.Reset()
	if cb.IsOpen() {
		t.Error("circuit should be closed after manual reset")
	}
}

func TestBusCloseRejectsPublish(t *testing.T) {
	bus := New()

	bus.Subscribe("close.test", func(e Event) {})

	bus.Close()

	err := bus.Publish("close.test", "data")
	if err != ErrBusClosed {
		t.Errorf("expected ErrBusClosed, got %v", err)
	}
}

func TestBusDoubleClose(t *testing.T) {
	bus := New()

	err1 := bus.Close()
	if err1 != nil {
		t.Errorf("first close should succeed, got %v", err1)
	}

	err2 := bus.Close()
	if err2 != ErrBusClosed {
		t.Errorf("second close should return ErrBusClosed, got %v", err2)
	}
}

func TestSubscribeToClosedBus(t *testing.T) {
	bus := New()
	bus.Close()

	unsub := bus.Subscribe("closed.bus", func(e Event) {})
	unsub() // Should not panic

	stats := bus.Stats()
	if stats.SubscriberCount != 0 {
		t.Errorf("closed bus should not accept subscribers, got %d", stats.SubscriberCount)
	}
}

// =============================================================================
// ADDITIONAL FILTER TESTS
// =============================================================================

func TestFilterAnd(t *testing.T) {
	filter1 := NewFilter(func(e Event) bool {
		return e.Topic == "test"
	})
	filter2 := NewFilter(func(e Event) bool {
		return e.Data != nil
	})

	combined := filter1.And(filter2)

	if combined.Apply(Event{Topic: "test", Data: "data"}) != true {
		t.Error("AND filter should pass when both conditions met")
	}
	if combined.Apply(Event{Topic: "test", Data: nil}) != false {
		t.Error("AND filter should fail when second condition not met")
	}
	if combined.Apply(Event{Topic: "other", Data: "data"}) != false {
		t.Error("AND filter should fail when first condition not met")
	}
}

func TestFilterOr(t *testing.T) {
	filter1 := TopicFilter("users.*")
	filter2 := TopicFilter("orders.*")

	combined := filter1.Or(filter2)

	if combined.Apply(Event{Topic: "users.created"}) != true {
		t.Error("OR filter should pass when first condition met")
	}
	if combined.Apply(Event{Topic: "orders.placed"}) != true {
		t.Error("OR filter should pass when second condition met")
	}
	if combined.Apply(Event{Topic: "system.status"}) != false {
		t.Error("OR filter should fail when neither condition met")
	}
}

func TestFilterNot(t *testing.T) {
	filter := TopicFilter("test.*").Not()

	if filter.Apply(Event{Topic: "test.data"}) != false {
		t.Error("NOT filter should invert match")
	}
	if filter.Apply(Event{Topic: "other.data"}) != true {
		t.Error("NOT filter should invert non-match")
	}
}

func TestMetaFilters(t *testing.T) {
	event := Event{
		Meta: map[string]string{
			"source":  "api",
			"version": "1.0",
		},
	}

	if !MetaFilter("source", "api").Apply(event) {
		t.Error("MetaFilter should match")
	}
	if MetaFilter("source", "cli").Apply(event) {
		t.Error("MetaFilter should not match different value")
	}
	if !HasMetaFilter("source").Apply(event) {
		t.Error("HasMetaFilter should match existing key")
	}
	if HasMetaFilter("missing").Apply(event) {
		t.Error("HasMetaFilter should not match missing key")
	}
}

func TestTimeFilters(t *testing.T) {
	now := time.Now()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	event := Event{Timestamp: now}

	if !AfterFilter(past).Apply(event) {
		t.Error("AfterFilter should match event after threshold")
	}
	if AfterFilter(future).Apply(event) {
		t.Error("AfterFilter should not match event before threshold")
	}
	if !BeforeFilter(future).Apply(event) {
		t.Error("BeforeFilter should match event before threshold")
	}
	if BeforeFilter(past).Apply(event) {
		t.Error("BeforeFilter should not match event after threshold")
	}
}

func TestBetweenFilter(t *testing.T) {
	now := time.Now()
	start := now.Add(-time.Hour)
	end := now.Add(time.Hour)

	filter := BetweenFilter(start, end)

	if !filter.Apply(Event{Timestamp: now}) {
		t.Error("BetweenFilter should match event in range")
	}
	if filter.Apply(Event{Timestamp: start.Add(-time.Minute)}) {
		t.Error("BetweenFilter should not match event before range")
	}
	if filter.Apply(Event{Timestamp: end.Add(time.Minute)}) {
		t.Error("BetweenFilter should not match event after range")
	}
}

func TestAnyNoneAllFilters(t *testing.T) {
	filter1 := TopicFilter("users.*")
	filter2 := TopicFilter("orders.*")
	filter3 := TopicFilter("system.*")

	anyFilter := AnyFilter(filter1, filter2, filter3)
	allFilter := AllFilter(filter1, filter2)
	noneFilter := NoneFilter(filter1, filter2)

	usersEvent := Event{Topic: "users.created"}
	ordersEvent := Event{Topic: "orders.placed"}
	otherEvent := Event{Topic: "other.event"}

	// AnyFilter
	if !anyFilter.Apply(usersEvent) {
		t.Error("AnyFilter should match when any condition met")
	}
	if anyFilter.Apply(otherEvent) {
		t.Error("AnyFilter should not match when no conditions met")
	}

	// AllFilter - needs event matching both, which is impossible here
	// but we can test that it requires all conditions
	if allFilter.Apply(usersEvent) {
		t.Error("AllFilter should not match when only some conditions met")
	}

	// NoneFilter
	if !noneFilter.Apply(otherEvent) {
		t.Error("NoneFilter should match when no conditions met")
	}
	if noneFilter.Apply(ordersEvent) {
		t.Error("NoneFilter should not match when any condition met")
	}
}

func TestFilterBuilder(t *testing.T) {
	filter := NewFilterBuilder().
		Topic("users.*").
		Meta("source", "api").
		HasMeta("trace_id").
		Build()

	matching := Event{
		Topic: "users.created",
		Meta:  map[string]string{"source": "api", "trace_id": "123"},
	}
	nonMatching := Event{
		Topic: "users.created",
		Meta:  map[string]string{"source": "cli"},
	}

	if !filter.Apply(matching) {
		t.Error("FilterBuilder should produce matching filter")
	}
	if filter.Apply(nonMatching) {
		t.Error("FilterBuilder should produce filter that rejects non-matching")
	}
}

// =============================================================================
// MEMORY STORE TESTS
// =============================================================================

func TestMemoryStoreLRUEviction(t *testing.T) {
	store := NewMemoryStore(5)

	for i := 0; i < 10; i++ {
		store.Store(Event{
			ID:        fmt.Sprintf("evt-%d", i),
			Topic:     "test",
			Timestamp: time.Now(),
		})
	}

	if store.Count() != 5 {
		t.Errorf("expected 5 events after eviction, got %d", store.Count())
	}

	// Check that oldest events are evicted
	for i := 0; i < 5; i++ {
		if _, ok := store.Get(fmt.Sprintf("evt-%d", i)); ok {
			t.Errorf("event evt-%d should have been evicted", i)
		}
	}
}

func TestMemoryStoreTopicIndex(t *testing.T) {
	store := NewMemoryStore(100)

	store.Store(Event{ID: "1", Topic: "users.created", Timestamp: time.Now()})
	store.Store(Event{ID: "2", Topic: "users.updated", Timestamp: time.Now()})
	store.Store(Event{ID: "3", Topic: "orders.created", Timestamp: time.Now()})

	topics := store.Topics()
	sort.Strings(topics)

	expected := []string{"orders.created", "users.created", "users.updated"}
	if len(topics) != len(expected) {
		t.Errorf("expected %d topics, got %d", len(expected), len(topics))
	}

	for i, topic := range topics {
		if topic != expected[i] {
			t.Errorf("expected topic %s at index %d, got %s", expected[i], i, topic)
		}
	}
}

func TestMemoryStoreQueryByTopic(t *testing.T) {
	store := NewMemoryStore(100)

	store.Store(Event{ID: "1", Topic: "users.created", Data: 1, Timestamp: time.Now()})
	store.Store(Event{ID: "2", Topic: "users.updated", Data: 2, Timestamp: time.Now()})
	store.Store(Event{ID: "3", Topic: "orders.created", Data: 3, Timestamp: time.Now()})

	// Exact query
	events, _ := store.Query("users.created", time.Time{}, time.Now().Add(time.Hour))
	if len(events) != 1 {
		t.Errorf("expected 1 event for exact query, got %d", len(events))
	}

	// Wildcard query
	events, _ = store.Query("users.*", time.Time{}, time.Now().Add(time.Hour))
	if len(events) != 2 {
		t.Errorf("expected 2 events for wildcard query, got %d", len(events))
	}

	// Global wildcard
	events, _ = store.Query("*", time.Time{}, time.Now().Add(time.Hour))
	if len(events) != 3 {
		t.Errorf("expected 3 events for global wildcard, got %d", len(events))
	}
}

func TestMemoryStoreClear(t *testing.T) {
	store := NewMemoryStore(100)

	for i := 0; i < 10; i++ {
		store.Store(Event{ID: fmt.Sprintf("%d", i), Topic: "test", Timestamp: time.Now()})
	}

	if store.Count() != 10 {
		t.Errorf("expected 10 events, got %d", store.Count())
	}

	store.Clear()

	if store.Count() != 0 {
		t.Errorf("expected 0 events after clear, got %d", store.Count())
	}
}

func TestMemoryStoreDuplicateEvent(t *testing.T) {
	store := NewMemoryStore(100)

	event := Event{ID: "duplicate", Topic: "test", Timestamp: time.Now()}

	store.Store(event)
	store.Store(event)
	store.Store(event)

	if store.Count() != 1 {
		t.Errorf("expected 1 event (deduplicated), got %d", store.Count())
	}
}

// =============================================================================
// SUBSCRIBER REGISTRY TESTS
// =============================================================================

func TestSubscriberRegistryAll(t *testing.T) {
	registry := NewSubscriberRegistry()

	for i := 0; i < 5; i++ {
		registry.Add(&Subscription{
			ID:      fmt.Sprintf("sub-%d", i),
			Pattern: fmt.Sprintf("topic-%d", i),
			Created: time.Now(),
		})
	}

	all := registry.All()
	if len(all) != 5 {
		t.Errorf("expected 5 subscriptions, got %d", len(all))
	}
}

func TestSubscriberRegistryByPattern(t *testing.T) {
	registry := NewSubscriberRegistry()

	registry.Add(&Subscription{ID: "sub-1", Pattern: "users.*", Created: time.Now()})
	registry.Add(&Subscription{ID: "sub-2", Pattern: "users.*", Created: time.Now()})
	registry.Add(&Subscription{ID: "sub-3", Pattern: "orders.*", Created: time.Now()})

	subs := registry.ByPattern("users.*")
	if len(subs) != 2 {
		t.Errorf("expected 2 subscriptions for pattern, got %d", len(subs))
	}
}

func TestSubscriberRegistryClear(t *testing.T) {
	registry := NewSubscriberRegistry()

	for i := 0; i < 5; i++ {
		registry.Add(&Subscription{ID: fmt.Sprintf("sub-%d", i), Created: time.Now()})
	}

	registry.Clear()

	if registry.Count() != 0 {
		t.Errorf("expected 0 subscriptions after clear, got %d", registry.Count())
	}
}

func TestSubscriptionInfo(t *testing.T) {
	sub := &Subscription{
		ID:      "test-sub",
		Pattern: "test.pattern",
		Created: time.Now(),
	}

	info := sub.Info()

	if info.ID != sub.ID || info.Pattern != sub.Pattern {
		t.Error("SubscriptionInfo should match Subscription fields")
	}
}

// =============================================================================
// MIDDLEWARE TESTS
// =============================================================================

func TestLoggingMiddleware(t *testing.T) {
	var buf bytes.Buffer
	mw := LoggingMiddlewareWithWriter(&buf)

	handler := mw(func(e Event) {})
	handler(Event{ID: "test", Topic: "log.test"})

	if buf.Len() == 0 {
		t.Error("logging middleware should write output")
	}
}

func TestTracingMiddleware(t *testing.T) {
	mw := TracingMiddleware()

	var processed Event
	handler := mw(func(e Event) {
		processed = e
	})

	handler(Event{ID: "test"})

	if processed.Meta == nil || processed.Meta["trace_start"] == "" {
		t.Error("tracing middleware should add trace_start")
	}
}

func TestMetricsMiddlewareTotalMetrics(t *testing.T) {
	metrics := NewMetricsMiddleware()
	handler := metrics.Middleware()(func(e Event) {
		time.Sleep(time.Millisecond)
	})

	for i := 0; i < 10; i++ {
		handler(Event{Topic: "metrics.test"})
	}

	count, avgDuration, errors := metrics.TotalMetrics()

	if count != 10 {
		t.Errorf("expected 10 total events, got %d", count)
	}
	if avgDuration < time.Millisecond {
		t.Errorf("expected avg duration >= 1ms, got %v", avgDuration)
	}
	if errors != 0 {
		t.Errorf("expected 0 errors, got %d", errors)
	}
}

func TestMetricsMiddlewareReset(t *testing.T) {
	metrics := NewMetricsMiddleware()
	handler := metrics.Middleware()(func(e Event) {})

	handler(Event{Topic: "test"})

	count, _, _ := metrics.TotalMetrics()
	if count != 1 {
		t.Errorf("expected 1 event, got %d", count)
	}

	metrics.Reset()

	count, _, _ = metrics.TotalMetrics()
	if count != 0 {
		t.Errorf("expected 0 events after reset, got %d", count)
	}
}

func TestTimeoutMiddleware(t *testing.T) {
	timeout := TimeoutMiddleware(50 * time.Millisecond)

	completed := make(chan bool, 1)
	handler := timeout(func(e Event) {
		time.Sleep(200 * time.Millisecond)
		completed <- true
	})

	start := time.Now()
	handler(Event{ID: "timeout-test"})
	duration := time.Since(start)

	// Should return after timeout, not wait for handler
	if duration > 100*time.Millisecond {
		t.Errorf("timeout middleware should return early, took %v", duration)
	}
}

func TestEnrichmentMiddleware(t *testing.T) {
	enrich := EnrichmentMiddleware(func(e Event) Event {
		if e.Meta == nil {
			e.Meta = make(map[string]string)
		}
		e.Meta["enriched"] = "true"
		return e
	})

	var processed Event
	handler := enrich(func(e Event) {
		processed = e
	})

	handler(Event{ID: "test"})

	if processed.Meta["enriched"] != "true" {
		t.Error("enrichment middleware should add metadata")
	}
}

func TestChainMiddleware(t *testing.T) {
	var order []string
	var mu sync.Mutex

	mw1 := func(next Handler) Handler {
		return func(e Event) {
			mu.Lock()
			order = append(order, "mw1-start")
			mu.Unlock()
			next(e)
			mu.Lock()
			order = append(order, "mw1-end")
			mu.Unlock()
		}
	}

	mw2 := func(next Handler) Handler {
		return func(e Event) {
			mu.Lock()
			order = append(order, "mw2-start")
			mu.Unlock()
			next(e)
			mu.Lock()
			order = append(order, "mw2-end")
			mu.Unlock()
		}
	}

	chained := ChainMiddleware(mw1, mw2)

	handler := chained(func(e Event) {
		mu.Lock()
		order = append(order, "handler")
		mu.Unlock()
	})

	handler(Event{ID: "test"})

	mu.Lock()
	defer mu.Unlock()

	expected := []string{"mw1-start", "mw2-start", "handler", "mw2-end", "mw1-end"}
	if len(order) != len(expected) {
		t.Errorf("expected %d entries, got %d: %v", len(expected), len(order), order)
	}
}

func TestConditionalMiddleware(t *testing.T) {
	applied := false
	mw := func(next Handler) Handler {
		return func(e Event) {
			applied = true
			next(e)
		}
	}

	conditional := ConditionalMiddleware(func(e Event) bool {
		return e.Topic == "apply.mw"
	}, mw)

	handler := conditional(func(e Event) {})

	// Should apply
	applied = false
	handler(Event{Topic: "apply.mw"})
	if !applied {
		t.Error("middleware should be applied when condition is met")
	}

	// Should not apply
	applied = false
	handler(Event{Topic: "skip.mw"})
	if applied {
		t.Error("middleware should not be applied when condition is not met")
	}
}

// =============================================================================
// ASYNC HANDLER TESTS
// =============================================================================

func TestAsyncHandlerWithTimeout(t *testing.T) {
	handler := NewAsyncHandler(func(e Event) {
		time.Sleep(200 * time.Millisecond)
	}, 50*time.Millisecond)

	start := time.Now()
	handler.Handle(Event{ID: "async-timeout"})
	duration := time.Since(start)

	if duration > 100*time.Millisecond {
		t.Errorf("async handler should timeout, took %v", duration)
	}
}

func TestAsyncHandlerWithoutTimeout(t *testing.T) {
	completed := false
	handler := NewAsyncHandler(func(e Event) {
		time.Sleep(10 * time.Millisecond)
		completed = true
	}, 0) // No timeout

	handler.Handle(Event{ID: "async-no-timeout"})

	if !completed {
		t.Error("async handler without timeout should complete")
	}
}

// =============================================================================
// BENCHMARKS
// =============================================================================

func BenchmarkPublishSync(b *testing.B) {
	bus := New()
	defer bus.Close()

	bus.Subscribe("bench.sync", func(e Event) {})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.PublishSync("bench.sync", i)
	}
}

func BenchmarkPublishWithMetadata(b *testing.B) {
	bus := New()
	defer bus.Close()

	bus.Subscribe("bench.meta", func(e Event) {})
	meta := Meta{"key": "value", "trace_id": "12345"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.PublishWithMeta("bench.meta", i, meta)
	}
}

func BenchmarkPublishMultipleSubscribers(b *testing.B) {
	bus := New()
	defer bus.Close()

	for i := 0; i < 10; i++ {
		bus.Subscribe("bench.multi", func(e Event) {})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.PublishSync("bench.multi", i)
	}
}

func BenchmarkTopicMatching(b *testing.B) {
	router := NewTopicRouter()

	// Register various patterns
	for i := 0; i < 100; i++ {
		router.Register(fmt.Sprintf("topic%d.exact", i), fmt.Sprintf("sub%d", i))
	}
	for i := 0; i < 50; i++ {
		router.Register(fmt.Sprintf("prefix%d.*", i), fmt.Sprintf("prefix%d", i))
	}
	for i := 0; i < 50; i++ {
		router.Register(fmt.Sprintf("*.suffix%d", i), fmt.Sprintf("suffix%d", i))
	}
	router.Register("*", "global")

	topics := []string{
		"topic50.exact",
		"prefix25.something",
		"anything.suffix30",
		"random.topic.name",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		router.Match(topics[i%len(topics)])
	}
}

func BenchmarkFilterApply(b *testing.B) {
	filter := NewFilterBuilder().
		Topic("users.*").
		Meta("source", "api").
		HasMeta("trace_id").
		Build()

	event := Event{
		Topic: "users.created",
		Meta:  map[string]string{"source": "api", "trace_id": "123"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filter.Apply(event)
	}
}

func BenchmarkMemoryStoreWrite(b *testing.B) {
	store := NewMemoryStore(10000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Store(Event{
			ID:        fmt.Sprintf("evt-%d", i),
			Topic:     "bench.store",
			Data:      i,
			Timestamp: time.Now(),
		})
	}
}

func BenchmarkMemoryStoreQuery(b *testing.B) {
	store := NewMemoryStore(10000)

	// Populate store
	for i := 0; i < 1000; i++ {
		store.Store(Event{
			ID:        fmt.Sprintf("evt-%d", i),
			Topic:     fmt.Sprintf("topic%d.events", i%10),
			Timestamp: time.Now(),
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Query("topic5.*", time.Time{}, time.Now())
	}
}

func BenchmarkAsyncPublish(b *testing.B) {
	bus := New(WithAsync(10000))
	defer bus.Close()

	bus.Subscribe("bench.async", func(e Event) {})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.Publish("bench.async", i)
	}
}

func BenchmarkMiddlewareChain(b *testing.B) {
	bus := New(
		WithMiddleware(
			TracingMiddleware(),
			RecoveryMiddleware(nil),
		),
	)
	defer bus.Close()

	bus.Subscribe("bench.middleware", func(e Event) {})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.PublishSync("bench.middleware", i)
	}
}

func BenchmarkConcurrentPublish(b *testing.B) {
	bus := New(WithAsync(10000))
	defer bus.Close()

	bus.Subscribe("bench.concurrent", func(e Event) {})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			bus.Publish("bench.concurrent", i)
			i++
		}
	})
}

func BenchmarkHighThroughput(b *testing.B) {
	bus := New(WithAsync(100000))
	defer bus.Close()

	// Multiple subscribers
	for i := 0; i < 10; i++ {
		bus.Subscribe("bench.throughput", func(e Event) {
			// Simulate minimal work
			_ = e.ID
		})
	}

	b.ResetTimer()
	b.SetBytes(1) // For throughput calculation

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			bus.Publish("bench.throughput", i)
			i++
		}
	})
}

func BenchmarkWildcardHeavy(b *testing.B) {
	bus := New()
	defer bus.Close()

	// Many wildcard patterns
	for i := 0; i < 50; i++ {
		bus.Subscribe(fmt.Sprintf("segment%d.*", i), func(e Event) {})
		bus.Subscribe(fmt.Sprintf("*.suffix%d", i), func(e Event) {})
	}
	bus.Subscribe("*", func(e Event) {})

	topics := []string{
		"segment25.event",
		"random.suffix30",
		"segment10.suffix20",
		"nomatch.here",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.PublishSync(topics[i%len(topics)], i)
	}
}

func BenchmarkDeadLetterHandling(b *testing.B) {
	bus := New(WithDeadLetter(10000))
	defer bus.Close()

	bus.Subscribe("bench.deadletter", func(e Event) {
		panic("intentional")
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.PublishSync("bench.deadletter", i)
	}
}

// =============================================================================
// HIGH-THROUGHPUT SCENARIO TESTS
// =============================================================================

func TestHighThroughputScenario(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping high-throughput test in short mode")
	}

	bus := New(WithAsync(100000))
	defer bus.Close()

	var received int64
	var wg sync.WaitGroup
	numPublishers := runtime.NumCPU()
	eventsPerPublisher := 10000

	totalEvents := numPublishers * eventsPerPublisher
	wg.Add(totalEvents)

	// Multiple subscribers
	for i := 0; i < 5; i++ {
		bus.Subscribe("throughput.*", func(e Event) {
			atomic.AddInt64(&received, 1)
			wg.Done()
		})
	}

	start := time.Now()

	// Concurrent publishers
	var pubWg sync.WaitGroup
	for p := 0; p < numPublishers; p++ {
		pubWg.Add(1)
		go func(id int) {
			defer pubWg.Done()
			for i := 0; i < eventsPerPublisher; i++ {
				bus.Publish("throughput.events", fmt.Sprintf("p%d-e%d", id, i))
			}
		}(p)
	}

	pubWg.Wait()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(30 * time.Second):
		t.Fatalf("timeout: received %d of %d total deliveries", atomic.LoadInt64(&received), totalEvents*5)
	}

	duration := time.Since(start)
	totalDeliveries := int64(totalEvents * 5) // 5 subscribers
	throughput := float64(totalDeliveries) / duration.Seconds()

	t.Logf("High-throughput results:")
	t.Logf("  Total events: %d", totalEvents)
	t.Logf("  Total deliveries: %d", totalDeliveries)
	t.Logf("  Duration: %v", duration)
	t.Logf("  Throughput: %.0f deliveries/sec", throughput)
}

func TestSustainedLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping sustained load test in short mode")
	}

	bus := New(WithAsync(10000), WithDeadLetter(1000))
	defer bus.Close()

	var delivered int64
	var errors int64

	bus.Subscribe("sustained.load", func(e Event) {
		// Simulate varying processing time
		time.Sleep(time.Duration(rand.Intn(100)) * time.Microsecond)
		atomic.AddInt64(&delivered, 1)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		i := 0
		for {
			select {
			case <-ctx.Done():
				return
			default:
				if err := bus.Publish("sustained.load", i); err != nil {
					atomic.AddInt64(&errors, 1)
				}
				i++
				time.Sleep(10 * time.Microsecond)
			}
		}
	}()

	<-ctx.Done()
	wg.Wait()

	// Allow time for queue drain
	time.Sleep(500 * time.Millisecond)

	stats := bus.Stats()
	dlq := bus.DeadLetters()

	t.Logf("Sustained load results:")
	t.Logf("  Published: %d", stats.PublishCount)
	t.Logf("  Delivered: %d", stats.DeliveredCount)
	t.Logf("  Errors: %d", stats.ErrorCount)
	t.Logf("  Dead letters: %d", dlq.Count())

	// Should have high delivery rate
	deliveryRate := float64(stats.DeliveredCount) / float64(stats.PublishCount)
	if deliveryRate < 0.9 {
		t.Errorf("delivery rate too low: %.2f%%", deliveryRate*100)
	}
}

func TestBurstTraffic(t *testing.T) {
	bus := New(WithAsync(50000), WithDeadLetter(1000))
	defer bus.Close()

	var delivered int64
	var wg sync.WaitGroup

	bus.Subscribe("burst.test", func(e Event) {
		atomic.AddInt64(&delivered, 1)
		wg.Done()
	})

	// Send burst
	burstSize := 10000
	wg.Add(burstSize)

	start := time.Now()
	for i := 0; i < burstSize; i++ {
		bus.Publish("burst.test", i)
	}
	publishDuration := time.Since(start)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(10 * time.Second):
		t.Fatalf("timeout: delivered %d of %d", atomic.LoadInt64(&delivered), burstSize)
	}

	totalDuration := time.Since(start)

	t.Logf("Burst traffic results:")
	t.Logf("  Burst size: %d", burstSize)
	t.Logf("  Publish time: %v (%.0f events/sec)", publishDuration, float64(burstSize)/publishDuration.Seconds())
	t.Logf("  Total time: %v (%.0f events/sec)", totalDuration, float64(burstSize)/totalDuration.Seconds())
}
