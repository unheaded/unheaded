// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package upc

import (
	"encoding/binary"
	"fmt"
)

// UNFS — Unheaded Nano Filesystem
//
// A minimal flat filesystem for the UPC ramdisk. Stored in the ramdisk
// region of RAM_MAP at RAMDISK_BASE_WORD. Layout:
//
//	Block 0:     Superblock
//	Block 1-63:  Inode table (504 inodes, 8 per block)
//	Block 64+:   Data blocks (512 bytes each)
//
// Each block is 512 bytes. Total capacity: 8192 blocks = 4 MB.

const (
	// UNFSMagic is the superblock magic: "UNFS" in little-endian.
	UNFSMagic = 0x554E4653

	// UNFSVersion is the filesystem format version.
	UNFSVersion = 1

	// UNFSBlockSize is the size of one block in bytes.
	UNFSBlockSize = 512

	// UNFSBlockWords is the number of uint32 words per block.
	UNFSBlockWords = UNFSBlockSize / 4

	// UNFSTotalBlocks is the total number of blocks in the filesystem (4 MB).
	UNFSTotalBlocks = 8192

	// UNFSInodeBlocks is the number of blocks reserved for the inode table.
	UNFSInodeBlocks = 63

	// UNFSInodesPerBlock is how many inodes fit in one block (512 / 64 = 8).
	UNFSInodesPerBlock = 8

	// UNFSMaxInodes is the total number of inodes.
	UNFSMaxInodes = UNFSInodeBlocks * UNFSInodesPerBlock // 504

	// UNFSFirstDataBlock is the first block available for file data.
	UNFSFirstDataBlock = 64

	// UNFSInodeSize is the size of one inode in bytes.
	UNFSInodeSize = 64

	// UNFSNameLen is the maximum filename length (including null terminator).
	UNFSNameLen = 28

	// File types.
	UNFSTypeFile   = 1
	UNFSTypeDir    = 2
	UNFSTypeDevice = 3
)

// Superblock holds the UNFS filesystem metadata in block 0.
type Superblock struct {
	Magic     uint32 // 0x554E4653 ("UNFS")
	Version   uint32
	TotalBlks uint32
	FreeBlks  uint32
	RootInode uint32
	FirstData uint32
}

// Inode represents a single file or directory entry.
// Each inode is exactly 64 bytes on disk.
type Inode struct {
	Name       [28]byte
	Size       uint32
	Mode       uint32
	BlockStart uint32
	BlockCount uint32
	Created    uint32
	Modified   uint32
	FileType   uint32 // 1=file, 2=dir, 3=device
	Reserved   [8]byte
}

// FormatFilesystem creates an empty UNFS filesystem image as []uint32 words
// suitable for loading into RAM_MAP at RAMDISK_BASE_WORD.
func FormatFilesystem() []uint32 {
	totalWords := UNFSTotalBlocks * UNFSBlockWords
	fs := make([]uint32, totalWords)

	// Write superblock at block 0
	sb := Superblock{
		Magic:     UNFSMagic,
		Version:   UNFSVersion,
		TotalBlks: UNFSTotalBlocks,
		FreeBlks:  UNFSTotalBlocks - UNFSFirstDataBlock,
		RootInode: 1,
		FirstData: UNFSFirstDataBlock,
	}
	writeSuperblock(fs, sb)

	return fs
}

// AddFile adds a file to the filesystem image.
// Returns the inode number (0-based within the inode table) or error.
func AddFile(fs []uint32, name string, data []byte, mode uint32) (int, error) {
	return AddFileWithType(fs, name, data, mode, UNFSTypeFile)
}

