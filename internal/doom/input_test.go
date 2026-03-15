// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package doom

import (
	"encoding/binary"
	"strings"
	"testing"
)

// TestInjectKey tests injecting a single key event.
func TestInjectKey(t *testing.T) {
	ResetKeySequence()
	m := newMockMap()

	// Inject a key press
	if err := InjectKey(m, 42, true); err != nil {
		t.Fatalf("InjectKey: %v", err)
	}

	// Read back and verify
	ks, err := ReadKeyState(m)
	if err != nil {
		t.Fatalf("ReadKeyState: %v", err)
	}

	if ks.Key != 42 {
		t.Errorf("Key = %d, want 42", ks.Key)
	}
	if ks.Pressed != 1 {
		t.Errorf("Pressed = %d, want 1", ks.Pressed)
	}
	if ks.Sequence != 1 {
		t.Errorf("Sequence = %d, want 1", ks.Sequence)
	}
}

// TestInjectKeyRelease tests injecting a key release event.
func TestInjectKeyRelease(t *testing.T) {
	ResetKeySequence()
	m := newMockMap()

	if err := InjectKey(m, 10, false); err != nil {
		t.Fatalf("InjectKey: %v", err)
	}

	ks, err := ReadKeyState(m)
	if err != nil {
		t.Fatalf("ReadKeyState: %v", err)
	}

	if ks.Key != 10 {
		t.Errorf("Key = %d, want 10", ks.Key)
	}
	if ks.Pressed != 0 {
		t.Errorf("Pressed = %d, want 0", ks.Pressed)
	}
}

// TestInjectKeySequenceIncrement tests that sequence increments monotonically.
func TestInjectKeySequenceIncrement(t *testing.T) {
	ResetKeySequence()
	m := newMockMap()

	for i := 0; i < 5; i++ {
		if err := InjectKey(m, uint32(i), true); err != nil {
			t.Fatalf("InjectKey[%d]: %v", i, err)
		}
	}

	// Read back - should have the last key with sequence 5
	ks, err := ReadKeyState(m)
	if err != nil {
		t.Fatalf("ReadKeyState: %v", err)
	}

	if ks.Sequence != 5 {
		t.Errorf("Sequence = %d, want 5", ks.Sequence)
	}
	if ks.Key != 4 {
		t.Errorf("Key = %d, want 4 (last injected)", ks.Key)
	}
}

// TestInjectKeyError tests InjectKey with error map.
func TestInjectKeyError(t *testing.T) {
	ResetKeySequence()
	m := newErrorMap("bpf error")

	err := InjectKey(m, 42, true)
	if err == nil {
		t.Fatal("InjectKey should fail with error map")
	}
}

// TestReadKeyStateNotFound tests ReadKeyState with empty map.
func TestReadKeyStateNotFound(t *testing.T) {
	m := newMockMap()
	_, err := ReadKeyState(m)
	if err == nil {
		t.Fatal("ReadKeyState should fail on empty map")
	}
}

// TestReadKeyStateError tests ReadKeyState with error map.
func TestReadKeyStateError(t *testing.T) {
	m := newErrorMap("bpf error")
	_, err := ReadKeyState(m)
	if err == nil {
		t.Fatal("ReadKeyState should fail with error map")
	}
}

// TestParseKeyBitmap tests parsing hex key bitmaps.
func TestParseKeyBitmap(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantKeys  []uint32
		wantError bool
	}{
		{
			name:     "single key 0",
			input:    "0x01",
			wantKeys: []uint32{0},
		},
		{
			name:     "keys 0 and 1",
			input:    "0x03",
			wantKeys: []uint32{0, 1},
		},
		{
			name:     "key 7",
			input:    "80",
			wantKeys: []uint32{7},
		},
		{
			name:     "keys 0, 2, 4",
			input:    "0x15",
			wantKeys: []uint32{0, 2, 4},
		},
		{
			name:     "no keys",
			input:    "0x00",
			wantKeys: nil,
		},
		{
			name:     "prefix 0X uppercase",
			input:    "0XFF",
			wantKeys: []uint32{0, 1, 2, 3, 4, 5, 6, 7},
		},
		{
			name:      "invalid hex",
			input:     "0xZZZZ",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events, err := ParseKeyBitmap(tt.input)
			if tt.wantError {
				if err == nil {
					t.Fatal("ParseKeyBitmap should return error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseKeyBitmap: %v", err)
			}

			if len(events) != len(tt.wantKeys) {
				t.Fatalf("got %d events, want %d", len(events), len(tt.wantKeys))
			}

			for i, want := range tt.wantKeys {
				if events[i].Scancode != want {
					t.Errorf("events[%d].Scancode = %d, want %d", i, events[i].Scancode, want)
				}
				if !events[i].Pressed {
					t.Errorf("events[%d].Pressed should be true", i)
				}
			}
		})
	}
}

