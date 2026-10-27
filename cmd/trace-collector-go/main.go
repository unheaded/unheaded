// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

/*
 * This file is part of the Unheaded distributed system platform.
 *
 * Unheaded is licensed under the GNU General Public License v3.0 or later.
 * See the LICENSE file in the root directory for the full license text.
 *
 * For protocol specifications, see LICENSE-PROTOCOLS.
 * For GPL 2.0-licensed components (DOOM engine), see doom/LICENSE.
 */

// trace-collector-go — Unified BPF trace collector.
//
// Loads and manages three BPF programs:
//   - packet_marker (XDP) — injects trace IDs at XDP layer
//   - flow_tracker (TC) — tracks connection state via TC
//   - latency_probe (tracepoint) — measures RTT via kernel tracepoints
//
// Events from all three programs are read, cross-correlated, and published
// to Wotan topics for the dashboard and alerting pipeline.
//
// This is the Go rewrite of cmd/trace-collector (Rust), using the shared
// pkg/ebpf infrastructure for ring buffer reading and event decoding.
//
// Per RFC 9669: we use "BPF" (not "eBPF") throughout.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	ciliumebpf "github.com/cilium/ebpf"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"unheaded/pkg/auth"
	"unheaded/pkg/discovery"
	"unheaded/pkg/ebpf"
	"unheaded/pkg/healthreport"
	"unheaded/pkg/logagg"
	"unheaded/pkg/metrics/auto"
	"unheaded/pkg/metrics/prom"
	"unheaded/pkg/ports"
	"unheaded/pkg/transport"
	wotanClient "unheaded/pkg/wotan-client"
)

// ── CLI flags ───────────────────────────────────────────────────────────

var (
	// Existing Anamnesis-mode flags
	ringPath      = flag.String("ring-path", ebpf.DefaultAnamnesisRingPath, "BPF ring buffer pin path")
	wotanAddr     = flag.String("wotan-addr", "localhost:18001", "Wotan gRPC address")
	wotanHTTPAddr = flag.String("wotan-http-addr", "localhost:18000", "Wotan HTTP address (fallback)")
	httpAddr      = flag.String("http-addr", ports.DefaultAddr(ports.TraceCollectorHTTP), "HTTP address for health/metrics")
	maxRate       = flag.Int("max-rate", 10000, "Max events/sec (0 = unlimited)")
	batchSize     = flag.Int("batch-size", 100, "Publish batch size")
	batchTimeout  = flag.Duration("batch-timeout", 50*time.Millisecond, "Publish batch timeout")
	bufSize       = flag.Int("buf-size", 8192, "Event channel buffer size")
	logJSON       = flag.Bool("log-json", false, "Use JSON log format")

	// Unified BPF loader flags
	iface              = flag.String("interface", "lo", "Network interface to attach BPF programs")
	metricsAddr        = flag.String("metrics-addr", ports.DefaultAddr(ports.TraceCollectorMetrics), "Prometheus metrics address (unified mode)")
	mapPinPath         = flag.String("map-pin-path", "/sys/fs/bpf/unheaded/", "BPF map pin path")
	readInterval       = flag.Duration("read-interval", 100*time.Millisecond, "Map read interval")
	enablePacketMarker = flag.Bool("enable-packet-marker", true, "Enable packet_marker XDP program")
	enableFlowTracker  = flag.Bool("enable-flow-tracker", true, "Enable flow_tracker TC program")
	enableLatencyProbe = flag.Bool("enable-latency-probe", true, "Enable latency_probe tracepoint program")
	ebpfObjDir         = flag.String("ebpf-obj-dir", "", "Directory containing compiled BPF ELF objects (default: ebpf/target/bpfel-unknown-none/release)")
	unifiedMode        = flag.Bool("unified", false, "Run in unified mode (load all BPF programs)")
	allowUntrustedObjs = flag.Bool("allow-untrusted-ebpf-obj-dir", false,
		"Development only: as root, load BPF objects from paths a non-root user can write (e.g. a user-owned ebpf/target). Off, such objects are refused")
)

// ── Prometheus metrics ──────────────────────────────────────────────────

var (
	eventsTotal = auto.NewCounterVec(prom.CounterOpts{
		Name: "trace_collector_events_total",
		Help: "Total Anamnesis events processed",
	}, []string{"type"})

	eventsPublished = auto.NewCounterVec(prom.CounterOpts{
		Name: "trace_collector_events_published_total",
		Help: "Total events published to Wotan",
	}, []string{"topic"})

	eventsDropped = auto.NewCounter(prom.CounterOpts{
		Name: "trace_collector_events_dropped_total",
		Help: "Events dropped due to rate limiting",
	})

	flowsCorrelated = auto.NewCounter(prom.CounterOpts{
		Name: "trace_collector_flows_correlated_total",
		Help: "Flows correlated (BIRTH to DEATH)",
	})

	batchLatency = auto.NewHistogram(prom.HistogramOpts{
		Name:    "trace_collector_batch_latency_seconds",
		Help:    "Time to publish a batch to Wotan",
		Buckets: prom.DefBuckets,
	})

	programsLoaded = auto.NewGaugeVec(prom.GaugeOpts{
		Name: "trace_collector_programs_loaded",
		Help: "BPF programs loaded (1=loaded, 0=not)",
	}, []string{"program"})
)

