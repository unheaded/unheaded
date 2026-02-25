// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package main

import (
	"encoding/binary"
	"sync"
)

// Doom internal keycodes (from i_input.c doomkeys.h)
const (
	KeyFireCode   = 0xA3  // KEY_FIRE
	KeyUseCode    = 0xA2  // KEY_USE
	KeyRshiftCode = 0xB6  // KEY_RSHIFT (run)
	KeyStrafeCode = 0xA4  // KEY_STRAFE

	KeyUparrow   = 0xAD  // KEY_UPARROW
	KeyDownarrow = 0xAF  // KEY_DOWNARROW
	KeyLeftarrow = 0xAC  // KEY_LEFTARROW
	KeyRightarrow = 0xAE // KEY_RIGHTARROW
)

// KeyStateBitmap tracks all 256 possible Doom key states simultaneously.
// Each bit represents one Doom internal keycode (0-255). A bit set to 1 means
// the key is pressed; set to 0 means released.
//
// The bitmap is flushed to BPF KBD_MAP[0] as a 40-byte value:
//   - Bytes 0-31: 256-bit state bitmap (32 bytes)
//   - Bytes 32-39: 64-bit sequence counter (little-endian)
//
// The sequence counter helps the kernel detect dirty state changes.
// Each call to Flush() increments this counter if the bitmap changed.
type KeyStateBitmap struct {
	mu       sync.Mutex
	bitmap   [32]byte    // 256-bit key state (32 bytes)
	sequence uint64      // Incremented on changes
	lastSent [32]byte    // Last flushed bitmap
}

// NewKeyStateBitmap creates and returns a new, empty key state bitmap.
func NewKeyStateBitmap() *KeyStateBitmap {
	return &KeyStateBitmap{
		sequence: 0,
	}
}

// SetKey updates the state of a single key (Doom internal keycode 0-255).
// If pressed is true, the key bit is set; if false, it is cleared.
// This does NOT update KBD_MAP immediately; call Flush() to do that.
func (ks *KeyStateBitmap) SetKey(code uint8, pressed bool) {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	byteIdx := code / 8
	bitIdx := code % 8

	if pressed {
		// Set the bit
		ks.bitmap[byteIdx] |= (1 << bitIdx)
	} else {
		// Clear the bit
		ks.bitmap[byteIdx] &= ^(1 << bitIdx)
	}
}

// Flush writes the current key state bitmap to BPF KBD_MAP[0].
// The write includes the 32-byte state bitmap and an 8-byte sequence counter.
// Returns an error if the BPF map write fails.
// If IsDirty() is false, this may be optimized to skip the write.
func (ks *KeyStateBitmap) Flush(kbdMap *BPFMap) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	// Check if bitmap changed since last flush
	changed := false
	for i := 0; i < 32; i++ {
		if ks.bitmap[i] != ks.lastSent[i] {
			changed = true
			break
		}
	}

	if !changed {
		return nil // No change, skip the write
	}

	// Build 40-byte KBD_MAP value: [32 bytes state] + [8 bytes sequence LE]
	value := make([]byte, 40)
	copy(value[0:32], ks.bitmap[:])
	binary.LittleEndian.PutUint64(value[32:40], ks.sequence)

	// Write to BPF map with key [0] (single-slot keyboard map)
	key := []byte{0}
	if err := kbdMap.UpdateElem(key, value); err != nil {
		return err
	}

	// Remember what we just flushed
	copy(ks.lastSent[:], ks.bitmap[:])
	ks.sequence++

	return nil
}

// IsDirty returns true if the bitmap has changed since the last Flush.
func (ks *KeyStateBitmap) IsDirty() bool {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	for i := 0; i < 32; i++ {
		if ks.bitmap[i] != ks.lastSent[i] {
			return true
		}
	}
	return false
}

// State returns a copy of the current 32-byte bitmap (for testing/debugging).
func (ks *KeyStateBitmap) State() [32]byte {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	return ks.bitmap
}

// IsKeyPressed returns true if the given Doom keycode is currently pressed.
func (ks *KeyStateBitmap) IsKeyPressed(code uint8) bool {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	byteIdx := code / 8
	bitIdx := code % 8
	return (ks.bitmap[byteIdx] & (1 << bitIdx)) != 0
}
