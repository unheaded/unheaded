// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package main

import (
	"encoding/binary"
	"sync"
)

// Doom internal keycodes (from i_input.c doomkeys.h)
const (
	KeyFireCode   = 0xA3 // KEY_FIRE
	KeyUseCode    = 0xA2 // KEY_USE
	KeyRshiftCode = 0xB6 // KEY_RSHIFT (run)
	KeyStrafeCode = 0xA4 // KEY_STRAFE

	KeyUparrow    = 0xAD // KEY_UPARROW
	KeyDownarrow  = 0xAF // KEY_DOWNARROW
	KeyLeftarrow  = 0xAC // KEY_LEFTARROW
	KeyRightarrow = 0xAE // KEY_RIGHTARROW
)

// KBD_MAP event queue: 8 slots of u32, each encoding one key event.
// Format per slot: (key_code << 1) | pressed_flag
// BPF scans slots 0-7, consumes first non-zero, clears it.
const kbdSlots = 8

// KeyStateBitmap tracks keyboard events and flushes them to BPF KBD_MAP.
// It now uses the event queue protocol matching the BPF SYS_GET_KEY handler:
// each key press/release is written as a u32 to KBD_MAP[slot].
type KeyStateBitmap struct {
	mu       sync.Mutex
	events   []keyEvent // Pending events
	nextSlot uint32     // Next slot to write (0-7 circular)
	bitmap   [32]byte   // 256-bit key state (for State() query)
	sequence uint64
	lastSent [32]byte
}

type keyEvent struct {
	code    uint8
	pressed bool
}

// NewKeyStateBitmap creates and returns a new, empty key state bitmap.
func NewKeyStateBitmap() *KeyStateBitmap {
	return &KeyStateBitmap{}
}

// SetKey records a key press/release event.
func (ks *KeyStateBitmap) SetKey(code uint8, pressed bool) {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	// Update bitmap for state queries
	byteIdx := code / 8
	bitIdx := code % 8
	if pressed {
		ks.bitmap[byteIdx] |= (1 << bitIdx)
	} else {
		ks.bitmap[byteIdx] &= ^(1 << bitIdx)
	}

	// Queue the event for BPF flush
	ks.events = append(ks.events, keyEvent{code: code, pressed: pressed})
}

// Flush writes pending key events to BPF KBD_MAP slots.
// Each event is written as a u32: (key_code << 1) | pressed_flag.
// Uses circular slot allocation (0-7).
func (ks *KeyStateBitmap) Flush(kbdMap *BPFMap) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	if len(ks.events) == 0 {
		return nil
	}

	for _, ev := range ks.events {
		// Encode: (key_code << 1) | pressed_flag
		var pressed uint32
		if ev.pressed {
			pressed = 1
		}
		val := (uint32(ev.code) << 1) | pressed

		// Write to KBD_MAP[nextSlot]
		key := make([]byte, 4)
		binary.LittleEndian.PutUint32(key, ks.nextSlot)

		value := make([]byte, 4)
		binary.LittleEndian.PutUint32(value, val)

		if err := kbdMap.UpdateElem(key, value); err != nil {
			return err
		}

		ks.nextSlot = (ks.nextSlot + 1) % kbdSlots
	}

	ks.events = ks.events[:0] // Clear pending events
	copy(ks.lastSent[:], ks.bitmap[:])
	ks.sequence++

	return nil
}

// IsDirty returns true if there are pending events.
func (ks *KeyStateBitmap) IsDirty() bool {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	return len(ks.events) > 0
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
