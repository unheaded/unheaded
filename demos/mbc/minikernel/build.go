// SPDX-License-Identifier: GPL-3.0-or-later

// Generates minikernel.bin — a bootable MBC binary for the UPC.
//
// This program emits MBC instructions as little-endian u32 words, followed
// by a data section containing strings and constants.  The resulting binary
// can be booted with: wotan-ctl boot --kernel minikernel.bin
//
// MBC instruction format: [opcode:8][dst:4][src:4][imm16:16]
// Opcodes sourced from monad-common/src/lib.rs (mbc_opcodes module).
// Syscall numbers from mbc_linux_syscalls (i386 ABI via INT 0x80).
package main

import (
	"encoding/binary"
	"fmt"
	"os"
)

// ── MBC Opcodes (from monad-common mbc_opcodes) ──────────────────────
const (
	NOP  = 0x00
	ADD  = 0x01
	SUB  = 0x02
	MUL  = 0x03
	MOV  = 0x0E
	MOVI = 0x0F
	CMP  = 0x10
	INT  = 0x17
	IRET = 0x18
	JMP  = 0x20
	JZ   = 0x21
	JNZ  = 0x22
	CALL = 0x27
	RET  = 0x28
	LD   = 0x30
	ST   = 0x31
	HALT = 0xFF
)

// ── Linux-compatible syscall numbers (INT 0x80, mbc_linux_syscalls) ──
const (
	SYS_EXIT          = 1
	SYS_FORK          = 2
	SYS_WRITE         = 4
	SYS_GETPID        = 20
	SYS_BRK           = 45
	SYS_SCHED_YIELD   = 158
	SYS_NANOSLEEP     = 162
	SYS_CLOCK_GETTIME = 265
	SYS_READ_BLOCK    = 200
	SYS_WRITE_BLOCK   = 201
)

// encode packs an MBC instruction: [opcode:8][dst:4][src:4][imm16:16].
func encode(opcode, dst, src uint8, imm uint16) uint32 {
	return (uint32(opcode) << 24) |
		(uint32(dst&0x0F) << 20) |
		(uint32(src&0x0F) << 16) |
		uint32(imm)
}

// packStringWords converts a string into a slice of u32 words (byte-packed,
// little-endian, zero-padded to 4-byte alignment).
func packStringWords(s string) []uint32 {
	b := []byte(s)
	// Pad to 4-byte boundary.
	for len(b)%4 != 0 {
		b = append(b, 0)
	}
	words := make([]uint32, len(b)/4)
	for i := range words {
		words[i] = binary.LittleEndian.Uint32(b[i*4 : i*4+4])
	}
	return words
}

