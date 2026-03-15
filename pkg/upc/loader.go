// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package upc

import (
	"encoding/binary"
	"fmt"
	"os"
)

// LoadKernel reads a kernel binary and returns it as u32 words for RAM_MAP.
// The file is padded to 4-byte alignment if necessary.
func LoadKernel(path string) ([]uint32, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read kernel: %w", err)
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("kernel file is empty: %s", path)
	}

	// Pad to 4-byte alignment
	for len(data)%4 != 0 {
		data = append(data, 0)
	}

	words := make([]uint32, len(data)/4)
	for i := range words {
		words[i] = binary.LittleEndian.Uint32(data[i*4 : (i+1)*4])
	}

	return words, nil
}

// LoadRamdisk reads a ramdisk image and returns it as u32 words.
func LoadRamdisk(path string) ([]uint32, error) {
	return LoadKernel(path) // Same binary format
}

// PrepareBootMemory sets up the initial RAM state for booting.
// Returns a map of word_address -> value to write to RAM_MAP.
func PrepareBootMemory(cfg BootConfig) (map[uint32]uint32, error) {
	mem := make(map[uint32]uint32)

	// 1. Write IVT at word addresses 0x0000-0x00FF
	ivt := IVTToWords(DefaultIVT())
	for i, word := range ivt {
		mem[uint32(i)] = word
	}

	// 2. Write default HLT handler at word address 0x100 (byte 0x0400)
	mem[DefaultHandlerAddrWord] = HLTOpcode

	// 3. Build boot params
	params := BootParams{
		Magic:      BootMagic,
		Version:    BootProtocolVersion,
		MemorySize: DefaultMemorySize,
		KernelAddr: cfg.KernelLoadAddr,
		TickRateHz: DefaultTickRateHz,
		NumCPUs:    1,
	}

	// 4. Load kernel if specified
	if cfg.KernelPath != "" {
		kernelWords, err := LoadKernel(cfg.KernelPath)
		if err != nil {
			return nil, err
		}
		params.KernelSize = uint32(len(kernelWords) * 4)
		baseWord := cfg.KernelLoadAddr >> 2
		for i, word := range kernelWords {
			mem[baseWord+uint32(i)] = word
		}
	}

	// 5. Load ramdisk if specified
	if cfg.RamdiskPath != "" {
		rdWords, err := LoadRamdisk(cfg.RamdiskPath)
		if err != nil {
			return nil, err
		}
		params.RamdiskSize = uint32(len(rdWords) * 4)
		params.RamdiskAddr = cfg.RamdiskAddr
		baseWord := cfg.RamdiskAddr >> 2
		for i, word := range rdWords {
			mem[baseWord+uint32(i)] = word
		}
	}

	// 6. Store boot args if specified
	if cfg.BootArgs != "" {
		params.BootArgsAddr = BootArgsBase
		params.BootArgsLen = uint32(len(cfg.BootArgs))
		argsBytes := []byte(cfg.BootArgs)
		// Pad to 4-byte alignment
		for len(argsBytes)%4 != 0 {
			argsBytes = append(argsBytes, 0)
		}
		for i := 0; i < len(argsBytes)/4; i++ {
			word := binary.LittleEndian.Uint32(argsBytes[i*4 : (i+1)*4])
			mem[BootArgsBaseWord+uint32(i)] = word
		}
	}

	// 7. Serialize boot params to words at 0x40-0x7F
	paramWords := paramsToWords(params)
	for i, word := range paramWords {
		mem[BootParamsBaseWord+uint32(i)] = word
	}

	return mem, nil
}

// paramsToWords serializes BootParams to a slice of u32 words.
// Only the active fields are serialized (not the Reserved block).
func paramsToWords(p BootParams) []uint32 {
	words := []uint32{
		p.Magic, p.Version, p.MemorySize,
		p.RamdiskAddr, p.RamdiskSize,
		p.KernelAddr, p.KernelSize,
		p.BootArgsAddr, p.BootArgsLen,
		p.NumCPUs, p.TickRateHz,
	}
	// Pad with reserved zeros to fill the params region
	for i := 0; i < len(p.Reserved); i++ {
		words = append(words, 0)
	}
	return words
}
