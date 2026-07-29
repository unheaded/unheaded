// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package dns

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// TestCompressionPointerBitOverlap demonstrates why offsets >= 0x4000 must
// never be stored for compression.
//
// A DNS name-compression pointer is 0xC000 | offset: two marker bits followed
// by 14 bits of offset (RFC 1035 s4.1.4). The OR does not fail on an oversized
// offset — it quietly merges into the marker bits. 0x4000 ORs to 0xC000, which
// is a pointer to byte 0 of the message rather than byte 16384.
func TestCompressionPointerBitOverlap(t *testing.T) {
	cases := []struct {
		offset     uint16
		wantBroken bool
	}{
		{0x0000, false},
		{0x3FFF, false}, // largest representable offset
		{0x4000, true},  // first offset that collides with the marker bits
		{0xC000, true},
	}
	for _, c := range cases {
		ptr := 0xC000 | c.offset
		recovered := ptr &^ 0xC000
		broken := recovered != c.offset
		if broken != c.wantBroken {
			t.Errorf("offset 0x%04X: ptr=0x%04X recovers 0x%04X (broken=%v, want %v)",
				c.offset, ptr, recovered, broken, c.wantBroken)
		}
		if c.offset < MaxCompressionOffset && broken {
			t.Errorf("offset 0x%04X is below MaxCompressionOffset but still corrupts", c.offset)
		}
	}
}

// TestPackName_NoPointerBeyond14Bits packs enough names to push the buffer past
// 0x4000 and asserts every compression pointer emitted still resolves to the
// offset it claims. Before the guard, names recorded past 16 KiB produced
// pointers aimed at the wrong part of the message.
func TestPackName_NoPointerBeyond14Bits(t *testing.T) {
	buf := &bytes.Buffer{}
	offsets := map[string]uint16{}

	// Pad past the 14-bit boundary.
	buf.Write(make([]byte, MaxCompressionOffset+512))

	if err := packName(buf, "example.com.", offsets); err != nil {
		t.Fatalf("packName: %v", err)
	}
	for name, off := range offsets {
		if off >= MaxCompressionOffset {
			t.Errorf("stored unrepresentable offset 0x%04X for %q", off, name)
		}
	}

	// A second pack of the same name must not emit a corrupt pointer.
	before := buf.Len()
	if err := packName(buf, "example.com.", offsets); err != nil {
		t.Fatalf("packName (second): %v", err)
	}
	emitted := buf.Bytes()[before:]
	if len(emitted) == 2 {
		ptr := binary.BigEndian.Uint16(emitted)
		if ptr&0xC000 == 0xC000 {
			off := ptr &^ 0xC000
			if off >= MaxCompressionOffset {
				t.Errorf("emitted pointer to unrepresentable offset 0x%04X", off)
			}
		}
	}
}
