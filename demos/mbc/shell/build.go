// SPDX-License-Identifier: GPL-3.0-or-later

// Generates shell.upcf — a minimal interactive shell for the UPC in UPCFlat
// (bFLT-like) format.
//
// This becomes /bin/sh in the UNFS filesystem. Behavior:
//   - Prints "mbc-linux> " prompt to stdout (fd 1)
//   - Reads a line from stdin (fd 0), one byte at a time until '\n'
//   - Recognizes built-in commands:
//       exit   — calls SYS_EXIT(0)
//       help   — prints list of available commands
//   - Recognizes external commands (fork + exec):
//       echo, cat, ls, ps, uname, uptime
//   - Unknown commands: prints "unknown command\n" and loops
//
// MBC instruction format: [opcode:8][dst:4][src:4][imm16:16]
// Syscall convention: r0=syscall_nr, r1-r3=args, INT 0x80, result in r0.
//
// Output format: UPCFlat binary (.upcf) with separate text and data segments.
// The text segment contains executable instructions; the data segment holds
// the prompt string, read buffer, line buffer, and command strings.
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
	SYS_EXIT    = 1
	SYS_FORK    = 2
	SYS_READ    = 3
	SYS_WRITE   = 4
	SYS_WAITPID = 7
	SYS_EXECVE  = 11
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

// cmdEntry represents a command the shell can dispatch.
type cmdEntry struct {
	name string // command name (what user types, e.g. "ls")
	path string // filesystem path (e.g. "/bin/ls\x00")
}

