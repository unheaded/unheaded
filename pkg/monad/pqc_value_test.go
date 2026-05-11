// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package monad

import (
	"encoding/binary"
	"testing"
)

func TestMarshalUnmarshalRoundTrip(t *testing.T) {
	original := PQCValue{
		SigRef:  0x00ABCDEF,
		KeyRef:  0x1234,
		HashPfx: [4]byte{0xDE, 0xAD, 0xBE, 0xEF},
		SeqNum:  0x5678,
	}

	buf := original.Marshal()
	restored, err := UnmarshalPQCValue(buf[:])
	if err != nil {
		t.Fatalf("UnmarshalPQCValue failed: %v", err)
	}

	if original != restored {
		t.Fatalf("round-trip mismatch: original=%+v, restored=%+v", original, restored)
	}
}

func TestMarshalSize(t *testing.T) {
	// Marshal returns [PQCValueSize]byte (fixed-size); type system already
	// guarantees the length. Pin the constant matches the spec instead.
	if PQCValueSize != 12 {
		t.Fatalf("PQCValueSize constant should be 12 (per draft-bellis-unheaded-pqc-authentication-00), got %d", PQCValueSize)
	}
	v := PQCValue{}
	_ = v.Marshal()
}

func TestPseudoHeaderMarshalSize(t *testing.T) {
	// Marshal returns [PseudoHeaderSize]byte; type guarantees length.
	// Pin the constant matches the spec.
	if PseudoHeaderSize != 52 {
		t.Fatalf("PseudoHeaderSize constant should be 52, got %d", PseudoHeaderSize)
	}
	ph := PseudoHeader{}
	_ = ph.Marshal()
}

func TestPseudoHeaderMarshalLayout(t *testing.T) {
	srcIP := [16]byte{0x20, 0x01, 0x0d, 0xb8, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01}
	dstIP := [16]byte{0x20, 0x01, 0x0d, 0xb8, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02}
	monadPfx := [8]byte{0x01, 0x0A, 0x0B, 0x00, 0x03, 0x01, 0x01, 0x0A}
	pqcVal := [12]byte{0x00, 0xAB, 0xCD, 0xEF, 0x12, 0x34, 0xDE, 0xAD, 0xBE, 0xEF, 0x56, 0x78}

	ph := BuildPseudoHeader(srcIP, dstIP, monadPfx, pqcVal)
	buf := ph.Marshal()

	// Verify SrcIP at offset 0
	for i := 0; i < 16; i++ {
		if buf[i] != srcIP[i] {
			t.Fatalf("SrcIP byte %d: expected 0x%02X, got 0x%02X", i, srcIP[i], buf[i])
		}
	}
	// Verify DstIP at offset 16
	for i := 0; i < 16; i++ {
		if buf[16+i] != dstIP[i] {
			t.Fatalf("DstIP byte %d: expected 0x%02X, got 0x%02X", i, dstIP[i], buf[16+i])
		}
	}
	// Verify Monad prefix at offset 32
	for i := 0; i < 8; i++ {
		if buf[32+i] != monadPfx[i] {
			t.Fatalf("Monad byte %d: expected 0x%02X, got 0x%02X", i, monadPfx[i], buf[32+i])
		}
	}
	// Verify PQC value at offset 40
	for i := 0; i < 12; i++ {
		if buf[40+i] != pqcVal[i] {
			t.Fatalf("PQCValue byte %d: expected 0x%02X, got 0x%02X", i, pqcVal[i], buf[40+i])
		}
	}
}

func TestIsPQCActive(t *testing.T) {
	tests := []struct {
		name     string
		flags    byte
		expected bool
	}{
		{"no flags", 0x00, false},
		{"sampled only", FlagSampled, false},
		{"custom only", FlagCustom, false},
		{"both set", FlagSampled | FlagCustom, true},
		{"both set with others", 0xFF, true},
		{"chaos+sampled+custom", FlagChaos | FlagSampled | FlagCustom, true},
		{"all except custom", 0xFF &^ FlagCustom, false},
		{"all except sampled", 0xFF &^ FlagSampled, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsPQCActive(tt.flags)
			if got != tt.expected {
				t.Fatalf("IsPQCActive(0x%02X) = %v, want %v", tt.flags, got, tt.expected)
			}
		})
	}
}

