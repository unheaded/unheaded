// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package doom

import (
	"testing"
	"unsafe"
)

// TestCpuStateSize verifies the Go CpuState struct is exactly 104 bytes,
// matching the Rust MbcCpuState #[repr(C)] layout.
func TestCpuStateSize(t *testing.T) {
	size := unsafe.Sizeof(CpuState{})
	if size != CpuStateSize {
		t.Fatalf("CpuState size = %d, want %d (must match Rust MbcCpuState)", size, CpuStateSize)
	}
}

// TestCpuStateSizeConst verifies the CpuStateSize constant is 104.
func TestCpuStateSizeConst(t *testing.T) {
	if CpuStateSize != 104 {
		t.Fatalf("CpuStateSize = %d, want 104", CpuStateSize)
	}
}

// TestCpuStateFieldOffsets verifies field offsets match the Rust #[repr(C)] layout.
func TestCpuStateFieldOffsets(t *testing.T) {
	var cpu CpuState
	base := uintptr(unsafe.Pointer(&cpu))

	tests := []struct {
		name   string
		offset uintptr
		want   uintptr
	}{
		{"Regs", uintptr(unsafe.Pointer(&cpu.Regs)) - base, 0},
		{"PC", uintptr(unsafe.Pointer(&cpu.PC)) - base, 64},
		{"Flags", uintptr(unsafe.Pointer(&cpu.Flags)) - base, 68},
		{"Halted", uintptr(unsafe.Pointer(&cpu.Halted)) - base, 69},
		{"Stalled", uintptr(unsafe.Pointer(&cpu.Stalled)) - base, 70},
		{"Pad", uintptr(unsafe.Pointer(&cpu.Pad)) - base, 71},
		{"SleepUntil", uintptr(unsafe.Pointer(&cpu.SleepUntil)) - base, 72},
		{"InsnCount", uintptr(unsafe.Pointer(&cpu.InsnCount)) - base, 80},
		{"CacheHits", uintptr(unsafe.Pointer(&cpu.CacheHits)) - base, 88},
		{"CacheMisses", uintptr(unsafe.Pointer(&cpu.CacheMisses)) - base, 96},
	}

	for _, tt := range tests {
		if tt.offset != tt.want {
			t.Errorf("offset(%s) = %d, want %d", tt.name, tt.offset, tt.want)
		}
	}
}

// TestCpuStateMarshalRoundtrip verifies marshal/unmarshal round-trip.
func TestCpuStateMarshalRoundtrip(t *testing.T) {
	cpu := &CpuState{
		PC:          0x1234,
		Flags:       FlagZero | FlagCarry,
		Halted:      1,
		Stalled:     0,
		SleepUntil:  999999,
		InsnCount:   42000,
		CacheHits:   300,
		CacheMisses: 50,
	}
	cpu.Regs[0] = 0xDEADBEEF
	cpu.Regs[RegSP] = RegSPDefault

	data := cpu.MarshalBinary()
	if len(data) != CpuStateSize {
		t.Fatalf("MarshalBinary() len = %d, want %d", len(data), CpuStateSize)
	}

	got, err := UnmarshalCpuState(data)
	if err != nil {
		t.Fatalf("UnmarshalCpuState: %v", err)
	}

	if got.PC != cpu.PC {
		t.Errorf("PC = 0x%X, want 0x%X", got.PC, cpu.PC)
	}
	if got.Flags != cpu.Flags {
		t.Errorf("Flags = 0x%X, want 0x%X", got.Flags, cpu.Flags)
	}
	if got.Halted != cpu.Halted {
		t.Errorf("Halted = %d, want %d", got.Halted, cpu.Halted)
	}
	if got.Stalled != cpu.Stalled {
		t.Errorf("Stalled = %d, want %d", got.Stalled, cpu.Stalled)
	}
	if got.SleepUntil != cpu.SleepUntil {
		t.Errorf("SleepUntil = %d, want %d", got.SleepUntil, cpu.SleepUntil)
	}
	if got.InsnCount != cpu.InsnCount {
		t.Errorf("InsnCount = %d, want %d", got.InsnCount, cpu.InsnCount)
	}
	if got.CacheHits != cpu.CacheHits {
		t.Errorf("CacheHits = %d, want %d", got.CacheHits, cpu.CacheHits)
	}
	if got.CacheMisses != cpu.CacheMisses {
		t.Errorf("CacheMisses = %d, want %d", got.CacheMisses, cpu.CacheMisses)
	}
	if got.Regs[0] != cpu.Regs[0] {
		t.Errorf("Regs[0] = 0x%X, want 0x%X", got.Regs[0], cpu.Regs[0])
	}
	if got.Regs[RegSP] != cpu.Regs[RegSP] {
		t.Errorf("Regs[SP] = 0x%X, want 0x%X", got.Regs[RegSP], cpu.Regs[RegSP])
	}
}

