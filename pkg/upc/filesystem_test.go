// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package upc

import (
	"testing"
)

func TestFormatFilesystem(t *testing.T) {
	fs := FormatFilesystem()

	// Check total size: 8192 blocks * 128 words/block
	expectedWords := UNFSTotalBlocks * UNFSBlockWords
	if len(fs) != expectedWords {
		t.Fatalf("fs size = %d words, want %d", len(fs), expectedWords)
	}

	// Verify superblock magic
	if fs[0] != UNFSMagic {
		t.Errorf("magic = 0x%08X, want 0x%08X", fs[0], UNFSMagic)
	}

	// Verify version
	if fs[1] != UNFSVersion {
		t.Errorf("version = %d, want %d", fs[1], UNFSVersion)
	}

	// Verify total blocks
	if fs[2] != UNFSTotalBlocks {
		t.Errorf("total blocks = %d, want %d", fs[2], UNFSTotalBlocks)
	}

	// Verify free blocks
	expectedFree := uint32(UNFSTotalBlocks - UNFSFirstDataBlock)
	if fs[3] != expectedFree {
		t.Errorf("free blocks = %d, want %d", fs[3], expectedFree)
	}

	// Verify root inode block
	if fs[4] != 1 {
		t.Errorf("root inode = %d, want 1", fs[4])
	}

	// Verify first data block
	if fs[5] != UNFSFirstDataBlock {
		t.Errorf("first data = %d, want %d", fs[5], UNFSFirstDataBlock)
	}
}

func TestAddFileAndReadBack(t *testing.T) {
	fs := FormatFilesystem()

	data := []byte("Hello, UNFS!\n")
	inodeNum, err := AddFile(fs, "hello.txt", data, 0o644)
	if err != nil {
		t.Fatalf("AddFile: %v", err)
	}
	if inodeNum != 0 {
		t.Errorf("inode number = %d, want 0 (first free)", inodeNum)
	}

	// Read it back
	got, err := ReadFile(fs, "hello.txt")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("ReadFile = %q, want %q", got, data)
	}
}

func TestAddMultipleFiles(t *testing.T) {
	fs := FormatFilesystem()

	files := map[string]string{
		"file1.txt": "First file\n",
		"file2.txt": "Second file with more data\n",
		"file3.txt": "Third\n",
	}

	for name, content := range files {
		if _, err := AddFile(fs, name, []byte(content), 0o644); err != nil {
			t.Fatalf("AddFile(%s): %v", name, err)
		}
	}

	// Verify all files
	for name, content := range files {
		got, err := ReadFile(fs, name)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", name, err)
		}
		if string(got) != content {
			t.Errorf("ReadFile(%s) = %q, want %q", name, got, content)
		}
	}

	// Verify file list
	names := ListFiles(fs)
	if len(names) != 3 {
		t.Errorf("ListFiles count = %d, want 3", len(names))
	}
}

func TestAddFileEmptyData(t *testing.T) {
	fs := FormatFilesystem()

	inodeNum, err := AddFileWithType(fs, "/dev/null", nil, 0o666, UNFSTypeDevice)
	if err != nil {
		t.Fatalf("AddFileWithType: %v", err)
	}

	inode := LookupInode(fs, "/dev/null")
	if inode == nil {
		t.Fatal("LookupInode returned nil")
	}
	if inode.FileType != UNFSTypeDevice {
		t.Errorf("file type = %d, want %d", inode.FileType, UNFSTypeDevice)
	}
	if inode.Size != 0 {
		t.Errorf("size = %d, want 0", inode.Size)
	}
	_ = inodeNum
}

func TestAddFileNameTooLong(t *testing.T) {
	fs := FormatFilesystem()
	longName := "this-name-is-way-too-long-for-unfs-inode"
	_, err := AddFile(fs, longName, []byte("data"), 0o644)
	if err == nil {
		t.Error("expected error for long filename")
	}
}

func TestAddFileEmptyName(t *testing.T) {
	fs := FormatFilesystem()
	_, err := AddFile(fs, "", []byte("data"), 0o644)
	if err == nil {
		t.Error("expected error for empty filename")
	}
}

