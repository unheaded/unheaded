// SPDX-License-Identifier: GPL-3.0-or-later

// Generates shell.bin — a minimal interactive shell for the UPC.
//
// This becomes /bin/sh in the UNFS filesystem. Behavior:
//   - Prints "> " prompt to stdout (fd 1)
//   - Reads a line from stdin (fd 0), one byte at a time until '\n'
//   - Echos the line back to stdout
//   - If the line is "exit", calls SYS_EXIT(0)
//   - Otherwise loops back to prompt
//
// MBC instruction format: [opcode:8][dst:4][src:4][imm16:16]
// Syscall convention: r0=syscall_nr, r1-r3=args, INT 0x80, result in r0.
package main

import (
	"encoding/binary"
	"fmt"
	"os"
)

// ── MBC Opcodes ──────────────────────────────────────────────────────
const (
	NOP    = 0x00
	ADD    = 0x01
	SUB    = 0x02
	MOV    = 0x0E
	MOVI   = 0x0F
	CMP    = 0x10
	INT    = 0x17
	PUSH   = 0x1A
	POP    = 0x1B
	LOAD32 = 0x1C
	JMP    = 0x20
	JZ     = 0x21
	JNZ    = 0x22
	CALL   = 0x27
	RET    = 0x28
	LD     = 0x30
	ST     = 0x31
	LDB    = 0x32
	STB    = 0x33
	HALT   = 0xFF
)

// ── Syscall numbers ──────────────────────────────────────────────────
const (
	SYS_EXIT  = 1
	SYS_READ  = 3
	SYS_WRITE = 4
)

