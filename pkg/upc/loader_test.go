// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package upc

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultBootConfig(t *testing.T) {
	cfg := DefaultBootConfig()
	if cfg.KernelLoadAddr != 0x10000 {
		t.Errorf("KernelLoadAddr = 0x%X, want 0x10000", cfg.KernelLoadAddr)
	}
	if cfg.StackTop != 0x03F00000 {
		t.Errorf("StackTop = 0x%X, want 0x03F00000", cfg.StackTop)
	}
	if cfg.NumProcesses != 1 {
		t.Errorf("NumProcesses = %d, want 1", cfg.NumProcesses)
	}
}

func TestDefaultIVT(t *testing.T) {
	ivt := DefaultIVT()
	if len(ivt) != 256 {
		t.Fatalf("IVT length = %d, want 256", len(ivt))
	}
	for i, entry := range ivt {
		if entry.Vector != uint8(i) {
			t.Errorf("IVT[%d].Vector = %d, want %d", i, entry.Vector, i)
		}
		if entry.Handler != DefaultHandlerAddr {
			t.Errorf("IVT[%d].Handler = 0x%X, want 0x%X", i, entry.Handler, DefaultHandlerAddr)
		}
	}
}

func TestIVTToWords(t *testing.T) {
	ivt := DefaultIVT()
	words := IVTToWords(ivt)
	if len(words) != 256 {
		t.Fatalf("IVTToWords length = %d, want 256", len(words))
	}
	for i, w := range words {
		if w != DefaultHandlerAddr {
			t.Errorf("words[%d] = 0x%X, want 0x%X", i, w, DefaultHandlerAddr)
		}
	}
}

func TestIVTToWordsCustomHandler(t *testing.T) {
	ivt := DefaultIVT()
	// Set a custom handler for the timer interrupt (vector 0x20)
	ivt[0x20].Handler = 0x8000
	words := IVTToWords(ivt)
	if words[0x20] != 0x8000 {
		t.Errorf("words[0x20] = 0x%X, want 0x8000", words[0x20])
	}
	// Other vectors should still point to default
	if words[0x21] != DefaultHandlerAddr {
		t.Errorf("words[0x21] = 0x%X, want 0x%X", words[0x21], DefaultHandlerAddr)
	}
}

func TestLoadKernel(t *testing.T) {
	tmpdir := t.TempDir()
	kernelFile := filepath.Join(tmpdir, "test.bin")

	// Write 3 little-endian u32 words
	instructions := []uint32{0x0F100042, 0x01120005, 0x28000000}
	var buf []byte
	for _, insn := range instructions {
		b := make([]byte, 4)
		binary.LittleEndian.PutUint32(b, insn)
		buf = append(buf, b...)
	}
	if err := os.WriteFile(kernelFile, buf, 0o644); err != nil {
		t.Fatal(err)
	}

	words, err := LoadKernel(kernelFile)
	if err != nil {
		t.Fatalf("LoadKernel: %v", err)
	}
	if len(words) != 3 {
		t.Fatalf("LoadKernel returned %d words, want 3", len(words))
	}
	for i, want := range instructions {
		if words[i] != want {
			t.Errorf("words[%d] = 0x%08X, want 0x%08X", i, words[i], want)
		}
	}
}

func TestLoadKernelPadding(t *testing.T) {
	tmpdir := t.TempDir()
	kernelFile := filepath.Join(tmpdir, "unaligned.bin")

	// Write 5 bytes (not aligned to 4)
	if err := os.WriteFile(kernelFile, []byte{0x01, 0x02, 0x03, 0x04, 0x05}, 0o644); err != nil {
		t.Fatal(err)
	}

	words, err := LoadKernel(kernelFile)
	if err != nil {
		t.Fatalf("LoadKernel: %v", err)
	}
	if len(words) != 2 {
		t.Errorf("LoadKernel returned %d words, want 2 (padded)", len(words))
	}
}

