// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package cartridge

// Benchmark baselines (S5 Phase 11 Steps 301-304), opt-in: CART_BENCH=1 and
// root. BPF_PROG_TEST_RUN with repeat; 5 runs of N repeats, median ns/pkt,
// noise = (max - min) / median. Lines start with "BENCH" for the sprint log.

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/cilium/ebpf"
)

const benchRepeat = 1_000_000

type benchResult struct {
	median, min, max time.Duration
}

func (r benchResult) noise() float64 {
	return float64(r.max-r.min) / float64(r.median)
}

func bench5(t *testing.T, run func() (time.Duration, error)) benchResult {
	t.Helper()
	var d []time.Duration
	for i := 0; i < 5; i++ {
		x, err := run()
		if err != nil {
			t.Fatal(err)
		}
		d = append(d, x)
	}
	sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
	return benchResult{median: d[2], min: d[0], max: d[4]}
}

func benchEnabled(t *testing.T) {
	t.Helper()
	if os.Getenv("CART_BENCH") != "1" {
		t.Skip("set CART_BENCH=1 to run the benchmark baselines")
	}
}

func logBench(t *testing.T, name string, r benchResult) {
	t.Helper()
	t.Logf("BENCH %s median=%dns min=%dns max=%dns noise=%.1f%%", name, r.median.Nanoseconds(), r.min.Nanoseconds(), r.max.Nanoseconds(), 100*r.noise())
	if r.noise() >= 0.10 {
		t.Logf("BENCH %s noise >= 10%%: pin the CPU (taskset -c 2) and rerun", name)
	}
}

// TestBench_Baselines records b0_empty (no active slot), b0_pass (pass.s in
// slot 0), and the native references: the heimdall Gjallarhorn XDP observer
// (a native parse-and-PASS program) and qos-ebpf (native token bucket; on a
// frame without the Monad HbH option it takes its early PASS path).
func TestBench_Baselines(t *testing.T) {
	benchEnabled(t)
	c := loadCart(t)
	pkt := IPv4Packet(17, ipA, ipB, 4000, 53, 128)
	xdp := func() (time.Duration, error) {
		_, d, err := c.BenchXDP(pkt, benchRepeat)
		return d, err
	}
	logBench(t, "b0_empty", bench5(t, xdp))
	mustSlot(t, c, 0, buildCart(t, "../../cartridges/pass.s", "xdp"), 1)
	logBench(t, "b0_pass", bench5(t, xdp))

	native := func(name, obj, prog string) {
		if _, err := os.Stat(obj); err != nil {
			t.Logf("BENCH %s skipped: %s not built", name, obj)
			return
		}
		coll, err := ebpf.LoadCollection(obj)
		if err != nil {
			t.Logf("BENCH %s skipped: %v", name, err)
			return
		}
		defer coll.Close()
		p := coll.Programs[prog]
		if p == nil {
			t.Logf("BENCH %s skipped: no program %s", name, prog)
			return
		}
		logBench(t, name, bench5(t, func() (time.Duration, error) {
			_, d, err := p.Benchmark(pkt, benchRepeat, nil)
			return d, err
		}))
	}
	root := filepath.Join("..", "..")
	native("native_gjallarhorn_xdp", filepath.Join(root, "crates", "heimdall-bpf", "target", "bpfel-unknown-none", "release", "heimdall-gjallarhorn-xdp"), "heimdall_gjallarhorn")
	native("native_qos_ebpf", filepath.Join(root, "ebpf", "target", "bpfel-unknown-none", "release", "qos-ebpf"), "qos_xdp")
}
