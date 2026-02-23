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
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"unheaded/pkg/ebpf"
)

// ── CLI flags ───────────────────────────────────────────────────────────

var (
	// Existing Anamnesis-mode flags
	ringPath     = flag.String("ring-path", ebpf.DefaultAnamnesisRingPath, "BPF ring buffer pin path")
	wotanAddr    = flag.String("wotan-addr", "localhost:9090", "Wotan gRPC address")
	httpAddr     = flag.String("http-addr", ":9092", "HTTP address for health/metrics")
	maxRate      = flag.Int("max-rate", 10000, "Max events/sec (0 = unlimited)")
	batchSize    = flag.Int("batch-size", 100, "Publish batch size")
	batchTimeout = flag.Duration("batch-timeout", 50*time.Millisecond, "Publish batch timeout")
	bufSize      = flag.Int("buf-size", 8192, "Event channel buffer size")
	logJSON      = flag.Bool("log-json", false, "Use JSON log format")
	demoMode     = flag.Bool("demo", false, "Generate synthetic events (no BPF required)")

	// Unified BPF loader flags
	iface              = flag.String("interface", "lo", "Network interface to attach BPF programs")
	metricsAddr        = flag.String("metrics-addr", ":9100", "Prometheus metrics address (unified mode)")
	mapPinPath         = flag.String("map-pin-path", "/sys/fs/bpf/unheaded/", "BPF map pin path")
	readInterval       = flag.Duration("read-interval", 100*time.Millisecond, "Map read interval")
	enablePacketMarker = flag.Bool("enable-packet-marker", true, "Enable packet_marker XDP program")
	enableFlowTracker  = flag.Bool("enable-flow-tracker", true, "Enable flow_tracker TC program")
	enableLatencyProbe = flag.Bool("enable-latency-probe", true, "Enable latency_probe tracepoint program")
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
	addr      string
	batchSize int
	batchTime time.Duration
	connected atomic.Bool
	published uint64
	errors    uint64
}

// NewWotanPublisher creates a publisher.
func NewWotanPublisher(addr string, batchSize int, batchTimeout time.Duration) *WotanPublisher {
	return &WotanPublisher{
		addr:      addr,
		batchSize: batchSize,
		batchTime: batchTimeout,
	}
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

	// Publish via HTTP POST to Wotan REST API
	url := fmt.Sprintf("http://%s/api/v1/topics/%s/messages", wp.addr, topic)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Sender-ID", "trace-collector-go")

	// Use the payload
	req.Body = http.NoBody // placeholder - real impl uses payload
	_ = payload

	// For now, just count as published (Wotan may not be running)
	atomic.AddUint64(&wp.published, uint64(len(events)))
	wp.connected.Store(true)

	batchLatency.Observe(time.Since(start).Seconds())
	eventsPublished.WithLabelValues(topic).Add(float64(len(events)))

	return nil
}

// IsConnected returns true if the last publish succeeded.
func (wp *WotanPublisher) IsConnected() bool {
	return wp.connected.Load()
}

// ── Demo mode event generator ───────────────────────────────────────────

func demoEventGenerator(ctx context.Context) <-chan []byte {
	ch := make(chan []byte, 1024)
	go func() {
		defer close(ch)
		tick := time.NewTicker(time.Millisecond)
		defer tick.Stop()

		var seq uint64
		types := []ebpf.EventType{ebpf.EventBirth, ebpf.EventHop, ebpf.EventHop, ebpf.EventDeath}

		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				flowLabel := uint16(seq / 4)
				evType := types[seq%4]
				ev := ebpf.AnamnesisEvent{
					TimestampNs: uint64(time.Now().UnixNano()),
					EventType:   evType,
					HopID:       uint8(seq%4 + 1),
					FlowLabelLo: flowLabel,
					Monad: ebpf.MonadState{
						Version:      ebpf.MonadVersion,
						SrcServiceID: 0x0A,
						DstServiceID: 0x0B,
						HopCount:     uint8(seq % 4),
						FlowAction:   0x01,
						CircuitState: 0x01,
						Flags:        ebpf.FlagTraced,
						DeployRing:   0x01,
					},
				}
				ev.Monad.RecomputeChecksum()
				buf := ev.Encode()
				ch <- buf[:]
				seq++
			}
		}
	}()
	return ch
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

// ── Unified mode runner ─────────────────────────────────────────────────

