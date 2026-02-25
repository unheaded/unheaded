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

	"unheaded/internal/bpf"
)

const (
	defaultCPUMapPath = "/sys/fs/bpf/unheaded/doom-ring/maps/CPU_MAP"
	defaultInstance   = 0xDE
)

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

// instanceKey encodes the instance ID as a 4-byte little-endian uint32 key
// suitable for BPF map lookups.
func instanceKey(id uint32) []byte {
	key := make([]byte, 4)
	binary.LittleEndian.PutUint32(key, id)
	return key
}

func dumpCPU(args []string) error {
	fs := flag.NewFlagSet("dump", flag.ExitOnError)
	instance := fs.String("instance", "DE", "CPU instance ID (hex)")
	mapPath := fs.String("map", defaultCPUMapPath, "BPF CPU_MAP path")

	if err := fs.Parse(args); err != nil {
		return err
	}

	instanceID, err := strconv.ParseUint(*instance, 16, 32)
	if err != nil {
		return fmt.Errorf("invalid instance ID: %v", err)
	}

	m, err := bpf.OpenMap(*mapPath)
	if err != nil {
		return fmt.Errorf("open CPU_MAP: %v", err)
	}
	defer m.Close()

	key := instanceKey(uint32(instanceID))
	data, err := m.LookupElem(key, bpf.CpuStateSize)
	if err != nil {
		return fmt.Errorf("read CPU state: %v", err)
	}

	cpu := bpf.DecodeCpuState(data)
	printCPUState(cpu, uint32(instanceID))

	return nil
}

func watchCPU(args []string) error {
	fs := flag.NewFlagSet("watch", flag.ExitOnError)
	instance := fs.String("instance", "DE", "CPU instance ID (hex)")
	interval := fs.Int("interval", 200, "Update interval in milliseconds")
	mapPath := fs.String("map", defaultCPUMapPath, "BPF CPU_MAP path")

	if err := fs.Parse(args); err != nil {
		return err
	}

	instanceID, err := strconv.ParseUint(*instance, 16, 32)
	if err != nil {
		return fmt.Errorf("invalid instance ID: %v", err)
	}

	m, err := bpf.OpenMap(*mapPath)
	if err != nil {
		return fmt.Errorf("open CPU_MAP: %v", err)
	}
	defer m.Close()

	fmt.Printf("Watching CPU 0x%02X every %dms (Ctrl+C to stop)...\n", instanceID, *interval)
	fmt.Println()

	ticker := time.NewTicker(time.Duration(*interval) * time.Millisecond)
	defer ticker.Stop()

	key := instanceKey(uint32(instanceID))
	for range ticker.C {
		data, err := m.LookupElem(key, bpf.CpuStateSize)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read CPU state: %v\n", err)
			continue
		}

		cpu := bpf.DecodeCpuState(data)

		// Clear screen (ANSI escape)
		fmt.Print("\033[H\033[2J")

		fmt.Printf("=== CPU State (instance 0x%02X) ===\n", instanceID)
		fmt.Printf("Last updated: %s\n\n", time.Now().Format("15:04:05.000"))

		printCPUState(cpu, uint32(instanceID))
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

	instanceID, err := strconv.ParseUint(*instance, 16, 32)
	if err != nil {
		return fmt.Errorf("invalid instance ID: %v", err)
	}

	m, err := bpf.OpenMap(*mapPath)
	if err != nil {
		return fmt.Errorf("open CPU_MAP: %v", err)
	}
	defer m.Close()

	// Create zeroed CPU state
	zeroCPU := &bpf.CpuState{}
	value := bpf.EncodeCpuState(zeroCPU)
	key := instanceKey(uint32(instanceID))

	fmt.Printf("Resetting CPU 0x%02X to all zeros...\n", instanceID)
	fmt.Printf("  Map:      %s\n", *mapPath)
	fmt.Printf("  Key:      0x%02X\n", instanceID)

	if err := m.UpdateElem(key, value); err != nil {
		return fmt.Errorf("write CPU state: %v", err)
	}

	fmt.Println("  Status:   OK")
	return nil
}

func printCPUState(cpu *bpf.CpuState, instance uint32) {
	fmt.Println("CPU Registers:")
	fmt.Println("  +-------+------------+  +-------+------------+")

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

		fmt.Printf("  | %-5s | %s0x%08X%s |  | %-5s | %s0x%08X%s |\n",
			nameLeft, colorLeft, valLeft, resetColor,
			nameRight, colorRight, valRight, resetColor)
	}

	fmt.Println("  +-------+------------+  +-------+------------+")
	fmt.Println()

	// CPU state summary
	haltedStr := "no"
	if cpu.Halted != 0 {
		haltedStr = fmt.Sprintf("\033[91mYES\033[0m (0x%02X)", cpu.Halted)
	}

	stalledStr := "no"
	if cpu.Stalled != 0 {
		stalledStr = fmt.Sprintf("\033[93mYES\033[0m (0x%02X)", cpu.Stalled)
	}

	fmt.Printf("CPU State:\n")
	fmt.Printf("  PC (Program Counter): 0x%08X (%d)\n", cpu.PC, cpu.PC)
	fmt.Printf("  Flags:                0x%02X\n", cpu.Flags)
	fmt.Printf("  Halted:               %s\n", haltedStr)
	fmt.Printf("  Stalled:              %s\n", stalledStr)
	fmt.Printf("  Instruction Count:    %d (0x%X)\n", cpu.InsnCount, cpu.InsnCount)
	fmt.Println()

	// Performance counters
	fmt.Printf("Performance:\n")
	fmt.Printf("  Cache Hits:           %d\n", cpu.CacheHits)
	fmt.Printf("  Cache Misses:         %d\n", cpu.CacheMisses)
	totalCache := cpu.CacheHits + cpu.CacheMisses
	if totalCache > 0 {
		hitRate := float64(cpu.CacheHits) / float64(totalCache) * 100.0
		fmt.Printf("  Cache Hit Rate:       %.1f%%\n", hitRate)
	}
	fmt.Printf("  Sleep Until:          %d", cpu.SleepUntil)
	if cpu.SleepUntil > 0 {
		fmt.Printf(" (active)")
	}
	fmt.Println()
}
