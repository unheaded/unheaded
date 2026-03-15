// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Generates boot_demo.bin (kernel) + ramdisk.img (UNFS filesystem) for the
// UPC boot demo. This is the complete FUZIX boot sequence:
//
//	/init → fork → child exec(/bin/sh) → parent wait → "Shell exited" → halt
//
// The ramdisk contains a UNFS filesystem with /init, /bin/sh, /dev/console,
// and /etc/hostname. The kernel (boot_demo.bin) is the init program itself.
//
// Boot with: wotan-ctl boot --kernel boot_demo.bin --ramdisk ramdisk.img
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
	NOP  = 0x00
	ADD  = 0x01
	SUB  = 0x02
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
	SYS_EXIT    = 1
	SYS_FORK    = 2
	SYS_READ    = 3
	SYS_WRITE   = 4
	SYS_OPEN    = 5
	SYS_CLOSE   = 6
	SYS_WAITPID = 7
	SYS_EXECVE  = 11
	SYS_GETPID  = 20
)

// ── UNFS filesystem constants ────────────────────────────────────────
const (
	UNFSMagic          = 0x554E4653
	UNFSVersion        = 1
	UNFSBlockSize      = 512
	UNFSBlockWords     = UNFSBlockSize / 4
	UNFSTotalBlocks    = 8192
	UNFSInodeBlocks    = 63
	UNFSInodesPerBlock = 8
	UNFSMaxInodes      = UNFSInodeBlocks * UNFSInodesPerBlock
	UNFSFirstDataBlock = 64
	UNFSInodeSize      = 64
	UNFSNameLen        = 28
	UNFSTypeFile       = 1
	UNFSTypeDir        = 2
	UNFSTypeDevice     = 3
)

// encode packs an MBC instruction: [opcode:8][dst:4][src:4][imm16:16].
func encode(opcode, dst, src uint8, imm uint16) uint32 {
	return (uint32(opcode) << 24) |
		(uint32(dst&0x0F) << 20) |
		(uint32(src&0x0F) << 16) |
		uint32(imm)
}

