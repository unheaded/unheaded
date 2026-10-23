// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package wotanClient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeWotan approves members in memory, like Wotan; restart() forgets them.
type fakeWotan struct {
	mu        sync.Mutex
	approved  map[string]bool
	next      int
	status    string // status given to new members
	published []string
	attempts  int // publish requests, refused or not
}

func newFakeWotan(t *testing.T) (*fakeWotan, string) {
	fw := &fakeWotan{approved: map[string]bool{}, status: "approved"}
	ts := httptest.NewServer(http.HandlerFunc(fw.serve))
	t.Cleanup(ts.Close)
	return fw, strings.TrimPrefix(ts.URL, "http://")
}

func (fw *fakeWotan) restart() {
	fw.mu.Lock()
	fw.approved = map[string]bool{}
	fw.mu.Unlock()
}

func (fw *fakeWotan) serve(w http.ResponseWriter, r *http.Request) {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	switch {
	case strings.HasSuffix(r.URL.Path, "/subscribe"):
		var req struct {
			DisplayName string `json:"display_name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		fw.next++
		id := fmt.Sprintf("00000000-0000-0000-0000-%012d", fw.next)
		fw.approved[id] = fw.status == "approved"
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"subscriber": map[string]string{
			"subscriber_id": id, "display_name": req.DisplayName, "status": fw.status}})
	case strings.HasSuffix(r.URL.Path, "/publish"):
		var req struct {
			SubscriberID string `json:"subscriber_id"`
			Payload      string `json:"payload"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		fw.attempts++
		if !fw.approved[req.SubscriberID] {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"success":false,"error":{"code":"Forbidden","message":"subscriber not approved"}}`))
			return
		}
		fw.published = append(fw.published, req.SubscriberID+":"+req.Payload)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"success":true}`))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// Wotan keeps memberships in memory. After it restarts, a long-lived
// publisher's subscriber ID is unknown and every publish was refused until
// the publisher itself restarted: sophia's and kanban's log forwarders
// (pkg/logagg joins once at startup) were dropping logs live on 2026-09-29.
func TestPublish_RejoinsAfterWotanRestart(t *testing.T) {
	fw, addr := newFakeWotan(t)
	c, err := NewClient(addr)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := c.Subscribe(ctx, "logs.svc.info", "svc"); err != nil {
		t.Fatal(err)
	}
	if err := c.Publish(ctx, "logs.svc.info", []byte("before")); err != nil {
		t.Fatal(err)
	}

	fw.restart()
	if err := c.Publish(ctx, "logs.svc.info", []byte("after")); err != nil {
		t.Fatalf("publish after restart: %v", err)
	}
	// And the new membership sticks: no rejoin per message.
	if err := c.Publish(ctx, "logs.svc.info", []byte("again")); err != nil {
		t.Fatal(err)
	}
	fw.mu.Lock()
	defer fw.mu.Unlock()
	want := []string{
		"00000000-0000-0000-0000-000000000001:before",
		"00000000-0000-0000-0000-000000000002:after",
		"00000000-0000-0000-0000-000000000002:again",
	}
	if fmt.Sprint(fw.published) != fmt.Sprint(want) {
		t.Errorf("published %v, want %v", fw.published, want)
	}
	if fw.next != 2 {
		t.Errorf("%d subscribes, want 2", fw.next)
	}
}

// Rejoin is attempted once; a publisher Wotan will not approve gets an error,
// not a loop.
func TestPublish_RejoinNotApproved(t *testing.T) {
	fw, addr := newFakeWotan(t)
	c, _ := NewClient(addr)
	ctx := context.Background()
	if _, err := c.Subscribe(ctx, "logs.svc.info", "svc"); err != nil {
		t.Fatal(err)
	}
	fw.restart()
	fw.mu.Lock()
	fw.status = "pending"
	fw.mu.Unlock()

	err := c.Publish(ctx, "logs.svc.info", []byte("x"))
	if !errors.Is(err, ErrSubscriptionPending) {
		t.Errorf("err = %v, want ErrSubscriptionPending", err)
	}
	fw.mu.Lock()
	defer fw.mu.Unlock()
	if fw.next != 2 || len(fw.published) != 0 {
		t.Errorf("subscribes=%d published=%v, want one rejoin and nothing published", fw.next, fw.published)
	}
}

// A pending subscription is put to Wotan again now and then. It used to be
// final: kev-poller on east joined while Wotan's allowlist lacked it, and
// kept failing "subscription pending approval" for 17 hours after Wotan
// restarted with it on the list, until kev-poller itself was restarted.
func TestPublish_PendingIsRechecked(t *testing.T) {
	newPending := func(t *testing.T) (*fakeWotan, *Client) {
		fw, addr := newFakeWotan(t)
		fw.status = "pending"
		c, _ := NewClient(addr)
		if sub, err := c.Subscribe(context.Background(), "threats.kev", "kev"); err != nil || sub.Status != "pending" {
			t.Fatalf("subscribe: %v %v", sub, err)
		}
		return fw, c
	}
	ctx := context.Background()

	t.Run("Wotan restarted with the name allowlisted", func(t *testing.T) {
		fw, c := newPending(t)
		c.pendingRecheck = 10 * time.Millisecond
		if err := c.Publish(ctx, "threats.kev", []byte("early")); !errors.Is(err, ErrSubscriptionPending) {
			t.Fatalf("publish while pending: %v", err)
		}
		fw.restart()
		fw.mu.Lock()
		fw.status = "approved"
		fw.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
		if err := c.Publish(ctx, "threats.kev", []byte("late")); err != nil {
			t.Fatalf("publish after approval: %v", err)
		}
		if !c.IsApproved("threats.kev") {
			t.Error("subscription still recorded pending")
		}
		fw.mu.Lock()
		defer fw.mu.Unlock()
		if len(fw.published) != 1 || !strings.HasSuffix(fw.published[0], ":late") {
			t.Errorf("published %v, want [late]", fw.published)
		}
	})

	t.Run("an admin approved the pending member", func(t *testing.T) {
		fw, c := newPending(t)
		c.pendingRecheck = 10 * time.Millisecond
		_ = c.Publish(ctx, "threats.kev", []byte("early"))
		sub, _ := c.GetSubscriber("threats.kev")
		fw.mu.Lock()
		fw.approved[sub.SubscriberID] = true
		fw.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
		for _, p := range []string{"a", "b"} {
			if err := c.Publish(ctx, "threats.kev", []byte(p)); err != nil {
				t.Fatalf("publish %s: %v", p, err)
			}
		}
		fw.mu.Lock()
		defer fw.mu.Unlock()
		if fw.next != 1 {
			t.Errorf("%d subscribes, want the approved member kept", fw.next)
		}
	})

	t.Run("still pending: asked at most once per interval", func(t *testing.T) {
		fw, c := newPending(t)
		c.pendingRecheck = time.Hour
		for i := 0; i < 5; i++ {
			if err := c.Publish(ctx, "threats.kev", []byte("x")); !errors.Is(err, ErrSubscriptionPending) {
				t.Fatalf("publish %d: %v", i, err)
			}
		}
		fw.mu.Lock()
		defer fw.mu.Unlock()
		if fw.attempts != 0 || fw.next != 1 {
			t.Errorf("attempts=%d subscribes=%d, want 0 and 1", fw.attempts, fw.next)
		}
	})
}

// A 201 with no subscriber in it is an error, not a nil membership that
// crashes the next publish.
func TestSubscribe_NoSubscriberInResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(ts.Close)
	c, _ := NewClient(strings.TrimPrefix(ts.URL, "http://"))
	if _, err := c.Subscribe(context.Background(), "t", "svc"); err == nil {
		t.Fatal("subscribe succeeded without a subscriber")
	}
	if err := c.Publish(context.Background(), "t", []byte("x")); err == nil || !strings.Contains(err.Error(), "not subscribed") {
		t.Errorf("publish: %v, want not subscribed", err)
	}
}

