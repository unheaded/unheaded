// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// publisher.go — Wotan topic router for trace events.
//
// TracePublisher routes events from different BPF programs to the
// appropriate Wotan topics. It batches events for efficient publishing
// and supports configurable flush intervals.
//
// Topic routing:
//   traces.packet     — from packet_marker (XDP)
//   traces.flow       — from flow_tracker (TC)
//   traces.latency    — from latency_probe (tracepoint)
//   traces.correlated — cross-correlated events from PacketCorrelator
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

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/rs/zerolog/log"

	"unheaded/pkg/ebpf"
	wotanClient "unheaded/pkg/wotan-client"
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

	// Mirror topics for dashboard ingestor compatibility.
	// The dashboard eBPF ingestor subscribes to ebpf.*.events topics.
	TopicEBPFPacket  = "ebpf.packet.events"
	TopicEBPFFlow    = "ebpf.flow.events"
	TopicEBPFLatency = "ebpf.latency.events"
)

// ── Prometheus metrics for publisher ──────────────────────────────────────

var (
	publisherBatchesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "trace_publisher_batches_total",
		Help: "Total batches published to Wotan",
	}, []string{"topic"})

	publisherEventsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "trace_publisher_events_total",
		Help: "Total events published to Wotan",
	}, []string{"topic"})

	publisherErrorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "trace_publisher_errors_total",
		Help: "Total publish errors",
	}, []string{"topic"})

	publisherFlushLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
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

	// gRPC client for Wotan topic publishing
	tsc *wotanClient.TopicStreamClient
}

// NewTracePublisher creates a new publisher with the given configuration.
func NewTracePublisher(config TracePublisherConfig) *TracePublisher {
	return &TracePublisher{
		config:  config,
		batches: make(map[string][]json.RawMessage),
	}
}

// Connect establishes the gRPC connection to Wotan and subscribes to
// all trace topics. Must be called before Run().
func (tp *TracePublisher) Connect(ctx context.Context) error {
	// Create HTTP fallback client
	var opts []wotanClient.TopicStreamOption
	if tp.config.WotanHTTPAddr != "" {
		httpClient, err := wotanClient.NewClient(tp.config.WotanHTTPAddr)
		if err == nil {
			opts = append(opts, wotanClient.WithHTTPFallback(httpClient))
		}
	}

	tsc, err := wotanClient.NewTopicStreamClient(tp.config.WotanAddr, opts...)
	if err != nil {
		return fmt.Errorf("connect to Wotan gRPC at %s: %w", tp.config.WotanAddr, err)
	}
	tp.tsc = tsc

	// Subscribe to all trace topics + mirror topics for dashboard ingestor
	topics := []string{
		TopicTracesPacket, TopicTracesFlow,
		TopicTracesLatency, TopicTracesCorrelated,
		TopicEBPFPacket, TopicEBPFFlow, TopicEBPFLatency,
	}
	for _, topic := range topics {
		if _, err := tsc.Subscribe(ctx, topic, "trace-collector-go"); err != nil {
			tsc.Close()
			return fmt.Errorf("subscribe to %s: %w", topic, err)
		}
	}

	tp.connected.Store(true)
	log.Info().
		Str("grpc_addr", tp.config.WotanAddr).
		Str("http_addr", tp.config.WotanHTTPAddr).
		Int("topics", len(topics)).
		Msg("trace publisher connected to Wotan (gRPC-first)")
	return nil
}

// PublishPacketEvent queues a packet marker event for publishing.
func (tp *TracePublisher) PublishPacketEvent(entry *TraceEntry) error {
	payload, err := marshalTraceEntry(entry)
	if err != nil {
		return fmt.Errorf("marshal packet event: %w", err)
	}
	tp.enqueue(TopicTracesPacket, payload)
	tp.enqueue(TopicEBPFPacket, payload)
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
	tp.enqueue(TopicEBPFFlow, payload)
	atomic.AddUint64(&tp.stats.FlowEvents, 1)
}

// PublishLatencyEvent queues a latency probe event for publishing.
func (tp *TracePublisher) PublishLatencyEvent(payload json.RawMessage) {
	tp.enqueue(TopicTracesLatency, payload)
	tp.enqueue(TopicEBPFLatency, payload)
	atomic.AddUint64(&tp.stats.LatencyEvents, 1)
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
			log.Error().Err(err).
				Str("topic", topic).
				Int("count", len(messages)).
				Msg("failed to publish batch")
		} else {
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
			// Close gRPC connection
			if tp.tsc != nil {
				tp.tsc.Close()
			}
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
	if tp.tsc == nil {
		log.Debug().Str("topic", topic).Msg("Wotan not connected, dropping batch")
		return nil
	}

	if err := tp.tsc.Publish(ctx, topic, payload); err != nil {
		tp.connected.Store(false)
		return fmt.Errorf("publish to %s: %w", topic, err)
	}

	tp.connected.Store(true)
	return nil
}

// marshalTraceEntry converts a TraceEntry to JSON.
func marshalTraceEntry(te *TraceEntry) (json.RawMessage, error) {
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

	return json.Marshal(jsonEntry{
		TraceID:     te.TraceIDHex(),
		TimestampNS: te.TimestampNS,
		SrcIP:       te.SrcIPAddr().String(),
		DstIP:       te.DstIPAddr().String(),
		SrcPort:     te.SrcPort,
		DstPort:     te.DstPort,
		Protocol:    te.ProtocolName(),
		Flags:       te.Flags,
		PacketLen:   te.PacketLen,
		HopCount:    te.HopCount,
	})
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