func main() {
	// ── String constants ─────────────────────────────────────────
	prompt := "mbc-linux> "
	helpMsg := "Commands: echo cat ls ps uname uptime exit help\n"
	unknownMsg := "unknown command\n"

	// External commands (fork+exec)
	commands := []cmdEntry{
		{"echo", "/bin/echo\x00"},
		{"cat", "/bin/cat\x00"},
		{"ls", "/bin/ls\x00"},
		{"ps", "/bin/ps\x00"},
		{"uname", "/bin/uname\x00"},
		{"uptime", "/bin/uptime\x00"},
	}

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

	// Print "mbc-linux> " prompt
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
	// GOT A LINE — check for commands
	// ═══════════════════════════════════════════════════════════════

	// ── Check for "exit\n" (5 bytes) ─────────────────────────────
	emit(MOVI, 6, 0, 5)
	emit(CMP, 9, 6, 0)
	notExitLen := emit(JNZ, 0, 0, 0) // if len != 5, skip exit check

	emit(LD, 4, 8, 0) // r4 = first 4 bytes of buffer
	exitWordInstr := emit(LOAD32, 6, 0, 0) // r6 = "exit" as u32
	exitWordData := emit(NOP, 0, 0, 0)     // second word of LOAD32

	emit(CMP, 4, 6, 0)
	notExitCmp := emit(JNZ, 0, 0, 0) // if not "exit", skip

	// It IS "exit" — call SYS_EXIT(0)
	emit(MOVI, 0, 0, SYS_EXIT)
	emit(MOVI, 1, 0, 0)
	emit(INT, 0, 0, 0x80)
	emit(HALT, 0, 0, 0) // should not reach

	// ── Check for "help\n" (5 bytes) ─────────────────────────────
	notExitTarget := len(code)

	emit(MOVI, 6, 0, 5)
	emit(CMP, 9, 6, 0)
	notHelpLen := emit(JNZ, 0, 0, 0) // if len != 5, skip help check

	emit(LD, 4, 8, 0) // r4 = first 4 bytes
	helpWordInstr := emit(LOAD32, 6, 0, 0) // r6 = "help" as u32
	helpWordData := emit(NOP, 0, 0, 0)

	emit(CMP, 4, 6, 0)
	notHelpCmp := emit(JNZ, 0, 0, 0)

	// It IS "help" — print help message
	emit(MOVI, 0, 0, SYS_WRITE)
	emit(MOVI, 1, 0, 1)
	helpMsgInstr := emit(MOVI, 2, 0, 0) // patched
	emit(MOVI, 3, 0, uint16(len(helpMsg)))
	emit(INT, 0, 0, 0x80)

	// Jump back to loop
	helpDoneJmpOffset := int16(loopStart - (len(code) + 1))
	emit(JMP, 0, 0, uint16(helpDoneJmpOffset))

	notHelpTarget := len(code)

	// ── Check external commands ──────────────────────────────────
	// For each command, check if the line starts with the command name + '\n'
	// If match: fork, exec the binary path, waitpid, then loop
	type cmdPatch struct {
		nameWordInstr int
		nameWordData  int
		pathInstr     int
		notMatchLen   int
		notMatchCmp   int
	}
	cmdPatches := make([]cmdPatch, len(commands))

	for i, cmd := range commands {
		expectedLen := uint16(len(cmd.name) + 1) // name + '\n'

		emit(MOVI, 6, 0, expectedLen)
		emit(CMP, 9, 6, 0)
		cmdPatches[i].notMatchLen = emit(JNZ, 0, 0, 0) // skip if len mismatch

		// Compare first 4 bytes (all cmd names <= 6 chars, first 4 is enough for uniqueness)
		emit(LD, 4, 8, 0)
		cmdPatches[i].nameWordInstr = emit(LOAD32, 6, 0, 0)
		cmdPatches[i].nameWordData = emit(NOP, 0, 0, 0)

		emit(CMP, 4, 6, 0)
		cmdPatches[i].notMatchCmp = emit(JNZ, 0, 0, 0) // skip if no match

		// MATCH — fork + exec
		emit(MOVI, 0, 0, SYS_FORK)
		emit(INT, 0, 0, 0x80)

		// r0 == 0 -> child
		emit(MOVI, 6, 0, 0)
		emit(CMP, 0, 6, 0)
		childJmpInstr := emit(JZ, 0, 0, 0) // patched to child

		// Parent: waitpid
		emit(MOVI, 0, 0, SYS_WAITPID)
		emit(MOVI, 1, 0, 0xFFFF) // -1 (any child)
		emit(MOVI, 2, 0, 0)
		emit(MOVI, 3, 0, 0)
		emit(INT, 0, 0, 0x80)

		// Jump back to loop
		parentJmpOffset := int16(loopStart - (len(code) + 1))
		emit(JMP, 0, 0, uint16(parentJmpOffset))

		// Child: execve(path, NULL, NULL)
		childTarget := len(code)
		emit(MOVI, 0, 0, SYS_EXECVE)
		cmdPatches[i].pathInstr = emit(MOVI, 1, 0, 0) // patched to path addr
		emit(MOVI, 2, 0, 0) // argv = NULL
		emit(MOVI, 3, 0, 0) // envp = NULL
		emit(INT, 0, 0, 0x80)

		// If execve fails, exit(1)
		emit(MOVI, 0, 0, SYS_EXIT)
		emit(MOVI, 1, 0, 1)
		emit(INT, 0, 0, 0x80)

		// Patch child JZ
		childOffset := int16(childTarget - (childJmpInstr + 1))
		code[childJmpInstr] = encode(JZ, 0, 0, uint16(childOffset))
	}

	// ── Unknown command — print error, loop ──────────────────────
	unknownTarget := len(code)

	emit(MOVI, 0, 0, SYS_WRITE)
	emit(MOVI, 1, 0, 1)
	unknownMsgInstr := emit(MOVI, 2, 0, 0) // patched
	emit(MOVI, 3, 0, uint16(len(unknownMsg)))
	emit(INT, 0, 0, 0x80)

	// Jump back to loop
	unknownJmpOffset := int16(loopStart - (len(code) + 1))
	emit(JMP, 0, 0, uint16(unknownJmpOffset))

	// ═══════════════════════════════════════════════════════════════
	// DATA SECTION
	// ═══════════════════════════════════════════════════════════════
	codeWords := len(code)
	dataOffset := uint16(codeWords * 4)

	// Build data segment
	var dataWords []uint32
	currentDataAddr := dataOffset

	// Helper to add a string and return its address
	addData := func(s string) uint16 {
		addr := currentDataAddr
		words := packStringWords(s)
		dataWords = append(dataWords, words...)
		currentDataAddr += uint16(len(words) * 4)
		return addr
	}

	promptAddr := addData(prompt)
	helpMsgAddr := addData(helpMsg)
	unknownMsgAddr := addData(unknownMsg)

	// Read buffer (1 word)
	readBufAddr := currentDataAddr
	dataWords = append(dataWords, 0)
	currentDataAddr += 4

	// Line buffer (64 words = 256 bytes)
	lineBufAddr := currentDataAddr
	dataWords = append(dataWords, make([]uint32, 64)...)
	currentDataAddr += 64 * 4

	// Command path strings
	cmdPathAddrs := make([]uint16, len(commands))
	for i, cmd := range commands {
		cmdPathAddrs[i] = addData(cmd.path)
	}

	// ═══════════════════════════════════════════════════════════════
	// PATCH ALL ADDRESSES
	// ═══════════════════════════════════════════════════════════════

	code[promptInstr] = encode(MOVI, 2, 0, promptAddr)
	code[bufBaseInstr] = encode(MOVI, 8, 0, lineBufAddr)
	code[readBufInstr] = encode(MOVI, 2, 0, readBufAddr)
	code[readBufLd] = encode(LDB, 4, 0, readBufAddr)
	code[helpMsgInstr] = encode(MOVI, 2, 0, helpMsgAddr)
	code[unknownMsgInstr] = encode(MOVI, 2, 0, unknownMsgAddr)

	// Patch exit word (LOAD32 immediate)
	exitCmd := "exit"
	exitWord := binary.LittleEndian.Uint32([]byte(exitCmd))
	code[exitWordInstr] = encode(LOAD32, 6, 0, 0)
	code[exitWordData] = exitWord

	// Patch help word
	helpCmd := "help"
	helpWord := binary.LittleEndian.Uint32([]byte(helpCmd))
	code[helpWordInstr] = encode(LOAD32, 6, 0, 0)
	code[helpWordData] = helpWord

	// Patch JNZ for not-exit branches
	code[notExitLen] = encode(JNZ, 0, 0, uint16(int16(notExitTarget-(notExitLen+1))))
	code[notExitCmp] = encode(JNZ, 0, 0, uint16(int16(notExitTarget-(notExitCmp+1))))

	// Patch JNZ for not-help branches
	code[notHelpLen] = encode(JNZ, 0, 0, uint16(int16(notHelpTarget-(notHelpLen+1))))
	code[notHelpCmp] = encode(JNZ, 0, 0, uint16(int16(notHelpTarget-(notHelpCmp+1))))

	// Patch command check branches
	for i, cmd := range commands {
		cp := cmdPatches[i]

		// Compute the next command's start or the unknown target
		var nextTarget int
		if i+1 < len(commands) {
			// The next command check starts right after this command's code block.
			// Each command block layout: 2 (len check) + JNZ + LD + LOAD32 + NOP + CMP + JNZ
			// + fork/exec/wait block. We need the instruction index of the next block.
			nextTarget = cp.notMatchLen + 1 // will be recomputed below
		}
		// Actually, we need to find the target for the JNZ (skip on mismatch).
		// For the last command, skip to unknownTarget; for others, skip to next cmd check.

		// Determine skip target: for the last command, jump to unknownTarget.
		// For other commands, the next command's check starts after the current
		// command's child execve/exit block. We stored the instruction indices
		// of the JNZ instructions, so we can compute forward.
		if i+1 < len(commands) {
			// Next command's len check is 2 instructions before its notMatchLen
			nextTarget = cmdPatches[i+1].notMatchLen - 2
		} else {
			nextTarget = unknownTarget
		}

		code[cp.notMatchLen] = encode(JNZ, 0, 0, uint16(int16(nextTarget-(cp.notMatchLen+1))))
		code[cp.notMatchCmp] = encode(JNZ, 0, 0, uint16(int16(nextTarget-(cp.notMatchCmp+1))))

		// Patch command name word (first 4 bytes of command name, padded)
		nameBytes := make([]byte, 4)
		copy(nameBytes, cmd.name)
		nameWord := binary.LittleEndian.Uint32(nameBytes)
		code[cp.nameWordInstr] = encode(LOAD32, 6, 0, 0)
		code[cp.nameWordData] = nameWord

		// Patch path address
		code[cp.pathInstr] = encode(MOVI, 1, 0, cmdPathAddrs[i])
	}

	// ═══════════════════════════════════════════════════════════════
	// OUTPUT: UPCFlat format (.upcf)
	// ═══════════════════════════════════════════════════════════════
	upcfBin := createUPCFlat(code, dataWords, 0, 4096)

	f, err := os.Create("shell.upcf")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	if _, err := f.Write(upcfBin); err != nil {
		fmt.Fprintf(os.Stderr, "write error: %v\n", err)
		os.Exit(1)
	}

	totalWords := codeWords + len(dataWords)
	fmt.Printf("Generated shell.upcf: %d text + %d data words = %d total (%d bytes payload, %d bytes with header)\n",
		codeWords, len(dataWords), totalWords, totalWords*4, len(upcfBin))
	fmt.Printf("  Format:     UPCFlat v1 (bFLT-like)\n")
	fmt.Printf("  Header:     %d bytes\n", upcFlatHeaderSize)
	fmt.Printf("  Text:       %d instructions (%d bytes)\n", codeWords, codeWords*4)
	fmt.Printf("  Data:       %d words (%d bytes)\n", len(dataWords), len(dataWords)*4)
	fmt.Printf("  BSS:        0 words\n")
	fmt.Printf("  Stack:      4096 bytes\n")
	fmt.Printf("  Prompt:     @ 0x%04X  (%d bytes)\n", promptAddr, len(prompt))
	fmt.Printf("  Help msg:   @ 0x%04X  (%d bytes)\n", helpMsgAddr, len(helpMsg))
	fmt.Printf("  Read buf:   @ 0x%04X  (4 bytes)\n", readBufAddr)
	fmt.Printf("  Line buf:   @ 0x%04X  (256 bytes)\n", lineBufAddr)
	fmt.Println()
	fmt.Println("Shell features:")
	fmt.Println("  - Prints \"mbc-linux> \" prompt")
	fmt.Println("  - Reads line from stdin (byte-at-a-time until newline)")
	fmt.Println("  - Built-in: exit, help")
	fmt.Println("  - External: echo, cat, ls, ps, uname, uptime (fork+exec)")
	fmt.Println("  - Unknown commands print error message")
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

// ── UPCFlat format helpers ──────────────────────────────────────────
// These mirror pkg/upc.CreateUPCFlat but are duplicated here because
// this is a standalone build tool (package main).

const (
	upcFlatMagic      = "UPCF"
	upcFlatVersion    = 1
	upcFlatHeaderSize = 32
)

func createUPCFlat(text []uint32, data []uint32, bssWords uint32, stackSize uint32) []byte {
	totalBytes := upcFlatHeaderSize + (len(text)+len(data))*4
	out := make([]byte, totalBytes)

	// Header
	copy(out[0:4], upcFlatMagic)
	binary.LittleEndian.PutUint32(out[4:8], upcFlatVersion)
	binary.LittleEndian.PutUint32(out[8:12], 0)                    // entry = 0
	binary.LittleEndian.PutUint32(out[12:16], uint32(len(text)))   // text size
	binary.LittleEndian.PutUint32(out[16:20], uint32(len(text)))   // data start
	binary.LittleEndian.PutUint32(out[20:24], uint32(len(data)))   // data size
	binary.LittleEndian.PutUint32(out[24:28], bssWords)            // bss size
	binary.LittleEndian.PutUint32(out[28:32], stackSize)           // stack size

	// Text
	off := upcFlatHeaderSize
	for _, w := range text {
		binary.LittleEndian.PutUint32(out[off:off+4], w)
		off += 4
	}

	// Data
	for _, w := range data {
		binary.LittleEndian.PutUint32(out[off:off+4], w)
		off += 4
	}

	return out
}