// Close stops HTTP pollers. A poller on a quiet topic used to notice Close
// only when a message arrived, and kept polling every 500 ms until its ctx
// ended (never, for a Background ctx). A second stream on the same topic
// also lost its map entry when the first ended, so Close missed it.
func TestClose_StopsPollers(t *testing.T) {
	var polls atomic.Int64
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		polls.Add(1)
		_, _ = w.Write([]byte(`{"messages":[],"last_seq":0}`))
	}))
	t.Cleanup(ts.Close)
	c, _ := NewClient(strings.TrimPrefix(ts.URL, "http://"))
	c.subscribers["quiet"] = &Subscriber{SubscriberID: "s", Status: "approved"}

	ctx1, cancel1 := context.WithCancel(context.Background())
	if _, err := c.StreamMessages(ctx1, "quiet"); err != nil {
		t.Fatal(err)
	}
	ch2, err := c.StreamMessages(context.Background(), "quiet")
	if err != nil {
		t.Fatal(err)
	}
	cancel1() // the first stream ends; the second must stay closable
	time.Sleep(700 * time.Millisecond)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case _, ok := <-ch2:
		if ok {
			t.Fatal("got a message")
		}
	case <-time.After(time.Second):
		t.Fatal("Close left the second stream's channel open")
	}
	time.Sleep(100 * time.Millisecond) // a poll already in flight lands
	before := polls.Load()
	time.Sleep(1200 * time.Millisecond)
	if after := polls.Load(); after != before {
		t.Errorf("%d polls after Close", after-before)
	}
}

