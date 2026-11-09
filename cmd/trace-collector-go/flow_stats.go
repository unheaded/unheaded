// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

// flow_stats.go — flow_tracker's FLOW_STATS counters, published on
// traces.stats once per --stats-interval.
//
// Until 2026-11-09 the stats went out only from the trace reader's poll
// path, so with the PACKET_EVENTS ring buffer (the normal case) traces.stats
// was never published. They run on their own timer now, whatever the trace
// reader does.
package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"time"

	"github.com/rs/zerolog/log"
)

// FlowStats holds flow_tracker's FLOW_STATS counters under the names
// ebpf/flow-tracker/src/main.rs gives their keys (STAT_*; key 9 is unused).
// Until 2026-11-08 they went out as packets_total, bytes_total,
// traces_created, traces_updated and errors, packet_marker-style names that
// matched neither program's keys.
type FlowStats struct {
	PacketsIngress   uint64 `json:"packets_ingress"`
	PacketsEgress    uint64 `json:"packets_egress"`
	FlowsNew         uint64 `json:"flows_new"`
	FlowsClosed      uint64 `json:"flows_closed"`
	TCPSyns          uint64 `json:"tcp_syns"`
	TCPFins          uint64 `json:"tcp_fins"`
	TCPRsts          uint64 `json:"tcp_rsts"`
	EventsSent       uint64 `json:"events_sent"`
	EventsDropped    uint64 `json:"events_dropped"`
	Migrations       uint64 `json:"migrations"`
	MigrationExpired uint64 `json:"migration_expired"`
	Cancelled        uint64 `json:"cancelled"`
}

// collectFlowStats reads every FLOW_STATS entry into a FlowStats. Short
// entries and keys flow_tracker does not define are skipped.
func collectFlowStats(statsMap MapReader) (FlowStats, error) {
	var stats FlowStats
	err := statsMap.Iterate(func(key, value []byte) error {
		if len(value) < 8 || len(key) < 4 {
			return nil
		}
		val := binary.LittleEndian.Uint64(value[:8])
		switch binary.LittleEndian.Uint32(key[:4]) {
		case 0:
			stats.PacketsIngress = val
		case 1:
			stats.PacketsEgress = val
		case 2:
			stats.FlowsNew = val
		case 3:
			stats.FlowsClosed = val
		case 4:
			stats.TCPSyns = val
		case 5:
			stats.TCPFins = val
		case 6:
			stats.TCPRsts = val
		case 7:
			stats.EventsSent = val
		case 8:
			stats.EventsDropped = val
		case 10:
			stats.Migrations = val
		case 11:
			stats.MigrationExpired = val
		case 12:
			stats.Cancelled = val
		}
		return nil
	})
	return stats, err
}

// statsSource has flow_tracker's FLOW_STATS map; every BPFLoader is one.
type statsSource interface {
	GetStatsMap() MapReader
}

// statsSink takes one FLOW_STATS snapshot as JSON; TracePublisher is one.
type statsSink interface {
	PublishStats(payload json.RawMessage)
}

// runFlowStats publishes a FLOW_STATS snapshot to sink on every tick until
// ctx ends. The caller owns the ticker (time.NewTicker(*statsInterval).C in
// the collector), so a test drives the ticks itself. A tick with no map or
// an unreadable one publishes nothing; the failure is counted, and logged
// when it starts and when it ends.
func runFlowStats(ctx context.Context, src statsSource, sink statsSink, tick <-chan time.Time) {
	failing := ""
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick:
			op, err := publishFlowStats(src, sink)
			switch {
			case op != "":
				readerErrors.WithLabelValues(op).Inc()
				if failing != op {
					log.Warn().Err(err).Str("operation", op).Msg("traces.stats: FLOW_STATS not published")
				}
				failing = op
			case failing != "":
				log.Info().Msg("traces.stats: FLOW_STATS published again")
				failing = ""
			}
		}
	}
}

// publishFlowStats reads FLOW_STATS once and hands it to sink. On failure it
// names the step that failed (a readerErrors operation label).
func publishFlowStats(src statsSource, sink statsSink) (string, error) {
	statsMap := src.GetStatsMap()
	if statsMap == nil {
		return "stats_unavailable", errNoFlowStatsMap
	}
	stats, err := collectFlowStats(statsMap)
	if err != nil {
		return "stats_iterate", err
	}
	payload, err := json.Marshal(stats)
	if err != nil {
		return "stats_marshal", err
	}
	sink.PublishStats(payload)
	return "", nil
}

// errNoFlowStatsMap: flow_tracker is not loaded and attached.
var errNoFlowStatsMap = errors.New("flow_tracker's FLOW_STATS map is not available")
