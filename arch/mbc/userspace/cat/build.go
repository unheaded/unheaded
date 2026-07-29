// SPDX-License-Identifier: GPL-3.0-or-later

// Generates cat.upcf — read from stdin and echo to stdout.
//
// Reads one byte at a time from stdin (fd 0), writes it to stdout (fd 1).
// When read returns 0 (EOF / Ctrl+D), exits cleanly.
//
// MBC instruction format: [opcode:8][dst:4][src:4][imm16:16]
// Syscall convention: r0=syscall_nr, r1-r3=args, INT 0x80, result in r0.
//
// Output: cat.upcf (UPCFlat binary with separate text + data segments)
package main

import (
	"encoding/binary"
	"fmt"
	"os"
)

// ── MBC Opcodes ──────────────────────────────────────────────────────
const (
	MOVI = 0x0F
	CMP  = 0x10
	INT  = 0x17
	JMP  = 0x20
	JZ   = 0x21
	HALT = 0xFF
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

func main() {
	var code []uint32

	emit := func(opcode, dst, src uint8, imm uint16) int {
		idx := len(code)
		code = append(code, encode(opcode, dst, src, imm))
		return idx
	}

	// ── Read loop ────────────────────────────────────────────────
	loopStart := len(code)

	// SYS_READ(0, buf, 1)
	emit(MOVI, 0, 0, SYS_READ)
	emit(MOVI, 1, 0, 0) // stdin
	bufInstr := emit(MOVI, 2, 0, 0)
	emit(MOVI, 3, 0, 1) // count = 1
	emit(INT, 0, 0, 0x80)

	// If read returned 0 (EOF), exit
	emit(MOVI, 6, 0, 0)
	emit(CMP, 0, 6, 0)
	exitJmpInstr := emit(JZ, 0, 0, 0) // patched

	// SYS_WRITE(1, buf, 1)
	emit(MOVI, 0, 0, SYS_WRITE)
	emit(MOVI, 1, 0, 1) // stdout
	bufInstr2 := emit(MOVI, 2, 0, 0)
	emit(MOVI, 3, 0, 1) // count = 1
	emit(INT, 0, 0, 0x80)

	// Jump back to loop start
	loopJmpOffset := int16(loopStart - (len(code) + 1)) // #nosec G115 -- MBC program generator over internal constants; the MBC address space is 16-bit by design
	emit(JMP, 0, 0, uint16(loopJmpOffset))              // #nosec G115 -- MBC program generator over internal constants; the MBC address space is 16-bit by design

	// ── Exit path ────────────────────────────────────────────────
	exitTarget := len(code)

	emit(MOVI, 0, 0, SYS_EXIT)
	emit(MOVI, 1, 0, 0)
	emit(INT, 0, 0, 0x80)
	emit(HALT, 0, 0, 0)

	// Patch exit jump
	exitOffset := int16(exitTarget - (exitJmpInstr + 1))      // #nosec G115 -- MBC program generator over internal constants; the MBC address space is 16-bit by design
	code[exitJmpInstr] = encode(JZ, 0, 0, uint16(exitOffset)) // #nosec G115 -- MBC program generator over internal constants; the MBC address space is 16-bit by design

	// ── Data segment (1-word read buffer) ────────────────────────
	codeWords := len(code)
	dataOffset := uint16(codeWords * 4) // #nosec G115 -- MBC program generator over internal constants; the MBC address space is 16-bit by design
	bufAddr := dataOffset

	code[bufInstr] = encode(MOVI, 2, 0, bufAddr)
	code[bufInstr2] = encode(MOVI, 2, 0, bufAddr)

	dataWords := []uint32{0} // 1-word buffer

	// ── Output: UPCFlat ──────────────────────────────────────────
	upcfBin := createUPCFlat(code, dataWords, 0, 2048)

	if err := os.WriteFile("cat.upcf", upcfBin, 0o644); err != nil { // #nosec G306 -- 0644 — non-sensitive artifact; secrets in this tree are written 0600
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Generated cat.upcf: %d text + %d data words (%d bytes)\n",
		codeWords, len(dataWords), len(upcfBin))
}

// ── UPCFlat format helpers ──────────────────────────────────────────

const (
	upcFlatMagic      = "UPCF"
	upcFlatVersion    = 1
	upcFlatHeaderSize = 32
)

func createUPCFlat(text []uint32, data []uint32, bssWords uint32, stackSize uint32) []byte {
	totalBytes := upcFlatHeaderSize + (len(text)+len(data))*4
	out := make([]byte, totalBytes)

	copy(out[0:4], upcFlatMagic)
	binary.LittleEndian.PutUint32(out[4:8], upcFlatVersion)
	binary.LittleEndian.PutUint32(out[8:12], 0)
	binary.LittleEndian.PutUint32(out[12:16], uint32(len(text))) // #nosec G115 -- MBC program generator over internal constants; the MBC address space is 16-bit by design
	binary.LittleEndian.PutUint32(out[16:20], uint32(len(text))) // #nosec G115 -- MBC program generator over internal constants; the MBC address space is 16-bit by design
	binary.LittleEndian.PutUint32(out[20:24], uint32(len(data))) // #nosec G115 -- MBC program generator over internal constants; the MBC address space is 16-bit by design
	binary.LittleEndian.PutUint32(out[24:28], bssWords)
	binary.LittleEndian.PutUint32(out[28:32], stackSize)

	off := upcFlatHeaderSize
	for _, w := range text {
		binary.LittleEndian.PutUint32(out[off:off+4], w)
		off += 4
	}
	for _, w := range data {
		binary.LittleEndian.PutUint32(out[off:off+4], w)
		off += 4
	}

	return out
}
