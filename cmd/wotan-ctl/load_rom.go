// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"os"

	"unheaded/pkg/logger"
)

// newLoadRomCmd creates the load-rom subcommand.
func newLoadRomCmd() *Command {
	var (
		flowLabel  uint64
		file       string
		showStats  bool
		showDisasm bool
		doReset    bool
		mapPinPath string
		verbose    bool
	)

	flags := flag.NewFlagSet("load-rom", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.Uint64Var(&flowLabel, "flow-label", 0, "IPv6 flow label (can use hex: 0x0A3F7E)")
	flags.StringVar(&file, "file", "", "Path to .mbc binary file (required)")
	flags.BoolVar(&showStats, "stats", false, "Print instruction count + estimated runtime")
	flags.BoolVar(&showDisasm, "disasm", false, "Disassemble and print each instruction")
	flags.BoolVar(&doReset, "reset", false, "Reset cpu_map entry (restart from PC=0)")
	flags.StringVar(&mapPinPath, "map-pin", "/sys/fs/bpf", "BPF map pin directory")
	flags.BoolVar(&verbose, "verbose", false, "Enable verbose logging")

	cmd := &Command{
		Name:  "load-rom",
		Short: "Load MBC bytecode into BPF rom_map",
		Long: `Load MBC bytecode into the BPF rom_map array (CPU instruction memory).

The rom_map is a BPF array map that stores the MBC bytecode instructions.
Each instruction is a 32-bit little-endian word.

Examples:
  # Load program with flow label
  wotan-ctl load-rom --file program.mbc --flow-label 0x0A3F7E

  # Load and show statistics
  wotan-ctl load-rom --file doom1.mbc --flow-label 0x123456 --stats

  # Load and disassemble
  wotan-ctl load-rom --file test.mbc --flow-label 0x100 --disasm --stats`,
		Usage:   "wotan-ctl load-rom --file <path> --flow-label <label> [options]",
		Aliases: []string{"load-program", "load-code"},
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
				ctx.Logger.SetLevel(logger.DebugLevel)
			}

			return loadRom(ctx, flowLabel, file, mapPinPath, showStats, showDisasm, doReset)
		},
	}

	return cmd
}

// loadRom loads MBC bytecode into the rom_map.
func loadRom(ctx *Context, flowLabel uint64, filePath string, mapPinPath string, stats bool, disasm bool, reset bool) error {
	// 1. Read .mbc file (sequence of little-endian u32 instruction words)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	if len(data)%4 != 0 {
		return fmt.Errorf("MBC file size %d is not a multiple of 4", len(data))
	}

	instructions := make([]uint32, len(data)/4)
	for i := range instructions {
		instructions[i] = binary.LittleEndian.Uint32(data[i*4 : i*4+4])
	}

	ctx.Logger.Debug().Int("count", len(instructions)).Str("file", filePath).Msg("Loaded MBC instructions")

	// 2. Open/pin rom_map via BPF syscall
	romMap, err := openBPFMap(mapPinPath + "/ROM_MAP")
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: Could not open BPF map at %s/rom_map: %v\n", mapPinPath, err)
		fmt.Fprintf(os.Stderr, "Running in dry-run mode (no BPF available)\n")
		romMap = nil
	}

	// 3. Write instructions to rom_map[i]
	if romMap != nil {
		for i, insn := range instructions {
			key := uint32(i)
			if err := romMap.Update(key, insn); err != nil {
				return fmt.Errorf("rom_map update at index %d: %w", i, err)
			}
		}
		fmt.Printf("Loaded %d instructions into rom_map\n", len(instructions))
	} else {
		fmt.Printf("[DRY-RUN] Would load %d instructions into rom_map\n", len(instructions))
	}

	// 4. Initialize or reset cpu_map entry
	if romMap != nil || true {
		// Even in dry-run, show what would happen
		if romMap != nil {
			cpuMap, err := openBPFMap(mapPinPath + "/CPU_MAP")
			if err == nil && cpuMap != nil {
				cpu := defaultCpuState(flowLabel)
				if reset || true {
					// Always initialize/reset on load
					if err := cpuMap.Update(uint32(flowLabel&0xFFFFFF), cpu); err != nil {
						ctx.Logger.Warn().Err(err).Msg("Failed to update cpu_map")
					} else {
						fmt.Printf("Initialized CPU state for flow label 0x%06X\n", uint32(flowLabel&0xFFFFFF))
					}
				}
			}
		} else {
			fmt.Printf("[DRY-RUN] Would initialize CPU state for flow label 0x%06X\n", uint32(flowLabel&0xFFFFFF))
		}
	}

	// 5. --stats
	if stats {
		instrCount := len(instructions)
		sizeBytes := len(data)
		estSeconds := float64(instrCount) / 2_000_000.0 // 2MHz effective clock
		fmt.Println()
		fmt.Printf("Statistics:\n")
		fmt.Printf("  Instructions: %d\n", instrCount)
		fmt.Printf("  Size: %d bytes (%.1f KB)\n", sizeBytes, float64(sizeBytes)/1024)
		fmt.Printf("  Est. runtime @ 2MHz: %.3f seconds\n", estSeconds)
		fmt.Printf("  Est. runtime @ 35fps: %.1f frames\n", estSeconds*35)
	}

	// 6. --disasm
	if disasm {
		fmt.Println()
		fmt.Println("Disassembly:")
		for i, insn := range instructions {
			fmt.Printf("  %06d: %s\n", i, disassembleMBC(insn))
		}
	}

	return nil
}