// packStringWords converts a string into u32 words (LE, zero-padded to 4B).
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
	// ══════════════════════════════════════════════════════════════
	// Build the init program (boot_demo.bin)
	// ══════════════════════════════════════════════════════════════

	type strEntry struct {
		label string
		data  string
	}
	strings := []strEntry{
		{"boot_banner", "UPC Boot v1.0\n"},
		{"shell_exit", "Shell exited\n"},
		{"init_done", "Init complete — system halted.\n"},
	}

	strLens := make(map[string]uint16)
	for _, s := range strings {
		strLens[s.label] = uint16(len(s.data))
	}

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

	emitPrint := func(label string) {
		emit(MOVI, 0, 0, SYS_WRITE)
		emit(MOVI, 1, 0, 1) // stdout
		emitWithStrRef(MOVI, 2, 0, label)
		emit(MOVI, 3, 0, strLens[label])
		emit(INT, 0, 0, 0x80)
	}

	// ── _start: Print boot banner ────────────────────────────────
	emitPrint("boot_banner") // 0-4

	// ── SYS_FORK ─────────────────────────────────────────────────
	emit(MOVI, 0, 0, SYS_FORK) // 5
	emit(INT, 0, 0, 0x80)      // 6

	// Save child_pid in r7 before CMP clobbers nothing but let's be safe
	emit(MOV, 7, 0, 0) // 7: r7 = fork return value

	// Check if child (r0 == 0)
	emit(MOVI, 6, 0, 0)                   // 8: r6 = 0
	emit(CMP, 0, 6, 0)                    // 9: CMP r0, r6
	childJmpInstr := emit(JZ, 0, 0, 0)    // 10: JZ child_process (patched)

	// ── Parent path ──────────────────────────────────────────────
	// SYS_WAITPID on child (r7 = child_pid)
	emit(MOVI, 0, 0, SYS_WAITPID) // 11
	emit(MOV, 1, 7, 0)            // 12: r1 = child_pid
	emit(INT, 0, 0, 0x80)         // 13

	// Print "Shell exited"
	emitPrint("shell_exit") // 14-18

	// Print "Init complete"
	emitPrint("init_done") // 19-23

	// SYS_EXIT(0)
	emit(MOVI, 0, 0, SYS_EXIT) // 24
	emit(MOVI, 1, 0, 0)        // 25
	emit(INT, 0, 0, 0x80)      // 26

	// ── Child path ───────────────────────────────────────────────
	childStart := len(code)

	// SYS_EXECVE: r1 = entry point of /bin/sh (word address in ROM)
	// The shell entry point will be patched after we know the layout.
	emit(MOVI, 0, 0, SYS_EXECVE)      // 27
	shellEntryInstr := emit(MOVI, 1, 0, 0) // 28: patched
	emit(INT, 0, 0, 0x80)             // 29
	// If execve succeeds, execution continues at the shell entry point.
	// If it fails (shouldn't happen), fall through to halt.
	emit(HALT, 0, 0, 0) // 30: fallback

	// ── Patch child JZ offset ────────────────────────────────────
	childOffset := int16(childStart - (childJmpInstr + 1))
	code[childJmpInstr] = encode(JZ, 0, 0, uint16(childOffset))

	// ── Data section ─────────────────────────────────────────────
	codeWords := len(code)
	dataByteOffset := uint16(codeWords * 4)

	strOffsets := make(map[string]uint16)
	currentOffset := uint16(0)
	for _, s := range strings {
		strOffsets[s.label] = dataByteOffset + currentOffset
		blen := uint16(len(s.data))
		aligned := (blen + 3) & ^uint16(3)
		currentOffset += aligned
	}

	// Patch string addresses
	for _, p := range patches {
		addr := strOffsets[p.label]
		code[p.instrIdx] = encode(MOVI, uint8((code[p.instrIdx]>>20)&0x0F), 0, addr)
	}

	// Pack data words
	var dataWords []uint32
	for _, s := range strings {
		dataWords = append(dataWords, packStringWords(s.data)...)
	}

	// ══════════════════════════════════════════════════════════════
	// Build the shell program (appended to ROM after init)
	// ══════════════════════════════════════════════════════════════

	shellPrompt := "> "
	shellBye := "Goodbye.\n"

	var shellCode []uint32
	shellEmit := func(opcode, dst, src uint8, imm uint16) int {
		idx := len(shellCode)
		shellCode = append(shellCode, encode(opcode, dst, src, imm))
		return idx
	}

	// Print prompt
	shellEmit(MOVI, 0, 0, SYS_WRITE)                     // sh+0
	shellEmit(MOVI, 1, 0, 1)                              // sh+1
	shellPromptAddrInstr := shellEmit(MOVI, 2, 0, 0)      // sh+2: patched
	shellEmit(MOVI, 3, 0, uint16(len(shellPrompt)))       // sh+3
	shellEmit(INT, 0, 0, 0x80)                            // sh+4

	// Read 1 byte from stdin
	shellEmit(MOVI, 0, 0, SYS_READ)                      // sh+5
	shellEmit(MOVI, 1, 0, 0)                              // sh+6: fd=stdin
	shellReadBufInstr := shellEmit(MOVI, 2, 0, 0)         // sh+7: patched
	shellEmit(MOVI, 3, 0, 1)                              // sh+8
	shellEmit(INT, 0, 0, 0x80)                            // sh+9

	// If read returned 0, exit (EOF / no input)
	shellEmit(MOVI, 6, 0, 0)                              // sh+10
	shellEmit(CMP, 0, 6, 0)                               // sh+11
	shellExitJmpInstr := shellEmit(JZ, 0, 0, 0)           // sh+12: patched

	// Echo byte back
	shellEmit(MOVI, 0, 0, SYS_WRITE)                     // sh+13
	shellEmit(MOVI, 1, 0, 1)                              // sh+14
	// reuse readbuf addr
	shellEchoBufInstr := shellEmit(MOVI, 2, 0, 0)         // sh+15: patched
	shellEmit(MOVI, 3, 0, 1)                              // sh+16
	shellEmit(INT, 0, 0, 0x80)                            // sh+17

	// Loop back to prompt
	shellLoopOffset := int16(0 - (len(shellCode) + 1))
	shellEmit(JMP, 0, 0, uint16(shellLoopOffset))         // sh+18

	// Exit path: print bye and exit
	shellExitStart := len(shellCode)
	shellEmit(MOVI, 0, 0, SYS_WRITE)                     // sh+19
	shellEmit(MOVI, 1, 0, 1)                              // sh+20
	shellByeAddrInstr := shellEmit(MOVI, 2, 0, 0)         // sh+21: patched
	shellEmit(MOVI, 3, 0, uint16(len(shellBye)))          // sh+22
	shellEmit(INT, 0, 0, 0x80)                            // sh+23

	shellEmit(MOVI, 0, 0, SYS_EXIT)                      // sh+24
	shellEmit(MOVI, 1, 0, 0)                              // sh+25
	shellEmit(INT, 0, 0, 0x80)                            // sh+26

	// Patch shell exit JZ offset
	shellExitOffset := int16(shellExitStart - (shellExitJmpInstr + 1))
	shellCode[shellExitJmpInstr] = encode(JZ, 0, 0, uint16(shellExitOffset))

	// Shell data section
	shellCodeWords := len(shellCode)
	shellDataByteBase := uint16(shellCodeWords * 4) // relative to shell start

	promptWords := packStringWords(shellPrompt)
	byeWords := packStringWords(shellBye)
	readBufWord := uint32(0) // 1 word read buffer

	promptAddr := shellDataByteBase
	byeAddr := shellDataByteBase + uint16(len(promptWords)*4)
	readBufAddr := byeAddr + uint16(len(byeWords)*4)

	shellCode[shellPromptAddrInstr] = encode(MOVI, 2, 0, promptAddr)
	shellCode[shellReadBufInstr] = encode(MOVI, 2, 0, readBufAddr)
	shellCode[shellEchoBufInstr] = encode(MOVI, 2, 0, readBufAddr)
	shellCode[shellByeAddrInstr] = encode(MOVI, 2, 0, byeAddr)

	var shellDataWords []uint32
	shellDataWords = append(shellDataWords, promptWords...)
	shellDataWords = append(shellDataWords, byeWords...)
	shellDataWords = append(shellDataWords, readBufWord)

	shellAllWords := append(shellCode, shellDataWords...)
	shellBinary := wordsToBytes(shellAllWords)

	// ══════════════════════════════════════════════════════════════
	// Combine init code + data, then append shell
	// ══════════════════════════════════════════════════════════════

	initBin := append(code, dataWords...)

	// Shell entry point in the combined ROM (word address)
	shellEntryWordAddr := uint16(len(initBin))
	code[shellEntryInstr] = encode(MOVI, 1, 0, shellEntryWordAddr)

	// Relocate shell data addresses: add (shellEntryWordAddr * 4) to each
	// data address in the shell code since it will execute at that offset.
	shellBaseByteOffset := uint16(shellEntryWordAddr * 4)
	shellCode[shellPromptAddrInstr] = encode(MOVI, 2, 0, promptAddr+shellBaseByteOffset)
	shellCode[shellReadBufInstr] = encode(MOVI, 2, 0, readBufAddr+shellBaseByteOffset)
	shellCode[shellEchoBufInstr] = encode(MOVI, 2, 0, readBufAddr+shellBaseByteOffset)
	shellCode[shellByeAddrInstr] = encode(MOVI, 2, 0, byeAddr+shellBaseByteOffset)

	// Rebuild combined binary with patched addresses
	combined := append(code, dataWords...)
	shellAllWordsRelocated := append(shellCode, shellDataWords...)
	combined = append(combined, shellAllWordsRelocated...)

	// ══════════════════════════════════════════════════════════════
	// Write boot_demo.bin (kernel image)
	// ══════════════════════════════════════════════════════════════

	kernelFile, err := os.Create("boot_demo.bin")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer kernelFile.Close()

	if err := writeLE32(kernelFile, combined); err != nil {
		fmt.Fprintf(os.Stderr, "write error: %v\n", err)
		os.Exit(1)
	}

	// ══════════════════════════════════════════════════════════════
	// Build and write ramdisk.img (UNFS filesystem)
	// ══════════════════════════════════════════════════════════════

	fs := formatFilesystem()
	addFile(fs, "/init", wordsToBytes(combined), 0o755, UNFSTypeFile)
	addFile(fs, "/bin/sh", shellBinary, 0o755, UNFSTypeFile)
	addFile(fs, "/dev/console", nil, 0o666, UNFSTypeDevice)
	addFile(fs, "/etc/hostname", []byte("upc\n"), 0o644, UNFSTypeFile)

	ramdiskFile, err := os.Create("ramdisk.img")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating ramdisk: %v\n", err)
		os.Exit(1)
	}
	defer ramdiskFile.Close()

	if err := writeLE32(ramdiskFile, fs); err != nil {
		fmt.Fprintf(os.Stderr, "ramdisk write error: %v\n", err)
		os.Exit(1)
	}

	// ══════════════════════════════════════════════════════════════
	// Summary
	// ══════════════════════════════════════════════════════════════

	totalWords := len(combined)
	fmt.Printf("Generated boot_demo.bin: %d total words (%d bytes)\n", totalWords, totalWords*4)
	fmt.Printf("  Init code:  %d instructions (%d bytes)\n", codeWords, codeWords*4)
	fmt.Printf("  Init data:  %d words (%d bytes)\n", len(dataWords), len(dataWords)*4)
	fmt.Printf("  Shell code: %d instructions (%d bytes)\n", shellCodeWords, shellCodeWords*4)
	fmt.Printf("  Shell data: %d words (%d bytes)\n", len(shellDataWords), len(shellDataWords)*4)
	fmt.Printf("  Shell entry: word %d (byte 0x%04X)\n", shellEntryWordAddr, shellEntryWordAddr*4)
	fmt.Printf("  Strings:\n")
	for _, s := range strings {
		fmt.Printf("    %-14s @ 0x%04X  (%d bytes)\n", s.label, strOffsets[s.label], len(s.data))
	}
	fmt.Printf("  Child process @ instruction %d (JZ offset %+d)\n", childStart, childOffset)
	fmt.Println()
	fmt.Printf("Generated ramdisk.img: %d words (%d bytes, %d KB)\n",
		len(fs), len(fs)*4, len(fs)*4/1024)
	fmt.Printf("  Files: /init, /bin/sh, /dev/console, /etc/hostname\n")
	fmt.Println()
	fmt.Println("FUZIX Boot Sequence:")
	fmt.Println("  1. Init prints \"UPC Boot v1.0\"")
	fmt.Println("  2. Init calls SYS_FORK")
	fmt.Println("  3. Child calls SYS_EXECVE -> /bin/sh entry point")
	fmt.Println("  4. Shell prints \"> \" prompt, reads stdin, echoes")
	fmt.Println("  5. On EOF: shell prints \"Goodbye.\" and SYS_EXIT(0)")
	fmt.Println("  6. Parent SYS_WAITPID -> prints \"Shell exited\"")
	fmt.Println("  7. Init prints \"Init complete\" and SYS_EXIT(0)")
	fmt.Println()
	fmt.Println("Boot with: wotan-ctl boot --kernel boot_demo.bin --ramdisk ramdisk.img")
}

