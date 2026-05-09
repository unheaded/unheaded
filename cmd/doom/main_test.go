// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"strings"
	"testing"
)

// ─── cmdLoad ────────────────────────────────────────────────────────────────

func TestCmdLoad_RequiresROMPath(t *testing.T) {
	t.Parallel()
	err := cmdLoad(nil)
	if err == nil {
		t.Fatal("expected error for missing ROM path")
	}
	if !strings.Contains(err.Error(), "ROM file") {
		t.Errorf("wrong error: %v", err)
	}
}

func TestCmdLoad_HelpFlagReturnsNil(t *testing.T) {
	t.Parallel()
	for _, flag := range []string{"-h", "--help"} {
		if err := cmdLoad([]string{flag}); err != nil {
			t.Errorf("%s: expected nil err, got %v", flag, err)
		}
	}
}

func TestCmdLoad_NonExistentROMErrors(t *testing.T) {
	t.Parallel()
	err := cmdLoad([]string{"/nonexistent/rom.bin"})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

// ─── cmdInput ───────────────────────────────────────────────────────────────

func TestCmdInput_RequiresBitmap(t *testing.T) {
	t.Parallel()
	err := cmdInput(nil)
	if err == nil {
		t.Fatal("expected error for missing bitmap")
	}
	if !strings.Contains(err.Error(), "key_bitmap") {
		t.Errorf("wrong error: %v", err)
	}
}

func TestCmdInput_HelpFlagReturnsNil(t *testing.T) {
	t.Parallel()
	for _, flag := range []string{"-h", "--help"} {
		if err := cmdInput([]string{flag}); err != nil {
			t.Errorf("%s: expected nil err, got %v", flag, err)
		}
	}
}

func TestCmdInput_InvalidBitmapErrors(t *testing.T) {
	t.Parallel()
	// doom.ParseKeyBitmap should reject non-hex input.
	err := cmdInput([]string{"not-hex"})
	if err == nil {
		t.Errorf("expected error for non-hex bitmap")
	}
}

func TestCmdInput_EmptyBitmapZero(t *testing.T) {
	t.Parallel()
	// 0x00 should parse fine and report no keys.
	if err := cmdInput([]string{"0x00"}); err != nil {
		t.Errorf("0x00 should not error, got %v", err)
	}
}

// ─── cmdReset ───────────────────────────────────────────────────────────────

func TestCmdReset_HelpFlagReturnsNil(t *testing.T) {
	t.Parallel()
	for _, flag := range []string{"-h", "--help"} {
		if err := cmdReset([]string{flag}); err != nil {
			t.Errorf("%s: expected nil err, got %v", flag, err)
		}
	}
}

// ─── cmdStatus ──────────────────────────────────────────────────────────────

func TestCmdStatus_HelpFlagReturnsNil(t *testing.T) {
	t.Parallel()
	for _, flag := range []string{"-h", "--help"} {
		if err := cmdStatus([]string{flag}); err != nil {
			t.Errorf("%s: expected nil err, got %v", flag, err)
		}
	}
}
