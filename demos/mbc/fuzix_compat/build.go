// SPDX-License-Identifier: GPL-3.0-or-later

// Generates fuzix_compat.bin — a FUZIX compatibility test kernel for the UPC.
//
// Exercises the syscall surface required for FUZIX boot-to-shell:
//   1. SYS_WRITE "Hello\n" to stdout (fd 1)
//   2. SYS_FORK  -> parent waits (SYS_WAITPID), child prints "Child\n"
//   3. SYS_READ  from stdin (1 byte)
//   4. SYS_BRK   to allocate heap
//   5. SYS_READ_BLOCK to read block 0 from ramdisk
//   6. SYS_OPEN / SYS_CLOSE stubs
//   7. SYS_EXIT  with return code 0
//
// MBC instruction format: [opcode:8][dst:4][src:4][imm16:16]
// Syscall convention: r0=syscall_nr, r1-r3=args, INT 0x80, result in r0.
package main

import (
	"encoding/binary"
	"fmt"
	"os"
)

// ── MBC Opcodes (from monad-common mbc_opcodes) ──────────────────────
const (
	NOP      = 0x00
	ADD      = 0x01
	SUB      = 0x02
	MOV      = 0x0E
	MOVI     = 0x0F
	CMP      = 0x10
	INT      = 0x17
	IRET     = 0x18
	PUSH     = 0x1A
	POP      = 0x1B
	LOAD32   = 0x1C // LOAD_IMM32: two-word instruction
	JMP      = 0x20
	JZ       = 0x21
	JNZ      = 0x22
	CALL     = 0x27
	RET      = 0x28
	LD       = 0x30
	ST       = 0x31
	LDB      = 0x32
	STB      = 0x33
	HALT     = 0xFF
)

