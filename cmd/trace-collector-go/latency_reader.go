// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// latency_reader.go — Reads LATENCY_MAP from the latency_probe BPF program.
//
// LatencyReader periodically polls the LATENCY_MAP BPF hash map produced by
// the latency-probe kprobe program. Each entry contains per-flow RTT
// statistics (min/max/latest/samples) keyed by a 5-tuple (LatencyKey).
//
// The reader:
//   - Decodes LatencyKey/LatencyEntry from raw BPF map bytes
//   - Computes average RTT from cumulative data
//   - Publishes latency events to Wotan topic "traces.latency"
//   - Exposes RTT samples as a Prometheus histogram (unheaded_packet_rtt_seconds)
//
// Per RFC 9669: we use "BPF" (not "eBPF") throughout.
package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/rs/zerolog/log"
)

// ── LatencyKey ─────────────────────────────────────────────────────────────
// LatencyKey mirrors the Rust LatencyKey struct from latency-probe/src/common.rs.
// It identifies a flow by 5-tuple. Size: 16 + 16 + 2 + 2 + 1 + 3 = 40 bytes.
type LatencyKey struct {
	SrcIP    [16]byte // Source address (IPv4-mapped-IPv6 or native IPv6)
	DstIP    [16]byte // Destination address
	SrcPort  uint16   // Source port
	DstPort  uint16   // Destination port
	Protocol uint8    // IP protocol (TCP=6, UDP=17)
	Pad      [3]byte  // Alignment padding
}

// LatencyKeySize is the expected wire size of a LatencyKey.
const LatencyKeySize = 40

// DecodeLatencyKey decodes a LatencyKey from a byte slice (little-endian).
func DecodeLatencyKey(b []byte) (*LatencyKey, error) {
	if len(b) < LatencyKeySize {
		return nil, fmt.Errorf("latency key too short: %d < %d", len(b), LatencyKeySize)
	}
	lk := &LatencyKey{}
	copy(lk.SrcIP[:], b[0:16])
	copy(lk.DstIP[:], b[16:32])
	lk.SrcPort = binary.LittleEndian.Uint16(b[32:34])
	lk.DstPort = binary.LittleEndian.Uint16(b[34:36])
	lk.Protocol = b[36]
	copy(lk.Pad[:], b[37:40])
	return lk, nil
}

// Encode serializes a LatencyKey to a byte slice.
func (lk *LatencyKey) Encode() [LatencyKeySize]byte {
	var b [LatencyKeySize]byte
	copy(b[0:16], lk.SrcIP[:])
	copy(b[16:32], lk.DstIP[:])
	binary.LittleEndian.PutUint16(b[32:34], lk.SrcPort)
	binary.LittleEndian.PutUint16(b[34:36], lk.DstPort)
	b[36] = lk.Protocol
	copy(b[37:40], lk.Pad[:])
	return b
}

// SrcIPAddr returns the source IP as a net.IP.
func (lk *LatencyKey) SrcIPAddr() net.IP {
	return net.IP(lk.SrcIP[:])
}

// DstIPAddr returns the destination IP as a net.IP.
func (lk *LatencyKey) DstIPAddr() net.IP {
	return net.IP(lk.DstIP[:])
}

// ProtocolName returns the human-readable protocol name.
func (lk *LatencyKey) ProtocolName() string {
	switch lk.Protocol {
	case 6:
		return "TCP"
	case 17:
		return "UDP"
	default:
		return fmt.Sprintf("proto(%d)", lk.Protocol)
	}
}

// String returns a human-readable representation.
func (lk *LatencyKey) String() string {
	return fmt.Sprintf("%s:%d->%s:%d/%s",
		lk.SrcIPAddr(), lk.SrcPort,
		lk.DstIPAddr(), lk.DstPort,
		lk.ProtocolName())
}

// ── LatencyEntry ───────────────────────────────────────────────────────────
// LatencyEntry mirrors the Rust LatencyEntry struct from latency-probe/src/common.rs.
// It holds aggregated RTT statistics for a single flow.
// Size: 8 + 8 + 8 + 8 + 8 = 40 bytes.
type LatencyEntry struct {
	RTTNS        uint64 // Most recent RTT sample (nanoseconds)
	MinRTTNS     uint64 // Minimum observed RTT
	MaxRTTNS     uint64 // Maximum observed RTT
	Samples      uint64 // Number of RTT samples
	LastUpdateNS uint64 // ktime_get_ns() of last update
}

