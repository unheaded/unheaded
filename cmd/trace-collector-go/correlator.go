// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// correlator.go — Cross-correlation engine for packet trace events.
//
// PacketCorrelator matches trace entries by 5-tuple to build bidirectional
// flow records. It detects request/response pairs, measures round-trip
// latency, and identifies anomalous flows.
//
// The correlation algorithm:
// 1. Incoming trace entries are indexed by FiveTuple.
// 2. For each entry, check if a reverse-direction flow exists.
// 3. If matched, compute RTT from timestamps and emit a CorrelatedFlow.
// 4. Stale flows are garbage-collected after a configurable TTL.
package main

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// ── Prometheus metrics for correlator ───────────────────────────────────────

var (
	correlatorFlowsActive = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "trace_correlator_flows_active",
		Help: "Number of in-flight flows being tracked",
	})

	correlatorFlowsMatched = promauto.NewCounter(prometheus.CounterOpts{
		Name: "trace_correlator_flows_matched_total",
		Help: "Number of bidirectional flows successfully correlated",
	})

	correlatorFlowsExpired = promauto.NewCounter(prometheus.CounterOpts{
		Name: "trace_correlator_flows_expired_total",
		Help: "Number of flows expired by GC",
	})

	correlatorRTTHistogram = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "trace_correlator_rtt_seconds",
		Help:    "Observed round-trip times for correlated flows",
		Buckets: []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0},
	})
)

// ── Flow tracking types ─────────────────────────────────────────────────────

// FlowDirection indicates which direction of a bidirectional flow an entry belongs to.
type FlowDirection uint8

const (
	FlowForward FlowDirection = iota // Client -> Server (initial direction)
	FlowReverse                      // Server -> Client (response direction)
)

// TrackedFlow represents a unidirectional flow being tracked.
type TrackedFlow struct {
	FiveTuple   FiveTuple     `json:"five_tuple"`
	FirstSeen   uint64        `json:"first_seen_ns"`
	LastSeen    uint64        `json:"last_seen_ns"`
	PacketCount uint64        `json:"packet_count"`
	ByteCount   uint64        `json:"byte_count"`
	TraceID     [16]byte      `json:"trace_id"`
	Direction   FlowDirection `json:"direction"`
	LastEntry   *TraceEntry   `json:"-"` // Most recent entry (not serialized)
}

// CorrelatedFlow represents a matched bidirectional flow with RTT measurement.
type CorrelatedFlow struct {
	Forward   *TrackedFlow `json:"forward"`
	Reverse   *TrackedFlow `json:"reverse"`
	RTTNS     uint64       `json:"rtt_ns"`  // Round-trip time in nanoseconds
	Matched   bool         `json:"matched"` // True if bidirectional match found
	MatchedAt uint64       `json:"matched_at_ns"`
}

// RTTDuration returns the RTT as a time.Duration.
func (cf *CorrelatedFlow) RTTDuration() time.Duration {
	return time.Duration(cf.RTTNS) * time.Nanosecond
}

// ── PacketCorrelator ────────────────────────────────────────────────────────

// PacketCorrelatorConfig controls the correlator behavior.
type PacketCorrelatorConfig struct {
	// FlowTTL is how long to keep unmatched flows before GC.
	FlowTTL time.Duration

	// GCInterval is how often to run garbage collection.
	GCInterval time.Duration

	// OutputBufferSize is the capacity of the correlated flow output channel.
	OutputBufferSize int
}

// DefaultPacketCorrelatorConfig returns sensible defaults.
func DefaultPacketCorrelatorConfig() PacketCorrelatorConfig {
	return PacketCorrelatorConfig{
		FlowTTL:          30 * time.Second,
		GCInterval:       5 * time.Second,
		OutputBufferSize: 4096,
	}
}

// PacketCorrelatorStats holds runtime counters.
type PacketCorrelatorStats struct {
	EntriesProcessed uint64 `json:"entries_processed"`
	FlowsCreated     uint64 `json:"flows_created"`
	FlowsMatched     uint64 `json:"flows_matched"`
	FlowsExpired     uint64 `json:"flows_expired"`
	GCCycles         uint64 `json:"gc_cycles"`
}

// PacketCorrelator matches trace entries by 5-tuple into bidirectional flows.
type PacketCorrelator struct {
	config PacketCorrelatorConfig
	stats  PacketCorrelatorStats

	mu    sync.Mutex
	flows map[FiveTuple]*TrackedFlow
	out   chan *CorrelatedFlow
}

