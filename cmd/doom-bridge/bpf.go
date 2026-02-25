// Package main provides BPF map access for the doom-bridge.
// Low-level BPF primitives live in internal/bpf; this file contains
// doom-bridge-specific helpers (screen reads, stats, keyboard, palette).
package main

import (
	"encoding/binary"
	"math"
	"math/rand"
	"sync/atomic"

	bpfpkg "unheaded/internal/bpf"
)

// Import math functions used in dry-run screen generation.
var (
	sin  = math.Sin
	sqrt = math.Sqrt
)

// Type aliases so the rest of doom-bridge doesn't need to change imports.
type BPFMap = bpfpkg.Map
type CpuState = bpfpkg.CpuState

const cpuStateSize = bpfpkg.CpuStateSize

// openBPFMap opens a pinned BPF map at the given path.
var openBPFMap = bpfpkg.OpenMap

// decodeCpuState decodes raw bytes into a CpuState struct.
var decodeCpuState = bpfpkg.DecodeCpuState

// Atomic write index for round-robin KBD_MAP slot selection (0-7).
var kbdWriteIdx atomic.Uint32

// readScreenBatch reads the entire SCREEN_MAP using batch lookup.
// Returns 64000 bytes of pixel data (320x200, 8-bit palette indices).
func readScreenBatch(screenMap *BPFMap) ([]byte, error) {
	const screenSz = 64000
	_, values, count, err := screenMap.LookupBatch(screenSz, 4, 1)
	if err != nil {
		return nil, err
	}
	if count < screenSz {
		// Pad remaining with zeros
		result := make([]byte, screenSz)
		copy(result, values[:count])
		return result, nil
	}
	return values[:screenSz], nil
}

// readScreenIndividual reads the SCREEN_MAP one element at a time.
// Fallback for kernels without batch support.
func readScreenIndividual(screenMap *BPFMap) ([]byte, error) {
	const screenSz = 64000
	pixels := make([]byte, screenSz)
	keyBuf := make([]byte, 4)

	for i := uint32(0); i < screenSz; i++ {
		binary.LittleEndian.PutUint32(keyBuf, i)
		val, err := screenMap.LookupElem(keyBuf, 1)
		if err != nil {
			// On error, leave as zero (black pixel)
			continue
		}
		pixels[i] = val[0]
	}
	return pixels, nil
}

// readStatsMap reads the 4 stats counters from the STATS map.
// Keys: 0=packets_total, 1=cpu_ticks, 2=insns_executed, 3=halted
func readStatsMap(statsMap *BPFMap) (packets, ticks, insns, halted uint64, err error) {
	keyBuf := make([]byte, 4)

	for i, ptr := range []*uint64{&packets, &ticks, &insns, &halted} {
		binary.LittleEndian.PutUint32(keyBuf, uint32(i))
		val, lookupErr := statsMap.LookupElem(keyBuf, 8)
		if lookupErr != nil {
			continue // leave as zero
		}
		*ptr = binary.LittleEndian.Uint64(val)
	}
	return
}

// readCpuMap reads the CPU state from CPU_MAP at key 0xDE (instance ID).
func readCpuMap(cpuMap *BPFMap) (*CpuState, error) {
	keyBuf := make([]byte, 4)
	binary.LittleEndian.PutUint32(keyBuf, 0xDE)

	val, err := cpuMap.LookupElem(keyBuf, cpuStateSize)
	if err != nil {
		return nil, err
	}

	return decodeCpuState(val), nil
}

// writeKbdMap writes a key event to KBD_MAP using round-robin slot selection.
// 8 slots (0-7) allow multiple simultaneous key events.
// Encoding: (scancode << 1) | pressed_flag
func writeKbdMap(kbdMap *BPFMap, scancode uint16, pressed bool) error {
	slot := kbdWriteIdx.Add(1) % 8

	keyBuf := make([]byte, 4)
	binary.LittleEndian.PutUint32(keyBuf, slot)

	var pressedBit uint32
	if pressed {
		pressedBit = 1
	}
	value := (uint32(scancode) << 1) | pressedBit

	valBuf := make([]byte, 4)
	binary.LittleEndian.PutUint32(valBuf, value)

	return kbdMap.UpdateElem(keyBuf, valBuf)
}

// ---------------------------------------------------------------------------
// Doom Palette and Screen Buffer Conversion
// ---------------------------------------------------------------------------

// DoomPaletteRGB is the Doom 256-color palette in RGB format.
// Each entry is 3 bytes (R, G, B). Total: 256 * 3 = 768 bytes.
var DoomPaletteRGB [768]byte

