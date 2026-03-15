// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
// cmd/ebpf-exporter/main.go
//
// eBPF → Prometheus exporter for Unheaded Kingdom.
// Reads BPF ring buffers and maps from Shield + Monad programs,
// exposes Prometheus metrics on :9435/metrics.
//
// Architecture:
//   BPF ring buffer (kernel) → ringbuf.Reader → MetricCollector → /metrics
//   BPF hash map (per-flow latency) → polling goroutine → gauge vec
//
// BPF map conventions (must match shield.bpf.c / monad.bpf.c):
//   unheaded_xdp_latency_ns   - histogram map: flow_id → [latency_ns]
//   unheaded_pkt_count        - hash map: action → count
//   unheaded_ring_events      - ring buffer: MonadEvent structs
//   unheaded_drop_count       - array map: [0] = total drops

package main

import (
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// ── CLI flags ──────────────────────────────────────────────────────────────

var (
	flagPort     = flag.Int("port", 9435, "HTTP metrics port")
	flagBPFPath  = flag.String("bpf-path", "/opt/unheaded/ebpf", "BPF object files directory")
	flagInterval = flag.Duration("interval", time.Second, "BPF map polling interval")
	flagLogLevel = flag.String("log-level", "info", "Log level: debug|info|warn|error")
)

// ── BPF map names (must match kernel-side C definitions) ──────────────────

const (
	mapXDPLatency  = "unheaded_xdp_latency_ns"
	mapPktCount    = "unheaded_pkt_count"
	mapRingEvents  = "unheaded_ring_events"
	mapDropCount   = "unheaded_drop_count"
)

// ── MonadEvent mirrors the kernel struct in monad.bpf.c ───────────────────
// Must be kept in sync with:
//   struct monad_event { u64 ts_ns; u32 flow_id; u16 action; u8 hop; u8 pad; u32 latency_ns; };

type MonadEvent struct {
	TsNs      uint64
	FlowID    uint32
	Action    uint16
	Hop       uint8
	Pad       uint8
	LatencyNs uint32
}

const monandEventSize = int(unsafe.Sizeof(MonadEvent{}))

// ── XDP action labels ─────────────────────────────────────────────────────

var xdpActions = map[uint32]string{
	0: "aborted",
	1: "drop",
	2: "pass",
	3: "tx",
	4: "redirect",
}

// ── Prometheus metric definitions ─────────────────────────────────────────

var (
	// Packet counts by XDP action
	xdpPacketsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "unheaded",
			Subsystem: "xdp",
			Name:      "packets_total",
			Help:      "Total XDP-processed packets by action.",
		},
		[]string{"action"},
	)

	// Per-flow latency histogram (ns)
	flowLatencyNs = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "unheaded",
			Subsystem: "flow",
			Name:      "latency_ns",
			Help:      "Per-flow packet latency in nanoseconds (sampled from BPF ring buffer).",
			Buckets:   []float64{100, 500, 1000, 2500, 5000, 10000, 25000, 50000, 100000, 250000},
		},
		[]string{"hop"},
	)

	// Total drop count (from BPF array map)
	dropTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "unheaded",
			Subsystem: "xdp",
			Name:      "drops_total",
			Help:      "Total XDP-dropped packets (policy enforcement).",
		},
	)

	// Ring buffer event rate
	ringEventsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "unheaded",
			Subsystem: "ring",
			Name:      "events_total",
			Help:      "Total events consumed from the Monad BPF ring buffer.",
		},
	)

	// BPF map error counter
	bpfMapErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "unheaded",
			Subsystem: "ebpf_exporter",
			Name:      "map_errors_total",
			Help:      "Errors reading BPF maps.",
		},
		[]string{"map", "error"},
	)

	// Exporter health gauge
	exporterUp = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "unheaded",
			Subsystem: "ebpf_exporter",
			Name:      "up",
			Help:      "1 if the eBPF exporter is running and connected to BPF maps, 0 otherwise.",
		},
	)
)

// ── BPFCollector wraps BPF map access ─────────────────────────────────────

type BPFCollector struct {
	log         *slog.Logger
	interval    time.Duration
	bpfPath     string

	// BPF objects (loaded from pinned maps or BPF_OBJ_GET)
	pktCountMap  *ebpf.Map
	dropCountMap *ebpf.Map
	ringReader   *ringbuf.Reader
}

func NewBPFCollector(bpfPath string, interval time.Duration, log *slog.Logger) (*BPFCollector, error) {
	c := &BPFCollector{
		log:      log,
		interval: interval,
		bpfPath:  bpfPath,
	}
	
	if err := c.loadMaps(); err != nil {
		return nil, fmt.Errorf("loading BPF maps: %w", err)
	}
	
	return c, nil
}

