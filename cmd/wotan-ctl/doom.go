// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"os"

	"unheaded/internal/doom"
)

// newDoomCmd creates the doom subcommand group.
func newDoomCmd() *Command {
	cmd := &Command{
		Name:  "doom",
		Short: "Doom-over-IPv6 compute engine management",
		Long: `Doom-over-IPv6 compute engine management commands.

Subcommands:
  load          Load MBC ROM into BPF rom_map
  status        Show current CPU state
  input         Inject keyboard input
  reset         Reset CPU to default state
  inject-tick   Inject a compute tick (stub)

Use "wotan-ctl doom <command> --help" for more information.`,
		Usage:       "wotan-ctl doom <command> [flags]",
		SubCommands: make(map[string]*Command),
	}

	cmd.SubCommands["load"] = newDoomLoadCmd()
	cmd.SubCommands["status"] = newDoomStatusCmd()
	cmd.SubCommands["input"] = newDoomInputCmd()
	cmd.SubCommands["reset"] = newDoomResetCmd()
	cmd.SubCommands["inject-tick"] = newDoomInjectTickCmd()

	return cmd
}

// newDoomLoadCmd creates the "doom load <rom.bin>" subcommand.
func newDoomLoadCmd() *Command {
	var (
		showStats  bool
		showHMAC   bool
		hmacKey    string
		mapPinPath string
	)

	flags := flag.NewFlagSet("doom load", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.BoolVar(&showStats, "stats", false, "Print instruction count and estimated runtime")
	flags.BoolVar(&showHMAC, "hmac", false, "Print HMAC-SHA256 of ROM bytes")
	flags.StringVar(&hmacKey, "hmac-key", "unheaded-doom-secret", "HMAC key (hex or ASCII)")
	flags.StringVar(&mapPinPath, "map-pin", "/sys/fs/bpf/unheaded/doom-ring/maps", "BPF map pin directory")

	return &Command{
		Name:  "load",
		Short: "Load MBC ROM into BPF rom_map",
		Long: `Load MBC bytecode ROM into the BPF rom_map (instruction memory).

The ROM file must be a sequence of little-endian u32 instruction words.
File size must be a multiple of 4 and cannot exceed 1 MiB (262144 instructions).

Examples:
  wotan-ctl doom load program.mbc
  wotan-ctl doom load doom1.mbc --stats
  wotan-ctl doom load doom1.mbc --hmac`,
		Usage:   "wotan-ctl doom load <rom.bin> [flags]",
		Aliases: []string{"load-rom"},
		Flags:   flags,
		RunFunc: func(ctx *Context, args []string) error {
			if err := flags.Parse(args); err != nil {
				return err
			}

			remaining := flags.Args()
			if len(remaining) < 1 {
				fmt.Fprintf(os.Stderr, "Error: ROM file path is required\n")
				flags.Usage()
				os.Exit(1)
			}

			romPath := remaining[0]

			// Load and parse ROM
			rom, err := doom.LoadROM(romPath)
			if err != nil {
				return fmt.Errorf("load ROM: %w", err)
			}

			fmt.Printf("Loaded ROM: %s\n", romPath)
			fmt.Printf("  %s\n", rom.Summary())

			// Write to BPF map if available
			romMap, bpfErr := openBPFMap(mapPinPath + "/ROM_MAP")
			if bpfErr != nil {
				fmt.Printf("  [DRY-RUN] BPF maps not available: %v\n", bpfErr)
			} else {
				defer romMap.Close()
				// Wrap BPFMap as doom.MapAccessor
				adapter := &bpfMapAdapter{m: romMap}
				if err := rom.WriteToMap(adapter); err != nil {
					return fmt.Errorf("write to rom_map: %w", err)
				}
				fmt.Printf("  Written %d instructions to rom_map\n", rom.InstructionCount())
			}

			// Stats
			if showStats {
				fmt.Println()
				fmt.Printf("Statistics:\n")
				fmt.Printf("  Instructions: %d\n", rom.InstructionCount())
				sizeKB := float64(rom.SizeBytes()) / 1024.0
				fmt.Printf("  Size: %d bytes (%.1f KB)\n", rom.SizeBytes(), sizeKB)
				estSeconds := float64(rom.InstructionCount()) / 2_000_000.0
				fmt.Printf("  Est. runtime @ 2MHz: %.3f seconds\n", estSeconds)
				fmt.Printf("  Est. runtime @ 35fps: %.1f frames\n", estSeconds*35)
			}

			// HMAC
			if showHMAC {
				keyBytes := []byte(hmacKey)
				mac := rom.HMAC(keyBytes)
				fmt.Printf("\nHMAC-SHA256: %s\n", hex.EncodeToString(mac))
			}

			return nil
		},
	}
}

// newDoomStatusCmd creates the "doom status" subcommand.
func newDoomStatusCmd() *Command {
	var (
		mapPinPath string
		oneLiner   bool
	)

	flags := flag.NewFlagSet("doom status", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.StringVar(&mapPinPath, "map-pin", "/sys/fs/bpf/unheaded/doom-ring/maps", "BPF map pin directory")
	flags.BoolVar(&oneLiner, "short", false, "Print one-line summary instead of table")

	return &Command{
		Name:  "status",
		Short: "Show current CPU state",
		Long: `Display the current Doom CPU state from BPF cpu_map.

Shows all registers, flags, program counter, execution counters,
and cache statistics.

Examples:
  wotan-ctl doom status
  wotan-ctl doom status --short`,
		Usage:   "wotan-ctl doom status [flags]",
		Aliases: []string{"state", "info"},
		Flags:   flags,
		RunFunc: func(ctx *Context, args []string) error {
			if err := flags.Parse(args); err != nil {
				return err
			}

			cpuMap, err := openBPFMap(mapPinPath + "/CPU_MAP")
			if err != nil {
				// Dry-run: show default state
				fmt.Println("[DRY-RUN] BPF CPU_MAP not available, showing default state")
				cpu := doom.DefaultCpuState()
				if oneLiner {
					fmt.Println(cpu.FormatOneLiner())
				} else {
					fmt.Print(cpu.FormatTable())
				}
				return nil
			}
			defer cpuMap.Close()

			adapter := &bpfMapAdapter{m: cpuMap}
			cpu, err := doom.ReadCpuState(adapter)
			if err != nil {
				return fmt.Errorf("read CPU state: %w", err)
			}

			if oneLiner {
				fmt.Println(cpu.FormatOneLiner())
			} else {
				fmt.Print(cpu.FormatTable())
			}
			return nil
		},
	}
}

// newDoomInputCmd creates the "doom input <key_bitmap>" subcommand.
func newDoomInputCmd() *Command {
	var mapPinPath string

	flags := flag.NewFlagSet("doom input", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.StringVar(&mapPinPath, "map-pin", "/sys/fs/bpf/unheaded/doom-ring/maps", "BPF map pin directory")

	return &Command{
		Name:  "input",
		Short: "Inject keyboard input",
		Long: `Inject keyboard input into the Doom BPF kbd_map.

The key_bitmap is a hex value where each bit represents a scancode.
Bits that are set indicate pressed keys.

Examples:
  wotan-ctl doom input 0x01       # Press key 0
  wotan-ctl doom input 0x03       # Press keys 0 and 1
  wotan-ctl doom input 0xFF       # Press keys 0-7`,
		Usage:   "wotan-ctl doom input <key_bitmap> [flags]",
		Aliases: []string{"key", "kbd"},
		Flags:   flags,
		RunFunc: func(ctx *Context, args []string) error {
			if err := flags.Parse(args); err != nil {
				return err
			}

			remaining := flags.Args()
			if len(remaining) < 1 {
				fmt.Fprintf(os.Stderr, "Error: key_bitmap (hex) is required\n")
				flags.Usage()
				os.Exit(1)
			}

			bitmap := remaining[0]
			events, err := doom.ParseKeyBitmap(bitmap)
			if err != nil {
				return fmt.Errorf("parse key bitmap: %w", err)
			}

			if len(events) == 0 {
				fmt.Println("No keys to inject (bitmap is 0)")
				return nil
			}

			fmt.Printf("Injecting %d key events from bitmap %s:\n", len(events), bitmap)
			for _, ev := range events {
				fmt.Printf("  %s\n", ev.String())
			}

			kbdMap, err := openBPFMap(mapPinPath + "/KBD_MAP")
			if err != nil {
				fmt.Printf("[DRY-RUN] BPF KBD_MAP not available: %v\n", err)
				return nil
			}
			defer kbdMap.Close()

			adapter := &bpfMapAdapter{m: kbdMap}
			if err := doom.InjectKeyEvents(adapter, events); err != nil {
				return fmt.Errorf("inject keys: %w", err)
			}

			fmt.Printf("Injected %d key events\n", len(events))
			return nil
		},
	}
}

// newDoomResetCmd creates the "doom reset" subcommand.
func newDoomResetCmd() *Command {
	var mapPinPath string

	flags := flag.NewFlagSet("doom reset", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.StringVar(&mapPinPath, "map-pin", "/sys/fs/bpf/unheaded/doom-ring/maps", "BPF map pin directory")

	return &Command{
		Name:  "reset",
		Short: "Reset CPU to default state",
		Long: `Reset the Doom CPU to its default boot state.

This sets:
  - PC = 0 (start of ROM)
  - SP (r15) = 0xFFFF0000
  - All other registers = 0
  - All flags and counters = 0

Examples:
  wotan-ctl doom reset`,
		Usage: "wotan-ctl doom reset [flags]",
		Flags: flags,
		RunFunc: func(ctx *Context, args []string) error {
			if err := flags.Parse(args); err != nil {
				return err
			}

			cpuMap, err := openBPFMap(mapPinPath + "/CPU_MAP")
			if err != nil {
				fmt.Println("[DRY-RUN] BPF CPU_MAP not available")
				cpu := doom.DefaultCpuState()
				fmt.Printf("[DRY-RUN] Would write default state:\n")
				fmt.Printf("  PC=0x%08X SP=0x%08X\n", cpu.PC, cpu.Regs[doom.RegSP])
				return nil
			}
			defer cpuMap.Close()

			adapter := &bpfMapAdapter{m: cpuMap}
			if err := doom.ResetCpuState(adapter); err != nil {
				return fmt.Errorf("reset CPU state: %w", err)
			}

			fmt.Println("CPU state reset to defaults")
			fmt.Printf("  PC=0x%08X SP=0x%08X\n", doom.RegPCDefault, doom.RegSPDefault)
			return nil
		},
	}
}

// newDoomInjectTickCmd creates the "doom inject-tick" subcommand (stub).
func newDoomInjectTickCmd() *Command {
	var (
		mapPinPath string
		count      int
	)

	flags := flag.NewFlagSet("doom inject-tick", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.StringVar(&mapPinPath, "map-pin", "/sys/fs/bpf/unheaded/doom-ring/maps", "BPF map pin directory")
	flags.IntVar(&count, "count", 1, "Number of ticks to inject")

	return &Command{
		Name:  "inject-tick",
		Short: "Inject compute tick(s) into the ring",
		Long: `Inject one or more compute ticks into the Doom-over-IPv6 packet ring.

Each tick sends a packet through the 6-hop ring, executing up to
16 MBC instructions per hop (96 instructions total per tick).

This is a stub — full implementation requires the ring setup scripts.

Examples:
  wotan-ctl doom inject-tick
  wotan-ctl doom inject-tick --count 100`,
		Usage: "wotan-ctl doom inject-tick [flags]",
		Flags: flags,
		RunFunc: func(ctx *Context, args []string) error {
			if err := flags.Parse(args); err != nil {
				return err
			}

			fmt.Printf("[STUB] Would inject %d tick(s) into the packet ring\n", count)
			fmt.Println("  Each tick: 6 hops x 16 insns = 96 instructions")
			fmt.Printf("  Total: %d instructions\n", count*96)
			fmt.Println()
			fmt.Println("Full implementation requires:")
			fmt.Println("  - Ring namespaces (monad0-5) set up via doom-ring.sh")
			fmt.Println("  - XDP programs attached to each hop")
			fmt.Println("  - Packet injection via raw socket")
			return nil
		},
	}
}

// bpfMapAdapter wraps the existing wotan-ctl BPFMap to implement doom.MapAccessor.
type bpfMapAdapter struct {
	m *BPFMap
}

// LookupElem implements doom.MapAccessor.
func (a *bpfMapAdapter) LookupElem(key []byte, valueSize int) ([]byte, error) {
	if a.m == nil || a.m.fd < 0 {
		return nil, fmt.Errorf("BPF map not initialized")
	}

	value := make([]byte, valueSize)

	// Use the existing Lookup method's underlying syscall approach
	val, found, err := a.m.Lookup(key)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("key not found")
	}

	copy(value, val)
	return value, nil
}

// UpdateElem implements doom.MapAccessor.
func (a *bpfMapAdapter) UpdateElem(key, value []byte) error {
	if a.m == nil || a.m.fd < 0 {
		return fmt.Errorf("BPF map not initialized")
	}
	return a.m.updateElem(key, value)
}
