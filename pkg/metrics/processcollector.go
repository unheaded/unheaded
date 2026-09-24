// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package metrics

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// ProcessCollector publishes the process_* series that prometheus/client_golang
// registers by default — the same 7 names, types, help text and units.
//
// It replaces a placeholder whose Write returned nil: it built six series and
// emitted none, and ADR-094 listed it as done. process_resident_memory_bytes
// is the series you read when a service is OOM-killed.
//
// Linux only, like everything this repo deploys. Elsewhere Collectors
// returns nil and nothing is published rather than something invented.
type ProcessCollector struct {
	namespace string

	mu    sync.Mutex
	ttl   time.Duration
	taken time.Time
	snap  procSnapshot
	err   error

	// startTime never changes, so it is read once. A failure is kept and
	// the series omitted, not retried on every scrape.
	startTime    float64
	startTimeErr error
}

// procSnapshot is one reading of the process, shared by every process_*
// family in a scrape.
type procSnapshot struct {
	cpuSeconds  float64
	openFDs     float64
	maxFDs      float64
	virtualMem  float64
	virtualMax  float64
	residentMem float64
}

// NewProcessCollector returns a collector for the current process. An empty
// namespace gives client_golang's names (process_*); otherwise they are
// prefixed with namespace_.
func NewProcessCollector(namespace string) *ProcessCollector {
	pc := &ProcessCollector{namespace: namespace, ttl: goSnapshotTTL}
	pc.startTime, pc.startTimeErr = readProcessStartTime()
	return pc
}

func (pc *ProcessCollector) snapshot() (procSnapshot, error) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	if !pc.taken.IsZero() && time.Since(pc.taken) < pc.ttl {
		return pc.snap, pc.err
	}
	pc.snap, pc.err = readProcess()
	pc.taken = time.Now()
	return pc.snap, pc.err
}

// Collectors returns one Collector per process_* family, or nil where the
// platform has no /proc.
func (pc *ProcessCollector) Collectors() []Collector {
	if !processSupported {
		return nil
	}
	name := func(s string) string {
		if pc.namespace == "" {
			return "process_" + s
		}
		return pc.namespace + "_process_" + s
	}
	f := func(n, help string, t MetricType, v func(procSnapshot) float64) Collector {
		return &optionalMetric{
			desc: NewDesc(name(n), help, t, nil, nil),
			read: func() (float64, bool) {
				s, err := pc.snapshot()
				return v(s), err == nil
			},
		}
	}
	return []Collector{
		f("cpu_seconds_total", "Total user and system CPU time spent in seconds.", TypeCounter,
			func(s procSnapshot) float64 { return s.cpuSeconds }),
		f("max_fds", "Maximum number of open file descriptors.", TypeGauge,
			func(s procSnapshot) float64 { return s.maxFDs }),
		f("open_fds", "Number of open file descriptors.", TypeGauge,
			func(s procSnapshot) float64 { return s.openFDs }),
		f("resident_memory_bytes", "Resident memory size in bytes.", TypeGauge,
			func(s procSnapshot) float64 { return s.residentMem }),
		&optionalMetric{
			desc: NewDesc(name("start_time_seconds"), "Start time of the process since unix epoch in seconds.", TypeGauge, nil, nil),
			read: func() (float64, bool) { return pc.startTime, pc.startTimeErr == nil },
		},
		f("virtual_memory_bytes", "Virtual memory size in bytes.", TypeGauge,
			func(s procSnapshot) float64 { return s.virtualMem }),
		f("virtual_memory_max_bytes", "Maximum amount of virtual memory available in bytes.", TypeGauge,
			func(s procSnapshot) float64 { return s.virtualMax }),
	}
}

// Register adds every process_* family to reg.
func (pc *ProcessCollector) Register(reg *Registry) error {
	for _, c := range pc.Collectors() {
		if err := reg.Register(c); err != nil {
			return fmt.Errorf("process collector: %w", err)
		}
	}
	return nil
}

// optionalMetric omits its sample when the read fails. Reporting 0 instead
// would be worse than a gap: process_cpu_seconds_total dropping to 0 reads
// as a counter reset, and rate() turns a reset into a spike.
type optionalMetric struct {
	desc *Desc
	read func() (float64, bool)
}

func (o *optionalMetric) Describe() *Desc { return o.desc }

func (o *optionalMetric) Write(w io.Writer) error {
	v, ok := o.read()
	if !ok {
		return nil
	}
	if _, err := fmt.Fprintf(w, "%s %s\n", o.desc.Name, formatFloat(v)); err != nil {
		return fmt.Errorf("write %s: %w", o.desc.Name, err)
	}
	return nil
}