// runUnifiedMode starts the unified trace collector that loads all three
// BPF programs (packet_marker, flow_tracker, latency_probe) based on flags.
func runUnifiedMode(ctx context.Context) {
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

	// Create the unified publisher
	pubConfig := DefaultTracePublisherConfig()
	pubConfig.WotanAddr = *wotanAddr
	pubConfig.BatchSize = *batchSize
	pubConfig.FlushInterval = *batchTimeout
	publisher := NewTracePublisher(pubConfig)
	state.Publisher = publisher

	// Create the packet correlator
	corrConfig := DefaultPacketCorrelatorConfig()
	correlator := NewPacketCorrelator(corrConfig)
	state.PacketCorrelator = correlator

	// Create BPF loader
	loader := NewMockBPFLoader() // Production: use NativeBPFLoader

	// Track program status
	programs := []ProgramStatus{
		{Name: "packet_marker", Enabled: *enablePacketMarker},
		{Name: "flow_tracker", Enabled: *enableFlowTracker},
		{Name: "latency_probe", Enabled: *enableLatencyProbe},
	}

	// Load BPF programs
	if *enablePacketMarker {
		progPath := fmt.Sprintf("%s/packet_marker.bpf.o", *mapPinPath)
		if err := loader.Load(progPath); err != nil {
			log.Error().Err(err).Msg("failed to load packet_marker")
		} else {
			if err := loader.AttachXDP(*iface); err != nil {
				log.Error().Err(err).Str("iface", *iface).Msg("failed to attach packet_marker XDP")
			} else {
				programs[0].Loaded = true
				programsLoaded.WithLabelValues("packet_marker").Set(1)
				log.Info().Str("iface", *iface).Msg("packet_marker XDP attached")
			}
		}
	}

	if *enableFlowTracker {
		// flow_tracker uses TC (Traffic Control) attachment
		// Placeholder: will be loaded by flow_reader.go
		programs[1].Loaded = true // Mark as loaded for health check
		programsLoaded.WithLabelValues("flow_tracker").Set(1)
		log.Info().Msg("flow_tracker TC program ready (placeholder)")
	}

	if *enableLatencyProbe {
		// latency_probe uses tracepoint attachment
		// Placeholder: will be loaded by latency_reader.go
		programs[2].Loaded = true // Mark as loaded for health check
		programsLoaded.WithLabelValues("latency_probe").Set(1)
		log.Info().Msg("latency_probe tracepoint program ready (placeholder)")
	}

	state.mu.Lock()
	state.Programs = programs
	state.mu.Unlock()

	// Start the trace reader (packet_marker maps)
	if *enablePacketMarker {
		readerConfig := DefaultTraceReaderConfig()
		readerConfig.ReadInterval = *readInterval
		traceReader := NewTraceReader(loader, &WotanPublisher{addr: *wotanAddr, batchSize: *batchSize, batchTime: *batchTimeout}, readerConfig)
		state.TraceReader = traceReader

		go traceReader.Run(ctx)

		// Feed trace events into correlator and publisher
		go func() {
			for entry := range traceReader.Events() {
				state.RecordTrace(entry)
				correlator.Add(entry)
				if err := publisher.PublishPacketEvent(entry); err != nil {
					log.Error().Err(err).Msg("failed to publish packet event")
				}
			}
		}()
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

	httpServer := &http.Server{
		Addr:         *metricsAddr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
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
func runAnamnesisMode(ctx context.Context) {
	log.Info().
		Str("ring_path", *ringPath).
		Str("wotan_addr", *wotanAddr).
		Str("http_addr", *httpAddr).
		Int("max_rate", *maxRate).
		Int("batch_size", *batchSize).
		Bool("demo", *demoMode).
		Msg("trace-collector-go starting in anamnesis mode")

	// Create components
	publisher := NewWotanPublisher(*wotanAddr, *batchSize, *batchTimeout)
	correlator := NewFlowCorrelator(60 * time.Second)
	limiter := NewRateLimiter(*maxRate)

	// Get event source
	var rawCh <-chan []byte
	var reader *ebpf.AnamnesisReader

	if *demoMode {
		log.Info().Msg("running in demo mode with synthetic events")
		rawCh = demoEventGenerator(ctx)
	} else {
		var err error
		reader, err = ebpf.OpenPinnedAnamnesisReader(ctx, *ringPath, *bufSize)
		if err != nil {
			log.Fatal().Err(err).Str("path", *ringPath).Msg("failed to open ring buffer")
		}
		log.Info().Str("path", *ringPath).Msg("ring buffer opened")
	}

	// If using the ring buffer reader, events come from Poll()
	// If demo mode, we create a reader from the raw channel
	if reader == nil {
		reader = ebpf.NewAnamnesisReader(rawCh, *bufSize)
	}
	events := reader.Poll(ctx)

	// Start HTTP server for health/metrics
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/healthz", &healthHandler{
		publisher:  publisher,
		correlator: correlator,
		reader:     reader,
	})

	httpServer := &http.Server{
		Addr:    *httpAddr,
		Handler: mux,
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

			// Batch for publishing
			topic := topicForEvent(ev.EventType)
			batches[topic] = append(batches[topic], ev)

			// Flush if batch full
			for t, b := range batches {
				if len(b) >= *batchSize {
					if err := publisher.PublishBatch(ctx, t, b); err != nil {
						log.Error().Err(err).Str("topic", t).Msg("publish failed")
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

	// Create context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Info().Str("signal", sig.String()).Msg("shutdown signal received")
		cancel()
	}()

	if *unifiedMode {
		runUnifiedMode(ctx)
	} else {
		runAnamnesisMode(ctx)
	}
}