// AddFileWithType adds a file with a specified type to the filesystem image.
func AddFileWithType(fs []uint32, name string, data []byte, mode uint32, fileType uint32) (int, error) {
	if len(name) == 0 {
		return -1, fmt.Errorf("filename is empty")
	}
	if len(name) >= UNFSNameLen {
		return -1, fmt.Errorf("filename too long: %d bytes (max %d)", len(name), UNFSNameLen-1)
	}

	// Read superblock
	sb := readSuperblock(fs)
	if sb.Magic != UNFSMagic {
		return -1, fmt.Errorf("invalid UNFS magic: 0x%08X", sb.Magic)
	}

	// Find a free inode slot
	inodeNum := -1
	for i := 0; i < UNFSMaxInodes; i++ {
		inode := readInode(fs, i)
		if inode.Name[0] == 0 {
			inodeNum = i
			break
		}
	}
	if inodeNum < 0 {
		return -1, fmt.Errorf("no free inodes")
	}

	// Calculate blocks needed for data
	blocksNeeded := uint32(0)
	if len(data) > 0 {
		blocksNeeded = (uint32(len(data)) + UNFSBlockSize - 1) / UNFSBlockSize
	}

	if blocksNeeded > sb.FreeBlks {
		return -1, fmt.Errorf("not enough free blocks: need %d, have %d", blocksNeeded, sb.FreeBlks)
	}

	// Find contiguous free data blocks
	blockStart := findFreeBlocks(fs, sb.FirstData, blocksNeeded)
	if blockStart == 0 && blocksNeeded > 0 {
		return -1, fmt.Errorf("cannot find %d contiguous free blocks", blocksNeeded)
	}

	// Build inode
	inode := Inode{
		Size:       uint32(len(data)),
		Mode:       mode,
		BlockStart: blockStart,
		BlockCount: blocksNeeded,
		Created:    1710460800, // 2024-03-15 00:00:00 UTC
		Modified:   1710460800,
		FileType:   fileType,
	}
	copy(inode.Name[:], name)

	// Write inode
	writeInode(fs, inodeNum, inode)

	// Write data blocks
	if len(data) > 0 {
		writeData(fs, blockStart, data)
	}

	// Update superblock
	sb.FreeBlks -= blocksNeeded
	writeSuperblock(fs, sb)

	return inodeNum, nil
}

// ReadFile reads a file's data from the filesystem by name.
// Returns the data bytes or error if the file is not found.
func ReadFile(fs []uint32, name string) ([]byte, error) {
	for i := 0; i < UNFSMaxInodes; i++ {
		inode := readInode(fs, i)
		if inode.Name[0] == 0 {
			continue
		}
		inodeName := inodeNameString(inode)
		if inodeName == name {
			return readData(fs, inode.BlockStart, inode.Size), nil
		}
	}
	return nil, fmt.Errorf("file not found: %s", name)
}

// ListFiles returns the names of all files in the filesystem.
func ListFiles(fs []uint32) []string {
	var names []string
	for i := 0; i < UNFSMaxInodes; i++ {
		inode := readInode(fs, i)
		if inode.Name[0] == 0 {
			continue
		}
		names = append(names, inodeNameString(inode))
	}
	return names
}

// LookupInode returns the inode for a given filename, or nil if not found.
func LookupInode(fs []uint32, name string) *Inode {
	for i := 0; i < UNFSMaxInodes; i++ {
		inode := readInode(fs, i)
		if inode.Name[0] == 0 {
			continue
		}
		if inodeNameString(inode) == name {
			return &inode
		}
	}
	return nil
}

// CreateBootFilesystem builds a filesystem with essential files for FUZIX boot:
//
//	/init     — simple init program (prints "UNFS boot" and spawns shell)
//	/bin/sh   — shell stub (reads stdin, echos, handles "exit")
//	/dev/console — device node (type=3)
//	/etc/hostname — contains "upc\n"
func CreateBootFilesystem() ([]uint32, error) {
	fs := FormatFilesystem()

	// /init — a small MBC program that prints "UNFS boot\n" then calls SYS_EXIT
	initCode := buildInitProgram()
	if _, err := AddFile(fs, "/init", initCode, 0o755); err != nil {
		return nil, fmt.Errorf("add /init: %w", err)
	}

	// /bin/sh — a shell stub program
	shellCode := buildShellProgram()
	if _, err := AddFile(fs, "/bin/sh", shellCode, 0o755); err != nil {
		return nil, fmt.Errorf("add /bin/sh: %w", err)
	}

	// /dev/console — device node (type=3, no data)
	if _, err := AddFileWithType(fs, "/dev/console", nil, 0o666, UNFSTypeDevice); err != nil {
		return nil, fmt.Errorf("add /dev/console: %w", err)
	}

	// /etc/hostname — contains "upc\n"
	if _, err := AddFile(fs, "/etc/hostname", []byte("upc\n"), 0o644); err != nil {
		return nil, fmt.Errorf("add /etc/hostname: %w", err)
	}

	return fs, nil
}

// ── Internal helpers ─────────────────────────────────────────────────

func writeSuperblock(fs []uint32, sb Superblock) {
	fs[0] = sb.Magic
	fs[1] = sb.Version
	fs[2] = sb.TotalBlks
	fs[3] = sb.FreeBlks
	fs[4] = sb.RootInode
	fs[5] = sb.FirstData
}

