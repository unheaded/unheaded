// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package prom_test

import (
	"testing"

	"unheaded/pkg/metrics/prom"
)

// LatencyBuckets see a tenth of a millisecond (ADR-147, Scientist F1) and
// keep DefBuckets' bounds from 5 ms up, so old and new series share them.
func TestLatencyBuckets(t *testing.T) {
	b := prom.LatencyBuckets
	if len(b) != 16 || b[0] != 0.0001 || b[len(b)-1] != 10 {
		t.Fatalf("LatencyBuckets = %v: want 16 bounds from 0.0001 to 10", b)
	}
	for i := 1; i < len(b); i++ {
		if b[i] <= b[i-1] {
			t.Fatalf("not strictly increasing at %d: %v", i, b)
		}
	}
	have := map[float64]bool{}
	for _, v := range b {
		have[v] = true
	}
	for _, v := range []float64{0.005, 0.01, 0.025} {
		if !have[v] {
			t.Errorf("missing the old bound %v", v)
		}
	}
	for _, v := range prom.DefBuckets {
		if v >= 0.005 && !have[v] {
			t.Errorf("DefBuckets bound %v from 5 ms up is not kept", v)
		}
	}
}