// TestKeyEventString tests the KeyEvent String() method.
func TestKeyEventString(t *testing.T) {
	ev := KeyEvent{Scancode: 42, Pressed: true}
	s := ev.String()
	if !strings.Contains(s, "42") {
		t.Errorf("String should contain scancode, got: %s", s)
	}
	if !strings.Contains(s, "pressed") {
		t.Errorf("String should contain 'pressed', got: %s", s)
	}

	ev2 := KeyEvent{Scancode: 10, Pressed: false}
	s2 := ev2.String()
	if !strings.Contains(s2, "released") {
		t.Errorf("String should contain 'released', got: %s", s2)
	}
}

// TestInjectKeyEvents tests injecting multiple key events.
func TestInjectKeyEvents(t *testing.T) {
	ResetKeySequence()
	m := newMockMap()

	events := []KeyEvent{
		{Scancode: 1, Pressed: true},
		{Scancode: 2, Pressed: true},
		{Scancode: 1, Pressed: false},
	}

	if err := InjectKeyEvents(m, events); err != nil {
		t.Fatalf("InjectKeyEvents: %v", err)
	}

	// Read back - should have the last event
	ks, err := ReadKeyState(m)
	if err != nil {
		t.Fatalf("ReadKeyState: %v", err)
	}

	if ks.Key != 1 {
		t.Errorf("Key = %d, want 1 (last event)", ks.Key)
	}
	if ks.Pressed != 0 {
		t.Errorf("Pressed = %d, want 0 (released)", ks.Pressed)
	}
	if ks.Sequence != 3 {
		t.Errorf("Sequence = %d, want 3", ks.Sequence)
	}
}

// TestInjectKeyEventsError tests InjectKeyEvents with error map.
func TestInjectKeyEventsError(t *testing.T) {
	ResetKeySequence()
	m := newErrorMap("bpf error")

	events := []KeyEvent{
		{Scancode: 1, Pressed: true},
	}

	err := InjectKeyEvents(m, events)
	if err == nil {
		t.Fatal("InjectKeyEvents should fail with error map")
	}
}

// TestInjectKeyEventsEmpty tests injecting an empty event list.
func TestInjectKeyEventsEmpty(t *testing.T) {
	m := newMockMap()
	if err := InjectKeyEvents(m, nil); err != nil {
		t.Fatalf("InjectKeyEvents(nil): %v", err)
	}
}

// TestResetKeySequence tests resetting the key sequence counter.
func TestResetKeySequence(t *testing.T) {
	// Inject some keys to increment the counter
	m := newMockMap()
	_ = InjectKey(m, 1, true)
	_ = InjectKey(m, 2, true)

	ResetKeySequence()

	// Next inject should have sequence 1
	if err := InjectKey(m, 3, true); err != nil {
		t.Fatalf("InjectKey: %v", err)
	}

	ks, err := ReadKeyState(m)
	if err != nil {
		t.Fatalf("ReadKeyState: %v", err)
	}

	if ks.Sequence != 1 {
		t.Errorf("Sequence = %d, want 1 after reset", ks.Sequence)
	}
}

// TestKeyboardStateWriteToMap tests writing keyboard state directly.
func TestKeyboardStateWriteToMap(t *testing.T) {
	m := newMockMap()

	ks := &KeyboardState{
		Key:      99,
		Pressed:  1,
		Sequence: 42,
	}

	key := make([]byte, 4)
	binary.LittleEndian.PutUint32(key, 0)
	if err := m.UpdateElem(key, ks.MarshalBinary()); err != nil {
		t.Fatalf("UpdateElem: %v", err)
	}

	got, err := ReadKeyState(m)
	if err != nil {
		t.Fatalf("ReadKeyState: %v", err)
	}

	if got.Key != 99 {
		t.Errorf("Key = %d, want 99", got.Key)
	}
	if got.Pressed != 1 {
		t.Errorf("Pressed = %d, want 1", got.Pressed)
	}
	if got.Sequence != 42 {
		t.Errorf("Sequence = %d, want 42", got.Sequence)
	}
}
