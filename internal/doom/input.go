// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package doom

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"sync/atomic"
)

// keySequence is a global monotonically increasing counter for keyboard events.
// Each new key injection gets a unique sequence number so the BPF program can
// detect changes.
var keySequence uint64

// InjectKey writes a keyboard event to the kbd_map via a MapAccessor.
// scancode is the key scancode, pressed indicates key-down (true) or key-up (false).
// sequence is automatically incremented on each call.
func InjectKey(m MapAccessor, scancode uint32, pressed bool) error {
	seq := atomic.AddUint64(&keySequence, 1)

	var pressedVal uint32
	if pressed {
		pressedVal = 1
	}

	ks := &KeyboardState{
		Key:      scancode,
		Pressed:  pressedVal,
		Sequence: seq,
	}

	key := make([]byte, 4)
	binary.LittleEndian.PutUint32(key, 0) // kbd_map index 0

	return m.UpdateElem(key, ks.MarshalBinary())
}

// ReadKeyState reads the current keyboard state from kbd_map via a MapAccessor.
func ReadKeyState(m MapAccessor) (*KeyboardState, error) {
	key := make([]byte, 4)
	binary.LittleEndian.PutUint32(key, 0) // kbd_map index 0

	data, err := m.LookupElem(key, KeyboardStateSize)
	if err != nil {
		return nil, fmt.Errorf("kbd_map lookup: %w", err)
	}

	return UnmarshalKeyboardState(data)
}

// ParseKeyBitmap parses a hex string representing a key bitmap into
// individual key events. The bitmap is a bitmask where each bit position
// corresponds to a scancode. Bits that are set indicate pressed keys.
//
// For example, bitmap "0x03" means scancodes 0 and 1 are pressed.
func ParseKeyBitmap(hexStr string) ([]KeyEvent, error) {
	// Strip "0x" or "0X" prefix if present
	clean := hexStr
	if len(clean) >= 2 && (clean[:2] == "0x" || clean[:2] == "0X") {
		clean = clean[2:]
	}

	val, err := strconv.ParseUint(clean, 16, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid hex bitmap %q: %w", hexStr, err)
	}

	var events []KeyEvent
	for bit := 0; bit < 64; bit++ {
		if val&(1<<uint(bit)) != 0 {
			events = append(events, KeyEvent{
				Scancode: uint32(bit),
				Pressed:  true,
			})
		}
	}

	return events, nil
}

// KeyEvent represents a single keyboard event (scancode + pressed state).
type KeyEvent struct {
	Scancode uint32
	Pressed  bool
}

// String returns a human-readable representation of the key event.
func (e KeyEvent) String() string {
	action := "released"
	if e.Pressed {
		action = "pressed"
	}
	return fmt.Sprintf("key=%d %s", e.Scancode, action)
}

// InjectKeyEvents writes multiple key events to the kbd_map via a MapAccessor.
// Events are written sequentially; each gets a unique sequence number.
func InjectKeyEvents(m MapAccessor, events []KeyEvent) error {
	for _, ev := range events {
		if err := InjectKey(m, ev.Scancode, ev.Pressed); err != nil {
			return fmt.Errorf("inject key %d: %w", ev.Scancode, err)
		}
	}
	return nil
}

// ResetKeySequence resets the global key sequence counter to zero.
// This is primarily useful for testing.
func ResetKeySequence() {
	atomic.StoreUint64(&keySequence, 0)
}