// ── Wotan topic names (Anamnesis events) ────────────────────────────────

const (
	TopicBirth   = "anamnesis.birth"
	TopicHop     = "anamnesis.hop"
	TopicDeath   = "anamnesis.death"
	TopicAnomaly = "anamnesis.anomaly"
	TopicChaos   = "anamnesis.chaos"
)

func topicForEvent(et ebpf.EventType) string {
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

// ── Flow correlator ─────────────────────────────────────────────────────

// FlowRecord accumulates events for a single flow (grouped by flow_label).
type FlowRecord struct {
	FlowLabel uint16                 `json:"flow_label"`
	Birth     *ebpf.AnamnesisEvent   `json:"birth,omitempty"`
	Hops      []*ebpf.AnamnesisEvent `json:"hops"`
	Death     *ebpf.AnamnesisEvent   `json:"death,omitempty"`
	Anomalies []*ebpf.AnamnesisEvent `json:"anomalies,omitempty"`
	StartNs   uint64                 `json:"start_ns"`
	EndNs     uint64                 `json:"end_ns"`
	HopCount  int                    `json:"hop_count"`
	Complete  bool                   `json:"complete"`
}

// FlowCorrelator groups events by flow_label into FlowRecords.
type FlowCorrelator struct {
	mu    sync.Mutex
	flows map[uint16]*FlowRecord
	out   chan *FlowRecord
	ttl   time.Duration
}

// NewFlowCorrelator creates a correlator that emits completed flows.
func NewFlowCorrelator(ttl time.Duration) *FlowCorrelator {
	return &FlowCorrelator{
		flows: make(map[uint16]*FlowRecord),
		out:   make(chan *FlowRecord, 1024),
		ttl:   ttl,
	}
}

// Add adds an event to the correlator.
func (fc *FlowCorrelator) Add(ev *ebpf.AnamnesisEvent) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	fl := ev.FlowLabelLo
	rec, exists := fc.flows[fl]
	if !exists {
		rec = &FlowRecord{
			FlowLabel: fl,
			StartNs:   ev.TimestampNs,
		}
		fc.flows[fl] = rec
	}

	switch ev.EventType {
	case ebpf.EventBirth:
		rec.Birth = ev
		rec.StartNs = ev.TimestampNs
	case ebpf.EventHop:
		rec.Hops = append(rec.Hops, ev)
		rec.HopCount = len(rec.Hops)
	case ebpf.EventDeath:
		rec.Death = ev
		rec.EndNs = ev.TimestampNs
		rec.Complete = true
	case ebpf.EventAnomaly, ebpf.EventChaos:
		rec.Anomalies = append(rec.Anomalies, ev)
	}

	if rec.Complete {
		delete(fc.flows, fl)
		select {
		case fc.out <- rec:
			flowsCorrelated.Inc()
		default:
			// Channel full, drop
		}
	}
}

// CompletedFlows returns the channel of completed flow records.
func (fc *FlowCorrelator) CompletedFlows() <-chan *FlowRecord {
	return fc.out
}

// GC removes stale flows older than TTL.
func (fc *FlowCorrelator) GC(nowNs uint64) int {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	ttlNs := uint64(fc.ttl.Nanoseconds()) // #nosec G115 -- bounded by construction; see the surrounding guard
	removed := 0
	for fl, rec := range fc.flows {
		if olderThan(nowNs, rec.StartNs, ttlNs) {
			delete(fc.flows, fl)
			removed++
		}
	}
	return removed
}

// Len returns the number of in-flight flows.
func (fc *FlowCorrelator) Len() int {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	return len(fc.flows)
}

// ── Rate limiter ────────────────────────────────────────────────────────

// RateLimiter implements a simple token bucket rate limiter.
type RateLimiter struct {
	maxRate  int64
	tokens   int64
	lastFill int64 // unix nano
	mu       sync.Mutex
	dropped  uint64
}

// NewRateLimiter creates a rate limiter. maxRate=0 means unlimited.
func NewRateLimiter(maxRate int) *RateLimiter {
	return &RateLimiter{
		maxRate:  int64(maxRate),
		tokens:   int64(maxRate),
		lastFill: time.Now().UnixNano(),
	}
}