// loadMaps opens pinned BPF maps from the filesystem.
// Maps are pinned by the Shield/Monad eBPF programs when they load.
func (c *BPFCollector) loadMaps() error {
	var err error
	
	pktPath := fmt.Sprintf("%s/pinned/%s", c.bpfPath, mapPktCount)
	c.pktCountMap, err = ebpf.LoadPinnedMap(pktPath, nil)
	if err != nil {
		return fmt.Errorf("loading %s from %s: %w", mapPktCount, pktPath, err)
	}
	
	dropPath := fmt.Sprintf("%s/pinned/%s", c.bpfPath, mapDropCount)
	c.dropCountMap, err = ebpf.LoadPinnedMap(dropPath, nil)
	if err != nil {
		return fmt.Errorf("loading %s from %s: %w", mapDropCount, dropPath, err)
	}
	
	ringPath := fmt.Sprintf("%s/pinned/%s", c.bpfPath, mapRingEvents)
	ringMap, err := ebpf.LoadPinnedMap(ringPath, nil)
	if err != nil {
		return fmt.Errorf("loading %s from %s: %w", mapRingEvents, ringPath, err)
	}
	
	c.ringReader, err = ringbuf.NewReader(ringMap)
	if err != nil {
		return fmt.Errorf("creating ring buffer reader: %w", err)
	}
	
	c.log.Info("BPF maps loaded", "path", c.bpfPath)
	return nil
}

// PollMaps reads BPF hash maps on the configured interval.
// This goroutine runs for the lifetime of the program.
func (c *BPFCollector) PollMaps(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.collectPktCounts()
			c.collectDropCount()
		}
	}
}

func (c *BPFCollector) collectPktCounts() {
	var key, value uint32
	iter := c.pktCountMap.Iterate()
	for iter.Next(&key, &value) {
		action, ok := xdpActions[key]
		if !ok {
			action = fmt.Sprintf("unknown_%d", key)
		}
		// CounterVec.With() is idempotent for the same label set
		xdpPacketsTotal.With(prometheus.Labels{"action": action}).Add(float64(value))
	}
	if err := iter.Err(); err != nil {
		c.log.Warn("iterating pkt_count map", "error", err)
		bpfMapErrors.With(prometheus.Labels{"map": mapPktCount, "error": err.Error()}).Inc()
	}
}

func (c *BPFCollector) collectDropCount() {
	var idx, drops uint32
	if err := c.dropCountMap.Lookup(&idx, &drops); err != nil {
		c.log.Debug("drop count lookup", "error", err)
		return
	}
	dropTotal.Add(float64(drops))
}

// ConsumeRingBuffer reads MonadEvent structs from the BPF ring buffer.
// Each event contributes to latency histograms.
func (c *BPFCollector) ConsumeRingBuffer(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		
		record, err := c.ringReader.Read()
		if err != nil {
			if err == ringbuf.ErrClosed {
				return
			}
			c.log.Warn("ring buffer read error", "error", err)
			bpfMapErrors.With(prometheus.Labels{"map": mapRingEvents, "error": err.Error()}).Inc()
			continue
		}
		
		if len(record.RawSample) < monandEventSize {
			c.log.Warn("ring buffer short read", "got", len(record.RawSample), "want", monandEventSize)
			continue
		}
		
		var ev MonadEvent
		ev.TsNs      = binary.LittleEndian.Uint64(record.RawSample[0:8])
		ev.FlowID    = binary.LittleEndian.Uint32(record.RawSample[8:12])
		ev.Action    = binary.LittleEndian.Uint16(record.RawSample[12:14])
		ev.Hop       = record.RawSample[14]
		ev.Pad       = record.RawSample[15]
		ev.LatencyNs = binary.LittleEndian.Uint32(record.RawSample[16:20])
		
		hop := fmt.Sprintf("%d", ev.Hop)
		flowLatencyNs.With(prometheus.Labels{"hop": hop}).Observe(float64(ev.LatencyNs))
		ringEventsTotal.Inc()
	}
}

// ── main ──────────────────────────────────────────────────────────────────

func main() {
	flag.Parse()
	
	// Logger
	level := slog.LevelInfo
	switch *flagLogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	
	// Prometheus registry
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		xdpPacketsTotal,
		flowLatencyNs,
		dropTotal,
		ringEventsTotal,
		bpfMapErrors,
		exporterUp,
	)
	
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	
	// BPF collector
	collector, err := NewBPFCollector(*flagBPFPath, *flagInterval, log)
	if err != nil {
		log.Warn("BPF maps not available (bare metal required)", "error", err)
		exporterUp.Set(0)
		// Serve empty metrics — don't exit, let Prometheus see we're up but degraded
	} else {
		exporterUp.Set(1)
		// Start background workers
		go collector.PollMaps(ctx)
		go collector.ConsumeRingBuffer(ctx)
		log.Info("BPF collector started", "path", *flagBPFPath, "interval", *flagInterval)
	}
	
	// HTTP server
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	}))
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","component":"ebpf-exporter"}`))
	})
	
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", *flagPort),
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	
	go func() {
		log.Info("HTTP metrics server starting", "port", *flagPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("HTTP server error", "error", err)
			cancel()
		}
	}()
	
	<-ctx.Done()
	log.Info("Shutting down")
	_ = srv.Close()
}
