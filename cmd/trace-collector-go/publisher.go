// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// publisher.go — Wotan topic router for trace events.
//
// TracePublisher routes events from different BPF programs to the
// appropriate Wotan topics. It batches events for efficient publishing
// and supports configurable flush intervals.
//
// Topic routing:
//
//	traces.packet     — from packet_marker (XDP)
//	traces.flow       — from flow_tracker (TC)
//	traces.latency    — from latency_probe (tracepoint)
//	traces.correlated — cross-correlated events from PacketCorrelator
//	traces.stats      — flow_tracker's FLOW_STATS counters, on a timer
//
// Per RFC 9669: we use "BPF" (not "eBPF") throughout.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"

	"unheaded/pkg/ebpf"
	"unheaded/pkg/metrics/auto"
	"unheaded/pkg/metrics/prom"
)

// ── Wotan topic constants ─────────────────────────────────────────────────

const (
	// TopicTracesPacket receives packet marker events (XDP).
	TopicTracesPacket = "traces.packet"

	// TopicTracesFlow receives flow tracker events (TC).
	TopicTracesFlow = "traces.flow"

	// TopicTracesLatency receives latency probe events (tracepoint).
	TopicTracesLatency = "traces.latency"

	// TopicTracesCorrelated receives cross-correlated events.
	TopicTracesCorrelated = "traces.correlated"

	// TopicTracesStats receives flow_tracker's FLOW_STATS counters, one
	// snapshot per --stats-interval.
	TopicTracesStats = "traces.stats"

	// The ebpf.{packet,flow,latency}.events mirrors are gone (ADR-147
	// Phase 10): each event is published once, on traces.*, in the shape
	// the dashboard's ingestor parses; its trace view reads the same topic.
)

// ── Prometheus metrics for publisher ──────────────────────────────────────

var (
	publisherBatchesTotal = auto.NewCounterVec(prom.CounterOpts{
		Name: "trace_publisher_batches_total",
		Help: "Total batches published to Wotan",
	}, []string{"topic"})

	publisherEventsTotal = auto.NewCounterVec(prom.CounterOpts{
		Name: "trace_publisher_events_total",
		Help: "Total events published to Wotan",
	}, []string{"topic"})

	publisherErrorsTotal = auto.NewCounterVec(prom.CounterOpts{
		Name: "trace_publisher_errors_total",
		Help: "Total publish errors",
	}, []string{"topic"})

	publisherFlushLatency = auto.NewHistogramVec(prom.HistogramOpts{
		Name:    "trace_publisher_flush_duration_seconds",
		Help:    "Duration of each batch flush to Wotan",
		Buckets: []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1},
	}, []string{"topic"})
)

// ── TracePublisherConfig ──────────────────────────────────────────────────

// TracePublisherConfig holds configuration for the trace publisher.
type TracePublisherConfig struct {
	// WotanAddr is the Wotan gRPC address (host:port).
	WotanAddr string

	// WotanHTTPAddr is the Wotan HTTP address for fallback (host:port).
	WotanHTTPAddr string

	// BatchSize is the maximum number of events per flush.
	BatchSize int

	// FlushInterval is how often to flush pending batches.
	FlushInterval time.Duration
}

// DefaultTracePublisherConfig returns sensible defaults.
func DefaultTracePublisherConfig() TracePublisherConfig {
	return TracePublisherConfig{
		WotanAddr:     "localhost:18001",
		WotanHTTPAddr: "localhost:18000",
		BatchSize:     100,
		FlushInterval: 50 * time.Millisecond,
	}
}

// ── TracePublisherStats ───────────────────────────────────────────────────

// TracePublisherStats holds runtime counters for the publisher.
type TracePublisherStats struct {
	BatchesSent    uint64 `json:"batches_sent"`
	EventsSent     uint64 `json:"events_sent"`
	Errors         uint64 `json:"errors"`
	FlushCycles    uint64 `json:"flush_cycles"`
	PacketEvents   uint64 `json:"packet_events"`
	FlowEvents     uint64 `json:"flow_events"`
	LatencyEvents  uint64 `json:"latency_events"`
	CorrelatedSent uint64 `json:"correlated_sent"`
}

// ── TracePublisher ────────────────────────────────────────────────────────

// TracePublisher routes trace events to the correct Wotan topics with
// batching and configurable flush intervals. Uses gRPC-first transport
// with HTTP fallback via TopicStreamClient.
type TracePublisher struct {
	config    TracePublisherConfig
	stats     TracePublisherStats
	connected atomic.Bool

	mu      sync.Mutex
	batches map[string][]json.RawMessage

	// failures counts each topic's consecutive failed batches. Only the
	// first of an outage, every failLogEvery-th, and the recovery are
	// logged: while Wotan is down every batch fails, and logging each one
	// wrote 4.3 GB in ten hours (2026-10-18). Metrics still count them all.
	failMu   sync.Mutex
	failures map[string]uint64

	// wc is the process's one Wotan client (ADR-147 Phase 7): gRPC while
	// healthy, HTTP otherwise.
	wc topicPublisher
}