func TestSetPQCActive(t *testing.T) {
	result := SetPQCActive(0x00)
	if result != FlagPQCActive {
		t.Fatalf("SetPQCActive(0x00) = 0x%02X, want 0x%02X", result, FlagPQCActive)
	}

	// Setting on existing flags should preserve them
	result = SetPQCActive(FlagChaos | FlagTraced)
	expected := byte(FlagChaos | FlagTraced | FlagPQCActive)
	if result != expected {
		t.Fatalf("SetPQCActive(0x%02X) = 0x%02X, want 0x%02X", byte(FlagChaos|FlagTraced), result, expected)
	}
}

func TestClearPQCActive(t *testing.T) {
	result := ClearPQCActive(FlagPQCActive)
	if result != 0x00 {
		t.Fatalf("ClearPQCActive(0x%02X) = 0x%02X, want 0x00", FlagPQCActive, result)
	}

	// Clearing should preserve other flags
	input := byte(FlagChaos | FlagTraced | FlagPQCActive)
	result = ClearPQCActive(input)
	expected := byte(FlagChaos | FlagTraced)
	if result != expected {
		t.Fatalf("ClearPQCActive(0x%02X) = 0x%02X, want 0x%02X", input, result, expected)
	}
}

func TestIsZero(t *testing.T) {
	zero := PQCValue{}
	if !zero.IsZero() {
		t.Fatal("zero PQCValue should report IsZero() == true")
	}

	nonZero := PQCValue{SigRef: 1}
	if nonZero.IsZero() {
		t.Fatal("non-zero PQCValue should report IsZero() == false")
	}

	nonZeroHash := PQCValue{HashPfx: [4]byte{0, 0, 0, 1}}
	if nonZeroHash.IsZero() {
		t.Fatal("PQCValue with non-zero HashPfx should report IsZero() == false")
	}

	nonZeroSeq := PQCValue{SeqNum: 1}
	if nonZeroSeq.IsZero() {
		t.Fatal("PQCValue with non-zero SeqNum should report IsZero() == false")
	}

	nonZeroKey := PQCValue{KeyRef: 1}
	if nonZeroKey.IsZero() {
		t.Fatal("PQCValue with non-zero KeyRef should report IsZero() == false")
	}
}

func TestNetworkByteOrder(t *testing.T) {
	v := PQCValue{
		SigRef:  0x01020304,
		KeyRef:  0x0506,
		HashPfx: [4]byte{0x07, 0x08, 0x09, 0x0A},
		SeqNum:  0x0B0C,
	}

	buf := v.Marshal()

	// SigRef (big-endian u32): bytes 0-3 should be 01 02 03 04
	if buf[0] != 0x01 || buf[1] != 0x02 || buf[2] != 0x03 || buf[3] != 0x04 {
		t.Fatalf("SigRef big-endian: expected [01 02 03 04], got [%02X %02X %02X %02X]",
			buf[0], buf[1], buf[2], buf[3])
	}

	// KeyRef (big-endian u16): bytes 4-5 should be 05 06
	if buf[4] != 0x05 || buf[5] != 0x06 {
		t.Fatalf("KeyRef big-endian: expected [05 06], got [%02X %02X]", buf[4], buf[5])
	}

	// HashPfx: bytes 6-9 should be 07 08 09 0A
	if buf[6] != 0x07 || buf[7] != 0x08 || buf[8] != 0x09 || buf[9] != 0x0A {
		t.Fatalf("HashPfx: expected [07 08 09 0A], got [%02X %02X %02X %02X]",
			buf[6], buf[7], buf[8], buf[9])
	}

	// SeqNum (big-endian u16): bytes 10-11 should be 0B 0C
	if buf[10] != 0x0B || buf[11] != 0x0C {
		t.Fatalf("SeqNum big-endian: expected [0B 0C], got [%02X %02X]", buf[10], buf[11])
	}

	// Verify via encoding/binary directly
	if binary.BigEndian.Uint32(buf[0:4]) != 0x01020304 {
		t.Fatal("SigRef encoding/binary mismatch")
	}
	if binary.BigEndian.Uint16(buf[4:6]) != 0x0506 {
		t.Fatal("KeyRef encoding/binary mismatch")
	}
	if binary.BigEndian.Uint16(buf[10:12]) != 0x0B0C {
		t.Fatal("SeqNum encoding/binary mismatch")
	}
}

