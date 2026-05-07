// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package upc

import (
	"encoding/binary"
	"testing"
)

// ── CreateUPCFlat / ParseUPCFlat roundtrip ──────────────────────────

func TestUPCFlatRoundtrip(t *testing.T) {
	text := []uint32{
		mbcEncode(mbcMOVI, 0, 0, mbcSysWrite), // MOVI r0, SYS_WRITE
		mbcEncode(mbcMOVI, 1, 0, 1),           // MOVI r1, 1 (stdout)
		mbcEncode(mbcINT, 0, 0, 0x80),         // INT 0x80
		mbcEncode(mbcMOVI, 0, 0, mbcSysExit),  // MOVI r0, SYS_EXIT
		mbcEncode(mbcINT, 0, 0, 0x80),         // INT 0x80
	}
	data := []uint32{0x6C6C6548, 0x0000006F} // "Hello\0\0\0"
	bssWords := uint32(4)
	stackSize := uint32(4096)

	bin := CreateUPCFlat(text, data, bssWords, stackSize)

	// Verify header magic
	if string(bin[0:4]) != UPCFlatMagic {
		t.Fatalf("magic = %q, want %q", string(bin[0:4]), UPCFlatMagic)
	}

	// Verify total size
	expectedSize := UPCFlatHeaderSize + (len(text)+len(data))*4
	if len(bin) != expectedSize {
		t.Fatalf("binary size = %d, want %d", len(bin), expectedSize)
	}

	// Parse it back
	hdr, textOut, dataOut, err := ParseUPCFlat(bin)
	if err != nil {
		t.Fatalf("ParseUPCFlat: %v", err)
	}

	// Verify header fields
	if string(hdr.Magic[:]) != UPCFlatMagic {
		t.Errorf("hdr.Magic = %q, want %q", hdr.Magic, UPCFlatMagic)
	}
	if hdr.Version != UPCFlatVersion {
		t.Errorf("hdr.Version = %d, want %d", hdr.Version, UPCFlatVersion)
	}
	if hdr.Entry != 0 {
		t.Errorf("hdr.Entry = %d, want 0", hdr.Entry)
	}
	if hdr.TextSize != uint32(len(text)) {
		t.Errorf("hdr.TextSize = %d, want %d", hdr.TextSize, len(text))
	}
	if hdr.DataSize != uint32(len(data)) {
		t.Errorf("hdr.DataSize = %d, want %d", hdr.DataSize, len(data))
	}
	if hdr.BSSSize != bssWords {
		t.Errorf("hdr.BSSSize = %d, want %d", hdr.BSSSize, bssWords)
	}
	if hdr.StackSize != stackSize {
		t.Errorf("hdr.StackSize = %d, want %d", hdr.StackSize, stackSize)
	}

	// Verify text roundtrip
	if len(textOut) != len(text) {
		t.Fatalf("text length = %d, want %d", len(textOut), len(text))
	}
	for i, w := range text {
		if textOut[i] != w {
			t.Errorf("text[%d] = 0x%08X, want 0x%08X", i, textOut[i], w)
		}
	}

	// Verify data roundtrip
	if len(dataOut) != len(data) {
		t.Fatalf("data length = %d, want %d", len(dataOut), len(data))
	}
	for i, w := range data {
		if dataOut[i] != w {
			t.Errorf("data[%d] = 0x%08X, want 0x%08X", i, dataOut[i], w)
		}
	}
}

func TestUPCFlatRoundtripEmptyData(t *testing.T) {
	text := []uint32{mbcEncode(mbcMOVI, 0, 0, mbcSysExit), mbcEncode(mbcINT, 0, 0, 0x80)}
	bin := CreateUPCFlat(text, nil, 0, 0)

	hdr, textOut, dataOut, err := ParseUPCFlat(bin)
	if err != nil {
		t.Fatalf("ParseUPCFlat: %v", err)
	}
	if hdr.DataSize != 0 {
		t.Errorf("DataSize = %d, want 0", hdr.DataSize)
	}
	if len(dataOut) != 0 {
		t.Errorf("data length = %d, want 0", len(dataOut))
	}
	if len(textOut) != len(text) {
		t.Errorf("text length = %d, want %d", len(textOut), len(text))
	}
}

// ── Invalid magic rejection ─────────────────────────────────────────

func TestUPCFlatInvalidMagic(t *testing.T) {
	// Create valid binary then corrupt the magic
	text := []uint32{0x0F000001}
	bin := CreateUPCFlat(text, nil, 0, 0)
	bin[0] = 'X' // corrupt magic

	_, _, _, err := ParseUPCFlat(bin)
	if err == nil {
		t.Fatal("ParseUPCFlat should reject invalid magic")
	}
}