// failLogEvery: during a long outage, one reminder per this many failed
// batches on a topic (about every 40 s at the observed ~25 batches/s).
const failLogEvery = 1000

// NewTracePublisher creates a new publisher with the given configuration.
func NewTracePublisher(config TracePublisherConfig) *TracePublisher {
	return &TracePublisher{
		config:   config,
		batches:  make(map[string][]json.RawMessage),
		failures: make(map[string]uint64),
	}
}

// Connect gives the publisher the process's one Wotan client. Topics are
// joined on first publish. Must be called before Run(); a nil client is an
// error and batches are dropped until one is given.
func (tp *TracePublisher) Connect(wc topicPublisher) error {
	if wc == nil || isNilClient(wc) {
		return fmt.Errorf("no Wotan client")
	}
	tp.wc = wc
	tp.connected.Store(true)
	log.Info().
		Str("grpc_addr", tp.config.WotanAddr).
		Str("http_addr", tp.config.WotanHTTPAddr).
		Msg("trace publisher on the process's Wotan client (gRPC-first)")
	return nil
}

// PublishPacketEvent queues a packet marker event on traces.packet, once,
// in the dashboard's PacketEvent shape (trace_id {high, low}, flow_key).
// The flat shape it used to carry beside it (trace_id hex, src_ip/dst_ip,
// flags, hop_count) went with the ebpf.packet.events mirror: a kernel
// packet record sets neither flags nor hop_count.
func (tp *TracePublisher) PublishPacketEvent(entry *TraceEntry) error {
	payload, err := marshalDashboardPacket(entry)
	if err != nil {
		return fmt.Errorf("marshal packet event: %w", err)
	}
	tp.enqueue(TopicTracesPacket, payload)
	atomic.AddUint64(&tp.stats.PacketEvents, 1)
	return nil
}

// PublishAnamnesisEvent queues an Anamnesis event for publishing to the
// correct topic based on event type.
func (tp *TracePublisher) PublishAnamnesisEvent(ev *ebpf.AnamnesisEvent) error {
	payload, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal anamnesis event: %w", err)
	}

	topic := anamnesisTopicForEvent(ev.EventType)
	tp.enqueue(topic, payload)
	return nil
}

// PublishFlowEvent queues a flow tracker event for publishing.
func (tp *TracePublisher) PublishFlowEvent(payload json.RawMessage) {
	tp.enqueue(TopicTracesFlow, payload)
	atomic.AddUint64(&tp.stats.FlowEvents, 1)
}

// PublishLatencyEvent queues a latency probe event for publishing.
func (tp *TracePublisher) PublishLatencyEvent(payload json.RawMessage) {
	tp.enqueue(TopicTracesLatency, payload)
	atomic.AddUint64(&tp.stats.LatencyEvents, 1)
}

// PublishStats queues a FLOW_STATS snapshot on traces.stats.
func (tp *TracePublisher) PublishStats(payload json.RawMessage) {
	tp.enqueue(TopicTracesStats, payload)
}

// PublishCorrelatedFlow queues a correlated flow for publishing.
func (tp *TracePublisher) PublishCorrelatedFlow(flow *CorrelatedFlow) error {
	payload, err := json.Marshal(flow)
	if err != nil {
		return fmt.Errorf("marshal correlated flow: %w", err)
	}
	tp.enqueue(TopicTracesCorrelated, payload)
	atomic.AddUint64(&tp.stats.CorrelatedSent, 1)
	return nil
}

// Flush sends all pending batches to Wotan.
func (tp *TracePublisher) Flush(ctx context.Context) {
	tp.mu.Lock()
	batches := tp.batches
	tp.batches = make(map[string][]json.RawMessage)
	tp.mu.Unlock()

	atomic.AddUint64(&tp.stats.FlushCycles, 1)

	for topic, messages := range batches {
		if len(messages) == 0 {
			continue
		}
		if err := tp.sendBatch(ctx, topic, messages); err != nil {
			atomic.AddUint64(&tp.stats.Errors, 1)
			publisherErrorsTotal.WithLabelValues(topic).Inc()
			if n := tp.noteFailure(topic); n == 1 || n%failLogEvery == 0 {
				log.Error().Err(err).
					Str("topic", topic).
					Int("count", len(messages)).
					Uint64("failed_batches", n).
					Msg("failed to publish batch (logged once per outage, then every 1000)")
			}
		} else {
			if n := tp.noteSuccess(topic); n > 0 {
				log.Info().Str("topic", topic).Uint64("failed_batches", n).Msg("publishing recovered")
			}
			atomic.AddUint64(&tp.stats.BatchesSent, 1)
			atomic.AddUint64(&tp.stats.EventsSent, uint64(len(messages)))
			publisherBatchesTotal.WithLabelValues(topic).Inc()
			publisherEventsTotal.WithLabelValues(topic).Add(float64(len(messages)))
		}
	}
}

