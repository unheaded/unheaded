// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"encoding/binary"
	"testing"
)

func TestInstanceKey_LittleEndianEncoding(t *testing.T) {
	t.Parallel()
	cases := []struct {
		id   uint32
		want []byte
	}{
		{0x000000DE, []byte{0xDE, 0x00, 0x00, 0x00}},
		{0xCAFEBABE, []byte{0xBE, 0xBA, 0xFE, 0xCA}},
		{0, []byte{0x00, 0x00, 0x00, 0x00}},
		{0xFFFFFFFF, []byte{0xFF, 0xFF, 0xFF, 0xFF}},
	}
	for _, c := range cases {
		got := instanceKey(c.id)
		if len(got) != 4 {
			t.Errorf("instanceKey(0x%X): got len %d, want 4", c.id, len(got))
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("instanceKey(0x%X): got %v, want %v", c.id, got, c.want)
				break
			}
		}
	}
}

func TestInstanceKey_RoundTripsBPF(t *testing.T) {
	t.Parallel()
	// Verify the encoding is the standard BPF map-key shape (4-byte LE u32).
	// The kernel side reads the same way; this guarantees host/kernel agreement.
	for _, id := range []uint32{0xDE, 0xC001, 0xCAFEBABE, 0xFFFFFFFF} {
		key := instanceKey(id)
		got := binary.LittleEndian.Uint32(key)
		if got != id {
			t.Errorf("round-trip 0x%X: encoded → 0x%X via LE decode", id, got)
		}
	}
}

func TestInstanceKey_IndependentBuffers(t *testing.T) {
	t.Parallel()
	// Each call should return a fresh slice — mutating one shouldn't affect
	// another (catches accidental shared-buffer regression).
	a := instanceKey(0xDE)
	b := instanceKey(0xDE)
	a[0] = 0x99
	if b[0] == 0x99 {
		t.Errorf("instanceKey returns shared buffers — caller mutation leaked")
	}
}
