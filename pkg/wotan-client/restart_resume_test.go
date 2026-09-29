// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package wotanClient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	chatpb "unheaded/services/wotan/proto"
)

func TestStreamCursor(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		requested, cursor, got int64
		want                   int64
	}{
		{"forward", 3, 3, 4, 4},
		{"reordered duplicate stays", 3, 7, 5, 7},
		{"restart: below requested", 9, 9, 1, 1},
		{"restart: equal to requested", 9, 9, 9, 9},
		{"after rewind, forward again", 9, 1, 2, 2},
		{"fresh stream never rewinds", 0, 0, 1, 1},
		{"live start (-1) never rewinds", -1, -1, 1, 1},
		// Seqs are global, so a pattern stream's low seq is a restart too
		// (it used to be ignored while seqs were per topic).
		{"pattern stream restart", 9, 12, 2, 2},
	} {
		if got := streamCursor(tc.requested, tc.cursor, tc.got); got != tc.want {
			t.Errorf("%s: streamCursor(%d,%d,%d) = %d, want %d",
				tc.name, tc.requested, tc.cursor, tc.got, got, tc.want)
		}
	}
}

// restartScenario drives a stream through a reconnect blip, a Wotan restart
// (seqs begin again at 1, two messages published before the client is
// back), and a second blip, checking what the client asks for and delivers.
func restartScenario(t *testing.T, srv *mockTopicStreamServer, ch <-chan *Message, cursor func() int64) {
	t.Helper()
	const topic = "tasks.created"
	recv := func(want ...string) {
		t.Helper()
		for _, w := range want {
			select {
			case m := <-ch:
				if string(m.Payload) != w {
					t.Fatalf("got %q (seq %d), want %q", m.Payload, m.Seq, w)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("timed out waiting for %q", w)
			}
		}
	}
	quiet := func(what string) {
		t.Helper()
		select {
		case m := <-ch:
			t.Fatalf("%s: unexpected redelivery %q (seq %d)", what, m.Payload, m.Seq)
		case <-time.After(400 * time.Millisecond):
		}
	}
	calls := func() int { srv.mu.Lock(); defer srv.mu.Unlock(); return len(srv.sinces) }
	lastSince := func() int64 { srv.mu.Lock(); defer srv.mu.Unlock(); return srv.sinces[len(srv.sinces)-1] }
	blip := func(during func()) {
		t.Helper()
		n := calls()
		srv.setStreamError(status.Error(codes.Unavailable, "blip"))
		if during != nil {
			during()
		}
		srv.setStreamError(nil)
		deadline := time.Now().Add(5 * time.Second)
		for {
			// A healthy reconnect is a call after the error was cleared.
			if calls() > n {
				srv.mu.Lock()
				healthy := srv.streamErr == nil
				srv.mu.Unlock()
				if healthy {
					time.Sleep(50 * time.Millisecond)
					return
				}
			}
			if time.Now().After(deadline) {
				t.Fatal("client did not reconnect")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}

	recv("old-1", "old-2", "old-3")

	blip(nil)
	quiet("reconnect without restart")
	if got := lastSince(); got != 3 {
		t.Errorf("reconnect asked since_seq=%d, want 3", got)
	}

	blip(func() {
		srv.mu.Lock()
		srv.messages = nil
		srv.mu.Unlock()
		srv.addMessage(topic, 1, "new-1")
		srv.addMessage(topic, 2, "new-2")
	})
	recv("new-1", "new-2")
	if cursor == nil {
		// no exported cursor; the since_seq checks cover it
	} else if got := cursor(); got != 2 {
		t.Errorf("cursor after restart = %d, want 2", got)
	}

	blip(nil)
	quiet("reconnect after restart")
	if got := lastSince(); got != 2 {
		t.Errorf("post-restart reconnect asked since_seq=%d, want 2", got)
	}
}

func seedOld(srv *mockTopicStreamServer) {
	srv.mu.Lock()
	srv.restartRule = true
	srv.killErr = status.Error(codes.Unavailable, "stream reset")
	srv.mu.Unlock()
	for i := 1; i <= 3; i++ {
		srv.addMessage("tasks.created", int64(i), fmt.Sprintf("old-%d", i))
	}
}

func TestTopicStreamClient_ResumeAcrossWotanRestart(t *testing.T) {
	srv, client, _ := setupBufconn(t)
	seedOld(srv)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client.Subscribe(ctx, "tasks.created", "resumer")
	ch, err := client.StreamMessages(ctx, "tasks.created")
	if err != nil {
		t.Fatal(err)
	}
	restartScenario(t, srv, ch, func() int64 {
		client.streamMu.RLock()
		defer client.streamMu.RUnlock()
		return client.streams["tasks.created"].lastSeq.Load()
	})
}

func TestGRPCClient_ResumeAcrossWotanRestart(t *testing.T) {
	srv, gc := setupGRPCBufconn(t)
	seedOld(srv)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := gc.Subscribe(ctx, "tasks.created", "resumer"); err != nil {
		t.Fatal(err)
	}
	ch, err := gc.StreamMessages(ctx, "tasks.created")
	if err != nil {
		t.Fatal(err)
	}
	// GRPCClient keeps its cursor inside the retry loop; what it asks the
	// server for (since_seq) is the observable cursor.
	restartScenario(t, srv, ch, nil)
}

// The retry budget is per outage, not per process: a stream that delivered
// or stayed up was healthy, and its failure starts a fresh run of retries.
// It used to be ten transient errors for the life of the process, after
// which the channel closed for good (timeguru logs "message channel
// closed" and stops consuming).
func TestGRPCClient_RetryBudgetResetsAfterHealthyStream(t *testing.T) {
	for _, tc := range []struct {
		name    string
		deliver bool
		age     time.Duration // healthyAge; 1h keeps the age path out
	}{
		{"stream delivered", true, time.Hour},
		{"idle stream stayed up", false, 100 * time.Millisecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, gc := setupGRPCBufconn(t) // maxRetries 3
			gc.healthyAge = tc.age
			srv.mu.Lock()
			srv.killErr = status.Error(codes.Unavailable, "blip")
			srv.mu.Unlock()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			gc.Subscribe(ctx, "tasks.created", "survivor")
			ch, err := gc.StreamMessages(ctx, "tasks.created")
			if err != nil {
				t.Fatal(err)
			}
			for i := 1; i <= 6; i++ {
				if tc.deliver {
					srv.addMessage("tasks.created", int64(i), fmt.Sprintf("m%d", i))
				}
				// Let the current stream be healthy, then break it.
				time.Sleep(150 * time.Millisecond)
				srv.setStreamError(status.Error(codes.Unavailable, "blip"))
				srv.setStreamError(nil)
				if tc.deliver {
					select {
					case m, ok := <-ch:
						if !ok {
							t.Fatalf("channel closed after blip %d", i)
						}
						if m.Seq != int64(i) {
							t.Fatalf("blip %d: got seq %d", i, m.Seq)
						}
					case <-time.After(5 * time.Second):
						t.Fatalf("blip %d: no message", i)
					}
				}
			}
			time.Sleep(200 * time.Millisecond)
			select {
			case _, ok := <-ch:
				if !ok {
					t.Fatal("channel closed after 6 blips with maxRetries 3")
				}
			default:
			}
		})
	}
}

// Backoff resets after a healthy stream. It used to double on every failure
// for the life of the stream (to maxBackoff, 30 s by default), so after a
// handful of Wotan blips each reconnect waited the maximum: a 30 s hole in
// the dashboard's ingest per blip.
func TestTopicStreamClient_BackoffResetsAfterHealthyStream(t *testing.T) {
	srv := newMockServer()
	srv.killErr = status.Error(codes.Unavailable, "blip")
	lis := startMockServer(t, srv)
	client, err := newTopicStreamClientWithDialer(lis, WithRetryPolicy(50*time.Millisecond, 10*time.Second, 100))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client.Subscribe(ctx, "tasks.created", "backoff")
	ch, err := client.StreamMessages(ctx, "tasks.created")
	if err != nil {
		t.Fatal(err)
	}
	var last time.Duration
	for i := 1; i <= 7; i++ {
		srv.addMessage("tasks.created", int64(i), fmt.Sprintf("m%d", i))
		if i > 1 {
			srv.setStreamError(status.Error(codes.Unavailable, "blip"))
			srv.setStreamError(nil)
		}
		start := time.Now()
		select {
		case m := <-ch:
			if m.Seq != int64(i) {
				t.Fatalf("round %d: got seq %d", i, m.Seq)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("round %d: no message", i)
		}
		last = time.Since(start)
	}
	// Six doublings from 50 ms would be 3.2 s.
	if last > time.Second {
		t.Errorf("7th reconnect took %v, want the initial backoff", last)
	}
}

// startMockServer serves srv on a fresh bufconn listener.
func startMockServer(t *testing.T, srv *mockTopicStreamServer) *bufconn.Listener {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	gs := grpc.NewServer()
	chatpb.RegisterTopicStreamServer(gs, srv)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)
	return lis
}

// A live view starts at the live position: the dashboard used to replay the
// whole history Wotan held (~30k events) on every start, counting it as
// just-ingested and flooding its WebSocket broadcast (46k drops in the
// first minute, 2026-09-29).
func TestTopicStreamClient_LiveStartSkipsReplay(t *testing.T) {
	srv := newMockServer()
	srv.killErr = status.Error(codes.Unavailable, "blip")
	for i := 1; i <= 3; i++ {
		srv.addMessage("tasks.created", int64(i), fmt.Sprintf("old-%d", i))
	}
	lis := startMockServer(t, srv)
	client, err := newTopicStreamClientWithDialer(lis, WithRetryPolicy(10*time.Millisecond, 100*time.Millisecond, 100), WithLiveStart())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client.Subscribe(ctx, "tasks.created", "live")
	ch, err := client.StreamMessages(ctx, "tasks.created")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-srv.streamStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("no stream opened")
	}
	select {
	case m := <-ch:
		t.Fatalf("replayed history: %q", m.Payload)
	case <-time.After(300 * time.Millisecond):
	}
	srv.mu.Lock()
	first := srv.sinces[0]
	srv.mu.Unlock()
	if first != -1 {
		t.Errorf("first since_seq = %d, want -1", first)
	}

	// Live messages arrive, and a reconnect resumes from them as usual.
	srv.addMessage("tasks.created", 4, "new-4")
	srv.live <- &chatpb.TopicEvent{Topic: "tasks.created", Seq: 4, Payload: []byte("new-4")}
	select {
	case m := <-ch:
		if string(m.Payload) != "new-4" {
			t.Fatalf("got %q, want new-4", m.Payload)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("live message not delivered")
	}
	srv.addMessage("tasks.created", 5, "new-5")
	srv.setStreamError(status.Error(codes.Unavailable, "blip"))
	srv.setStreamError(nil)
	select {
	case m := <-ch:
		if string(m.Payload) != "new-5" {
			t.Fatalf("after reconnect got %q, want new-5", m.Payload)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reconnect did not resume")
	}
}

// Over the HTTP fallback a live start takes the server's last_seq as the
// cursor and delivers only what comes after it.
func TestTopicStreamClient_LiveStartHTTPFallback(t *testing.T) {
	var mu sync.Mutex
	var afters []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		after := r.URL.Query().Get("after_seq")
		mu.Lock()
		afters = append(afters, after)
		mu.Unlock()
		msgs := []map[string]any{}
		switch after {
		case "0": // the history, which a live start must not deliver
			msgs = append(msgs, map[string]any{"seq": 7, "topic": "t.x", "payload": "old"})
		case "7":
			msgs = append(msgs, map[string]any{"seq": 8, "topic": "t.x", "payload": "new"})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"messages": msgs, "last_seq": 8})
	}))
	defer ts.Close()
	hc, _ := NewClient(strings.TrimPrefix(ts.URL, "http://"))
	c := &TopicStreamClient{httpClient: hc, liveStart: true}
	as := &activeStream{sc: newSafeChannel(10)}
	as.lastSeq.Store(-1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go c.pollHTTPFallback(ctx, "t.x", as)

	// last_seq on the probe is 8, so nothing (not 7, not 8) is delivered...
	select {
	case m := <-as.sc.ch:
		t.Fatalf("delivered %q from before the live start", m.Payload)
	case <-time.After(1500 * time.Millisecond):
	}
	mu.Lock()
	defer mu.Unlock()
	if len(afters) < 2 || afters[0] != "0" || afters[1] != "8" {
		t.Errorf("after_seq sequence %v, want [0 8 ...]", afters)
	}
}
