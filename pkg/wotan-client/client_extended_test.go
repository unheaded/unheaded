// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package wotanClient

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"
	chatpb "unheaded/services/wotan/proto"
)

// ============================================================================
// BACKOFF TESTS
// ============================================================================

func TestBackoff_ZeroOrNegativeReturnsMin(t *testing.T) {
	if d := backoff(0); d != pollBackoffMin {
		t.Errorf("backoff(0) = %v, want %v", d, pollBackoffMin)
	}
	if d := backoff(-5); d != pollBackoffMin {
		t.Errorf("backoff(-5) = %v, want %v", d, pollBackoffMin)
	}
}

func TestBackoff_ExponentialGrowth(t *testing.T) {
	d1 := backoff(1)
	d2 := backoff(2)
	d3 := backoff(3)

	if d1 >= d2 {
		t.Errorf("backoff should grow: backoff(1)=%v >= backoff(2)=%v", d1, d2)
	}
	if d2 >= d3 {
		t.Errorf("backoff should grow: backoff(2)=%v >= backoff(3)=%v", d2, d3)
	}
}

func TestBackoff_CapsAtMax(t *testing.T) {
	// Very high failure count should cap at pollBackoffMax.
	d := backoff(100)
	if d > pollBackoffMax {
		t.Errorf("backoff(100) = %v, exceeds max %v", d, pollBackoffMax)
	}
	if d != pollBackoffMax {
		t.Errorf("backoff(100) = %v, want capped at %v", d, pollBackoffMax)
	}
}

// ============================================================================
// FallbackDrops TESTS
// ============================================================================

func TestFallbackDrops_InitiallyZero(t *testing.T) {
	c, _ := NewClient("localhost:9090")
	defer c.Close()

	if drops := c.FallbackDrops(); drops != 0 {
		t.Errorf("FallbackDrops() = %d, want 0", drops)
	}
}

func TestFallbackDrops_IncrementOnQueueFull(t *testing.T) {
	c, _ := NewClient("localhost:9090")
	defer c.Close()

	// Fill the queue past maxFallbackQueue.
	for i := 0; i < maxFallbackQueue+5; i++ {
		c.enqueueFallback("test", []byte(fmt.Sprintf("msg-%d", i)))
	}

	drops := c.FallbackDrops()
	if drops != 5 {
		t.Errorf("FallbackDrops() = %d, want 5", drops)
	}

	// Queue length should be capped at maxFallbackQueue.
	if qLen := c.FallbackQueueLen(); qLen != maxFallbackQueue {
		t.Errorf("FallbackQueueLen() = %d, want %d", qLen, maxFallbackQueue)
	}
}

// ============================================================================
// FlushFallbackQueue TESTS
// ============================================================================

func TestFlushFallbackQueue_Empty(t *testing.T) {
	c, _ := NewClient("localhost:9090")
	defer c.Close()

	flushed, err := c.FlushFallbackQueue(context.Background())
	if flushed != 0 {
		t.Errorf("flushed = %d, want 0", flushed)
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFlushFallbackQueue_SuccessfulFlush(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"/api/v1/topics/flush-topic/publish": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
		},
	})

	c.mu.Lock()
	c.subscribers["flush-topic"] = &Subscriber{
		SubscriberID: "sub-flush",
		Topic:        "flush-topic",
		Status:       "approved",
	}
	c.mu.Unlock()

	// Enqueue some messages.
	c.enqueueFallback("flush-topic", []byte("msg-1"))
	c.enqueueFallback("flush-topic", []byte("msg-2"))
	c.enqueueFallback("flush-topic", []byte("msg-3"))

	if c.FallbackQueueLen() != 3 {
		t.Fatalf("expected 3 queued, got %d", c.FallbackQueueLen())
	}

	flushed, err := c.FlushFallbackQueue(context.Background())
	if err != nil {
		t.Fatalf("FlushFallbackQueue: %v", err)
	}
	if flushed != 3 {
		t.Errorf("flushed = %d, want 3", flushed)
	}
	if c.FallbackQueueLen() != 0 {
		t.Errorf("queue should be empty after flush, got %d", c.FallbackQueueLen())
	}
}