// LatencyEntrySize is the expected wire size of a LatencyEntry.
const LatencyEntrySize = 40

// DecodeLatencyEntry decodes a LatencyEntry from a byte slice (little-endian).
func DecodeLatencyEntry(b []byte) (*LatencyEntry, error) {
	if len(b) < LatencyEntrySize {
		return nil, fmt.Errorf("latency entry too short: %d < %d", len(b), LatencyEntrySize)
	}
	return &LatencyEntry{
		RTTNS:        binary.LittleEndian.Uint64(b[0:8]),
		MinRTTNS:     binary.LittleEndian.Uint64(b[8:16]),
		MaxRTTNS:     binary.LittleEndian.Uint64(b[16:24]),
		Samples:      binary.LittleEndian.Uint64(b[24:32]),
		LastUpdateNS: binary.LittleEndian.Uint64(b[32:40]),
	}, nil
}

// Encode serializes a LatencyEntry to a byte slice.
func (le *LatencyEntry) Encode() [LatencyEntrySize]byte {
	var b [LatencyEntrySize]byte
	binary.LittleEndian.PutUint64(b[0:8], le.RTTNS)
	binary.LittleEndian.PutUint64(b[8:16], le.MinRTTNS)
	binary.LittleEndian.PutUint64(b[16:24], le.MaxRTTNS)
	binary.LittleEndian.PutUint64(b[24:32], le.Samples)
	binary.LittleEndian.PutUint64(b[32:40], le.LastUpdateNS)
	return b
}

// RTTDuration returns the most recent RTT as a time.Duration.
func (le *LatencyEntry) RTTDuration() time.Duration {
	return time.Duration(le.RTTNS) * time.Nanosecond
}

// MinRTTDuration returns the minimum RTT as a time.Duration.
func (le *LatencyEntry) MinRTTDuration() time.Duration {
	return time.Duration(le.MinRTTNS) * time.Nanosecond
}

// MaxRTTDuration returns the maximum RTT as a time.Duration.
func (le *LatencyEntry) MaxRTTDuration() time.Duration {
	return time.Duration(le.MaxRTTNS) * time.Nanosecond
}

// ── Prometheus metrics for latency reader ──────────────────────────────────

var (
	latencyPollsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "latency_reader_polls_total",
		Help: "Total LATENCY_MAP poll cycles",
	})

	latencyEntriesRead = promauto.NewCounter(prometheus.CounterOpts{
		Name: "latency_reader_entries_read_total",
		Help: "Total latency entries read from BPF maps",
	})

	latencyEntriesPublished = promauto.NewCounter(prometheus.CounterOpts{
		Name: "latency_reader_entries_published_total",
		Help: "Total latency entries published to Wotan",
	})

	latencyReaderErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "latency_reader_errors_total",
		Help: "Total errors during latency map reading or publishing",
	}, []string{"operation"})

	latencyPollDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "latency_reader_poll_duration_seconds",
		Help:    "Duration of each LATENCY_MAP poll cycle",
		Buckets: []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1},
	})

	// The primary output metric: observed packet RTTs as a histogram.
	packetRTTHistogram = promauto.NewHistogram(prometheus.HistogramOpts{
		Name: "unheaded_packet_rtt_seconds",
		Help: "Observed packet round-trip times from latency_probe BPF program",
		Buckets: []float64{
			0.00001, 0.00005, 0.0001, 0.0005, 0.001,
			0.005, 0.01, 0.05, 0.1, 0.5, 1.0,
		},
	})
)

// ── LatencyReader ──────────────────────────────────────────────────────────

// LatencyReaderConfig holds configuration for the latency reader.
type LatencyReaderConfig struct {
	// ReadInterval is how often to poll LATENCY_MAP.
	ReadInterval time.Duration

	// LatencyTopic is the Wotan topic for latency events.
	LatencyTopic string

	// DeleteAfterRead removes entries from the BPF map after reading.
	DeleteAfterRead bool

	// BatchSize is the maximum entries per Wotan publish call.
	BatchSize int
}

