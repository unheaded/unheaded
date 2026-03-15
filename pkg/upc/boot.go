// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package upc provides the Unheaded Protocol Computer boot infrastructure.
//
// The UPC boot protocol defines how kernel images are loaded into RAM,
// how the interrupt vector table is initialized, and how boot parameters
// are communicated to the kernel via a well-known memory region.
package upc

// BootConfig defines the kernel boot parameters.
type BootConfig struct {
	KernelPath     string // Path to kernel binary (MBC or raw binary)
	RamdiskPath    string // Optional: path to initrd/ramdisk image
	KernelLoadAddr uint32 // Where to load kernel in RAM (default: 0x10000)
	StackTop       uint32 // Initial stack pointer (default: 0x03F00000)
	RamdiskAddr    uint32 // Where to load ramdisk (default: RAMDISK_BASE)
	MMUEnabled     bool   // Enable paging after boot
	NumProcesses   uint8  // Initial process count (default: 1)
	BootArgs       string // Kernel command line (stored at boot params region)
}

// DefaultBootConfig returns sensible defaults for UPC boot.
func DefaultBootConfig() BootConfig {
	return BootConfig{
		KernelLoadAddr: KernelBase,
		StackTop:       StackTopDefault,
		RamdiskAddr:    RamdiskBase,
		NumProcesses:   1,
	}
}

// BootParams is written to RAM at 0x0100-0x01FF for the kernel to read.
// The kernel locates this structure by checking for the magic value "UNHD"
// (0x554E4844) at word address 0x40.
type BootParams struct {
	Magic        uint32 // 0x554E4844 = "UNHD"
	Version      uint32 // Boot protocol version (1)
	MemorySize   uint32 // Total RAM in bytes
	RamdiskAddr  uint32 // Ramdisk physical address
	RamdiskSize  uint32 // Ramdisk size in bytes
	KernelAddr   uint32 // Kernel load address
	KernelSize   uint32 // Kernel size in bytes
	BootArgsAddr uint32 // Address of command line string
	BootArgsLen  uint32 // Length of command line
	NumCPUs      uint32 // Number of CPU instances
	TickRateHz   uint32 // Timer interrupt rate (Hz)
	Reserved     [20]uint32
}

// Memory layout constants for the UPC boot protocol.
const (
	// IVT occupies RAM[0x0000-0x03FF] (256 entries x 4 bytes each).
	IVTBase    = 0x0000
	IVTEntries = 256
	IVTSize    = IVTEntries * 4 // 1024 bytes

	// Boot parameters at RAM[0x0100-0x01FF] (word addresses 0x40-0x7F).
	BootParamsBase     = 0x0100
	BootParamsBaseWord = BootParamsBase >> 2 // 0x40

	// Default halt handler at 0x0400 (word address 0x100).
	DefaultHandlerAddr     = 0x0400
	DefaultHandlerAddrWord = DefaultHandlerAddr >> 2

	// Kernel loads at 0x10000 (word address 0x4000).
	KernelBase = 0x10000

	// Boot args string at 0x0200 (word address 0x80).
	BootArgsBase     = 0x0200
	BootArgsBaseWord = BootArgsBase >> 2

	// Stack top default.
	StackTopDefault = 0x03F00000

	// Ramdisk base (RAMDISK_BASE_WORD * 4).
	RamdiskBase = 0x200000 * 4

	// Total RAM default (64 MB).
	DefaultMemorySize = 64 * 1024 * 1024

	// Timer tick rate default (~12 Hz).
	DefaultTickRateHz = 12

	// Boot magic: "UNHD" in little-endian.
	BootMagic = 0x554E4844

	// Boot protocol version.
	BootProtocolVersion = 1

	// HLT opcode for the default interrupt handler.
	HLTOpcode = 0x19000000
)