func TestFlushFallbackQueue_NetworkErrorRetainsMessages(t *testing.T) {
	// Point at a dead server so all flushes get network errors.
	c, err := NewClient("127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	c.mu.Lock()
	c.subscribers["net-fail"] = &Subscriber{
		SubscriberID: "sub-net-fail",
		Topic:        "net-fail",
		Status:       "approved",
	}
	c.mu.Unlock()

	c.enqueueFallback("net-fail", []byte("msg-1"))
	c.enqueueFallback("net-fail", []byte("msg-2"))

	flushed, flushErr := c.FlushFallbackQueue(context.Background())
	if flushed != 0 {
		t.Errorf("flushed = %d, want 0 (all should fail)", flushed)
	}
	if flushErr == nil {
		t.Error("expected error from flush against dead server")
	}

	// Messages should be re-enqueued.
	if c.FallbackQueueLen() != 2 {
		t.Errorf("FallbackQueueLen = %d, want 2 (re-enqueued)", c.FallbackQueueLen())
	}
}

func TestFlushFallbackQueue_UnsubscribedTopicDropped(t *testing.T) {
	c, _ := NewClient("localhost:9090")
	defer c.Close()

	// Enqueue a message for a topic we're NOT subscribed to.
	c.enqueueFallback("no-sub-topic", []byte("orphaned"))

	flushed, err := c.FlushFallbackQueue(context.Background())
	if flushed != 0 {
		t.Errorf("flushed = %d, want 0", flushed)
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	// The message should be dropped (not re-enqueued).
	if c.FallbackQueueLen() != 0 {
		t.Errorf("FallbackQueueLen = %d, want 0 (unsubscribed messages should be dropped)", c.FallbackQueueLen())
	}
}

func TestFlushFallbackQueue_AppErrorDropsMessage(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"/api/v1/topics/app-err-flush/publish": func(w http.ResponseWriter, r *http.Request) {
			errorResponse(w, http.StatusForbidden, "NOT_AUTHORIZED", "denied")
		},
	})

	c.mu.Lock()
	c.subscribers["app-err-flush"] = &Subscriber{
		SubscriberID: "sub-app-err-flush",
		Topic:        "app-err-flush",
		Status:       "approved",
	}
	c.mu.Unlock()

	c.enqueueFallback("app-err-flush", []byte("will-be-rejected"))

	flushed, err := c.FlushFallbackQueue(context.Background())
	if flushed != 0 {
		t.Errorf("flushed = %d, want 0 (app error)", flushed)
	}
	if err == nil {
		t.Error("expected error from app rejection")
	}
	// App errors cause drop, not re-enqueue.
	if c.FallbackQueueLen() != 0 {
		t.Errorf("FallbackQueueLen = %d, want 0 (app error should drop)", c.FallbackQueueLen())
	}
}

// ============================================================================
// safeChannel.send TESTS
// ============================================================================

func TestSafeChannel_SendAfterClose(t *testing.T) {
	sc := newSafeChannel(10)
	sc.closeCh()

	msg := &Message{MessageID: "test"}
	sent := sc.send(context.Background(), msg)
	if sent {
		t.Error("send should return false after channel is closed")
	}
}

func TestSafeChannel_SendContextCancelled(t *testing.T) {
	sc := newSafeChannel(0) // unbuffered — will block

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	msg := &Message{MessageID: "test"}
	sent := sc.send(ctx, msg)
	if sent {
		t.Error("send should return false when context is cancelled")
	}
}

func TestSafeChannel_DoubleClose(t *testing.T) {
	sc := newSafeChannel(10)
	sc.closeCh()
	sc.closeCh() // should not panic
}

// ============================================================================
// isTransientError TESTS
// ============================================================================

func TestIsTransientError_Nil(t *testing.T) {
	if isTransientError(nil) {
		t.Error("nil error should not be transient")
	}
}

func TestIsTransientError_Unavailable(t *testing.T) {
	err := status.Error(codes.Unavailable, "service unavailable")
	if !isTransientError(err) {
		t.Error("Unavailable should be transient")
	}
}

func TestIsTransientError_ResourceExhausted(t *testing.T) {
	err := status.Error(codes.ResourceExhausted, "resource exhausted")
	if !isTransientError(err) {
		t.Error("ResourceExhausted should be transient")
	}
}

func TestIsTransientError_DeadlineExceeded_GRPC(t *testing.T) {
	err := status.Error(codes.DeadlineExceeded, "deadline")
	if !isTransientError(err) {
		t.Error("DeadlineExceeded should be transient")
	}
}

func TestIsTransientError_ContextDeadlineExceeded(t *testing.T) {
	if !isTransientError(context.DeadlineExceeded) {
		t.Error("context.DeadlineExceeded should be transient")
	}
}

func TestIsTransientError_NotFound(t *testing.T) {
	err := status.Error(codes.NotFound, "not found")
	if isTransientError(err) {
		t.Error("NotFound should not be transient")
	}
}

func TestIsTransientError_PermissionDenied(t *testing.T) {
	err := status.Error(codes.PermissionDenied, "denied")
	if isTransientError(err) {
		t.Error("PermissionDenied should not be transient")
	}
}

func TestIsTransientError_PlainError(t *testing.T) {
	err := errors.New("some random error")
	if isTransientError(err) {
		t.Error("plain error should not be transient")
	}
}

func TestIsTransientError_WrappedDeadlineExceeded(t *testing.T) {
	err := fmt.Errorf("wrapped: %w", context.DeadlineExceeded)
	if !isTransientError(err) {
		t.Error("wrapped context.DeadlineExceeded should be transient")
	}
}

// ============================================================================
// topicEventToMessage TESTS
// ============================================================================

func TestTopicEventToMessage_Nil(t *testing.T) {
	msg := topicEventToMessage(nil)
	if msg != nil {
		t.Error("expected nil for nil event")
	}
}

