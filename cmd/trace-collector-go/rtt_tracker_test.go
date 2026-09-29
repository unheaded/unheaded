// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import "testing"

// The latency map is swept every 200 ms. It used to emit every flow's
// latest RTT on every sweep, so one measurement became five samples a
// second for as long as the flow stayed in the map, and the dashboard's
// percentiles were weighted by flows x sweeps, not by measurements.
func TestRTTTracker(t *testing.T) {
	tr := newRTTTracker()
	a, b := []byte("flow-a"), []byte("flow-b")

	sweep := func(entries map[string]uint64) map[string]bool {
		out := map[string]bool{}
		for k, n := range entries {
			out[k] = tr.fresh([]byte(k), n)
		}
		tr.endSweep()
		return out
	}

	if got := sweep(map[string]uint64{string(a): 1, string(b): 0}); !got["flow-a"] || got["flow-b"] {
		t.Fatalf("first sweep %v: want a (1 sample) fresh, b (no samples) not", got)
	}
	if got := sweep(map[string]uint64{string(a): 1, string(b): 0}); got["flow-a"] || got["flow-b"] {
		t.Fatalf("unchanged sweep %v: want nothing fresh", got)
	}
	if got := sweep(map[string]uint64{string(a): 4, string(b): 1}); !got["flow-a"] || !got["flow-b"] {
		t.Fatalf("new samples %v: want both fresh", got)
	}
	// a leaves the map (evicted) and is forgotten; b stays quiet.
	sweep(map[string]uint64{string(b): 1})
	if len(tr.seen) != 1 {
		t.Fatalf("tracker holds %d flows after eviction, want 1", len(tr.seen))
	}
	// a comes back with a fresh counter: that is a new measurement.
	if got := sweep(map[string]uint64{string(a): 1, string(b): 1}); !got["flow-a"] || got["flow-b"] {
		t.Fatalf("re-created flow %v: want a fresh, b not", got)
	}
}
