// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package validation

import (
	"math"
	"testing"
)

func TestValidateMemoryRange_Valid(t *testing.T) {
	tests := []struct {
		name    string
		addr    uint32
		size    uint32
		maxAddr uint32
	}{
		{"zero size", 0x100, 0, DefaultMaxMemoryAddr},
		{"start of memory", 0, 100, DefaultMaxMemoryAddr},
		{"framebuffer base", 0x100000, 160 * 100, DefaultMaxMemoryAddr},
		{"near end", DefaultMaxMemoryAddr - 100, 100, DefaultMaxMemoryAddr},
		{"single word", 0x500000, 1, DefaultMaxMemoryAddr},
		{"exact end", 0, DefaultMaxMemoryAddr, DefaultMaxMemoryAddr},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !ValidateMemoryRange(tt.addr, tt.size, tt.maxAddr) {
				t.Errorf("ValidateMemoryRange(0x%x, %d, 0x%x) = false, want true",
					tt.addr, tt.size, tt.maxAddr)
			}
		})
	}
}

func TestValidateMemoryRange_Invalid(t *testing.T) {
	tests := []struct {
		name    string
		addr    uint32
		size    uint32
		maxAddr uint32
	}{
		{"past end", DefaultMaxMemoryAddr - 10, 20, DefaultMaxMemoryAddr},
		{"at max", DefaultMaxMemoryAddr, 1, DefaultMaxMemoryAddr},
		{"overflow uint32", 0xFFFFFFFF, 10, DefaultMaxMemoryAddr},
		{"large size overflow", 0x80000000, 0x80000001, DefaultMaxMemoryAddr},
		{"way past end", 0x2000000, 1, DefaultMaxMemoryAddr},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if ValidateMemoryRange(tt.addr, tt.size, tt.maxAddr) {
				t.Errorf("ValidateMemoryRange(0x%x, %d, 0x%x) = true, want false",
					tt.addr, tt.size, tt.maxAddr)
			}
		})
	}
}

func TestValidateMemoryAddr(t *testing.T) {
	if !ValidateMemoryAddr(0, DefaultMaxMemoryAddr) {
		t.Error("address 0 should be valid")
	}
	if !ValidateMemoryAddr(DefaultMaxMemoryAddr-1, DefaultMaxMemoryAddr) {
		t.Error("address maxAddr-1 should be valid")
	}
	if ValidateMemoryAddr(DefaultMaxMemoryAddr, DefaultMaxMemoryAddr) {
		t.Error("address maxAddr should be invalid")
	}
	if ValidateMemoryAddr(0xFFFFFFFF, DefaultMaxMemoryAddr) {
		t.Error("address 0xFFFFFFFF should be invalid")
	}
}

func TestValidateSleepDuration(t *testing.T) {
	// Valid durations
	if !ValidateSleepDuration(0) {
		t.Error("0ms should be valid")
	}
	if !ValidateSleepDuration(1000) {
		t.Error("1000ms should be valid")
	}
	if !ValidateSleepDuration(MaxSleepMS) {
		t.Error("MaxSleepMS should be valid")
	}

	// Invalid durations
	if ValidateSleepDuration(MaxSleepMS + 1) {
		t.Error("MaxSleepMS+1 should be invalid")
	}
	if ValidateSleepDuration(0xFFFFFFFF) {
		t.Error("max uint32 should be invalid")
	}
}

func TestSaturatingMulNS(t *testing.T) {
	// Normal values
	if got := SaturatingMulNS(1); got != 1_000_000 {
		t.Errorf("SaturatingMulNS(1) = %d, want 1000000", got)
	}
	if got := SaturatingMulNS(1000); got != 1_000_000_000 {
		t.Errorf("SaturatingMulNS(1000) = %d, want 1000000000", got)
	}
	if got := SaturatingMulNS(60000); got != 60_000_000_000 {
		t.Errorf("SaturatingMulNS(60000) = %d, want 60000000000", got)
	}

	// Overflow should saturate to MaxUint64
	if got := SaturatingMulNS(math.MaxUint64); got != math.MaxUint64 {
		t.Errorf("SaturatingMulNS(MaxUint64) = %d, want MaxUint64", got)
	}
	if got := SaturatingMulNS(math.MaxUint64 / 500_000); got != math.MaxUint64 {
		t.Errorf("SaturatingMulNS(large) should saturate, got %d", got)
	}

	// Zero
	if got := SaturatingMulNS(0); got != 0 {
		t.Errorf("SaturatingMulNS(0) = %d, want 0", got)
	}
}

