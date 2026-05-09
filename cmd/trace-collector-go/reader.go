// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// reader.go — Map reader that periodically polls BPF maps and publishes traces.
//
// TraceReader reads FLOW_STATE and STATS maps from the packet_marker BPF
// program, converts entries to JSON, and publishes them to Wotan topics.
// It also clears processed entries to prevent map exhaustion.
//
// The reader runs on a configurable interval (default 100ms) and batches
// events for efficient Wotan publishing.
package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/rs/zerolog/log"
)

// ── Prometheus metrics for reader ───────────────────────────────────────────

var (
	readerPollsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "trace_reader_polls_total",
		Help: "Total map poll cycles",
	})

	readerEntriesRead = promauto.NewCounter(prometheus.CounterOpts{
		Name: "trace_reader_entries_read_total",
		Help: "Total trace entries read from BPF maps",
	})

	readerEntriesPublished = promauto.NewCounter(prometheus.CounterOpts{
		Name: "trace_reader_entries_published_total",
		Help: "Total trace entries published to Wotan",
	})

	readerErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "trace_reader_errors_total",
		Help: "Total errors during map reading or publishing",
	}, []string{"operation"})

	readerPollLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "trace_reader_poll_duration_seconds",
		Help:    "Duration of each map poll cycle",
		Buckets: []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1},
	})
)

// ── TraceReader ─────────────────────────────────────────────────────────────

// TraceReaderConfig holds configuration for the trace reader.
type TraceReaderConfig struct {
	// ReadInterval is how often to poll BPF maps.
	ReadInterval time.Duration

	// BatchSize is the maximum number of events per Wotan publish call.
	BatchSize int

	// DeleteAfterRead removes entries from the BPF map after reading.
	// This prevents map exhaustion but means entries are consumed exactly once.
	DeleteAfterRead bool

	// TraceTopic is the Wotan topic for trace events.
	TraceTopic string

	// StatsTopic is the Wotan topic for stats events.
	StatsTopic string
}

// DefaultTraceReaderConfig returns sensible defaults.
func DefaultTraceReaderConfig() TraceReaderConfig {
	return TraceReaderConfig{
		ReadInterval:    100 * time.Millisecond,
		BatchSize:       256,
		DeleteAfterRead: true,
		TraceTopic:      "traces.packet",
		StatsTopic:      "traces.stats",
	}
}

// TraceReaderStats holds runtime counters for the trace reader.
type TraceReaderStats struct {
	PollCycles     uint64 `json:"poll_cycles"`
	EntriesRead    uint64 `json:"entries_read"`
	EntriesDeleted uint64 `json:"entries_deleted"`
	PublishCalls   uint64 `json:"publish_calls"`
	PublishErrors  uint64 `json:"publish_errors"`
	DecodeErrors   uint64 `json:"decode_errors"`
}

// TraceReader periodically reads BPF maps and publishes trace events.
type TraceReader struct {
	loader    BPFLoader
	publisher *WotanPublisher
	config    TraceReaderConfig
	stats     TraceReaderStats
	eventCh   chan *TraceEntry
}

// NewTraceReader creates a trace reader with the given loader, publisher, and config.
func NewTraceReader(loader BPFLoader, publisher *WotanPublisher, config TraceReaderConfig) *TraceReader {
	return &TraceReader{
		loader:    loader,
		publisher: publisher,
		config:    config,
		eventCh:   make(chan *TraceEntry, config.BatchSize*4),
	}
}

