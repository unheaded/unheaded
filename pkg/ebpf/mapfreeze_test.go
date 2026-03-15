// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package ebpf

import (
	"testing"
)

func TestBPFMapFreezeConstants(t *testing.T) {
	// Verify BPF constants match kernel header values (include/uapi/linux/bpf.h)
	if BPF_F_RDONLY_PROG != 0x08 {
		t.Errorf("BPF_F_RDONLY_PROG = %#x, want 0x08", BPF_F_RDONLY_PROG)
	}
	if BPF_F_WRONLY_PROG != 0x10 {
		t.Errorf("BPF_F_WRONLY_PROG = %#x, want 0x10", BPF_F_WRONLY_PROG)
	}
	if BPF_F_RDONLY != 0x20 {
		t.Errorf("BPF_F_RDONLY = %#x, want 0x20", BPF_F_RDONLY)
	}
	if BPF_F_WRONLY != 0x40 {
		t.Errorf("BPF_F_WRONLY = %#x, want 0x40", BPF_F_WRONLY)
	}
	if BPF_MAP_FREEZE_CMD != 22 {
		t.Errorf("BPF_MAP_FREEZE_CMD = %d, want 22", BPF_MAP_FREEZE_CMD)
	}
}

func TestROMMapFlags(t *testing.T) {
	// ROM maps MUST include BPF_F_RDONLY_PROG (D1-002)
	flags := ROMMapFlags(0)
	if flags&BPF_F_RDONLY_PROG == 0 {
		t.Error("ROMMapFlags(0) must include BPF_F_RDONLY_PROG")
	}

	// Additional flags should be OR'd in
	flags = ROMMapFlags(BPF_F_RDONLY)
	if flags != BPF_F_RDONLY_PROG|BPF_F_RDONLY {
		t.Errorf("ROMMapFlags(BPF_F_RDONLY) = %#x, want %#x",
			flags, BPF_F_RDONLY_PROG|BPF_F_RDONLY)
	}
}

func TestLoadSequence_CorrectOrder(t *testing.T) {
	// D6-004: Enforce CREATE -> LOAD -> VERIFY -> FREEZE -> ATTACH
	cfg := MapFreezeConfig{
		MapName:         "ROM_MAP",
		MapFD:           42,
		ReadOnlyProg:    true,
		ExpectedEntries: 1024,
		PinPath:         "/sys/fs/bpf/unheaded/doom-ring/maps/ROM_MAP",
		PinPermissions:  0600,
	}
	ls := NewLoadSequence(cfg)

	steps := []LoadSequenceStep{StepCreate, StepLoad, StepVerify, StepFreeze, StepAttach}
	for _, step := range steps {
		if err := ls.AdvanceTo(step); err != nil {
			t.Fatalf("step %s failed: %v", step, err)
		}
	}

	if !ls.IsFrozen() {
		t.Error("map should be frozen after StepFreeze")
	}
}

func TestLoadSequence_SkipStepFails(t *testing.T) {
	cfg := MapFreezeConfig{MapName: "ROM_MAP"}
	ls := NewLoadSequence(cfg)

	// Try to skip from CREATE to VERIFY (skipping LOAD)
	err := ls.AdvanceTo(StepVerify)
	if err == nil {
		t.Error("skipping LOAD step should fail")
	}
}

func TestLoadSequence_RepeatStepFails(t *testing.T) {
	cfg := MapFreezeConfig{MapName: "ROM_MAP"}
	ls := NewLoadSequence(cfg)

	if err := ls.AdvanceTo(StepCreate); err != nil {
		t.Fatalf("first CREATE failed: %v", err)
	}

	// Repeating CREATE should fail
	err := ls.AdvanceTo(StepCreate)
	if err == nil {
		t.Error("repeating CREATE step should fail")
	}
}

func TestLoadSequence_AttachBeforeFreezeFails(t *testing.T) {
	// D6-004: ATTACH must come AFTER FREEZE
	cfg := MapFreezeConfig{MapName: "ROM_MAP"}
	ls := NewLoadSequence(cfg)

	// Advance to VERIFY
	_ = ls.AdvanceTo(StepCreate)
	_ = ls.AdvanceTo(StepLoad)
	_ = ls.AdvanceTo(StepVerify)

	// Try to skip FREEZE and go to ATTACH
	err := ls.AdvanceTo(StepAttach)
	if err == nil {
		t.Error("ATTACH before FREEZE should fail (D6-004)")
	}
}

func TestLoadSequence_CurrentStep(t *testing.T) {
	cfg := MapFreezeConfig{MapName: "ROM_MAP"}
	ls := NewLoadSequence(cfg)

	if ls.CurrentStep() != StepCreate {
		t.Errorf("initial step should be CREATE, got %s", ls.CurrentStep())
	}

	_ = ls.AdvanceTo(StepCreate)
	if ls.CurrentStep() != StepLoad {
		t.Errorf("after CREATE, step should be LOAD, got %s", ls.CurrentStep())
	}
}

