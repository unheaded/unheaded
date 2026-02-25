// cmd/doom-cpu-dump reads and displays CPU state from BPF maps.
// Replaces Python scripts: doom-cpu-dump.py, read_cpu.py, reset_cpu.py
//
// Usage:
//   doom-cpu-dump dump [--instance 0xDE] [--map /sys/fs/bpf/.../CPU_MAP]
//   doom-cpu-dump watch [--instance 0xDE] [--interval 200] [--map /sys/fs/bpf/.../CPU_MAP]
//   doom-cpu-dump reset [--instance 0xDE] [--map /sys/fs/bpf/.../CPU_MAP]
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"
)

const (
	defaultCPUMapPath = "/sys/fs/bpf/unheaded/doom-ring/maps/CPU_MAP"
	defaultInstance   = 0xDE
	cpuStateSize      = 136

	// CpuState layout (136 bytes):
	// regs[16] uint64 = 128 bytes (offset 0-127)
	// pc uint64 = 8 bytes (offset 128-135)
	// sp uint64 = 8 bytes (offset 136-143)
	// flags uint64 = 8 bytes (offset 144-151)
	// halted bool = 1 byte (offset 152)
	// _pad [7]byte = 7 bytes (offset 153-159)
	// insn_count uint64 = 8 bytes (offset 160-167)
	// last_kbd_state [32]byte = 32 bytes (offset 168-199)
)

