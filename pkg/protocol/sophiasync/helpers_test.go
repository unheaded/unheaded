// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package sophiasync

import (
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

func TestComputeSignature(t *testing.T) {
	delta := &DictionaryDelta{
		Version: 1,
		Additions: []DictEntry{
			{Index: 0, Value: "hello"},
		},
		Removals: []uint32{5},
	}

	sig := computeSignature(delta)
	if len(sig) != 32 { // SHA-256 = 32 bytes
		t.Errorf("signature length = %d, want 32", len(sig))
	}

	// Same delta should produce same signature
	sig2 := computeSignature(delta)
	if !bytesEqual(sig, sig2) {
		t.Error("same delta should produce same signature")
	}

	// Different delta should produce different signature
	delta2 := &DictionaryDelta{
		Version: 2,
		Additions: []DictEntry{
			{Index: 0, Value: "different"},
		},
	}
	sig3 := computeSignature(delta2)
	if bytesEqual(sig, sig3) {
		t.Error("different deltas should produce different signatures")
	}
}

func TestBytesEqual(t *testing.T) {
	a := []byte("hello")
	b := []byte("hello")
	c := []byte("world")
	d := []byte("helloX")

	if !bytesEqual(a, b) {
		t.Error("identical bytes should be equal")
	}
	if bytesEqual(a, c) {
		t.Error("different bytes should not be equal")
	}
	if bytesEqual(a, d) {
		t.Error("different length bytes should not be equal")
	}
	if !bytesEqual(nil, nil) {
		t.Error("nil bytes should be equal")
	}
	if !bytesEqual([]byte{}, []byte{}) {
		t.Error("empty bytes should be equal")
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

	err = es.DistributeDelta(nil, nil)
	if err == nil {
		t.Error("expected error for nil delta")
	}
}

func makeTestLogger() *logger.Logger {
	return logger.New(io.Discard)
}