func main() {
	// ── Data strings ─────────────────────────────────────────────
	// We will place data right after the code section.  Addresses are
	// computed once we know the instruction count.
	type strEntry struct {
		label string
		data  string
	}
	strings := []strEntry{
		{"banner", "MiniKernel v0.1 — UPC Level 5\n"}, // 32 bytes (UTF-8 em dash = 3 bytes)
		{"parent_msg", "Parent process running\n"},    // 23 bytes
		{"child_msg", "Child process running\n"},      // 22 bytes
		{"block_msg", "Block 0 read OK\n"},            // 16 bytes
		{"bwrite_msg", "Block 1 write OK\n"},          // 17 bytes
		{"pid_msg", "PID queried OK\n"},               // 15 bytes
		{"tick_msg", ".\n"},                           // 2 bytes
	}

	// Compute actual byte lengths for each string.
	strLens := make(map[string]uint16)
	for _, s := range strings {
		strLens[s.label] = uint16(len(s.data)) // #nosec G115 -- MBC program generator over internal constants; the MBC address space is 16-bit by design
	}

	// ── Emit instructions ────────────────────────────────────────
	// We build the code array first, using placeholder addresses (0x0000)
	// for string references, then patch them once we know the data offset.

	var code []uint32

	emit := func(opcode, dst, src uint8, imm uint16) int {
		idx := len(code)
		code = append(code, encode(opcode, dst, src, imm))
		return idx
	}

	// Instruction indices that need address patching (data references).
	type patch struct {
		instrIdx int
		label    string
	}
	var patches []patch

	emitWithStrRef := func(opcode, dst, src uint8, label string) int {
		idx := emit(opcode, dst, src, 0x0000) // placeholder
		patches = append(patches, patch{idx, label})
		return idx
	}

	// ── _start: Print boot banner ────────────────────────────────
	emit(MOVI, 0, 0, SYS_WRITE)          // 0: r0 = 4 (SYS_WRITE)
	emit(MOVI, 1, 0, 1)                  // 1: r1 = 1 (stdout)
	emitWithStrRef(MOVI, 2, 0, "banner") // 2: r2 = &banner
	emit(MOVI, 3, 0, strLens["banner"])  // 3: r3 = banner_len
	emit(INT, 0, 0, 0x80)                // 4: INT 0x80

	// ── Install timer handler in IVT[0x20] ───────────────────────
	// timer_handler address will be patched below.
	timerAddrInstr := emit(MOVI, 4, 0, 0x0000) // 5: r4 = timer_handler (patched)
	emit(MOVI, 5, 0, 0x0080)                   // 6: r5 = 0x0080 (IVT[0x20])
	emit(ST, 5, 4, 0)                          // 7: RAM[r5+0] = r4

	// ── Fork ─────────────────────────────────────────────────────
	emit(MOVI, 0, 0, SYS_FORK)         // 8: r0 = 2 (SYS_FORK)
	emit(INT, 0, 0, 0x80)              // 9: INT 0x80
	emit(MOVI, 6, 0, 0)                // 10: r6 = 0
	emit(CMP, 0, 6, 0)                 // 11: flags = cmp(r0, r6)
	childJmpInstr := emit(JZ, 0, 0, 0) // 12: JZ child_process (offset patched)

	// ── parent_process ───────────────────────────────────────────
	parentStart := len(code)
	_ = parentStart

	// Print "Parent process running"
	emit(MOVI, 0, 0, SYS_WRITE)              // 13
	emit(MOVI, 1, 0, 1)                      // 14
	emitWithStrRef(MOVI, 2, 0, "parent_msg") // 15
	emit(MOVI, 3, 0, strLens["parent_msg"])  // 16
	emit(INT, 0, 0, 0x80)                    // 17

	// Read block 0 from ramdisk
	emit(MOVI, 0, 0, SYS_READ_BLOCK) // 18
	emit(MOVI, 1, 0, 0)              // 19: block 0
	emit(MOVI, 2, 0, 0x3000)         // 20: buffer at 0x3000
	emit(INT, 0, 0, 0x80)            // 21

	// Print "Block 0 read OK"
	emit(MOVI, 0, 0, SYS_WRITE)             // 22
	emit(MOVI, 1, 0, 1)                     // 23
	emitWithStrRef(MOVI, 2, 0, "block_msg") // 24
	emit(MOVI, 3, 0, strLens["block_msg"])  // 25
	emit(INT, 0, 0, 0x80)                   // 26

	// Write buffer to block 1
	emit(MOVI, 0, 0, SYS_WRITE_BLOCK) // 27
	emit(MOVI, 1, 0, 1)               // 28: block 1
	emit(MOVI, 2, 0, 0x3000)          // 29: source buffer
	emit(INT, 0, 0, 0x80)             // 30

	// Print "Block 1 write OK"
	emit(MOVI, 0, 0, SYS_WRITE)              // 31
	emit(MOVI, 1, 0, 1)                      // 32
	emitWithStrRef(MOVI, 2, 0, "bwrite_msg") // 33
	emit(MOVI, 3, 0, strLens["bwrite_msg"])  // 34
	emit(INT, 0, 0, 0x80)                    // 35

	// Get our PID
	emit(MOVI, 0, 0, SYS_GETPID) // 36
	emit(INT, 0, 0, 0x80)        // 37

	// Print "PID queried OK"
	emit(MOVI, 0, 0, SYS_WRITE)           // 38
	emit(MOVI, 1, 0, 1)                   // 39
	emitWithStrRef(MOVI, 2, 0, "pid_msg") // 40
	emit(MOVI, 3, 0, strLens["pid_msg"])  // 41
	emit(INT, 0, 0, 0x80)                 // 42

	// Get clock time
	emit(MOVI, 0, 0, uint16(SYS_CLOCK_GETTIME)) // 43
	emit(MOVI, 1, 0, 0)                         // 44: CLOCK_REALTIME
	emit(MOVI, 2, 0, 0x3200)                    // 45: output buffer
	emit(INT, 0, 0, 0x80)                       // 46

	// parent_loop: yield forever
	parentLoopStart := len(code)
	emit(MOVI, 0, 0, SYS_SCHED_YIELD) // 47
	emit(INT, 0, 0, 0x80)             // 48
	// JMP is pc-relative (signed imm16). Offset = target - (current+1).
	parentLoopOffset := int16(parentLoopStart - (len(code) + 1)) // #nosec G115 -- MBC program generator over internal constants; the MBC address space is 16-bit by design
	// #nosec G115 -- MBC program generator over internal constants; the MBC address space is 16-bit by design
	emit(JMP, 0, 0, uint16(parentLoopOffset)) // 49: JMP parent_loop

	// ── child_process ────────────────────────────────────────────
	childStart := len(code)

	// Print "Child process running"
	emit(MOVI, 0, 0, SYS_WRITE)             // 50
	emit(MOVI, 1, 0, 1)                     // 51
	emitWithStrRef(MOVI, 2, 0, "child_msg") // 52
	emit(MOVI, 3, 0, strLens["child_msg"])  // 53
	emit(INT, 0, 0, 0x80)                   // 54

	// child_loop:
	childLoopStart := len(code)

	// Sleep 100ms
	emit(MOVI, 0, 0, SYS_NANOSLEEP) // 55
	// timespec address will be patched
	timspecInstr := emit(MOVI, 1, 0, 0x0000) // 56: r1 = &timespec (patched)
	emit(INT, 0, 0, 0x80)                    // 57

	// Print "."
	emit(MOVI, 0, 0, SYS_WRITE)            // 58
	emit(MOVI, 1, 0, 1)                    // 59
	emitWithStrRef(MOVI, 2, 0, "tick_msg") // 60
	emit(MOVI, 3, 0, strLens["tick_msg"])  // 61
	emit(INT, 0, 0, 0x80)                  // 62

	// JMP child_loop
	childLoopOffset := int16(childLoopStart - (len(code) + 1)) // #nosec G115 -- MBC program generator over internal constants; the MBC address space is 16-bit by design
	// #nosec G115 -- MBC program generator over internal constants; the MBC address space is 16-bit by design
	emit(JMP, 0, 0, uint16(childLoopOffset)) // 63: JMP child_loop

	// ── timer_handler ────────────────────────────────────────────
	timerHandlerPC := len(code)
	emit(IRET, 0, 0, 0) // 64: IRET

	// ── Record code size, compute data offset ────────────────────
	codeWords := len(code)
	// Data is placed right after code. Each instruction is 4 bytes,
	// so data starts at byte offset codeWords*4.
	dataByteOffset := uint16(codeWords * 4) // #nosec G115 -- MBC program generator over internal constants; the MBC address space is 16-bit by design

	// ── Build data section ───────────────────────────────────────
	// Compute per-string byte offsets within the data section.
	strOffsets := make(map[string]uint16)
	currentOffset := uint16(0)
	for _, s := range strings {
		strOffsets[s.label] = dataByteOffset + currentOffset
		blen := uint16(len(s.data)) // #nosec G115 -- MBC program generator over internal constants; the MBC address space is 16-bit by design
		// Advance to next 4-byte aligned position.
		aligned := (blen + 3) & ^uint16(3)
		currentOffset += aligned
	}

	// timespec follows the strings: two u32 words {0, 100_000_000}.
	timspecAddr := dataByteOffset + currentOffset
	// (timespec occupies 8 bytes = 2 words)

	// ── Patch string addresses ───────────────────────────────────
	for _, p := range patches {
		addr := strOffsets[p.label]
		code[p.instrIdx] = encode(MOVI, uint8((code[p.instrIdx]>>20)&0x0F), 0, addr)
	}

	// Patch timer_handler address (absolute byte address = timerHandlerPC * 4).
	timerAddr := uint16(timerHandlerPC * 4) // #nosec G115 -- MBC program generator over internal constants; the MBC address space is 16-bit by design
	code[timerAddrInstr] = encode(MOVI, 4, 0, timerAddr)

	// Patch child_process JZ offset (signed relative from PC+1).
	childOffset := int16(childStart - (childJmpInstr + 1))      // #nosec G115 -- MBC program generator over internal constants; the MBC address space is 16-bit by design
	code[childJmpInstr] = encode(JZ, 0, 0, uint16(childOffset)) // #nosec G115 -- MBC program generator over internal constants; the MBC address space is 16-bit by design

	// Patch timespec address.
	code[timspecInstr] = encode(MOVI, 1, 0, timspecAddr)

	// ── Pack data words ──────────────────────────────────────────
	var dataWords []uint32
	for _, s := range strings {
		dataWords = append(dataWords, packStringWords(s.data)...)
	}
	// timespec: {tv_sec=0, tv_nsec=100_000_000}
	dataWords = append(dataWords, 0x00000000)  // tv_sec = 0
	dataWords = append(dataWords, 100_000_000) // tv_nsec = 100ms

	// ── Combine code + data ──────────────────────────────────────
	binary := append(code, dataWords...)

	// ── Write output file ────────────────────────────────────────
	f, err := os.Create("minikernel.bin")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = f.Close() }()

	if err := writeLE32(f, binary); err != nil {
		fmt.Fprintf(os.Stderr, "write error: %v\n", err)
		os.Exit(1)
	}

	totalWords := len(binary)
	fmt.Printf("Generated minikernel.bin: %d instructions + %d data words = %d total words (%d bytes)\n",
		codeWords, len(dataWords), totalWords, totalWords*4)
	fmt.Printf("  Code:  %d instructions (%d bytes)\n", codeWords, codeWords*4)
	fmt.Printf("  Data:  %d words (%d bytes)\n", len(dataWords), len(dataWords)*4)
	fmt.Printf("  Strings:\n")
	for _, s := range strings {
		fmt.Printf("    %-12s @ 0x%04X  (%d bytes)\n", s.label, strOffsets[s.label], len(s.data))
	}
	fmt.Printf("    %-12s @ 0x%04X  (8 bytes)\n", "timespec", timspecAddr)
	fmt.Printf("  Timer handler @ 0x%04X (instruction %d)\n", timerAddr, timerHandlerPC)
	fmt.Printf("  Child process @ instruction %d (JZ offset %+d)\n", childStart, int16(childStart-(childJmpInstr+1))) // #nosec G115 -- MBC program generator over internal constants; the MBC address space is 16-bit by design
}

// writeLE32 writes a slice of uint32 values in little-endian format.
func writeLE32(f *os.File, words []uint32) error {
	buf := make([]byte, 4)
	for _, w := range words {
		binary.LittleEndian.PutUint32(buf, w)
		if _, err := f.Write(buf); err != nil {
			return err
		}
	}
	return nil
}