func TestTopicEventToMessage_WithTimestamp(t *testing.T) {
	now := time.Now()
	event := &chatpb.TopicEvent{
		MessageId: "msg-123",
		Topic:     "tasks.created",
		SenderId:  "sender-1",
		Seq:       42,
		Payload:   []byte("hello"),
		CreatedAt: timestamppb.New(now),
		Type:      chatpb.TopicEventType_TOPIC_MESSAGE_PUBLISHED,
	}

	msg := topicEventToMessage(event)
	if msg == nil {
		t.Fatal("expected non-nil message")
	}
	if msg.MessageID != "msg-123" {
		t.Errorf("MessageID = %q, want msg-123", msg.MessageID)
	}
	if msg.Topic != "tasks.created" {
		t.Errorf("Topic = %q, want tasks.created", msg.Topic)
	}
	if msg.SenderID != "sender-1" {
		t.Errorf("SenderID = %q, want sender-1", msg.SenderID)
	}
	if msg.Seq != 42 {
		t.Errorf("Seq = %d, want 42", msg.Seq)
	}
	if msg.Payload != "hello" {
		t.Errorf("Payload = %q, want hello", msg.Payload)
	}
	if msg.Deleted {
		t.Error("expected Deleted=false for PUBLISHED type")
	}
}

func TestTopicEventToMessage_Deleted(t *testing.T) {
	event := &chatpb.TopicEvent{
		MessageId: "msg-del",
		Topic:     "tasks.deleted",
		Seq:       99,
		Payload:   []byte("removed"),
		CreatedAt: timestamppb.Now(),
		Type:      chatpb.TopicEventType_TOPIC_MESSAGE_DELETED,
	}

	msg := topicEventToMessage(event)
	if msg == nil {
		t.Fatal("expected non-nil message")
	}
	if !msg.Deleted {
		t.Error("expected Deleted=true for DELETED type")
	}
}

func TestTopicEventToMessage_NoTimestamp(t *testing.T) {
	event := &chatpb.TopicEvent{
		MessageId: "msg-notime",
		Topic:     "test",
		Seq:       1,
		Payload:   []byte("data"),
		CreatedAt: nil, // no timestamp
	}

	msg := topicEventToMessage(event)
	if msg == nil {
		t.Fatal("expected non-nil message")
	}
	// Should use time.Now() as fallback, so CreatedAt should be recent.
	if time.Since(msg.CreatedAt) > 5*time.Second {
		t.Errorf("CreatedAt should be recent, got %v", msg.CreatedAt)
	}
}

// ============================================================================
// GRPCClient TESTS (using bufconn)
// ============================================================================

func setupGRPCBufconn(t *testing.T) (*mockTopicStreamServer, *GRPCClient) {
	t.Helper()

	const bufSize = 1024 * 1024
	lis := bufconn.Listen(bufSize)

	srv := grpc.NewServer()
	mockSrv := newMockServer()
	chatpb.RegisterTopicStreamServer(srv, mockSrv)

	go func() {
		if err := srv.Serve(lis); err != nil {
			// expected during cleanup
		}
	}()
	t.Cleanup(func() { srv.Stop() })

	conn, err := grpc.NewClient(
		"passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial bufconn: %v", err)
	}

	gc := &GRPCClient{
		conn:        conn,
		client:      chatpb.NewTopicStreamClient(conn),
		subscribers: make(map[string]*Subscriber),
		channels:    make(map[string]*safeChannel),
		maxRetries:  3,
		retryDelay:  10 * time.Millisecond,
	}
	t.Cleanup(func() { gc.Close() })

	return mockSrv, gc
}

func TestGRPCClient_Subscribe(t *testing.T) {
	_, gc := setupGRPCBufconn(t)
	ctx := context.Background()

	sub, err := gc.Subscribe(ctx, "tasks.created", "test-sub")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if sub.Topic != "tasks.created" {
		t.Errorf("Topic = %q, want tasks.created", sub.Topic)
	}
	if sub.Status != "approved" {
		t.Errorf("Status = %q, want approved", sub.Status)
	}
	if sub.SubscriberID == "" {
		t.Error("SubscriberID should be non-empty")
	}
	if sub.DisplayName != "test-sub" {
		t.Errorf("DisplayName = %q, want test-sub", sub.DisplayName)
	}
}

func TestGRPCClient_Subscribe_EmptyTopic(t *testing.T) {
	_, gc := setupGRPCBufconn(t)
	_, err := gc.Subscribe(context.Background(), "", "test")
	if err == nil {
		t.Error("expected error for empty topic")
	}
}

func TestGRPCClient_Subscribe_EmptyDisplayName(t *testing.T) {
	_, gc := setupGRPCBufconn(t)
	_, err := gc.Subscribe(context.Background(), "tasks", "")
	if err == nil {
		t.Error("expected error for empty display name")
	}
}

func TestGRPCClient_Publish(t *testing.T) {
	mockSrv, gc := setupGRPCBufconn(t)
	ctx := context.Background()

	gc.Subscribe(ctx, "tasks.created", "publisher")

	err := gc.Publish(ctx, "tasks.created", []byte(`{"id":"task-1"}`))
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}

	if mockSrv.publishCount.Load() != 1 {
		t.Errorf("expected 1 publish, got %d", mockSrv.publishCount.Load())
	}
}