// Run starts the periodic flush loop. Blocks until ctx is cancelled.
func (tp *TracePublisher) Run(ctx context.Context) {
	ticker := time.NewTicker(tp.config.FlushInterval)
	defer ticker.Stop()

	log.Info().
		Str("wotan_addr", tp.config.WotanAddr).
		Int("batch_size", tp.config.BatchSize).
		Dur("flush_interval", tp.config.FlushInterval).
		Msg("trace publisher started")

	for {
		select {
		case <-ctx.Done():
			// Final flush on shutdown
			tp.Flush(context.Background())
			log.Info().Msg("trace publisher stopped")
			return
		case <-ticker.C:
			tp.Flush(ctx)
		}
	}
}

// IsConnected returns true if the last publish succeeded.
func (tp *TracePublisher) IsConnected() bool {
	return tp.connected.Load()
}

// Stats returns a snapshot of publisher statistics.
func (tp *TracePublisher) Stats() TracePublisherStats {
	return TracePublisherStats{
		BatchesSent:    atomic.LoadUint64(&tp.stats.BatchesSent),
		EventsSent:     atomic.LoadUint64(&tp.stats.EventsSent),
		Errors:         atomic.LoadUint64(&tp.stats.Errors),
		FlushCycles:    atomic.LoadUint64(&tp.stats.FlushCycles),
		PacketEvents:   atomic.LoadUint64(&tp.stats.PacketEvents),
		FlowEvents:     atomic.LoadUint64(&tp.stats.FlowEvents),
		LatencyEvents:  atomic.LoadUint64(&tp.stats.LatencyEvents),
		CorrelatedSent: atomic.LoadUint64(&tp.stats.CorrelatedSent),
	}
}

// ── Internal methods ──────────────────────────────────────────────────────

// noteFailure counts a failed batch on topic and returns the run so far.
func (tp *TracePublisher) noteFailure(topic string) uint64 {
	tp.failMu.Lock()
	defer tp.failMu.Unlock()
	tp.failures[topic]++
	return tp.failures[topic]
}

// noteSuccess ends topic's failure run and returns how long it was.
func (tp *TracePublisher) noteSuccess(topic string) uint64 {
	tp.failMu.Lock()
	defer tp.failMu.Unlock()
	n := tp.failures[topic]
	delete(tp.failures, topic)
	return n
}

// enqueue adds a JSON payload to the given topic's batch. If the batch
// reaches BatchSize, it is flushed immediately.
func (tp *TracePublisher) enqueue(topic string, payload json.RawMessage) {
	tp.mu.Lock()
	tp.batches[topic] = append(tp.batches[topic], payload)
	shouldFlush := len(tp.batches[topic]) >= tp.config.BatchSize
	tp.mu.Unlock()

	if shouldFlush {
		tp.Flush(context.Background())
	}
}

// sendBatch publishes a batch of JSON messages to a Wotan topic via gRPC.
// TopicStreamClient handles retry, circuit breaking, and HTTP fallback.
func (tp *TracePublisher) sendBatch(ctx context.Context, topic string, messages []json.RawMessage) error {
	start := time.Now()
	defer func() {
		publisherFlushLatency.WithLabelValues(topic).Observe(time.Since(start).Seconds())
	}()

	// Serialize batch as JSON array
	payload, err := json.Marshal(messages)
	if err != nil {
		return fmt.Errorf("marshal batch: %w", err)
	}

	// Use gRPC client if connected, fall back to no-op if not yet connected
	if tp.wc == nil {
		log.Debug().Str("topic", topic).Msg("Wotan not connected, dropping batch")
		return nil
	}

	if err := tp.wc.PublishPreferGRPC(ctx, topic, wotanName, payload); err != nil {
		tp.connected.Store(false)
		return fmt.Errorf("publish to %s: %w", topic, err)
	}

	tp.connected.Store(true)
	return nil
}

// anamnesisTopicForEvent maps an Anamnesis event type to its Wotan topic.
func anamnesisTopicForEvent(et ebpf.EventType) string {
	switch et {
	case ebpf.EventBirth:
		return TopicBirth
	case ebpf.EventHop:
		return TopicHop
	case ebpf.EventDeath:
		return TopicDeath
	case ebpf.EventAnomaly:
		return TopicAnomaly
	case ebpf.EventChaos:
		return TopicChaos
	default:
		return "anamnesis.unknown"
	}
}
