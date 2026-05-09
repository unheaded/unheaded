// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package bpf

import (
	"bytes"
	"testing"
)

// ─── CpuStateSize invariant ─────────────────────────────────────────────────

func TestCpuStateSize_PinnedAt128(t *testing.T) {
	t.Parallel()
	// Per source comment + offset comments: total = 128 bytes.
	// MUST match MbcCpuState in ebpf/monad-common/src/lib.rs (and the
	// 136-byte ABI v2 in doom-runner is intentionally a wider replica
	// — see commit 138568df). Pinning here surfaces drift immediately.
	if CpuStateSize != 128 {
		t.Errorf("CpuStateSize = %d, want 128", CpuStateSize)
	}
}

// ─── EncodeCpuState size ────────────────────────────────────────────────────

func TestEncodeCpuState_Returns128Bytes(t *testing.T) {
	t.Parallel()
	cpu := &CpuState{}
	got := EncodeCpuState(cpu)
	if len(got) != CpuStateSize {
		t.Errorf("len(EncodeCpuState) = %d, want %d", len(got), CpuStateSize)
	}
}

// ─── round-trip: Encode → Decode === identity ───────────────────────────────

func TestEncodeDecode_RoundTripsAllFields(t *testing.T) {
	t.Parallel()
	in := &CpuState{
		Regs: [16]uint32{
			0xDEADBEEF, 0xCAFEBABE, 0x12345678, 0x87654321,
			0x00000001, 0xFFFFFFFF, 0x80000000, 0x7FFFFFFF,
			0x55555555, 0xAAAAAAAA, 0x10000000, 0x20000000,
			0x30000000, 0x40000000, 0x50000000, 0x60000000,
		},
		PC:                0x00010000,
		Flags:             0x05,
		Halted:            1,
		Stalled:           0,
		Pad:               0xFF,
		SleepUntil:        0x1122334455667788,
		InsnCount:         0xAABBCCDDEEFF0011,
		CacheHits:         12345,
		CacheMisses:       67890,
		InterruptPending:  1,
		InterruptVector:   42,
		InterruptsEnabled: 1,
		Pad2:              0,
		TickCounter:       9999,
		ProgramBreak:      0x00400000,
		ExitCode:          0xCAFE,
		CurrentPID:        7,
		NumProcesses:      3,
		MmuEnabled:        1,
		Pad3:              0,
		PageDirBase:       0x10000000,
	}
	encoded := EncodeCpuState(in)
	out := DecodeCpuState(encoded)

	if *out != *in {
		t.Errorf("round-trip mismatch:\n  in:  %+v\n  out: %+v", in, out)
	}
}

// ─── decode short input: zero-pads ──────────────────────────────────────────

func TestDecodeCpuState_ShortInputZeroPads(t *testing.T) {
	t.Parallel()
	// Per impl: short data is copied into a CpuStateSize buffer (zero
	// padding the trailing bytes). Verify a 64-byte input gives an
	// all-zero PC/Flags/etc.
	short := make([]byte, 64) // just registers, no PC/Flags/etc.
	for i := 0; i < 64; i++ {
		short[i] = byte(i)
	}
	cpu := DecodeCpuState(short)
	if cpu == nil {
		t.Fatal("nil result")
	}
	if cpu.PC != 0 {
		t.Errorf("PC should be zero-padded, got 0x%X", cpu.PC)
	}
	if cpu.Flags != 0 {
		t.Errorf("Flags should be zero-padded, got 0x%X", cpu.Flags)
	}
	if cpu.SleepUntil != 0 {
		t.Errorf("SleepUntil should be zero-padded, got 0x%X", cpu.SleepUntil)
	}
}

// ─── decode empty input: all zeros ──────────────────────────────────────────

func TestDecodeCpuState_EmptyInputAllZeros(t *testing.T) {
	t.Parallel()
	cpu := DecodeCpuState(nil)
	if cpu == nil {
		t.Fatal("nil result on nil input")
	}
	for i, r := range cpu.Regs {
		if r != 0 {
			t.Errorf("Regs[%d] = 0x%X, want 0", i, r)
		}
	}
	if cpu.PC != 0 || cpu.SleepUntil != 0 || cpu.InsnCount != 0 {
		t.Errorf("expected all-zero CpuState, got %+v", cpu)
	}
}

// ─── encode produces little-endian for u32/u64 ─────────────────────────────

func TestEncodeCpuState_RegsAreLittleEndian(t *testing.T) {
	t.Parallel()
	cpu := &CpuState{}
	cpu.Regs[0] = 0x11223344
	encoded := EncodeCpuState(cpu)
	want := []byte{0x44, 0x33, 0x22, 0x11}
	if !bytes.Equal(encoded[0:4], want) {
		t.Errorf("Regs[0] LE encoding = %v, want %v", encoded[0:4], want)
	}
}

func TestEncodeCpuState_PCAtOffset64(t *testing.T) {
	t.Parallel()
	cpu := &CpuState{PC: 0xCAFEBABE}
	encoded := EncodeCpuState(cpu)
	want := []byte{0xBE, 0xBA, 0xFE, 0xCA}
	if !bytes.Equal(encoded[64:68], want) {
		t.Errorf("PC at offset 64 = %v, want %v", encoded[64:68], want)
	}
}

func TestEncodeCpuState_SleepUntilAtOffset72(t *testing.T) {
	t.Parallel()
	cpu := &CpuState{SleepUntil: 0x0102030405060708}
	encoded := EncodeCpuState(cpu)
	want := []byte{0x08, 0x07, 0x06, 0x05, 0x04, 0x03, 0x02, 0x01}
	if !bytes.Equal(encoded[72:80], want) {
		t.Errorf("SleepUntil at offset 72 = %v, want %v", encoded[72:80], want)
	}
}