func TestGRPCClient_Publish_EmptyTopic(t *testing.T) {
	_, gc := setupGRPCBufconn(t)
	err := gc.Publish(context.Background(), "", []byte("data"))
	if err == nil {
		t.Error("expected error for empty topic")
	}
}

func TestGRPCClient_Publish_EmptyPayload(t *testing.T) {
	_, gc := setupGRPCBufconn(t)
	gc.Subscribe(context.Background(), "test", "pub")
	err := gc.Publish(context.Background(), "test", nil)
	if err == nil {
		t.Error("expected error for empty payload")
	}
}

func TestGRPCClient_Publish_NotSubscribed(t *testing.T) {
	_, gc := setupGRPCBufconn(t)
	err := gc.Publish(context.Background(), "unknown", []byte("data"))
	if err == nil {
		t.Error("expected error for unsubscribed topic")
	}
}

func TestGRPCClient_Publish_PendingSubscription(t *testing.T) {
	_, gc := setupGRPCBufconn(t)
	gc.mu.Lock()
	gc.subscribers["pending"] = &Subscriber{
		SubscriberID: "sub-p",
		Topic:        "pending",
		Status:       "pending",
	}
	gc.mu.Unlock()

	err := gc.Publish(context.Background(), "pending", []byte("data"))
	if err != ErrSubscriptionPending {
		t.Errorf("expected ErrSubscriptionPending, got: %v", err)
	}
}

func TestGRPCClient_GetSubscriber(t *testing.T) {
	_, gc := setupGRPCBufconn(t)
	ctx := context.Background()

	gc.Subscribe(ctx, "tasks.created", "tester")

	sub, ok := gc.GetSubscriber("tasks.created")
	if !ok {
		t.Error("expected subscriber to exist")
	}
	if sub.DisplayName != "tester" {
		t.Errorf("DisplayName = %q, want tester", sub.DisplayName)
	}

	_, ok = gc.GetSubscriber("nonexistent")
	if ok {
		t.Error("expected subscriber to not exist")
	}
}

func TestGRPCClient_IsApproved(t *testing.T) {
	_, gc := setupGRPCBufconn(t)
	ctx := context.Background()

	gc.Subscribe(ctx, "tasks.created", "tester")

	if !gc.IsApproved("tasks.created") {
		t.Error("expected approved after subscribe")
	}
	if gc.IsApproved("nonexistent") {
		t.Error("expected not approved for nonexistent topic")
	}
}

func TestGRPCClient_GetMessages(t *testing.T) {
	mockSrv, gc := setupGRPCBufconn(t)
	ctx := context.Background()

	gc.Subscribe(ctx, "tasks.created", "reader")

	mockSrv.addMessage("tasks.created", 1, `{"id":"task-1"}`)
	mockSrv.addMessage("tasks.created", 2, `{"id":"task-2"}`)

	msgs, err := gc.GetMessages(ctx, "tasks.created", 0, 10)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages, got %d", len(msgs))
	}
}

func TestGRPCClient_GetMessages_EmptyTopic(t *testing.T) {
	_, gc := setupGRPCBufconn(t)
	_, err := gc.GetMessages(context.Background(), "", 0, 10)
	if err == nil {
		t.Error("expected error for empty topic")
	}
}

func TestGRPCClient_GetMessages_NotSubscribed(t *testing.T) {
	_, gc := setupGRPCBufconn(t)
	_, err := gc.GetMessages(context.Background(), "unknown", 0, 10)
	if err == nil {
		t.Error("expected error for unsubscribed topic")
	}
}

func TestGRPCClient_GetMessages_WithLimit(t *testing.T) {
	mockSrv, gc := setupGRPCBufconn(t)
	ctx := context.Background()

	gc.Subscribe(ctx, "tasks.created", "reader")

	mockSrv.addMessage("tasks.created", 1, `msg-1`)
	mockSrv.addMessage("tasks.created", 2, `msg-2`)
	mockSrv.addMessage("tasks.created", 3, `msg-3`)

	msgs, err := gc.GetMessages(ctx, "tasks.created", 0, 2)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages (limited), got %d", len(msgs))
	}
}

func TestGRPCClient_GetMessages_AfterSeq(t *testing.T) {
	mockSrv, gc := setupGRPCBufconn(t)
	ctx := context.Background()

	gc.Subscribe(ctx, "tasks.created", "reader")

	mockSrv.addMessage("tasks.created", 1, `old`)
	mockSrv.addMessage("tasks.created", 2, `old`)
	mockSrv.addMessage("tasks.created", 3, `new`)

	msgs, err := gc.GetMessages(ctx, "tasks.created", 2, 10)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(msgs) != 1 {
		t.Errorf("expected 1 message after seq 2, got %d", len(msgs))
	}
}

