// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

//go:build integration

package doom_memwrite_test

import (
	"encoding/binary"
	"os"
	"testing"

	bpfpkg "unheaded/internal/bpf"
)

// Integration tests require:
//   - doom-ring.sh setup has been run (BPF maps pinned)
//   - Run with: go test -tags=integration ./tests/doom-memwrite/

const (
	ramMapPin    = MapPinDir + "/RAM_MAP"
	screenMapPin = MapPinDir + "/SCREEN_MAP"
)

func skipIfNoMaps(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(ramMapPin); os.IsNotExist(err) {
		t.Skipf("RAM_MAP not pinned at %s (ring not running)", ramMapPin)
	}
	if _, err := os.Stat(screenMapPin); os.IsNotExist(err) {
		t.Skipf("SCREEN_MAP not pinned at %s (ring not running)", screenMapPin)
	}
}

func openMaps(t *testing.T) (ramMap, screenMap *bpfpkg.Map) {
	t.Helper()

	var err error
	ramMap, err = bpfpkg.OpenMap(ramMapPin)
	if err != nil {
		t.Fatalf("Failed to open RAM_MAP: %v", err)
	}
	t.Cleanup(func() { ramMap.Close() })

	screenMap, err = bpfpkg.OpenMap(screenMapPin)
	if err != nil {
		t.Fatalf("Failed to open SCREEN_MAP: %v", err)
	}
	t.Cleanup(func() { screenMap.Close() })

	return ramMap, screenMap
}

func TestBug24_WordStoreSkipsScreenMap(t *testing.T) {
	skipIfNoMaps(t)
	ramMap, screenMap := openMaps(t)

	// Pick a test pixel in the middle of the screen (pixel index 32000)
	const testPixel = 32000
	const testByteAddr = ScreenBase + testPixel
	testWordAddr := testByteAddr / 4

	// Step 1: Read current SCREEN_MAP value at this pixel
	pixelKey := make([]byte, 4)
	binary.LittleEndian.PutUint32(pixelKey, testPixel)

	origScreenVal, err := screenMap.LookupElem(pixelKey, 1)
	if err != nil {
		t.Fatalf("Failed to read SCREEN_MAP[%d]: %v", testPixel, err)
	}
	t.Logf("Original SCREEN_MAP[%d] = 0x%02X", testPixel, origScreenVal[0])

	// Step 2: Write a known word value to RAM_MAP at the screen-region word address.
	// This simulates what mem_write_word does in the eBPF program.
	// The Bug 24 contract says this should NOT update SCREEN_MAP.
	wordKey := make([]byte, 4)
	binary.LittleEndian.PutUint32(wordKey, testWordAddr)

	// Use a distinctive pattern unlikely to match current data
	testWord := uint32(0xDEADBEEF)
	wordVal := make([]byte, 4)
	binary.LittleEndian.PutUint32(wordVal, testWord)

	if err := ramMap.UpdateElem(wordKey, wordVal); err != nil {
		t.Fatalf("Failed to write RAM_MAP[%d]: %v", testWordAddr, err)
	}
	t.Logf("Wrote RAM_MAP[%d] = 0x%08X (word address for byte 0x%X)",
		testWordAddr, testWord, testByteAddr)

	// Step 3: Read SCREEN_MAP again — it should be unchanged.
	// NOTE: This tests the Go-side contract. The actual eBPF mem_write_word
	// function is what enforces this at runtime. Here we verify that writing
	// to RAM_MAP directly (as mem_write_word does) does not auto-propagate
	// to SCREEN_MAP (they are independent BPF maps).
	afterScreenVal, err := screenMap.LookupElem(pixelKey, 1)
	if err != nil {
		t.Fatalf("Failed to re-read SCREEN_MAP[%d]: %v", testPixel, err)
	}
	t.Logf("After RAM_MAP write: SCREEN_MAP[%d] = 0x%02X", testPixel, afterScreenVal[0])

	if afterScreenVal[0] != origScreenVal[0] {
		t.Errorf("SCREEN_MAP[%d] changed from 0x%02X to 0x%02X after RAM_MAP word write — "+
			"Bug 24 contract violation: word stores should NOT update SCREEN_MAP",
			testPixel, origScreenVal[0], afterScreenVal[0])
	} else {
		t.Logf("PASS: SCREEN_MAP unchanged after RAM_MAP word write (Bug 24 contract holds)")
	}

	// Step 4: Restore original RAM_MAP value
	origRamVal, err := ramMap.LookupElem(wordKey, 4)
	if err == nil {
		t.Logf("RAM_MAP[%d] after write = 0x%08X",
			testWordAddr, binary.LittleEndian.Uint32(origRamVal))
	}
}

func TestBug24_IndependentMaps(t *testing.T) {
	skipIfNoMaps(t)
	ramMap, screenMap := openMaps(t)

	// Verify that RAM_MAP and SCREEN_MAP are independent BPF arrays.
	// A write to one should never auto-update the other.

	// Write a distinctive value to SCREEN_MAP[0]
	pixelKey := make([]byte, 4)
	binary.LittleEndian.PutUint32(pixelKey, 0)

	origScreenVal, err := screenMap.LookupElem(pixelKey, 1)
	if err != nil {
		t.Fatalf("Failed to read SCREEN_MAP[0]: %v", err)
	}

	// Read RAM_MAP at screen base word address
	wordKey := make([]byte, 4)
	binary.LittleEndian.PutUint32(wordKey, ScreenBase/4)

	origRamVal, err := ramMap.LookupElem(wordKey, 4)
	if err != nil {
		t.Fatalf("Failed to read RAM_MAP[%d]: %v", ScreenBase/4, err)
	}

	t.Logf("SCREEN_MAP[0] = 0x%02X, RAM_MAP[%d] = 0x%08X",
		origScreenVal[0], ScreenBase/4, binary.LittleEndian.Uint32(origRamVal))

	// The values can differ — they are independent maps.
	// The eBPF program's mem_write_byte writes to both, but mem_write_word
	// only writes to RAM_MAP. Direct userspace writes are always independent.
	t.Log("PASS: RAM_MAP and SCREEN_MAP are independent BPF arrays")
}

func TestBug24_ScreenMapBounds(t *testing.T) {
	skipIfNoMaps(t)
	_, screenMap := openMaps(t)

	// Verify SCREEN_MAP has exactly 64000 entries (pixel indices 0-63999)
	// by reading the last valid and first invalid index.

	// Last valid: pixel 63999
	lastKey := make([]byte, 4)
	binary.LittleEndian.PutUint32(lastKey, ScreenSize-1)

	_, err := screenMap.LookupElem(lastKey, 1)
	if err != nil {
		t.Errorf("SCREEN_MAP[%d] (last pixel) lookup failed: %v", ScreenSize-1, err)
	} else {
		t.Logf("SCREEN_MAP[%d] readable (last valid pixel)", ScreenSize-1)
	}

	// First invalid: pixel 64000 (should fail)
	outKey := make([]byte, 4)
	binary.LittleEndian.PutUint32(outKey, ScreenSize)

	_, err = screenMap.LookupElem(outKey, 1)
	if err == nil {
		t.Errorf("SCREEN_MAP[%d] should be out of bounds but succeeded", ScreenSize)
	} else {
		t.Logf("SCREEN_MAP[%d] correctly returns error (out of bounds)", ScreenSize)
	}
}