// ── Filesystem helpers (self-contained, mirrors pkg/upc/filesystem.go) ──

func formatFilesystem() []uint32 {
	totalWords := UNFSTotalBlocks * UNFSBlockWords
	fs := make([]uint32, totalWords)
	fs[0] = UNFSMagic
	fs[1] = UNFSVersion
	fs[2] = UNFSTotalBlocks
	fs[3] = UNFSTotalBlocks - UNFSFirstDataBlock
	fs[4] = 1 // root inode
	fs[5] = UNFSFirstDataBlock
	return fs
}

func addFile(fs []uint32, name string, data []byte, mode uint32, fileType uint32) {
	// Find free inode
	inodeNum := -1
	for i := 0; i < UNFSMaxInodes; i++ {
		off := inodeWordOffset(i)
		if fs[off] == 0 { // name[0] == 0 means free
			inodeNum = i
			break
		}
	}
	if inodeNum < 0 {
		fmt.Fprintf(os.Stderr, "no free inodes for %s\n", name)
		os.Exit(1)
	}

	blocksNeeded := uint32(0)
	if len(data) > 0 {
		blocksNeeded = (uint32(len(data)) + UNFSBlockSize - 1) / UNFSBlockSize
	}

	// Find contiguous free blocks (simple: use first_data + scan)
	blockStart := findFreeBlock(fs, blocksNeeded)

	// Write inode
	off := inodeWordOffset(inodeNum)
	nameBytes := [28]byte{}
	copy(nameBytes[:], name)
	for i := 0; i < 7; i++ {
		fs[off+i] = binary.LittleEndian.Uint32(nameBytes[i*4 : i*4+4])
	}
	fs[off+7] = uint32(len(data))
	fs[off+8] = mode
	fs[off+9] = blockStart
	fs[off+10] = blocksNeeded
	fs[off+11] = 1710460800 // created
	fs[off+12] = 1710460800 // modified
	fs[off+13] = fileType

	// Write data
	if len(data) > 0 {
		padded := make([]byte, len(data))
		copy(padded, data)
		for len(padded)%4 != 0 {
			padded = append(padded, 0)
		}
		wordOff := int(blockStart) * UNFSBlockWords
		for i := 0; i < len(padded)/4; i++ {
			fs[wordOff+i] = binary.LittleEndian.Uint32(padded[i*4 : i*4+4])
		}
	}

	// Update superblock free count
	fs[3] -= blocksNeeded
}

