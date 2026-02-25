// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package main

import (
	"testing"
)

func TestPaletteIndex8ToRGB_Black(t *testing.T) {
	got := paletteIndex8ToRGB(0)
	want := [3]byte{0x00, 0x00, 0x00}
	if got != want {
		t.Errorf("paletteIndex8ToRGB(0) = %v, want %v", got, want)
	}
}

func TestPaletteIndex8ToRGB_White(t *testing.T) {
	got := paletteIndex8ToRGB(15)
	want := [3]byte{0xFF, 0xFF, 0xFF}
	if got != want {
		t.Errorf("paletteIndex8ToRGB(15) = %v, want %v", got, want)
	}
}

func TestPaletteIndex8ToRGB_Red(t *testing.T) {
	got := paletteIndex8ToRGB(4)
	want := [3]byte{0xAA, 0x00, 0x00}
	if got != want {
		t.Errorf("paletteIndex8ToRGB(4) = %v, want %v", got, want)
	}
}

func TestPaletteIndex8ToRGB_Blue(t *testing.T) {
	got := paletteIndex8ToRGB(1)
	want := [3]byte{0x00, 0x00, 0xAA}
	if got != want {
		t.Errorf("paletteIndex8ToRGB(1) = %v, want %v", got, want)
	}
}

func TestPaletteIndex8ToRGB_AllIndices(t *testing.T) {
	// Ensure no index panics
	for i := 0; i < 256; i++ {
		rgb := paletteIndex8ToRGB(uint8(i))
		// Just verify no out-of-bounds access
		_ = rgb[0]
		_ = rgb[1]
		_ = rgb[2]
	}
}

func TestScreenBufferToRGBA_AllBlack(t *testing.T) {
	pixels := make([]byte, screenSize)
	// All zeros = all black

	rgba := screenBufferToRGBA(pixels)

	expectedLen := screenWidth * screenHeight * 4
	if len(rgba) != expectedLen {
		t.Fatalf("screenBufferToRGBA returned %d bytes, want %d", len(rgba), expectedLen)
	}

	// Check first pixel is black (RGBA 0,0,0,255)
	if rgba[0] != 0 || rgba[1] != 0 || rgba[2] != 0 || rgba[3] != 255 {
		t.Errorf("First pixel = [%d,%d,%d,%d], want [0,0,0,255]",
			rgba[0], rgba[1], rgba[2], rgba[3])
	}

	// Check all alpha bytes are 0xFF
	for i := 0; i < screenWidth*screenHeight; i++ {
		if rgba[i*4+3] != 0xFF {
			t.Errorf("Pixel %d alpha = %d, want 255", i, rgba[i*4+3])
			break
		}
	}
}

func TestScreenBufferToRGBA_White(t *testing.T) {
	pixels := make([]byte, screenSize)
	for i := range pixels {
		pixels[i] = 15 // White
	}

	rgba := screenBufferToRGBA(pixels)

	// First pixel should be white (255, 255, 255, 255)
	if rgba[0] != 0xFF || rgba[1] != 0xFF || rgba[2] != 0xFF || rgba[3] != 0xFF {
		t.Errorf("White pixel = [%d,%d,%d,%d], want [255,255,255,255]",
			rgba[0], rgba[1], rgba[2], rgba[3])
	}
}

func TestScreenBufferToRGBA_ShortInput(t *testing.T) {
	// Test with short input (should be padded with zeros = black)
	pixels := make([]byte, 100)
	for i := range pixels {
		pixels[i] = 15 // White
	}

	rgba := screenBufferToRGBA(pixels)

	expectedLen := screenWidth * screenHeight * 4
	if len(rgba) != expectedLen {
		t.Fatalf("screenBufferToRGBA returned %d bytes, want %d", len(rgba), expectedLen)
	}

	// First 100 pixels should be white
	if rgba[0] != 0xFF || rgba[1] != 0xFF || rgba[2] != 0xFF {
		t.Errorf("First pixel should be white, got [%d,%d,%d]", rgba[0], rgba[1], rgba[2])
	}

	// Pixel 100 should be black (from zero padding)
	if rgba[100*4] != 0 || rgba[100*4+1] != 0 || rgba[100*4+2] != 0 {
		t.Errorf("Pixel 100 should be black, got [%d,%d,%d]",
			rgba[100*4], rgba[100*4+1], rgba[100*4+2])
	}
}

func TestScreenBufferToRGBA_LongInput(t *testing.T) {
	// Test with overly long input (should be truncated)
	pixels := make([]byte, screenSize+1000)
	for i := range pixels {
		pixels[i] = 4 // Red
	}

	rgba := screenBufferToRGBA(pixels)

	expectedLen := screenWidth * screenHeight * 4
	if len(rgba) != expectedLen {
		t.Fatalf("screenBufferToRGBA returned %d bytes, want %d", len(rgba), expectedLen)
	}
}

func TestDecodeCpuState(t *testing.T) {
	// Create a 104-byte buffer with known values
	data := make([]byte, cpuStateSize)

	// Set register 0 to 0x12345678
	data[0] = 0x78
	data[1] = 0x56
	data[2] = 0x34
	data[3] = 0x12

	// Set PC (offset 64) to 0xABCD
	data[64] = 0xCD
	data[65] = 0xAB
	data[66] = 0x00
	data[67] = 0x00

	// Set flags (offset 68)
	data[68] = 0x03

	// Set halted (offset 69)
	data[69] = 0x01

	cpu := decodeCpuState(data)

	if cpu.Regs[0] != 0x12345678 {
		t.Errorf("Regs[0] = 0x%08X, want 0x12345678", cpu.Regs[0])
	}
	if cpu.PC != 0xABCD {
		t.Errorf("PC = 0x%X, want 0xABCD", cpu.PC)
	}
	if cpu.Flags != 0x03 {
		t.Errorf("Flags = 0x%X, want 0x03", cpu.Flags)
	}
	if cpu.Halted != 0x01 {
		t.Errorf("Halted = %d, want 1", cpu.Halted)
	}
}

func TestDecodeCpuState_ShortInput(t *testing.T) {
	// Test with short input (should pad with zeros, not panic)
	data := make([]byte, 10)
	data[0] = 0x42

	cpu := decodeCpuState(data)

	if cpu.Regs[0] != 0x42 {
		t.Errorf("Regs[0] = 0x%X, want 0x42", cpu.Regs[0])
	}
	if cpu.PC != 0 {
		t.Errorf("PC = 0x%X, want 0x0", cpu.PC)
	}
}

func TestGenerateDryRunScreen(t *testing.T) {
	pixels := generateDryRunScreen(0)

	if len(pixels) != screenSize {
		t.Fatalf("generateDryRunScreen returned %d bytes, want %d", len(pixels), screenSize)
	}

	// Second frame should differ from first (animated)
	pixels2 := generateDryRunScreen(10)
	same := true
	for i := range pixels {
		if pixels[i] != pixels2[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("Frames 0 and 10 are identical; animation not working")
	}
}

func BenchmarkScreenBufferToRGBA(b *testing.B) {
	pixels := make([]byte, screenSize)
	for i := range pixels {
		pixels[i] = byte(i & 0xFF)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = screenBufferToRGBA(pixels)
	}
}

func BenchmarkGenerateDryRunScreen(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = generateDryRunScreen(uint64(i))
	}
}
