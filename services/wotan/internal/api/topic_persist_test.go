// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"unheaded/services/wotan/internal/store"
)

// slowStore is a store whose Push starts only once the test says so, the
// way a Postgres INSERT is still in flight when the handler has answered.
type slowStore struct {
	store.MessageStore
	release chan struct{}
	done    chan error
}

func (s *slowStore) Push(ctx context.Context, msg *store.Message) (uuid.UUID, bool, error) {
	<-s.release
	id, over, err := s.MessageStore.Push(ctx, msg)
	s.done <- err
	return id, over, err
}

// The publish handler persisted in a goroutine with r.Context(), which the
// HTTP server cancels as soon as the handler returns. Any store that honours
// ctx (PGStore's ExecContext, MemoryStore's ctx.Err check) then failed with
// "context canceled" after the client had already been told 201: with
// --store-type postgres, published messages were almost never stored, and
// only a warning said so.
func TestPublishTopic_PersistOutlivesTheRequest(t *testing.T) {
	srv := setupTestServerWithTopics()
	slow := &slowStore{MessageStore: store.NewMemoryStore(100), release: make(chan struct{}), done: make(chan error, 1)}
	srv.Store = slow

	ts := httptest.NewServer(http.HandlerFunc(srv.TopicRouter))
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/v1/topics/persist.me/subscribe", "application/json", strings.NewReader(`{"display_name":"svc"}`))
	if err != nil {
		t.Fatal(err)
	}
	var sub struct {
		Subscriber TopicSubscriber `json:"subscriber"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&sub); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	body := `{"subscriber_id":"` + sub.Subscriber.SubscriberID + `","payload":"keep me"}`
	resp, err = http.Post(ts.URL+"/api/v1/topics/persist.me/publish", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("publish status = %d, want 201", resp.StatusCode)
	}

	// The response is in: the handler has returned. Now let the store work.
	close(slow.release)
	select {
	case err := <-slow.done:
		if err != nil {
			t.Fatalf("persist after the response: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("store Push never finished")
	}
	if n, _ := slow.Count(context.Background()); n != 1 {
		t.Fatalf("store holds %d messages, want 1", n)
	}
}