func TestBoundsCheckStats_Memory(t *testing.T) {
	stats := NewBoundsCheckStats()

	// Valid access
	if !stats.CheckAndRecordMemory(0x100, 50, DefaultMaxMemoryAddr) {
		t.Error("valid memory access rejected")
	}

	// Invalid access
	if stats.CheckAndRecordMemory(DefaultMaxMemoryAddr, 1, DefaultMaxMemoryAddr) {
		t.Error("invalid memory access accepted")
	}

	if stats.MemoryOOBCount() != 1 {
		t.Errorf("MemoryOOBCount() = %d, want 1", stats.MemoryOOBCount())
	}
	if stats.TotalChecks() != 2 {
		t.Errorf("TotalChecks() = %d, want 2", stats.TotalChecks())
	}
}

func TestBoundsCheckStats_Syscall(t *testing.T) {
	stats := NewBoundsCheckStats()

	if !stats.CheckAndRecordSyscall(SYS_DRAW_FRAME) {
		t.Error("valid syscall rejected")
	}
	if stats.CheckAndRecordSyscall(0xFF) {
		t.Error("invalid syscall accepted")
	}

	if stats.InvalidSyscallCount() != 1 {
		t.Errorf("InvalidSyscallCount() = %d, want 1", stats.InvalidSyscallCount())
	}
}

func TestBoundsCheckStats_Sleep(t *testing.T) {
	stats := NewBoundsCheckStats()

	if !stats.CheckAndRecordSleep(1000) {
		t.Error("valid sleep rejected")
	}
	if stats.CheckAndRecordSleep(MaxSleepMS + 1) {
		t.Error("invalid sleep accepted")
	}

	if stats.SleepOverflowCount() != 1 {
		t.Errorf("SleepOverflowCount() = %d, want 1", stats.SleepOverflowCount())
	}
}

func TestBoundsCheckStats_RejectionRate(t *testing.T) {
	stats := NewBoundsCheckStats()

	// No checks yet — rate should be 0
	if rate := stats.RejectionRate(); rate != 0 {
		t.Errorf("empty RejectionRate() = %f, want 0", rate)
	}

	// 2 valid, 2 rejected = 50%
	stats.CheckAndRecordMemory(0, 10, DefaultMaxMemoryAddr)         // valid
	stats.CheckAndRecordSyscall(SYS_GET_KEY)                        // valid
	stats.CheckAndRecordMemory(DefaultMaxMemoryAddr, 1, DefaultMaxMemoryAddr) // rejected
	stats.CheckAndRecordSyscall(0x00)                               // rejected

	rate := stats.RejectionRate()
	if rate != 50 {
		t.Errorf("RejectionRate() = %f, want 50", rate)
	}
}

func TestBoundsCheckStats_ConcurrentAccess(t *testing.T) {
	stats := NewBoundsCheckStats()
	done := make(chan struct{})

	// Run concurrent checks from multiple goroutines
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				stats.CheckAndRecordMemory(0, 10, DefaultMaxMemoryAddr)
				stats.CheckAndRecordSyscall(SYS_DRAW_FRAME)
				stats.CheckAndRecordSleep(1000)
			}
			done <- struct{}{}
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// 10 goroutines * 100 iterations * 3 checks = 3000
	if stats.TotalChecks() != 3000 {
		t.Errorf("TotalChecks() = %d, want 3000", stats.TotalChecks())
	}
}

func TestDefaultMaxMemoryAddr(t *testing.T) {
	// Verify the constant matches the documented 16M entry RAM_MAP
	if DefaultMaxMemoryAddr != 0x01000000 {
		t.Errorf("DefaultMaxMemoryAddr = 0x%x, want 0x01000000", DefaultMaxMemoryAddr)
	}
}

func TestMaxSleepMS(t *testing.T) {
	// Verify the constant is 60 seconds
	if MaxSleepMS != 60_000 {
		t.Errorf("MaxSleepMS = %d, want 60000", MaxSleepMS)
	}
}
