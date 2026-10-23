// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package logagg

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/transport"
)

// fakeWotan models the HTTP membership model: publish is rejected unless the
// subscriber joined the topic first, and approval depends on display name.
type fakeWotan struct {
	mu       sync.Mutex
	approved map[string]bool // display names on auto_approve
	joined   map[string]string
	pubs     map[string]int
	down     bool // answer 503 to everything
}

func newFakeWotan(approved ...string) *fakeWotan {
	f := &fakeWotan{approved: map[string]bool{}, joined: map[string]string{}, pubs: map[string]int{}}
	for _, a := range approved {
		f.approved[a] = true
	}
	return f
}

func (f *fakeWotan) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/topics/"), "/")
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	topic, action := parts[0], parts[1]
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	switch action {
	case "subscribe":
		var body struct {
			DisplayName string `json:"display_name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		status := "pending"
		if f.approved[body.DisplayName] {
			status = "approved"
			f.joined[topic] = "sub-" + topic
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"subscriber": map[string]any{
			"subscriber_id": "sub-" + topic, "topic": topic, "display_name": body.DisplayName, "status": status,
		}})
	case "publish":
		var body struct {
			SubscriberID string `json:"subscriber_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if f.joined[topic] != body.SubscriberID {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": map[string]string{"code": "Bad Request", "message": "invalid subscriber_id"}})
			return
		}
		f.pubs[topic]++
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "seq": f.pubs[topic]})
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeWotan) published(topic string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pubs[topic]
}

func TestConnect_JoinsEveryLevelThenPublishes(t *testing.T) {
	fw := newFakeWotan("svc")
	srv := httptest.NewServer(fw)
	defer srv.Close()

	cfg := transport.DefaultConfig()
	cfg.WotanHTTPAddr = srv.URL
	conn, err := Connect(context.Background(), cfg, "svc")
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if conn.Type() != transport.HTTP {
		t.Errorf("type = %s, want http", conn.Type())
	}
	for _, lvl := range levels {
		if _, ok := fw.joined["logs.svc."+lvl]; !ok {
			t.Errorf("logs.svc.%s not joined", lvl)
		}
	}

	if err := conn.Publish(context.Background(), "logs.svc.error", []byte(`{"m":1}`)); err != nil {
		t.Fatalf("Publish after join: %v", err)
	}
	if fw.published("logs.svc.error") != 1 {
		t.Errorf("server saw %d publishes, want 1", fw.published("logs.svc.error"))
	}
}

// A Wotan that is not answering when the service starts does not switch log
// forwarding off for good: the connection joins on a later publish. Connect
// used to fail, and every caller then ran with forwarding disabled for the
// life of the process (east, 2026-10-20: five services for 17 hours).
func TestConnect_WotanDownAtStartJoinsLater(t *testing.T) {
	old := joinRetry
	joinRetry = 20 * time.Millisecond
	t.Cleanup(func() { joinRetry = old })

	fw := newFakeWotan("svc")
	fw.down = true
	srv := httptest.NewServer(fw)
	defer srv.Close()
	cfg := transport.DefaultConfig()
	cfg.WotanHTTPAddr = srv.URL
	conn, err := Connect(context.Background(), cfg, "svc")
	if err != nil {
		t.Fatalf("Connect with Wotan down: %v", err)
	}
	if err := conn.Publish(context.Background(), "logs.svc.info", []byte(`{}`)); err == nil {
		t.Fatal("publish succeeded with Wotan down")
	}

	fw.mu.Lock()
	fw.down = false
	fw.mu.Unlock()
	time.Sleep(40 * time.Millisecond)
	if err := conn.Publish(context.Background(), "logs.svc.info", []byte(`{}`)); err != nil {
		t.Fatalf("publish once Wotan is back: %v", err)
	}
	if fw.published("logs.svc.info") != 1 {
		t.Errorf("server saw %d publishes, want 1", fw.published("logs.svc.info"))
	}
}

// A display name Wotan does not auto-approve must fail Connect loudly
// instead of yielding a connection whose every publish is rejected.
func TestConnect_UnapprovedNameFails(t *testing.T) {
	srv := httptest.NewServer(newFakeWotan("someone-else"))
	defer srv.Close()

	cfg := transport.DefaultConfig()
	cfg.WotanHTTPAddr = srv.URL
	conn, err := Connect(context.Background(), cfg, "svc")
	if err == nil {
		t.Fatal("expected error for unapproved display name")
	}
	if conn != nil {
		t.Error("connection must be nil on error")
	}
	if !strings.Contains(err.Error(), "pending") {
		t.Errorf("error should carry the status: %v", err)
	}
}
