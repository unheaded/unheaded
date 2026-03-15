// SPDX-License-Identifier: GPL-3.0-or-later

// Generates rootfs.img — a UNFS filesystem image for MBC Linux boot.
//
// Contents:
//   /init         — PID 1 init process (from arch/mbc/boot/init/init.upcf)
//   /bin/sh       — interactive shell  (from demos/mbc/shell/shell.upcf)
//   /dev/console  — console device node (type=device)
//   /etc/hostname — hostname file ("mbc-linux\n")
//
// The image is a raw UNFS filesystem suitable for use as a ramdisk root.
// Boot with: --ramdisk rootfs.img --args "root=/dev/ram0"
package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
)

// ── UNFS constants (mirrors pkg/upc/filesystem.go) ──────────────────
const (
	unfsMagic          = 0x554E4653
	unfsVersion        = 1
	unfsBlockSize      = 512
	unfsBlockWords     = unfsBlockSize / 4
	unfsTotalBlocks    = 8192
	unfsInodeBlocks    = 63
	unfsInodesPerBlock = 8
	unfsMaxInodes      = unfsInodeBlocks * unfsInodesPerBlock
	unfsFirstDataBlock = 64
	unfsInodeSize      = 64
	unfsNameLen        = 28

	unfsTypeFile   = 1
	unfsTypeDir    = 2
	unfsTypeDevice = 3
)

type superblock struct {
	Magic     uint32
	Version   uint32
	TotalBlks uint32
	FreeBlks  uint32
	RootInode uint32
	FirstData uint32
}

type inode struct {
	Name       [28]byte
	Size       uint32
	Mode       uint32
	BlockStart uint32
	BlockCount uint32
	Created    uint32
	Modified   uint32
	FileType   uint32
	Reserved   [8]byte
}

func main() {
	// Locate project root (we expect to be run from the project root)
	initPath := filepath.Join("arch", "mbc", "boot", "init", "init.upcf")
	shellPath := filepath.Join("demos", "mbc", "shell", "shell.upcf")
	outputPath := filepath.Join("arch", "mbc", "boot", "rootfs.img")

	// Read init binary
	initBin, err := os.ReadFile(initPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading init: %v\n", err)
		fmt.Fprintf(os.Stderr, "hint: run 'cd arch/mbc/boot/init && go run build.go' first\n")
		os.Exit(1)
	}

	// Read shell binary
	shellBin, err := os.ReadFile(shellPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading shell: %v\n", err)
		fmt.Fprintf(os.Stderr, "hint: run 'cd demos/mbc/shell && go run build.go' first\n")
		os.Exit(1)
	}

	fmt.Printf("Building UNFS root filesystem:\n")
	fmt.Printf("  /init         %d bytes (from %s)\n", len(initBin), initPath)
	fmt.Printf("  /bin/sh       %d bytes (from %s)\n", len(shellBin), shellPath)
	fmt.Printf("  /dev/console  device node\n")
	fmt.Printf("  /etc/hostname 10 bytes\n")
	fmt.Println()

	// Format empty filesystem
	fs := formatFilesystem()

	// Add /init
	if _, err := addFile(fs, "/init", initBin, 0o755, unfsTypeFile); err != nil {
		fmt.Fprintf(os.Stderr, "error adding /init: %v\n", err)
		os.Exit(1)
	}

	// Add /bin/sh
	if _, err := addFile(fs, "/bin/sh", shellBin, 0o755, unfsTypeFile); err != nil {
		fmt.Fprintf(os.Stderr, "error adding /bin/sh: %v\n", err)
		os.Exit(1)
	}

	// Add /dev/console (device node, no data)
	if _, err := addFile(fs, "/dev/console", nil, 0o666, unfsTypeDevice); err != nil {
		fmt.Fprintf(os.Stderr, "error adding /dev/console: %v\n", err)
		os.Exit(1)
	}

	// Add /etc/hostname
	hostname := []byte("mbc-linux\n")
	if _, err := addFile(fs, "/etc/hostname", hostname, 0o644, unfsTypeFile); err != nil {
		fmt.Fprintf(os.Stderr, "error adding /etc/hostname: %v\n", err)
		os.Exit(1)
	}

	// Write filesystem image to disk
	imgBytes := fsToBytes(fs)
	if err := os.WriteFile(outputPath, imgBytes, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing %s: %v\n", outputPath, err)
		os.Exit(1)
	}

	fmt.Printf("Generated %s: %d bytes (%d blocks, %d KB)\n",
		outputPath, len(imgBytes), unfsTotalBlocks, len(imgBytes)/1024)
	fmt.Printf("  UNFS v%d, block size %d bytes\n", unfsVersion, unfsBlockSize)
	fmt.Println()
	fmt.Println("Root filesystem contents:")

	// List files
	for i := 0; i < unfsMaxInodes; i++ {
		ino := readInode(fs, i)
		if ino.Name[0] == 0 {
			continue
		}
		name := inodeNameStr(ino)
		typeStr := "file"
		switch ino.FileType {
		case unfsTypeDir:
			typeStr = "dir"
		case unfsTypeDevice:
			typeStr = "dev"
		}
		fmt.Printf("  %-20s %6d bytes  mode=%04o  type=%s  blocks=%d-%d\n",
			name, ino.Size, ino.Mode, typeStr,
			ino.BlockStart, ino.BlockStart+ino.BlockCount-1)
	}
	fmt.Println()
	fmt.Println("Boot with:")
	fmt.Println("  wotan-ctl boot --kernel <kernel> --ramdisk", outputPath,
		"--args \"console=ttyMBC0 root=/dev/ram0\"")
}

// ── Filesystem operations (standalone, mirrors pkg/upc/filesystem.go) ──