func TestGRPCClient_StreamMessages(t *testing.T) {
	mockSrv, gc := setupGRPCBufconn(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	gc.Subscribe(ctx, "tasks.created", "streamer")

	mockSrv.addMessage("tasks.created", 1, `msg-1`)
	mockSrv.addMessage("tasks.created", 2, `msg-2`)

	ch, err := gc.StreamMessages(ctx, "tasks.created")
	if err != nil {
		t.Fatalf("StreamMessages: %v", err)
	}

	var received []*Message
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()

	for len(received) < 2 {
		select {
		case msg, ok := <-ch:
			if !ok {
				t.Fatal("channel closed unexpectedly")
			}
			received = append(received, msg)
		case <-timer.C:
			t.Fatalf("timed out, got %d messages", len(received))
		}
	}

	if len(received) != 2 {
		t.Errorf("expected 2 messages, got %d", len(received))
	}
}

func TestGRPCClient_StreamMessages_EmptyTopic(t *testing.T) {
	_, gc := setupGRPCBufconn(t)
	_, err := gc.StreamMessages(context.Background(), "")
	if err == nil {
		t.Error("expected error for empty topic")
	}
}

func TestGRPCClient_StreamMessages_NotSubscribed(t *testing.T) {
	_, gc := setupGRPCBufconn(t)
	_, err := gc.StreamMessages(context.Background(), "unknown")
	if err == nil {
		t.Error("expected error for unsubscribed topic")
	}
}

func TestGRPCClient_StreamMessages_PendingSubscription(t *testing.T) {
	_, gc := setupGRPCBufconn(t)
	gc.mu.Lock()
	gc.subscribers["pending"] = &Subscriber{
		SubscriberID: "sub-p",
		Topic:        "pending",
		Status:       "pending",
	}
	gc.mu.Unlock()

	_, err := gc.StreamMessages(context.Background(), "pending")
	if err != ErrSubscriptionPending {
		t.Errorf("expected ErrSubscriptionPending, got: %v", err)
	}
}

func TestGRPCClient_Close(t *testing.T) {
	_, gc := setupGRPCBufconn(t)

	err := gc.Close()
	if err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestGRPCClient_Close_NilConn(t *testing.T) {
	gc := &GRPCClient{
		subscribers: make(map[string]*Subscriber),
		channels:    make(map[string]*safeChannel),
	}
	err := gc.Close()
	if err != nil {
		t.Fatalf("Close with nil conn: %v", err)
	}
}

func TestGRPCClient_Close_WithActiveChannels(t *testing.T) {
	_, gc := setupGRPCBufconn(t)

	// Add a channel manually.
	sc := newSafeChannel(10)
	gc.chanMu.Lock()
	gc.channels["test-topic"] = sc
	gc.chanMu.Unlock()

	err := gc.Close()
	if err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Channel should be closed.
	select {
	case <-sc.done:
		// expected
	default:
		t.Error("expected channel to be closed after Close()")
	}
}

// ============================================================================
// NewGRPCClient TESTS
// ============================================================================

func TestNewGRPCClient_EmptyAddr(t *testing.T) {
	_, err := NewGRPCClient("")
	if err == nil {
		t.Error("expected error for empty address")
	}
}

func TestNewGRPCClientWithTLS_EmptyAddr(t *testing.T) {
	_, err := NewGRPCClientWithTLS("", nil)
	if err == nil {
		t.Error("expected error for empty address")
	}
}

func TestNewGRPCClientWithTLS_NilCreds(t *testing.T) {
	_, err := NewGRPCClientWithTLS("localhost:9090", nil)
	if err == nil {
		t.Error("expected error for nil credentials")
	}
}

// ============================================================================
// publishDeadLetter TESTS
// ============================================================================

func TestPublishDeadLetter_ToSubscribedDLTopic(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"/api/v1/topics/dead_letter.my-topic/publish": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
		},
	})

	c.mu.Lock()
	c.subscribers["dead_letter.my-topic"] = &Subscriber{
		SubscriberID: "sub-dl",
		Topic:        "dead_letter.my-topic",
		Status:       "approved",
	}
	c.mu.Unlock()

	lastErr := errors.New("connection refused")
	now := time.Now()
	c.publishDeadLetter(context.Background(), "my-topic", []byte("payload"), lastErr, 3, now.Add(-time.Second), now)

	if c.DeadLetterCount() != 1 {
		t.Errorf("DeadLetterCount = %d, want 1", c.DeadLetterCount())
	}
}

