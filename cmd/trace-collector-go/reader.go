// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// reader.go — packet_marker's PACKET_EVENTS ring buffer reader.
//
// TraceReader decodes each PacketEvent from the PACKET_EVENTS ring buffer
// into a TraceEntry and hands it on through Events(). There is no poll
// mode: it used to iterate packet_marker's STATS map, whose 8-byte counters
// are not traces, so every poll counted decode errors and published nothing
// (removed 2026-11-09). Without the ring buffer the collector does not start;
// see openPacketEvents.
package main

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/rs/zerolog/log"

	"unheaded/pkg/metrics/auto"
	"unheaded/pkg/metrics/prom"
)

// ── Prometheus metrics for reader ───────────────────────────────────────────

var (
	readerEntriesRead = auto.NewCounter(prom.CounterOpts{
		Name: "trace_reader_entries_read_total",
		Help: "Total trace entries read from the PACKET_EVENTS ring buffer",
	})

	readerErrors = auto.NewCounterVec(prom.CounterOpts{
		Name: "trace_reader_errors_total",
		Help: "Total errors decoding ring buffer events or reading FLOW_STATS",
	}, []string{"operation"})
)

// ── TraceReader ─────────────────────────────────────────────────────────────

// TraceReaderConfig holds configuration for the trace reader.
type TraceReaderConfig struct {
	// BatchSize sizes the Events() channel (four batches deep).
	BatchSize int
}

// DefaultTraceReaderConfig returns sensible defaults.
func DefaultTraceReaderConfig() TraceReaderConfig {
	return TraceReaderConfig{
		BatchSize: 256,
	}
}

// TraceReaderStats holds runtime counters for the trace reader.
type TraceReaderStats struct {
	EntriesRead  uint64 `json:"entries_read"`
	DecodeErrors uint64 `json:"decode_errors"`
}

// TraceReader decodes PACKET_EVENTS records into trace entries.
type TraceReader struct {
	config  TraceReaderConfig
	stats   TraceReaderStats
	eventCh chan *TraceEntry
}

// NewTraceReader creates a trace reader with the given config.
func NewTraceReader(config TraceReaderConfig) *TraceReader {
	return &TraceReader{
		config:  config,
		eventCh: make(chan *TraceEntry, config.BatchSize*4),
	}
}

// openPacketEvents opens packet_marker's PACKET_EVENTS ring buffer. With no
// ring buffer it fails, naming the way to run without packet traces: there
// is no poll fallback.
func openPacketEvents(ctx context.Context, loader BPFLoader) (<-chan []byte, error) {
	ch, err := loader.GetPacketEventsCh(ctx)
	if err == nil && ch == nil {
		err = fmt.Errorf("loader returned no channel")
	}
	if err != nil {
		return nil, fmt.Errorf("packet_marker: PACKET_EVENTS ring buffer unavailable (there is no poll fallback; "+
			"--enable-packet-marker=false runs without packet traces): %w", err)
	}
	return ch, nil
}

// Run decodes events from ch, the PACKET_EVENTS ring buffer, until ctx ends
// or ch closes.
func (r *TraceReader) Run(ctx context.Context, ch <-chan []byte) {
	log.Info().
		Int("batch_size", r.config.BatchSize).
		Msg("trace reader started (PACKET_EVENTS ring buffer)")
	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("trace reader shutting down")
			return
		case raw, ok := <-ch:
			if !ok {
				log.Warn().Msg("ringbuf channel closed, trace reader stopping")
				return
			}
			// PACKET_EVENTS carries packet_marker's 48-byte PacketEvent, not
			// the 68-byte TraceEntry this used to decode (every record failed).
			entry, err := DecodeKernelPacketEvent(raw)
			if err != nil {
				atomic.AddUint64(&r.stats.DecodeErrors, 1)
				readerErrors.WithLabelValues("decode").Inc()
				continue
			}
			atomic.AddUint64(&r.stats.EntriesRead, 1)
			readerEntriesRead.Inc()

			// Emit to event channel (non-blocking)
			select {
			case r.eventCh <- entry:
			default:
			}
		}
	}
}

// Events returns a channel of decoded trace entries for downstream processing.
func (r *TraceReader) Events() <-chan *TraceEntry {
	return r.eventCh
}

// Stats returns a snapshot of reader statistics.
func (r *TraceReader) Stats() TraceReaderStats {
	return TraceReaderStats{
		EntriesRead:  atomic.LoadUint64(&r.stats.EntriesRead),
		DecodeErrors: atomic.LoadUint64(&r.stats.DecodeErrors),
	}
}
