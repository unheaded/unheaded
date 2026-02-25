// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package wotanClient

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"
	chatpb "unheaded/services/wotan/proto"
)

// ============================================================================
// MOCK gRPC SERVER (bufconn — in-process, no real network)
// ============================================================================

// mockTopicStreamServer implements chatpb.TopicStreamServer for testing
type mockTopicStreamServer struct {
	chatpb.UnimplementedTopicStreamServer

	mu       sync.Mutex
	messages []*chatpb.TopicEvent
	// Controls when the stream should error
	streamErr     error
	publishCount  atomic.Int64
	pingCount     atomic.Int64
	publishErr    error
	streamStarted chan struct{} // signals when a StreamTopics call arrives
	streamKill    chan struct{} // signals active streams to close (set by setStreamError)
}

func newMockServer() *mockTopicStreamServer {
	return &mockTopicStreamServer{
		streamStarted: make(chan struct{}, 10),
		streamKill:    make(chan struct{}),
	}
}

func (s *mockTopicStreamServer) StreamTopics(req *chatpb.TopicStreamRequest, stream chatpb.TopicStream_StreamTopicsServer) error {
	// Drain any previous signal
	select {
	case <-s.streamStarted:
	default:
	}

	s.mu.Lock()
	err := s.streamErr
	msgs := make([]*chatpb.TopicEvent, len(s.messages))
	copy(msgs, s.messages)
	killCh := s.streamKill
	s.mu.Unlock()

	// Signal that stream has been requested
	select {
	case s.streamStarted <- struct{}{}:
	default:
	}

	if err != nil {
		return err
	}

	// Send all queued messages
	for _, msg := range msgs {
		// Filter by since_seq
		if msg.Seq <= req.SinceSeq {
			continue
		}
		if err := stream.Send(msg); err != nil {
			return err
		}
	}

	// Keep stream open until context is cancelled OR kill signal received
	select {
	case <-stream.Context().Done():
		return stream.Context().Err()
	case <-killCh:
		return fmt.Errorf("stream killed by setStreamError")
	}
}

func (s *mockTopicStreamServer) PublishTopic(ctx context.Context, req *chatpb.TopicPublishRequest) (*chatpb.TopicPublishResponse, error) {
	s.mu.Lock()
	pubErr := s.publishErr
	s.mu.Unlock()

	if pubErr != nil {
		return nil, pubErr
	}

	seq := s.publishCount.Add(1)

	// Store as a message for streaming
	event := &chatpb.TopicEvent{
		MessageId: fmt.Sprintf("msg-%d", seq),
		Topic:     req.Topic,
		Payload:   req.Payload,
		Seq:       seq,
		CreatedAt: timestamppb.Now(),
		Type:      chatpb.TopicEventType_TOPIC_MESSAGE_PUBLISHED,
	}

	s.mu.Lock()
	s.messages = append(s.messages, event)
	s.mu.Unlock()

	return &chatpb.TopicPublishResponse{
		MessageId: event.MessageId,
		Seq:       seq,
		CreatedAt: timestamppb.Now(),
	}, nil
}

func (s *mockTopicStreamServer) TopicPing(ctx context.Context, req *chatpb.TopicPingRequest) (*chatpb.TopicPongResponse, error) {
	s.pingCount.Add(1)
	return &chatpb.TopicPongResponse{
		Timestamp:     timestamppb.Now(),
		Status:        "ok",
		ActiveStreams: 1,
		TotalTopics:   5,
	}, nil
}

func (s *mockTopicStreamServer) addMessage(topic string, seq int64, payload string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, &chatpb.TopicEvent{
		MessageId: fmt.Sprintf("msg-%d", seq),
		Topic:     topic,
		Payload:   []byte(payload),
		Seq:       seq,
		CreatedAt: timestamppb.Now(),
		Type:      chatpb.TopicEventType_TOPIC_MESSAGE_PUBLISHED,
	})
}

func (s *mockTopicStreamServer) setStreamError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.streamErr = err
	if err != nil {
		// Kill any active streams so they see the error on reconnect
		select {
		case <-s.streamKill:
			// already closed
		default:
			close(s.streamKill)
		}
	} else {
		// Reset kill channel for new streams
		s.streamKill = make(chan struct{})
	}
}

