// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package doom

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
)

// ROM holds a parsed MBC ROM image (sequence of 32-bit instruction words).
type ROM struct {
	// Instructions is the sequence of little-endian u32 MBC instruction words.
	Instructions []uint32
	// RawBytes is the original file contents.
	RawBytes []byte
}

// LoadROM reads an MBC binary file from disk, validates its size, and returns
// the parsed ROM. The file must contain a sequence of little-endian u32
// instruction words (size must be a multiple of 4, max 1 MiB).
func LoadROM(path string) (*ROM, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- build/ROM artifact path from a configured or CLI-supplied location
	if err != nil {
		return nil, fmt.Errorf("read ROM file: %w", err)
	}

	return ParseROM(data)
}

// ParseROM validates and parses raw bytes into a ROM.
func ParseROM(data []byte) (*ROM, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("ROM file is empty")
	}

	if len(data)%4 != 0 {
		return nil, fmt.Errorf("ROM size %d is not a multiple of 4 bytes", len(data))
	}

	if len(data) > MaxROMBytes {
		return nil, fmt.Errorf("ROM size %d exceeds maximum %d bytes (1 MiB)", len(data), MaxROMBytes)
	}

	instrCount := len(data) / 4
	instructions := make([]uint32, instrCount)
	for i := range instructions {
		instructions[i] = binary.LittleEndian.Uint32(data[i*4 : i*4+4])
	}

	return &ROM{
		Instructions: instructions,
		RawBytes:     data,
	}, nil
}

// InstructionCount returns the number of instructions in the ROM.
func (r *ROM) InstructionCount() int {
	return len(r.Instructions)
}

// SizeBytes returns the size of the ROM in bytes.
func (r *ROM) SizeBytes() int {
	return len(r.RawBytes)
}

// HMAC computes HMAC-SHA256 of the ROM bytes using the given key.
func (r *ROM) HMAC(key []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(r.RawBytes)
	return mac.Sum(nil)
}

// VerifyHMAC verifies the HMAC-SHA256 of the ROM bytes against an expected digest.
func (r *ROM) VerifyHMAC(key, expectedMAC []byte) bool {
	actualMAC := r.HMAC(key)
	return hmac.Equal(actualMAC, expectedMAC)
}

// Summary returns a human-readable summary of the ROM.
func (r *ROM) Summary() string {
	sizeKB := float64(r.SizeBytes()) / 1024.0
	estSeconds := float64(r.InstructionCount()) / 2_000_000.0 // 2 MHz effective clock
	return fmt.Sprintf("Instructions: %d, Size: %d bytes (%.1f KB), Est. runtime @ 2MHz: %.3fs",
		r.InstructionCount(), r.SizeBytes(), sizeKB, estSeconds)
}

// WriteToMap writes ROM instructions to a MapAccessor (rom_map).
// Each instruction is stored at its index as a 32-bit LE value.
func (r *ROM) WriteToMap(m MapAccessor) error {
	keyBuf := make([]byte, 4)
	valBuf := make([]byte, 4)

	for i, insn := range r.Instructions {
		binary.LittleEndian.PutUint32(keyBuf, uint32(i))
		binary.LittleEndian.PutUint32(valBuf, insn)
		if err := m.UpdateElem(keyBuf, valBuf); err != nil {
			return fmt.Errorf("rom_map update at index %d: %w", i, err)
		}
	}
	return nil
}
