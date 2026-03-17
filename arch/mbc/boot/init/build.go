// SPDX-License-Identifier: GPL-3.0-or-later

//go:build ignore

// Generates init.upcf — the /init binary for MBC Linux in UPCFlat format.
//
// This is PID 1: the first userspace process.  It prints a boot banner,
// then enters a fork/exec/wait loop that keeps /bin/sh alive.
//
// MBC instruction format: [opcode:8][dst:4][src:4][imm16:16]
// Syscall convention: r0=syscall_nr, r1-r5=args, INT 0x80, result in r0.
//
// Output: init.upcf (UPCFlat binary with separate text + data segments)
package main

import (
	"encoding/binary"
	"fmt"
	"os"
)

// ── MBC Opcodes ──────────────────────────────────────────────────────
const (
	NOP  = 0x00
	ADD  = 0x01
	SUB  = 0x02
	MOV  = 0x0E
	MOVI = 0x0F
	CMP  = 0x10
	INT  = 0x17
	JMP  = 0x20
	JZ   = 0x21
	JNZ  = 0x22
	HALT = 0xFF
)

// ── Syscall numbers (Linux/i386 convention) ──────────────────────────
const (
	SYS_EXIT    = 1
	SYS_FORK    = 2
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

func main() {
	// Data strings (will be placed in data segment)
	banner := "MBC Linux init starting\n"        // 24 bytes
	childExitMsg := "init: child exited\n"        // 19 bytes + null = 20
	shPath := "/bin/sh\x00"                       // 8 bytes

	var code []uint32

	emit := func(opcode, dst, src uint8, imm uint16) int {
		idx := len(code)
		code = append(code, encode(opcode, dst, src, imm))
		return idx
	}

	// ═══════════════════════════════════════════════════════════════
	// _start: Print boot banner
	// ═══════════════════════════════════════════════════════════════
	emit(MOVI, 0, 0, SYS_WRITE)                       // r0 = SYS_WRITE
	emit(MOVI, 1, 0, 1)                               // r1 = stdout
	bannerInstr := emit(MOVI, 2, 0, 0)                // r2 = &banner (patched)
	emit(MOVI, 3, 0, uint16(len(banner)))             // r3 = banner length
	emit(INT, 0, 0, 0x80)                             // INT 0x80

	// ═══════════════════════════════════════════════════════════════
	// spawn_shell: Fork
	// ═══════════════════════════════════════════════════════════════
	spawnShell := len(code)

	emit(MOVI, 0, 0, SYS_FORK)                        // r0 = SYS_FORK
	emit(INT, 0, 0, 0x80)                             // INT 0x80

	// Check: r0 == 0 → child
	emit(MOVI, 6, 0, 0)                               // r6 = 0
	emit(CMP, 0, 6, 0)                                // flags = cmp(r0, 0)
	childJmpInstr := emit(JZ, 0, 0, 0)                // JZ child (patched)

	// ═══════════════════════════════════════════════════════════════
	// Parent: waitpid(-1), print message, respawn
	// ═══════════════════════════════════════════════════════════════
	emit(MOVI, 0, 0, SYS_WAITPID)                     // r0 = SYS_WAITPID
	emit(MOVI, 1, 0, 0xFFFF)                          // r1 = -1 (any child)
	emit(MOVI, 2, 0, 0)                               // r2 = NULL (status)
	emit(MOVI, 3, 0, 0)                               // r3 = 0 (options)
	emit(INT, 0, 0, 0x80)                             // INT 0x80

	// Print "init: child exited\n"
	emit(MOVI, 0, 0, SYS_WRITE)                       // r0 = SYS_WRITE
	emit(MOVI, 1, 0, 1)                               // r1 = stdout
	childMsgInstr := emit(MOVI, 2, 0, 0)              // r2 = &child_exit_msg (patched)
	emit(MOVI, 3, 0, 20)                              // r3 = 20 bytes
	emit(INT, 0, 0, 0x80)                             // INT 0x80

	// Jump back to spawn_shell
	spawnJmpOffset := int16(spawnShell - (len(code) + 1))
	emit(JMP, 0, 0, uint16(spawnJmpOffset))           // JMP spawn_shell

	// ═══════════════════════════════════════════════════════════════
	// Child: execve("/bin/sh", NULL, NULL)
	// ═══════════════════════════════════════════════════════════════
	childStart := len(code)

	emit(MOVI, 0, 0, SYS_EXECVE)                      // r0 = SYS_EXECVE
	shPathInstr := emit(MOVI, 1, 0, 0)                // r1 = &sh_path (patched)
	emit(MOVI, 2, 0, 0)                               // r2 = NULL (argv)
	emit(MOVI, 3, 0, 0)                               // r3 = NULL (envp)
	emit(INT, 0, 0, 0x80)                             // INT 0x80

	// If execve fails, exit(1)
	emit(MOVI, 0, 0, SYS_EXIT)                        // r0 = SYS_EXIT
	emit(MOVI, 1, 0, 1)                               // r1 = 1
	emit(INT, 0, 0, 0x80)                             // INT 0x80
	emit(HALT, 0, 0, 0)                               // should not reach

	// ═══════════════════════════════════════════════════════════════
	// DATA SEGMENT
	// ═══════════════════════════════════════════════════════════════
	codeWords := len(code)
	dataOffset := uint16(codeWords * 4) // byte offset of data from payload start

	bannerWords := packStringWords(banner)
	childMsgWords := packStringWords(childExitMsg)
	shPathWords := packStringWords(shPath)

	bannerAddr := dataOffset
	childMsgAddr := bannerAddr + uint16(len(bannerWords)*4)
	shPathAddr := childMsgAddr + uint16(len(childMsgWords)*4)

	// Patch data addresses in code
	code[bannerInstr] = encode(MOVI, 2, 0, bannerAddr)
	code[childMsgInstr] = encode(MOVI, 2, 0, childMsgAddr)
	code[shPathInstr] = encode(MOVI, 1, 0, shPathAddr)

	// Patch child JZ target
	childOffset := int16(childStart - (childJmpInstr + 1))
	code[childJmpInstr] = encode(JZ, 0, 0, uint16(childOffset))

	// Build data segment
	var dataWords []uint32
	dataWords = append(dataWords, bannerWords...)
	dataWords = append(dataWords, childMsgWords...)
	dataWords = append(dataWords, shPathWords...)

	// ═══════════════════════════════════════════════════════════════
	// OUTPUT: UPCFlat format (.upcf)
	// ═══════════════════════════════════════════════════════════════
	upcfBin := createUPCFlat(code, dataWords, 0, 4096)

	f, err := os.Create("init.upcf")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	if _, err := f.Write(upcfBin); err != nil {
		fmt.Fprintf(os.Stderr, "write error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Generated init.upcf: %d text + %d data words = %d total (%d bytes payload, %d bytes with header)\n",
		codeWords, len(dataWords), codeWords+len(dataWords),
		(codeWords+len(dataWords))*4, len(upcfBin))
	fmt.Printf("  Format:     UPCFlat v1 (bFLT-like)\n")
	fmt.Printf("  Header:     %d bytes\n", upcFlatHeaderSize)
	fmt.Printf("  Text:       %d instructions (%d bytes)\n", codeWords, codeWords*4)
	fmt.Printf("  Data:       %d words (%d bytes)\n", len(dataWords), len(dataWords)*4)
	fmt.Printf("  BSS:        0 words\n")
	fmt.Printf("  Stack:      4096 bytes\n")
	fmt.Printf("  Banner:     @ 0x%04X  (%d bytes)\n", bannerAddr, len(banner))
	fmt.Printf("  ChildMsg:   @ 0x%04X  (%d bytes)\n", childMsgAddr, len(childExitMsg))
	fmt.Printf("  ShPath:     @ 0x%04X  (%d bytes)\n", shPathAddr, len(shPath))
	fmt.Println()
	fmt.Println("Init sequence:")
	fmt.Println("  1. Print \"MBC Linux init starting\"")
	fmt.Println("  2. Fork child process")
	fmt.Println("  3. Child: execve(\"/bin/sh\")")
	fmt.Println("  4. Parent: waitpid(-1) loop, respawn shell on exit")
	fmt.Println()
	fmt.Println("This binary becomes /init in the UNFS root filesystem.")
}

// ── UPCFlat format helpers ──────────────────────────────────────────
// Duplicated from pkg/upc because this is a standalone build tool.

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
	binary.LittleEndian.PutUint32(out[8:12], 0)                  // entry = 0
	binary.LittleEndian.PutUint32(out[12:16], uint32(len(text))) // text size
	binary.LittleEndian.PutUint32(out[16:20], uint32(len(text))) // data start
	binary.LittleEndian.PutUint32(out[20:24], uint32(len(data))) // data size
	binary.LittleEndian.PutUint32(out[24:28], bssWords)          // bss size
	binary.LittleEndian.PutUint32(out[28:32], stackSize)         // stack size

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
