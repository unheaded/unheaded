// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
)

func pgConnStr() string {
	cs := os.Getenv("WOTAN_PG_TEST")
	if cs == "" {
		cs = "postgres://unheaded:unheaded_dev@localhost:5432/unheaded?sslmode=disable"
	}
	return cs
}

func skipIfNoPG(t *testing.T) *PGStore {
	t.Helper()
	s, err := NewPGStore(pgConnStr(), 10000)
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	// Clean test data
	s.db.Exec("DELETE FROM wotan_messages WHERE room LIKE 'test-%'")
	s.db.Exec("DELETE FROM wotan_sequences WHERE topic LIKE 'test-%'")
	return s
}

func TestPGStorePushGet(t *testing.T) {
	s := skipIfNoPG(t)
	defer s.Close()
	ctx := context.Background()

	msg := &Message{
		ID:        uuid.New(),
		CreatorID: uuid.New(),
		RoomID:    "test-room-" + uuid.New().String()[:8],
		Content:   "hello from pg_store test",
		Timestamp: time.Now().Truncate(time.Millisecond),
	}

	id, _, err := s.Push(ctx, msg)
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	if id != msg.ID {
		t.Fatalf("id mismatch: %v != %v", id, msg.ID)
	}

	got, err := s.Get(ctx, msg.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Content != msg.Content {
		t.Errorf("content mismatch: %q != %q", got.Content, msg.Content)
	}
	if got.RoomID != msg.RoomID {
		t.Errorf("room mismatch: %s != %s", got.RoomID, msg.RoomID)
	}
}

func TestPGStoreGetNotFound(t *testing.T) {
	s := skipIfNoPG(t)
	defer s.Close()

	_, err := s.Get(context.Background(), uuid.New())
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestPGStoreDelete(t *testing.T) {
	s := skipIfNoPG(t)
	defer s.Close()
	ctx := context.Background()

	msg := &Message{
		ID:        uuid.New(),
		CreatorID: uuid.New(),
		RoomID:    "test-del-" + uuid.New().String()[:8],
		Content:   "to be deleted",
		Timestamp: time.Now(),
	}
	s.Push(ctx, msg)

	err := s.Delete(ctx, msg.ID, msg.CreatorID, msg.Content)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err = s.Get(ctx, msg.ID)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestPGStoreGetSince(t *testing.T) {
	s := skipIfNoPG(t)
	defer s.Close()
	ctx := context.Background()

	room := "test-since-" + uuid.New().String()[:8]
	for i := 0; i < 5; i++ {
		msg := &Message{
			ID:        uuid.New(),
			CreatorID: uuid.New(),
			RoomID:    room,
			Content:   "msg",
			Timestamp: time.Now(),
		}
		s.PushWithSeq(ctx, msg, int64(i+1))
	}

	msgs, err := s.GetSince(ctx, room, 2, 100)
	if err != nil {
		t.Fatalf("get_since: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages after seq 2, got %d", len(msgs))
	}
}

func TestPGStoreNextSeq(t *testing.T) {
	s := skipIfNoPG(t)
	defer s.Close()
	ctx := context.Background()

	topic := "test-seq-" + uuid.New().String()[:8]
	seq1, err := s.NextSeq(ctx, topic)
	if err != nil {
		t.Fatalf("next_seq 1: %v", err)
	}
	if seq1 != 1 {
		t.Fatalf("expected seq 1, got %d", seq1)
	}

	seq2, err := s.NextSeq(ctx, topic)
	if err != nil {
		t.Fatalf("next_seq 2: %v", err)
	}
	if seq2 != 2 {
		t.Fatalf("expected seq 2, got %d", seq2)
	}

	// Verify persistence
	last, err := s.LastSeq(ctx, topic)
	if err != nil {
		t.Fatalf("last_seq: %v", err)
	}
	if last != 2 {
		t.Fatalf("expected last seq 2, got %d", last)
	}
}

func TestPGStoreCount(t *testing.T) {
	s := skipIfNoPG(t)
	defer s.Close()
	ctx := context.Background()

	// Count before (may have existing data)
	before, _ := s.Count(ctx)

	msg := &Message{
		ID:        uuid.New(),
		CreatorID: uuid.New(),
		RoomID:    "test-count-" + uuid.New().String()[:8],
		Content:   "count test",
		Timestamp: time.Now(),
	}
	s.Push(ctx, msg)

	after, _ := s.Count(ctx)
	if after != before+1 {
		t.Fatalf("count: expected %d, got %d", before+1, after)
	}
}