func inodeWordOffset(inodeNum int) int {
	block := 1 + (inodeNum / UNFSInodesPerBlock)
	slot := inodeNum % UNFSInodesPerBlock
	return block*UNFSBlockWords + slot*(UNFSInodeSize/4)
}

func findFreeBlock(fs []uint32, count uint32) uint32 {
	if count == 0 {
		return 0
	}
	used := make(map[uint32]bool)
	for i := 0; i < UNFSMaxInodes; i++ {
		off := inodeWordOffset(i)
		if fs[off] == 0 {
			continue
		}
		start := fs[off+9]
		cnt := fs[off+10]
		for b := start; b < start+cnt; b++ {
			used[b] = true
		}
	}
	run := uint32(0)
	runStart := uint32(UNFSFirstDataBlock)
	for blk := uint32(UNFSFirstDataBlock); blk < UNFSTotalBlocks; blk++ {
		if used[blk] {
			run = 0
			runStart = blk + 1
		} else {
			run++
			if run >= count {
				return runStart
			}
		}
	}
	fmt.Fprintf(os.Stderr, "not enough free blocks: need %d\n", count)
	os.Exit(1)
	return 0
}

func wordsToBytes(words []uint32) []byte {
	out := make([]byte, len(words)*4)
	for i, w := range words {
		binary.LittleEndian.PutUint32(out[i*4:i*4+4], w)
	}
	return out
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