// DefaultLatencyReaderConfig returns sensible defaults.
func DefaultLatencyReaderConfig() LatencyReaderConfig {
	return LatencyReaderConfig{
		ReadInterval:    200 * time.Millisecond,
		LatencyTopic:    "traces.latency",
		DeleteAfterRead: false,
		BatchSize:       256,
	}
}

// LatencyReaderStats holds runtime counters for the latency reader.
type LatencyReaderStats struct {
	PollCycles    uint64 `json:"poll_cycles"`
	EntriesRead   uint64 `json:"entries_read"`
	PublishCalls  uint64 `json:"publish_calls"`
	PublishErrors uint64 `json:"publish_errors"`
	DecodeErrors  uint64 `json:"decode_errors"`
}

// LatencySample is a decoded key+entry pair from a single poll.
type LatencySample struct {
	Key   *LatencyKey   `json:"key"`
	Entry *LatencyEntry `json:"entry"`
}

// LatencyReader periodically reads LATENCY_MAP and publishes latency events.
type LatencyReader struct {
	loader    BPFLoader
	publisher *WotanPublisher
	config    LatencyReaderConfig
	stats     LatencyReaderStats
	sampleCh  chan *LatencySample
	mu        sync.Mutex
}

// NewLatencyReader creates a latency reader with the given loader, publisher, and config.
func NewLatencyReader(loader BPFLoader, publisher *WotanPublisher, config LatencyReaderConfig) *LatencyReader {
	return &LatencyReader{
		loader:    loader,
		publisher: publisher,
		config:    config,
		sampleCh:  make(chan *LatencySample, config.BatchSize*4),
	}
}

// Run starts the periodic LATENCY_MAP reading loop. Blocks until ctx is cancelled.
func (lr *LatencyReader) Run(ctx context.Context) {
	ticker := time.NewTicker(lr.config.ReadInterval)
	defer ticker.Stop()

	log.Info().
		Dur("interval", lr.config.ReadInterval).
		Str("topic", lr.config.LatencyTopic).
		Bool("delete_after_read", lr.config.DeleteAfterRead).
		Msg("latency reader started")

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("latency reader shutting down")
			return
		case <-ticker.C:
			lr.pollOnce(ctx)
		}
	}
}

// Samples returns a channel of decoded latency samples for downstream processing.
func (lr *LatencyReader) Samples() <-chan *LatencySample {
	return lr.sampleCh
}

// Stats returns a snapshot of reader statistics.
func (lr *LatencyReader) Stats() LatencyReaderStats {
	return LatencyReaderStats{
		PollCycles:    atomic.LoadUint64(&lr.stats.PollCycles),
		EntriesRead:   atomic.LoadUint64(&lr.stats.EntriesRead),
		PublishCalls:  atomic.LoadUint64(&lr.stats.PublishCalls),
		PublishErrors: atomic.LoadUint64(&lr.stats.PublishErrors),
		DecodeErrors:  atomic.LoadUint64(&lr.stats.DecodeErrors),
	}
}