func TestUnmarshalShortBuffer(t *testing.T) {
	short := make([]byte, 5)
	_, err := UnmarshalPQCValue(short)
	if err == nil {
		t.Fatal("expected error for short buffer, got nil")
	}
	if err != ErrShortBuffer {
		t.Fatalf("expected ErrShortBuffer, got %v", err)
	}

	// Empty buffer
	_, err = UnmarshalPQCValue(nil)
	if err == nil {
		t.Fatal("expected error for nil buffer, got nil")
	}

	// Exactly 11 bytes (one short)
	_, err = UnmarshalPQCValue(make([]byte, 11))
	if err == nil {
		t.Fatal("expected error for 11-byte buffer, got nil")
	}
}

func TestUnmarshalExactSize(t *testing.T) {
	buf := make([]byte, PQCValueSize)
	binary.BigEndian.PutUint32(buf[0:4], 42)
	binary.BigEndian.PutUint16(buf[4:6], 7)
	buf[6] = 0xAA
	buf[7] = 0xBB
	buf[8] = 0xCC
	buf[9] = 0xDD
	binary.BigEndian.PutUint16(buf[10:12], 99)

	v, err := UnmarshalPQCValue(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.SigRef != 42 {
		t.Fatalf("SigRef = %d, want 42", v.SigRef)
	}
	if v.KeyRef != 7 {
		t.Fatalf("KeyRef = %d, want 7", v.KeyRef)
	}
	if v.HashPfx != [4]byte{0xAA, 0xBB, 0xCC, 0xDD} {
		t.Fatalf("HashPfx = %v, want [AA BB CC DD]", v.HashPfx)
	}
	if v.SeqNum != 99 {
		t.Fatalf("SeqNum = %d, want 99", v.SeqNum)
	}
}

func TestFlagConstants(t *testing.T) {
	// Verify individual flag values
	if FlagChaos != 0x80 {
		t.Fatalf("FlagChaos = 0x%02X, want 0x80", FlagChaos)
	}
	if FlagCanary != 0x40 {
		t.Fatalf("FlagCanary = 0x%02X, want 0x40", FlagCanary)
	}
	if FlagTraced != 0x20 {
		t.Fatalf("FlagTraced = 0x%02X, want 0x20", FlagTraced)
	}
	if FlagEncrypt != 0x10 {
		t.Fatalf("FlagEncrypt = 0x%02X, want 0x10", FlagEncrypt)
	}
	if FlagSampled != 0x08 {
		t.Fatalf("FlagSampled = 0x%02X, want 0x08", FlagSampled)
	}
	if FlagMirror != 0x04 {
		t.Fatalf("FlagMirror = 0x%02X, want 0x04", FlagMirror)
	}
	if FlagCustom != 0x02 {
		t.Fatalf("FlagCustom = 0x%02X, want 0x02", FlagCustom)
	}
	if FlagReserved != 0x01 {
		t.Fatalf("FlagReserved = 0x%02X, want 0x01", FlagReserved)
	}
	if FlagPQCActive != 0x0A {
		t.Fatalf("FlagPQCActive = 0x%02X, want 0x0A", FlagPQCActive)
	}
}

func TestUnmarshalLongerBuffer(t *testing.T) {
	// Unmarshal should succeed with a buffer longer than 12 bytes
	buf := make([]byte, 20)
	binary.BigEndian.PutUint32(buf[0:4], 0xDEADBEEF)
	binary.BigEndian.PutUint16(buf[4:6], 0x1234)
	copy(buf[6:10], []byte{0xAA, 0xBB, 0xCC, 0xDD})
	binary.BigEndian.PutUint16(buf[10:12], 0x5678)
	// Extra bytes should be ignored
	buf[12] = 0xFF

	v, err := UnmarshalPQCValue(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.SigRef != 0xDEADBEEF {
		t.Fatalf("SigRef = 0x%08X, want 0xDEADBEEF", v.SigRef)
	}
}
