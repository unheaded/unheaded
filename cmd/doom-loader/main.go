// Package main implements doom-loader: bulk-load data into BPF maps for the
// Doom-over-IPv6 compute ring.
//
// Replaces the Python doom-loader-core.py with native Go using direct BPF
// syscalls for maximum throughput. Supports the same subcommand interface:
//
//	doom-loader rom   <rom_pin> <mbc_file>
//	doom-loader ram   <ram_pin> <data_file> <start_byte_addr>
//	doom-loader rv2mbc <rv2mbc_pin> <rv2mbc_file>
//	doom-loader cpu   <cpu_pin> [--instance 0xDE]
//	doom-loader dump  <cpu_pin> [--instance 0xDE]
package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"strconv"
	"time"

	bpfpkg "unheaded/internal/bpf"
)

const defaultInstance = 0xDE

func usage() {
	fmt.Fprintf(os.Stderr, `Usage:
  doom-loader rom    <rom_pin> <mbc_file>
  doom-loader ram    <ram_pin> <data_file> <start_byte_addr>
  doom-loader rv2mbc <rv2mbc_pin> <rv2mbc_file>
  doom-loader cpu    <cpu_pin> [--instance <hex>]
  doom-loader dump   <cpu_pin> [--instance <hex>]
`)
	os.Exit(1)
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}

	switch os.Args[1] {
	case "rom":
		cmdRom()
	case "ram":
		cmdRam()
	case "rv2mbc":
		cmdRv2mbc()
	case "cpu":
		cmdCpu()
	case "dump":
		cmdDump()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		usage()
	}
}

// parseInstance parses --instance flag from args[startIdx:]. Returns instance ID.
func parseInstance(args []string) uint32 {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--instance" {
			v, err := strconv.ParseUint(args[i+1], 0, 32)
			if err != nil {
				fatal("invalid instance ID %q: %v", args[i+1], err)
			}
			return uint32(v)
		}
	}
	return defaultInstance
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "ERROR: "+format+"\n", args...)
	os.Exit(1)
}

// bulkLoad loads a binary file word-by-word into a BPF Array map.
// Each u32 in the file maps to key=startIndex+i, value=word[i].
func bulkLoad(tag string, m *bpfpkg.Map, data []byte, startIndex uint32) {
	numWords := uint32(len(data) / 4)
	if numWords == 0 {
		fmt.Printf("[%s] No data to load\n", tag)
		return
	}

	keyBuf := make([]byte, 4)
	start := time.Now()
	const batchSize = 10000

	for i := uint32(0); i < numWords; i++ {
		binary.LittleEndian.PutUint32(keyBuf, startIndex+i)
		valSlice := data[i*4 : (i+1)*4]
		if err := m.UpdateElem(keyBuf, valSlice); err != nil {
			fatal("[%s] write error at index %d: %v", tag, startIndex+i, err)
		}

		if (i+1)%batchSize == 0 || i+1 == numWords {
			elapsed := time.Since(start).Seconds()
			pct := float64(i+1) / float64(numWords) * 100
			rate := float64(0)
			if elapsed > 0 {
				rate = float64(i+1) / elapsed
			}
			fmt.Printf("\r[%s] %d/%d (%.1f%%) — %.0f entries/sec", tag, i+1, numWords, pct, rate)
		}
	}

	elapsed := time.Since(start).Seconds()
	rate := float64(numWords) / elapsed
	fmt.Printf("\n[%s] Done: %d words in %.1fs (%.0f entries/sec)\n", tag, numWords, elapsed, rate)
}

func cmdRom() {
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "Usage: doom-loader rom <rom_pin> <mbc_file>")
		os.Exit(1)
	}
	romPin := os.Args[2]
	mbcFile := os.Args[3]

	fmt.Printf("[ROM] Opening %s...\n", mbcFile)
	data, err := os.ReadFile(mbcFile)
	if err != nil {
		fatal("read %s: %v", mbcFile, err)
	}

	numWords := len(data) / 4
	fmt.Printf("[ROM] %d words to load\n", numWords)

	m, err := bpfpkg.OpenMap(romPin)
	if err != nil {
		fatal("open ROM_MAP: %v", err)
	}
	defer m.Close()
	fmt.Printf("[ROM] Map FD: %d\n", m.Fd())

	bulkLoad("ROM", m, data[:numWords*4], 0)
}

