// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package wotanClient

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestOrderedPublisher_FIFODelivery(t *testing.T) {
	// Track the order messages arrive at the server.
	var mu sync.Mutex
	var received []string

	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"/api/v1/topics/ordered/publish": func(w http.ResponseWriter, r *http.Request) {
			// Small delay to make ordering observable.
			time.Sleep(5 * time.Millisecond)

			body := readRequestBody(t, r)
			mu.Lock()
			received = append(received, body)
			mu.Unlock()

			w.WriteHeader(http.StatusCreated)
		},
	})

	c.mu.Lock()
	c.subscribers["ordered"] = &Subscriber{
		SubscriberID: "sub-ordered",
		Topic:        "ordered",
		Status:       "approved",
	}
	c.mu.Unlock()

	op := NewOrderedPublisher(c)
	defer op.Close()

	// Send 5 messages — they must arrive in order.
	messages := []string{"msg-A", "msg-B", "msg-C", "msg-D", "msg-E"}
	for _, msg := range messages {
		err := op.Publish(context.Background(), "service-x", "ordered", []byte(msg))
		if err != nil {
			t.Fatalf("Publish(%s): %v", msg, err)
		}
	}

	mu.Lock()
	defer mu.Unlock()

	if len(received) != 5 {
		t.Fatalf("expected 5 messages, got %d", len(received))
	}

	// Verify order: each received body should contain the payload in order.
	for i, msg := range messages {
		if !containsPayload(received[i], msg) {
			t.Errorf("message %d: expected payload containing %q, got %q", i, msg, received[i])
		}
	}
}

func TestOrderedPublisher_DifferentDestinations_Independent(t *testing.T) {
	var callCount int64

	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"/api/v1/topics/dest-a/publish": func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&callCount, 1)
			w.WriteHeader(http.StatusCreated)
		},
		"/api/v1/topics/dest-b/publish": func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&callCount, 1)
			w.WriteHeader(http.StatusCreated)
		},
	})

	c.mu.Lock()
	c.subscribers["dest-a"] = &Subscriber{
		SubscriberID: "sub-a",
		Topic:        "dest-a",
		Status:       "approved",
	}
	c.subscribers["dest-b"] = &Subscriber{
		SubscriberID: "sub-b",
		Topic:        "dest-b",
		Status:       "approved",
	}
	c.mu.Unlock()

	op := NewOrderedPublisher(c)
	defer op.Close()

	// Send to two different destinations concurrently.
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 3; i++ {
			op.Publish(context.Background(), "dest-a", "dest-a", []byte("a-msg"))
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 3; i++ {
			op.Publish(context.Background(), "dest-b", "dest-b", []byte("b-msg"))
		}
	}()

	wg.Wait()

	count := atomic.LoadInt64(&callCount)
	if count != 6 {
		t.Errorf("expected 6 total publishes, got %d", count)
	}
}

func TestOrderedPublisher_QueueDepth(t *testing.T) {
	op := NewOrderedPublisher(nil)
	defer op.Close()

	// No queue created yet.
	if depth := op.QueueDepth("unknown"); depth != 0 {
		t.Errorf("QueueDepth for unknown = %d, want 0", depth)
	}
}

func TestOrderedPublisher_ContextCancellation(t *testing.T) {
	// Create a server that blocks forever.
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"/api/v1/topics/slow/publish": func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(10 * time.Second)
			w.WriteHeader(http.StatusCreated)
		},
	})

	c.mu.Lock()
	c.subscribers["slow"] = &Subscriber{
		SubscriberID: "sub-slow",
		Topic:        "slow",
		Status:       "approved",
	}
	c.mu.Unlock()

	op := NewOrderedPublisher(c)
	defer op.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := op.Publish(ctx, "slow-dest", "slow", []byte("data"))
	if err == nil {
		t.Fatal("expected error from context cancellation")
	}
}

func TestOrderedPublisher_CloseErrorsRemaining(t *testing.T) {
	op := NewOrderedPublisher(nil)

	// Create a queue but don't process anything.
	_ = op.getOrCreateQueue("test-dest")

	op.Close()

	// Double close is safe.
	op.Close()
}

// containsPayload checks if the JSON body contains the expected payload string.
func containsPayload(body, payload string) bool {
	return len(body) > 0 && (body == payload || // direct match
		len(body) > len(payload) && // JSON wrapping
		indexOfSubstring(body, payload) >= 0)
}

func indexOfSubstring(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// readRequestBody reads the full body of an HTTP request as a string.
func readRequestBody(t *testing.T, r *http.Request) string {
	t.Helper()
	buf := make([]byte, 4096)
	n, _ := r.Body.Read(buf)
	return string(buf[:n])
}