func readSuperblock(fs []uint32) Superblock {
	return Superblock{
		Magic:     fs[0],
		Version:   fs[1],
		TotalBlks: fs[2],
		FreeBlks:  fs[3],
		RootInode: fs[4],
		FirstData: fs[5],
	}
}

// inodeWordOffset returns the word offset in the fs slice for inodeNum.
// Inodes live in blocks 1..63, each block has 8 inodes of 64 bytes (16 words).
func inodeWordOffset(inodeNum int) int {
	block := 1 + (inodeNum / UNFSInodesPerBlock)
	slot := inodeNum % UNFSInodesPerBlock
	return block*UNFSBlockWords + slot*(UNFSInodeSize/4)
}

func writeInode(fs []uint32, inodeNum int, inode Inode) {
	off := inodeWordOffset(inodeNum)

	// Name: 28 bytes = 7 words
	for i := 0; i < 7; i++ {
		fs[off+i] = binary.LittleEndian.Uint32(inode.Name[i*4 : i*4+4])
	}

	fs[off+7] = inode.Size
	fs[off+8] = inode.Mode
	fs[off+9] = inode.BlockStart
	fs[off+10] = inode.BlockCount
	fs[off+11] = inode.Created
	fs[off+12] = inode.Modified
	fs[off+13] = inode.FileType

	// Reserved: 8 bytes = 2 words
	fs[off+14] = 0
	fs[off+15] = 0
}

func readInode(fs []uint32, inodeNum int) Inode {
	off := inodeWordOffset(inodeNum)
	var inode Inode

	// Name: 28 bytes = 7 words
	for i := 0; i < 7; i++ {
		binary.LittleEndian.PutUint32(inode.Name[i*4:i*4+4], fs[off+i])
	}

	inode.Size = fs[off+7]
	inode.Mode = fs[off+8]
	inode.BlockStart = fs[off+9]
	inode.BlockCount = fs[off+10]
	inode.Created = fs[off+11]
	inode.Modified = fs[off+12]
	inode.FileType = fs[off+13]

	return inode
}

func inodeNameString(inode Inode) string {
	for i, b := range inode.Name {
		if b == 0 {
			return string(inode.Name[:i])
		}
	}
	return string(inode.Name[:])
}

