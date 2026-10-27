// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package sophiasync

import (
	"context"
	"encoding/binary"
	"io"
	"testing"

	"unheaded/pkg/logger"
)

func TestEncodeDelta(t *testing.T) {
	delta := &DictionaryDelta{
		Version: 1,
		Additions: []DictEntry{
			{Index: 0, Value: "hello"},
			{Index: 1, Value: "world"},
		},
		Removals:  []uint32{5, 10},
		Signature: []byte("sig123"),
	}

	data, err := encodeDelta(delta)
	if err != nil {
		t.Fatalf("encodeDelta: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty encoded data")
	}

	// Verify version encoding (first 8 bytes)
	version := binary.BigEndian.Uint64(data[:8])
	if version != 1 {
		t.Errorf("encoded version = %d, want 1", version)
	}

	// Verify additions count (next 4 bytes)
	addCount := binary.BigEndian.Uint32(data[8:12])
	if addCount != 2 {
		t.Errorf("additions count = %d, want 2", addCount)
	}
}

func TestEncodeDelta_Nil(t *testing.T) {
	_, err := encodeDelta(nil)
	if err == nil {
		t.Error("expected error for nil delta")
	}
}

func TestEncodeDelta_Empty(t *testing.T) {
	delta := &DictionaryDelta{
		Version: 42,
	}

	data, err := encodeDelta(delta)
	if err != nil {
		t.Fatalf("encodeDelta: %v", err)
	}

	// Should encode: version(8) + addCount(4) + remCount(4) + sigLen(4)
	if len(data) < 20 {
		t.Errorf("expected at least 20 bytes, got %d", len(data))
	}
}

func TestDigest(t *testing.T) {
	delta := &DictionaryDelta{
		Version: 1,
		Additions: []DictEntry{
			{Index: 0, Value: "hello"},
		},
		Removals: []uint32{5},
	}

	d1 := delta.Digest()
	if d1 != delta.Digest() {
		t.Error("same delta should produce the same digest")
	}

	delta2 := &DictionaryDelta{
		Version: 2,
		Additions: []DictEntry{
			{Index: 0, Value: "different"},
		},
	}
	if d1 == delta2.Digest() {
		t.Error("different deltas should produce different digests")
	}

	// The old digest concatenated values without their lengths, so these
	// two collided; the canonical form is length-prefixed.
	a := &DictionaryDelta{Version: 1, Additions: []DictEntry{{Index: 0, Value: "ab"}, {Index: 1, Value: "c"}}}
	b := &DictionaryDelta{Version: 1, Additions: []DictEntry{{Index: 0, Value: "a"}, {Index: 1, Value: "bc"}}}
	if a.Digest() == b.Digest() {
		t.Error("value boundaries are not part of the digest")
	}
}

func TestDistributeDelta_NilDelta(t *testing.T) {
	// Test the nil check without needing a real wotan client
	// EncoderStream checks for nil delta before publishing
	log := makeTestLogger()
	client := NewMockWotanClient()

	es, err := NewEncoderStream(client, "control", log)
	if err != nil {
		t.Fatalf("NewEncoderStream: %v", err)
	}

	err = es.DistributeDelta(context.Background(), nil)
	if err == nil {
		t.Error("expected error for nil delta")
	}
}

func makeTestLogger() *logger.Logger {
	return logger.New(io.Discard)
}