// Allow returns true if the event should be processed.
func (rl *RateLimiter) Allow() bool {
	if rl.maxRate <= 0 {
		return true
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now().UnixNano()
	elapsed := now - rl.lastFill
	if elapsed > 0 {
		newTokens := (elapsed * rl.maxRate) / int64(time.Second)
		rl.tokens += newTokens
		if rl.tokens > rl.maxRate {
			rl.tokens = rl.maxRate
		}
		rl.lastFill = now
	}

	if rl.tokens > 0 {
		rl.tokens--
		return true
	}

	atomic.AddUint64(&rl.dropped, 1)
	return false
}

// Dropped returns the number of dropped events.
func (rl *RateLimiter) Dropped() uint64 {
	return atomic.LoadUint64(&rl.dropped)
}

// ── Event transformer ───────────────────────────────────────────────────

// TransformEvent converts an AnamnesisEvent to a Wotan-publishable JSON payload.
func TransformEvent(ev *ebpf.AnamnesisEvent) ([]byte, error) {
	return json.Marshal(ev)
}

// ── Legacy Wotan publisher (Anamnesis mode) ─────────────────────────────

// WotanPublisher batches events and publishes them to Wotan topics.
// This is the legacy publisher used in Anamnesis mode. The unified mode
// uses TracePublisher from publisher.go instead, and gives its readers no
// WotanPublisher: they used to get one that was never connected, so every
// batch also went out over the REST path below, duplicating what
// TracePublisher had already sent.
type WotanPublisher struct {
	addr      string
	batchSize int
	batchTime time.Duration
	connected atomic.Bool
	published uint64
	tsc       *wotanClient.TopicStreamClient
	// rest publishes when gRPC never connected: wotan-client's HTTP client,
	// which joins the topic and sends {subscriber_id, payload}. This path
	// used to POST the raw batch as the body; Wotan answered 400 to every
	// one ("invalid request body" for an array, "payload is required" for
	// an object) and the status was never read, so it counted as success
	// (~150 rejected requests/s on west, 2026-10-18).
	rest   *wotanClient.Client
	restMu sync.Mutex // one join per topic

	// failures: each topic's run of failed PublishRaw calls, logged like
	// TracePublisher's (first, every failLogEvery-th, recovery). The readers
	// logged every failure: 755k lines in west's syslog in four days with
	// Wotan down (2026-10-18..22).
	failMu   sync.Mutex
	failures map[string]uint64
}

// NewWotanPublisher creates a publisher.
func NewWotanPublisher(addr string, batchSize int, batchTimeout time.Duration) *WotanPublisher {
	wp := &WotanPublisher{addr: addr, batchSize: batchSize, batchTime: batchTimeout}
	if c, err := wotanClient.NewClient(addr); err == nil {
		wp.rest = c
	}
	return wp
}

// restPublish publishes over Wotan's REST API, joining the topic first.
func (wp *WotanPublisher) restPublish(ctx context.Context, topic string, payload []byte) error {
	if wp.rest == nil {
		return fmt.Errorf("no Wotan address for REST publishing")
	}
	wp.restMu.Lock()
	if _, ok := wp.rest.GetSubscriber(topic); !ok {
		if _, err := wp.rest.Subscribe(ctx, topic, "trace-collector"); err != nil {
			wp.restMu.Unlock()
			return fmt.Errorf("join %s: %w", topic, err)
		}
	}
	wp.restMu.Unlock()
	return wp.rest.Publish(ctx, topic, payload)
}

// ConnectGRPC establishes a gRPC TopicStream connection to Wotan.
// If this fails, PublishBatch falls back to HTTP.
func (wp *WotanPublisher) ConnectGRPC(ctx context.Context, grpcAddr, httpAddr string) error {
	var opts []wotanClient.TopicStreamOption
	if httpAddr != "" {
		httpFallback, err := wotanClient.NewClient(httpAddr)
		if err == nil {
			opts = append(opts, wotanClient.WithHTTPFallback(httpFallback))
		}
	}
	tsc, err := wotanClient.NewTopicStreamClient(grpcAddr, opts...)
	if err != nil {
		return fmt.Errorf("connect gRPC at %s: %w", grpcAddr, err)
	}

	// Subscribe to all topics we'll publish to
	topics := []string{
		TopicBirth, TopicHop, TopicDeath, TopicAnomaly, TopicChaos,
		"ebpf.flow.events", "ebpf.packet.events", "ebpf.latency.events",
	}
	for _, topic := range topics {
		if _, err := tsc.Subscribe(ctx, topic, "trace-collector"); err != nil {
			log.Warn().Err(err).Str("topic", topic).Msg("failed to subscribe")
		}
	}

	wp.tsc = tsc
	return nil
}

// PublishBatch publishes a batch of events to a Wotan topic.
// In production this would use the gRPC TopicStreamClient.
// For now, we use the HTTP REST API as a simple starting point.
func (wp *WotanPublisher) PublishBatch(ctx context.Context, topic string, events []*ebpf.AnamnesisEvent) error {
	if len(events) == 0 {
		return nil
	}

	start := time.Now()

	// Serialize batch as JSON array
	payload, err := json.Marshal(events)
	if err != nil {
		return fmt.Errorf("marshal batch: %w", err)
	}

	// Publish via gRPC TracePublisher if available, else HTTP fallback.
	if wp.tsc != nil {
		if err := wp.tsc.Publish(ctx, topic, payload); err != nil {
			wp.connected.Store(false)
			return fmt.Errorf("gRPC publish to %s: %w", topic, err)
		}
	} else if err := wp.restPublish(ctx, topic, payload); err != nil {
		wp.connected.Store(false)
		return fmt.Errorf("HTTP publish to %s: %w", topic, err)
	}

	atomic.AddUint64(&wp.published, uint64(len(events)))
	wp.connected.Store(true)

	batchLatency.Observe(time.Since(start).Seconds())
	eventsPublished.WithLabelValues(topic).Add(float64(len(events)))

	return nil
}

// PublishRaw publishes a raw JSON payload to a Wotan topic. Callers treat
// it as best-effort; failures are logged here, once per outage.
func (wp *WotanPublisher) PublishRaw(ctx context.Context, topic string, payload []byte) error {
	var err error
	if wp.tsc != nil {
		err = wp.tsc.Publish(ctx, topic, payload)
	} else {
		err = wp.restPublish(ctx, topic, payload)
	}
	wp.notePublish(topic, err)
	return err
}

// notePublish records a PublishRaw outcome on topic and logs the first
// failure of a run, every failLogEvery-th, and the recovery.
func (wp *WotanPublisher) notePublish(topic string, err error) {
	wp.failMu.Lock()
	n := wp.failures[topic]
	if err == nil {
		delete(wp.failures, topic)
	} else {
		n++
		if wp.failures == nil {
			wp.failures = make(map[string]uint64)
		}
		wp.failures[topic] = n
	}
	wp.failMu.Unlock()
	switch {
	case err == nil && n > 0:
		log.Info().Str("topic", topic).Uint64("failed", n).Msg("best-effort publishing recovered")
	case err != nil && (n == 1 || n%failLogEvery == 0):
		log.Warn().Err(err).Str("topic", topic).Uint64("failed", n).Msg("best-effort publish failed (logged once per outage, then every 1000)")
	}
}

// IsConnected returns true if the last publish succeeded.
func (wp *WotanPublisher) IsConnected() bool {
	return wp.connected.Load()
}

// anamnesisToFlowJSON converts an AnamnesisEvent to a dashboard-compatible
// FlowEvent JSON. Returns nil for event types that don't map to flows.
func anamnesisToFlowJSON(ev *ebpf.AnamnesisEvent) []byte {
	// Map anamnesis birth/hop events to flow events the dashboard understands
	var state, eventType string
	switch ev.EventType {
	case ebpf.EventBirth:
		state = "new"
		eventType = "new"
	case ebpf.EventHop:
		state = "established"
		eventType = "update"
	case ebpf.EventDeath:
		state = "closed"
		eventType = "close"
	default:
		return nil
	}

	// Build a FlowEvent-compatible JSON object
	fl := ev.FlowLabelLo
	hop := ev.HopID
	flow := map[string]interface{}{
		"timestamp_ns": ev.TimestampNs,
		"flow_key": map[string]interface{}{
			"src_addr": fmt.Sprintf("10.%d.%d.%d", (fl>>8)&0xff, fl&0xff, hop),
			"dst_addr": fmt.Sprintf("10.%d.%d.%d", (fl>>8)&0xff, (fl&0xff)+1, hop),
			"src_port": 1024 + (uint32(fl) % 64000),
			"dst_port": 443,
			"protocol": 6,
		},
		"flow_state": map[string]interface{}{
			"state":      state,
			"packets_in": uint32(hop) + 1,
			"bytes_in":   (uint64(hop) + 1) * 1500,
		},
		"event_type": eventType,
	}
	data, err := json.Marshal(flow)
	if err != nil {
		return nil
	}
	return data
}

// ── Legacy health check handler (Anamnesis mode) ────────────────────────

type healthHandler struct {
	publisher  *WotanPublisher
	correlator *FlowCorrelator
	reader     *ebpf.AnamnesisReader
}

func (h *healthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	status := struct {
		Status        string           `json:"status"`
		Wotan         bool             `json:"wotan_connected"`
		InFlightFlows int              `json:"in_flight_flows"`
		ReaderStats   ebpf.ReaderStats `json:"reader_stats"`
	}{
		Status:        "ok",
		Wotan:         h.publisher.IsConnected(),
		InFlightFlows: h.correlator.Len(),
	}
	if h.reader != nil {
		status.ReaderStats = h.reader.Stats()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status) // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
}

// ── FlowReader / LatencyReader interfaces ───────────────────────────────
// These interfaces define the contract for flow_reader.go and
// latency_reader.go, which are being built by parallel agents.

// FlowReaderI reads flow state from the flow_tracker BPF program.
type FlowReaderI interface {
	// Run starts the periodic flow state reading loop.
	Run(ctx context.Context)

	// Stats returns a snapshot of reader statistics.
	FlowStats() map[string]uint64
}

// LatencyReaderI reads latency measurements from the latency_probe BPF program.
type LatencyReaderI interface {
	// Run starts the periodic latency reading loop.
	Run(ctx context.Context)

	// Stats returns a snapshot of reader statistics.
	LatencyStats() map[string]uint64
}

// ── BPF object directory resolution ─────────────────────────────────────

// resolveEBPFObjDir determines the directory containing compiled BPF ELF
// objects. It checks (in priority order):
//  1. --ebpf-obj-dir flag
//  2. UNHEADED_EBPF_OBJ_DIR environment variable
//  3. /opt/unheaded/ebpf/ (production install path)
//  4. ebpf/target/bpfel-unknown-none/release (development default)
func resolveEBPFObjDir() string {
	// 1. CLI flag takes highest priority
	if *ebpfObjDir != "" {
		abs, err := filepath.Abs(*ebpfObjDir)
		if err != nil {
			log.Warn().Err(err).Str("path", *ebpfObjDir).Msg("cannot resolve --ebpf-obj-dir, using as-is")
			return *ebpfObjDir
		}
		return abs
	}

	// 2. Environment variable
	if envDir := os.Getenv("UNHEADED_EBPF_OBJ_DIR"); envDir != "" {
		abs, err := filepath.Abs(envDir)
		if err != nil {
			log.Warn().Err(err).Str("path", envDir).Msg("cannot resolve UNHEADED_EBPF_OBJ_DIR, using as-is")
			return envDir
		}
		return abs
	}

	// 3. Production install path
	prodPath := "/opt/unheaded/ebpf"
	if info, err := os.Stat(prodPath); err == nil && info.IsDir() {
		return prodPath
	}

	// 4. Development default (relative to working directory)
	devPath := "ebpf/target/bpfel-unknown-none/release"
	abs, err := filepath.Abs(devPath)
	if err != nil {
		return devPath
	}
	return abs
}

// ── Unified mode runner ─────────────────────────────────────────────────

// runUnifiedMode starts the unified trace collector that loads all three
// BPF programs (packet_marker, flow_tracker, latency_probe) based on flags.
func runUnifiedMode(ctx context.Context, healthSrv *transport.HealthServer, transportCfg transport.Config) {
	log.Info().
		Str("interface", *iface).
		Str("wotan_addr", *wotanAddr).
		Str("metrics_addr", *metricsAddr).
		Str("map_pin_path", *mapPinPath).
		Dur("read_interval", *readInterval).
		Bool("packet_marker", *enablePacketMarker).
		Bool("flow_tracker", *enableFlowTracker).
		Bool("latency_probe", *enableLatencyProbe).
		Msg("trace-collector-go starting in unified mode")

	// Create shared state
	state := NewCollectorState()

	// Create the unified publisher with gRPC-first transport
	pubConfig := DefaultTracePublisherConfig()
	// Feed the publisher the RESOLVED addresses, not the raw flags, so it
	// honours the environment the same way the transport config does.
	pubConfig.WotanAddr = transportCfg.WotanGRPCAddr
	pubConfig.WotanHTTPAddr = transportCfg.WotanHTTPAddr
	pubConfig.BatchSize = *batchSize
	pubConfig.FlushInterval = *batchTimeout
	publisher := NewTracePublisher(pubConfig)

	// Connect publisher to Wotan via gRPC
	if err := publisher.Connect(ctx); err != nil {
		log.Warn().Err(err).Msg("Wotan gRPC connection failed, events will be dropped until connected")
	}
	state.Publisher = publisher

	// Create the packet correlator
	corrConfig := DefaultPacketCorrelatorConfig()
	correlator := NewPacketCorrelator(corrConfig)
	state.PacketCorrelator = correlator

	// Create BPF loader — NativeBPFLoader bridges to pkg/ebpf.NativeLoader
	var loader BPFLoader
	loaderCfg := DefaultNativeBPFLoaderConfig()
	loaderCfg.PinPath = *mapPinPath
	loaderCfg.AllowUntrustedObjectPaths = *allowUntrustedObjs
	nativeLoader, err := NewNativeBPFLoader(ctx, loaderCfg)
	if err != nil {
		log.Warn().Err(err).Msg("native BPF loader unavailable, falling back to mock")
		loader = NewMockBPFLoader()
	} else {
		loader = nativeLoader
	}

	// Track program status
	programs := []ProgramStatus{
		{Name: "packet_marker", Enabled: *enablePacketMarker},
		{Name: "flow_tracker", Enabled: *enableFlowTracker},
		{Name: "latency_probe", Enabled: *enableLatencyProbe},
	}

	// BPF ELF binary base path — compiled by Aya/Rust.
	// Resolved from (in priority order):
	//   1. --ebpf-obj-dir flag
	//   2. UNHEADED_EBPF_OBJ_DIR environment variable
	//   3. /opt/unheaded/ebpf/ (production install)
	//   4. ebpf/target/bpfel-unknown-none/release (development)
	elfBase := resolveEBPFObjDir()

	// Load BPF programs from compiled ELF binaries
	if *enablePacketMarker {
		progPath := fmt.Sprintf("%s/packet-marker", elfBase)
		if err := loader.Load(progPath); err != nil {
			log.Error().Err(err).Str("path", progPath).Msg("failed to load packet_marker")
		} else {
			programs[0].Loaded = true
			programsLoaded.WithLabelValues("packet_marker").Set(1)
			log.Info().Msg("packet_marker loaded")
		}
	}

	if *enableFlowTracker {
		progPath := fmt.Sprintf("%s/flow-tracker", elfBase)
		if err := loader.Load(progPath); err != nil {
			log.Error().Err(err).Str("path", progPath).Msg("failed to load flow_tracker")
		} else {
			programs[1].Loaded = true
			programsLoaded.WithLabelValues("flow_tracker").Set(1)
			log.Info().Msg("flow_tracker loaded")
		}
	}

	// Latency probe: use direct cilium/ebpf loader for proper multi-kprobe support.
	// The standard loader only attaches one kprobe; we need all 6 (3 enter + 3 exit).
	var latencyKprobeLoader *LatencyKprobeLoader
	var directLatencyMap interface {
		Iterate() *ciliumebpf.MapIterator
	}
	if *enableLatencyProbe {
		progPath := fmt.Sprintf("%s/latency-probe", elfBase)
		kl, lmap, err := LoadLatencyKprobes(progPath)
		if err != nil {
			// No fallback: the native loader cannot load this object (one
			// program per object vs six kprobe functions). The old fallback
			// loaded one function, attached none and reported loaded.
			log.Error().Err(err).Str("path", progPath).Msg("failed to load latency kprobes via cilium/ebpf; latency_probe NOT loaded")
		} else {
			latencyKprobeLoader = kl
			directLatencyMap = lmap
			programs[2].Loaded = true
			// Already attached by LoadLatencyKprobes
			programsLoaded.WithLabelValues("latency_probe").Set(1)
			log.Info().Msg("latency_probe loaded via cilium/ebpf (all kprobes attached)")
		}
	}

	// Attach all loaded programs to the target interface
	if err := loader.AttachXDP(*iface); err != nil {
		log.Error().Err(err).Str("iface", *iface).Msg("failed to attach BPF programs")
	} else {
		log.Info().Str("iface", *iface).Msg("BPF programs attached")
	}

	state.mu.Lock()
	state.Programs = programs
	state.mu.Unlock()

	// Leave out the collector's own connections (to Wotan, mostly): each
	// batch it publishes would otherwise be traced into the next one.
	self := newSelfFilter("/proc/self")
	go self.run(ctx, 5*time.Second, func(err error) {
		log.Warn().Err(err).Msg("self-traffic filter: could not read /proc/self sockets")
	})

	// Start the trace reader (packet_marker maps)
	if *enablePacketMarker {
		readerConfig := DefaultTraceReaderConfig()
		readerConfig.ReadInterval = *readInterval
		// No reader-side publisher: the loop below publishes each entry
		// through TracePublisher.
		traceReader := NewTraceReader(loader, nil, readerConfig)
		state.TraceReader = traceReader

		go traceReader.Run(ctx)

		// Feed trace events into correlator and publisher
		go func() {
			for entry := range traceReader.Events() {
				if self.drop(entry.SrcIPAddr(), entry.SrcPort, entry.DstIPAddr(), entry.DstPort) {
					continue
				}
				state.RecordTrace(entry)
				correlator.Add(entry)
				if err := publisher.PublishPacketEvent(entry); err != nil {
					log.Error().Err(err).Msg("failed to publish packet event")
					healthSrv.SetGRPCStatus(false)
				}
			}
		}()
	}

	// Start FlowReader (flow_tracker TC maps)
	if *enableFlowTracker && programs[1].Loaded {
		flowConfig := DefaultFlowReaderConfig()
		flowReader := NewFlowReader(loader, nil, flowConfig) // published below via TracePublisher

		go flowReader.Run(ctx)

		// Feed flow events into publisher (dashboard-compatible format)
		go func() {
			for ev := range flowReader.Events() {
				if self.drop(ev.Key.SrcIP(), ev.Key.SrcPort, ev.Key.DstIP(), ev.Key.DstPort) {
					continue
				}
				payload, err := marshalDashboardFlow(ev)
				if err != nil {
					log.Error().Err(err).Msg("failed to marshal flow event")
					continue
				}
				publisher.PublishFlowEvent(payload)
			}
		}()

		log.Info().Msg("flow reader started for flow_tracker")
	}

	// Start LatencyReader (latency_probe kprobe maps)
	if *enableLatencyProbe && programs[2].Loaded {
		if directLatencyMap != nil {
			// Use direct cilium/ebpf map reader for proper kprobe data
			go func() {
				ticker := time.NewTicker(200 * time.Millisecond)
				defer ticker.Stop()
				rtts := newRTTTracker()
				log.Info().Msg("direct latency map reader started (cilium/ebpf)")
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						iter := directLatencyMap.Iterate()
						var key, value []byte
						for iter.Next(&key, &value) {
							lk, err := DecodeLatencyKey(key)
							if err != nil {
								continue
							}
							le, err := DecodeLatencyEntry(value)
							if err != nil {
								continue
							}
							// Only flows with a new measurement since the last
							// sweep; re-emitting every flow's latest RTT each
							// 200 ms weighted percentiles by flows x sweeps.
							if !rtts.fresh(key, le.Samples) {
								continue
							}

							payload, err := json.Marshal(latencyMapEvent(lk, le, time.Now()))
							if err != nil {
								continue
							}
							publisher.PublishLatencyEvent(payload)
						}
						if iter.Err() == nil { // a partial sweep must not forget unvisited flows
							rtts.endSweep()
						}
					}
				}
			}()
			log.Info().Msg("latency reader started (direct cilium/ebpf map)")
		} else {
			// Fallback: use standard loader's map reader
			latencyConfig := DefaultLatencyReaderConfig()
			latencyReader := NewLatencyReader(loader, nil, latencyConfig) // published below via TracePublisher

			go latencyReader.Run(ctx)

			go func() {
				for sample := range latencyReader.Samples() {
					payload, err := json.Marshal(sample)
					if err != nil {
						log.Error().Err(err).Msg("failed to marshal latency sample")
						continue
					}
					publisher.PublishLatencyEvent(payload)
				}
			}()

			log.Info().Msg("latency reader started for latency_probe (fallback)")
		}
	}
	// Ensure kprobe loader is cleaned up on exit
	if latencyKprobeLoader != nil {
		defer latencyKprobeLoader.Close()
	}

	// Start correlator GC
	go correlator.RunGC(ctx)

	// Feed correlated flows to publisher
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case flow, ok := <-correlator.CorrelatedFlows():
				if !ok {
					return
				}
				if err := publisher.PublishCorrelatedFlow(flow); err != nil {
					log.Error().Err(err).Msg("failed to publish correlated flow")
					healthSrv.SetGRPCStatus(false)
				}
			}
		}
	}()

	// Start publisher flush loop
	go publisher.Run(ctx) // #nosec G118 -- publisher.Run already takes and honours ctx

	// Start HTTP server
	mux := http.NewServeMux()
	mux.Handle("/health", &HealthHandler{State: state})
	mux.Handle("/ready", &ReadyHandler{State: state})
	mux.Handle("/metrics", prom.Handler())
	mux.Handle("/api/v1/traces", &TracesHandler{State: state})
	mux.Handle("/api/v1/stats", &StatsHandler{State: state})

	// Auth middleware (activated via AUTH_ENABLED=true)
	unifiedAuthCfg := auth.LoadServiceAuthConfig("trace-collector")
	unifiedHandler := auth.WrapHandler(mux, auth.SetupMiddleware(unifiedAuthCfg))

	httpServer := &http.Server{
		Addr:           *metricsAddr,
		Handler:        unifiedHandler,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1 MB
		// ReadHeaderTimeout bounds slowloris; this server already sets
		// Read/WriteTimeout above. gosec G112.
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Info().Str("addr", *metricsAddr).Msg("unified HTTP server starting")
		if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
			log.Error().Err(err).Msg("HTTP server error")
		}
	}()

	// Wait for shutdown
	<-ctx.Done()

	log.Info().Msg("unified mode shutting down")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = httpServer.Shutdown(shutdownCtx)

	// Cleanup BPF programs
	if err := loader.Close(); err != nil {
		log.Error().Err(err).Msg("error closing BPF loader")
	}

	log.Info().Msg("unified trace-collector-go stopped")
}