func encode(opcode, dst, src uint8, imm uint16) uint32 {
	return (uint32(opcode) << 24) |
		(uint32(dst&0x0F) << 20) |
		(uint32(src&0x0F) << 16) |
		uint32(imm)
}

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
	// Data strings
	prompt := "> "
	newline := "\n"
	exitCmd := "exit"

	var code []uint32

	emit := func(opcode, dst, src uint8, imm uint16) int {
		idx := len(code)
		code = append(code, encode(opcode, dst, src, imm))
		return idx
	}

	// ═══════════════════════════════════════════════════════════════
	// LOOP START — print prompt
	// ═══════════════════════════════════════════════════════════════
	loopStart := len(code)

	// Print "> " prompt
	emit(MOVI, 0, 0, SYS_WRITE)    // r0 = SYS_WRITE
	emit(MOVI, 1, 0, 1)            // r1 = stdout
	promptInstr := emit(MOVI, 2, 0, 0) // r2 = prompt addr (patched)
	emit(MOVI, 3, 0, uint16(len(prompt)))
	emit(INT, 0, 0, 0x80)

	// ═══════════════════════════════════════════════════════════════
	// READ LOOP — read one byte at a time until '\n'
	// ═══════════════════════════════════════════════════════════════
	// r8 = line buffer base address (patched)
	// r9 = current offset into buffer (starts at 0)
	bufBaseInstr := emit(MOVI, 8, 0, 0) // r8 = line buffer addr (patched)
	emit(MOVI, 9, 0, 0)                 // r9 = 0 (offset)

	readLoopStart := len(code)

	// Read 1 byte from stdin into readbuf
	emit(MOVI, 0, 0, SYS_READ)          // r0 = SYS_READ
	emit(MOVI, 1, 0, 0)                 // r1 = stdin (fd 0)
	readBufInstr := emit(MOVI, 2, 0, 0) // r2 = readbuf addr (patched)
	emit(MOVI, 3, 0, 1)                 // r3 = count 1
	emit(INT, 0, 0, 0x80)

	// Load the byte we just read
	readBufLd := emit(LDB, 4, 0, 0) // r4 = byte at readbuf (patched)

	// Store byte into line buffer at offset r9
	// r5 = buffer_base + offset
	emit(MOV, 5, 8, 0)  // r5 = r8 (buffer base)
	emit(ADD, 5, 9, 0)  // r5 = r5 + r9
	emit(STB, 4, 5, 0)  // store r4 at [r5]
	emit(MOVI, 10, 0, 1)
	emit(ADD, 9, 10, 0) // r9++ (offset)

	// Check if byte == '\n' (0x0A)
	emit(MOVI, 6, 0, 0x0A)
	emit(CMP, 4, 6, 0)
	// If not newline, loop back to read more
	readJmpOffset := int16(readLoopStart - (len(code) + 1))
	emit(JNZ, 0, 0, uint16(readJmpOffset))

	// ═══════════════════════════════════════════════════════════════
	// GOT A LINE — echo it back, then check for "exit"
	// ═══════════════════════════════════════════════════════════════

	// Echo the line (buffer base in r8, length in r9)
	emit(MOVI, 0, 0, SYS_WRITE) // r0 = SYS_WRITE
	emit(MOVI, 1, 0, 1)         // r1 = stdout
	emit(MOV, 2, 8, 0)          // r2 = buffer base
	emit(MOV, 3, 9, 0)          // r3 = length
	emit(INT, 0, 0, 0x80)

	// Check if line is "exit\n" (5 bytes)
	// Compare r9 (length) with 5
	emit(MOVI, 6, 0, 5)
	emit(CMP, 9, 6, 0)
	notExitLen := emit(JNZ, 0, 0, 0) // if len != 5, skip exit check

	// Check first 4 bytes of buffer against "exit"
	emit(LD, 4, 8, 0)                       // r4 = first 4 bytes of buffer
	exitWordInstr := emit(LOAD32, 6, 0, 0)  // r6 = "exit" as u32 (patched, 2-word insn)
	exitWordData := emit(NOP, 0, 0, 0)      // second word of LOAD32

	emit(CMP, 4, 6, 0)
	notExitCmp := emit(JNZ, 0, 0, 0) // if not "exit", skip

	// It IS "exit" — call SYS_EXIT(0)
	emit(MOVI, 0, 0, SYS_EXIT)
	emit(MOVI, 1, 0, 0)
	emit(INT, 0, 0, 0x80)
	emit(HALT, 0, 0, 0) // should not reach

	// NOT EXIT — jump back to loop start
	notExitTarget := len(code)
	loopJmpOffset := int16(loopStart - (len(code) + 1))
	emit(JMP, 0, 0, uint16(loopJmpOffset))

	// ═══════════════════════════════════════════════════════════════
	// DATA SECTION
	// ═══════════════════════════════════════════════════════════════
	codeWords := len(code)
	dataOffset := uint16(codeWords * 4)

	promptWords := packStringWords(prompt)
	newlineWords := packStringWords(newline)
	_ = newlineWords

	promptAddr := dataOffset
	readBufAddr := promptAddr + uint16(len(promptWords)*4)
	lineBufAddr := readBufAddr + 4 // 1 word for read buf, then line buffer
	exitWord := binary.LittleEndian.Uint32([]byte(exitCmd))

	// Patch addresses
	code[promptInstr] = encode(MOVI, 2, 0, promptAddr)
	code[bufBaseInstr] = encode(MOVI, 8, 0, lineBufAddr)
	code[readBufInstr] = encode(MOVI, 2, 0, readBufAddr)
	code[readBufLd] = encode(LDB, 4, 0, readBufAddr)

	// Patch exit word (LOAD32 immediate)
	code[exitWordInstr] = encode(LOAD32, 6, 0, 0)
	code[exitWordData] = exitWord

	// Patch JNZ/JMP for not-exit branches
	code[notExitLen] = encode(JNZ, 0, 0, uint16(int16(notExitTarget-(notExitLen+1))))
	code[notExitCmp] = encode(JNZ, 0, 0, uint16(int16(notExitTarget-(notExitCmp+1))))

	// Append data: prompt, readbuf (1 word), line buffer (64 words)
	var dataWords []uint32
	dataWords = append(dataWords, promptWords...)
	dataWords = append(dataWords, 0)          // readbuf (1 word)
	dataWords = append(dataWords, make([]uint32, 64)...) // line buffer

	// Combine
	bin := append(code, dataWords...)

	// Write output
	f, err := os.Create("shell.bin")
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
	fmt.Printf("Generated shell.bin: %d instructions + %d data words = %d total (%d bytes)\n",
		codeWords, len(dataWords), totalWords, totalWords*4)
	fmt.Printf("  Code:       %d instructions (%d bytes)\n", codeWords, codeWords*4)
	fmt.Printf("  Data:       %d words (%d bytes)\n", len(dataWords), len(dataWords)*4)
	fmt.Printf("  Prompt:     @ 0x%04X  (%d bytes)\n", promptAddr, len(prompt))
	fmt.Printf("  Read buf:   @ 0x%04X  (4 bytes)\n", readBufAddr)
	fmt.Printf("  Line buf:   @ 0x%04X  (256 bytes)\n", lineBufAddr)
	fmt.Println()
	fmt.Println("Shell features:")
	fmt.Println("  - Prints \"> \" prompt")
	fmt.Println("  - Reads line from stdin (byte-at-a-time until newline)")
	fmt.Println("  - Echos line back to stdout")
	fmt.Println("  - Handles \"exit\" command (SYS_EXIT)")
	fmt.Println()
	fmt.Println("This binary becomes /bin/sh in the UNFS filesystem.")
}

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
