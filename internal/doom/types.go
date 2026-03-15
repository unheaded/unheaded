// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package doom provides Go-side helpers for the Doom-over-IPv6 compute engine.
//
// It defines Go types that match the Rust #[repr(C)] structs from
// ebpf/monad-common/src/lib.rs, and provides ROM loading, CPU state
// management, keyboard input injection, and CLI integration.
//
// BPF map interactions are abstracted behind the MapAccessor interface
// so that tests can use mock implementations without requiring real
// BPF maps.
package doom

import (
	"encoding/binary"
	"fmt"
)

// CpuStateSize is the expected binary size of MbcCpuState.
// This MUST match the Rust #[repr(C)] struct MbcCpuState in
// ebpf/monad-common/src/lib.rs (128 bytes).
const CpuStateSize = 128

// RegCount is the number of general-purpose registers (r0-r15).
const RegCount = 16

// RegSP is the stack pointer register index (r15).
const RegSP = 15

// RegSPDefault is the default stack pointer value (top of RAM).
const RegSPDefault uint32 = 0xFFFF_0000

// RegPCDefault is the default program counter (start of ROM).
const RegPCDefault uint32 = 0

// MaxROMInstructions is the maximum number of 32-bit instruction words
// that can be loaded into rom_map (262144 entries = 1 MiB).
const MaxROMInstructions = 262144

// MaxROMBytes is MaxROMInstructions * 4.
const MaxROMBytes = MaxROMInstructions * 4

// CpuState mirrors the Rust MbcCpuState struct from ebpf/monad-common/src/lib.rs.
//
// Field order and sizes MUST match the Rust #[repr(C)] layout exactly:
//
//	regs:               [u32; 16]  = 64 bytes  (offset 0)
//	pc:                 u32        = 4 bytes   (offset 64)
//	flags:              u8         = 1 byte    (offset 68)
//	halted:             u8         = 1 byte    (offset 69)
//	stalled:            u8         = 1 byte    (offset 70)
//	_pad:               u8         = 1 byte    (offset 71)
//	sleep_until_ns:     u64        = 8 bytes   (offset 72)
//	insn_count:         u64        = 8 bytes   (offset 80)
//	cache_hits:         u64        = 8 bytes   (offset 88)
//	cache_misses:       u64        = 8 bytes   (offset 96)
//	interrupt_pending:  u8         = 1 byte    (offset 104)
//	interrupt_vector:   u8         = 1 byte    (offset 105)
//	interrupts_enabled: u8         = 1 byte    (offset 106)
//	_pad2:              u8         = 1 byte    (offset 107)
//	tick_counter:       u32        = 4 bytes   (offset 108)
//	program_break:      u32        = 4 bytes   (offset 112)
//	exit_code:          u32        = 4 bytes   (offset 116)
//	current_pid:        u8         = 1 byte    (offset 120)
//	num_processes:      u8         = 1 byte    (offset 121)
//	_pad3:              [u8; 6]    = 6 bytes   (offset 122)
//	TOTAL                          = 128 bytes
type CpuState struct {
	Regs              [RegCount]uint32 // General purpose registers r0-r15. r15 = SP.
	PC                uint32           // Program counter (index into rom_map).
	Flags             uint8            // Status flags: bit 0=Z, bit 1=N, bit 2=C.
	Halted            uint8            // 1 if HALT instruction executed.
	Stalled           uint8            // 1 if waiting for cache miss to be serviced.
	Pad               uint8            // Padding byte for alignment.
	SleepUntil        uint64           // bpf_ktime_get_ns() value; sleep if < now.
	InsnCount         uint64           // Total instructions executed (for IPS stats).
	CacheHits         uint64           // L1 cache hits.
	CacheMisses       uint64           // L1 cache misses.
	InterruptPending  uint8            // Non-zero when interrupt waiting.
	InterruptVector   uint8            // Vector number of pending interrupt.
	InterruptsEnabled uint8            // Non-zero when CPU accepts interrupts.
	Pad2              uint8            // Padding for alignment.
	TickCounter       uint32           // Tick counter for timer interrupt generation.
	ProgramBreak      uint32           // Heap end address for SYS_BRK.
	ExitCode          uint32           // Exit code from SYS_EXIT.
	CurrentPID        uint8            // Current process ID (0-3). Level 4c scheduler.
	NumProcesses      uint8            // Number of active processes. Level 4c scheduler.
	Pad3              [6]uint8         // Alignment padding.
}

// CPU flag bit positions — must match mbc_flags in monad-common/src/lib.rs.
const (
	FlagZero     uint8 = 0x01 // Z: set when last ALU result was zero.
	FlagNegative uint8 = 0x02 // N: set when MSB of last result is 1.
	FlagCarry    uint8 = 0x04 // C: set on unsigned overflow/borrow.
)