func TestPublishDeadLetter_DLTopicNetworkError(t *testing.T) {
	// DL topic publish fails with network error — should buffer.
	c, err := NewClient("127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	c.mu.Lock()
	c.subscribers["dead_letter.fail-topic"] = &Subscriber{
		SubscriberID: "sub-dl-fail",
		Topic:        "dead_letter.fail-topic",
		Status:       "approved",
	}
	c.mu.Unlock()

	lastErr := errors.New("connection refused")
	now := time.Now()
	c.publishDeadLetter(context.Background(), "fail-topic", []byte("payload"), lastErr, 3, now.Add(-time.Second), now)

	if c.DeadLetterCount() != 1 {
		t.Errorf("DeadLetterCount = %d, want 1", c.DeadLetterCount())
	}
	// Should be buffered in fallback queue.
	if c.FallbackQueueLen() < 1 {
		t.Errorf("FallbackQueueLen = %d, want >= 1", c.FallbackQueueLen())
	}
}

func TestPublishDeadLetter_DLTopicAppError(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"/api/v1/topics/dead_letter.app-err-topic/publish": func(w http.ResponseWriter, r *http.Request) {
			errorResponse(w, http.StatusForbidden, "NOT_AUTHORIZED", "denied")
		},
	})

	c.mu.Lock()
	c.subscribers["dead_letter.app-err-topic"] = &Subscriber{
		SubscriberID: "sub-dl-app",
		Topic:        "dead_letter.app-err-topic",
		Status:       "approved",
	}
	c.mu.Unlock()

	lastErr := errors.New("original error")
	now := time.Now()
	c.publishDeadLetter(context.Background(), "app-err-topic", []byte("payload"), lastErr, 3, now.Add(-time.Second), now)

	// Should be buffered because publish returned an app error.
	if c.FallbackQueueLen() < 1 {
		t.Errorf("FallbackQueueLen = %d, want >= 1 (app error on DL topic should buffer)", c.FallbackQueueLen())
	}
}

// ============================================================================
// NewClientWithGRPC TESTS — improve from 40% to higher
// ============================================================================

func TestNewClientWithGRPC_ValidAddrs(t *testing.T) {
	// gRPC probe will "succeed" because grpc.NewClient uses lazy connections.
	c, err := NewClientWithGRPC("localhost:18000", "localhost:18001")
	if err != nil {
		t.Fatalf("NewClientWithGRPC: %v", err)
	}
	defer c.Close()

	if c.baseURL != "http://localhost:18000/api/v1" {
		t.Errorf("baseURL = %q, want http://localhost:18000/api/v1", c.baseURL)
	}
	if c.grpcAddr != "localhost:18001" {
		t.Errorf("grpcAddr = %q, want localhost:18001", c.grpcAddr)
	}
}

// ============================================================================
// NewClientWithTLS TESTS
// ============================================================================

func TestNewClientWithTLS_EmptyAddr(t *testing.T) {
	_, err := NewClientWithTLS("", nil)
	if err == nil {
		t.Error("expected error for empty address")
	}
}

func TestNewClientWithTLS_Valid(t *testing.T) {
	c, err := NewClientWithTLS("localhost:8443", nil)
	if err != nil {
		t.Fatalf("NewClientWithTLS: %v", err)
	}
	defer c.Close()

	if !strings.HasPrefix(c.baseURL, "https://") {
		t.Errorf("baseURL should start with https://, got %q", c.baseURL)
	}
}

// ============================================================================
// NewTopicStreamClientWithTLS TESTS
// ============================================================================

func TestNewTopicStreamClientWithTLS_EmptyAddr(t *testing.T) {
	_, err := NewTopicStreamClientWithTLS("", nil)
	if err == nil {
		t.Error("expected error for empty address")
	}
}

func TestNewTopicStreamClientWithTLS_NilCreds(t *testing.T) {
	_, err := NewTopicStreamClientWithTLS("localhost:9090", nil)
	if err == nil {
		t.Error("expected error for nil credentials")
	}
}

func TestNewTopicStreamClientWithTLS_Valid(t *testing.T) {
	creds := insecure.NewCredentials()
	c, err := NewTopicStreamClientWithTLS("localhost:9090", creds)
	if err != nil {
		t.Fatalf("NewTopicStreamClientWithTLS: %v", err)
	}
	defer c.Close()

	if c.grpcAddr != "localhost:9090" {
		t.Errorf("grpcAddr = %q, want localhost:9090", c.grpcAddr)
	}
}

// ============================================================================
// WithTLS TESTS
// ============================================================================

func TestWithTLS_SetsMetadataMarker(t *testing.T) {
	c := &TopicStreamClient{
		metadata: make(map[string]string),
	}
	opt := WithTLS(nil)
	opt(c)

	if _, ok := c.metadata["_tls_creds"]; !ok {
		t.Error("expected _tls_creds marker in metadata")
	}
}

// ============================================================================
// TopicStreamClient.GetMessages via bufconn TESTS
// ============================================================================

func TestTopicStreamClient_GetMessages(t *testing.T) {
	mockSrv, client, _ := setupBufconn(t)
	ctx := context.Background()

	client.Subscribe(ctx, "tasks.created", "reader")

	mockSrv.addMessage("tasks.created", 1, `msg-1`)
	mockSrv.addMessage("tasks.created", 2, `msg-2`)

	msgs, err := client.GetMessages(ctx, "tasks.created", 0, 10)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages, got %d", len(msgs))
	}
}

func TestTopicStreamClient_GetMessages_EmptyTopic(t *testing.T) {
	_, client, _ := setupBufconn(t)
	_, err := client.GetMessages(context.Background(), "", 0, 10)
	if err == nil {
		t.Error("expected error for empty topic")
	}
}

func TestTopicStreamClient_GetMessages_NotSubscribed(t *testing.T) {
	_, client, _ := setupBufconn(t)
	_, err := client.GetMessages(context.Background(), "unknown", 0, 10)
	if err == nil {
		t.Error("expected error for unsubscribed topic")
	}
}