func TestUPCFlatTooShort(t *testing.T) {
	_, _, _, err := ParseUPCFlat([]byte("UPCF"))
	if err == nil {
		t.Fatal("ParseUPCFlat should reject binary shorter than header")
	}
}

func TestUPCFlatBadVersion(t *testing.T) {
	text := []uint32{0x0F000001}
	bin := CreateUPCFlat(text, nil, 0, 0)
	// Set version to 99
	binary.LittleEndian.PutUint32(bin[4:8], 99)

	_, _, _, err := ParseUPCFlat(bin)
	if err == nil {
		t.Fatal("ParseUPCFlat should reject unsupported version")
	}
}

func TestUPCFlatTruncatedPayload(t *testing.T) {
	text := []uint32{0x0F000001, 0x0F000002, 0x0F000003}
	bin := CreateUPCFlat(text, nil, 0, 0)
	// Truncate payload (remove last 4 bytes)
	bin = bin[:len(bin)-4]

	_, _, _, err := ParseUPCFlat(bin)
	if err == nil {
		t.Fatal("ParseUPCFlat should reject truncated payload")
	}
}

// ── LoadUPCFlat into CPU memory ─────────────────────────────────────

func TestLoadUPCFlat(t *testing.T) {
	text := []uint32{
		mbcEncode(mbcMOVI, 0, 0, 42),         // MOVI r0, 42
		mbcEncode(mbcMOVI, 1, 0, 1),          // MOVI r1, 1
		mbcEncode(mbcMOVI, 0, 0, mbcSysExit), // MOVI r0, SYS_EXIT
		mbcEncode(mbcINT, 0, 0, 0x80),        // INT 0x80
	}
	data := []uint32{0xDEADBEEF, 0xCAFEBABE}
	bssWords := uint32(3)
	stackSize := uint32(4096)

	bin := CreateUPCFlat(text, data, bssWords, stackSize)

	rom := make([]uint32, 1024)
	ram := make([]uint32, 1024)
	dataAddr := uint32(100)

	// Pre-fill RAM with 0xFF to verify BSS zeroing
	for i := range ram {
		ram[i] = 0xFFFFFFFF
	}

	hdr, err := LoadUPCFlat(bin, rom, ram, dataAddr)
	if err != nil {
		t.Fatalf("LoadUPCFlat: %v", err)
	}

	// Verify text in ROM
	for i, w := range text {
		if rom[i] != w {
			t.Errorf("rom[%d] = 0x%08X, want 0x%08X", i, rom[i], w)
		}
	}

	// Verify data in RAM
	if ram[100] != 0xDEADBEEF {
		t.Errorf("ram[100] = 0x%08X, want 0xDEADBEEF", ram[100])
	}
	if ram[101] != 0xCAFEBABE {
		t.Errorf("ram[101] = 0x%08X, want 0xCAFEBABE", ram[101])
	}

	// Verify BSS is zeroed (ram[102], ram[103], ram[104])
	for i := uint32(0); i < bssWords; i++ {
		addr := dataAddr + uint32(len(data)) + i
		if ram[addr] != 0 {
			t.Errorf("BSS ram[%d] = 0x%08X, want 0 (zero-filled)", addr, ram[addr])
		}
	}

	// Verify that ram beyond BSS is still 0xFFFFFFFF (untouched)
	beyondBSS := dataAddr + uint32(len(data)) + bssWords
	if ram[beyondBSS] != 0xFFFFFFFF {
		t.Errorf("ram[%d] = 0x%08X, should be untouched (0xFFFFFFFF)", beyondBSS, ram[beyondBSS])
	}

	// Verify header
	if hdr.Entry != 0 {
		t.Errorf("hdr.Entry = %d, want 0", hdr.Entry)
	}
	if hdr.StackSize != stackSize {
		t.Errorf("hdr.StackSize = %d, want %d", hdr.StackSize, stackSize)
	}
}

func TestLoadUPCFlatTextOverflow(t *testing.T) {
	// ROM too small for text
	text := make([]uint32, 100)
	bin := CreateUPCFlat(text, nil, 0, 0)

	rom := make([]uint32, 10) // too small
	ram := make([]uint32, 100)

	_, err := LoadUPCFlat(bin, rom, ram, 0)
	if err == nil {
		t.Fatal("LoadUPCFlat should reject text that exceeds ROM capacity")
	}
}

func TestLoadUPCFlatDataOverflow(t *testing.T) {
	text := []uint32{0x0F000001}
	data := make([]uint32, 50)
	bin := CreateUPCFlat(text, data, 0, 0)

	rom := make([]uint32, 100)
	ram := make([]uint32, 30) // too small for data at any offset

	_, err := LoadUPCFlat(bin, rom, ram, 0)
	if err == nil {
		t.Fatal("LoadUPCFlat should reject data that exceeds RAM capacity")
	}
}