// KeyboardState mirrors the Rust KeyboardState struct.
//
//	key:      u32  = 4 bytes
//	pressed:  u32  = 4 bytes
//	sequence: u64  = 8 bytes
//	TOTAL          = 16 bytes
type KeyboardState struct {
	Key      uint32 // Keycode.
	Pressed  uint32 // 1=pressed, 0=released.
	Sequence uint64 // Monotonically increasing; BPF checks if changed.
}

// KeyboardStateSize is the binary size of KeyboardState.
const KeyboardStateSize = 16

// MarshalBinary serializes CpuState to a 120-byte little-endian representation.
func (c *CpuState) MarshalBinary() []byte {
	buf := make([]byte, CpuStateSize)
	for i := 0; i < RegCount; i++ {
		binary.LittleEndian.PutUint32(buf[i*4:i*4+4], c.Regs[i])
	}
	binary.LittleEndian.PutUint32(buf[64:68], c.PC)
	buf[68] = c.Flags
	buf[69] = c.Halted
	buf[70] = c.Stalled
	buf[71] = c.Pad
	binary.LittleEndian.PutUint64(buf[72:80], c.SleepUntil)
	binary.LittleEndian.PutUint64(buf[80:88], c.InsnCount)
	binary.LittleEndian.PutUint64(buf[88:96], c.CacheHits)
	binary.LittleEndian.PutUint64(buf[96:104], c.CacheMisses)
	buf[104] = c.InterruptPending
	buf[105] = c.InterruptVector
	buf[106] = c.InterruptsEnabled
	buf[107] = c.Pad2
	binary.LittleEndian.PutUint32(buf[108:112], c.TickCounter)
	binary.LittleEndian.PutUint32(buf[112:116], c.ProgramBreak)
	binary.LittleEndian.PutUint32(buf[116:120], c.ExitCode)
	buf[120] = c.CurrentPID
	buf[121] = c.NumProcesses
	copy(buf[122:128], c.Pad3[:])
	return buf
}

// UnmarshalCpuState deserializes a 128-byte little-endian buffer into CpuState.
// Returns an error if the buffer is too small.
func UnmarshalCpuState(data []byte) (*CpuState, error) {
	if len(data) < CpuStateSize {
		return nil, fmt.Errorf("buffer too small: got %d bytes, need %d", len(data), CpuStateSize)
	}
	cpu := &CpuState{}
	for i := 0; i < RegCount; i++ {
		cpu.Regs[i] = binary.LittleEndian.Uint32(data[i*4 : i*4+4])
	}
	cpu.PC = binary.LittleEndian.Uint32(data[64:68])
	cpu.Flags = data[68]
	cpu.Halted = data[69]
	cpu.Stalled = data[70]
	cpu.Pad = data[71]
	cpu.SleepUntil = binary.LittleEndian.Uint64(data[72:80])
	cpu.InsnCount = binary.LittleEndian.Uint64(data[80:88])
	cpu.CacheHits = binary.LittleEndian.Uint64(data[88:96])
	cpu.CacheMisses = binary.LittleEndian.Uint64(data[96:104])
	cpu.InterruptPending = data[104]
	cpu.InterruptVector = data[105]
	cpu.InterruptsEnabled = data[106]
	cpu.Pad2 = data[107]
	cpu.TickCounter = binary.LittleEndian.Uint32(data[108:112])
	cpu.ProgramBreak = binary.LittleEndian.Uint32(data[112:116])
	cpu.ExitCode = binary.LittleEndian.Uint32(data[116:120])
	cpu.CurrentPID = data[120]
	cpu.NumProcesses = data[121]
	copy(cpu.Pad3[:], data[122:128])
	return cpu, nil
}

// MarshalBinary serializes KeyboardState to a 16-byte little-endian representation.
func (k *KeyboardState) MarshalBinary() []byte {
	buf := make([]byte, KeyboardStateSize)
	binary.LittleEndian.PutUint32(buf[0:4], k.Key)
	binary.LittleEndian.PutUint32(buf[4:8], k.Pressed)
	binary.LittleEndian.PutUint64(buf[8:16], k.Sequence)
	return buf
}

// UnmarshalKeyboardState deserializes a 16-byte buffer into KeyboardState.
func UnmarshalKeyboardState(data []byte) (*KeyboardState, error) {
	if len(data) < KeyboardStateSize {
		return nil, fmt.Errorf("buffer too small: got %d bytes, need %d", len(data), KeyboardStateSize)
	}
	ks := &KeyboardState{}
	ks.Key = binary.LittleEndian.Uint32(data[0:4])
	ks.Pressed = binary.LittleEndian.Uint32(data[4:8])
	ks.Sequence = binary.LittleEndian.Uint64(data[8:16])
	return ks, nil
}

// MapAccessor abstracts BPF map read/write operations so that tests can
// use mock implementations without real BPF maps.
type MapAccessor interface {
	// LookupElem reads a single element from the map.
	// key is a raw byte slice; valueSize determines output buffer size.
	LookupElem(key []byte, valueSize int) ([]byte, error)

	// UpdateElem writes a single element to the map.
	// key and value are raw byte slices.
	UpdateElem(key, value []byte) error
}