// NewPacketCorrelator creates a new correlator.
func NewPacketCorrelator(config PacketCorrelatorConfig) *PacketCorrelator {
	return &PacketCorrelator{
		config: config,
		flows:  make(map[FiveTuple]*TrackedFlow),
		out:    make(chan *CorrelatedFlow, config.OutputBufferSize),
	}
}

// Add processes a trace entry and attempts cross-correlation.
func (pc *PacketCorrelator) Add(entry *TraceEntry) {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	atomic.AddUint64(&pc.stats.EntriesProcessed, 1)

	ft := entry.FiveTuple()
	reverseFT := ft.Reverse()

	// Check for existing flow in forward direction
	flow, exists := pc.flows[ft]
	if exists {
		// Update existing flow
		flow.LastSeen = entry.TimestampNS
		flow.PacketCount++
		flow.ByteCount += uint64(entry.PacketLen)
		flow.LastEntry = entry
	} else {
		// Create new flow
		flow = &TrackedFlow{
			FiveTuple:   ft,
			FirstSeen:   entry.TimestampNS,
			LastSeen:    entry.TimestampNS,
			PacketCount: 1,
			ByteCount:   uint64(entry.PacketLen),
			TraceID:     entry.TraceID,
			Direction:   FlowForward,
			LastEntry:   entry,
		}
		pc.flows[ft] = flow
		atomic.AddUint64(&pc.stats.FlowsCreated, 1)
		correlatorFlowsActive.Set(float64(len(pc.flows)))
	}

	// Check for reverse direction flow (bidirectional match)
	reverseFlow, reverseExists := pc.flows[reverseFT]
	if reverseExists {
		// Compute RTT: time between first packet in forward direction
		// and first packet in reverse direction.
		var rttNS uint64
		if entry.TimestampNS > reverseFlow.FirstSeen {
			rttNS = entry.TimestampNS - reverseFlow.FirstSeen
		} else if reverseFlow.LastSeen > flow.FirstSeen {
			rttNS = reverseFlow.LastSeen - flow.FirstSeen
		}

		correlated := &CorrelatedFlow{
			Forward:   flow,
			Reverse:   reverseFlow,
			RTTNS:     rttNS,
			Matched:   true,
			MatchedAt: entry.TimestampNS,
		}

		// Emit correlated flow (non-blocking)
		select {
		case pc.out <- correlated:
			atomic.AddUint64(&pc.stats.FlowsMatched, 1)
			correlatorFlowsMatched.Inc()
			if rttNS > 0 {
				correlatorRTTHistogram.Observe(float64(rttNS) / 1e9)
			}
		default:
			// Output channel full; drop
		}
	}
}

// CorrelatedFlows returns the channel of matched bidirectional flows.
func (pc *PacketCorrelator) CorrelatedFlows() <-chan *CorrelatedFlow {
	return pc.out
}

// GC removes flows that have not been updated within the TTL.
// Returns the number of flows removed.
func (pc *PacketCorrelator) GC(nowNS uint64) int {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	atomic.AddUint64(&pc.stats.GCCycles, 1)

	ttlNS := uint64(pc.config.FlowTTL.Nanoseconds())
	removed := 0

	for ft, flow := range pc.flows {
		if nowNS-flow.LastSeen > ttlNS {
			delete(pc.flows, ft)
			removed++
		}
	}

	if removed > 0 {
		atomic.AddUint64(&pc.stats.FlowsExpired, uint64(removed))
		correlatorFlowsExpired.Add(float64(removed))
		correlatorFlowsActive.Set(float64(len(pc.flows)))
	}

	return removed
}

// Len returns the number of currently tracked flows.
func (pc *PacketCorrelator) Len() int {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	return len(pc.flows)
}

// Stats returns a snapshot of correlator statistics.
func (pc *PacketCorrelator) Stats() PacketCorrelatorStats {
	return PacketCorrelatorStats{
		EntriesProcessed: atomic.LoadUint64(&pc.stats.EntriesProcessed),
		FlowsCreated:     atomic.LoadUint64(&pc.stats.FlowsCreated),
		FlowsMatched:     atomic.LoadUint64(&pc.stats.FlowsMatched),
		FlowsExpired:     atomic.LoadUint64(&pc.stats.FlowsExpired),
		GCCycles:         atomic.LoadUint64(&pc.stats.GCCycles),
	}
}

// RunGC starts a background goroutine that periodically garbage-collects
// stale flows. Blocks until ctx is cancelled.
func (pc *PacketCorrelator) RunGC(ctx context.Context) {
	ticker := time.NewTicker(pc.config.GCInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			nowNS := uint64(time.Now().UnixNano())
			removed := pc.GC(nowNS)
			if removed > 0 {
				// Logged at debug level to avoid noise
				_ = removed
			}
		}
	}
}
