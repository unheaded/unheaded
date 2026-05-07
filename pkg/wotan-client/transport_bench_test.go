// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package wotanClient

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"
	chatpb "unheaded/services/wotan/proto"
)

// ============================================================================
// TRANSPORT BENCHMARK: HTTP POLLING vs gRPC STREAMING
//
// Run:
//   go test -bench=BenchmarkTransport -benchmem -run=^$ ./pkg/wotan-client/
//
// All benchmarks share ONE mock HTTP server and ONE mock gRPC server
// initialized via TestMain to avoid macOS ephemeral port exhaustion.
//
// drainBody() in client.go ensures HTTP keep-alive connection reuse.
// ============================================================================

// ---------------------------------------------------------------------------
// Shared test infrastructure — initialized once via TestMain.
// ---------------------------------------------------------------------------
var (
	sharedAddr       string
	sharedHTTPClient *http.Client
	sharedServer     *httptest.Server

	sharedGRPCAddr   string
	sharedGRPCServer *grpc.Server
	sharedGRPCLis    net.Listener
)

// mockTopicServer implements chatpb.TopicStreamServer for benchmarks.
type mockTopicServer struct {
	chatpb.UnimplementedTopicStreamServer
	mu       sync.Mutex
	messages []*chatpb.TopicEvent
	seqNo    int64
}

func (s *mockTopicServer) PublishTopic(_ context.Context, req *chatpb.TopicPublishRequest) (*chatpb.TopicPublishResponse, error) {
	seq := atomic.AddInt64(&s.seqNo, 1)
	event := &chatpb.TopicEvent{
		MessageId: fmt.Sprintf("grpc-msg-%d", seq),
		Topic:     req.Topic,
		SenderId:  req.SenderId,
		Seq:       seq,
		Payload:   req.Payload,
		CreatedAt: timestamppb.Now(),
		Type:      chatpb.TopicEventType_TOPIC_MESSAGE_PUBLISHED,
	}
	s.mu.Lock()
	// Bounded: keep last 200 messages max to avoid 60MB alloc blowup
	s.messages = append(s.messages, event)
	if len(s.messages) > 200 {
		s.messages = s.messages[len(s.messages)-200:]
	}
	s.mu.Unlock()

	return &chatpb.TopicPublishResponse{
		MessageId: event.MessageId,
		Seq:       seq,
		CreatedAt: timestamppb.Now(),
	}, nil
}

func (s *mockTopicServer) StreamTopics(req *chatpb.TopicStreamRequest, stream grpc.ServerStreamingServer[chatpb.TopicEvent]) error {
	s.mu.Lock()
	snapshot := make([]*chatpb.TopicEvent, len(s.messages))
	copy(snapshot, s.messages)
	s.mu.Unlock()

	for _, event := range snapshot {
		if event.Seq > req.SinceSeq {
			if err := stream.Send(event); err != nil {
				return err
			}
		}
	}
	return nil // end of stream
}

func (s *mockTopicServer) TopicPing(_ context.Context, req *chatpb.TopicPingRequest) (*chatpb.TopicPongResponse, error) {
	return &chatpb.TopicPongResponse{
		Timestamp:     timestamppb.Now(),
		Status:        "ok",
		ActiveStreams: 1,
		TotalTopics:   1,
	}, nil
}

