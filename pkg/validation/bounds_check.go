// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package validation

import (
	"sync/atomic"
)

// DefaultMaxMemoryAddr is the maximum valid memory address for BPF memory operations.
// This corresponds to the RAM_MAP Array size of 16M entries (0x1000000).
// See MEMORY.md: "Array needs 16M entries" (S31 fix).
const DefaultMaxMemoryAddr uint32 = 0x01000000 // 16 MiB word-addressable

// MaxSleepMS is the maximum allowed sleep duration in milliseconds.
// D5-001: SYS_SLEEP overflow — ms * 1_000_000 can overflow u64 for large values.
// Cap at 60 seconds to prevent overflow and denial of service.
const MaxSleepMS uint32 = 60_000

// ValidateMemoryRange checks that a memory range [addr, addr+size) is within bounds.
// Returns false if the range exceeds maxAddr or if addr+size overflows uint32.
//
// D5: All BPF memory operations must be bounds-checked before execution.
func ValidateMemoryRange(addr, size, maxAddr uint32) bool {
	if size == 0 {
		return true // Zero-size access is valid (no-op)
	}

	// Check for uint32 overflow: addr + size must not wrap
	end := uint64(addr) + uint64(size)
	if end > uint64(maxAddr) {
		return false
	}

	return true
}

// ValidateMemoryAddr checks that a single memory address is within bounds.
func ValidateMemoryAddr(addr, maxAddr uint32) bool {
	return addr < maxAddr
}

// ValidateSleepDuration checks that a sleep duration is within safe bounds.
// D5-001: Prevents u64 overflow in nanosecond conversion (ms * 1_000_000).
func ValidateSleepDuration(ms uint32) bool {
	return ms <= MaxSleepMS
}

// SaturatingMulNS converts milliseconds to nanoseconds with saturation.
// If ms * 1_000_000 would overflow uint64, returns math.MaxUint64 instead.
// D5-001 remediation: prevents overflow in SYS_SLEEP handler.
func SaturatingMulNS(ms uint64) uint64 {
	const nsPerMS = 1_000_000
	const maxSafeMS = ^uint64(0) / nsPerMS // ~18,446,744,073,709 ms

	if ms > maxSafeMS {
		return ^uint64(0) // saturate to max uint64
	}
	return ms * nsPerMS
}

// BoundsCheckStats tracks rejected operations for observability.
// Thread-safe via atomic operations.
type BoundsCheckStats struct {
	memoryOutOfBounds atomic.Uint64
	invalidSyscall    atomic.Uint64
	sleepOverflow     atomic.Uint64
	totalChecks       atomic.Uint64
}

// NewBoundsCheckStats creates a new stats tracker.
func NewBoundsCheckStats() *BoundsCheckStats {
	return &BoundsCheckStats{}
}

// RecordMemoryOOB increments the out-of-bounds memory access counter.
func (s *BoundsCheckStats) RecordMemoryOOB() {
	s.memoryOutOfBounds.Add(1)
	s.totalChecks.Add(1)
}

// RecordInvalidSyscall increments the invalid syscall counter.
func (s *BoundsCheckStats) RecordInvalidSyscall() {
	s.invalidSyscall.Add(1)
	s.totalChecks.Add(1)
}

// RecordSleepOverflow increments the sleep overflow counter.
func (s *BoundsCheckStats) RecordSleepOverflow() {
	s.sleepOverflow.Add(1)
	s.totalChecks.Add(1)
}

// RecordValidCheck increments only the total checks counter (no rejection).
func (s *BoundsCheckStats) RecordValidCheck() {
	s.totalChecks.Add(1)
}

// MemoryOOBCount returns the number of out-of-bounds memory rejections.
func (s *BoundsCheckStats) MemoryOOBCount() uint64 {
	return s.memoryOutOfBounds.Load()
}

// InvalidSyscallCount returns the number of invalid syscall rejections.
func (s *BoundsCheckStats) InvalidSyscallCount() uint64 {
	return s.invalidSyscall.Load()
}

// SleepOverflowCount returns the number of sleep overflow rejections.
func (s *BoundsCheckStats) SleepOverflowCount() uint64 {
	return s.sleepOverflow.Load()
}

// TotalChecks returns the total number of bounds checks performed.
func (s *BoundsCheckStats) TotalChecks() uint64 {
	return s.totalChecks.Load()
}

// RejectionRate returns the percentage of checks that resulted in rejection.
// Returns 0 if no checks have been performed.
func (s *BoundsCheckStats) RejectionRate() float64 {
	total := s.totalChecks.Load()
	if total == 0 {
		return 0
	}
	rejected := s.memoryOutOfBounds.Load() + s.invalidSyscall.Load() + s.sleepOverflow.Load()
	return float64(rejected) / float64(total) * 100
}

// CheckAndRecordMemory validates a memory range and records the result.
// Returns true if the access is within bounds.
func (s *BoundsCheckStats) CheckAndRecordMemory(addr, size, maxAddr uint32) bool {
	if ValidateMemoryRange(addr, size, maxAddr) {
		s.RecordValidCheck()
		return true
	}
	s.RecordMemoryOOB()
	return false
}

// CheckAndRecordSyscall validates a syscall number and records the result.
// Returns true if the syscall is known.
func (s *BoundsCheckStats) CheckAndRecordSyscall(num uint32) bool {
	if ValidateSyscallNumber(num) {
		s.RecordValidCheck()
		return true
	}
	s.RecordInvalidSyscall()
	return false
}

// CheckAndRecordSleep validates a sleep duration and records the result.
// Returns true if the duration is within safe bounds.
func (s *BoundsCheckStats) CheckAndRecordSleep(ms uint32) bool {
	if ValidateSleepDuration(ms) {
		s.RecordValidCheck()
		return true
	}
	s.RecordSleepOverflow()
	return false
}
