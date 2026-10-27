// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package grpc

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"unheaded/services/wotan/internal/store"
	chatpb "unheaded/services/wotan/proto"
)

// recordingStore is a MessageStore that keeps what it is pushed.
type recordingStore struct {
	mu     sync.Mutex
	pushed []*store.Message
	got    chan struct{}
	err    error
}

func newRecordingStore() *recordingStore { return &recordingStore{got: make(chan struct{}, 16)} }

func (r *recordingStore) Push(ctx context.Context, m *store.Message) (uuid.UUID, bool, error) {
	if err := ctx.Err(); err != nil {
		return uuid.Nil, false, err // a cancelled context must not reach the store
	}
	r.mu.Lock()
	r.pushed = append(r.pushed, m)
	r.mu.Unlock()
	r.got <- struct{}{}
	return m.ID, false, r.err
}

// PublishTopic only ever calls Push. Every other MessageStore method comes
// from the embedded nil interface, so a call to one panics loudly instead of
// passing silently, and the fake keeps compiling if the interface grows.
type fullStore struct {
	store.MessageStore
	*recordingStore
}

func (f fullStore) Push(ctx context.Context, m *store.Message) (uuid.UUID, bool, error) {
	return f.recordingStore.Push(ctx, m)
}

// M-038 / sweep F-09: the HTTP topic publish path persisted every message to
// the store, the gRPC PublishTopic path never did, so gRPC-only publishers
// (signed config.* and drift.* events) existed only in the ring buffer and
// were gone after a Wotan restart.
func TestPublishTopic_PersistsToStore(t *testing.T) {
	svc := productionTopicService()
	svc.AutoApprove = allowList{"service": true}
	rs := newRecordingStore()
	svc.Store = fullStore{recordingStore: rs}

	ctx, cancel := context.WithCancel(context.Background())
	resp, err := svc.PublishTopic(ctx, &chatpb.TopicPublishRequest{Topic: "alerts.critical", Payload: []byte(`{"x":1}`)})
	if err != nil {
		t.Fatal(err)
	}
	cancel() // the RPC is over: the write must not depend on its context
	select {
	case <-rs.got:
	case <-time.After(2 * time.Second):
		t.Fatal("a gRPC publish never reached the store")
	}
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if len(rs.pushed) != 1 {
		t.Fatalf("pushed %d messages, want 1", len(rs.pushed))
	}
	m := rs.pushed[0]
	if m.ID.String() != resp.MessageId || m.RoomID != "alerts.critical" || m.Content != `{"x":1}` || m.Timestamp.IsZero() {
		t.Fatalf("stored %+v, want the published message (id %s)", m, resp.MessageId)
	}
}

// A refused publish must not reach the store either.
func TestPublishTopic_RefusedPublishIsNotStored(t *testing.T) {
	svc := productionTopicService()
	svc.AutoApprove = allowList{"service": true}
	rs := newRecordingStore()
	svc.Store = fullStore{recordingStore: rs}
	_, err := svc.PublishTopic(context.Background(), &chatpb.TopicPublishRequest{
		Topic: "alerts.critical", Payload: []byte(`{}`), Metadata: map[string]string{"display_name": "intruder"},
	})
	if err == nil {
		t.Fatal("a publisher off the allowlist was accepted")
	}
	select {
	case <-rs.got:
		t.Fatal("a refused publish reached the store")
	case <-time.After(200 * time.Millisecond):
	}
}

// A store error is logged, never turned into a failed publish: the message is
// already in the ring buffer and on its way to subscribers.
func TestPublishTopic_StoreErrorDoesNotFailThePublish(t *testing.T) {
	svc := productionTopicService()
	svc.AutoApprove = allowList{"service": true}
	rs := newRecordingStore()
	rs.err = errors.New("disk full")
	svc.Store = fullStore{recordingStore: rs}
	if _, err := svc.PublishTopic(context.Background(), &chatpb.TopicPublishRequest{Topic: "alerts.critical", Payload: []byte(`{}`)}); err != nil {
		t.Fatalf("a store error failed the publish: %v", err)
	}
	select {
	case <-rs.got:
	case <-time.After(2 * time.Second):
		t.Fatal("the store was never asked")
	}
}
