// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package doom

import (
	"encoding/binary"
	"fmt"
	"strings"
)

// DefaultCpuState returns a CpuState initialized to the default boot state:
//   - PC = 0 (start of ROM)
//   - SP (r15) = 0xFFFF_0000 (top of RAM)
//   - All other registers = 0
//   - All flags/counters = 0
func DefaultCpuState() *CpuState {
	cpu := &CpuState{}
	cpu.Regs[RegSP] = RegSPDefault
	cpu.PC = RegPCDefault
	return cpu
}

// ReadCpuState reads the CPU state from a MapAccessor (cpu_map) at key 0.
func ReadCpuState(m MapAccessor) (*CpuState, error) {
	key := make([]byte, 4)
	binary.LittleEndian.PutUint32(key, 0)

	data, err := m.LookupElem(key, CpuStateSize)
	if err != nil {
		return nil, fmt.Errorf("cpu_map lookup: %w", err)
	}

	return UnmarshalCpuState(data)
}

// WriteCpuState writes the CPU state to a MapAccessor (cpu_map) at key 0.
func WriteCpuState(m MapAccessor, cpu *CpuState) error {
	key := make([]byte, 4)
	binary.LittleEndian.PutUint32(key, 0)

	return m.UpdateElem(key, cpu.MarshalBinary())
}

// ResetCpuState writes a default CPU state to the map, effectively rebooting
// the CPU to its initial state.
func ResetCpuState(m MapAccessor) error {
	return WriteCpuState(m, DefaultCpuState())
}

// IsHalted returns true if the CPU has executed a HALT instruction.
func (c *CpuState) IsHalted() bool {
	return c.Halted != 0
}

// IsStalled returns true if the CPU is waiting for a cache miss to be serviced.
func (c *CpuState) IsStalled() bool {
	return c.Stalled != 0
}

// HasFlag returns true if the specified flag bit is set.
func (c *CpuState) HasFlag(flag uint8) bool {
	return c.Flags&flag != 0
}

// FormatTable returns a human-readable table of the CPU state.
func (c *CpuState) FormatTable() string {
	var sb strings.Builder

	sb.WriteString("CPU State:\n")
	sb.WriteString(fmt.Sprintf("  PC:       0x%08X (%d)\n", c.PC, c.PC))

	// Flags
	flagStr := ""
	if c.HasFlag(FlagZero) {
		flagStr += "Z"
	} else {
		flagStr += "-"
	}
	if c.HasFlag(FlagNegative) {
		flagStr += "N"
	} else {
		flagStr += "-"
	}
	if c.HasFlag(FlagCarry) {
		flagStr += "C"
	} else {
		flagStr += "-"
	}
	sb.WriteString(fmt.Sprintf("  Flags:    0x%02X [%s]\n", c.Flags, flagStr))

	// Status
	status := "running"
	if c.IsHalted() {
		status = "HALTED"
	} else if c.IsStalled() {
		status = "STALLED"
	}
	sb.WriteString(fmt.Sprintf("  Status:   %s\n", status))

	// Counters
	sb.WriteString(fmt.Sprintf("  Insns:    %d\n", c.InsnCount))
	sb.WriteString(fmt.Sprintf("  Cache:    %d hits, %d misses", c.CacheHits, c.CacheMisses))
	total := c.CacheHits + c.CacheMisses
	if total > 0 {
		hitRate := float64(c.CacheHits) / float64(total) * 100.0
		sb.WriteString(fmt.Sprintf(" (%.1f%% hit rate)", hitRate))
	}
	sb.WriteString("\n")

	if c.SleepUntil > 0 {
		sb.WriteString(fmt.Sprintf("  Sleep:    until %d ns\n", c.SleepUntil))
	}

	// Registers
	sb.WriteString("  Registers:\n")
	for i := 0; i < RegCount; i++ {
		name := fmt.Sprintf("r%d", i)
		if i == RegSP {
			name = "SP"
		}
		sb.WriteString(fmt.Sprintf("    %-4s = 0x%08X (%d)\n", name, c.Regs[i], c.Regs[i]))
	}

	return sb.String()
}

// FormatOneLiner returns a compact one-line summary of the CPU state.
func (c *CpuState) FormatOneLiner() string {
	status := "run"
	if c.IsHalted() {
		status = "HALT"
	} else if c.IsStalled() {
		status = "STALL"
	}

	flagStr := ""
	if c.HasFlag(FlagZero) {
		flagStr += "Z"
	}
	if c.HasFlag(FlagNegative) {
		flagStr += "N"
	}
	if c.HasFlag(FlagCarry) {
		flagStr += "C"
	}
	if flagStr == "" {
		flagStr = "-"
	}

	return fmt.Sprintf("PC=0x%08X SP=0x%08X flags=%s status=%s insns=%d",
		c.PC, c.Regs[RegSP], flagStr, status, c.InsnCount)
}