func TestTopicStreamClient_GetMessages_WithLimit(t *testing.T) {
	mockSrv, client, _ := setupBufconn(t)
	ctx := context.Background()

	client.Subscribe(ctx, "tasks.created", "reader")
	mockSrv.addMessage("tasks.created", 1, `msg-1`)
	mockSrv.addMessage("tasks.created", 2, `msg-2`)
	mockSrv.addMessage("tasks.created", 3, `msg-3`)

	msgs, err := client.GetMessages(ctx, "tasks.created", 0, 2)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages (limited), got %d", len(msgs))
	}
}

// ============================================================================
// TopicStreamClient.Publish with circuit-open + HTTP fallback TESTS
// ============================================================================

func TestTopicStreamClient_Publish_EmptyTopic(t *testing.T) {
	_, client, _ := setupBufconn(t)
	err := client.Publish(context.Background(), "", []byte("data"))
	if err == nil {
		t.Error("expected error for empty topic")
	}
}

func TestTopicStreamClient_Publish_PendingSubscription(t *testing.T) {
	_, client, _ := setupBufconn(t)
	client.mu.Lock()
	client.subscribers["pending"] = &Subscriber{
		SubscriberID: "sub-p",
		Topic:        "pending",
		Status:       "pending",
	}
	client.mu.Unlock()

	err := client.Publish(context.Background(), "pending", []byte("data"))
	if err != ErrSubscriptionPending {
		t.Errorf("expected ErrSubscriptionPending, got: %v", err)
	}
}

// ============================================================================
// TopicStreamClient.StreamMessages — pending subscription
// ============================================================================

func TestTopicStreamClient_StreamMessages_PendingSubscription(t *testing.T) {
	_, client, _ := setupBufconn(t)
	client.mu.Lock()
	client.subscribers["pending"] = &Subscriber{
		SubscriberID: "sub-p",
		Topic:        "pending",
		Status:       "pending",
	}
	client.mu.Unlock()

	_, err := client.StreamMessages(context.Background(), "pending")
	if err != ErrSubscriptionPending {
		t.Errorf("expected ErrSubscriptionPending, got: %v", err)
	}
}

// ============================================================================
// Client.StreamMessages — not subscribed and empty topic
// ============================================================================

func TestClient_StreamMessages_EmptyTopic(t *testing.T) {
	c, _ := NewClient("localhost:9090")
	defer c.Close()

	_, err := c.StreamMessages(context.Background(), "")
	if err == nil {
		t.Error("expected error for empty topic")
	}
}

func TestClient_StreamMessages_NotSubscribed(t *testing.T) {
	c, _ := NewClient("localhost:9090")
	defer c.Close()

	_, err := c.StreamMessages(context.Background(), "unknown")
	if err == nil {
		t.Error("expected error for unsubscribed topic")
	}
}

func TestClient_StreamMessages_PendingSubscription(t *testing.T) {
	c, _ := NewClient("localhost:9090")
	defer c.Close()

	c.mu.Lock()
	c.subscribers["pending"] = &Subscriber{
		SubscriberID: "sub-p",
		Topic:        "pending",
		Status:       "pending",
	}
	c.mu.Unlock()

	_, err := c.StreamMessages(context.Background(), "pending")
	if err != ErrSubscriptionPending {
		t.Errorf("expected ErrSubscriptionPending, got: %v", err)
	}
}

// ============================================================================
// enqueueFallback — payload copy safety
// ============================================================================

func TestEnqueueFallback_CopiesPayload(t *testing.T) {
	c, _ := NewClient("localhost:9090")
	defer c.Close()

	payload := []byte("original")
	c.enqueueFallback("test", payload)

	// Mutate the original.
	payload[0] = 'X'

	c.fallbackMu.Lock()
	queued := c.fallbackQueue[0].payload
	c.fallbackMu.Unlock()

	if string(queued) != "original" {
		t.Errorf("fallback queue should have a copy, got %q", string(queued))
	}
}

// ============================================================================
// drainBody edge cases
// ============================================================================

func TestDrainBody_NilResponse(t *testing.T) {
	drainBody(nil) // should not panic
}

func TestDrainBody_NilBody(t *testing.T) {
	resp := &http.Response{Body: nil}
	drainBody(resp) // should not panic
}

// ============================================================================
// Client.Subscribe validation
// ============================================================================

func TestClient_Subscribe_EmptyTopic(t *testing.T) {
	c, _ := NewClient("localhost:9090")
	defer c.Close()

	_, err := c.Subscribe(context.Background(), "", "name")
	if err == nil {
		t.Error("expected error for empty topic")
	}
}

func TestClient_Subscribe_EmptyDisplayName(t *testing.T) {
	c, _ := NewClient("localhost:9090")
	defer c.Close()

	_, err := c.Subscribe(context.Background(), "test", "")
	if err == nil {
		t.Error("expected error for empty display name")
	}
}

// ============================================================================
// Client.Publish validation
// ============================================================================

func TestClient_Publish_EmptyTopic(t *testing.T) {
	c, _ := NewClient("localhost:9090")
	defer c.Close()

	err := c.Publish(context.Background(), "", []byte("data"))
	if err == nil {
		t.Error("expected error for empty topic")
	}
}