func cmdRam() {
	if len(os.Args) < 5 {
		fmt.Fprintln(os.Stderr, "Usage: doom-loader ram <ram_pin> <data_file> <start_byte_addr>")
		os.Exit(1)
	}
	ramPin := os.Args[2]
	dataFile := os.Args[3]
	startByteAddr, err := strconv.ParseUint(os.Args[4], 0, 64)
	if err != nil {
		fatal("invalid start address %q: %v", os.Args[4], err)
	}

	fmt.Printf("[RAM] Opening %s...\n", dataFile)
	data, err := os.ReadFile(dataFile)
	if err != nil {
		fatal("read %s: %v", dataFile, err)
	}

	// Pad to 4-byte alignment
	if len(data)%4 != 0 {
		pad := make([]byte, 4-len(data)%4)
		data = append(data, pad...)
	}

	numWords := len(data) / 4
	startWordAddr := uint32(startByteAddr >> 2)
	fmt.Printf("[RAM] %d words at word_addr 0x%x (byte_addr 0x%x)\n", numWords, startWordAddr, startByteAddr)

	m, err := bpfpkg.OpenMap(ramPin)
	if err != nil {
		fatal("open RAM_MAP: %v", err)
	}
	defer m.Close()
	fmt.Printf("[RAM] Map FD: %d\n", m.Fd())

	bulkLoad("RAM", m, data, startWordAddr)
}

func cmdRv2mbc() {
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "Usage: doom-loader rv2mbc <rv2mbc_pin> <rv2mbc_file>")
		os.Exit(1)
	}
	rv2mbcPin := os.Args[2]
	rv2mbcFile := os.Args[3]

	fmt.Printf("[RV2MBC] Opening %s...\n", rv2mbcFile)
	data, err := os.ReadFile(rv2mbcFile)
	if err != nil {
		fatal("read %s: %v", rv2mbcFile, err)
	}

	numWords := len(data) / 4
	fmt.Printf("[RV2MBC] %d entries to load\n", numWords)

	m, err := bpfpkg.OpenMap(rv2mbcPin)
	if err != nil {
		fatal("open RV2MBC_MAP: %v", err)
	}
	defer m.Close()
	fmt.Printf("[RV2MBC] Map FD: %d\n", m.Fd())

	bulkLoad("RV2MBC", m, data[:numWords*4], 0)
}

func cmdCpu() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: doom-loader cpu <cpu_pin> [--instance <hex>]")
		os.Exit(1)
	}
	cpuPin := os.Args[2]
	instance := parseInstance(os.Args[3:])

	fmt.Printf("[CPU] Initializing CPU state (instance %d / 0x%x)...\n", instance, instance)

	m, err := bpfpkg.OpenMap(cpuPin)
	if err != nil {
		fatal("open CPU_MAP: %v", err)
	}
	defer m.Close()
	fmt.Printf("[CPU] Map FD: %d\n", m.Fd())

	cpu := &bpfpkg.CpuState{}
	cpu.Regs[15] = 0x3F00000 // SP = stack top (63MB, from crt0_monad.S)
	// Everything else zero: PC=0, flags=0, halted=0, etc.

	keyBuf := make([]byte, 4)
	binary.LittleEndian.PutUint32(keyBuf, instance)

	valBuf := bpfpkg.EncodeCpuState(cpu)

	fmt.Printf("[CPU] State size: %d bytes\n", len(valBuf))

	if err := m.UpdateElem(keyBuf, valBuf); err != nil {
		fatal("write CPU state: %v", err)
	}

	fmt.Printf("[CPU] Instance %d: PC=0x%x, SP=0x%x, halted=%d\n",
		instance, cpu.PC, cpu.Regs[15], cpu.Halted)
}

func cmdDump() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: doom-loader dump <cpu_pin> [--instance <hex>]")
		os.Exit(1)
	}
	cpuPin := os.Args[2]
	instance := parseInstance(os.Args[3:])

	m, err := bpfpkg.OpenMap(cpuPin)
	if err != nil {
		fatal("open CPU_MAP: %v", err)
	}
	defer m.Close()

	keyBuf := make([]byte, 4)
	binary.LittleEndian.PutUint32(keyBuf, instance)

	val, err := m.LookupElem(keyBuf, bpfpkg.CpuStateSize)
	if err != nil {
		fatal("read CPU state: %v", err)
	}

	cpu := bpfpkg.DecodeCpuState(val)

	fmt.Printf("=== CPU State (instance 0x%X) ===\n", instance)
	fmt.Printf("  PC:          0x%08X (%d)\n", cpu.PC, cpu.PC)
	fmt.Printf("  Flags:       0x%02X\n", cpu.Flags)
	fmt.Printf("  Halted:      %d\n", cpu.Halted)
	fmt.Printf("  Stalled:     %d\n", cpu.Stalled)
	fmt.Printf("  SleepUntil:  %d ns\n", cpu.SleepUntil)
	fmt.Printf("  InsnCount:   %d\n", cpu.InsnCount)
	fmt.Printf("  CacheHits:   %d\n", cpu.CacheHits)
	fmt.Printf("  CacheMisses: %d\n", cpu.CacheMisses)
	fmt.Println()
	fmt.Println("  Registers:")
	for i := 0; i < 16; i++ {
		name := fmt.Sprintf("r%d", i)
		if i == 14 {
			name = "LR"
		} else if i == 15 {
			name = "SP"
		}
		fmt.Printf("    %-4s = 0x%08X (%d)\n", name, cpu.Regs[i], cpu.Regs[i])
	}
}