// findFreeBlocks finds the first contiguous run of count free data blocks.
// A block is considered free if it is not referenced by any inode.
// Returns the starting block number, or 0 if not found (and count > 0).
func findFreeBlocks(fs []uint32, firstData, count uint32) uint32 {
	if count == 0 {
		return 0
	}

	// Build a used-block set from all inodes
	used := make(map[uint32]bool)
	for i := 0; i < UNFSMaxInodes; i++ {
		inode := readInode(fs, i)
		if inode.Name[0] == 0 {
			continue
		}
		for b := inode.BlockStart; b < inode.BlockStart+inode.BlockCount; b++ {
			used[b] = true
		}
	}

	// Scan for a contiguous run starting at firstData
	run := uint32(0)
	runStart := firstData
	for blk := firstData; blk < UNFSTotalBlocks; blk++ {
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
	return 0
}

// writeData writes raw byte data into data blocks starting at blockStart.
func writeData(fs []uint32, blockStart uint32, data []byte) {
	// Pad to 4-byte alignment for word conversion
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

// readData reads size bytes from data blocks starting at blockStart.
func readData(fs []uint32, blockStart, size uint32) []byte {
	if size == 0 {
		return nil
	}
	wordOff := int(blockStart) * UNFSBlockWords
	wordCount := (int(size) + 3) / 4
	data := make([]byte, wordCount*4)
	for i := 0; i < wordCount; i++ {
		binary.LittleEndian.PutUint32(data[i*4:i*4+4], fs[wordOff+i])
	}
	return data[:size]
}

// ── MBC program builders ────────────────────────────────────────────

// MBC opcodes (matches demos/mbc/fuzix_compat/build.go).
const (
	mbcMOVI = 0x0F
	mbcINT  = 0x17
	mbcHALT = 0xFF
)

// MBC syscall numbers.
const (
	mbcSysExit  = 1
	mbcSysWrite = 4
)

func mbcEncode(opcode, dst, src uint8, imm uint16) uint32 {
	return (uint32(opcode) << 24) |
		(uint32(dst&0x0F) << 20) |
		(uint32(src&0x0F) << 16) |
		uint32(imm)
}

func mbcPackString(s string) []uint32 {
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

func mbcWordsToBytes(words []uint32) []byte {
	out := make([]byte, len(words)*4)
	for i, w := range words {
		binary.LittleEndian.PutUint32(out[i*4:i*4+4], w)
	}
	return out
}

// buildInitProgram generates a small MBC binary that:
//  1. Prints "UNFS boot\n" via SYS_WRITE to stdout
//  2. Calls SYS_EXIT(0)
func buildInitProgram() []byte {
	msg := "UNFS boot\n"

	// Code instructions
	code := []uint32{
		mbcEncode(mbcMOVI, 0, 0, mbcSysWrite),   // r0 = SYS_WRITE
		mbcEncode(mbcMOVI, 1, 0, 1),              // r1 = fd 1 (stdout)
		mbcEncode(mbcMOVI, 2, 0, 0),              // r2 = msg addr (patched below)
		mbcEncode(mbcMOVI, 3, 0, uint16(len(msg))), // r3 = msg len
		mbcEncode(mbcINT, 0, 0, 0x80),             // INT 0x80
		mbcEncode(mbcMOVI, 0, 0, mbcSysExit),      // r0 = SYS_EXIT
		mbcEncode(mbcMOVI, 1, 0, 0),               // r1 = exit code 0
		mbcEncode(mbcINT, 0, 0, 0x80),             // INT 0x80
	}

	dataWords := mbcPackString(msg)
	dataOffset := uint16(len(code) * 4)
	code[2] = mbcEncode(mbcMOVI, 2, 0, dataOffset)

	all := append(code, dataWords...)
	return mbcWordsToBytes(all)
}

// buildShellProgram generates a minimal shell MBC binary that:
//  1. Prints "> " prompt via SYS_WRITE
//  2. Reads one byte from stdin via SYS_READ
//  3. Echos the byte back via SYS_WRITE
//  4. Loops back to prompt
//
// (The "exit" command handling would require string comparison; this stub
// simply loops reading/echoing. The full shell is in demos/mbc/shell/.)
func buildShellProgram() []byte {
	prompt := "> "

	code := []uint32{
		// Print prompt
		mbcEncode(mbcMOVI, 0, 0, mbcSysWrite),       // r0 = SYS_WRITE
		mbcEncode(mbcMOVI, 1, 0, 1),                  // r1 = fd 1 (stdout)
		mbcEncode(mbcMOVI, 2, 0, 0),                  // r2 = prompt addr (patched)
		mbcEncode(mbcMOVI, 3, 0, uint16(len(prompt))), // r3 = prompt len
		mbcEncode(mbcINT, 0, 0, 0x80),                 // INT 0x80

		// Read 1 byte from stdin into buffer area
		mbcEncode(mbcMOVI, 0, 0, 3),    // r0 = SYS_READ
		mbcEncode(mbcMOVI, 1, 0, 0),    // r1 = fd 0 (stdin)
		mbcEncode(mbcMOVI, 2, 0, 0),    // r2 = read buf addr (patched)
		mbcEncode(mbcMOVI, 3, 0, 1),    // r3 = count 1
		mbcEncode(mbcINT, 0, 0, 0x80),  // INT 0x80

		// Echo byte back to stdout
		mbcEncode(mbcMOVI, 0, 0, mbcSysWrite), // r0 = SYS_WRITE
		mbcEncode(mbcMOVI, 1, 0, 1),            // r1 = fd 1 (stdout)
		mbcEncode(mbcMOVI, 2, 0, 0),            // r2 = read buf addr (patched)
		mbcEncode(mbcMOVI, 3, 0, 1),            // r3 = count 1
		mbcEncode(mbcINT, 0, 0, 0x80),          // INT 0x80

		// Loop back to prompt (JMP offset = -16 relative)
		// JMP opcode = 0x20, imm16 = 0xFFF0 (-16 as unsigned)
		(0x20 << 24) | 0xFFF0, // JMP -16 (relative)
	}

	promptWords := mbcPackString(prompt)
	readBufWord := uint32(0) // 1 word for the read buffer

	dataOffset := uint16(len(code) * 4)
	promptAddr := dataOffset
	readBufAddr := dataOffset + uint16(len(promptWords)*4)

	// Patch addresses
	code[2] = mbcEncode(mbcMOVI, 2, 0, promptAddr)
	code[7] = mbcEncode(mbcMOVI, 2, 0, readBufAddr)
	code[12] = mbcEncode(mbcMOVI, 2, 0, readBufAddr)

	all := append(code, promptWords...)
	all = append(all, readBufWord) // read buffer
	return mbcWordsToBytes(all)
}