func formatFilesystem() []uint32 {
	totalWords := unfsTotalBlocks * unfsBlockWords
	fs := make([]uint32, totalWords)

	sb := superblock{
		Magic:     unfsMagic,
		Version:   unfsVersion,
		TotalBlks: unfsTotalBlocks,
		FreeBlks:  unfsTotalBlocks - unfsFirstDataBlock,
		RootInode: 1,
		FirstData: unfsFirstDataBlock,
	}
	writeSuperblock(fs, sb)
	return fs
}

func addFile(fs []uint32, name string, data []byte, mode uint32, fileType uint32) (int, error) {
	if len(name) >= unfsNameLen {
		return -1, fmt.Errorf("filename too long: %s", name)
	}

	sb := readSuperblock(fs)

	// Find free inode
	inodeNum := -1
	for i := 0; i < unfsMaxInodes; i++ {
		ino := readInode(fs, i)
		if ino.Name[0] == 0 {
			inodeNum = i
			break
		}
	}
	if inodeNum < 0 {
		return -1, fmt.Errorf("no free inodes")
	}

	// Calculate blocks needed
	blocksNeeded := uint32(0)
	if len(data) > 0 {
		blocksNeeded = (uint32(len(data)) + unfsBlockSize - 1) / unfsBlockSize
	}

	if blocksNeeded > sb.FreeBlks {
		return -1, fmt.Errorf("not enough free blocks")
	}

	// Find contiguous free blocks
	blockStart := findFreeBlocks(fs, sb.FirstData, blocksNeeded)
	if blockStart == 0 && blocksNeeded > 0 {
		return -1, fmt.Errorf("no contiguous blocks")
	}

	// Build and write inode
	ino := inode{
		Size:       uint32(len(data)),
		Mode:       mode,
		BlockStart: blockStart,
		BlockCount: blocksNeeded,
		Created:    1742083200, // 2025-03-15 (approx)
		Modified:   1742083200,
		FileType:   fileType,
	}
	copy(ino.Name[:], name)
	writeInode(fs, inodeNum, ino)

	// Write data
	if len(data) > 0 {
		writeData(fs, blockStart, data)
	}

	// Update superblock
	sb.FreeBlks -= blocksNeeded
	writeSuperblock(fs, sb)

	return inodeNum, nil
}

func writeSuperblock(fs []uint32, sb superblock) {
	fs[0] = sb.Magic
	fs[1] = sb.Version
	fs[2] = sb.TotalBlks
	fs[3] = sb.FreeBlks
	fs[4] = sb.RootInode
	fs[5] = sb.FirstData
}

func readSuperblock(fs []uint32) superblock {
	return superblock{
		Magic:     fs[0],
		Version:   fs[1],
		TotalBlks: fs[2],
		FreeBlks:  fs[3],
		RootInode: fs[4],
		FirstData: fs[5],
	}
}

func inodeWordOffset(inodeNum int) int {
	block := 1 + (inodeNum / unfsInodesPerBlock)
	slot := inodeNum % unfsInodesPerBlock
	return block*unfsBlockWords + slot*(unfsInodeSize/4)
}

func writeInode(fs []uint32, inodeNum int, ino inode) {
	off := inodeWordOffset(inodeNum)
	for i := 0; i < 7; i++ {
		fs[off+i] = binary.LittleEndian.Uint32(ino.Name[i*4 : i*4+4])
	}
	fs[off+7] = ino.Size
	fs[off+8] = ino.Mode
	fs[off+9] = ino.BlockStart
	fs[off+10] = ino.BlockCount
	fs[off+11] = ino.Created
	fs[off+12] = ino.Modified
	fs[off+13] = ino.FileType
	fs[off+14] = 0
	fs[off+15] = 0
}

func readInode(fs []uint32, inodeNum int) inode {
	off := inodeWordOffset(inodeNum)
	var ino inode
	for i := 0; i < 7; i++ {
		binary.LittleEndian.PutUint32(ino.Name[i*4:i*4+4], fs[off+i])
	}
	ino.Size = fs[off+7]
	ino.Mode = fs[off+8]
	ino.BlockStart = fs[off+9]
	ino.BlockCount = fs[off+10]
	ino.Created = fs[off+11]
	ino.Modified = fs[off+12]
	ino.FileType = fs[off+13]
	return ino
}

func inodeNameStr(ino inode) string {
	for i, b := range ino.Name {
		if b == 0 {
			return string(ino.Name[:i])
		}
	}
	return string(ino.Name[:])
}

func findFreeBlocks(fs []uint32, firstData, count uint32) uint32 {
	if count == 0 {
		return 0
	}
	used := make(map[uint32]bool)
	for i := 0; i < unfsMaxInodes; i++ {
		ino := readInode(fs, i)
		if ino.Name[0] == 0 {
			continue
		}
		for b := ino.BlockStart; b < ino.BlockStart+ino.BlockCount; b++ {
			used[b] = true
		}
	}
	run := uint32(0)
	runStart := firstData
	for blk := firstData; blk < unfsTotalBlocks; blk++ {
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

func writeData(fs []uint32, blockStart uint32, data []byte) {
	padded := make([]byte, len(data))
	copy(padded, data)
	for len(padded)%4 != 0 {
		padded = append(padded, 0)
	}
	wordOff := int(blockStart) * unfsBlockWords
	for i := 0; i < len(padded)/4; i++ {
		fs[wordOff+i] = binary.LittleEndian.Uint32(padded[i*4 : i*4+4])
	}
}

func fsToBytes(fs []uint32) []byte {
	out := make([]byte, len(fs)*4)
	for i, w := range fs {
		binary.LittleEndian.PutUint32(out[i*4:i*4+4], w)
	}
	return out
}