// Close and the lazy gRPC client do not race (run with -race): Close
// cleared grpcClient under chanMu while getOrCreateGRPCClient reads and
// sets it under grpcMu.
func TestClose_RacesNothingWithGRPCInit(t *testing.T) {
	c, err := NewClientWithGRPC("127.0.0.1:1", "127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 50; i++ {
			_ = c.ProbeGRPC()
		}
	}()
	for i := 0; i < 50; i++ {
		_ = c.Close()
	}
	<-done
	_ = c.Close()
}

// "#" starts a URL fragment: a topic pattern like "logs.#" put into the path
// unescaped reached Wotan as "/api/v1/topics/logs." with the rest (and the
// query) cut off. Topics are path-escaped.
func TestTopicPathIsEscaped(t *testing.T) {
	var paths []string
	var queries []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		queries = append(queries, r.URL.RawQuery)
		switch {
		case strings.HasSuffix(r.URL.Path, "/subscribe"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"subscriber":{"subscriber_id":"00000000-0000-0000-0000-000000000009","status":"approved"}}`))
		default:
			_, _ = w.Write([]byte(`{"messages":[],"last_seq":0}`))
		}
	}))
	defer ts.Close()
	c, _ := NewClient(strings.TrimPrefix(ts.URL, "http://"))
	ctx := context.Background()
	if _, err := c.Subscribe(ctx, "logs.#", "svc"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.getMessagesPage(ctx, "logs.#", 7, 5); err != nil {
		t.Fatal(err)
	}
	want := []string{"/api/v1/topics/logs.#/subscribe", "/api/v1/topics/logs.#/messages"}
	if fmt.Sprint(paths) != fmt.Sprint(want) || queries[1] != "after_seq=7&limit=5" {
		t.Errorf("paths %v queries %v, want %v and after_seq=7&limit=5", paths, queries, want)
	}
}

// A poller never delivers a seq at or below its cursor, whatever the server
// sends: defence in depth against a server (or an old Wotan) that ignores
// after_seq and re-sends what it holds.
func TestPollers_SkipAlreadyDelivered(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/subscribe") {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"subscriber":{"subscriber_id":"00000000-0000-0000-0000-000000000009","status":"approved"}}`))
			return
		}
		// Ignores after_seq: always the same three.
		_, _ = w.Write([]byte(`{"messages":[{"seq":1,"topic":"t.x"},{"seq":2,"topic":"t.x"},{"seq":3,"topic":"t.x"}],"last_seq":3}`))
	}))
	defer ts.Close()
	addr := strings.TrimPrefix(ts.URL, "http://")
	collect := func(ch <-chan *Message) []int64 {
		var seqs []int64
		deadline := time.After(1500 * time.Millisecond)
		for {
			select {
			case m := <-ch:
				seqs = append(seqs, m.Seq)
			case <-deadline:
				return seqs
			}
		}
	}

	c, _ := NewClient(addr)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := c.Subscribe(ctx, "t.x", "svc"); err != nil {
		t.Fatal(err)
	}
	ch, err := c.StreamMessages(ctx, "t.x")
	if err != nil {
		t.Fatal(err)
	}
	if got := collect(ch); fmt.Sprint(got) != "[1 2 3]" {
		t.Errorf("Client poller delivered %v, want [1 2 3]", got)
	}

	tc := &TopicStreamClient{httpClient: c}
	as := &activeStream{sc: newSafeChannel(10)}
	go tc.pollHTTPFallback(ctx, "t.x", as)
	if got := collect(as.sc.ch); fmt.Sprint(got) != "[1 2 3]" {
		t.Errorf("fallback poller delivered %v, want [1 2 3]", got)
	}
}

// A message the fallback's pattern filter drops still moves the cursor, or
// every poll would fetch it again.
func TestFallbackPoller_FilteredMessagesAdvanceCursor(t *testing.T) {
	var afters []string
	var mu sync.Mutex
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		afters = append(afters, r.URL.Query().Get("after_seq"))
		mu.Unlock()
		if r.URL.Query().Get("after_seq") == "0" {
			_, _ = w.Write([]byte(`{"messages":[{"seq":1,"topic":"other.y"},{"seq":2,"topic":"t.x"},{"seq":3,"topic":"other.y"}],"last_seq":3}`))
			return
		}
		_, _ = w.Write([]byte(`{"messages":[],"last_seq":3}`))
	}))
	defer ts.Close()
	c, _ := NewClient(strings.TrimPrefix(ts.URL, "http://"))
	tc := &TopicStreamClient{httpClient: c}
	as := &activeStream{sc: newSafeChannel(10)}
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	go tc.pollHTTPFallback(ctx, "t.x", as)
	<-ctx.Done()
	mu.Lock()
	defer mu.Unlock()
	if len(afters) < 2 || afters[1] != "3" {
		t.Errorf("after_seq sequence %v, want [0 3 ...]", afters)
	}
}
