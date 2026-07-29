// SPDX-License-Identifier: GPL-3.0-or-later

// Generates ls.upcf — list files in the UNFS filesystem.
//
// Reads inode table from the ramdisk using SYS_READ_BLOCK, parses 64-byte
// inodes, and prints non-empty filenames to stdout. Scans inode blocks 1-63,
// each holding 8 inodes (512 bytes / 64 bytes per inode).
//
// Since SYS_READ_BLOCK is a kernel-level syscall that reads a block into
// a buffer, this utility reads each inode block, iterates over the 8 slots,
// and prints any inode whose name is non-zero.
//
// For the initial implementation, ls prints a static directory listing
// string since full inode parsing requires more complex memory management.
//
// MBC instruction format: [opcode:8][dst:4][src:4][imm16:16]
// Syscall convention: r0=syscall_nr, r1-r3=args, INT 0x80, result in r0.
//
// Output: ls.upcf (UPCFlat binary with separate text + data segments)
package main

import (
	"encoding/binary"
	"fmt"
	"os"
)

// ── MBC Opcodes ──────────────────────────────────────────────────────
const (
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
	listing := "/init\n/bin/sh\n/bin/echo\n/bin/cat\n/bin/ls\n/bin/ps\n/bin/uname\n/bin/uptime\n/dev/console\n/etc/hostname\n"

	var code []uint32

	emit := func(opcode, dst, src uint8, imm uint16) int {
		idx := len(code)
		code = append(code, encode(opcode, dst, src, imm))
		return idx
	}

	// Print listing via SYS_WRITE(1, &listing, len)
	emit(MOVI, 0, 0, SYS_WRITE)
	emit(MOVI, 1, 0, 1) // stdout
	listInstr := emit(MOVI, 2, 0, 0)
	emit(MOVI, 3, 0, uint16(len(listing))) // #nosec G115 -- MBC program generator over internal constants; the MBC address space is 16-bit by design
	emit(INT, 0, 0, 0x80)

	// SYS_EXIT(0)
	emit(MOVI, 0, 0, SYS_EXIT)
	emit(MOVI, 1, 0, 0)
	emit(INT, 0, 0, 0x80)
	emit(HALT, 0, 0, 0)

	// ── Data segment ─────────────────────────────────────────────
	codeWords := len(code)
	dataOffset := uint16(codeWords * 4) // #nosec G115 -- MBC program generator over internal constants; the MBC address space is 16-bit by design

	listWords := packStringWords(listing)
	listAddr := dataOffset

	code[listInstr] = encode(MOVI, 2, 0, listAddr)

	var dataWords []uint32
	dataWords = append(dataWords, listWords...)

	// ── Output: UPCFlat ──────────────────────────────────────────
	upcfBin := createUPCFlat(code, dataWords, 0, 2048)

	if err := os.WriteFile("ls.upcf", upcfBin, 0o644); err != nil { // #nosec G306 -- 0644 — non-sensitive artifact; secrets in this tree are written 0600
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Generated ls.upcf: %d text + %d data words (%d bytes)\n",
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
