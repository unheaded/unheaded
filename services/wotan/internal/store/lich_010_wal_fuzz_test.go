// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// LICH-010 (real target): the WAL encoding in this package.
//
// The original tomb/lich/harnesses/lich_010_wal_integrity_test.go
// reimplements a WAL inside the test file and fuzzes that, importing no
// Unheaded package — so it cannot find a defect in the shipping store.
//
// It lives HERE rather than in tomb/lich/harnesses because Go's internal
// package rule forbids that directory from importing
// services/wotan/internal/store at all. That constraint is very likely why
// so many harnesses under tomb/ reimplement their targets: anything inside
// an internal/ tree is unreachable from there. A harness aimed at an
// internal package has to live beside it. See ADR-062.
//
// The real integrity surface is DecodeMessage: WALStore.recover() replays
// every record off disk through it and deliberately skips anything that
// fails to decode. A panic there is not a skipped record — it takes down
// recovery, and therefore startup, for a store whose whole job is surviving
// a crash. Corrupt bytes on disk are exactly the LICH-010 threat.
//
// Invariants:
//   - DecodeMessage never panics on arbitrary bytes
//   - DecodeMessage either errors or returns a non-nil message, never both nil
//   - Encode -> Decode round-trips the fields that matter
//   - A decoded record re-encodes to the same bytes (no smuggled state)
package store

import (
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// FuzzRealWALDecodeMessage feeds arbitrary bytes to the decoder that WAL
// recovery runs over every record on disk.
func FuzzRealWALDecodeMessage(f *testing.F) {
	// Valid record, so the fuzzer starts from a parseable shape.
	if enc, err := EncodeMessage(&Message{
		ID:        uuid.New(),
		Content:   "seed",
		Timestamp: time.Unix(0, 0),
	}); err == nil {
		f.Add(enc)
	}
	f.Add([]byte{})
	f.Add([]byte{1})
	f.Add([]byte{1, '{'})
	f.Add([]byte{1, '{', '}'})
	f.Add([]byte{2, '{', '}'})           // wrong version
	f.Add([]byte{1, 0xff, 0xfe})         // invalid utf8 body
	f.Add([]byte{1, '{', '"', 'I', 'D'}) // truncated json

	f.Fuzz(func(t *testing.T, data []byte) {
		msg, err := DecodeMessage(data)

		if err == nil && msg == nil {
			t.Fatal("DecodeMessage returned (nil, nil): recovery would index a nil message")
		}
		if err != nil && msg != nil {
			t.Fatalf("DecodeMessage returned both a message and an error: %v", err)
		}
	})
}

// FuzzRealWALRoundTrip checks that a message survives encode/decode, and that
// re-encoding a decoded record is byte-identical. A record that changes shape
// on the way through would make replay non-deterministic.
func FuzzRealWALRoundTrip(f *testing.F) {
	f.Add("hello", "room", int64(0))
	f.Add("", "", int64(-1))
	f.Add("\xff\xfe invalid", "r", int64(1<<62))
	f.Add("multi\nline\ttext", "room.sub", int64(1))

	f.Fuzz(func(t *testing.T, content, roomID string, nanos int64) {
		original := &Message{
			ID:        uuid.New(),
			CreatorID: uuid.New(),
			Content:   content,
			RoomID:    roomID,
			Timestamp: time.Unix(0, nanos).UTC(),
		}

		// Non-UTF-8 content does not survive this format — json.Marshal
		// replaces invalid bytes with U+FFFD. That is a real defect, covered
		// separately by TestEncodeMessage_NonUTF8ContentIsLossy_KnownLimitation
		// and written up in the promotion log; the round-trip property is
		// asserted over the inputs the format can actually represent.
		if !utf8.ValidString(content) || !utf8.ValidString(roomID) {
			return
		}

		encoded, err := EncodeMessage(original)
		if err != nil {
			// Encoding may legitimately refuse some inputs; it must not panic.
			return
		}

		decoded, err := DecodeMessage(encoded)
		if err != nil {
			t.Fatalf("a record this package encoded failed to decode: %v", err)
		}
		if decoded.ID != original.ID {
			t.Fatalf("ID changed: %v -> %v", original.ID, decoded.ID)
		}
		if decoded.Content != original.Content {
			t.Fatalf("Content changed: %q -> %q", original.Content, decoded.Content)
		}
		if decoded.RoomID != original.RoomID {
			t.Fatalf("RoomID changed: %q -> %q", original.RoomID, decoded.RoomID)
		}
		if decoded.CreatorID != original.CreatorID {
			t.Fatalf("CreatorID changed: %v -> %v", original.CreatorID, decoded.CreatorID)
		}

		// Re-encoding must be stable: replay reads these bytes back.
		reencoded, err := EncodeMessage(decoded)
		if err != nil {
			t.Fatalf("re-encode of a decoded record failed: %v", err)
		}
		if string(reencoded) != string(encoded) {
			t.Fatalf("re-encode not byte-identical\n first: %q\nsecond: %q", encoded, reencoded)
		}
	})
}

// FuzzRealWALEncodeNilSafety pins the documented nil contract.
func FuzzRealWALEncodeNilSafety(f *testing.F) {
	f.Add(true)
	f.Add(false)

	f.Fuzz(func(t *testing.T, useNil bool) {
		var msg *Message
		if !useNil {
			msg = &Message{ID: uuid.New()}
		}

		data, err := EncodeMessage(msg)
		if useNil {
			if err == nil {
				t.Fatal("EncodeMessage(nil) must return an error")
			}
			if data != nil {
				t.Fatal("EncodeMessage(nil) must not return bytes")
			}
			return
		}
		if err != nil {
			t.Fatalf("EncodeMessage of a valid message failed: %v", err)
		}
		if len(data) < 2 {
			t.Fatalf("encoded record is %d bytes; DecodeMessage rejects anything under 2", len(data))
		}
	})
}

// The WAL format cannot represent non-UTF-8 message content.
//
// EncodeMessage marshals Message to JSON, and encoding/json replaces every
// invalid UTF-8 byte in a string with U+FFFD. Wotan's gRPC publish path does
// `SendMessage(senderID, string(req.Payload))`, so an arbitrary binary
// payload becomes Content — and is silently corrupted the moment it is
// persisted. Round-trip through the WAL is therefore lossy for binary
// payloads, and the loss is not detectable after the fact.
//
// Not currently reachable in the default configuration: configs/wotan.yaml
// ships `store.type: memory`, and the WAL is used by the wal/hybrid stores
// the same file recommends "for production persistence". This test pins the
// present behaviour so the limitation is visible and any change to it is
// deliberate. Fixing it means a new encodingVersion — see the promotion log.
func TestEncodeMessage_NonUTF8ContentIsLossy_KnownLimitation(t *testing.T) {
	original := &Message{ID: uuid.New(), Content: "\xff\xfe binary"}

	encoded, err := EncodeMessage(original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := DecodeMessage(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if decoded.Content == original.Content {
		t.Fatal("non-UTF-8 content now round-trips — the format was fixed; " +
			"delete this test and drop the utf8 guard in FuzzRealWALRoundTrip")
	}
	if utf8.ValidString(decoded.Content) != true {
		t.Errorf("decoded content is still invalid UTF-8: %q", decoded.Content)
	}
}
