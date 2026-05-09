// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"encoding/binary"
	"testing"
)

func TestUint32Key_LittleEndianEncoding(t *testing.T) {
	t.Parallel()
	cases := []struct {
		v    uint32
		want []byte
	}{
		{0, []byte{0x00, 0x00, 0x00, 0x00}},
		{1, []byte{0x01, 0x00, 0x00, 0x00}},
		{0xCAFEBABE, []byte{0xBE, 0xBA, 0xFE, 0xCA}},
		{0xFFFFFFFF, []byte{0xFF, 0xFF, 0xFF, 0xFF}},
	}
	for _, c := range cases {
		got := uint32Key(c.v)
		if len(got) != 4 {
			t.Errorf("uint32Key(0x%X): len = %d, want 4", c.v, len(got))
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("uint32Key(0x%X) = %v, want %v", c.v, got, c.want)
				break
			}
		}
	}
}

func TestUint32Key_RoundTripsBPF(t *testing.T) {
	t.Parallel()
	for _, v := range []uint32{0, 1, 0xDE, 0xCAFEBABE, 0xFFFFFFFF} {
		key := uint32Key(v)
		got := binary.LittleEndian.Uint32(key)
		if got != v {
			t.Errorf("round-trip 0x%X → 0x%X", v, got)
		}
	}
}

func TestUint32Key_IndependentBuffers(t *testing.T) {
	t.Parallel()
	a := uint32Key(0xDE)
	b := uint32Key(0xDE)
	a[0] = 0x99
	if b[0] == 0x99 {
		t.Errorf("uint32Key returns shared buffers — caller mutation leaked")
	}
}
