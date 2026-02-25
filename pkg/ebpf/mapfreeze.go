// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// mapfreeze.go provides BPF map freeze helpers for ROM protection.
//
// Remediation for Lich D1-001, D6-001: ROM_MAP must be frozen after load
// to prevent TOCTOU (time-of-check-time-of-use) attacks.
//
// Correct load sequence:
//
//  1. CREATE  - Create BPF map with BPF_F_RDONLY_PROG flag
//  2. LOAD    - Populate map entries (ROM words, etc.)
//  3. VERIFY  - Compute and check integrity (checksum, entry count)
//  4. FREEZE  - Call BPF_MAP_FREEZE to make map immutable
//  5. ATTACH  - Attach BPF program that uses the frozen map
//
// After FREEZE, any attempt to update the map (from userspace or BPF program)
// returns EPERM. This eliminates the TOCTOU window identified in D6-002 (~28ms).
//
// BPF_F_RDONLY_PROG (step 1) prevents in-BPF writes even before freeze.
// BPF_MAP_FREEZE (step 4) additionally prevents userspace writes.
// Together they provide defense-in-depth.
package ebpf

import "fmt"

// BPF map creation flags from include/uapi/linux/bpf.h
const (
	// BPF_F_RDONLY_PROG makes the map read-only from BPF program side.
	// Set during map creation. Prevents BPF programs from writing to the map.
	// D1-002: Required on ROM_MAP creation.
	BPF_F_RDONLY_PROG = 1 << 3 // 0x08

	// BPF_F_WRONLY_PROG makes the map write-only from BPF program side.
	BPF_F_WRONLY_PROG = 1 << 4 // 0x10

	// BPF_F_RDONLY makes the map read-only from userspace side.
	BPF_F_RDONLY = 1 << 5 // 0x20

	// BPF_F_WRONLY makes the map write-only from userspace side.
	BPF_F_WRONLY = 1 << 6 // 0x40
)

// BPF_MAP_FREEZE_CMD is the BPF syscall command number for freezing a map.
// After freeze, all userspace update/delete operations return EPERM.
// Defined in include/uapi/linux/bpf.h as BPF_MAP_FREEZE = 22.
const BPF_MAP_FREEZE_CMD = 22

// MapFreezeConfig holds the configuration for a frozen (immutable) BPF map.
// Use this to document and validate the correct load sequence.
type MapFreezeConfig struct {
	// MapName is the human-readable name (e.g., "ROM_MAP").
	MapName string

	// MapFD is the file descriptor of the BPF map to freeze.
	// Set after map creation (step 1).
	MapFD int

	// ReadOnlyProg indicates BPF_F_RDONLY_PROG was set during creation.
	// D1-002: MUST be true for ROM maps.
	ReadOnlyProg bool

	// ExpectedEntries is the number of entries expected after load.
	// Used during verification (step 3).
	ExpectedEntries uint32

	// Frozen indicates the map has been frozen (step 4 complete).
	Frozen bool

	// PinPath is the filesystem path where the map is pinned.
	// D1-003: Must have permissions 0600 root:root.
	PinPath string

	// PinPermissions is the expected file permissions for the pinned map.
	// D1-003: Must be 0600.
	PinPermissions uint32
}

// LoadSequenceStep represents a step in the BPF map load sequence.
type LoadSequenceStep int

const (
	// StepCreate is the initial map creation with appropriate flags.
	StepCreate LoadSequenceStep = iota
	// StepLoad is the population of map entries.
	StepLoad
	// StepVerify is the integrity verification (checksum, count).
	StepVerify
	// StepFreeze is the BPF_MAP_FREEZE call.
	StepFreeze
	// StepAttach is the BPF program attachment.
	StepAttach
)

// String returns the human-readable name of the load sequence step.
func (s LoadSequenceStep) String() string {
	switch s {
	case StepCreate:
		return "CREATE"
	case StepLoad:
		return "LOAD"
	case StepVerify:
		return "VERIFY"
	case StepFreeze:
		return "FREEZE"
	case StepAttach:
		return "ATTACH"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", int(s))
	}
}

// LoadSequence tracks the current step in the BPF map load sequence.
// It enforces that steps are executed in the correct order:
// CREATE -> LOAD -> VERIFY -> FREEZE -> ATTACH
type LoadSequence struct {
	config      MapFreezeConfig
	currentStep LoadSequenceStep
}

// NewLoadSequence creates a new load sequence tracker for a BPF map.
func NewLoadSequence(config MapFreezeConfig) *LoadSequence {
	return &LoadSequence{
		config:      config,
		currentStep: StepCreate,
	}
}

// ValidateStep checks whether the given step can be executed next.
// Returns an error if the step is out of order. Only the exact next step
// in the sequence is allowed (no skipping, no repeating).
func (ls *LoadSequence) ValidateStep(step LoadSequenceStep) error {
	if step < ls.currentStep {
		return fmt.Errorf("map %s: step %s already completed (current: %s)",
			ls.config.MapName, step, ls.currentStep)
	}
	if step > ls.currentStep {
		return fmt.Errorf("map %s: cannot skip to %s (current: %s, next must be %s)",
			ls.config.MapName, step, ls.currentStep, ls.currentStep)
	}
	return nil
}

// AdvanceTo marks the given step as complete and advances to the next step.
// Returns an error if the step is out of order.
func (ls *LoadSequence) AdvanceTo(step LoadSequenceStep) error {
	if err := ls.ValidateStep(step); err != nil {
		return err
	}
	ls.currentStep = step + 1
	if step == StepFreeze {
		ls.config.Frozen = true
	}
	return nil
}

// CurrentStep returns the next step that should be executed.
func (ls *LoadSequence) CurrentStep() LoadSequenceStep {
	return ls.currentStep
}

// IsFrozen returns true if the map has been frozen.
func (ls *LoadSequence) IsFrozen() bool {
	return ls.config.Frozen
}

// Config returns the current freeze configuration.
func (ls *LoadSequence) Config() MapFreezeConfig {
	return ls.config
}

// ValidateMapFreezeConfig checks that a MapFreezeConfig has the required
// security properties for ROM maps (D1, D6 findings).
func ValidateMapFreezeConfig(cfg MapFreezeConfig) []string {
	var issues []string

	if !cfg.ReadOnlyProg {
		issues = append(issues, "D1-002: BPF_F_RDONLY_PROG not set on map creation")
	}

	if cfg.PinPermissions != 0 && cfg.PinPermissions != 0600 {
		issues = append(issues, fmt.Sprintf(
			"D1-003: pin permissions are %04o, must be 0600", cfg.PinPermissions))
	}

	if cfg.PinPath == "" {
		issues = append(issues, "D1-003: pin path not set (map must be pinned for persistence)")
	}

	if cfg.ExpectedEntries == 0 {
		issues = append(issues, "D6-004: expected entry count not set (needed for verification)")
	}

	return issues
}

// ROMMapFlags returns the BPF map creation flags required for ROM maps.
// Combines BPF_F_RDONLY_PROG (prevent in-BPF writes) with any additional flags.
// D1-002: ROM maps MUST include BPF_F_RDONLY_PROG.
func ROMMapFlags(additionalFlags uint32) uint32 {
	return BPF_F_RDONLY_PROG | additionalFlags
}