// Run starts the trace reading loop. If the PACKET_EVENTS ringbuf is
// available, events are received via mmap (push-based, zero-copy). Otherwise
// falls back to periodic map polling.
func (r *TraceReader) Run(ctx context.Context) {
	// Try ringbuf first — much lower latency than poll-based reading.
	ringbufCh := r.loader.GetPacketEventsCh(ctx)
	if ringbufCh != nil {
		log.Info().
			Int("batch_size", r.config.BatchSize).
			Str("trace_topic", r.config.TraceTopic).
			Msg("trace reader started (ringbuf mode)")
		r.runRingbuf(ctx, ringbufCh)
		return
	}

	// Fallback: poll STATS hash map on a timer.
	log.Info().
		Dur("interval", r.config.ReadInterval).
		Int("batch_size", r.config.BatchSize).
		Bool("delete_after_read", r.config.DeleteAfterRead).
		Str("trace_topic", r.config.TraceTopic).
		Msg("trace reader started (poll mode)")

	ticker := time.NewTicker(r.config.ReadInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("trace reader shutting down")
			return
		case <-ticker.C:
			r.pollOnce(ctx)
		}
	}
}

// runRingbuf consumes events from the PACKET_EVENTS ring buffer channel.
func (r *TraceReader) runRingbuf(ctx context.Context, ch <-chan []byte) {
	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("trace reader (ringbuf) shutting down")
			return
		case raw, ok := <-ch:
			if !ok {
				log.Warn().Msg("ringbuf channel closed, trace reader stopping")
				return
			}
			entry, err := DecodeTraceEntry(raw)
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
		PollCycles:     atomic.LoadUint64(&r.stats.PollCycles),
		EntriesRead:    atomic.LoadUint64(&r.stats.EntriesRead),
		EntriesDeleted: atomic.LoadUint64(&r.stats.EntriesDeleted),
		PublishCalls:   atomic.LoadUint64(&r.stats.PublishCalls),
		PublishErrors:  atomic.LoadUint64(&r.stats.PublishErrors),
		DecodeErrors:   atomic.LoadUint64(&r.stats.DecodeErrors),
	}
}

// pollOnce reads all current map entries, publishes them, and optionally deletes.
func (r *TraceReader) pollOnce(ctx context.Context) {
	start := time.Now()
	defer func() {
		readerPollLatency.Observe(time.Since(start).Seconds())
		readerPollsTotal.Inc()
		atomic.AddUint64(&r.stats.PollCycles, 1)
	}()

	traceMap := r.loader.GetTraceMap()
	if traceMap == nil {
		return
	}

	// Collect entries
	var entries []*TraceEntry
	var keysToDelete [][]byte

	err := traceMap.Iterate(func(key, value []byte) error {
		entry, err := DecodeTraceEntry(value)
		if err != nil {
			atomic.AddUint64(&r.stats.DecodeErrors, 1)
			readerErrors.WithLabelValues("decode").Inc()
			return nil // Continue iteration despite decode errors
		}

		entries = append(entries, entry)
		atomic.AddUint64(&r.stats.EntriesRead, 1)
		readerEntriesRead.Inc()

		// Queue key for deletion after publishing
		if r.config.DeleteAfterRead {
			k := make([]byte, len(key))
			copy(k, key)
			keysToDelete = append(keysToDelete, k)
		}

		// Emit to event channel (non-blocking)
		select {
		case r.eventCh <- entry:
		default:
			// Channel full, skip
		}

		return nil
	})

	if err != nil {
		readerErrors.WithLabelValues("iterate").Inc()
		log.Error().Err(err).Msg("error iterating trace map")
		return
	}

	if len(entries) == 0 {
		return
	}

	// Publish in batches
	for i := 0; i < len(entries); i += r.config.BatchSize {
		end := i + r.config.BatchSize
		if end > len(entries) {
			end = len(entries)
		}
		batch := entries[i:end]

		if err := r.publishTraceEntries(ctx, batch); err != nil {
			atomic.AddUint64(&r.stats.PublishErrors, 1)
			readerErrors.WithLabelValues("publish").Inc()
			log.Error().Err(err).Int("batch_size", len(batch)).Msg("failed to publish trace entries")
		} else {
			atomic.AddUint64(&r.stats.PublishCalls, 1)
			readerEntriesPublished.Add(float64(len(batch)))
		}
	}

	// Delete processed entries
	if r.config.DeleteAfterRead {
		for _, key := range keysToDelete {
			if err := traceMap.Delete(key); err != nil {
				readerErrors.WithLabelValues("delete").Inc()
			} else {
				atomic.AddUint64(&r.stats.EntriesDeleted, 1)
			}
		}
	}

	// Read and publish stats
	r.readStats(ctx)
}

