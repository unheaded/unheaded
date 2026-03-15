// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package ebpf

import (
	"math"
	"sort"
	"sync"
	"time"
)

// LatencyHistogramConfig configures the latency histogram tracker.
type LatencyHistogramConfig struct {
	Windows    []time.Duration // Time windows (default: 1s, 10s, 60s)
	MaxSamples int             // Max samples per window per operation
}

// LatencyHistogram tracks latency measurements per operation type
// across multiple sliding time windows.
type LatencyHistogram struct {
	config  LatencyHistogramConfig
	windows map[LatencyOperation]map[time.Duration]*slidingWindow
	mu      sync.RWMutex

	// Aggregate counters
	totalSamples map[LatencyOperation]uint64
}

// slidingWindow stores timestamped latency samples within a window.
type slidingWindow struct {
	samples    []latencySample
	maxSamples int
}

type latencySample struct {
	timestamp time.Time
	latencyNs uint64
}

// NewLatencyHistogram creates a new latency histogram tracker.
func NewLatencyHistogram(config LatencyHistogramConfig) *LatencyHistogram {
	if len(config.Windows) == 0 {
		config.Windows = []time.Duration{1 * time.Second, 10 * time.Second, 60 * time.Second}
	}
	if config.MaxSamples == 0 {
		config.MaxSamples = 10000
	}

	ops := []LatencyOperation{OpTcpSend, OpTcpRecv, OpTcpConnect, OpTcpAccept}
	windows := make(map[LatencyOperation]map[time.Duration]*slidingWindow)
	totals := make(map[LatencyOperation]uint64)

	for _, op := range ops {
		windows[op] = make(map[time.Duration]*slidingWindow)
		for _, w := range config.Windows {
			windows[op][w] = &slidingWindow{
				samples:    make([]latencySample, 0, 256),
				maxSamples: config.MaxSamples,
			}
		}
		totals[op] = 0
	}

	return &LatencyHistogram{
		config:       config,
		windows:      windows,
		totalSamples: totals,
	}
}

// Ingest adds a latency event to all windows for its operation type.
func (lh *LatencyHistogram) Ingest(e *LatencyEvent) {
	lh.mu.Lock()
	defer lh.mu.Unlock()

	opWindows, ok := lh.windows[e.Operation]
	if !ok {
		return
	}

	sample := latencySample{
		timestamp: e.Time(),
		latencyNs: e.LatencyNs,
	}

	lh.totalSamples[e.Operation]++

	for _, sw := range opWindows {
		sw.samples = append(sw.samples, sample)
		if len(sw.samples) > sw.maxSamples {
			sw.samples = sw.samples[len(sw.samples)-sw.maxSamples:]
		}
	}
}

// Expire removes samples outside each window's time range.
func (lh *LatencyHistogram) Expire() {
	now := time.Now()

	lh.mu.Lock()
	defer lh.mu.Unlock()

	for _, opWindows := range lh.windows {
		for windowDur, sw := range opWindows {
			cutoff := now.Add(-windowDur)
			i := sort.Search(len(sw.samples), func(i int) bool {
				return sw.samples[i].timestamp.After(cutoff)
			})
			if i > 0 {
				sw.samples = sw.samples[i:]
			}
		}
	}
}

// GetPercentiles returns P50/P90/P99 for a given operation and window.
func (lh *LatencyHistogram) GetPercentiles(op LatencyOperation, window time.Duration) PercentileResult {
	now := time.Now()
	cutoff := now.Add(-window)

	lh.mu.RLock()
	defer lh.mu.RUnlock()

	opWindows, ok := lh.windows[op]
	if !ok {
		return PercentileResult{}
	}

	sw, ok := opWindows[window]
	if !ok {
		return PercentileResult{}
	}

	// Collect active samples
	var values []uint64
	for _, s := range sw.samples {
		if s.timestamp.After(cutoff) {
			values = append(values, s.latencyNs)
		}
	}

	if len(values) == 0 {
		return PercentileResult{Window: window, SampleCount: 0}
	}

	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })

	return PercentileResult{
		Window:      window,
		SampleCount: len(values),
		P50Ns:       percentile(values, 0.50),
		P90Ns:       percentile(values, 0.90),
		P99Ns:       percentile(values, 0.99),
		MinNs:       values[0],
		MaxNs:       values[len(values)-1],
		MeanNs:      mean(values),
	}
}

// GetAllPercentiles returns percentiles for all operations and windows.
func (lh *LatencyHistogram) GetAllPercentiles() map[LatencyOperation][]PercentileResult {
	result := make(map[LatencyOperation][]PercentileResult)
	for _, op := range []LatencyOperation{OpTcpSend, OpTcpRecv, OpTcpConnect, OpTcpAccept} {
		var perOp []PercentileResult
		for _, w := range lh.config.Windows {
			perOp = append(perOp, lh.GetPercentiles(op, w))
		}
		result[op] = perOp
	}
	return result
}

// Stats returns aggregate latency histogram stats.
func (lh *LatencyHistogram) Stats() LatencyStats {
	lh.mu.RLock()
	defer lh.mu.RUnlock()

	totals := make(map[string]uint64)
	for op, count := range lh.totalSamples {
		totals[string(op)] = count
	}

	return LatencyStats{
		TotalSamples: totals,
		Windows:      lh.config.Windows,
	}
}

// PercentileResult holds computed percentile values.
type PercentileResult struct {
	Window      time.Duration `json:"window"`
	SampleCount int           `json:"sample_count"`
	P50Ns       uint64        `json:"p50_ns"`
	P90Ns       uint64        `json:"p90_ns"`
	P99Ns       uint64        `json:"p99_ns"`
	MinNs       uint64        `json:"min_ns"`
	MaxNs       uint64        `json:"max_ns"`
	MeanNs      uint64        `json:"mean_ns"`
}

// LatencyStats holds aggregate histogram stats.
type LatencyStats struct {
	TotalSamples map[string]uint64 `json:"total_samples"`
	Windows      []time.Duration   `json:"windows"`
}

func percentile(sorted []uint64, p float64) uint64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(p*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func mean(values []uint64) uint64 {
	if len(values) == 0 {
		return 0
	}
	var sum uint64
	for _, v := range values {
		sum += v
	}
	return sum / uint64(len(values))
}