func init() {
	// Color 0-15: Standard VGA colors
	vga16 := [][3]byte{
		{0x00, 0x00, 0x00}, // 0: Black
		{0x00, 0x00, 0xAA}, // 1: Blue
		{0x00, 0xAA, 0x00}, // 2: Green
		{0x00, 0xAA, 0xAA}, // 3: Cyan
		{0xAA, 0x00, 0x00}, // 4: Red
		{0xAA, 0x00, 0xAA}, // 5: Magenta
		{0xAA, 0x55, 0x00}, // 6: Brown
		{0xAA, 0xAA, 0xAA}, // 7: Light Gray
		{0x55, 0x55, 0x55}, // 8: Dark Gray
		{0x55, 0x55, 0xFF}, // 9: Bright Blue
		{0x55, 0xFF, 0x55}, // 10: Bright Green
		{0x55, 0xFF, 0xFF}, // 11: Bright Cyan
		{0xFF, 0x55, 0x55}, // 12: Bright Red
		{0xFF, 0x55, 0xFF}, // 13: Bright Magenta
		{0xFF, 0xFF, 0x55}, // 14: Bright Yellow
		{0xFF, 0xFF, 0xFF}, // 15: White
	}
	for i, c := range vga16 {
		DoomPaletteRGB[i*3] = c[0]
		DoomPaletteRGB[i*3+1] = c[1]
		DoomPaletteRGB[i*3+2] = c[2]
	}

	// Colors 16-47: Gray shades (32 levels)
	for i := 16; i < 48; i++ {
		v := byte((i - 16) * 8)
		DoomPaletteRGB[i*3] = v
		DoomPaletteRGB[i*3+1] = v
		DoomPaletteRGB[i*3+2] = v
	}

	// Colors 48-63: Reds
	for i := 48; i < 64; i++ {
		r := byte(0x88 + (i-48)*8)
		g := byte(0)
		if i > 52 {
			g = byte((i - 52) * 8)
		}
		DoomPaletteRGB[i*3] = r
		DoomPaletteRGB[i*3+1] = g
		DoomPaletteRGB[i*3+2] = 0x00
	}

	// Colors 64-79: Oranges
	for i := 64; i < 80; i++ {
		g := byte(0x68 + (i-64)*8)
		b := byte(0)
		if i > 68 {
			b = byte((i - 68) * 8)
		}
		DoomPaletteRGB[i*3] = 0xFF
		DoomPaletteRGB[i*3+1] = g
		DoomPaletteRGB[i*3+2] = b
	}

	// Colors 80-255: Systematic gradient (greens, cyans, blues, flesh tones)
	for i := 80; i < 256; i++ {
		base := i - 80
		switch {
		case base < 32: // Greens
			DoomPaletteRGB[i*3] = 0
			DoomPaletteRGB[i*3+1] = byte(base * 8)
			DoomPaletteRGB[i*3+2] = 0
		case base < 64: // Cyans
			DoomPaletteRGB[i*3] = 0
			DoomPaletteRGB[i*3+1] = 255
			DoomPaletteRGB[i*3+2] = byte((base - 32) * 8)
		case base < 96: // Blues
			DoomPaletteRGB[i*3] = byte((base - 64) * 4)
			DoomPaletteRGB[i*3+1] = 255
			DoomPaletteRGB[i*3+2] = 255
		default: // Flesh tones & misc
			DoomPaletteRGB[i*3] = byte(min(255, 192+(base-96)/2))
			DoomPaletteRGB[i*3+1] = byte(min(255, 160+(base-96)/3))
			DoomPaletteRGB[i*3+2] = byte(min(255, 128+(base-96)/4))
		}
	}
}

// paletteIndex8ToRGB converts an 8-bit palette index to RGB [3]byte.
func paletteIndex8ToRGB(idx uint8) [3]byte {
	off := int(idx) * 3
	return [3]byte{
		DoomPaletteRGB[off],
		DoomPaletteRGB[off+1],
		DoomPaletteRGB[off+2],
	}
}

// screenBufferToRGBA converts a 320x200 palette-indexed screen buffer
// to RGBA format suitable for HTML5 canvas rendering.
func screenBufferToRGBA(pixels []byte) []byte {
	sz := screenWidth * screenHeight
	if len(pixels) < sz {
		temp := make([]byte, sz)
		copy(temp, pixels)
		pixels = temp
	} else if len(pixels) > sz {
		pixels = pixels[:sz]
	}

	rgba := make([]byte, sz*4)
	for i := 0; i < sz; i++ {
		rgb := paletteIndex8ToRGB(pixels[i])
		rgba[i*4] = rgb[0]   // R
		rgba[i*4+1] = rgb[1] // G
		rgba[i*4+2] = rgb[2] // B
		rgba[i*4+3] = 0xFF   // A (opaque)
	}
	return rgba
}

// min returns the smaller of two ints.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// generateDryRunScreen produces a synthetic screen for testing without BPF maps.
func generateDryRunScreen(frame uint64) []byte {
	pixels := make([]byte, screenSize)
	t := float64(frame) * 0.05

	for y := 0; y < screenHeight; y++ {
		for x := 0; x < screenWidth; x++ {
			fx := float64(x) / float64(screenWidth)
			fy := float64(y) / float64(screenHeight)

			v1 := sin(fx*10.0 + t)
			v2 := sin(fy*8.0 + t*0.7)
			v3 := sin((fx+fy)*6.0 + t*1.3)
			v4 := sin(sqrt(fx*fx+fy*fy)*12.0 + t*0.5)

			v := (v1 + v2 + v3 + v4 + 4.0) / 8.0
			idx := uint8(v * 255.0)

			if y > 160 {
				fireIntensity := float64(y-160) / 40.0
				idx = uint8(float64(idx)*(1.0-fireIntensity) + fireIntensity*float64(32+rand.Intn(16)))
			}

			pixels[y*screenWidth+x] = idx
		}
	}
	return pixels
}