func TestLoadSequenceStep_String(t *testing.T) {
	tests := []struct {
		step LoadSequenceStep
		want string
	}{
		{StepCreate, "CREATE"},
		{StepLoad, "LOAD"},
		{StepVerify, "VERIFY"},
		{StepFreeze, "FREEZE"},
		{StepAttach, "ATTACH"},
		{LoadSequenceStep(99), "UNKNOWN(99)"},
	}

	for _, tt := range tests {
		if got := tt.step.String(); got != tt.want {
			t.Errorf("LoadSequenceStep(%d).String() = %q, want %q", int(tt.step), got, tt.want)
		}
	}
}

func TestValidateMapFreezeConfig_Secure(t *testing.T) {
	cfg := MapFreezeConfig{
		MapName:         "ROM_MAP",
		MapFD:           42,
		ReadOnlyProg:    true,
		ExpectedEntries: 1024,
		PinPath:         "/sys/fs/bpf/unheaded/doom-ring/maps/ROM_MAP",
		PinPermissions:  0600,
	}

	issues := ValidateMapFreezeConfig(cfg)
	if len(issues) != 0 {
		t.Errorf("secure config should have no issues, got: %v", issues)
	}
}

func TestValidateMapFreezeConfig_NoReadOnlyProg(t *testing.T) {
	cfg := MapFreezeConfig{
		MapName:         "ROM_MAP",
		ReadOnlyProg:    false, // D1-002 violation
		ExpectedEntries: 1024,
		PinPath:         "/sys/fs/bpf/unheaded/doom-ring/maps/ROM_MAP",
		PinPermissions:  0600,
	}

	issues := ValidateMapFreezeConfig(cfg)
	found := false
	for _, issue := range issues {
		if issue == "D1-002: BPF_F_RDONLY_PROG not set on map creation" {
			found = true
		}
	}
	if !found {
		t.Error("expected D1-002 finding for missing BPF_F_RDONLY_PROG")
	}
}

func TestValidateMapFreezeConfig_BadPermissions(t *testing.T) {
	cfg := MapFreezeConfig{
		MapName:         "ROM_MAP",
		ReadOnlyProg:    true,
		ExpectedEntries: 1024,
		PinPath:         "/sys/fs/bpf/unheaded/doom-ring/maps/ROM_MAP",
		PinPermissions:  0644, // D1-003 violation
	}

	issues := ValidateMapFreezeConfig(cfg)
	found := false
	for _, issue := range issues {
		if len(issue) > 5 && issue[:6] == "D1-003" {
			found = true
		}
	}
	if !found {
		t.Error("expected D1-003 finding for bad pin permissions")
	}
}

func TestValidateMapFreezeConfig_NoPinPath(t *testing.T) {
	cfg := MapFreezeConfig{
		MapName:         "ROM_MAP",
		ReadOnlyProg:    true,
		ExpectedEntries: 1024,
		PinPath:         "", // Missing
		PinPermissions:  0600,
	}

	issues := ValidateMapFreezeConfig(cfg)
	found := false
	for _, issue := range issues {
		if len(issue) > 5 && issue[:6] == "D1-003" {
			found = true
		}
	}
	if !found {
		t.Error("expected D1-003 finding for missing pin path")
	}
}

func TestValidateMapFreezeConfig_NoExpectedEntries(t *testing.T) {
	cfg := MapFreezeConfig{
		MapName:         "ROM_MAP",
		ReadOnlyProg:    true,
		ExpectedEntries: 0, // Missing
		PinPath:         "/sys/fs/bpf/unheaded/doom-ring/maps/ROM_MAP",
		PinPermissions:  0600,
	}

	issues := ValidateMapFreezeConfig(cfg)
	found := false
	for _, issue := range issues {
		if len(issue) > 5 && issue[:6] == "D6-004" {
			found = true
		}
	}
	if !found {
		t.Error("expected D6-004 finding for missing expected entries")
	}
}

func TestValidateMapFreezeConfig_AllIssues(t *testing.T) {
	cfg := MapFreezeConfig{
		MapName: "ROM_MAP",
		// All fields missing/wrong
	}

	issues := ValidateMapFreezeConfig(cfg)
	if len(issues) < 3 {
		t.Errorf("expected at least 3 issues for empty config, got %d: %v", len(issues), issues)
	}
}

func TestLoadSequence_Config(t *testing.T) {
	cfg := MapFreezeConfig{
		MapName:         "ROM_MAP",
		MapFD:           42,
		ReadOnlyProg:    true,
		ExpectedEntries: 1024,
	}
	ls := NewLoadSequence(cfg)

	got := ls.Config()
	if got.MapName != "ROM_MAP" {
		t.Errorf("Config().MapName = %q, want %q", got.MapName, "ROM_MAP")
	}
	if got.MapFD != 42 {
		t.Errorf("Config().MapFD = %d, want 42", got.MapFD)
	}
}
