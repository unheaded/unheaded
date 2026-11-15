// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

//go:build linux

package main

import (
	"bytes"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"unheaded/pkg/metrics"
	"unheaded/pkg/metrics/metricstest"
	"unheaded/pkg/metrics/prom"
)

// fakeLatencyStats is LATENCY_STATS without a kernel.
type fakeLatencyStats struct {
	vals map[uint32]uint64
	err  error
}

func (f *fakeLatencyStats) stat(key uint32) (uint64, bool, error) {
	if f.err != nil {
		return 0, false, f.err
	}
	v, ok := f.vals[key]
	return v, ok, nil
}

func writeLatencyStats(t *testing.T, src latencyStatsSource) string {
	t.Helper()
	var buf bytes.Buffer
	if err := newLatencyStatsCollector(src).Write(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// The probe's own counters, by fixed names. Nothing exported them before, so
// a full ring buffer (events_dropped) and a full map were both invisible.
func TestLatencyStatsCollector(t *testing.T) {
	src := &fakeLatencyStats{vals: map[uint32]uint64{
		0: 10, 1: 9, 2: 8, 3: 7, 4: 6, 5: 5, 6: 4, 7: 3, 8: 2, 9: 1,
		12: 999, // a key this binary has no name for
	}}
	got := writeLatencyStats(t, src)
	want := `trace_collector_latency_probe_events_total{stat="send_enter"} 10
trace_collector_latency_probe_events_total{stat="send_exit"} 9
trace_collector_latency_probe_events_total{stat="recv_enter"} 8
trace_collector_latency_probe_events_total{stat="recv_exit"} 7
trace_collector_latency_probe_events_total{stat="connect_enter"} 6
trace_collector_latency_probe_events_total{stat="connect_exit"} 5
trace_collector_latency_probe_events_total{stat="events_sent"} 4
trace_collector_latency_probe_events_total{stat="events_dropped"} 3
trace_collector_latency_probe_events_total{stat="map_updates"} 2
trace_collector_latency_probe_events_total{stat="map_insert_failed"} 1
`
	if got != want {
		t.Fatalf("exposition\n got:\n%s\nwant:\n%s", got, want)
	}
	if strings.Contains(got, "999") {
		t.Fatal("an unknown stats key was exported")
	}

	d := newLatencyStatsCollector(src).Describe()
	if d.Type != metrics.TypeCounter {
		t.Fatalf("type = %v, want a counter", d.Type)
	}
	if len(d.VariableLabels) != 1 || d.VariableLabels[0] != "stat" {
		t.Fatalf("variable labels = %v, want [stat]", d.VariableLabels)
	}
}

// A key the probe has not written yet reads as 0: the series exists from the
// first scrape, so a rate() over it never starts from nothing.
func TestLatencyStatsCollector_UnwrittenKeyIsZero(t *testing.T) {
	got := writeLatencyStats(t, &fakeLatencyStats{vals: map[uint32]uint64{0: 3}})
	if !strings.Contains(got, `{stat="send_enter"} 3`+"\n") || !strings.Contains(got, `{stat="map_insert_failed"} 0`+"\n") {
		t.Fatalf("exposition:\n%s", got)
	}
	if n := strings.Count(got, "\n"); n != len(latencyStatNames) {
		t.Fatalf("%d series, want %d", n, len(latencyStatNames))
	}
}

// A failed read emits nothing (a half-read page would show false zeros) and
// is counted. It must not fail the scrape: every other series is on the page.
func TestLatencyStatsCollector_ReadError(t *testing.T) {
	before := latencyStatsReadErrors.Value()
	got := writeLatencyStats(t, &fakeLatencyStats{err: errors.New("bad file descriptor")})
	if got != "" {
		t.Fatalf("emitted on a read error:\n%s", got)
	}
	if d := latencyStatsReadErrors.Value() - before; d != 1 {
		t.Fatalf("trace_collector_latency_stats_read_errors_total rose by %v, want 1", d)
	}
}

// The names are the probe's keys 0-9 in order (ebpf/latency-probe/src).
func TestLatencyStatNamesMatchTheProbe(t *testing.T) {
	want := []string{"send_enter", "send_exit", "recv_enter", "recv_exit", "connect_enter", "connect_exit",
		"events_sent", "events_dropped", "map_updates", "map_insert_failed"}
	if len(latencyStatNames) != len(want) {
		t.Fatalf("%d names, want %d", len(latencyStatNames), len(want))
	}
	for i, n := range want {
		if latencyStatNames[i] != n {
			t.Fatalf("key %d is %q, want %q", i, latencyStatNames[i], n)
		}
	}
}

// Through a registry the family is well-formed text: one HELP, one TYPE
// counter, ten samples, nothing the format forbids.
func TestLatencyStatsCollector_Exposition(t *testing.T) {
	reg := prom.NewRegistry()
	reg.MustRegister(newLatencyStatsCollector(&fakeLatencyStats{vals: map[uint32]uint64{9: 4}}))
	rec := httptest.NewRecorder()
	reg.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	body := rec.Body.String()
	if problems := metricstest.Lint(body); len(problems) != 0 {
		t.Fatalf("lint: %v\n%s", problems, body)
	}
	if !strings.Contains(body, "# TYPE trace_collector_latency_probe_events_total counter\n") {
		t.Fatalf("no counter TYPE line:\n%s", body)
	}
	if !strings.Contains(body, `trace_collector_latency_probe_events_total{stat="map_insert_failed"} 4`+"\n") {
		t.Fatalf("no map_insert_failed sample:\n%s", body)
	}
}
