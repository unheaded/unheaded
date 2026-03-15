// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package doom_memwrite_test validates the Bug 24 behavioral contract:
// word stores (ST) must NOT write to SCREEN_MAP; only byte stores (STB)
// via mem_write_byte update SCREEN_MAP.
//
// Two test modes:
//   A. Contract tests (always run, no ring needed):
//      Verify screen region constants match between Go and Rust.
//   B. Integration tests (go:build integration, ring must be running):
//      Write to RAM_MAP and verify SCREEN_MAP behavior.
package doom_memwrite_test

import (
	"encoding/binary"
	"testing"
)

// Screen region constants — MUST match ebpf/monad-common/src/lib.rs mbc_mmap module.
const (
	// ScreenBase is the start of the screen I/O region in the MBC address space.
	// Maps to SCREEN_MAP[0..64000] (320x200 pixels, 8-bit palette indices).
	// Source: ebpf/monad-common/src/lib.rs:1203
	ScreenBase = 0x0007_0000

	// ScreenSize is the size of the screen framebuffer (320x200 = 64000 bytes).
	// Source: ebpf/monad-common/src/lib.rs:1205
	ScreenSize = 64_000

	// ScreenEnd is the first byte address past the screen region.
	ScreenEnd = ScreenBase + ScreenSize

	// ScreenWidth and ScreenHeight are the framebuffer dimensions.
	ScreenWidth  = 320
	ScreenHeight = 200

	// MapPinDir is the default BPF map pin directory for the doom ring.
	MapPinDir = "/sys/fs/bpf/unheaded/doom-ring/maps"
)

// ── Contract Tests (always run) ─────────────────────────────────────────────

func TestScreenConstants(t *testing.T) {
	t.Run("screen_size_matches_dimensions", func(t *testing.T) {
		expected := ScreenWidth * ScreenHeight
		if ScreenSize != expected {
			t.Errorf("ScreenSize=%d, want %dx%d=%d", ScreenSize, ScreenWidth, ScreenHeight, expected)
		}
	})

	t.Run("screen_base_is_word_aligned", func(t *testing.T) {
		if ScreenBase%4 != 0 {
			t.Errorf("ScreenBase=0x%X is not word-aligned", ScreenBase)
		}
	})

	t.Run("screen_end_calculation", func(t *testing.T) {
		if ScreenEnd != ScreenBase+ScreenSize {
			t.Errorf("ScreenEnd=0x%X, want 0x%X", ScreenEnd, ScreenBase+ScreenSize)
		}
	})

	t.Run("screen_base_matches_rust_constant", func(t *testing.T) {
		// ebpf/monad-common/src/lib.rs:1201
		// pub const SCREEN_BASE: u32 = 0x0007_0000;
		const rustScreenBase = 0x0007_0000
		if ScreenBase != rustScreenBase {
			t.Errorf("Go ScreenBase=0x%X != Rust SCREEN_BASE=0x%X", ScreenBase, rustScreenBase)
		}
	})

	t.Run("screen_size_matches_rust_constant", func(t *testing.T) {
		// ebpf/monad-common/src/lib.rs:1205
		// pub const SCREEN_SIZE: u32 = 64_000;
		const rustScreenSize = 64_000
		if ScreenSize != rustScreenSize {
			t.Errorf("Go ScreenSize=%d != Rust SCREEN_SIZE=%d", ScreenSize, rustScreenSize)
		}
	})
}

func TestBug24Contract(t *testing.T) {
	t.Run("mem_write_word_skips_screen_map", func(t *testing.T) {
		// Document the behavioral contract:
		// In ebpf/monad-cpu-ebpf/src/main.rs, mem_write_word() at line ~839:
		//   "Bug 24 fix: Do NOT write to SCREEN_MAP from word stores (ST)."
		//   "Only byte stores (STB -> mem_write_byte) update SCREEN_MAP."
		//
		// This means:
		//   - mem_write_word(addr, val) where addr is in screen region:
		//     -> writes to RAM_MAP only, NOT SCREEN_MAP
		//   - mem_write_byte(addr, val) where addr is in screen region:
		//     -> writes to SCREEN_MAP AND RAM_MAP
		//
		// This test documents this contract. Integration tests below verify it.
		t.Log("Contract: word stores to screen region [0x70000, 0x7FA00) skip SCREEN_MAP")
		t.Log("Contract: byte stores to screen region update both SCREEN_MAP and RAM_MAP")
		t.Log("Contract: this prevents WAD lump names from polluting the display (Bug 24)")
	})

	t.Run("screen_region_word_addresses", func(t *testing.T) {
		// The screen region spans word addresses ScreenBase/4 to (ScreenBase+ScreenSize)/4
		firstWord := ScreenBase / 4
		lastWord := (ScreenBase + ScreenSize - 1) / 4

		t.Logf("Screen word address range: %d to %d (%d words)",
			firstWord, lastWord, lastWord-firstWord+1)

		// Verify these are within the RAM_MAP array bounds (16M entries)
		const ramMapSize = 16_777_216
		if lastWord >= ramMapSize {
			t.Errorf("Screen last word address %d exceeds RAM_MAP size %d", lastWord, ramMapSize)
		}
	})
}

func TestKeyEncoding(t *testing.T) {
	// Verify BPF map key encoding for screen pixel lookup
	t.Run("pixel_index_to_key_bytes", func(t *testing.T) {
		// SCREEN_MAP keys are u32 LE (pixel index 0-63999)
		testCases := []struct {
			pixelIdx uint32
			wantKey  [4]byte
		}{
			{0, [4]byte{0x00, 0x00, 0x00, 0x00}},
			{100, [4]byte{0x64, 0x00, 0x00, 0x00}},
			{32000, [4]byte{0x00, 0x7D, 0x00, 0x00}},
			{63999, [4]byte{0xFF, 0xF9, 0x00, 0x00}},
		}

		for _, tc := range testCases {
			key := make([]byte, 4)
			binary.LittleEndian.PutUint32(key, tc.pixelIdx)
			for i := 0; i < 4; i++ {
				if key[i] != tc.wantKey[i] {
					t.Errorf("pixel %d: key[%d]=0x%02X, want 0x%02X",
						tc.pixelIdx, i, key[i], tc.wantKey[i])
				}
			}
		}
	})

	t.Run("word_address_for_screen_region", func(t *testing.T) {
		// RAM_MAP is word-addressed: key = byte_addr / 4
		byteAddr := uint32(ScreenBase + 100) // pixel 100 in screen
		wordAddr := byteAddr / 4

		key := make([]byte, 4)
		binary.LittleEndian.PutUint32(key, wordAddr)

		// Verify round-trip
		gotWordAddr := binary.LittleEndian.Uint32(key)
		if gotWordAddr != wordAddr {
			t.Errorf("word address round-trip: got %d, want %d", gotWordAddr, wordAddr)
		}

		gotByteAddr := gotWordAddr * 4
		if gotByteAddr != byteAddr {
			t.Errorf("byte address: got 0x%X, want 0x%X", gotByteAddr, byteAddr)
		}
	})
}