func TestReadFileNotFound(t *testing.T) {
	fs := FormatFilesystem()
	_, err := ReadFile(fs, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestLookupInodeNotFound(t *testing.T) {
	fs := FormatFilesystem()
	inode := LookupInode(fs, "missing")
	if inode != nil {
		t.Error("expected nil for missing inode")
	}
}

func TestCreateBootFilesystem(t *testing.T) {
	fs, err := CreateBootFilesystem()
	if err != nil {
		t.Fatalf("CreateBootFilesystem: %v", err)
	}

	// Verify superblock
	if fs[0] != UNFSMagic {
		t.Errorf("magic = 0x%08X, want 0x%08X", fs[0], UNFSMagic)
	}

	// Verify all 4 required files exist
	requiredFiles := []struct {
		name     string
		fileType uint32
	}{
		{"/init", UNFSTypeFile},
		{"/bin/sh", UNFSTypeFile},
		{"/dev/console", UNFSTypeDevice},
		{"/etc/hostname", UNFSTypeFile},
	}

	for _, rf := range requiredFiles {
		inode := LookupInode(fs, rf.name)
		if inode == nil {
			t.Errorf("file %s not found in boot filesystem", rf.name)
			continue
		}
		if inode.FileType != rf.fileType {
			t.Errorf("file %s type = %d, want %d", rf.name, inode.FileType, rf.fileType)
		}
	}

	// Verify /etc/hostname content
	hostname, err := ReadFile(fs, "/etc/hostname")
	if err != nil {
		t.Fatalf("ReadFile(/etc/hostname): %v", err)
	}
	if string(hostname) != "upc\n" {
		t.Errorf("/etc/hostname = %q, want %q", hostname, "upc\n")
	}

	// Verify /init has non-zero size (it's an MBC binary)
	initInode := LookupInode(fs, "/init")
	if initInode.Size == 0 {
		t.Error("/init has zero size")
	}

	// Verify /bin/sh has non-zero size
	shInode := LookupInode(fs, "/bin/sh")
	if shInode.Size == 0 {
		t.Error("/bin/sh has zero size")
	}

	// Verify /dev/console has zero size (device node)
	consoleInode := LookupInode(fs, "/dev/console")
	if consoleInode.Size != 0 {
		t.Errorf("/dev/console size = %d, want 0", consoleInode.Size)
	}

	// Verify total file count
	files := ListFiles(fs)
	if len(files) != 4 {
		t.Errorf("file count = %d, want 4 (got: %v)", len(files), files)
	}
}

func TestInodeSize64Bytes(t *testing.T) {
	// Verify that the inode struct serialization uses exactly 64 bytes (16 words).
	// This is critical for the on-disk layout.
	fs := FormatFilesystem()
	AddFile(fs, "test", []byte("x"), 0o644)

	// Read the inode back and verify the next slot is clean
	inode0 := readInode(fs, 0)
	if inodeNameString(inode0) != "test" {
		t.Errorf("inode 0 name = %q, want %q", inodeNameString(inode0), "test")
	}

	// Inode 1 should be empty (name[0] == 0)
	inode1 := readInode(fs, 1)
	if inode1.Name[0] != 0 {
		t.Error("inode 1 should be empty after adding only one file")
	}
}

func TestFreeBlockCount(t *testing.T) {
	fs := FormatFilesystem()

	// Initially all data blocks are free
	sb := readSuperblock(fs)
	initialFree := sb.FreeBlks

	// Add a file that takes 1 block
	data := make([]byte, 100) // < 512, so 1 block
	AddFile(fs, "small", data, 0o644)

	sb = readSuperblock(fs)
	if sb.FreeBlks != initialFree-1 {
		t.Errorf("free blocks = %d, want %d", sb.FreeBlks, initialFree-1)
	}

	// Add a file that takes 2 blocks
	data = make([]byte, 600) // > 512, so 2 blocks
	AddFile(fs, "bigger", data, 0o644)

	sb = readSuperblock(fs)
	if sb.FreeBlks != initialFree-3 {
		t.Errorf("free blocks = %d, want %d", sb.FreeBlks, initialFree-3)
	}
}
