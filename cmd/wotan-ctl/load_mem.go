// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package main

import (
	"crypto/md5"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"
)

// newLoadMemCmd creates the load-mem subcommand.
func newLoadMemCmd() *Command {
	var (
		flowLabel  uint64
		baseAddr   uint64
		file       string
		warm       bool
		verify     bool
		wotanAddr  string
		mapPinPath string
		verbose    bool
	)

	flags := flag.NewFlagSet("load-mem", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.Uint64Var(&flowLabel, "flow-label", 0, "IPv6 flow label (can use hex: 0x0A3F7E)")
	flags.Uint64Var(&baseAddr, "base-addr", 0, "Base memory address (can use hex: 0x100000)")
	flags.StringVar(&file, "file", "", "Path to binary file (required)")
	flags.BoolVar(&warm, "warm", false, "Pre-stage memory pages to L1 cache")
	flags.BoolVar(&verify, "verify", false, "Verify memory after loading (checksum)")
	flags.StringVar(&wotanAddr, "wotan-addr", "http://localhost:18000", "Wotan server address")
	flags.StringVar(&mapPinPath, "map-pin", "/sys/fs/bpf", "BPF map pin directory")
	flags.BoolVar(&verbose, "verbose", false, "Enable verbose logging")

	cmd := &Command{
		Name:  "load-mem",
		Short: "Load binary data into Wotan's L2 ring buffer (RAM)",
		Long: `Load binary data into the Wotan L2 ring buffer (RAM).

The L2 ring buffer is the primary RAM for the Doom-over-IPv6 compute engine.
Data is split into pages and streamed to Wotan via HTTP POST.

Page sizes:
  - 64 bytes:  L1 cache line size
  - 4096 bytes: L2 ring buffer granularity

Examples:
  # Load WAD file at address 0x100000
  wotan-ctl load-mem --file doom1.wad --base-addr 0x100000 --flow-label 0x123456

  # Load and pre-warm L1 cache
  wotan-ctl load-mem --file sprite.bin --base-addr 0x200000 --flow-label 0x100 --warm

  # Load with verification
  wotan-ctl load-mem --file data.bin --base-addr 0x0 --flow-label 0x50 --verify --stats`,
		Usage:   "wotan-ctl load-mem --file <path> --base-addr <addr> --flow-label <label> [options]",
		Aliases: []string{"load-data", "load-wad"},
		Flags:   flags,
		RunFunc: func(ctx *Context, args []string) error {
			if err := flags.Parse(args); err != nil {
				return err
			}

			if file == "" {
				fmt.Fprintf(os.Stderr, "Error: --file is required\n")
				flags.Usage()
				os.Exit(1)
			}

			if verbose {
				// Note: logger not imported, would use ctx.Logger.SetLevel
			}

			return loadMem(ctx, flowLabel, baseAddr, file, wotanAddr, mapPinPath, warm, verify)
		},
	}

	return cmd
}

// loadMem loads binary data into the Wotan L2 ring buffer.
func loadMem(ctx *Context, flowLabel uint64, baseAddr uint64, filePath string, wotanAddr string, mapPinPath string, warm bool, verify bool) error {
	// 1. Read binary file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	fmt.Printf("Loading %d bytes from %s\n", len(data), filePath)
	fmt.Printf("  Flow Label: 0x%06X\n", uint32(flowLabel&0xFFFFFF))
	fmt.Printf("  Base Address: 0x%08X\n", uint32(baseAddr&0xFFFFFFFF))

	// 2. Try to connect to Wotan
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// 3. Split into 4096-byte pages (L2 ring buffer granularity)
	const pageSize = 4096
	totalPages := (len(data) + pageSize - 1) / pageSize

	fmt.Printf("Splitting into %d pages of %d bytes\n", totalPages, pageSize)

	// 4. Stream pages to Wotan
	// For now, implement as dry-run since actual Wotan HTTP API would need to be documented
	fmt.Println("\n[DRY-RUN] Would stream pages to Wotan:")

	checksum := md5.New()
	for i := 0; i < totalPages; i++ {
		offset := i * pageSize
		end := offset + pageSize
		if end > len(data) {
			end = len(data)
		}

		page := data[offset:end]
		checksum.Write(page)

		pageAddr := baseAddr + uint64(offset)
		fmt.Printf("  Page %4d: addr=0x%08X size=%5d POST /wotan/memory\n",
			i, uint32(pageAddr&0xFFFFFFFF), len(page))
	}

	checksumHash := checksum.Sum(nil)
	fmt.Printf("\nChecksum (MD5): %x\n", checksumHash)

	// 5. --warm: pre-stage memory to L1 (would trigger warm request)
	if warm {
		fmt.Println("\n[DRY-RUN] Would trigger L1 cache warming for first N pages")
		fmt.Println("  Command: POST /wotan/memory/warm")
	}

	// 6. --verify: re-read and compare
	if verify {
		fmt.Println("\n[DRY-RUN] Would verify memory after loading")
		fmt.Println("  Command: GET /wotan/memory range (checksum comparison)")
	}

	// Note: Actual implementation would:
	// 1. POST each page to Wotan HTTP API
	// 2. Handle responses and errors
	// 3. Optionally trigger L1 cache warming
	// 4. Optionally verify loaded data via checksum or read-back

	_ = client // Suppress unused warning

	fmt.Println("\nNote: Full Wotan HTTP API integration requires documented endpoints:")
	fmt.Println("  - POST /wotan/memory/{flow_label}/{address} - Store page")
	fmt.Println("  - GET /wotan/memory/{flow_label}/{address} - Read page")
	fmt.Println("  - POST /wotan/memory/warm/{flow_label} - Warm L1 cache")

	return nil
}