// TestUnmarshalCpuStateTooSmall verifies error on short buffer.
func TestUnmarshalCpuStateTooSmall(t *testing.T) {
	data := make([]byte, 50)
	_, err := UnmarshalCpuState(data)
	if err == nil {
		t.Fatal("UnmarshalCpuState should fail on short buffer")
	}
}

// TestKeyboardStateSize verifies the Go KeyboardState struct is 16 bytes.
func TestKeyboardStateSize(t *testing.T) {
	size := unsafe.Sizeof(KeyboardState{})
	if size != KeyboardStateSize {
		t.Fatalf("KeyboardState size = %d, want %d", size, KeyboardStateSize)
	}
}

// TestKeyboardStateMarshalRoundtrip verifies marshal/unmarshal round-trip.
func TestKeyboardStateMarshalRoundtrip(t *testing.T) {
	ks := &KeyboardState{
		Key:      42,
		Pressed:  1,
		Sequence: 12345,
	}

	data := ks.MarshalBinary()
	if len(data) != KeyboardStateSize {
		t.Fatalf("MarshalBinary() len = %d, want %d", len(data), KeyboardStateSize)
	}

	got, err := UnmarshalKeyboardState(data)
	if err != nil {
		t.Fatalf("UnmarshalKeyboardState: %v", err)
	}

	if got.Key != ks.Key {
		t.Errorf("Key = %d, want %d", got.Key, ks.Key)
	}
	if got.Pressed != ks.Pressed {
		t.Errorf("Pressed = %d, want %d", got.Pressed, ks.Pressed)
	}
	if got.Sequence != ks.Sequence {
		t.Errorf("Sequence = %d, want %d", got.Sequence, ks.Sequence)
	}
}

// TestUnmarshalKeyboardStateTooSmall verifies error on short buffer.
func TestUnmarshalKeyboardStateTooSmall(t *testing.T) {
	data := make([]byte, 8)
	_, err := UnmarshalKeyboardState(data)
	if err == nil {
		t.Fatal("UnmarshalKeyboardState should fail on short buffer")
	}
}

// TestFlagConstants verifies flag constants match the Rust mbc_flags.
func TestFlagConstants(t *testing.T) {
	if FlagZero != 0x01 {
		t.Errorf("FlagZero = 0x%02X, want 0x01", FlagZero)
	}
	if FlagNegative != 0x02 {
		t.Errorf("FlagNegative = 0x%02X, want 0x02", FlagNegative)
	}
	if FlagCarry != 0x04 {
		t.Errorf("FlagCarry = 0x%02X, want 0x04", FlagCarry)
	}

	// Verify no overlap
	if FlagZero&FlagNegative != 0 {
		t.Error("FlagZero and FlagNegative overlap")
	}
	if FlagZero&FlagCarry != 0 {
		t.Error("FlagZero and FlagCarry overlap")
	}
	if FlagNegative&FlagCarry != 0 {
		t.Error("FlagNegative and FlagCarry overlap")
	}
}

// TestRegConstants verifies register constants.
func TestRegConstants(t *testing.T) {
	if RegSP != 15 {
		t.Errorf("RegSP = %d, want 15", RegSP)
	}
	if RegSPDefault != 0xFFFF_0000 {
		t.Errorf("RegSPDefault = 0x%X, want 0xFFFF0000", RegSPDefault)
	}
	if RegPCDefault != 0 {
		t.Errorf("RegPCDefault = %d, want 0", RegPCDefault)
	}
	if RegCount != 16 {
		t.Errorf("RegCount = %d, want 16", RegCount)
	}
}

// TestMaxROMConstants verifies ROM size limits.
func TestMaxROMConstants(t *testing.T) {
	if MaxROMInstructions != 262144 {
		t.Errorf("MaxROMInstructions = %d, want 262144", MaxROMInstructions)
	}
	if MaxROMBytes != 262144*4 {
		t.Errorf("MaxROMBytes = %d, want %d", MaxROMBytes, 262144*4)
	}
}