type CpuState struct {
	Regs        [16]uint64
	PC          uint64
	SP          uint64
	Flags       uint64
	Halted      bool
	InsnCount   uint64
	LastKBDState [32]byte
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "dump":
		err = dumpCPU(args)
	case "watch":
		err = watchCPU(args)
	case "reset":
		err = resetCPU(args)
	case "help", "-h", "--help":
		usage()
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		usage()
		os.Exit(1)
	}

	if err != nil {
		log.Fatalf("%s: %v", cmd, err)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: doom-cpu-dump <subcommand> [options]

Subcommands:
  dump      Print CPU state once
  watch     Print CPU state continuously
  reset     Reset CPU state to all zeros

Options:
  --instance <hex>   CPU instance ID (default: DE, hex without 0x prefix)
  --map <path>       BPF CPU_MAP path (default: /sys/fs/bpf/unheaded/doom-ring/maps/CPU_MAP)
  --interval <ms>    Watch interval in milliseconds (default: 200)

Examples:
  doom-cpu-dump dump --instance DE
  doom-cpu-dump watch --instance DE --interval 100
  doom-cpu-dump reset --instance DE

`)
}

func dumpCPU(args []string) error {
	fs := flag.NewFlagSet("dump", flag.ExitOnError)
	instance := fs.String("instance", "DE", "CPU instance ID (hex)")
	_ = fs.String("map", defaultCPUMapPath, "BPF CPU_MAP path")

	if err := fs.Parse(args); err != nil {
		return err
	}

	instanceByte, err := strconv.ParseUint(*instance, 16, 8)
	if err != nil {
		return fmt.Errorf("invalid instance ID: %v", err)
	}

	state := make([]byte, cpuStateSize)

	// TODO: Read from BPF map
	// For now, just pretty-print the structure
	cpu := decodeCpuState(state)
	printCPUState(cpu, byte(instanceByte))

	return nil
}

func watchCPU(args []string) error {
	fs := flag.NewFlagSet("watch", flag.ExitOnError)
	instance := fs.String("instance", "DE", "CPU instance ID (hex)")
	interval := fs.Int("interval", 200, "Update interval in milliseconds")
	_ = fs.String("map", defaultCPUMapPath, "BPF CPU_MAP path")

	if err := fs.Parse(args); err != nil {
		return err
	}

	instanceByte, err := strconv.ParseUint(*instance, 16, 8)
	if err != nil {
		return fmt.Errorf("invalid instance ID: %v", err)
	}

	fmt.Printf("Watching CPU 0x%02X every %dms (Ctrl+C to stop)...\n", instanceByte, *interval)
	fmt.Println()

	ticker := time.NewTicker(time.Duration(*interval) * time.Millisecond)
	defer ticker.Stop()

	state := make([]byte, cpuStateSize)
	for range ticker.C {
		// TODO: Read from BPF map
		cpu := decodeCpuState(state)

		// Clear screen (ANSI escape)
		fmt.Print("\033[H\033[2J")

		fmt.Printf("=== CPU State (instance 0x%02X) ===\n", instanceByte)
		fmt.Printf("Last updated: %s\n\n", time.Now().Format("15:04:05.000"))

		printCPUState(cpu, byte(instanceByte))
	}

	return nil
}

func resetCPU(args []string) error {
	fs := flag.NewFlagSet("reset", flag.ExitOnError)
	instance := fs.String("instance", "DE", "CPU instance ID (hex)")
	mapPath := fs.String("map", defaultCPUMapPath, "BPF CPU_MAP path")

	if err := fs.Parse(args); err != nil {
		return err
	}

	instanceByte, err := strconv.ParseUint(*instance, 16, 8)
	if err != nil {
		return fmt.Errorf("invalid instance ID: %v", err)
	}

	// Create zeroed CPU state
	state := make([]byte, cpuStateSize)

	fmt.Printf("Resetting CPU 0x%02X to all zeros...\n", instanceByte)
	fmt.Printf("  Would write to: %s[0x%02X]\n", *mapPath, instanceByte)

	// TODO: Write to BPF map
	_ = state

	return nil
}

func decodeCpuState(state []byte) *CpuState {
	if len(state) < cpuStateSize {
		// Return zero state if not enough data
		return &CpuState{}
	}

	cpu := &CpuState{}

	// Decode registers (16 x uint64)
	for i := 0; i < 16; i++ {
		cpu.Regs[i] = binary.LittleEndian.Uint64(state[i*8 : (i+1)*8])
	}

	// Decode PC, SP, flags
	cpu.PC = binary.LittleEndian.Uint64(state[128:136])
	cpu.SP = binary.LittleEndian.Uint64(state[136:144])
	cpu.Flags = binary.LittleEndian.Uint64(state[144:152])

	// Decode halted flag
	cpu.Halted = state[152] != 0

	// Decode instruction count
	cpu.InsnCount = binary.LittleEndian.Uint64(state[160:168])

	// Decode last KBD state
	copy(cpu.LastKBDState[:], state[168:200])

	return cpu
}

func printCPUState(cpu *CpuState, instance byte) {
	fmt.Println("CPU Registers:")
	fmt.Println("  +-------+----------+  +-------+----------+")

	for i := 0; i < 8; i++ {
		regLeft := i
		regRight := i + 8

		nameLeft := fmt.Sprintf("r%d", regLeft)
		nameRight := fmt.Sprintf("r%d", regRight)

		if regLeft == 14 {
			nameLeft = "LR"
		} else if regLeft == 15 {
			nameLeft = "SP"
		}

		if regRight == 14 {
			nameRight = "LR"
		} else if regRight == 15 {
			nameRight = "SP"
		}

		valLeft := cpu.Regs[regLeft]
		valRight := cpu.Regs[regRight]

		// Highlight non-zero values
		colorLeft := ""
		colorRight := ""
		resetColor := ""

		if valLeft != 0 {
			colorLeft = "\033[92m" // Green
			resetColor = "\033[0m"
		}
		if valRight != 0 {
			colorRight = "\033[92m"
			resetColor = "\033[0m"
		}

		fmt.Printf("  | %-5s | %s0x%08X%s | | %-5s | %s0x%08X%s |\n",
			nameLeft, colorLeft, valLeft, resetColor,
			nameRight, colorRight, valRight, resetColor)
	}

	fmt.Println("  +-------+----------+  +-------+----------+")
	fmt.Println()

	// CPU state summary
	fmt.Printf("CPU State:\n")
	fmt.Printf("  PC (Program Counter): 0x%08X (%d)\n", cpu.PC, cpu.PC)
	fmt.Printf("  SP (Stack Pointer):   0x%08X (%d)\n", cpu.SP, cpu.SP)
	fmt.Printf("  Flags:                0x%016X\n", cpu.Flags)
	fmt.Printf("  Halted:               %v\n", cpu.Halted)
	fmt.Printf("  Instruction Count:    %d (0x%X)\n", cpu.InsnCount, cpu.InsnCount)

	fmt.Println()
	fmt.Printf("Last Keyboard State:  %02X %02X %02X %02X %02X %02X %02X %02X\n",
		cpu.LastKBDState[0], cpu.LastKBDState[1], cpu.LastKBDState[2], cpu.LastKBDState[3],
		cpu.LastKBDState[4], cpu.LastKBDState[5], cpu.LastKBDState[6], cpu.LastKBDState[7])
}
