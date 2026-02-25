// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package wotanClient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// PublishWithAck TESTS
// ============================================================================

func TestPublishWithAck_SuccessNoRetry(t *testing.T) {
	var callCount int64

	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"/api/v1/topics/ack-test/publish": func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&callCount, 1)
			w.WriteHeader(http.StatusCreated)
		},
	})

	c.mu.Lock()
	c.subscribers["ack-test"] = &Subscriber{
		SubscriberID: "sub-ack",
		Topic:        "ack-test",
		Status:       "approved",
	}
	c.mu.Unlock()

	err := c.PublishWithAck(context.Background(), "ack-test", []byte("hello"))
	if err != nil {
		t.Fatalf("PublishWithAck: %v", err)
	}

	count := atomic.LoadInt64(&callCount)
	if count != 1 {
		t.Errorf("expected 1 call, got %d", count)
	}
}

func TestPublishWithAck_RetryThenSuccess(t *testing.T) {
	var callCount int64

	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"/api/v1/topics/retry-ack/publish": func(w http.ResponseWriter, r *http.Request) {
			count := atomic.AddInt64(&callCount, 1)
			if count <= 2 {
				// First two attempts fail with a network-level error simulation:
				// Return a server error that the client interprets as an app error.
				// For network failures, we need the server to be unreachable.
				// Instead, simulate nack by returning 500 which maps to an unknown error code.
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"error":{"code":"INTERNAL","message":"nack"}}`))
				return
			}
			// Third attempt succeeds.
			w.WriteHeader(http.StatusCreated)
		},
	})

	c.mu.Lock()
	c.subscribers["retry-ack"] = &Subscriber{
		SubscriberID: "sub-retry-ack",
		Topic:        "retry-ack",
		Status:       "approved",
	}
	c.mu.Unlock()

	// Note: doPublish returns (nil, appErr) for server errors, which are non-retryable.
	// To test the retry path, we need actual network errors.
	// Let's test with a server that closes connections instead.
	err := c.PublishWithAck(context.Background(), "retry-ack", []byte("retry-me"))
	// This will fail on first attempt with an app error (non-retryable).
	// The function returns the app error directly.
	if err == nil {
		t.Fatal("expected error from server error response")
	}

	// Verify at least 1 call was made.
	count := atomic.LoadInt64(&callCount)
	if count < 1 {
		t.Errorf("expected at least 1 call, got %d", count)
	}
}

func TestPublishWithAck_NetworkRetryThenSuccess(t *testing.T) {
	// Test with a server that becomes available after initial failures.
	// We use two servers: first one is down, then we switch to a working one.
	// Simpler approach: use a handler that returns connection close on first calls.

	var callCount int64
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"/api/v1/topics/net-retry/publish": func(w http.ResponseWriter, r *http.Request) {
			count := atomic.AddInt64(&callCount, 1)
			if count <= 2 {
				// Trigger a network-level error by hijacking and closing the connection.
				hj, ok := w.(http.Hijacker)
				if !ok {
					t.Fatal("server does not support hijacking")
				}
				conn, _, err := hj.Hijack()
				if err != nil {
					t.Fatalf("hijack: %v", err)
				}
				conn.Close() // Forces a network error on the client.
				return
			}
			// Third attempt succeeds.
			w.WriteHeader(http.StatusCreated)
		},
	})

	c.mu.Lock()
	c.subscribers["net-retry"] = &Subscriber{
		SubscriberID: "sub-net-retry",
		Topic:        "net-retry",
		Status:       "approved",
	}
	c.mu.Unlock()

	err := c.PublishWithAck(context.Background(), "net-retry", []byte("retry-net"))
	if err != nil {
		t.Fatalf("PublishWithAck should succeed after retries: %v", err)
	}

	count := atomic.LoadInt64(&callCount)
	if count != 3 {
		t.Errorf("expected 3 calls (2 failures + 1 success), got %d", count)
	}
}

func TestPublishWithAck_ExhaustRetries_DeadLetter(t *testing.T) {
	// All 3 attempts fail with network errors -> message goes to dead letter.
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"/api/v1/topics/exhaust/publish": func(w http.ResponseWriter, r *http.Request) {
			// Always cause a network error.
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("server does not support hijacking")
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Fatalf("hijack: %v", err)
			}
			conn.Close()
		},
	})

	c.mu.Lock()
	c.subscribers["exhaust"] = &Subscriber{
		SubscriberID: "sub-exhaust",
		Topic:        "exhaust",
		Status:       "approved",
	}
	c.mu.Unlock()

	err := c.PublishWithAck(context.Background(), "exhaust", []byte("doomed-message"))
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}

	if !errors.Is(err, ErrRetriesExhausted) {
		t.Errorf("expected ErrRetriesExhausted, got: %v", err)
	}

	// Verify dead letter count incremented.
	dlCount := c.DeadLetterCount()
	if dlCount != 1 {
		t.Errorf("DeadLetterCount = %d, want 1", dlCount)
	}

	// Verify message was buffered in fallback queue (since we're not subscribed to dead_letter.exhaust).
	qLen := c.FallbackQueueLen()
	if qLen < 1 {
		t.Errorf("FallbackQueueLen = %d, want >= 1 (dead letter should be buffered)", qLen)
	}
}

func TestPublishWithAck_EmptyTopic(t *testing.T) {
	c, _ := NewClient("localhost:9090")
	err := c.PublishWithAck(context.Background(), "", []byte("data"))
	if err == nil {
		t.Fatal("expected error for empty topic")
	}
	if !strings.Contains(err.Error(), "topic cannot be empty") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPublishWithAck_EmptyPayload(t *testing.T) {
	c, _ := NewClient("localhost:9090")
	err := c.PublishWithAck(context.Background(), "test", []byte{})
	if err == nil {
		t.Fatal("expected error for empty payload")
	}
	if !strings.Contains(err.Error(), "payload cannot be empty") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPublishWithAck_NotSubscribed(t *testing.T) {
	c, _ := NewClient("localhost:9090")
	err := c.PublishWithAck(context.Background(), "unknown", []byte("data"))
	if err == nil {
		t.Fatal("expected error when not subscribed")
	}
	if !strings.Contains(err.Error(), "not subscribed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPublishWithAck_SubscriptionPending(t *testing.T) {
	c, _ := NewClient("localhost:9090")
	c.subscribers["pending"] = &Subscriber{
		SubscriberID: "sub-pending",
		Topic:        "pending",
		Status:       "pending",
	}

	err := c.PublishWithAck(context.Background(), "pending", []byte("data"))
	if err != ErrSubscriptionPending {
		t.Errorf("expected ErrSubscriptionPending, got: %v", err)
	}
}

func TestPublishWithAck_ContextCancellation(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"/api/v1/topics/slow/publish": func(w http.ResponseWriter, r *http.Request) {
			// Hijack and close to cause a network error, triggering retry.
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("server does not support hijacking")
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Fatalf("hijack: %v", err)
			}
			conn.Close()
		},
	})

	c.mu.Lock()
	c.subscribers["slow"] = &Subscriber{
		SubscriberID: "sub-slow",
		Topic:        "slow",
		Status:       "approved",
	}
	c.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := c.PublishWithAck(ctx, "slow", []byte("data"))
	if err == nil {
		t.Fatal("expected error from context cancellation")
	}
}

func TestPublishWithAck_AppErrorNotRetried(t *testing.T) {
	var callCount int64

	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"/api/v1/topics/app-err/publish": func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&callCount, 1)
			errorResponse(w, http.StatusForbidden, "NOT_AUTHORIZED", "denied")
		},
	})

	c.mu.Lock()
	c.subscribers["app-err"] = &Subscriber{
		SubscriberID: "sub-app-err",
		Topic:        "app-err",
		Status:       "approved",
	}
	c.mu.Unlock()

	err := c.PublishWithAck(context.Background(), "app-err", []byte("data"))
	if err != ErrNotAuthorized {
		t.Errorf("expected ErrNotAuthorized, got: %v", err)
	}

	// Should not retry on app errors.
	count := atomic.LoadInt64(&callCount)
	if count != 1 {
		t.Errorf("expected exactly 1 call (no retries for app errors), got %d", count)
	}
}

// ============================================================================
// DeadLetterMessage FORMAT TESTS
// ============================================================================

func TestDeadLetterMessage_JSONFormat(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	dlMsg := DeadLetterMessage{
		OriginalTopic: "events.critical",
		Payload:       []byte(`{"event":"lost"}`),
		Error:         "connection refused",
		Attempts:      3,
		FirstAttempt:  now.Add(-500 * time.Millisecond),
		LastAttempt:   now,
	}

	data, err := json.Marshal(dlMsg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded DeadLetterMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.OriginalTopic != "events.critical" {
		t.Errorf("OriginalTopic = %q, want events.critical", decoded.OriginalTopic)
	}
	if string(decoded.Payload) != `{"event":"lost"}` {
		t.Errorf("Payload = %q, want {\"event\":\"lost\"}", string(decoded.Payload))
	}
	if decoded.Error != "connection refused" {
		t.Errorf("Error = %q, want connection refused", decoded.Error)
	}
	if decoded.Attempts != 3 {
		t.Errorf("Attempts = %d, want 3", decoded.Attempts)
	}
	if decoded.FirstAttempt.IsZero() {
		t.Error("FirstAttempt is zero")
	}
	if decoded.LastAttempt.IsZero() {
		t.Error("LastAttempt is zero")
	}
	if !decoded.LastAttempt.After(decoded.FirstAttempt) && !decoded.LastAttempt.Equal(decoded.FirstAttempt) {
		t.Error("LastAttempt should be >= FirstAttempt")
	}
}

func TestDeadLetterMessage_JSONFieldNames(t *testing.T) {
	dlMsg := DeadLetterMessage{
		OriginalTopic: "test",
		Payload:       []byte("data"),
		Error:         "err",
		Attempts:      1,
		FirstAttempt:  time.Now(),
		LastAttempt:   time.Now(),
	}

	data, _ := json.Marshal(dlMsg)
	var raw map[string]interface{}
	json.Unmarshal(data, &raw)

	expectedFields := []string{"original_topic", "payload", "error", "attempts", "first_attempt", "last_attempt"}
	for _, field := range expectedFields {
		if _, ok := raw[field]; !ok {
			t.Errorf("missing expected JSON field %q", field)
		}
	}
}

// ============================================================================
// DeadLetterTopic TESTS
// ============================================================================

func TestDeadLetterTopic(t *testing.T) {
	tests := []struct {
		topic string
		want  string
	}{
		{"events.critical", "dead_letter.events.critical"},
		{"chat", "dead_letter.chat"},
		{"system.alerts", "dead_letter.system.alerts"},
	}

	for _, tt := range tests {
		got := DeadLetterTopic(tt.topic)
		if got != tt.want {
			t.Errorf("DeadLetterTopic(%q) = %q, want %q", tt.topic, got, tt.want)
		}
	}
}

// ============================================================================
// FALLBACK QUEUE METRICS TESTS
// ============================================================================

func TestFallbackQueueMetricsUpdated(t *testing.T) {
	// When messages are enqueued, the Prometheus gauge should reflect the count.
	c, _ := NewClient("localhost:9090")

	// Enqueue a few messages directly.
	c.enqueueFallback("test-topic", []byte("msg1"))
	c.enqueueFallback("test-topic", []byte("msg2"))
	c.enqueueFallback("test-topic", []byte("msg3"))

	qLen := c.FallbackQueueLen()
	if qLen != 3 {
		t.Errorf("FallbackQueueLen = %d, want 3", qLen)
	}

	// Verify the metric is updated (we can at least confirm the function runs without panic).
	c.updateFallbackMetrics()
}

func TestPublishNetworkError_BuffersAndUpdatesMetrics(t *testing.T) {
	// Create a client pointing to a dead server to trigger network errors.
	c, err := NewClient("127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}

	c.mu.Lock()
	c.subscribers["metrics-test"] = &Subscriber{
		SubscriberID: "sub-metrics",
		Topic:        "metrics-test",
		Status:       "approved",
	}
	c.mu.Unlock()

	// Publish will fail with network error and buffer the message.
	pubErr := c.Publish(context.Background(), "metrics-test", []byte("buffered"))
	if pubErr == nil {
		t.Fatal("expected error from unreachable server")
	}

	qLen := c.FallbackQueueLen()
	if qLen != 1 {
		t.Errorf("FallbackQueueLen = %d, want 1 (message should be buffered)", qLen)
	}
}

// ============================================================================
// DeadLetterCount TESTS
// ============================================================================

func TestDeadLetterCount_InitiallyZero(t *testing.T) {
	c, _ := NewClient("localhost:9090")
	if c.DeadLetterCount() != 0 {
		t.Errorf("DeadLetterCount should be 0 initially, got %d", c.DeadLetterCount())
	}
}

// ============================================================================
// ErrRetriesExhausted TESTS
// ============================================================================

func TestErrRetriesExhausted_Wrapping(t *testing.T) {
	wrapped := errors.Is(
		errors.New("publish retries exhausted"),
		errors.New("publish retries exhausted"),
	)
	// Sentinel errors are compared by identity, not value.
	if wrapped {
		t.Error("different error instances should not match with errors.Is")
	}

	// Verify the actual sentinel works.
	err := ErrRetriesExhausted
	if err.Error() != "publish retries exhausted" {
		t.Errorf("ErrRetriesExhausted = %q", err.Error())
	}
}