// ── Legacy Anamnesis mode runner ────────────────────────────────────────

// runAnamnesisMode starts the original Anamnesis-based trace collector.
func runAnamnesisMode(ctx context.Context, healthSrv *transport.HealthServer, transportCfg transport.Config) {
	// Resolved addresses, not raw flags: this is the DEFAULT mode, and it
	// read the flags directly, so WOTAN_GRPC_ADDR / WOTAN_ADDR were ignored
	// entirely on the path that actually runs.
	wotanGRPC := transportCfg.WotanGRPCAddr
	wotanHTTP := transportCfg.WotanHTTPAddr

	log.Info().
		Str("ring_path", *ringPath).
		Str("wotan_addr", wotanGRPC).
		Str("http_addr", *httpAddr).
		Int("max_rate", *maxRate).
		Int("batch_size", *batchSize).
		Msg("trace-collector-go starting in anamnesis mode")

	// Create components
	publisher := NewWotanPublisher(wotanHTTP, *batchSize, *batchTimeout)

	// Connect via gRPC for reliable publishing
	if err := publisher.ConnectGRPC(ctx, wotanGRPC, wotanHTTP); err != nil {
		log.Warn().Err(err).Msg("gRPC connection failed, falling back to HTTP")
	} else {
		log.Info().Str("grpc", wotanGRPC).Msg("anamnesis publisher connected via gRPC")
	}

	correlator := NewFlowCorrelator(60 * time.Second)
	limiter := NewRateLimiter(*maxRate)

	// Get event source from BPF ring buffer
	reader, err := ebpf.OpenPinnedAnamnesisReader(ctx, *ringPath, *bufSize)
	if err != nil {
		log.Fatal().Err(err).Str("path", *ringPath).Msg("failed to open ring buffer")
	}
	log.Info().Str("path", *ringPath).Msg("ring buffer opened")

	events := reader.Poll(ctx)

	// Start HTTP server for health/metrics
	mux := http.NewServeMux()
	mux.Handle("/metrics", prom.Handler())
	mux.Handle("/healthz", &healthHandler{
		publisher:  publisher,
		correlator: correlator,
		reader:     reader,
	})
	healthSrv.RegisterHTTP(mux)

	// Auth middleware (activated via AUTH_ENABLED=true)
	anamnesisAuthCfg := auth.LoadServiceAuthConfig("trace-collector")
	anamnesisHandler := auth.WrapHandler(mux, auth.SetupMiddleware(anamnesisAuthCfg))

	httpServer := &http.Server{
		Addr:           *httpAddr,
		Handler:        anamnesisHandler,
		MaxHeaderBytes: 1 << 20, // 1 MB
		// Timeouts match the service template in pkg/service/service.go.
		// Without ReadHeaderTimeout a client can hold the connection open
		// sending headers a byte at a time (slowloris). gosec G112.
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		log.Info().Str("addr", *httpAddr).Msg("HTTP server starting")
		if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
			log.Error().Err(err).Msg("HTTP server error")
		}
	}()

	// Start flow GC goroutine
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				removed := correlator.GC(ktimeNow())
				if removed > 0 {
					log.Debug().Int("removed", removed).Int("remaining", correlator.Len()).Msg("flow GC")
				}
			}
		}
	}()

	// Batching state
	batches := make(map[string][]*ebpf.AnamnesisEvent)
	batchTimer := time.NewTicker(*batchTimeout)
	defer batchTimer.Stop()

	flushBatches := func() {
		for topic, evs := range batches {
			if len(evs) == 0 {
				continue
			}
			if err := publisher.PublishBatch(ctx, topic, evs); err != nil {
				log.Error().Err(err).Str("topic", topic).Int("count", len(evs)).Msg("publish failed")
				healthSrv.SetGRPCStatus(false)
			}
			delete(batches, topic)
		}
	}

	// Main event loop
	log.Info().Msg("event processing started")
	var wg sync.WaitGroup

	// Process completed flows (log them)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case flow, ok := <-correlator.CompletedFlows():
				if !ok {
					return
				}
				log.Debug().
					Uint16("flow_label", flow.FlowLabel).
					Int("hops", flow.HopCount).
					Bool("complete", flow.Complete).
					Msg("flow completed")
			}
		}
	}()

	// Main event processing loop
	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("shutting down...")
			flushBatches()
			_ = httpServer.Shutdown(context.Background())
			wg.Wait()

			stats := reader.Stats()
			log.Info().
				Uint64("events_read", stats.EventsRead).
				Uint64("events_dropped", stats.EventsDropped).
				Uint64("decode_errors", stats.DecodeErrors).
				Uint64("rate_limited", limiter.Dropped()).
				Msg("final stats")
			return

		case ev, ok := <-events:
			if !ok {
				log.Info().Msg("event channel closed")
				flushBatches()
				return
			}

			// Rate limit
			if !limiter.Allow() {
				eventsDropped.Inc()
				continue
			}

			// Count by type
			eventsTotal.WithLabelValues(ev.EventType.String()).Inc()

			// Correlate
			correlator.Add(ev)

			// Batch for publishing to anamnesis topics
			topic := topicForEvent(ev.EventType)
			batches[topic] = append(batches[topic], ev)

			// Also publish as dashboard-compatible flow events so the
			// dashboard ingestor can display them (it expects ebpf.*.events).
			if dashPayload := anamnesisToFlowJSON(ev); dashPayload != nil {
				_ = publisher.PublishRaw(ctx, "ebpf.flow.events", dashPayload)
			}

			// Flush if batch full
			for t, b := range batches {
				if len(b) >= *batchSize {
					if err := publisher.PublishBatch(ctx, t, b); err != nil {
						log.Error().Err(err).Str("topic", t).Msg("publish failed")
						healthSrv.SetGRPCStatus(false)
					}
					delete(batches, t)
				}
			}

		case <-batchTimer.C:
			flushBatches()
		}
	}
}

