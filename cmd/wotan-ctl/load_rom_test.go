// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"unheaded/pkg/logger"
)

// TestDisassembleMBC tests the MBC instruction disassembler.
func TestDisassembleMBC(t *testing.T) {
	tests := []struct {
		name     string
		insn     uint32
		contains string
	}{
		{
			name:     "ADD instruction",
			insn:     0x01_1_2_0000, // opcode=0x01, dst=1, src=2, imm=0
			contains: "ADD",
		},
		{
			name:     "MOVI instruction",
			insn:     0x0F_5_0_0100, // opcode=0x0F, dst=5, src=0, imm=0x0100
			contains: "MOVI",
		},
		{
			name:     "JMP instruction",
			insn:     0x20_0_0_0042, // opcode=0x20, dst=0, src=0, imm=0x0042
			contains: "JMP",
		},
		{
			name:     "SYSCALL DRAW_FRAME",
			insn:     0x40_0_0_0001, // opcode=0x40, imm=0x0001 (DRAW_FRAME)
			contains: "DRAW_FRAME",
		},
		{
			name:     "RET instruction",
			insn:     0x28_0_0_0000, // opcode=0x28 (RET)
			contains: "RET",
		},
		{
			name:     "MOV instruction",
			insn:     0x0E_3_7_0000, // opcode=0x0E, dst=3, src=7
			contains: "MOV",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := disassembleMBC(tt.insn)
			if !bytes.Contains([]byte(got), []byte(tt.contains)) {
				t.Errorf("disassembleMBC(0x%08X) = %q, want to contain %q", tt.insn, got, tt.contains)
			}
		})
	}
}

// TestDefaultCpuState tests the default CPU state initialization.
func TestDefaultCpuState(t *testing.T) {
	cpu := defaultCpuState(0x123456)

	if cpu.PC != 0 {
		t.Errorf("PC = %d, want 0", cpu.PC)
	}

	if cpu.Regs[15] != 0xFFFF_0000 {
		t.Errorf("Regs[15] = 0x%08X, want 0xFFFF0000", cpu.Regs[15])
	}

	if cpu.Flags != 0 {
		t.Errorf("Flags = %d, want 0", cpu.Flags)
	}
}

// TestLoadRomDryRun tests loading ROM in dry-run mode (no BPF).
func TestLoadRomDryRun(t *testing.T) {
	// Create a temporary .mbc file with 3 instructions
	tmpdir := t.TempDir()
	mbcFile := filepath.Join(tmpdir, "test.mbc")

	// Write 3 little-endian u32 instructions
	instructions := []uint32{
		0x0F_1_0_0000, // MOVI r1, 0
		0x01_1_2_0005, // ADD r1, r2, 5
		0x28_0_0_0000, // RET
	}

	var buf bytes.Buffer
	for _, insn := range instructions {
		binary.Write(&buf, binary.LittleEndian, insn)
	}

	if err := os.WriteFile(mbcFile, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Create test context
	ctx := &Context{
		MapPinDir: "/nonexistent", // Non-existent BPF pin directory
		Logger:    logger.New(os.Stderr),
	}

	// Run load-rom in dry-run mode (should succeed)
	err := loadRom(ctx, 0x123456, mbcFile, "/nonexistent", true, false, false)
	if err != nil {
		t.Errorf("loadRom: %v", err)
	}
}

// TestEncodeValue tests the encodeValue helper function.
func TestEncodeValue(t *testing.T) {
	tests := []struct {
		name string
		val  interface{}
		want []byte
	}{
		{
			name: "uint32",
			val:  uint32(0x12345678),
			want: []byte{0x78, 0x56, 0x34, 0x12}, // little-endian
		},
		{
			name: "uint64",
			val:  uint64(0x0102030405060708),
			want: []byte{0x08, 0x07, 0x06, 0x05, 0x04, 0x03, 0x02, 0x01}, // little-endian
		},
		{
			name: "int32",
			val:  int32(-1),
			want: []byte{0xFF, 0xFF, 0xFF, 0xFF}, // -1 as signed int32
		},
		{
			name: "byte slice",
			val:  []byte{0x01, 0x02, 0x03},
			want: []byte{0x01, 0x02, 0x03},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := encodeValue(tt.val)
			if err != nil {
				t.Fatalf("encodeValue: %v", err)
			}

			if !bytes.Equal(got, tt.want) {
				t.Errorf("encodeValue(%v) = %v, want %v", tt.val, got, tt.want)
			}
		})
	}
}

// TestEncodeValueErrors tests error cases for encodeValue.
func TestEncodeValueErrors(t *testing.T) {
	_, err := encodeValue(struct{}{})
	if err == nil {
		t.Errorf("encodeValue(struct{}) should return error, got nil")
	}
}

// BenchmarkDisassembleMBC benchmarks the disassembler.
func BenchmarkDisassembleMBC(b *testing.B) {
	insn := uint32(0x01_1_2_0005)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		disassembleMBC(insn)
	}
}
