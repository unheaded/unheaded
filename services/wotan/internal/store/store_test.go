// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package store

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestMessage_Validate(t *testing.T) {
	tests := []struct {
		name    string
		msg     *Message
		wantErr error
	}{
		{
			name:    "nil message",
			msg:     nil,
			wantErr: ErrInvalidMessage,
		},
		{
			name: "valid message",
			msg: &Message{
				ID:        uuid.New(),
				CreatorID: uuid.New(),
				RoomID:    "general",
				Content:   "Hello, World!",
				Timestamp: time.Now(),
			},
			wantErr: nil,
		},
		{
			name: "zero ID",
			msg: &Message{
				ID:        uuid.Nil,
				CreatorID: uuid.New(),
				RoomID:    "general",
				Content:   "Hello, World!",
				Timestamp: time.Now(),
			},
			wantErr: ErrInvalidMessage,
		},
		{
			name: "zero CreatorID",
			msg: &Message{
				ID:        uuid.New(),
				CreatorID: uuid.Nil,
				RoomID:    "general",
				Content:   "Hello, World!",
				Timestamp: time.Now(),
			},
			wantErr: ErrInvalidMessage,
		},
		{
			name: "empty RoomID",
			msg: &Message{
				ID:        uuid.New(),
				CreatorID: uuid.New(),
				RoomID:    "",
				Content:   "Hello, World!",
				Timestamp: time.Now(),
			},
			wantErr: ErrInvalidMessage,
		},
		{
			name: "empty Content",
			msg: &Message{
				ID:        uuid.New(),
				CreatorID: uuid.New(),
				RoomID:    "general",
				Content:   "",
				Timestamp: time.Now(),
			},
			wantErr: ErrInvalidMessage,
		},
		{
			name: "zero Timestamp",
			msg: &Message{
				ID:        uuid.New(),
				CreatorID: uuid.New(),
				RoomID:    "general",
				Content:   "Hello, World!",
				Timestamp: time.Time{},
			},
			wantErr: ErrInvalidMessage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.msg.Validate()
			if err != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestQueryOptions_Defaults(t *testing.T) {
	// QueryOptions should have sensible zero values
	opts := QueryOptions{}

	if opts.RoomID != "" {
		t.Error("RoomID should be empty by default")
	}
	if !opts.Since.IsZero() {
		t.Error("Since should be zero by default")
	}
	if !opts.Until.IsZero() {
		t.Error("Until should be zero by default")
	}
	if opts.Limit != 0 {
		t.Error("Limit should be 0 by default")
	}
	if opts.Offset != 0 {
		t.Error("Offset should be 0 by default")
	}
	if opts.Ascending != false {
		t.Error("Ascending should be false by default")
	}
}

func TestStoreStats_ZeroValues(t *testing.T) {
	stats := StoreStats{}

	if stats.Count != 0 {
		t.Error("Count should be 0 by default")
	}
	if stats.Capacity != 0 {
		t.Error("Capacity should be 0 by default")
	}
	if !stats.OldestTimestamp.IsZero() {
		t.Error("OldestTimestamp should be zero by default")
	}
	if !stats.NewestTimestamp.IsZero() {
		t.Error("NewestTimestamp should be zero by default")
	}
	if stats.IsWrapping != false {
		t.Error("IsWrapping should be false by default")
	}
	if stats.BytesUsed != 0 {
		t.Error("BytesUsed should be 0 by default")
	}
}

func TestSnapshotMetadata_ZeroValues(t *testing.T) {
	meta := SnapshotMetadata{}

	if meta.ID != "" {
		t.Error("ID should be empty by default")
	}
	if !meta.CreatedAt.IsZero() {
		t.Error("CreatedAt should be zero by default")
	}
	if meta.MessageCount != 0 {
		t.Error("MessageCount should be 0 by default")
	}
	if meta.ByteSize != 0 {
		t.Error("ByteSize should be 0 by default")
	}
}

// Helper function to create valid test messages
func newTestMessage(roomID, content string) *Message {
	return &Message{
		ID:        uuid.New(),
		CreatorID: uuid.New(),
		RoomID:    roomID,
		Content:   content,
		Timestamp: time.Now(),
	}
}

// Benchmark message validation
func BenchmarkMessage_Validate(b *testing.B) {
	msg := &Message{
		ID:        uuid.New(),
		CreatorID: uuid.New(),
		RoomID:    "general",
		Content:   "Hello, World!",
		Timestamp: time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = msg.Validate()
	}
}
