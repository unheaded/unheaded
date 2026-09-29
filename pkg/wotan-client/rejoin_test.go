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
