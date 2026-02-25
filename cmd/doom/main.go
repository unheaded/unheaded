// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package main provides a standalone CLI for the Doom-over-IPv6 compute engine.
//
// This is a lightweight alternative to "wotan-ctl doom" that can be built and
// deployed independently. It provides the same subcommands:
//
//	doom load <rom.bin>     - Load MBC ROM, print summary
//	doom status             - Format CPU state as table
//	doom input <key_bitmap> - Parse hex bitmap, inject keys
//	doom reset              - Reset CPU to defaults
//	doom inject-tick        - Inject compute tick (stub)
//
// All BPF map interactions are dry-run unless real maps are available.
package main

import (
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"unheaded/internal/doom"
)

func main() {
	args := os.Args[1:]

	if len(args) == 0 {
		printUsage()
		os.Exit(0)
	}

	cmd := args[0]
	remaining := args[1:]

	var err error
	switch cmd {
	case "load":
		err = cmdLoad(remaining)
	case "status":
		err = cmdStatus(remaining)
	case "input":
		err = cmdInput(remaining)
	case "reset":
		err = cmdReset(remaining)
	case "inject-tick":
		err = cmdInjectTick(remaining)
	case "version":
		fmt.Println("doom (Doom-over-IPv6 CLI) dev")
	case "-h", "--help", "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown command %q\n\n", cmd)
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`doom - Doom-over-IPv6 compute engine CLI

Usage: doom <command> [flags]

Commands:
  load <rom.bin>     Load MBC ROM, print summary
  status             Show CPU state
  input <bitmap>     Inject keyboard input (hex bitmap)
  reset              Reset CPU to default state
  inject-tick        Inject compute tick (stub)
  version            Print version

Use "doom <command> --help" for more information.`)
}

func cmdLoad(args []string) error {
	showStats := false
	showHMAC := false
	hmacKey := "unheaded-doom-secret"

	// Simple flag parsing
	var romPath string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--stats", "-stats":
			showStats = true
		case "--hmac", "-hmac":
			showHMAC = true
		case "--hmac-key", "-hmac-key":
			if i+1 < len(args) {
				i++
				hmacKey = args[i]
			}
		case "-h", "--help":
			fmt.Println("Usage: doom load <rom.bin> [--stats] [--hmac] [--hmac-key <key>]")
			return nil
		default:
			if !strings.HasPrefix(args[i], "-") {
				romPath = args[i]
			}
		}
	}

	if romPath == "" {
		return fmt.Errorf("ROM file path is required (doom load <rom.bin>)")
	}

	rom, err := doom.LoadROM(romPath)
	if err != nil {
		return err
	}

	fmt.Printf("Loaded ROM: %s\n", romPath)
	fmt.Printf("  %s\n", rom.Summary())

	if showStats {
		fmt.Println()
		fmt.Printf("Statistics:\n")
		fmt.Printf("  Instructions: %d\n", rom.InstructionCount())
		sizeKB := float64(rom.SizeBytes()) / 1024.0
		fmt.Printf("  Size: %d bytes (%.1f KB)\n", rom.SizeBytes(), sizeKB)
		estSeconds := float64(rom.InstructionCount()) / 2_000_000.0
		fmt.Printf("  Est. runtime @ 2MHz: %.3f seconds\n", estSeconds)
	}

	if showHMAC {
		mac := rom.HMAC([]byte(hmacKey))
		fmt.Printf("\nHMAC-SHA256: %s\n", hex.EncodeToString(mac))
	}

	return nil
}

func cmdStatus(args []string) error {
	oneLiner := false
	for _, a := range args {
		switch a {
		case "--short", "-short":
			oneLiner = true
		case "-h", "--help":
			fmt.Println("Usage: doom status [--short]")
			return nil
		}
	}

	fmt.Println("[DRY-RUN] BPF CPU_MAP not available, showing default state")
	cpu := doom.DefaultCpuState()
	if oneLiner {
		fmt.Println(cpu.FormatOneLiner())
	} else {
		fmt.Print(cpu.FormatTable())
	}
	return nil
}

func cmdInput(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("key_bitmap (hex) is required (doom input <bitmap>)")
	}

	for _, a := range args {
		if a == "-h" || a == "--help" {
			fmt.Println("Usage: doom input <key_bitmap>")
			fmt.Println("  key_bitmap is a hex value, e.g., 0x03 = keys 0 and 1")
			return nil
		}
	}

	bitmap := args[0]
	events, err := doom.ParseKeyBitmap(bitmap)
	if err != nil {
		return err
	}

	if len(events) == 0 {
		fmt.Println("No keys to inject (bitmap is 0)")
		return nil
	}

	fmt.Printf("Parsed %d key events from bitmap %s:\n", len(events), bitmap)
	for _, ev := range events {
		fmt.Printf("  %s\n", ev.String())
	}
	fmt.Println("[DRY-RUN] BPF KBD_MAP not available, events not injected")
	return nil
}

func cmdReset(args []string) error {
	for _, a := range args {
		if a == "-h" || a == "--help" {
			fmt.Println("Usage: doom reset")
			return nil
		}
	}

	cpu := doom.DefaultCpuState()
	fmt.Println("[DRY-RUN] BPF CPU_MAP not available")
	fmt.Printf("[DRY-RUN] Would write default state:\n")
	fmt.Printf("  PC=0x%08X SP=0x%08X\n", cpu.PC, cpu.Regs[doom.RegSP])
	return nil
}

func cmdInjectTick(args []string) error {
	count := 1
	for _, a := range args {
		if a == "-h" || a == "--help" {
			fmt.Println("Usage: doom inject-tick [--count N]")
			return nil
		}
	}
	for i := 0; i < len(args); i++ {
		if args[i] == "--count" || args[i] == "-count" {
			if i+1 < len(args) {
				i++
				fmt.Sscanf(args[i], "%d", &count)
			}
		}
	}

	fmt.Printf("[STUB] Would inject %d tick(s) into the packet ring\n", count)
	fmt.Println("  Each tick: 6 hops x 16 insns = 96 instructions")
	fmt.Printf("  Total: %d instructions\n", count*96)
	return nil
}