func TestMain(m *testing.M) {
	// ---- HTTP mock server ----
	var (
		mu       sync.Mutex
		messages []map[string]interface{}
		seqNo    int64
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/topics/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		if strings.HasSuffix(path, "/subscribe") && r.Method == "POST" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"subscriber": map[string]interface{}{
					"subscriber_id": "bench-sub-001",
					"topic":         "bench.topic",
					"display_name":  "benchmarker",
					"status":        "approved",
					"requested_at":  time.Now().Format(time.RFC3339),
				},
			})
			return
		}

		if strings.HasSuffix(path, "/publish") && r.Method == "POST" {
			seq := atomic.AddInt64(&seqNo, 1)
			msg := map[string]interface{}{
				"message_id": fmt.Sprintf("msg-%d", seq),
				"topic":      "bench.topic",
				"sender_id":  "bench-pub",
				"seq":        seq,
				"payload":    `{"benchmark": true}`,
				"created_at": time.Now().Format(time.RFC3339),
			}
			mu.Lock()
			messages = append(messages, msg)
			// Bounded: keep last 200 messages to avoid 60MB alloc blowup
			if len(messages) > 200 {
				messages = messages[len(messages)-200:]
			}
			mu.Unlock()

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"message": map[string]interface{}{
					"message_id": fmt.Sprintf("msg-%d", seq),
					"seq":        seq,
				},
			})
			return
		}

		if strings.Contains(path, "/messages") && r.Method == "GET" {
			mu.Lock()
			result := make([]map[string]interface{}, len(messages))
			copy(result, messages)
			mu.Unlock()

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"messages": result,
			})
			return
		}

		http.NotFound(w, r)
	})

	sharedServer = httptest.NewServer(mux)
	sharedAddr = strings.TrimPrefix(sharedServer.URL, "http://")

	// Single keep-alive transport — ONE connection reused by all benchmarks.
	sharedHTTPClient = &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        1,
			MaxIdleConnsPerHost: 1,
			IdleConnTimeout:     90 * time.Second,
			DisableKeepAlives:   false,
			DialContext: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
		},
		Timeout: 30 * time.Second,
	}

	// ---- gRPC mock server ----
	var err error
	sharedGRPCLis, err = net.Listen("tcp", "127.0.0.1:0") // random port
	if err != nil {
		fmt.Fprintf(os.Stderr, "grpc listen: %v\n", err)
		os.Exit(1)
	}
	sharedGRPCAddr = sharedGRPCLis.Addr().String()

	sharedGRPCServer = grpc.NewServer()
	chatpb.RegisterTopicStreamServer(sharedGRPCServer, &mockTopicServer{})

	go func() {
		if err := sharedGRPCServer.Serve(sharedGRPCLis); err != nil {
			// Stopped is normal on teardown
		}
	}()

	code := m.Run()

	sharedServer.Close()
	sharedGRPCServer.GracefulStop()
	os.Exit(code)
}

// ---------------------------------------------------------------------------
// Client helpers
// ---------------------------------------------------------------------------

// sharedHTTPBenchClient creates a Client wired to the shared HTTP mock server.
func sharedHTTPBenchClient(tb testing.TB) *Client {
	tb.Helper()
	c := &Client{
		baseURL:       fmt.Sprintf("http://%s/api/v1", sharedAddr),
		controlClient: sharedHTTPClient,
		streamClient:  sharedHTTPClient,
		transport:     TransportHTTP,
		subscribers:   make(map[string]*Subscriber),
		channels:      make(map[string]*safeChannel),
	}
	tb.Cleanup(func() { c.Close() })

	ctx := context.Background()
	if _, err := c.Subscribe(ctx, "bench.topic", "benchmarker"); err != nil {
		tb.Fatal(err)
	}
	return c
}

// sharedGRPCBenchClient creates a GRPCClient wired to the shared gRPC mock server.
func sharedGRPCBenchClient(tb testing.TB) *GRPCClient {
	tb.Helper()

	conn, err := grpc.NewClient(
		sharedGRPCAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		tb.Fatalf("grpc dial: %v", err)
	}

	gc := &GRPCClient{
		conn:        conn,
		client:      chatpb.NewTopicStreamClient(conn),
		subscribers: make(map[string]*Subscriber),
		channels:    make(map[string]*safeChannel),
		maxRetries:  3,
		retryDelay:  10 * time.Millisecond,
	}
	tb.Cleanup(func() { gc.Close() })

	ctx := context.Background()
	if _, err := gc.Subscribe(ctx, "bench.topic", "benchmarker"); err != nil {
		tb.Fatal(err)
	}
	return gc
}

// ============================================================================
// HTTP BENCHMARKS
// ============================================================================

