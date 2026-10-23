// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package wotanClient

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// lateWotan reserves an address with nothing listening and returns it with a
// start function that brings an HTTP Wotan up there later. The Wotan counts
// joins and serves one message on every topic.
func lateWotan(t *testing.T) (addr string, start func(), joins *atomic.Int64) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr = lis.Addr().String()
	_ = lis.Close() // nothing listening until start
	joins = &atomic.Int64{}
	start = func() {
		l, err := net.Listen("tcp", addr)
		if err != nil {
			t.Fatalf("relisten %s: %v", addr, err)
		}
		srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasSuffix(r.URL.Path, "/subscribe"):
				joins.Add(1)
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"subscriber":{"subscriber_id":"s1","status":"approved"}}`))
			case strings.HasSuffix(r.URL.Path, "/messages"):
				msgs := []map[string]any{}
				if r.URL.Query().Get("after_seq") == "0" {
					msgs = append(msgs, map[string]any{"seq": 1, "payload": "hello"})
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"messages": msgs, "last_seq": 1})
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		srv.Listener = l
		srv.Start()
		t.Cleanup(srv.Close)
	}
	return addr, start, joins
}

func fastRetries(t *testing.T) {
	t.Helper()
	oldMin, oldMax := subscribeRetryMin, subscribeRetryMax
	subscribeRetryMin, subscribeRetryMax = 20*time.Millisecond, 100*time.Millisecond
	t.Cleanup(func() { subscribeRetryMin, subscribeRetryMax = oldMin, oldMax })
}

// Wotan comes up after the service: the subscription arrives and messages
// flow, with no restart of the service.
func TestSubscribeWithRetry_JoinsWhenWotanStartsLater(t *testing.T) {
	fastRetries(t)
	addr, start, joins := lateWotan(t)
	c, err := NewClient(addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan string, 1)
	done := make(chan error, 1)
	go func() {
		done <- c.SubscribeWithRetry(ctx, "timeline.updates", "timeguru", func(m *Message) {
			select {
			case got <- m.Payload:
			default:
			}
		})
	}()

	time.Sleep(150 * time.Millisecond) // a few refused attempts
	if joins.Load() != 0 {
		t.Fatal("joined before Wotan existed")
	}
	start()
	select {
	case p := <-got:
		if p != "hello" {
			t.Errorf("payload %q, want hello", p)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("no message 5 s after Wotan came up (joins=%d)", joins.Load())
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("returned %v, want context.Canceled", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("SubscribeWithRetry did not stop when ctx ended")
	}
}

// Join-only (nil onMsg) returns once Wotan approves.
func TestSubscribeWithRetry_JoinOnlyReturnsWhenJoined(t *testing.T) {
	fastRetries(t)
	addr, start, joins := lateWotan(t)
	c, err := NewClient(addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	done := make(chan error, 1)
	go func() {
		done <- c.SubscribeWithRetry(context.Background(), "architecture.updates", "architect-service", nil)
	}()
	time.Sleep(100 * time.Millisecond)
	start()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("join-only returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("join-only never returned after Wotan came up")
	}
	if joins.Load() != 1 || !c.IsApproved("architecture.updates") {
		t.Errorf("joins=%d approved=%v, want 1 and true", joins.Load(), c.IsApproved("architecture.updates"))
	}
}

func TestRetryDelay_GrowsAndCaps(t *testing.T) {
	if d := retryDelay(1); d < 800*time.Millisecond || d > 1200*time.Millisecond {
		t.Errorf("retryDelay(1) = %v, want 1 s +/- 20 %%", d)
	}
	if d := retryDelay(3); d < 3200*time.Millisecond || d > 4800*time.Millisecond {
		t.Errorf("retryDelay(3) = %v, want 4 s +/- 20 %%", d)
	}
	if d := retryDelay(50); d > 36*time.Second {
		t.Errorf("retryDelay(50) = %v, want capped near 30 s", d)
	}
}
