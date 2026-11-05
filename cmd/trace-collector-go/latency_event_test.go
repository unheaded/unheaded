// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"testing"
	"time"
)

// LATENCY_MAP is keyed by trace id, pid and tid (DP-408). It used to carry
// those under 5-tuple names, truncated to 16 bits: "[::]:18570 -> [::]:18654".
func TestLatencyMapEvent_TraceKey(t *testing.T) {
	lk := &LatencyTraceKey{PID: 4194303, TID: 4194301}
	lk.TraceID[7], lk.TraceID[15] = 1, 2
	le := &LatencyEntry{RTTNS: 98879, MinRTTNS: 678, MaxRTTNS: 5460725, Samples: 607968}
	now := time.Unix(1792734554, 213496086)

	evt := latencyMapEvent(lk, le, now)
	if _, ok := evt["flow_key"]; ok {
		t.Errorf("flow_key present: the map key holds no addresses")
	}
	if evt["latency_ns"] != uint64(98879) || evt["samples"] != uint64(607968) {
		t.Errorf("latency_ns/samples = %v/%v, want 98879/607968", evt["latency_ns"], evt["samples"])
	}
	if evt["timestamp_ns"] != uint64(now.UnixNano()) {
		t.Errorf("timestamp_ns = %v, want wall clock %d", evt["timestamp_ns"], now.UnixNano())
	}
	if evt["pid"] != uint32(4194303) || evt["tid"] != uint32(4194301) {
		t.Errorf("pid/tid = %v/%v, want the full ids", evt["pid"], evt["tid"])
	}
	tr := evt["trace_id"].(map[string]uint64)
	if tr["high"] != 1 || tr["low"] != 2 {
		t.Errorf("trace_id = %v, want high 1 low 2", tr)
	}
}
