// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package doom

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadROMFromFile tests loading a ROM from disk.
func TestLoadROMFromFile(t *testing.T) {
	// Create a temporary .mbc file with 3 instructions
	tmpdir := t.TempDir()
	mbcFile := filepath.Join(tmpdir, "test.mbc")

	instructions := []uint32{
		0x0F_1_0_0000, // MOVI r1, 0
		0x01_1_2_0005, // ADD r1, r2, 5
		0xFF_0_0_0000, // HALT
	}

	var buf []byte
	for _, insn := range instructions {
		b := make([]byte, 4)
		binary.LittleEndian.PutUint32(b, insn)
		buf = append(buf, b...)
	}

	if err := os.WriteFile(mbcFile, buf, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	rom, err := LoadROM(mbcFile)
	if err != nil {
		t.Fatalf("LoadROM: %v", err)
	}

	if rom.InstructionCount() != 3 {
		t.Errorf("InstructionCount() = %d, want 3", rom.InstructionCount())
	}

	if rom.SizeBytes() != 12 {
		t.Errorf("SizeBytes() = %d, want 12", rom.SizeBytes())
	}

	for i, want := range instructions {
		if rom.Instructions[i] != want {
			t.Errorf("Instructions[%d] = 0x%08X, want 0x%08X", i, rom.Instructions[i], want)
		}
	}
}

// TestLoadROMNotFound tests loading a non-existent file.
func TestLoadROMNotFound(t *testing.T) {
	_, err := LoadROM("/nonexistent/file.mbc")
	if err == nil {
		t.Fatal("LoadROM should fail for non-existent file")
	}
}

// TestParseROMEmpty tests parsing an empty ROM.
func TestParseROMEmpty(t *testing.T) {
	_, err := ParseROM([]byte{})
	if err == nil {
		t.Fatal("ParseROM should fail for empty data")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error message should mention 'empty', got: %v", err)
	}
}

// TestParseROMNotAligned tests parsing a ROM with non-aligned size.
func TestParseROMNotAligned(t *testing.T) {
	_, err := ParseROM([]byte{0x01, 0x02, 0x03})
	if err == nil {
		t.Fatal("ParseROM should fail for non-aligned size")
	}
	if !strings.Contains(err.Error(), "multiple of 4") {
		t.Errorf("error message should mention alignment, got: %v", err)
	}
}

// TestParseROMTooLarge tests parsing a ROM that exceeds the max size.
func TestParseROMTooLarge(t *testing.T) {
	data := make([]byte, MaxROMBytes+4) // One instruction too many
	_, err := ParseROM(data)
	if err == nil {
		t.Fatal("ParseROM should fail for oversized ROM")
	}
	if !strings.Contains(err.Error(), "exceeds maximum") {
		t.Errorf("error message should mention maximum, got: %v", err)
	}
}

// TestParseROMMaxSize tests parsing a ROM at exactly the max size.
func TestParseROMMaxSize(t *testing.T) {
	data := make([]byte, MaxROMBytes) // Exactly at limit
	rom, err := ParseROM(data)
	if err != nil {
		t.Fatalf("ParseROM at max size should succeed: %v", err)
	}
	if rom.InstructionCount() != MaxROMInstructions {
		t.Errorf("InstructionCount() = %d, want %d", rom.InstructionCount(), MaxROMInstructions)
	}
}

// TestParseROMValid tests parsing valid ROM data.
func TestParseROMValid(t *testing.T) {
	data := make([]byte, 8)
	binary.LittleEndian.PutUint32(data[0:4], 0xDEADBEEF)
	binary.LittleEndian.PutUint32(data[4:8], 0xCAFEBABE)

	rom, err := ParseROM(data)
	if err != nil {
		t.Fatalf("ParseROM: %v", err)
	}

	if rom.Instructions[0] != 0xDEADBEEF {
		t.Errorf("Instructions[0] = 0x%X, want 0xDEADBEEF", rom.Instructions[0])
	}
	if rom.Instructions[1] != 0xCAFEBABE {
		t.Errorf("Instructions[1] = 0x%X, want 0xCAFEBABE", rom.Instructions[1])
	}
}

// TestROMSummary tests the ROM summary output.
func TestROMSummary(t *testing.T) {
	data := make([]byte, 100*4) // 100 instructions
	rom, err := ParseROM(data)
	if err != nil {
		t.Fatalf("ParseROM: %v", err)
	}

	summary := rom.Summary()
	if !strings.Contains(summary, "100") {
		t.Errorf("Summary should contain instruction count '100', got: %s", summary)
	}
	if !strings.Contains(summary, "400 bytes") {
		t.Errorf("Summary should contain '400 bytes', got: %s", summary)
	}
}

// TestROMHMAC tests HMAC-SHA256 computation and verification.
func TestROMHMAC(t *testing.T) {
	data := make([]byte, 8)
	binary.LittleEndian.PutUint32(data[0:4], 0x01020304)
	binary.LittleEndian.PutUint32(data[4:8], 0x05060708)

	rom, err := ParseROM(data)
	if err != nil {
		t.Fatalf("ParseROM: %v", err)
	}

	key := []byte("unheaded-doom-secret")
	mac := rom.HMAC(key)

	if len(mac) != sha256.Size {
		t.Fatalf("HMAC length = %d, want %d", len(mac), sha256.Size)
	}

	// Verify using standard library
	h := hmac.New(sha256.New, key)
	h.Write(data)
	expected := h.Sum(nil)

	if !hmac.Equal(mac, expected) {
		t.Error("HMAC does not match expected value")
	}

	// Verify positive verification
	if !rom.VerifyHMAC(key, expected) {
		t.Error("VerifyHMAC should return true for correct MAC")
	}

	// Verify negative verification
	badMAC := make([]byte, sha256.Size)
	if rom.VerifyHMAC(key, badMAC) {
		t.Error("VerifyHMAC should return false for incorrect MAC")
	}
}

// TestROMHMACDifferentKeys tests that different keys produce different HMACs.
func TestROMHMACDifferentKeys(t *testing.T) {
	data := make([]byte, 4)
	binary.LittleEndian.PutUint32(data, 0xDEADBEEF)

	rom, err := ParseROM(data)
	if err != nil {
		t.Fatalf("ParseROM: %v", err)
	}

	mac1 := rom.HMAC([]byte("key1"))
	mac2 := rom.HMAC([]byte("key2"))

	if hmac.Equal(mac1, mac2) {
		t.Error("Different keys should produce different HMACs")
	}
}

// TestROMWriteToMap tests writing ROM to a mock map.
func TestROMWriteToMap(t *testing.T) {
	instructions := []uint32{0x01020304, 0x05060708, 0xFF000000}
	data := make([]byte, len(instructions)*4)
	for i, insn := range instructions {
		binary.LittleEndian.PutUint32(data[i*4:], insn)
	}

	rom, err := ParseROM(data)
	if err != nil {
		t.Fatalf("ParseROM: %v", err)
	}

	m := newMockMap()
	if err := rom.WriteToMap(m); err != nil {
		t.Fatalf("WriteToMap: %v", err)
	}

	// Verify all instructions were written
	if m.count() != len(instructions) {
		t.Errorf("map count = %d, want %d", m.count(), len(instructions))
	}

	// Read back and verify
	for i, want := range instructions {
		key := make([]byte, 4)
		binary.LittleEndian.PutUint32(key, uint32(i))
		val, err := m.LookupElem(key, 4)
		if err != nil {
			t.Fatalf("LookupElem(%d): %v", i, err)
		}
		got := binary.LittleEndian.Uint32(val)
		if got != want {
			t.Errorf("rom_map[%d] = 0x%08X, want 0x%08X", i, got, want)
		}
	}
}

// TestROMWriteToMapError tests WriteToMap with an error-returning map.
func TestROMWriteToMapError(t *testing.T) {
	data := make([]byte, 4)
	binary.LittleEndian.PutUint32(data, 0xDEADBEEF)

	rom, err := ParseROM(data)
	if err != nil {
		t.Fatalf("ParseROM: %v", err)
	}

	m := newErrorMap("bpf error")
	err = rom.WriteToMap(m)
	if err == nil {
		t.Fatal("WriteToMap should fail with error map")
	}
	if !strings.Contains(err.Error(), "rom_map update") {
		t.Errorf("error should mention rom_map, got: %v", err)
	}
}