// disassembleMBC decodes an MBC instruction word.
func disassembleMBC(insn uint32) string {
	opcode := uint8((insn >> 24) & 0xFF)
	dst := uint8((insn >> 20) & 0xF)
	src := uint8((insn >> 16) & 0xF)
	imm := uint16(insn & 0xFFFF)

	// Opcodes must match ebpf/monad-common/src/lib.rs op:: constants.
	opNames := map[uint8]string{
		0x01: "ADD", 0x02: "SUB", 0x03: "MUL", 0x04: "DIV", 0x05: "MOD", 0x06: "NEG",
		0x07: "AND", 0x08: "OR", 0x09: "XOR", 0x0A: "NOT", 0x0B: "SHL", 0x0C: "SHR",
		0x0D: "SAR", 0x0E: "MOV", 0x0F: "MOVI", 0x10: "CMP",
		0x20: "JMP", 0x21: "JZ", 0x22: "JNZ", 0x23: "JN", 0x24: "JP", 0x25: "JC",
		0x26: "JNC", 0x27: "CALL", 0x28: "RET", 0x29: "JMPR", 0x2A: "CALLR",
		0x30: "LD", 0x31: "ST", 0x32: "LDB", 0x33: "STB", 0x34: "LDH", 0x35: "STH",
		0x40: "SYSCALL", 0xFF: "HALT",
	}

	name, ok := opNames[opcode]
	if !ok {
		name = fmt.Sprintf("UNKNOWN(0x%02X)", opcode)
	}

	// Format based on instruction type
	switch opcode {
	case 0x20, 0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27: // branches/call
		return fmt.Sprintf("%-8s 0x%04X", name, imm)
	case 0x28: // RET
		return "RET"
	case 0x29: // JMPR (indirect jump via register)
		return fmt.Sprintf("%-8s r%d", name, dst)
	case 0x2A: // CALLR (indirect call via register)
		return fmt.Sprintf("%-8s r%d", name, dst)
	case 0x0E: // MOV
		return fmt.Sprintf("%-8s r%d, r%d", name, dst, src)
	case 0x0F: // MOVI
		return fmt.Sprintf("%-8s r%d, %d", name, dst, int16(imm))
	case 0x40: // SYSCALL
		syscallNames := map[uint16]string{
			1: "DRAW_FRAME", 2: "GET_KEY", 3: "GET_TICKS", 4: "SLEEP", 0xFF: "HALT",
		}
		if sname, ok := syscallNames[imm]; ok {
			return fmt.Sprintf("%-8s %s", name, sname)
		}
		return fmt.Sprintf("%-8s 0x%04X", name, imm)
	default:
		return fmt.Sprintf("%-8s r%d, r%d, %d", name, dst, src, int16(imm))
	}
}

// CpuState represents the CPU state stored in BPF cpu_map.
// Field order must match MbcCpuState in ebpf/monad-common/src/lib.rs.
// Total size: 120 bytes.
type CpuState struct {
	Regs              [16]uint32
	PC                uint32
	Flags             uint8
	Halted            uint8
	Stalled           uint8
	Pad               uint8
	SleepUntil        uint64
	InsnCount         uint64
	CacheHits         uint64
	CacheMisses       uint64
	InterruptPending  uint8
	InterruptVector   uint8
	InterruptsEnabled uint8
	Pad2              uint8
	TickCounter       uint32
	ProgramBreak      uint32
	ExitCode          uint32
}

// defaultCpuState returns a default CPU state for a given flow label.
func defaultCpuState(flowLabel uint64) CpuState {
	cpu := CpuState{}
	cpu.Regs[15] = 0xFFFF_0000   // SP default (register 15 is stack pointer)
	cpu.PC = 0                   // Start at instruction 0
	cpu.Flags = 0
	cpu.ProgramBreak = 0x0040_0000 // Default heap start (4 MiB)
	return cpu
}