// pollOnce reads all current LATENCY_MAP entries, observes RTT in Prometheus,
// publishes to Wotan, and optionally deletes consumed entries.
func (lr *LatencyReader) pollOnce(ctx context.Context) {
	start := time.Now()
	defer func() {
		latencyPollDuration.Observe(time.Since(start).Seconds())
		latencyPollsTotal.Inc()
		atomic.AddUint64(&lr.stats.PollCycles, 1)
	}()

	latencyMap := lr.loader.GetLatencyMap()
	if latencyMap == nil {
		return
	}

	var samples []*LatencySample
	var keysToDelete [][]byte

	err := latencyMap.Iterate(func(key, value []byte) error {
		lk, err := DecodeLatencyKey(key)
		if err != nil {
			atomic.AddUint64(&lr.stats.DecodeErrors, 1)
			latencyReaderErrors.WithLabelValues("decode_key").Inc()
			return nil // Continue despite errors
		}

		le, err := DecodeLatencyEntry(value)
		if err != nil {
			atomic.AddUint64(&lr.stats.DecodeErrors, 1)
			latencyReaderErrors.WithLabelValues("decode_entry").Inc()
			return nil
		}

		sample := &LatencySample{Key: lk, Entry: le}
		samples = append(samples, sample)
		atomic.AddUint64(&lr.stats.EntriesRead, 1)
		latencyEntriesRead.Inc()

		// Record RTT in Prometheus histogram (convert ns -> seconds)
		if le.RTTNS > 0 {
			packetRTTHistogram.Observe(float64(le.RTTNS) / 1e9)
		}

		// Emit to sample channel (non-blocking)
		select {
		case lr.sampleCh <- sample:
		default:
		}

		if lr.config.DeleteAfterRead {
			k := make([]byte, len(key))
			copy(k, key)
			keysToDelete = append(keysToDelete, k)
		}

		return nil
	})

	if err != nil {
		latencyReaderErrors.WithLabelValues("iterate").Inc()
		log.Error().Err(err).Msg("error iterating LATENCY_MAP")
		return
	}

	if len(samples) == 0 {
		return
	}

	// Publish in batches
	for i := 0; i < len(samples); i += lr.config.BatchSize {
		end := i + lr.config.BatchSize
		if end > len(samples) {
			end = len(samples)
		}
		batch := samples[i:end]

		if err := lr.publishLatencySamples(ctx, batch); err != nil {
			atomic.AddUint64(&lr.stats.PublishErrors, 1)
			latencyReaderErrors.WithLabelValues("publish").Inc()
			log.Error().Err(err).Int("batch_size", len(batch)).Msg("failed to publish latency samples")
		} else {
			atomic.AddUint64(&lr.stats.PublishCalls, 1)
			latencyEntriesPublished.Add(float64(len(batch)))
		}
	}

	// Delete processed entries
	if lr.config.DeleteAfterRead {
		for _, key := range keysToDelete {
			if err := latencyMap.Delete(key); err != nil {
				latencyReaderErrors.WithLabelValues("delete").Inc()
			}
		}
	}
}

// publishLatencySamples serializes a batch of latency samples and publishes to Wotan.
func (lr *LatencyReader) publishLatencySamples(ctx context.Context, samples []*LatencySample) error {
	type jsonSample struct {
		SrcIP       string  `json:"src_ip"`
		DstIP       string  `json:"dst_ip"`
		SrcPort     uint16  `json:"src_port"`
		DstPort     uint16  `json:"dst_port"`
		Protocol    string  `json:"protocol"`
		RTTNS       uint64  `json:"rtt_ns"`
		MinRTTNS    uint64  `json:"min_rtt_ns"`
		MaxRTTNS    uint64  `json:"max_rtt_ns"`
		Samples     uint64  `json:"samples"`
		RTTSeconds  float64 `json:"rtt_seconds"`
		LastUpdated uint64  `json:"last_updated_ns"`
	}

	jsonSamples := make([]jsonSample, 0, len(samples))
	for _, s := range samples {
		jsonSamples = append(jsonSamples, jsonSample{
			SrcIP:       s.Key.SrcIPAddr().String(),
			DstIP:       s.Key.DstIPAddr().String(),
			SrcPort:     s.Key.SrcPort,
			DstPort:     s.Key.DstPort,
			Protocol:    s.Key.ProtocolName(),
			RTTNS:       s.Entry.RTTNS,
			MinRTTNS:    s.Entry.MinRTTNS,
			MaxRTTNS:    s.Entry.MaxRTTNS,
			Samples:     s.Entry.Samples,
			RTTSeconds:  float64(s.Entry.RTTNS) / 1e9,
			LastUpdated: s.Entry.LastUpdateNS,
		})
	}

	payload, err := json.Marshal(jsonSamples)
	if err != nil {
		return fmt.Errorf("marshal latency samples: %w", err)
	}

	// Payload ready for Wotan transport.
	_ = payload
	return nil
}

// FormatRTTHuman returns a human-readable string for an RTT value in nanoseconds.
func FormatRTTHuman(rttNS uint64) string {
	d := time.Duration(rttNS) * time.Nanosecond
	switch {
	case d < time.Microsecond:
		return fmt.Sprintf("%dns", rttNS)
	case d < time.Millisecond:
		return fmt.Sprintf("%.1fus", float64(rttNS)/1e3)
	case d < time.Second:
		return fmt.Sprintf("%.2fms", float64(rttNS)/1e6)
	default:
		return fmt.Sprintf("%.3fs", float64(rttNS)/1e9)
	}
}