func BenchmarkTransport_HTTP_Subscribe(b *testing.B) {
	c := sharedHTTPBenchClient(b)
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if _, err := c.Subscribe(ctx, "bench.topic", "benchmarker"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTransport_HTTP_Publish(b *testing.B) {
	c := sharedHTTPBenchClient(b)
	ctx := context.Background()
	payload := []byte(`{"event":"task_updated","data":{"id":"bench-task"}}`)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if err := c.Publish(ctx, "bench.topic", payload); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTransport_HTTP_GetMessages(b *testing.B) {
	c := sharedHTTPBenchClient(b)
	ctx := context.Background()

	// Seed some messages
	for i := 0; i < 50; i++ {
		_ = c.Publish(ctx, "bench.topic", []byte(fmt.Sprintf(`{"seq":%d}`, i)))
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		msgs, err := c.GetMessages(ctx, "bench.topic", 0, 50)
		if err != nil {
			b.Fatal(err)
		}
		_ = msgs
	}
}

func BenchmarkTransport_HTTP_PollCycle(b *testing.B) {
	c := sharedHTTPBenchClient(b)
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	var lastSeq int64
	for i := 0; i < b.N; i++ {
		_ = c.Publish(ctx, "bench.topic", []byte(`{"benchmark":true}`))
		msgs, err := c.GetMessages(ctx, "bench.topic", lastSeq, 50)
		if err != nil {
			b.Fatal(err)
		}
		for _, m := range msgs {
			if m.Seq > lastSeq {
				lastSeq = m.Seq
			}
		}
	}
}

// ============================================================================
// gRPC BENCHMARKS — apples-to-apples with HTTP above
// ============================================================================

func BenchmarkTransport_GRPC_Subscribe(b *testing.B) {
	gc := sharedGRPCBenchClient(b)
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if _, err := gc.Subscribe(ctx, "bench.topic", "benchmarker"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTransport_GRPC_Publish(b *testing.B) {
	gc := sharedGRPCBenchClient(b)
	ctx := context.Background()
	payload := []byte(`{"event":"task_updated","data":{"id":"bench-task"}}`)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if err := gc.Publish(ctx, "bench.topic", payload); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTransport_GRPC_GetMessages(b *testing.B) {
	gc := sharedGRPCBenchClient(b)
	ctx := context.Background()

	// Seed some messages via gRPC
	for i := 0; i < 50; i++ {
		_ = gc.Publish(ctx, "bench.topic", []byte(fmt.Sprintf(`{"seq":%d}`, i)))
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		msgs, err := gc.GetMessages(ctx, "bench.topic", 0, 50)
		if err != nil {
			b.Fatal(err)
		}
		_ = msgs
	}
}

func BenchmarkTransport_GRPC_PollCycle(b *testing.B) {
	gc := sharedGRPCBenchClient(b)
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	var lastSeq int64
	for i := 0; i < b.N; i++ {
		_ = gc.Publish(ctx, "bench.topic", []byte(`{"benchmark":true}`))
		msgs, err := gc.GetMessages(ctx, "bench.topic", lastSeq, 50)
		if err != nil {
			b.Fatal(err)
		}
		for _, m := range msgs {
			if m.Seq > lastSeq {
				lastSeq = m.Seq
			}
		}
	}
}

// ============================================================================
// CLIENT CREATION BENCHMARKS
// ============================================================================

func BenchmarkTransport_ClientCreation_HTTP(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		c, err := NewClient("localhost:8080")
		if err != nil {
			b.Fatal(err)
		}
		c.Close()
	}
}

func BenchmarkTransport_ClientCreation_DualTransport(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		c, err := NewClientWithGRPC("localhost:8080", "localhost:9090")
		if err != nil {
			b.Fatal(err)
		}
		c.Close()
	}
}

// ============================================================================
// TRANSPORT STRATEGY DOCUMENTATION
// ============================================================================

func TestTransportComparison_Documentation(t *testing.T) {
	t.Log("Transport Strategy: gRPC PRIMARY, HTTP CIRCUIT-BREAKER FALLBACK")
	t.Log("================================================================")
	t.Log("")
	t.Log("gRPC Streaming (PRIMARY — used exclusively when healthy):")
	t.Log("  Delivery lag:      ~0.01ms (server push, zero polling)")
	t.Log("  Connection model:  Single HTTP/2 stream, multiplexed")
	t.Log("  Memory per client: ~4KB (stream buffer + channel)")
	t.Log("  Auto-reconnect:    Exponential backoff, 10 retries")
	t.Log("  CPU cost:          Near zero (event-driven, no polling loop)")
	t.Log("")
	t.Log("HTTP Polling (FALLBACK — circuit breaker degraded mode only):")
	t.Log("  Poll interval:     500ms")
	t.Log("  Avg delivery lag:  250ms (half poll interval)")
	t.Log("  Per-poll cost:     1 HTTP GET roundtrip")
	t.Log("  Connection model:  Keep-alive HTTP/1.1")
	t.Log("  Memory per client: ~2KB (channel buffer)")
	t.Log("  CPU cost:          2 req/sec/topic (poll + parse)")
	t.Log("")
	t.Log("Startup sequence:")
	t.Log("  1. NewClientWithGRPC() probes gRPC → if healthy, TransportGRPC")
	t.Log("  2. If probe fails → TransportHTTP (degraded, logs warning)")
	t.Log("  3. StreamMessages() uses active transport exclusively (no dual)")
	t.Log("  4. If gRPC fails mid-stream → degradeToHTTP() circuit breaker")
	t.Log("  5. Call ProbeGRPC() periodically to re-promote when recovered")
	t.Log("  6. HTTP REST always used for control plane (Subscribe, Publish)")
}
