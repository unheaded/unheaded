// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

/*
 * This file is part of the Unheaded distributed system platform.
 *
 * Unheaded is licensed under the MIT License.
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
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"strings"
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
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"unheaded/pkg/auth"
	"unheaded/pkg/discovery"
	"unheaded/pkg/ebpf"
	"unheaded/pkg/logagg"
	"unheaded/pkg/ports"
	"unheaded/pkg/transport"
	wotanClient "unheaded/pkg/wotan-client"
)

// ── CLI flags ───────────────────────────────────────────────────────────

var (
	// Existing Anamnesis-mode flags
	ringPath     = flag.String("ring-path", ebpf.DefaultAnamnesisRingPath, "BPF ring buffer pin path")
	wotanAddr     = flag.String("wotan-addr", "localhost:18001", "Wotan gRPC address")
	wotanHTTPAddr = flag.String("wotan-http-addr", "localhost:18000", "Wotan HTTP address (fallback)")
	httpAddr     = flag.String("http-addr", ports.DefaultAddr(ports.TraceCollectorHTTP), "HTTP address for health/metrics")
	maxRate      = flag.Int("max-rate", 10000, "Max events/sec (0 = unlimited)")
	batchSize    = flag.Int("batch-size", 100, "Publish batch size")
	batchTimeout = flag.Duration("batch-timeout", 50*time.Millisecond, "Publish batch timeout")
	bufSize      = flag.Int("buf-size", 8192, "Event channel buffer size")
	logJSON      = flag.Bool("log-json", false, "Use JSON log format")

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
)

// ── Prometheus metrics ──────────────────────────────────────────────────

var (
	eventsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "trace_collector_events_total",
		Help: "Total Anamnesis events processed",
	}, []string{"type"})

	eventsPublished = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "trace_collector_events_published_total",
		Help: "Total events published to Wotan",
	}, []string{"topic"})

	eventsDropped = promauto.NewCounter(prometheus.CounterOpts{
		Name: "trace_collector_events_dropped_total",
		Help: "Events dropped due to rate limiting",
	})

	flowsCorrelated = promauto.NewCounter(prometheus.CounterOpts{
		Name: "trace_collector_flows_correlated_total",
		Help: "Flows correlated (BIRTH to DEATH)",
	})

	batchLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "trace_collector_batch_latency_seconds",
		Help:    "Time to publish a batch to Wotan",
		Buckets: prometheus.DefBuckets,
	})

	programsLoaded = promauto.NewGaugeVec(prometheus.GaugeOpts{
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
	FlowLabel uint16               `json:"flow_label"`
	Birth     *ebpf.AnamnesisEvent `json:"birth,omitempty"`
	Hops      []*ebpf.AnamnesisEvent `json:"hops"`
	Death     *ebpf.AnamnesisEvent `json:"death,omitempty"`
	Anomalies []*ebpf.AnamnesisEvent `json:"anomalies,omitempty"`
	StartNs   uint64               `json:"start_ns"`
	EndNs     uint64               `json:"end_ns"`
	HopCount  int                  `json:"hop_count"`
	Complete  bool                 `json:"complete"`
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

	ttlNs := uint64(fc.ttl.Nanoseconds())
	removed := 0
	for fl, rec := range fc.flows {
		if nowNs-rec.StartNs > ttlNs {
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
// uses TracePublisher from publisher.go instead.
type WotanPublisher struct {
	addr       string
	batchSize  int
	batchTime  time.Duration
	connected  atomic.Bool
	published  uint64
	errors     uint64
	tsc        *wotanClient.TopicStreamClient
	httpClient *http.Client
}

// NewWotanPublisher creates a publisher.
func NewWotanPublisher(addr string, batchSize int, batchTimeout time.Duration) *WotanPublisher {
	return &WotanPublisher{
		addr:       addr,
		batchSize:  batchSize,
		batchTime:  batchTimeout,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
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
		if _, err := tsc.Subscribe(ctx, topic, "trace-collector-go"); err != nil {
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
	} else {
		// HTTP fallback via Wotan REST API
		url := fmt.Sprintf("http://%s/api/v1/topics/%s/publish", wp.addr, topic)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Sender-ID", "trace-collector-go")

		resp, err := wp.httpClient.Do(req)
		if err != nil {
			wp.connected.Store(false)
			return fmt.Errorf("HTTP publish to %s: %w", topic, err)
		}
		resp.Body.Close()
	}

	atomic.AddUint64(&wp.published, uint64(len(events)))
	wp.connected.Store(true)

	batchLatency.Observe(time.Since(start).Seconds())
	eventsPublished.WithLabelValues(topic).Add(float64(len(events)))

	return nil
}

// PublishRaw publishes a raw JSON payload to a Wotan topic.
func (wp *WotanPublisher) PublishRaw(ctx context.Context, topic string, payload []byte) error {
	if wp.tsc != nil {
		return wp.tsc.Publish(ctx, topic, payload)
	}
	url := fmt.Sprintf("http://%s/api/v1/topics/%s/publish", wp.addr, topic)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := wp.httpClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
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
	json.NewEncoder(w).Encode(status)
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
func runUnifiedMode(ctx context.Context, healthSrv *transport.HealthServer) {
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
	pubConfig.WotanAddr = *wotanAddr
	pubConfig.WotanHTTPAddr = *wotanHTTPAddr
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
	var directLatencyMap interface{ Iterate() *ciliumebpf.MapIterator }
	if *enableLatencyProbe {
		progPath := fmt.Sprintf("%s/latency-probe", elfBase)
		kl, lmap, err := LoadLatencyKprobes(progPath)
		if err != nil {
			log.Error().Err(err).Str("path", progPath).Msg("failed to load latency kprobes via cilium/ebpf")
			// Fall back to standard loader
			if err := loader.Load(progPath); err != nil {
				log.Error().Err(err).Str("path", progPath).Msg("failed to load latency_probe (fallback)")
			} else {
				programs[2].Loaded = true
				programsLoaded.WithLabelValues("latency_probe").Set(1)
				log.Info().Msg("latency_probe loaded (fallback, single kprobe)")
			}
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

	// Start the trace reader (packet_marker maps)
	if *enablePacketMarker {
		readerConfig := DefaultTraceReaderConfig()
		readerConfig.ReadInterval = *readInterval
		traceReader := NewTraceReader(loader, NewWotanPublisher(*wotanHTTPAddr, *batchSize, *batchTimeout), readerConfig)
		state.TraceReader = traceReader

		go traceReader.Run(ctx)

		// Feed trace events into correlator and publisher
		go func() {
			for entry := range traceReader.Events() {
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
		wotanPub := NewWotanPublisher(*wotanHTTPAddr, *batchSize, *batchTimeout)
		flowReader := NewFlowReader(loader, wotanPub, flowConfig)

		go flowReader.Run(ctx)

		// Feed flow events into publisher (dashboard-compatible format)
		go func() {
			for ev := range flowReader.Events() {
				// Serialize in dashboard-backend FlowEvent format
				dashEvent := struct {
					TimestampNs uint64 `json:"timestamp_ns"`
					FlowKey     struct {
						SrcAddr  string `json:"src_addr"`
						DstAddr  string `json:"dst_addr"`
						SrcPort  uint16 `json:"src_port"`
						DstPort  uint16 `json:"dst_port"`
						Protocol uint8  `json:"protocol"`
					} `json:"flow_key"`
					FlowState struct {
						TraceID    struct{ High, Low uint64 } `json:"trace_id"`
						StartNs    uint64                     `json:"start_ns"`
						LastSeenNs uint64                     `json:"last_seen_ns"`
						PacketsIn  uint64                     `json:"packets_in"`
						PacketsOut uint64                     `json:"packets_out"`
						BytesIn    uint64                     `json:"bytes_in"`
						BytesOut   uint64                     `json:"bytes_out"`
						State      string                     `json:"state"`
					} `json:"flow_state"`
					EventType string `json:"event_type"`
				}{
					TimestampNs: ev.TimestampNS,
					EventType:   ev.EventTypeName(),
				}
				dashEvent.FlowKey.SrcAddr = ev.Key.SrcIP().String()
				dashEvent.FlowKey.DstAddr = ev.Key.DstIP().String()
				dashEvent.FlowKey.SrcPort = ev.Key.SrcPort
				dashEvent.FlowKey.DstPort = ev.Key.DstPort
				dashEvent.FlowKey.Protocol = ev.Key.Protocol
				dashEvent.FlowState.TraceID.High = binary.BigEndian.Uint64(ev.State.TraceID[0:8])
				dashEvent.FlowState.TraceID.Low = binary.BigEndian.Uint64(ev.State.TraceID[8:16])
				dashEvent.FlowState.StartNs = ev.State.StartNS
				dashEvent.FlowState.LastSeenNs = ev.State.LastSeenNS
				dashEvent.FlowState.PacketsIn = ev.State.PacketsIn
				dashEvent.FlowState.PacketsOut = ev.State.PacketsOut
				dashEvent.FlowState.BytesIn = ev.State.BytesIn
				dashEvent.FlowState.BytesOut = ev.State.BytesOut
				dashEvent.FlowState.State = strings.ToLower(ev.State.StateName())

				payload, err := json.Marshal(dashEvent)
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
			wotanPub := NewWotanPublisher(*wotanHTTPAddr, *batchSize, *batchTimeout)
			go func() {
				ticker := time.NewTicker(200 * time.Millisecond)
				defer ticker.Stop()
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

							// Convert BPF map entry to dashboard LatencyEvent format.
							// The map aggregates RTT per flow; we emit the latest RTT
							// as an individual event the dashboard can ingest.
							// Use current wall-clock time (not ktime) so the dashboard
							// latency histogram windows don't immediately expire the samples.
							evt := map[string]interface{}{
								"timestamp_ns": uint64(time.Now().UnixNano()),
								"latency_ns":   le.RTTNS,
								"operation":    "tcp_send",
								"pid":          uint32(0),
								"tid":          uint32(0),
								"trace_id":     map[string]uint64{"high": 0, "low": 0},
								"flow_key": map[string]interface{}{
									"src_addr": lk.SrcIPAddr().String(),
									"dst_addr": lk.DstIPAddr().String(),
									"src_port": lk.SrcPort,
									"dst_port": lk.DstPort,
									"protocol": lk.Protocol,
								},
								"min_rtt_ns":  le.MinRTTNS,
								"max_rtt_ns":  le.MaxRTTNS,
								"samples":     le.Samples,
							}
							payload, err := json.Marshal(evt)
							if err != nil {
								continue
							}
							wotanPub.PublishRaw(ctx, "traces.latency", payload)
							publisher.PublishLatencyEvent(payload)
						}
					}
				}
			}()
			log.Info().Msg("latency reader started (direct cilium/ebpf map)")
		} else {
			// Fallback: use standard loader's map reader
			latencyConfig := DefaultLatencyReaderConfig()
			wotanPub := NewWotanPublisher(*wotanHTTPAddr, *batchSize, *batchTimeout)
			latencyReader := NewLatencyReader(loader, wotanPub, latencyConfig)

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
	go publisher.Run(ctx)

	// Start HTTP server
	mux := http.NewServeMux()
	mux.Handle("/health", &HealthHandler{State: state})
	mux.Handle("/ready", &ReadyHandler{State: state})
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/api/v1/traces", &TracesHandler{State: state})
	mux.Handle("/api/v1/stats", &StatsHandler{State: state})

	// Auth middleware (activated via AUTH_ENABLED=true)
	unifiedAuthCfg := auth.LoadServiceAuthConfig("trace-collector")
	var unifiedHandler http.Handler = auth.WrapHandler(mux, auth.SetupMiddleware(unifiedAuthCfg))

	httpServer := &http.Server{
		Addr:         *metricsAddr,
		Handler:      unifiedHandler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1 MB
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
	httpServer.Shutdown(shutdownCtx)

	// Cleanup BPF programs
	if err := loader.Close(); err != nil {
		log.Error().Err(err).Msg("error closing BPF loader")
	}

	log.Info().Msg("unified trace-collector-go stopped")
}

// ── Legacy Anamnesis mode runner ────────────────────────────────────────

// runAnamnesisMode starts the original Anamnesis-based trace collector.
func runAnamnesisMode(ctx context.Context, healthSrv *transport.HealthServer) {
	log.Info().
		Str("ring_path", *ringPath).
		Str("wotan_addr", *wotanAddr).
		Str("http_addr", *httpAddr).
		Int("max_rate", *maxRate).
		Int("batch_size", *batchSize).
		Msg("trace-collector-go starting in anamnesis mode")

	// Create components
	publisher := NewWotanPublisher(*wotanHTTPAddr, *batchSize, *batchTimeout)

	// Connect via gRPC for reliable publishing
	if err := publisher.ConnectGRPC(ctx, *wotanAddr, *wotanHTTPAddr); err != nil {
		log.Warn().Err(err).Msg("gRPC connection failed, falling back to HTTP")
	} else {
		log.Info().Str("grpc", *wotanAddr).Msg("anamnesis publisher connected via gRPC")
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
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/healthz", &healthHandler{
		publisher:  publisher,
		correlator: correlator,
		reader:     reader,
	})
	healthSrv.RegisterHTTP(mux)

	// Auth middleware (activated via AUTH_ENABLED=true)
	anamnesisAuthCfg := auth.LoadServiceAuthConfig("trace-collector")
	var anamnesisHandler http.Handler = auth.WrapHandler(mux, auth.SetupMiddleware(anamnesisAuthCfg))

	httpServer := &http.Server{
		Addr:    *httpAddr,
		Handler: anamnesisHandler,
		MaxHeaderBytes: 1 << 20, // 1 MB
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
				removed := correlator.GC(uint64(time.Now().UnixNano()))
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
			httpServer.Shutdown(context.Background())
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

	// Configure logging
	if *logJSON {
		zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	} else {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
	}

	// Transport config: defaults → env → flag overrides
	transportCfg := transport.DefaultConfig()
	transport.ConfigFromEnv(&transportCfg)
	transportCfg.WotanGRPCAddr = *wotanAddr

	// Unified health server for transport-aware readiness
	healthSrv := transport.NewHealthServer("trace-collector")

	// Log aggregation publisher — forwards structured logs to Wotan
	var logConn transport.Connection
	if *wotanAddr != "" {
		logConn, _ = transport.Connect(context.Background(), transportCfg) // best-effort; nil on failure
	}
	logPublisher := logagg.NewPublisher("trace-collector", logConn)
	log.Logger = log.Logger.Hook(logPublisher)

	// Create context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Service discovery — best-effort registration (nil conn until transport is wired)
	discovery.SetupServiceDiscovery(ctx, nil, "trace-collector", 16670)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Info().Str("signal", sig.String()).Msg("shutdown signal received")
		cancel()
	}()

	if *unifiedMode {
		runUnifiedMode(ctx, healthSrv)
	} else {
		runAnamnesisMode(ctx, healthSrv)
	}
}