// ============================================================================
// BUFCONN SETUP
// ============================================================================

func setupBufconn(t *testing.T) (*mockTopicStreamServer, *TopicStreamClient, *bufconn.Listener) {
	t.Helper()

	const bufSize = 1024 * 1024
	lis := bufconn.Listen(bufSize)

	srv := grpc.NewServer()
	mockSrv := newMockServer()
	chatpb.RegisterTopicStreamServer(srv, mockSrv)

	go func() {
		if err := srv.Serve(lis); err != nil {
			// Server stopped — expected during test cleanup
		}
	}()
	t.Cleanup(func() { srv.Stop() })

	// Create TopicStreamClient that connects via bufconn
	client, err := newTopicStreamClientWithDialer(lis, WithRetryPolicy(10*time.Millisecond, 100*time.Millisecond, 3))
	if err != nil {
		t.Fatalf("failed to create topic stream client: %v", err)
	}
	t.Cleanup(func() { client.Close() })

	return mockSrv, client, lis
}

// newTopicStreamClientWithDialer creates a TopicStreamClient using a bufconn dialer.
func newTopicStreamClientWithDialer(lis *bufconn.Listener, opts ...TopicStreamOption) (*TopicStreamClient, error) {
	c := &TopicStreamClient{
		grpcAddr:       "bufconn",
		bufferSize:     256,
		metadata:       make(map[string]string),
		initialBackoff: 100 * time.Millisecond,
		maxBackoff:     30 * time.Second,
		maxRetries:     10,
		subscribers:    make(map[string]*Subscriber),
		streams:        make(map[string]*activeStream),
	}

	for _, opt := range opts {
		opt(c)
	}

	conn, err := grpc.NewClient(
		"passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("dial bufconn: %w", err)
	}

	c.conn = conn
	c.client = chatpb.NewTopicStreamClient(conn)

	return c, nil
}

// ============================================================================
// CONSTRUCTOR TESTS
// ============================================================================

func TestNewTopicStreamClient_Valid(t *testing.T) {
	// Use bufconn for real construction test
	_, client, _ := setupBufconn(t)

	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.bufferSize != 256 {
		t.Errorf("expected buffer size 256, got %d", client.bufferSize)
	}
}

func TestNewTopicStreamClient_EmptyAddr(t *testing.T) {
	_, err := NewTopicStreamClient("")
	if err == nil {
		t.Error("expected error for empty address")
	}
}

