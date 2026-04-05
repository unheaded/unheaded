// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

package store

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestEncodeDecodeMessage(t *testing.T) {
	msg := &Message{
		ID:        uuid.New(),
		CreatorID: uuid.New(),
		RoomID:    "test-room",
		Content:   "hello world",
		Timestamp: time.Now().Truncate(time.Millisecond),
	}

	data, err := EncodeMessage(msg)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	if data[0] != encodingVersion {
		t.Fatalf("expected version %d, got %d", encodingVersion, data[0])
	}

	decoded, err := DecodeMessage(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if decoded.ID != msg.ID {
		t.Errorf("ID mismatch: %v != %v", decoded.ID, msg.ID)
	}
	if decoded.RoomID != msg.RoomID {
		t.Errorf("RoomID mismatch: %s != %s", decoded.RoomID, msg.RoomID)
	}
	if decoded.Content != msg.Content {
		t.Errorf("Content mismatch: %s != %s", decoded.Content, msg.Content)
	}
}

func TestEncodeNilMessage(t *testing.T) {
	_, err := EncodeMessage(nil)
	if err != ErrInvalidMessage {
		t.Fatalf("expected ErrInvalidMessage, got %v", err)
	}
}

func TestDecodeShortData(t *testing.T) {
	_, err := DecodeMessage([]byte{})
	if err == nil {
		t.Fatal("expected error for empty data")
	}
	_, err = DecodeMessage([]byte{1})
	if err == nil {
		t.Fatal("expected error for single byte")
	}
}

func TestDecodeBadVersion(t *testing.T) {
	_, err := DecodeMessage([]byte{99, '{', '}'})
	if err == nil {
		t.Fatal("expected error for bad version")
	}
}

func TestEncodeLargeContent(t *testing.T) {
	msg := &Message{
		ID:        uuid.New(),
		CreatorID: uuid.New(),
		RoomID:    "big-room",
		Content:   strings.Repeat("x", 100_000),
		Timestamp: time.Now(),
	}

	data, err := EncodeMessage(msg)
	if err != nil {
		t.Fatalf("encode large: %v", err)
	}

	decoded, err := DecodeMessage(data)
	if err != nil {
		t.Fatalf("decode large: %v", err)
	}

	if len(decoded.Content) != 100_000 {
		t.Errorf("content length: %d != 100000", len(decoded.Content))
	}
}