// publishTraceEntries serializes a batch of trace entries and publishes to Wotan.
func (r *TraceReader) publishTraceEntries(ctx context.Context, entries []*TraceEntry) error {
	type jsonEntry struct {
		TraceID     string `json:"trace_id"`
		TimestampNS uint64 `json:"timestamp_ns"`
		SrcIP       string `json:"src_ip"`
		DstIP       string `json:"dst_ip"`
		SrcPort     uint16 `json:"src_port"`
		DstPort     uint16 `json:"dst_port"`
		Protocol    string `json:"protocol"`
		Flags       uint8  `json:"flags"`
		PacketLen   uint16 `json:"packet_len"`
		HopCount    uint8  `json:"hop_count"`
	}

	jsonEntries := make([]jsonEntry, 0, len(entries))
	for _, e := range entries {
		jsonEntries = append(jsonEntries, jsonEntry{
			TraceID:     e.TraceIDHex(),
			TimestampNS: e.TimestampNS,
			SrcIP:       e.SrcIPAddr().String(),
			DstIP:       e.DstIPAddr().String(),
			SrcPort:     e.SrcPort,
			DstPort:     e.DstPort,
			Protocol:    e.ProtocolName(),
			Flags:       e.Flags,
			PacketLen:   e.PacketLen,
			HopCount:    e.HopCount,
		})
	}

	payload, err := json.Marshal(jsonEntries)
	if err != nil {
		return fmt.Errorf("marshal trace entries: %w", err)
	}

	// Publish to Wotan via the legacy publisher (best-effort).
	// Publishing failures are logged but do not fail the reader — trace
	// collection must continue even when Wotan is temporarily unreachable.
	if r.publisher != nil {
		if pubErr := r.publisher.PublishRaw(ctx, r.config.TraceTopic, payload); pubErr != nil {
			log.Debug().Err(pubErr).Str("topic", r.config.TraceTopic).Msg("trace publish failed (best-effort)")
		}
	}
	return nil
}

// readStats reads the STATS BPF map and publishes aggregated stats.
// Stats map keys from packet_marker: 0=packets_total, 1=bytes_total,
// 2=traces_created, 3=traces_updated, 4=errors.
func (r *TraceReader) readStats(ctx context.Context) {
	statsMap := r.loader.GetStatsMap()
	if statsMap == nil {
		return
	}

	var stats StatsEntry
	err := statsMap.Iterate(func(key, value []byte) error {
		if len(value) < 8 || len(key) < 4 {
			return nil
		}
		keyIdx := binary.LittleEndian.Uint32(key[:4])
		val := binary.LittleEndian.Uint64(value[:8])
		switch keyIdx {
		case 0:
			stats.PacketsTotal = val
		case 1:
			stats.BytesTotal = val
		case 2:
			stats.TracesCreated = val
		case 3:
			stats.TracesUpdated = val
		case 4:
			stats.Errors = val
		}
		return nil
	})
	if err != nil {
		readerErrors.WithLabelValues("stats_iterate").Inc()
		return
	}

	// Publish aggregated stats to Wotan
	payload, err := json.Marshal(stats)
	if err != nil {
		readerErrors.WithLabelValues("stats_marshal").Inc()
		return
	}

	if r.publisher != nil {
		if pubErr := r.publisher.PublishRaw(ctx, r.config.StatsTopic, payload); pubErr != nil {
			readerErrors.WithLabelValues("stats_publish").Inc()
		}
	}
}