func TestClient_Publish_EmptyPayload(t *testing.T) {
	c, _ := NewClient("localhost:9090")
	defer c.Close()

	err := c.Publish(context.Background(), "test", nil)
	if err == nil {
		t.Error("expected error for empty payload")
	}
}

func TestClient_Publish_NotSubscribed(t *testing.T) {
	c, _ := NewClient("localhost:9090")
	defer c.Close()

	err := c.Publish(context.Background(), "unknown", []byte("data"))
	if err == nil {
		t.Error("expected error for unsubscribed topic")
	}
}

func TestClient_Publish_PendingSubscription(t *testing.T) {
	c, _ := NewClient("localhost:9090")
	defer c.Close()

	c.mu.Lock()
	c.subscribers["pending"] = &Subscriber{
		SubscriberID: "sub-p",
		Topic:        "pending",
		Status:       "pending",
	}
	c.mu.Unlock()

	err := c.Publish(context.Background(), "pending", []byte("data"))
	if err != ErrSubscriptionPending {
		t.Errorf("expected ErrSubscriptionPending, got: %v", err)
	}
}

// ============================================================================
// Client.GetMessages validation
// ============================================================================

func TestClient_GetMessages_EmptyTopic(t *testing.T) {
	c, _ := NewClient("localhost:9090")
	defer c.Close()

	_, err := c.GetMessages(context.Background(), "", 0, 10)
	if err == nil {
		t.Error("expected error for empty topic")
	}
}

// ============================================================================
// GRPCClient.Publish with server error
// ============================================================================

func TestGRPCClient_Publish_ServerError(t *testing.T) {
	mockSrv, gc := setupGRPCBufconn(t)
	ctx := context.Background()

	gc.Subscribe(ctx, "err-topic", "publisher")

	mockSrv.mu.Lock()
	mockSrv.publishErr = status.Error(codes.Internal, "server error")
	mockSrv.mu.Unlock()

	err := gc.Publish(ctx, "err-topic", []byte("data"))
	if err == nil {
		t.Error("expected error from server")
	}
}

// ============================================================================
// Concurrent FlushFallbackQueue
// ============================================================================

func TestFlushFallbackQueue_ConcurrentSafe(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"/api/v1/topics/concurrent-flush/publish": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
		},
	})

	c.mu.Lock()
	c.subscribers["concurrent-flush"] = &Subscriber{
		SubscriberID: "sub-cf",
		Topic:        "concurrent-flush",
		Status:       "approved",
	}
	c.mu.Unlock()

	// Enqueue messages.
	for i := 0; i < 20; i++ {
		c.enqueueFallback("concurrent-flush", []byte(fmt.Sprintf("msg-%d", i)))
	}

	// Flush concurrently.
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.FlushFallbackQueue(context.Background())
		}()
	}
	wg.Wait()

	// Queue should be empty after flush.
	if c.FallbackQueueLen() != 0 {
		t.Errorf("FallbackQueueLen = %d, want 0 after concurrent flush", c.FallbackQueueLen())
	}
}

// ============================================================================
// WithBufferSize edge case
// ============================================================================

func TestWithBufferSize_ZeroIgnored(t *testing.T) {
	c := &TopicStreamClient{
		bufferSize: 256,
		metadata:   make(map[string]string),
	}
	opt := WithBufferSize(0)
	opt(c)

	if c.bufferSize != 256 {
		t.Errorf("bufferSize = %d, want 256 (0 should be ignored)", c.bufferSize)
	}
}

// ============================================================================
// WithRetryPolicy edge cases
// ============================================================================

func TestWithRetryPolicy_ZeroValuesIgnored(t *testing.T) {
	c := &TopicStreamClient{
		initialBackoff: 100 * time.Millisecond,
		maxBackoff:     30 * time.Second,
		maxRetries:     10,
		metadata:       make(map[string]string),
	}
	opt := WithRetryPolicy(0, 0, -1)
	opt(c)

	// Zero durations should be ignored, negative maxRetries should be ignored.
	if c.initialBackoff != 100*time.Millisecond {
		t.Errorf("initialBackoff = %v, want 100ms", c.initialBackoff)
	}
	if c.maxBackoff != 30*time.Second {
		t.Errorf("maxBackoff = %v, want 30s", c.maxBackoff)
	}
	// maxRetries >= 0 check means -1 should be ignored.
	if c.maxRetries != 10 {
		t.Errorf("maxRetries = %d, want 10", c.maxRetries)
	}
}

// ============================================================================
// ErrMessageTimeout formatting
// ============================================================================

func TestErrMessageTimeout_Format(t *testing.T) {
	if ErrMessageTimeout.Error() != "message delivery timeout exceeded" {
		t.Errorf("ErrMessageTimeout = %q", ErrMessageTimeout.Error())
	}
}

// ============================================================================
// Client.Close with no channels
// ============================================================================

func TestClient_Close_Empty(t *testing.T) {
	c, _ := NewClient("localhost:9090")
	err := c.Close()
	if err != nil {
		t.Fatalf("Close: %v", err)
	}
}
