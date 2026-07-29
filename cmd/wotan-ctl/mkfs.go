// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"os"

	"unheaded/pkg/upc"
)

// newMkfsCmd creates the mkfs subcommand for formatting a UNFS ramdisk image.
func newMkfsCmd() *Command {
	var (
		output     string
		mapPinPath string
		dryRun     bool
	)

	flags := flag.NewFlagSet("mkfs", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.StringVar(&output, "output", "", "Write ramdisk image to file (e.g. ramdisk.img)")
	flags.StringVar(&mapPinPath, "map-pin", "/sys/fs/bpf/unheaded/doom-ring/maps", "BPF map pin directory")
	flags.BoolVar(&dryRun, "dry-run", false, "Show filesystem plan without writing")

	return &Command{
		Name:  "mkfs",
		Short: "Format a UNFS ramdisk filesystem",
		Long: `Create a UNFS (Unheaded Nano Filesystem) ramdisk image with boot files.

The mkfs command formats a 4MB ramdisk with a minimal filesystem containing
the files needed for FUZIX boot:
  /init         — init program (prints "UNFS boot", exits)
  /bin/sh       — shell stub (read/echo loop)
  /dev/console  — device node
  /etc/hostname — contains "upc"

The image can be written to a file (--output) and/or loaded directly into
the BPF RAM_MAP at the ramdisk base address.

Examples:
  # Write ramdisk image to file
  wotan-ctl mkfs --output ramdisk.img

  # Load directly into BPF maps
  wotan-ctl mkfs

  # Dry-run: show what would be created
  wotan-ctl mkfs --dry-run`,
		Usage:   "wotan-ctl mkfs [--output <path>] [--map-pin <path>] [--dry-run]",
		Aliases: []string{"format"},
		Flags:   flags,
		RunFunc: func(ctx *Context, args []string) error {
			if err := flags.Parse(args); err != nil {
				return err
			}
			return runMkfs(ctx, output, mapPinPath, dryRun)
		},
	}
}

// runMkfs formats a UNFS filesystem and writes it to output/BPF maps.
func runMkfs(ctx *Context, output, mapPinPath string, dryRun bool) error {
	fmt.Println("=== UNFS Filesystem Formatter ===")
	fmt.Println()

	// Create the boot filesystem
	fmt.Println("[1/3] Creating UNFS boot filesystem...")
	fs, err := upc.CreateBootFilesystem()
	if err != nil {
		return fmt.Errorf("create boot filesystem: %w", err)
	}

	// List files
	files := upc.ListFiles(fs)
	fmt.Printf("       Created %d files:\n", len(files))
	for _, name := range files {
		inode := upc.LookupInode(fs, name)
		typeStr := "file"
		switch inode.FileType {
		case upc.UNFSTypeDir:
			typeStr = "dir"
		case upc.UNFSTypeDevice:
			typeStr = "dev"
		}
		fmt.Printf("       %-20s %5d bytes  mode=%04o  type=%s\n",
			name, inode.Size, inode.Mode, typeStr)
	}

	// Superblock info
	fmt.Printf("\n[2/3] Superblock:\n")
	fmt.Printf("       Magic:       0x%08X (UNFS)\n", fs[0])
	fmt.Printf("       Version:     %d\n", fs[1])
	fmt.Printf("       Total:       %d blocks (%d KB)\n", fs[2], fs[2]*upc.UNFSBlockSize/1024)
	fmt.Printf("       Free:        %d blocks (%d KB)\n", fs[3], fs[3]*upc.UNFSBlockSize/1024)
	fmt.Printf("       First data:  block %d\n", fs[5])

	if dryRun {
		fmt.Println("\n[3/3] [DRY-RUN] No output written.")
		return nil
	}

	// Write to file if requested
	if output != "" {
		fmt.Printf("\n[3/3] Writing ramdisk image to %s...\n", output)
		if err := writeRamdiskImage(fs, output); err != nil {
			return fmt.Errorf("write image: %w", err)
		}
		fmt.Printf("       Wrote %d bytes (%d KB)\n",
			len(fs)*4, len(fs)*4/1024)
	}

	// Try to load into BPF RAM_MAP
	if output == "" {
		ramMap, bpfErr := openBPFMap(mapPinPath + "/RAM_MAP")
		if bpfErr != nil {
			fmt.Printf("\n[3/3] BPF maps not available: %v\n", bpfErr)
			fmt.Println("       Use --output to write to a file instead.")
			return nil
		}
		defer ramMap.Close()

		fmt.Printf("\n[3/3] Loading filesystem into RAM_MAP at word 0x%X...\n",
			upc.RamdiskBase>>2)

		ramdiskBaseWord := upc.RamdiskBase >> 2
		written := 0
		for i, word := range fs {
			if word == 0 {
				continue // Skip zero words for speed
			}
			addr := uint32(ramdiskBaseWord) + uint32(i)
			if err := ramMap.Update(addr, word); err != nil {
				return fmt.Errorf("RAM_MAP update at word 0x%X: %w", addr, err)
			}
			written++
		}
		fmt.Printf("       Wrote %d non-zero words to RAM_MAP\n", written)
	}

	fmt.Println()
	fmt.Println("=== UNFS Format Complete ===")
	return nil
}

// writeRamdiskImage writes the filesystem words to a binary file.
func writeRamdiskImage(fs []uint32, path string) error {
	f, err := os.Create(path) // #nosec G304 -- build/ROM artifact path from a configured or CLI-supplied location
	if err != nil {
		return err
	}
	defer f.Close()

	buf := make([]byte, 4)
	for _, word := range fs {
		binary.LittleEndian.PutUint32(buf, word)
		if _, err := f.Write(buf); err != nil {
			return err
		}
	}
	return nil
}