func TestNewTopicStreamClient_WithOptions(t *testing.T) {
	// Test options apply correctly (use real constructor with valid addr)
	client, err := NewTopicStreamClient("localhost:0",
		WithBufferSize(512),
		WithRetryPolicy(200*time.Millisecond, 60*time.Second, 5),
		WithMetadata(map[string]string{"node_id": "test-node"}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer client.Close()

	if client.bufferSize != 512 {
		t.Errorf("expected buffer size 512, got %d", client.bufferSize)
	}
	if client.initialBackoff != 200*time.Millisecond {
		t.Errorf("expected initial backoff 200ms, got %v", client.initialBackoff)
	}
	if client.maxBackoff != 60*time.Second {
		t.Errorf("expected max backoff 60s, got %v", client.maxBackoff)
	}
	if client.maxRetries != 5 {
		t.Errorf("expected max retries 5, got %d", client.maxRetries)
	}
	if client.metadata["node_id"] != "test-node" {
		t.Errorf("expected metadata node_id=test-node, got %s", client.metadata["node_id"])
	}
}

// ============================================================================
// SUBSCRIBE TESTS
// ============================================================================

func TestTopicStreamClient_Subscribe(t *testing.T) {
	_, client, _ := setupBufconn(t)
	ctx := context.Background()

	sub, err := client.Subscribe(ctx, "tasks.*", "test-subscriber")
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	if sub.Topic != "tasks.*" {
		t.Errorf("expected topic 'tasks.*', got %s", sub.Topic)
	}
	if sub.Status != "approved" {
		t.Errorf("expected status 'approved', got %s", sub.Status)
	}
	if sub.SubscriberID == "" {
		t.Error("expected non-empty subscriber ID")
	}
}

func TestTopicStreamClient_Subscribe_EmptyTopic(t *testing.T) {
	_, client, _ := setupBufconn(t)
	ctx := context.Background()

	_, err := client.Subscribe(ctx, "", "test")
	if err == nil {
		t.Error("expected error for empty topic")
	}
}

func TestTopicStreamClient_Subscribe_EmptyDisplayName(t *testing.T) {
	_, client, _ := setupBufconn(t)
	ctx := context.Background()

	_, err := client.Subscribe(ctx, "tasks.*", "")
	if err == nil {
		t.Error("expected error for empty display name")
	}
}

// ============================================================================
// PUBLISH TESTS
// ============================================================================

func TestTopicStreamClient_Publish(t *testing.T) {
	mockSrv, client, _ := setupBufconn(t)
	ctx := context.Background()

	// Subscribe first
	client.Subscribe(ctx, "tasks.created", "publisher")

	// Publish
	err := client.Publish(ctx, "tasks.created", []byte(`{"id":"task-1"}`))
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	if mockSrv.publishCount.Load() != 1 {
		t.Errorf("expected 1 publish, got %d", mockSrv.publishCount.Load())
	}
}

func TestTopicStreamClient_Publish_NotSubscribed(t *testing.T) {
	_, client, _ := setupBufconn(t)
	ctx := context.Background()

	err := client.Publish(ctx, "unknown.topic", []byte("data"))
	if err == nil {
		t.Error("expected error for unsubscribed topic")
	}
}

func TestTopicStreamClient_Publish_EmptyPayload(t *testing.T) {
	_, client, _ := setupBufconn(t)
	ctx := context.Background()

	client.Subscribe(ctx, "tasks.created", "publisher")

	err := client.Publish(ctx, "tasks.created", nil)
	if err == nil {
		t.Error("expected error for empty payload")
	}
}

// ============================================================================
// STREAM MESSAGES TESTS
// ============================================================================

func TestTopicStreamClient_StreamMessages_ReceivesMessages(t *testing.T) {
	mockSrv, client, _ := setupBufconn(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Add messages before streaming
	mockSrv.addMessage("tasks.created", 1, `{"id":"task-1"}`)
	mockSrv.addMessage("tasks.updated", 2, `{"id":"task-1","status":"done"}`)

	// Subscribe and stream
	client.Subscribe(ctx, "tasks.*", "streamer")

	ch, err := client.StreamMessages(ctx, "tasks.*")
	if err != nil {
		t.Fatalf("StreamMessages failed: %v", err)
	}

	// Collect messages (with timeout)
	var received []*Message
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()

	for len(received) < 2 {
		select {
		case msg, ok := <-ch:
			if !ok {
				t.Fatal("channel closed unexpectedly")
			}
			received = append(received, msg)
		case <-timer.C:
			t.Fatalf("timed out waiting for messages, got %d", len(received))
		}
	}

	if len(received) != 2 {
		t.Errorf("expected 2 messages, got %d", len(received))
	}

	if received[0].Seq != 1 {
		t.Errorf("expected seq 1, got %d", received[0].Seq)
	}
	if received[1].Seq != 2 {
		t.Errorf("expected seq 2, got %d", received[1].Seq)
	}
}

func TestTopicStreamClient_StreamMessages_PatternFilter(t *testing.T) {
	mockSrv, client, _ := setupBufconn(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Add messages — some match "tasks.*", some don't
	mockSrv.addMessage("tasks.created", 1, `{"match":true}`)
	mockSrv.addMessage("events.processed", 2, `{"match":false}`)
	mockSrv.addMessage("tasks.deleted", 3, `{"match":true}`)

	client.Subscribe(ctx, "tasks.*", "streamer")
	ch, err := client.StreamMessages(ctx, "tasks.*")
	if err != nil {
		t.Fatalf("StreamMessages failed: %v", err)
	}

	var received []*Message
	timer := time.NewTimer(2 * time.Second)
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

	// Should only get tasks.* messages (client-side filter)
	for _, msg := range received {
		if !MatchTopic("tasks.*", msg.Topic) {
			t.Errorf("received message with non-matching topic: %s", msg.Topic)
		}
	}
}

func TestTopicStreamClient_StreamMessages_NotSubscribed(t *testing.T) {
	_, client, _ := setupBufconn(t)
	ctx := context.Background()

	_, err := client.StreamMessages(ctx, "unknown.topic")
	if err == nil {
		t.Error("expected error for unsubscribed topic")
	}
}

func TestTopicStreamClient_StreamMessages_SinceSeqResume(t *testing.T) {
	mockSrv, client, _ := setupBufconn(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Add initial messages
	mockSrv.addMessage("tasks.created", 1, `{"id":"task-1"}`)
	mockSrv.addMessage("tasks.created", 2, `{"id":"task-2"}`)
	mockSrv.addMessage("tasks.created", 3, `{"id":"task-3"}`)

	client.Subscribe(ctx, "tasks.created", "streamer")
	ch, err := client.StreamMessages(ctx, "tasks.created")
	if err != nil {
		t.Fatalf("StreamMessages failed: %v", err)
	}

	// Receive all 3 messages
	for i := 0; i < 3; i++ {
		select {
		case <-ch:
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out after receiving %d messages", i)
		}
	}

	// Verify lastSeq was tracked
	client.streamMu.RLock()
	as, ok := client.streams["tasks.created"]
	client.streamMu.RUnlock()

	if ok && as.lastSeq.Load() != 3 {
		t.Errorf("expected lastSeq 3, got %d", as.lastSeq.Load())
	}
}

// ============================================================================
// PING TESTS
// ============================================================================

func TestTopicStreamClient_Ping(t *testing.T) {
	mockSrv, client, _ := setupBufconn(t)
	ctx := context.Background()

	err := client.Ping(ctx)
	if err != nil {
		t.Fatalf("Ping failed: %v", err)
	}

	if mockSrv.pingCount.Load() != 1 {
		t.Errorf("expected 1 ping, got %d", mockSrv.pingCount.Load())
	}
}

// ============================================================================
// CIRCUIT BREAKER TESTS
// ============================================================================

func TestTopicStreamClient_CircuitBreaker_NotOpenByDefault(t *testing.T) {
	_, client, _ := setupBufconn(t)

	if client.IsCircuitOpen() {
		t.Error("circuit breaker should not be open by default")
	}
}

func TestTopicStreamClient_CircuitBreaker_Reset(t *testing.T) {
	_, client, _ := setupBufconn(t)

	// Simulate failures
	for i := 0; i < 20; i++ {
		client.recordFailure()
	}
	client.circuitOpen.Store(true)

	if !client.IsCircuitOpen() {
		t.Error("expected circuit breaker to be open")
	}

	client.ResetCircuitBreaker()

	if client.IsCircuitOpen() {
		t.Error("expected circuit breaker to be closed after reset")
	}
	if client.consecutiveFailures.Load() != 0 {
		t.Error("expected failure count to be 0 after reset")
	}
}

func TestTopicStreamClient_CircuitBreaker_SuccessResetsCount(t *testing.T) {
	_, client, _ := setupBufconn(t)

	// Record some failures
	for i := 0; i < 5; i++ {
		client.recordFailure()
	}

	if client.consecutiveFailures.Load() != 5 {
		t.Errorf("expected 5 failures, got %d", client.consecutiveFailures.Load())
	}

	// A success resets the count
	client.recordSuccess()

	if client.consecutiveFailures.Load() != 0 {
		t.Errorf("expected 0 failures after success, got %d", client.consecutiveFailures.Load())
	}
}

// ============================================================================
// CLOSE TESTS
// ============================================================================

func TestTopicStreamClient_Close(t *testing.T) {
	_, client, _ := setupBufconn(t)
	ctx := context.Background()

	// Subscribe and stream
	client.Subscribe(ctx, "tasks.*", "test")

	streamCtx, streamCancel := context.WithCancel(ctx)
	_, err := client.StreamMessages(streamCtx, "tasks.*")
	if err != nil {
		t.Fatalf("StreamMessages failed: %v", err)
	}

	// Close should not panic
	streamCancel()
	time.Sleep(50 * time.Millisecond) // let goroutine notice cancel

	err = client.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Double close should not error
	err = client.Close()
	if err != nil {
		t.Fatalf("Double close failed: %v", err)
	}
}

func TestTopicStreamClient_Close_CancelsStreams(t *testing.T) {
	_, client, _ := setupBufconn(t)
	ctx := context.Background()

	client.Subscribe(ctx, "tasks.*", "test")
	ch, err := client.StreamMessages(ctx, "tasks.*")
	if err != nil {
		t.Fatalf("StreamMessages failed: %v", err)
	}

	// Close should cause channel to eventually close
	client.Close()

	// Wait for channel to close (with timeout)
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()

	select {
	case _, ok := <-ch:
		if ok {
			// Got a message — that's fine, keep draining
			for range ch {
			}
		}
		// Channel closed — expected
	case <-timer.C:
		t.Error("timed out waiting for channel to close after Close()")
	}
}

// ============================================================================
// SUBSCRIBER STATE TESTS
// ============================================================================

func TestTopicStreamClient_GetSubscriber(t *testing.T) {
	_, client, _ := setupBufconn(t)
	ctx := context.Background()

	client.Subscribe(ctx, "tasks.*", "tester")

	sub, ok := client.GetSubscriber("tasks.*")
	if !ok {
		t.Error("expected subscriber to exist")
	}
	if sub.DisplayName != "tester" {
		t.Errorf("expected display name 'tester', got %s", sub.DisplayName)
	}
}

func TestTopicStreamClient_GetSubscriber_NotFound(t *testing.T) {
	_, client, _ := setupBufconn(t)

	_, ok := client.GetSubscriber("nonexistent")
	if ok {
		t.Error("expected subscriber to not exist")
	}
}

func TestTopicStreamClient_IsApproved(t *testing.T) {
	_, client, _ := setupBufconn(t)
	ctx := context.Background()

	client.Subscribe(ctx, "tasks.*", "tester")

	if !client.IsApproved("tasks.*") {
		t.Error("expected subscription to be approved")
	}
	if client.IsApproved("nonexistent") {
		t.Error("expected non-subscribed topic to not be approved")
	}
}

func TestTopicStreamClient_StreamMessages_ReconnectOnError(t *testing.T) {
	mockSrv, client, _ := setupBufconn(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Add initial messages
	mockSrv.addMessage("tasks.created", 1, `{"id":"task-1"}`)
	mockSrv.addMessage("tasks.created", 2, `{"id":"task-2"}`)

	client.Subscribe(ctx, "tasks.created", "reconnector")
	ch, err := client.StreamMessages(ctx, "tasks.created")
	if err != nil {
		t.Fatalf("StreamMessages failed: %v", err)
	}

	// Receive first 2 messages
	for i := 0; i < 2; i++ {
		select {
		case <-ch:
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for initial messages (received %d)", i)
		}
	}

	// Simulate a stream error
	mockSrv.setStreamError(fmt.Errorf("simulated stream error"))

	// Wait for the stream to be re-established.
	// We know it's re-established when the mock server gets a new stream request.
	// The reconnect attempt will see the error and immediately fail, triggering
	// another reconnect with backoff. Give enough time for backoff + race detector.
	select {
	case <-mockSrv.streamStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for client to reconnect")
	}

	// After reconnect, the stream should be healthy again
	mockSrv.setStreamError(nil)

	// Add more messages
	mockSrv.addMessage("tasks.created", 3, `{"id":"task-3"}`)

	// The client needs to re-establish the stream, so we wait for the signal again
	select {
	case <-mockSrv.streamStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for client to send new stream request")
	}

	// Receive the next message
	select {
	case msg := <-ch:
		if msg.Seq != 3 {
			t.Errorf("expected message with seq 3, got %d", msg.Seq)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for message after reconnect")
	}
}

func TestTopicStreamClient_CircuitBreaker_DegradesToHTTP(t *testing.T) {
	mockSrv, client, lis := setupBufconn(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Setup HTTP fallback
	httpServer, httpRecorder := newTestHTTPServer()
	defer httpServer.Close()

	// NewClient expects host:port (it prepends http://), so strip the scheme
	httpAddr := httpServer.URL[len("http://"):]
	httpClient, err := NewClient(httpAddr)
	if err != nil {
		t.Fatalf("failed to create http client: %v", err)
	}

	// Re-create client with HTTP fallback option
	client.Close()

	client, err = newTopicStreamClientWithDialer(
		lis,
		WithHTTPFallback(httpClient),
		WithRetryPolicy(10*time.Millisecond, 20*time.Millisecond, 3), // Fast retries
	)
	if err != nil {
		t.Fatalf("failed to re-create client: %v", err)
	}

	// Force gRPC to fail consistently
	mockSrv.setStreamError(fmt.Errorf("permanent gRPC failure"))

	// Subscribe and stream
	client.Subscribe(ctx, "tasks.created", "http-fallback-tester")
	ch, err := client.StreamMessages(ctx, "tasks.created")
	if err != nil {
		t.Fatalf("StreamMessages failed: %v", err)
	}

	// Wait for circuit breaker to trip (3 failures with 10ms/20ms backoff + race overhead)
	for i := 0; i < 40; i++ {
		if client.IsCircuitOpen() {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !client.IsCircuitOpen() {
		t.Fatalf("circuit breaker did not open after consecutive failures (failures=%d)", client.consecutiveFailures.Load())
	}

	// Now that circuit is open, HTTP fallback should be active.
	// Queue a message to be returned by the HTTP server.
	// The HTTP client calls: GET /topics/{topic}/messages?after_seq={seq}&limit=100
	// Response format: {"messages": [...]}
	msgJSON := `{"messages":[{"message_id":"msg-http-1","seq":100,"topic":"tasks.created","sender_id":"server","payload":"{\"id\":\"task-http-1\"}","deleted":false,"created_at":"2026-02-16T14:00:00Z"}]}`
	httpRecorder.setResponse("/api/v1/topics/tasks.created/messages?after_seq=0&limit=100", 200, msgJSON)

	// Receive the message from the HTTP fallback
	select {
	case msg := <-ch:
		if msg.Seq != 100 {
			t.Errorf("expected message with seq 100 from HTTP, got %d", msg.Seq)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for message from HTTP fallback")
	}

	// Verify HTTP server was polled
	if httpRecorder.requestCount("/api/v1/topics/tasks.created/messages?after_seq=0&limit=100") == 0 {
		t.Error("HTTP server was not polled")
	}
}

// ============================================================================
// HELPER for HTTP tests
// ============================================================================

type httpTestRecorder struct {
	mu            sync.Mutex
	responses     map[string]struct{
		status int
		body   string
	}
	requestCounts map[string]int
}

func newHTTPTestRecorder() *httpTestRecorder {
	return &httpTestRecorder{
		responses:     make(map[string]struct{ status int; body string }),
		requestCounts: make(map[string]int),
	}
}

func (r *httpTestRecorder) setResponse(path string, status int, body string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.responses[path] = struct{ status int; body string }{status, body}
}

func (r *httpTestRecorder) handler(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := req.URL.RequestURI()
	r.requestCounts[key]++

	if resp, ok := r.responses[key]; ok {
		w.WriteHeader(resp.status)
		w.Write([]byte(resp.body))
		return
	}

	w.WriteHeader(http.StatusNotFound)
}

func (r *httpTestRecorder) requestCount(path string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.requestCounts[path]
}

func newTestHTTPServer() (*httptest.Server, *httpTestRecorder) {
	recorder := newHTTPTestRecorder()
	return httptest.NewServer(http.HandlerFunc(recorder.handler)), recorder
}


func TestTopicStreamClient_ConcurrentSubscribePublish(t *testing.T) {
	_, client, _ := setupBufconn(t)
	ctx := context.Background()

	const numOps = 50
	var wg sync.WaitGroup

	// Concurrent subscribes
	for i := 0; i < numOps; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			topic := fmt.Sprintf("concurrent.%d", n)
			client.Subscribe(ctx, topic, fmt.Sprintf("sub-%d", n))
		}(i)
	}
	wg.Wait()

	// Concurrent publishes
	for i := 0; i < numOps; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			topic := fmt.Sprintf("concurrent.%d", n)
			client.Publish(ctx, topic, []byte(fmt.Sprintf(`{"n":%d}`, n)))
		}(i)
	}
	wg.Wait()
}
