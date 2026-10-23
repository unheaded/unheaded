// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package wotanClient

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	chatpb "unheaded/services/wotan/proto"
)

// grpcAndHTTPWotan starts a mock gRPC Wotan (TopicStream + health) and an HTTP
// Wotan whose message polls answer 200 with no messages, or 500 while
// httpDown is set. It returns a client on both, subscribed to topic.
func grpcAndHTTPWotan(t *testing.T, topic string) (*Client, *mockTopicStreamServer, *atomic.Bool) {
	t.Helper()
	srv := newMockServer()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	gs := grpc.NewServer()
	chatpb.RegisterTopicStreamServer(gs, srv)
	healthpb.RegisterHealthServer(gs, health.NewServer())
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	var httpDown atomic.Bool
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if httpDown.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"code":"INTERNAL","message":"down"}}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"messages": []any{}, "last_seq": 0})
	}))
	t.Cleanup(hs.Close)

	c, err := NewClientWithGRPC(strings.TrimPrefix(hs.URL, "http://"), lis.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	c.streamUpAfter = 50 * time.Millisecond
	c.grpcMu.Lock()
	if c.grpcClient != nil { // created by the startup probe
		c.grpcClient.upAfter = c.streamUpAfter
	}
	c.grpcMu.Unlock()
	t.Cleanup(func() { _ = c.Close() })
	c.subscribers[topic] = &Subscriber{SubscriberID: "s", DisplayName: "timeguru", Status: "approved"}
	return c, srv, &httpDown
}

func streamUp(c *Client, topic string) (up, known bool) {
	up, known = c.Health().Streams[topic]
	return
}

// A stream the server ended shows as down within a second. It used to be
// invisible: the service's /health said 200 while it heard nothing.
func TestStreamHealth_ClosedStreamReportsDown(t *testing.T) {
	const topic = "alerts.critical"
	c, srv, httpDown := grpcAndHTTPWotan(t, topic)
	srv.addMessage(topic, 3, "first")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ch, err := c.StreamMessages(ctx, topic)
	if err != nil {
		t.Fatal(err)
	}
	if m := <-ch; m == nil || m.Seq != 3 {
		t.Fatalf("first message %+v, want seq 3", m)
	}
	if up, _ := streamUp(c, topic); !up {
		t.Fatal("stream not up after delivering a message")
	}
	if age, ok := c.Health().LastMessageAge[topic]; !ok || age < 0 || age > 5*time.Second {
		t.Errorf("last message age %v (known=%v), want a small age", age, ok)
	}

	httpDown.Store(true)                                        // no fallback either
	srv.setStreamError(status.Error(codes.Unavailable, "gone")) // Wotan ends the stream
	waitFor(t, time.Second, "the stream to report down", func() bool {
		up, known := streamUp(c, topic)
		return known && !up
	})
}

// An idle topic is not a dead stream: no messages, still up.
func TestStreamHealth_IdleStreamStaysUp(t *testing.T) {
	const topic = "alerts.critical"
	c, _, _ := grpcAndHTTPWotan(t, topic)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := c.StreamMessages(ctx, topic); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 2*time.Second, "the idle stream to report up", func() bool {
		up, _ := streamUp(c, topic)
		return up
	})
	time.Sleep(300 * time.Millisecond)
	h := c.Health()
	if !h.Streams[topic] {
		t.Error("idle stream went down with no error")
	}
	if _, ok := h.LastMessageAge[topic]; ok {
		t.Error("an idle stream reports a last message age")
	}
	if h.Transport != "gRPC" || !h.GRPCHealthy {
		t.Errorf("transport %q healthy %v, want gRPC and true", h.Transport, h.GRPCHealthy)
	}
}

// When gRPC gives up and the client polls over HTTP instead, Health says so:
// transport HTTP, stream up while polls succeed.
func TestStreamHealth_FallbackToPollingReportsTransportHTTP(t *testing.T) {
	const topic = "alerts.critical"
	c, srv, _ := grpcAndHTTPWotan(t, topic)
	srv.setStreamError(status.Error(codes.PermissionDenied, "gone")) // non-transient: gRPC gives up
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := c.StreamMessages(ctx, topic); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 3*time.Second, "polling to report the stream up over HTTP", func() bool {
		h := c.Health()
		return h.Transport == "HTTP" && h.Streams[topic]
	})
}

// An HTTP-only client (timeguru, architect) reports a failing poll as down
// and a recovered one as up again.
func TestStreamHealth_HTTPPollingFollowsWotan(t *testing.T) {
	var down atomic.Bool
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if down.Load() {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"messages": []any{}, "last_seq": 0})
	}))
	t.Cleanup(hs.Close)
	c, err := NewClient(strings.TrimPrefix(hs.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	c.subscribers["timeline.updated"] = &Subscriber{SubscriberID: "s", DisplayName: "timeguru", Status: "approved"}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := c.StreamMessages(ctx, "timeline.updated"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 2*time.Second, "up", func() bool { up, _ := streamUp(c, "timeline.updated"); return up })
	down.Store(true)
	waitFor(t, 3*time.Second, "down", func() bool { up, known := streamUp(c, "timeline.updated"); return known && !up })
	down.Store(false)
	waitFor(t, 5*time.Second, "up again", func() bool { up, _ := streamUp(c, "timeline.updated"); return up })

	cancel() // the caller ending its stream is not an outage: the entry goes
	waitFor(t, 2*time.Second, "the ended stream to be forgotten", func() bool {
		_, known := streamUp(c, "timeline.updated")
		return !known
	})
}
