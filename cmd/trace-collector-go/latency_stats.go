// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

//go:build linux

// latency_stats.go — latency-probe's own counters (LATENCY_STATS) on /metrics.
//
// The probe counts what it does in a small hash map: kprobe entries and
// exits, ring buffer events sent and dropped, LATENCY_MAP inserts taken and
// refused. Nothing read it, so a full ring buffer and a full map were both
// silent (FND-044). The map is read at scrape time, ten lookups.

package main

import (
	"errors"
	"fmt"
	"io"

	ciliumebpf "github.com/cilium/ebpf"

	"unheaded/pkg/metrics"
	"unheaded/pkg/metrics/auto"
	"unheaded/pkg/metrics/prom"
)

// latencyStatNames are the LATENCY_STATS keys in order: the index is the key
// (ebpf/latency-probe/src/main.rs STAT_*, common.rs for 8 and 9). The label
// alphabet is this list and nothing else: a key outside it is never exported.
var latencyStatNames = [...]string{
	"send_enter",
	"send_exit",
	"recv_enter",
	"recv_exit",
	"connect_enter",
	"connect_exit",
	"events_sent",
	"events_dropped",
	"map_updates",
	"map_insert_failed",
}

const latencyStatsMetric = "trace_collector_latency_probe_events_total"

var latencyStatsReadErrors = auto.NewCounter(prom.CounterOpts{
	Name: "trace_collector_latency_stats_read_errors_total",
	Help: "Scrapes that could not read latency-probe's LATENCY_STATS map (its series are left out of that scrape)",
})

// latencyStatsSource reads one LATENCY_STATS counter. ok is false for a key
// the probe has not written yet.
type latencyStatsSource interface {
	stat(key uint32) (value uint64, ok bool, err error)
}

// latencyStatsCollector exposes LATENCY_STATS as one counter family.
type latencyStatsCollector struct {
	src latencyStatsSource
}

func newLatencyStatsCollector(src latencyStatsSource) *latencyStatsCollector {
	return &latencyStatsCollector{src: src}
}

func (c *latencyStatsCollector) Describe() *metrics.Desc {
	return metrics.NewDesc(latencyStatsMetric,
		"latency-probe's own counters, read from its LATENCY_STATS map: kprobe entries and exits, ring buffer events sent and dropped, LATENCY_MAP inserts taken (map_updates) and refused (map_insert_failed)",
		metrics.TypeCounter, nil, []string{"stat"})
}

// Write emits the sample lines only; the registry writes HELP and TYPE. All
// ten are read first: on a read error none is written, because a page with
// some of them would show the rest as absent or zero.
func (c *latencyStatsCollector) Write(w io.Writer) error {
	var vals [len(latencyStatNames)]uint64
	for key := range latencyStatNames {
		v, _, err := c.src.stat(uint32(key))
		if err != nil {
			latencyStatsReadErrors.Inc()
			return nil // the scrape goes on without these series
		}
		vals[key] = v // an unwritten key is 0
	}
	for key, name := range latencyStatNames {
		if _, err := fmt.Fprintf(w, "%s{stat=%q} %d\n", latencyStatsMetric, name, vals[key]); err != nil {
			return fmt.Errorf("write %s: %w", latencyStatsMetric, err)
		}
	}
	return nil
}

// ebpfLatencyStats reads the kernel's LATENCY_STATS map.
type ebpfLatencyStats struct {
	m *ciliumebpf.Map
}

func (e ebpfLatencyStats) stat(key uint32) (uint64, bool, error) {
	var v uint64
	err := e.m.Lookup(&key, &v)
	if errors.Is(err, ciliumebpf.ErrKeyNotExist) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return v, true, nil
}
