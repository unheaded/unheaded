// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

// Package wotantest is a fake Wotan HTTP API for tests that need a Wotan
// which is not there yet: Reserve picks a loopback port that refuses
// connections, the service under test starts against it, and Start brings
// the fake up on that same port later. It speaks the slice of the REST API
// pkg/wotan-client uses (topic subscribe, publish, messages) and auto-approves
// every join, as Wotan does for names on topics.auto_approve.
package wotantest

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// Fake is a fake Wotan. Its zero value is not usable; call Reserve.
type Fake struct {
	// Addr is host:port, the value to give wotanClient.NewClient.
	Addr string

	mu        sync.Mutex
	joins     map[string][]string // topic -> display names, in join order
	polls     map[string]int      // topic -> GET messages count
	published map[string][]string // topic -> payloads
	queued    map[string][]message
	seq       int64
	srv       *http.Server
}

type message struct {
	MessageID string    `json:"message_id"`
	Topic     string    `json:"topic"`
	SenderID  string    `json:"sender_id"`
	CreatedAt time.Time `json:"created_at"`
	Seq       int64     `json:"seq"`
	Payload   string    `json:"payload"`
}

// Reserve returns a Fake whose Addr is a loopback port nothing listens on:
// bound and closed, so a client connecting there is refused at once.
func Reserve(t testing.TB) *Fake {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("wotantest: reserve port: %v", err)
	}
	addr := lis.Addr().String()
	if err := lis.Close(); err != nil {
		t.Fatalf("wotantest: release port: %v", err)
	}
	return &Fake{
		Addr:      addr,
		joins:     map[string][]string{},
		polls:     map[string]int{},
		published: map[string][]string{},
		queued:    map[string][]message{},
	}
}

// Start serves the fake on Addr until the test ends.
func (f *Fake) Start(t testing.TB) {
	t.Helper()
	var lis net.Listener
	var err error
	// Another test may grab the port between Reserve and Start; retry
	// briefly in case it is only a TIME_WAIT straggler.
	for i := 0; i < 20; i++ {
		if lis, err = net.Listen("tcp", f.Addr); err == nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("wotantest: listen %s: %v", f.Addr, err)
	}
	f.mu.Lock()
	f.srv = &http.Server{Handler: http.HandlerFunc(f.serve), ReadHeaderTimeout: 5 * time.Second}
	srv := f.srv
	f.mu.Unlock()
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(func() { _ = srv.Close() })
}

// Enqueue adds a message on topic for pollers to fetch.
func (f *Fake) Enqueue(topic, payload string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq++
	f.queued[topic] = append(f.queued[topic], message{
		MessageID: fmt.Sprintf("msg-%d", f.seq),
		Topic:     topic,
		SenderID:  "wotantest",
		CreatedAt: time.Now().UTC(),
		Seq:       f.seq,
		Payload:   payload,
	})
}

// Joins returns the display names that joined topic.
func (f *Fake) Joins(topic string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.joins[topic]...)
}

// Polls returns how many times topic's messages were fetched.
func (f *Fake) Polls(topic string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.polls[topic]
}

// Published returns the payloads published on topic.
func (f *Fake) Published(topic string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.published[topic]...)
}

// WaitFor polls cond every 10 ms until it holds or within passes.
func WaitFor(within time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return cond()
}

func (f *Fake) serve(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/health", "/ready":
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	rest, ok := strings.CutPrefix(r.URL.Path, "/api/v1/topics/")
	if !ok {
		http.NotFound(w, r)
		return
	}
	i := strings.LastIndex(rest, "/")
	if i <= 0 {
		http.NotFound(w, r)
		return
	}
	topic, action := rest[:i], rest[i+1:]

	switch {
	case action == "subscribe" && r.Method == http.MethodPost:
		var req struct {
			DisplayName string `json:"display_name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.mu.Lock()
		f.joins[topic] = append(f.joins[topic], req.DisplayName)
		n := len(f.joins[topic])
		f.mu.Unlock()
		writeJSON(w, http.StatusCreated, map[string]any{"subscriber": map[string]any{
			"subscriber_id": fmt.Sprintf("00000000-0000-0000-0000-%012d", n),
			"topic":         topic,
			"display_name":  req.DisplayName,
			"status":        "approved",
			"requested_at":  time.Now().UTC(),
		}})
	case action == "publish" && r.Method == http.MethodPost:
		var req struct {
			Payload string `json:"payload"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.mu.Lock()
		f.published[topic] = append(f.published[topic], req.Payload)
		f.mu.Unlock()
		writeJSON(w, http.StatusCreated, map[string]any{"message_id": "pub"})
	case action == "messages" && r.Method == http.MethodGet:
		var after int64
		_, _ = fmt.Sscan(r.URL.Query().Get("after_seq"), &after)
		f.mu.Lock()
		f.polls[topic]++
		out := []message{}
		for _, m := range f.queued[topic] {
			if m.Seq > after {
				out = append(out, m)
			}
		}
		last := f.seq
		f.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"messages": out, "last_seq": last})
	default:
		http.NotFound(w, r)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
