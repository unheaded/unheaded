// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package wotanClient

import (
	"context"
	"fmt"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestStreamCursor(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		requested, cursor, got int64
		single                 bool
		want                   int64
	}{
		{"forward", 3, 3, 4, true, 4},
		{"reordered duplicate stays", 3, 7, 5, true, 7},
		{"restart: at or below requested", 9, 9, 1, true, 1},
		{"restart: equal to requested", 9, 9, 9, true, 9},
		{"after rewind, forward again", 9, 1, 2, true, 2},
		{"fresh stream never rewinds", 0, 0, 1, true, 1},
		{"multi-topic low seq ignored", 9, 9, 1, false, 9},
		{"multi-topic forward", 9, 9, 12, false, 12},
	} {
		if got := streamCursor(tc.requested, tc.cursor, tc.got, tc.single); got != tc.want {
			t.Errorf("%s: streamCursor(%d,%d,%d,%v) = %d, want %d",
				tc.name, tc.requested, tc.cursor, tc.got, tc.single, got, tc.want)
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