func TestLoadUPCFlatBSSOverflow(t *testing.T) {
	text := []uint32{0x0F000001}
	data := []uint32{0xDEAD}
	bin := CreateUPCFlat(text, data, 1000, 0) // BSS way too large

	rom := make([]uint32, 100)
	ram := make([]uint32, 100)

	_, err := LoadUPCFlat(bin, rom, ram, 0)
	if err == nil {
		t.Fatal("LoadUPCFlat should reject BSS that exceeds RAM capacity")
	}
}

// ── Integration: simulate a loaded UPCFlat binary ───────────────────

func TestLoadUPCFlatAndSimulate(t *testing.T) {
	// Build a small program that prints "OK\n" and exits.
	msg := "OK\n"
	msgWords := mbcPackString(msg)

	code := []uint32{
		mbcEncode(mbcMOVI, 0, 0, mbcSysWrite),      // r0 = SYS_WRITE
		mbcEncode(mbcMOVI, 1, 0, 1),                // r1 = stdout
		mbcEncode(mbcMOVI, 2, 0, 0),                // r2 = msg addr (patched below)
		mbcEncode(mbcMOVI, 3, 0, uint16(len(msg))), // r3 = len
		mbcEncode(mbcINT, 0, 0, 0x80),              // INT 0x80
		mbcEncode(mbcMOVI, 0, 0, mbcSysExit),       // r0 = SYS_EXIT
		mbcEncode(mbcMOVI, 1, 0, 0),                // r1 = 0
		mbcEncode(mbcINT, 0, 0, 0x80),              // INT 0x80
	}

	// Data offset: the text is loaded at ROM[0], but data goes into RAM.
	// For simulation, we build a flat ROM with text+data (like the test simulator expects).
	// The data byte address = len(code)*4 in the flat image.
	dataByteAddr := uint16(len(code) * 4)
	code[2] = mbcEncode(mbcMOVI, 2, 0, dataByteAddr)

	// Create the UPCFlat binary
	bin := CreateUPCFlat(code, msgWords, 0, 4096)

	// Parse it back and load into a flat ROM for simulation
	hdr, textOut, dataOut, err := ParseUPCFlat(bin)
	if err != nil {
		t.Fatalf("ParseUPCFlat: %v", err)
	}

	// For simulation: combine text+data into a single flat ROM
	rom := append(textOut, dataOut...)

	output, exitCode, halted, errMsg := simulateMBCProgram(rom, nil, 1000)
	if errMsg != "" {
		t.Fatalf("simulation error: %s", errMsg)
	}
	if !halted {
		t.Error("program did not halt")
	}
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}
	if output != msg {
		t.Errorf("output = %q, want %q", output, msg)
	}

	// Verify header entry
	if hdr.Entry != 0 {
		t.Errorf("entry = %d, want 0", hdr.Entry)
	}
}

// ── UNFS integration: store and load UPCFlat from filesystem ────────

func TestUPCFlatFilesystemRoundtrip(t *testing.T) {
	// Build a UPCFlat binary
	text := []uint32{
		mbcEncode(mbcMOVI, 0, 0, mbcSysExit),
		mbcEncode(mbcMOVI, 1, 0, 0),
		mbcEncode(mbcINT, 0, 0, 0x80),
	}
	data := []uint32{0x12345678}
	bin := CreateUPCFlat(text, data, 2, 4096)

	// Store in a UNFS filesystem
	fs := FormatFilesystem()
	_, err := AddFile(fs, "/bin/hello", bin, 0o755)
	if err != nil {
		t.Fatalf("AddFile: %v", err)
	}

	// Read it back
	readBack, err := ReadFile(fs, "/bin/hello")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// Parse the read-back binary
	hdr, textOut, dataOut, err := ParseUPCFlat(readBack)
	if err != nil {
		t.Fatalf("ParseUPCFlat from filesystem: %v", err)
	}

	if hdr.TextSize != uint32(len(text)) {
		t.Errorf("TextSize = %d, want %d", hdr.TextSize, len(text))
	}
	if hdr.DataSize != uint32(len(data)) {
		t.Errorf("DataSize = %d, want %d", hdr.DataSize, len(data))
	}
	if hdr.BSSSize != 2 {
		t.Errorf("BSSSize = %d, want 2", hdr.BSSSize)
	}
	for i, w := range text {
		if textOut[i] != w {
			t.Errorf("text[%d] = 0x%08X, want 0x%08X", i, textOut[i], w)
		}
	}
	for i, w := range data {
		if dataOut[i] != w {
			t.Errorf("data[%d] = 0x%08X, want 0x%08X", i, dataOut[i], w)
		}
	}
}
