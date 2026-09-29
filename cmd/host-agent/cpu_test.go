// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import "testing"

func TestCPUSampler(t *testing.T) {
	var c cpuSampler
	c.observe(1000, 800) // prime: no window yet
	if got := c.percent(); got != 0 {
		t.Fatalf("after one reading = %v, want 0", got)
	}
	c.observe(1100, 830) // 100 ticks, 30 idle -> 70%
	if got := c.percent(); got != 70 {
		t.Fatalf("percent = %v, want 70", got)
	}
	// Readers do not move the window: every caller sees the same value.
	for i := 0; i < 3; i++ {
		if got := c.percent(); got != 70 {
			t.Fatalf("read %d = %v, want 70", i, got)
		}
	}
	// No elapsed ticks (same jiffy): keep 70, never publish 0.
	c.observe(1100, 830)
	if got := c.percent(); got != 70 {
		t.Errorf("zero-tick window = %v, want 70 kept", got)
	}
	c.observe(1300, 1030) // 200 ticks, 200 idle -> 0% is a real reading
	if got := c.percent(); got != 0 {
		t.Errorf("idle window = %v, want 0", got)
	}
}

func TestReadCPUStat(t *testing.T) {
	total, idle, ok := readCPUStat()
	if !ok {
		t.Skip("no /proc/stat")
	}
	if total <= 0 || idle < 0 || idle > total {
		t.Errorf("readCPUStat = %v, %v", total, idle)
	}
}
