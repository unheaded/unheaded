// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

//go:build linux

// latency_sweep.go — one pass over LATENCY_MAP, read in batches.
//
// The sweep used to walk the map with Iterate(): two bpf() calls per entry
// (next key, then lookup), every 200 ms. On a full map that is 65,536 x 2 x 5
// calls a second, most of a core for a collector that then published nothing
// new (FND-044, ADR-144). A batch lookup reads thousands of entries per call,
// so a full sweep is a few tens of calls.
package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"time"

	ciliumebpf "github.com/cilium/ebpf"

	"unheaded/pkg/metrics/auto"
	"unheaded/pkg/metrics/prom"
)

// latencySweepBatch is how many entries one batch lookup asks the kernel for
// (map_walk.go).
const latencySweepBatch = mapSweepBatch

// minLatencySweepInterval is the shortest --latency-sweep-interval accepted.
const minLatencySweepInterval = 500 * time.Millisecond

// checkLatencySweepInterval refuses an interval that would bring back the
// cost the batched sweep removed.
func checkLatencySweepInterval(d time.Duration) error {
	if d < minLatencySweepInterval {
		return fmt.Errorf("--latency-sweep-interval must be at least %v, not %v", minLatencySweepInterval, d)
	}
	return nil
}

var (
	latencySweepSeconds = auto.NewHistogram(prom.HistogramOpts{
		Name:    "trace_collector_latency_sweep_seconds",
		Help:    "Time one complete sweep of LATENCY_MAP took",
		Buckets: []float64{0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
	})

	latencyMapEntries = auto.NewGauge(prom.GaugeOpts{
		Name: "trace_collector_latency_map_entries",
		Help: "Entries in LATENCY_MAP at the last complete sweep (the map holds at most 65536)",
	})

	latencyDecodeErrors = auto.NewCounter(prom.CounterOpts{
		Name: "trace_collector_latency_decode_errors_total",
		Help: "LATENCY_MAP entries whose key or value was not the size this binary reads; not zero means the probe object and the collector are from different builds",
	})
)

// latencySweeper reads LATENCY_MAP and publishes each entry that took a new
// sample since the last sweep. It is used from one goroutine.
type latencySweeper struct {
	w       *mapWalker // the batched pass, shared with the flow reader (map_walk.go)
	rtts    *rttTracker
	publish func([]byte)
	now     func() time.Time
}

func newLatencySweeper(m batchedMap, publish func([]byte)) *latencySweeper {
	return &latencySweeper{
		w:       newMapWalker("latency", m),
		rtts:    newRTTTracker(),
		publish: publish,
		now:     time.Now,
	}
}

// sweep makes one pass over the map. On an error the pass was partial: the
// tracker keeps what it knew (it must not forget entries the pass did not
// reach), and the entries gauge and the sweep histogram are left alone.
func (s *latencySweeper) sweep(ctx context.Context) error {
	start := time.Now()
	entries, undecodable := 0, 0

	visit := func(key, value []byte) {
		entries++
		if len(key) != LatencyTraceKeySize || len(value) < LatencyEntrySize {
			undecodable++
			return
		}
		// Only threads with a new measurement since the last sweep; re-sending
		// every thread's latest RTT each sweep weighted percentiles by threads
		// x sweeps. Samples is read in place so a quiet entry costs no decode.
		if !s.rtts.fresh(key, binary.LittleEndian.Uint64(value[24:32])) {
			return
		}
		lk, err := DecodeLatencyTraceKey(key)
		if err != nil {
			undecodable++
			return
		}
		le, err := DecodeLatencyEntry(value)
		if err != nil {
			undecodable++
			return
		}
		payload, err := json.Marshal(latencyMapEvent(lk, le, s.now()))
		if err != nil {
			return
		}
		s.publish(payload)
	}

	err := s.w.read(ctx, visit)
	if undecodable > 0 {
		latencyDecodeErrors.Add(float64(undecodable))
	}
	if err != nil {
		latencyReaderErrors.WithLabelValues("sweep").Inc()
		return err
	}
	s.rtts.endSweep()
	latencyMapEntries.Set(float64(entries))
	latencySweepSeconds.Observe(time.Since(start).Seconds())
	return nil
}

// newEBPFLatencyMap is the kernel's LATENCY_MAP as a batchedMap.
func newEBPFLatencyMap(m *ciliumebpf.Map) batchedMap {
	return newEBPFBatchMap(m,
		func(k *[LatencyTraceKeySize]byte) []byte { return k[:] },
		func(v *[LatencyEntrySize]byte) []byte { return v[:] })
}
