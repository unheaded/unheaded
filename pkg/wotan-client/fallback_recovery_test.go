// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package wotanClient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	chatpb "unheaded/services/wotan/proto"
)

// fallbackWotan is an HTTP Wotan that, like the real one, refuses a publish
// from a subscriber it has not seen join.
func fallbackWotan(t *testing.T, subscribeStatus int) (*Client, *atomic.Int64, *atomic.Int64) {
	t.Helper()
	var joins, published atomic.Int64
	var joined atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/subscribe"):
			joins.Add(1)
			if subscribeStatus != http.StatusCreated {
				w.WriteHeader(subscribeStatus)
				return
			}
			joined.Store(true)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"subscriber":{"subscriber_id":"00000000-0000-0000-0000-000000000001","status":"approved"}}`))
		case strings.HasSuffix(r.URL.Path, "/publish"):
			if !joined.Load() {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":{"code":"Forbidden","message":"subscriber not approved"}}`))
				return
			}
			published.Add(1)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{}`))
		case strings.HasSuffix(r.URL.Path, "/messages"):
			_, _ = w.Write([]byte(`{"messages":[],"last_seq":0}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	c, err := NewClient(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c, &joins, &published
}

// The HTTP fallback joins the topic before publishing. Subscribe only ever
// recorded the membership on the gRPC side, so every fallback publish failed
// "not subscribed to topic" (trace-collector, 2026-10-18).
func TestTopicStreamClient_FallbackPublishJoinsFirst(t *testing.T) {
	srv := newMockServer()
	srv.publishErr = status.Error(codes.Unavailable, "wotan down")
	lis := startMockServer(t, srv)
	httpc, joins, published := fallbackWotan(t, http.StatusCreated)
	client, err := newTopicStreamClientWithDialer(lis, WithHTTPFallback(httpc))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	ctx := context.Background()
	if _, err := client.Subscribe(ctx, "traces.flow", "trace-collector"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := client.Publish(ctx, "traces.flow", []byte(`[{"n":1}]`)); err != nil {
			t.Fatalf("publish %d over the fallback: %v", i+1, err)
		}
	}
	if joins.Load() != 1 || published.Load() != 2 {
		t.Errorf("joins=%d published=%d, want 1 and 2", joins.Load(), published.Load())
	}
}

// When gRPC and the fallback both fail, the error names the gRPC failure:
// the fallback's error alone hid why gRPC was being bypassed.
func TestTopicStreamClient_BothFailNamesGRPCError(t *testing.T) {
	srv := newMockServer()
	srv.publishErr = status.Error(codes.Unavailable, "wotan down")
	lis := startMockServer(t, srv)
	httpc, _, _ := fallbackWotan(t, http.StatusServiceUnavailable)
	client, err := newTopicStreamClientWithDialer(lis, WithHTTPFallback(httpc))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	ctx := context.Background()
	client.Subscribe(ctx, "traces.flow", "trace-collector")
	err = client.Publish(ctx, "traces.flow", []byte(`[{"n":1}]`))
	if err == nil || !strings.Contains(err.Error(), "wotan down") || !strings.Contains(err.Error(), "HTTP fallback") {
		t.Fatalf("err = %v, want both the gRPC cause and the fallback's", err)
	}
}

// A stream that tripped the circuit to HTTP polling goes back to gRPC once
// a probe finds Wotan answering. Nothing used to close the circuit, so one
// outage left every consumer polling HTTP until it restarted.
func TestTopicStreamClient_StreamLeavesFallbackWhenGRPCBack(t *testing.T) {
	srv := newMockServer()
	srv.streamErr = status.Error(codes.Unavailable, "wotan down")
	lis := startMockServer(t, srv)
	httpc, _, _ := fallbackWotan(t, http.StatusCreated)
	client, err := newTopicStreamClientWithDialer(lis,
		WithHTTPFallback(httpc), WithRetryPolicy(5*time.Millisecond, 20*time.Millisecond, 2))
	if err != nil {
		t.Fatal(err)
	}
	client.circuitProbe = 50 * time.Millisecond
	t.Cleanup(func() { client.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client.Subscribe(ctx, "traces.flow", "dashboard")
	ch, err := client.StreamMessages(ctx, "traces.flow")
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for !client.IsCircuitOpen() {
		if time.Now().After(deadline) {
			t.Fatal("circuit never opened")
		}
		time.Sleep(5 * time.Millisecond)
	}

	srv.setStreamError(nil) // Wotan is back
	for client.IsCircuitOpen() {
		if time.Now().After(deadline) {
			t.Fatal("circuit still open 5 s after gRPC came back")
		}
		time.Sleep(5 * time.Millisecond)
	}
	// And the stream really is on gRPC again: a live event arrives.
	srv.live <- &chatpb.TopicEvent{Topic: "traces.flow", Seq: 7, Payload: []byte(`{"live":true}`)}
	select {
	case m := <-ch:
		if m.Seq != 7 {
			t.Errorf("got seq %d, want the live event 7", m.Seq)
		}
	case <-time.After(5 * time.Second):
		t.Error("no event over gRPC after the circuit closed")
	}
}
