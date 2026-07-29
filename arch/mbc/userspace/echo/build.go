// SPDX-License-Identifier: GPL-3.0-or-later

// Generates echo.upcf — print arguments to stdout.
//
// Usage: echo hello world
// Prints each argument separated by spaces, followed by a newline.
// Since the MBC exec setup passes argc in r1 and argv pointer in r2,
// this implementation prints a fixed "echo: ok\n" message (arguments
// are not yet passed through the exec ABI in UPCFlat).
//
// MBC instruction format: [opcode:8][dst:4][src:4][imm16:16]
// Syscall convention: r0=syscall_nr, r1-r3=args, INT 0x80, result in r0.
//
// Output: echo.upcf (UPCFlat binary with separate text + data segments)
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
	MOVI = 0x0F
	INT  = 0x17
	HALT = 0xFF
)

// ── Syscall numbers ──────────────────────────────────────────────────
const (
	SYS_EXIT  = 1
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
	msg := "echo: ok\n"

	var code []uint32

	emit := func(opcode, dst, src uint8, imm uint16) int {
		idx := len(code)
		code = append(code, encode(opcode, dst, src, imm))
		return idx
	}

	// Print message via SYS_WRITE(1, &msg, len(msg))
	emit(MOVI, 0, 0, SYS_WRITE)
	emit(MOVI, 1, 0, 1) // stdout
	msgInstr := emit(MOVI, 2, 0, 0)
	emit(MOVI, 3, 0, uint16(len(msg)))
	emit(INT, 0, 0, 0x80)

	// SYS_EXIT(0)
	emit(MOVI, 0, 0, SYS_EXIT)
	emit(MOVI, 1, 0, 0)
	emit(INT, 0, 0, 0x80)
	emit(HALT, 0, 0, 0)

	// ── Data segment ─────────────────────────────────────────────
	codeWords := len(code)
	dataOffset := uint16(codeWords * 4)

	msgWords := packStringWords(msg)
	msgAddr := dataOffset

	// Patch address
	code[msgInstr] = encode(MOVI, 2, 0, msgAddr)

	var dataWords []uint32
	dataWords = append(dataWords, msgWords...)

	// ── Output: UPCFlat ──────────────────────────────────────────
	upcfBin := createUPCFlat(code, dataWords, 0, 2048)

	if err := os.WriteFile("echo.upcf", upcfBin, 0o644); err != nil { // #nosec G306 -- 0644 — non-sensitive artifact; secrets in this tree are written 0600
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Generated echo.upcf: %d text + %d data words (%d bytes)\n",
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
	binary.LittleEndian.PutUint32(out[12:16], uint32(len(text)))
	binary.LittleEndian.PutUint32(out[16:20], uint32(len(text)))
	binary.LittleEndian.PutUint32(out[20:24], uint32(len(data)))
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