// ── Main ────────────────────────────────────────────────────────────────

func main() {
	flag.Parse()
	wotanClient.InitServiceIdentity("trace-collector") // ADR-103 §6: every Wotan call carries trace-collector's service token

	// Configure logging
	if *logJSON {
		zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	} else {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
	}

	// Transport config: defaults → env → flag overrides.
	//
	// "flag overrides" means a flag that was actually passed. Assigning
	// unconditionally let the "localhost:18001" default overwrite
	// WOTAN_GRPC_ADDR from the environment, which is how the comment and the
	// code disagreed.
	transportCfg := transport.DefaultConfig()
	transport.ConfigFromEnv(&transportCfg)
	if transport.FlagWasSet("wotan-addr") {
		transportCfg.WotanGRPCAddr = *wotanAddr
	}
	if transport.FlagWasSet("wotan-http-addr") {
		transportCfg.WotanHTTPAddr = *wotanHTTPAddr
	}

	// Unified health server for transport-aware readiness
	healthSrv := transport.NewHealthServer("trace-collector")

	// Log aggregation publisher — forwards structured logs to Wotan
	var logConn transport.Connection
	if *wotanAddr != "" {
		var err error
		if logConn, err = logagg.Connect(context.Background(), transportCfg, "trace-collector"); err != nil {
			// best-effort: logs stay local, but say why so a nil here is not silent
			log.Warn().Err(err).Msg("log aggregation: no transport connection; logs not forwarded")
			logConn = nil
		}
	}
	logPublisher := logagg.NewPublisher("trace-collector", logConn)
	// Surface log-forwarding health on /metrics (promhttp default registry).
	prom.MustRegister(logPublisher.Collectors()...)
	log.Logger = log.Logger.Hook(logPublisher)

	// Create context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Service discovery: announced on Wotan's system.discovery topic (best-effort)
	// Announce the port that actually serves: unified mode serves only
	// --metrics-addr (16671); it used to announce 16670 regardless.
	announceAddr := *httpAddr
	if *unifiedMode {
		announceAddr = *metricsAddr
	}
	discovery.Start(ctx, transportCfg, "trace-collector", ports.PortOf(announceAddr, ports.TraceCollectorHTTP))
	healthreport.StartService(ctx, transportCfg, "trace-collector", healthreport.HealthURL(announceAddr, ports.PortOf(announceAddr, ports.TraceCollectorHTTP))) // ADR-029: a vote on Akira's ballot

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Info().Str("signal", sig.String()).Msg("shutdown signal received")
		cancel()
	}()

	if *unifiedMode {
		runUnifiedMode(ctx, healthSrv, transportCfg)
	} else {
		runAnamnesisMode(ctx, healthSrv, transportCfg)
	}
}
