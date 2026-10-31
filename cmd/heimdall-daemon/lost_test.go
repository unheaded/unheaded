// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"fmt"
	"strings"
	"testing"
)

// LICH-012 L12d (2026-10-30): a full ringbuf drops hook events in the kernel.
// Each scan turns the hooks' cumulative lost counters into one log line per
// hook that lost anything and heimdall_hook_events_lost_total{hook}, so a
// flood that blinds the hooks is never silent.
func TestReportLost_LogsAndCountsDeltasOnly(t *testing.T) {
	cur := map[string]uint64{"vfs_write": 0, "execve": 0, "reader": 0}
	var lines []string
	h := &Heimdall{
		hookLost: func() map[string]uint64 { return cur },
		metrics:  newHeimdallMetrics(),
	}
	logf := func(f string, a ...any) { lines = append(lines, fmt.Sprintf(f, a...)) }

	h.reportLost(logf)
	if len(lines) != 0 {
		t.Fatalf("no loss, but logged %q", lines)
	}

	cur = map[string]uint64{"vfs_write": 1200, "execve": 0, "reader": 3}
	h.reportLost(logf)
	if len(lines) != 2 || !strings.Contains(lines[0], "reader: 3 events lost since the last scan (daemon fell behind)") ||
		!strings.Contains(lines[1], "vfs_write: 1200 events lost since the last scan (ringbuf full)") {
		t.Fatalf("lines %q", lines)
	}
	if got := h.metrics.lost.WithLabels(map[string]string{"hook": "vfs_write"}).Value(); got != 1200 {
		t.Errorf("vfs_write lost_total %v, want 1200", got)
	}

	lines = nil
	cur = map[string]uint64{"vfs_write": 1250, "execve": 0, "reader": 3}
	h.reportLost(logf)
	if len(lines) != 1 || !strings.Contains(lines[0], "vfs_write: 50 ") {
		t.Fatalf("second scan lines %q", lines)
	}
	if got := h.metrics.lost.WithLabels(map[string]string{"hook": "vfs_write"}).Value(); got != 1250 {
		t.Errorf("vfs_write lost_total %v, want 1250", got)
	}

	// A counter that goes backwards (hooks reloaded) counts from zero again.
	lines = nil
	cur = map[string]uint64{"vfs_write": 7, "execve": 0, "reader": 3}
	h.reportLost(logf)
	if len(lines) != 1 || !strings.Contains(lines[0], "vfs_write: 7 ") {
		t.Fatalf("after reset lines %q", lines)
	}
}

func TestReportLost_NoHooksNoOp(t *testing.T) {
	h := &Heimdall{}
	h.reportLost(func(string, ...any) { t.Fatal("logged without hooks") })
}