func TestLoadKernelEmpty(t *testing.T) {
	tmpdir := t.TempDir()
	kernelFile := filepath.Join(tmpdir, "empty.bin")
	if err := os.WriteFile(kernelFile, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadKernel(kernelFile)
	if err == nil {
		t.Error("LoadKernel with empty file should return error")
	}
}

func TestLoadKernelNotFound(t *testing.T) {
	_, err := LoadKernel("/nonexistent/kernel.bin")
	if err == nil {
		t.Error("LoadKernel with nonexistent file should return error")
	}
}

func TestPrepareBootMemoryMinimal(t *testing.T) {
	cfg := DefaultBootConfig()
	mem, err := PrepareBootMemory(cfg)
	if err != nil {
		t.Fatalf("PrepareBootMemory: %v", err)
	}

	// Check IVT: vectors 0-63 should point to default handler.
	// Vectors 64+ (word addresses 0x40+) are overwritten by BootParams.
	for i := uint32(0); i < 64; i++ {
		got, ok := mem[i]
		if !ok {
			t.Errorf("IVT[%d] missing from memory map", i)
			continue
		}
		if got != DefaultHandlerAddr {
			t.Errorf("IVT[%d] = 0x%X, want 0x%X", i, got, DefaultHandlerAddr)
		}
	}

	// Check HLT handler at word address 0x100
	if got, ok := mem[DefaultHandlerAddrWord]; !ok || got != HLTOpcode {
		t.Errorf("HLT handler = 0x%X, want 0x%X", got, HLTOpcode)
	}

	// Check boot params magic at word address 0x40
	if got, ok := mem[BootParamsBaseWord]; !ok || got != BootMagic {
		t.Errorf("BootParams magic = 0x%X, want 0x%X", got, BootMagic)
	}

	// Check version at word address 0x41
	if got, ok := mem[BootParamsBaseWord+1]; !ok || got != BootProtocolVersion {
		t.Errorf("BootParams version = %d, want %d", got, BootProtocolVersion)
	}
}

func TestPrepareBootMemoryWithKernel(t *testing.T) {
	tmpdir := t.TempDir()
	kernelFile := filepath.Join(tmpdir, "kernel.bin")

	// Write a small kernel: MOVI r0, 42 ; HLT
	instructions := []uint32{0x0F00002A, HLTOpcode}
	var buf []byte
	for _, insn := range instructions {
		b := make([]byte, 4)
		binary.LittleEndian.PutUint32(b, insn)
		buf = append(buf, b...)
	}
	if err := os.WriteFile(kernelFile, buf, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultBootConfig()
	cfg.KernelPath = kernelFile

	mem, err := PrepareBootMemory(cfg)
	if err != nil {
		t.Fatalf("PrepareBootMemory: %v", err)
	}

	// Kernel should be at word address 0x4000 (byte 0x10000)
	kernelBaseWord := uint32(0x10000 >> 2)
	if got, ok := mem[kernelBaseWord]; !ok || got != 0x0F00002A {
		t.Errorf("kernel word 0 = 0x%08X, want 0x0F00002A", got)
	}
	if got, ok := mem[kernelBaseWord+1]; !ok || got != HLTOpcode {
		t.Errorf("kernel word 1 = 0x%08X, want 0x%08X", got, HLTOpcode)
	}
}

func TestPrepareBootMemoryWithBootArgs(t *testing.T) {
	cfg := DefaultBootConfig()
	cfg.BootArgs = "console=tty0"

	mem, err := PrepareBootMemory(cfg)
	if err != nil {
		t.Fatalf("PrepareBootMemory: %v", err)
	}

	// Boot args addr should be set in params
	// Params word 7 = BootArgsAddr, word 8 = BootArgsLen
	argsAddrWord := uint32(BootParamsBaseWord + 7)
	argsLenWord := uint32(BootParamsBaseWord + 8)
	if got, ok := mem[argsAddrWord]; !ok || got != BootArgsBase {
		t.Errorf("BootArgsAddr = 0x%X, want 0x%X", got, BootArgsBase)
	}
	if got, ok := mem[argsLenWord]; !ok || got != uint32(len("console=tty0")) {
		t.Errorf("BootArgsLen = %d, want %d", got, len("console=tty0"))
	}
}

func TestParamsToWords(t *testing.T) {
	p := BootParams{
		Magic:      BootMagic,
		Version:    1,
		MemorySize: DefaultMemorySize,
	}
	words := paramsToWords(p)
	if words[0] != BootMagic {
		t.Errorf("words[0] = 0x%X, want 0x%X", words[0], BootMagic)
	}
	if words[1] != 1 {
		t.Errorf("words[1] = %d, want 1", words[1])
	}
	// Should have 11 active fields + 20 reserved = 31 words
	if len(words) != 31 {
		t.Errorf("paramsToWords length = %d, want 31", len(words))
	}
}
