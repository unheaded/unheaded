// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package validation provides input validation for Unheaded security boundaries.
//
// Remediation for Lich D3 and D5 findings:
//   - D3: SYSCALL topic input validation + source authentication
//   - D5: Defensive coding for SYSCALL handlers (bounds checks, default-deny)
package validation

import (
	"fmt"
	"sync"
	"time"
)

// Known SYSCALL numbers from ebpf/monad-common/src/lib.rs
// Only these syscalls are permitted; all others are rejected (default-deny).
const (
	// SYS_DRAW_FRAME renders the framebuffer to screen.
	SYS_DRAW_FRAME = 0x40

	// SYS_GET_TICKS returns the current tick count.
	SYS_GET_TICKS = 0x41

	// SYS_SLEEP puts the CPU to sleep for a number of milliseconds.
	SYS_SLEEP = 0x42

	// SYS_GET_KEY reads a keyboard input event.
	SYS_GET_KEY = 0x43
)

// SyscallName returns the human-readable name for a known syscall number.
func SyscallName(num uint32) string {
	switch num {
	case SYS_DRAW_FRAME:
		return "SYS_DRAW_FRAME"
	case SYS_GET_TICKS:
		return "SYS_GET_TICKS"
	case SYS_SLEEP:
		return "SYS_SLEEP"
	case SYS_GET_KEY:
		return "SYS_GET_KEY"
	default:
		return fmt.Sprintf("UNKNOWN(0x%02x)", num)
	}
}

// ValidateSyscallNumber checks whether a syscall number is in the allowed set.
// D5: Default-deny — only known syscall numbers (0x40-0x43) are accepted.
func ValidateSyscallNumber(num uint32) bool {
	return num >= SYS_DRAW_FRAME && num <= SYS_GET_KEY
}

// SyscallMessage represents a SYSCALL message received on the Wotan topic.
type SyscallMessage struct {
	// Source is the service name that sent this message.
	Source string

	// SyscallNumber is the syscall being requested.
	SyscallNumber uint32

	// MemoryAddr is the memory address argument (if applicable).
	MemoryAddr uint32

	// MemorySize is the size of the memory region (if applicable).
	MemorySize uint32

	// Timestamp is when the message was created.
	Timestamp time.Time
}

// SyscallValidator validates SYSCALL messages from Wotan topics.
// It enforces source authentication, syscall number validation, and rate limiting.
type SyscallValidator struct {
	mu             sync.Mutex
	allowedSources map[string]struct{}
	rateLimits     map[string]*rateBucket
	maxRate        int           // max messages per window
	rateWindow     time.Duration // rate limit window

	// Counters for observability
	accepted uint64
	rejected uint64
}

// rateBucket tracks message counts for rate limiting.
type rateBucket struct {
	count     int
	windowEnd time.Time
}

// SyscallValidatorConfig configures the SYSCALL validator.
type SyscallValidatorConfig struct {
	// AllowedSources is the list of service names permitted to send SYSCALL messages.
	// Empty list means all sources are rejected.
	AllowedSources []string

	// MaxRate is the maximum number of messages per source per window.
	// Zero means no rate limiting (not recommended for production).
	MaxRate int

	// RateWindow is the time window for rate limiting.
	RateWindow time.Duration
}

// NewSyscallValidator creates a new SYSCALL message validator.
func NewSyscallValidator(cfg SyscallValidatorConfig) *SyscallValidator {
	sources := make(map[string]struct{}, len(cfg.AllowedSources))
	for _, s := range cfg.AllowedSources {
		sources[s] = struct{}{}
	}

	maxRate := cfg.MaxRate
	if maxRate <= 0 {
		maxRate = 100 // Default: 100 messages per window
	}

	rateWindow := cfg.RateWindow
	if rateWindow <= 0 {
		rateWindow = time.Second // Default: 1 second window
	}

	return &SyscallValidator{
		allowedSources: sources,
		rateLimits:     make(map[string]*rateBucket),
		maxRate:        maxRate,
		rateWindow:     rateWindow,
	}
}

// ValidationError represents a specific validation failure.
type ValidationError struct {
	Code    int    // HTTP-style status code (400, 403, 429)
	Field   string // Which field failed validation
	Message string // Human-readable description
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("[%d] %s: %s", e.Code, e.Field, e.Message)
}

// Validate checks a SYSCALL message against all validation rules.
// Returns nil if the message is valid, or a ValidationError describing the failure.
//
// Validation order:
//  1. Source authentication (D3-001): Is the source in the allowed list?
//  2. Rate limiting (D3-004): Is the source under its rate limit?
//  3. Syscall number (D5-003): Is the syscall number known?
//  4. Memory bounds (D5): Are memory addresses in valid range?
func (v *SyscallValidator) Validate(msg SyscallMessage) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	// 1. Source authentication (D3-001)
	if _, ok := v.allowedSources[msg.Source]; !ok {
		v.rejected++
		return &ValidationError{
			Code:    403,
			Field:   "source",
			Message: fmt.Sprintf("unknown source %q not in allowed list", msg.Source),
		}
	}

	// 2. Rate limiting (D3-004)
	now := time.Now()
	bucket, exists := v.rateLimits[msg.Source]
	if !exists || now.After(bucket.windowEnd) {
		// New window
		v.rateLimits[msg.Source] = &rateBucket{
			count:     1,
			windowEnd: now.Add(v.rateWindow),
		}
	} else {
		bucket.count++
		if bucket.count > v.maxRate {
			v.rejected++
			return &ValidationError{
				Code:    429,
				Field:   "rate",
				Message: fmt.Sprintf("source %q exceeded rate limit (%d/%s)", msg.Source, v.maxRate, v.rateWindow),
			}
		}
	}

	// 3. Syscall number validation (D5-003)
	if !ValidateSyscallNumber(msg.SyscallNumber) {
		v.rejected++
		return &ValidationError{
			Code:    400,
			Field:   "syscall_number",
			Message: fmt.Sprintf("unknown syscall number 0x%02x (allowed: 0x40-0x43)", msg.SyscallNumber),
		}
	}

	// 4. Memory bounds checking (D5) — only for syscalls that use memory addresses
	if msg.MemorySize > 0 {
		if !ValidateMemoryRange(msg.MemoryAddr, msg.MemorySize, DefaultMaxMemoryAddr) {
			v.rejected++
			return &ValidationError{
				Code:    400,
				Field:   "memory_addr",
				Message: fmt.Sprintf("memory range [0x%x, 0x%x) out of bounds (max 0x%x)",
					msg.MemoryAddr, uint64(msg.MemoryAddr)+uint64(msg.MemorySize), DefaultMaxMemoryAddr),
			}
		}
	}

	v.accepted++
	return nil
}

// Stats returns the current accepted/rejected counters.
func (v *SyscallValidator) Stats() (accepted, rejected uint64) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.accepted, v.rejected
}

// ResetStats resets the accepted/rejected counters to zero.
func (v *SyscallValidator) ResetStats() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.accepted = 0
	v.rejected = 0
}
