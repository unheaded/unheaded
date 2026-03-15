// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package main

import (
	"flag"
	"fmt"
	"os"
	"sort"

	"unheaded/pkg/upc"
)

// newBootCmd creates the boot subcommand for loading and booting UPC kernels.
func newBootCmd() *Command {
	var (
		kernel     string
		ramdisk    string
		bootArgs   string
		mapPinPath string
		dryRun     bool
	)

	flags := flag.NewFlagSet("boot", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.StringVar(&kernel, "kernel", "", "Path to kernel binary (.mbc or raw binary)")
	flags.StringVar(&ramdisk, "ramdisk", "", "Path to ramdisk/initrd image (optional)")
	flags.StringVar(&bootArgs, "args", "", "Kernel command line arguments")
	flags.StringVar(&mapPinPath, "map-pin", "/sys/fs/bpf/unheaded/doom-ring/maps", "BPF map pin directory")
	flags.BoolVar(&dryRun, "dry-run", false, "Show boot plan without writing to BPF maps")

	return &Command{
		Name:  "boot",
		Short: "Load and boot a UPC kernel image",
		Long: `Load a kernel image into UPC RAM and initialize the boot environment.

The boot command performs the UPC boot protocol:
  1. Initializes the Interrupt Vector Table (IVT) at 0x0000
  2. Writes a default HLT handler at 0x0400
  3. Writes boot parameters (BootParams) at 0x0100
  4. Loads kernel image at 0x10000
  5. Optionally loads ramdisk at the ramdisk region
  6. Sets CPU state: PC=0x10000, SP=0x03F00000

Boot parameters include a magic value (0x554E4844 = "UNHD"), memory size,
kernel/ramdisk addresses and sizes, and an optional command line.

Examples:
  # Boot a kernel
  wotan-ctl boot --kernel path/to/kernel.mbc

  # Boot with ramdisk and command line
  wotan-ctl boot --kernel kernel.mbc --ramdisk initrd.img --args "console=tty0"

  # Dry-run: show what would be written
  wotan-ctl boot --kernel kernel.mbc --dry-run`,
		Usage:   "wotan-ctl boot --kernel <path> [--ramdisk <path>] [--args <cmdline>] [flags]",
		Aliases: []string{"boot-kernel"},
		Flags:   flags,
		RunFunc: func(ctx *Context, args []string) error {
			if err := flags.Parse(args); err != nil {
				return err
			}

			if kernel == "" {
				fmt.Fprintf(os.Stderr, "Error: --kernel is required\n")
				flags.Usage()
				os.Exit(1)
			}

			return runBoot(ctx, kernel, ramdisk, bootArgs, mapPinPath, dryRun)
		},
	}
}

// runBoot executes the UPC boot protocol.
func runBoot(ctx *Context, kernelPath, ramdiskPath, bootArgs, mapPinPath string, dryRun bool) error {
	fmt.Println("=== UPC Boot Protocol v1 ===")
	fmt.Println()

	// 1. Build boot config
	cfg := upc.DefaultBootConfig()
	cfg.KernelPath = kernelPath
	cfg.RamdiskPath = ramdiskPath
	cfg.BootArgs = bootArgs

	fmt.Printf("Kernel:       %s\n", kernelPath)
	if ramdiskPath != "" {
		fmt.Printf("Ramdisk:      %s\n", ramdiskPath)
	}
	if bootArgs != "" {
		fmt.Printf("Boot args:    %s\n", bootArgs)
	}
	fmt.Printf("Kernel load:  0x%08X\n", cfg.KernelLoadAddr)
	fmt.Printf("Stack top:    0x%08X\n", cfg.StackTop)
	if ramdiskPath != "" {
		fmt.Printf("Ramdisk addr: 0x%08X\n", cfg.RamdiskAddr)
	}
	fmt.Println()

	// 2. Prepare boot memory
	fmt.Println("[1/5] Preparing boot memory layout...")
	mem, err := upc.PrepareBootMemory(cfg)
	if err != nil {
		return fmt.Errorf("prepare boot memory: %w", err)
	}
	fmt.Printf("       %d words to write\n", len(mem))

	// 3. Initialize IVT
	fmt.Println("[2/5] Initialized IVT (256 vectors -> default HLT handler at 0x0400)")

	// 4. Boot params
	fmt.Printf("[3/5] Boot params at 0x%04X (magic: 0x%08X, version: %d)\n",
		upc.BootParamsBase, upc.BootMagic, upc.BootProtocolVersion)

	// 5. Kernel stats
	if kernelPath != "" {
		kernelWords, err := upc.LoadKernel(kernelPath)
		if err != nil {
			return fmt.Errorf("load kernel for stats: %w", err)
		}
		sizeBytes := len(kernelWords) * 4
		fmt.Printf("[4/5] Kernel loaded: %d words (%d bytes, %.1f KB)\n",
			len(kernelWords), sizeBytes, float64(sizeBytes)/1024.0)
	}

	// 6. Write to BPF maps (or dry-run)
	if dryRun {
		fmt.Println("[5/5] [DRY-RUN] Would write to BPF maps:")
		printMemorySummary(mem)
		fmt.Println()
		fmt.Println("[DRY-RUN] Would set CPU state:")
		fmt.Printf("  PC = 0x%08X (kernel entry)\n", cfg.KernelLoadAddr)
		fmt.Printf("  SP = 0x%08X (stack top)\n", cfg.StackTop)
		fmt.Printf("  NumProcesses = %d\n", cfg.NumProcesses)
		return nil
	}

	// Try to open RAM_MAP
	ramMap, bpfErr := openBPFMap(mapPinPath + "/RAM_MAP")
	if bpfErr != nil {
		fmt.Printf("[5/5] [DRY-RUN] BPF maps not available: %v\n", bpfErr)
		printMemorySummary(mem)
		fmt.Println()
		fmt.Println("[DRY-RUN] Would set CPU state:")
		fmt.Printf("  PC = 0x%08X\n", cfg.KernelLoadAddr)
		fmt.Printf("  SP = 0x%08X\n", cfg.StackTop)
		return nil
	}
	defer ramMap.Close()

	// Write memory words
	fmt.Printf("[5/5] Writing %d words to RAM_MAP...\n", len(mem))
	written := 0
	for addr, val := range mem {
		if err := ramMap.Update(addr, val); err != nil {
			return fmt.Errorf("RAM_MAP update at word 0x%X: %w", addr, err)
		}
		written++
	}
	fmt.Printf("       Wrote %d words to RAM_MAP\n", written)

	// Set CPU state
	cpuMap, err := openBPFMap(mapPinPath + "/CPU_MAP")
	if err != nil {
		fmt.Printf("WARNING: Could not open CPU_MAP: %v\n", err)
		fmt.Println("CPU state not initialized. Set PC and SP manually.")
		return nil
	}
	defer cpuMap.Close()

	cpu := CpuState{}
	cpu.PC = cfg.KernelLoadAddr >> 2 // Word address for PC
	cpu.Regs[15] = cfg.StackTop      // SP
	cpu.NumProcesses = cfg.NumProcesses
	cpu.ProgramBreak = 0x0040_0000

	if err := cpuMap.Update(uint32(0), cpu); err != nil {
		return fmt.Errorf("CPU_MAP update: %w", err)
	}

	fmt.Println()
	fmt.Println("=== Boot Complete ===")
	fmt.Printf("  PC = 0x%08X (kernel entry point)\n", cfg.KernelLoadAddr)
	fmt.Printf("  SP = 0x%08X\n", cfg.StackTop)
	fmt.Printf("  NumProcesses = %d\n", cfg.NumProcesses)
	fmt.Println("  CPU is ready. Inject ticks to begin execution.")

	return nil
}

// printMemorySummary prints a summary of memory regions being written.
func printMemorySummary(mem map[uint32]uint32) {
	// Group by region
	type region struct {
		name  string
		start uint32
		end   uint32
		count int
	}

	regions := []region{
		{"IVT", 0x0000, 0x00FF, 0},
		{"Boot Params", upc.BootParamsBaseWord, upc.BootParamsBaseWord + 31, 0},
		{"Boot Args", upc.BootArgsBaseWord, upc.BootArgsBaseWord + 63, 0},
		{"HLT Handler", upc.DefaultHandlerAddrWord, upc.DefaultHandlerAddrWord, 0},
		{"Kernel", upc.KernelBase >> 2, 0xFFFFFFFF, 0},
	}

	// Collect all addresses sorted
	addrs := make([]uint32, 0, len(mem))
	for addr := range mem {
		addrs = append(addrs, addr)
	}
	sort.Slice(addrs, func(i, j int) bool { return addrs[i] < addrs[j] })

	for _, addr := range addrs {
		for i := range regions {
			if addr >= regions[i].start && addr <= regions[i].end {
				regions[i].count++
				break
			}
		}
	}

	for _, r := range regions {
		if r.count > 0 {
			fmt.Printf("  %-12s 0x%04X-0x%04X: %d words\n",
				r.name, r.start*4, r.end*4, r.count)
		}
	}
}
