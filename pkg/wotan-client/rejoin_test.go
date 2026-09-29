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