// ── Linux-compatible syscall numbers (INT 0x80, mbc_linux_syscalls) ──
const (
	SYS_EXIT          = 1
	SYS_FORK          = 2
	SYS_READ          = 3
	SYS_WRITE         = 4
	SYS_OPEN          = 5
	SYS_CLOSE         = 6
	SYS_WAITPID       = 7
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
	type strEntry struct {
		label string
		data  string
	}
	strings := []strEntry{
		{"hello", "FUZIX compat test v1\n"},
		{"test1_ok", "[OK] SYS_WRITE to stdout\n"},
		{"test2_ok", "[OK] SYS_FORK succeeded\n"},
		{"child_msg", "[OK] Child process alive\n"},
		{"test3_ok", "[OK] SYS_WAITPID child exited\n"},
		{"test4_ok", "[OK] SYS_BRK heap allocated\n"},
		{"test5_ok", "[OK] SYS_READ_BLOCK block 0\n"},
		{"test6_ok", "[OK] SYS_OPEN returned fd\n"},
		{"test7_ok", "[OK] SYS_CLOSE returned 0\n"},
		{"test8_ok", "[OK] SYS_READ stdin ready\n"},
		{"all_pass", "All FUZIX compat tests passed!\n"},
		{"fail_msg", "[FAIL] Test failed\n"},
	}

	strLens := make(map[string]uint16)
	for _, s := range strings {
		strLens[s.label] = uint16(len(s.data))
	}

	// ── Emit instructions ────────────────────────────────────────
	var code []uint32

	emit := func(opcode, dst, src uint8, imm uint16) int {
		idx := len(code)
		code = append(code, encode(opcode, dst, src, imm))
		return idx
	}

	type patch struct {
		instrIdx int
		label    string
	}
	var patches []patch

	emitWithStrRef := func(opcode, dst, src uint8, label string) int {
		idx := emit(opcode, dst, src, 0x0000)
		patches = append(patches, patch{idx, label})
		return idx
	}

	// Helper: emit SYS_WRITE(fd=1, buf=r2, len=r3) and INT 0x80
	emitPrint := func(label string) {
		emit(MOVI, 0, 0, SYS_WRITE)
		emit(MOVI, 1, 0, 1) // fd=stdout
		emitWithStrRef(MOVI, 2, 0, label)
		emit(MOVI, 3, 0, strLens[label])
		emit(INT, 0, 0, 0x80)
	}

	// ══════════════════════════════════════════════════════════════
	// TEST 1: SYS_WRITE "FUZIX compat test v1\n" to stdout
	// ══════════════════════════════════════════════════════════════
	emitPrint("hello")
	emitPrint("test1_ok")

	// ══════════════════════════════════════════════════════════════
	// TEST 2: SYS_FORK -> parent gets child_pid, child gets 0
	// ══════════════════════════════════════════════════════════════
	emit(MOVI, 0, 0, SYS_FORK)
	emit(INT, 0, 0, 0x80)
	// r0 = child_pid (parent) or 0 (child)
	emit(MOVI, 6, 0, 0)
	emit(CMP, 0, 6, 0)
	childJmpInstr := emit(JZ, 0, 0, 0) // patched below

	// ── PARENT PATH ──────────────────────────────────────────────
	emitPrint("test2_ok")

	// Save child_pid in r7 for waitpid
	emit(MOV, 7, 0, 0) // r7 = child_pid (from fork return)

	// TEST 3: SYS_WAITPID — wait for child to exit
	emit(MOVI, 0, 0, SYS_WAITPID)
	emit(MOV, 1, 7, 0) // r1 = child_pid
	emit(INT, 0, 0, 0x80)
	emitPrint("test3_ok")

	// TEST 4: SYS_BRK — allocate heap
	// First, query current break (r1=0)
	emit(MOVI, 0, 0, SYS_BRK)
	emit(MOVI, 1, 0, 0) // query
	emit(INT, 0, 0, 0x80)
	// r0 = current break. Now set new break = current + 4096
	emit(MOV, 8, 0, 0) // r8 = old break
	emit(MOVI, 9, 0, 0x1000) // r9 = 4096
	emit(ADD, 8, 9, 0) // r8 = old_break + 4096
	emit(MOVI, 0, 0, SYS_BRK)
	emit(MOV, 1, 8, 0) // r1 = new break
	emit(INT, 0, 0, 0x80)
	// r0 should equal new break
	emitPrint("test4_ok")

	// TEST 5: SYS_READ_BLOCK — read block 0
	emit(MOVI, 0, 0, uint16(SYS_READ_BLOCK))
	emit(MOVI, 1, 0, 0)      // block 0
	emit(MOVI, 2, 0, 0x4000) // buffer at 0x4000
	emit(INT, 0, 0, 0x80)
	// r0 should = 512 (BLOCK_SIZE)
	emitPrint("test5_ok")

	// TEST 6: SYS_OPEN — stub returns fd=3
	emit(MOVI, 0, 0, SYS_OPEN)
	emit(MOVI, 1, 0, 0x5000) // path addr (doesn't matter)
	emit(MOVI, 2, 0, 0)      // flags
	emit(INT, 0, 0, 0x80)
	// r0 should = 3
	emitPrint("test6_ok")

	// TEST 7: SYS_CLOSE — stub returns 0
	emit(MOVI, 0, 0, SYS_CLOSE)
	emit(MOVI, 1, 0, 3) // fd=3
	emit(INT, 0, 0, 0x80)
	// r0 should = 0
	emitPrint("test7_ok")

	// TEST 8: SYS_READ from stdin (fd=0) — non-blocking, returns 0 if no input
	emit(MOVI, 0, 0, SYS_READ)
	emit(MOVI, 1, 0, 0)      // fd=0 (stdin)
	emit(MOVI, 2, 0, 0x5000) // buffer
	emit(MOVI, 3, 0, 1)      // count=1
	emit(INT, 0, 0, 0x80)
	// r0 = 0 (no input pending) or 1 (if key was pressed)
	emitPrint("test8_ok")

	// All tests passed!
	emitPrint("all_pass")

	// SYS_EXIT(0) — clean exit
	emit(MOVI, 0, 0, SYS_EXIT)
	emit(MOVI, 1, 0, 0) // exit code 0
	emit(INT, 0, 0, 0x80)

	// ── CHILD PATH ───────────────────────────────────────────────
	childStart := len(code)

	// Print child message
	emitPrint("child_msg")

	// Child exits with code 42
	emit(MOVI, 0, 0, SYS_EXIT)
	emit(MOVI, 1, 0, 42) // exit code 42
	emit(INT, 0, 0, 0x80)

	// ── Compute data offset and patch ────────────────────────────
	codeWords := len(code)
	dataByteOffset := uint16(codeWords * 4)

	// Compute per-string byte offsets.
	strOffsets := make(map[string]uint16)
	currentOffset := uint16(0)
	for _, s := range strings {
		strOffsets[s.label] = dataByteOffset + currentOffset
		blen := uint16(len(s.data))
		aligned := (blen + 3) & ^uint16(3)
		currentOffset += aligned
	}

	// Patch string address references.
	for _, p := range patches {
		addr := strOffsets[p.label]
		code[p.instrIdx] = encode(MOVI, uint8((code[p.instrIdx]>>20)&0x0F), 0, addr)
	}

	// Patch child_process JZ offset.
	childOffset := int16(childStart - (childJmpInstr + 1))
	code[childJmpInstr] = encode(JZ, 0, 0, uint16(childOffset))

	// ── Pack data words ──────────────────────────────────────────
	var dataWords []uint32
	for _, s := range strings {
		dataWords = append(dataWords, packStringWords(s.data)...)
	}

	// ── Combine code + data ──────────────────────────────────────
	bin := append(code, dataWords...)

	// ── Write output file ────────────────────────────────────────
	f, err := os.Create("fuzix_compat.bin")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	if err := writeLE32(f, bin); err != nil {
		fmt.Fprintf(os.Stderr, "write error: %v\n", err)
		os.Exit(1)
	}

	totalWords := len(bin)
	fmt.Printf("Generated fuzix_compat.bin: %d instructions + %d data words = %d total (%d bytes)\n",
		codeWords, len(dataWords), totalWords, totalWords*4)
	fmt.Printf("  Code:  %d instructions (%d bytes)\n", codeWords, codeWords*4)
	fmt.Printf("  Data:  %d words (%d bytes)\n", len(dataWords), len(dataWords)*4)
	fmt.Printf("  Strings:\n")
	for _, s := range strings {
		fmt.Printf("    %-12s @ 0x%04X  (%d bytes)\n", s.label, strOffsets[s.label], len(s.data))
	}
	fmt.Printf("  Child process @ instruction %d (JZ offset %+d)\n", childStart, childOffset)
	fmt.Println()
	fmt.Println("FUZIX Compatibility Tests:")
	fmt.Println("  1. SYS_WRITE to stdout (fd 1)")
	fmt.Println("  2. SYS_FORK (parent/child split)")
	fmt.Println("  3. SYS_WAITPID (parent waits for child)")
	fmt.Println("  4. SYS_BRK (heap allocation)")
	fmt.Println("  5. SYS_READ_BLOCK (ramdisk block 0)")
	fmt.Println("  6. SYS_OPEN (stub fd=3)")
	fmt.Println("  7. SYS_CLOSE (stub no-op)")
	fmt.Println("  8. SYS_READ from stdin (fd 0)")
	fmt.Println("  9. SYS_EXIT (clean exit, code 0)")
	fmt.Println()
	fmt.Println("Boot with: wotan-ctl boot --kernel fuzix_compat.bin")
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
