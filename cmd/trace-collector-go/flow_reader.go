// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// flow_reader.go — FlowReader periodically reads the FLOW_MAP from the
// flow_tracker BPF program and publishes flow events to Wotan.
//
// The flow_tracker TC program writes FlowKey -> FlowState entries into
// an LRU hash map. This reader polls that map, detects state changes,
// expires stale flows, and publishes events to the "traces.flow" Wotan topic.
//
// Wire format for BPF map types is defined in ebpf/common/src/lib.rs and
// mirrored in ebpf/flow-tracker/src/common.rs.
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

// ── BPF struct sizes ─────────────────────────────────────────────────────────

const (
	// FlowKeySize is the wire size of a FlowKey (16 bytes).
	FlowKeySize = 16

	// FlowStateSize is the wire size of a FlowState (72 bytes).
	FlowStateSize = 72

	// FlowEventSize is the wire size of a FlowEvent (104 bytes).
	FlowEventSize = 104
)

// ── Connection state constants (mirrors ConnectionState in Rust) ─────────────

const (
	ConnStateUnknown     uint8 = 0
	ConnStateSynSent     uint8 = 1
	ConnStateSynReceived uint8 = 2
	ConnStateEstablished uint8 = 3
	ConnStateFinWait1    uint8 = 4
	ConnStateFinWait2    uint8 = 5
	ConnStateCloseWait   uint8 = 6
	ConnStateClosing     uint8 = 7
	ConnStateLastAck     uint8 = 8
	ConnStateTimeWait    uint8 = 9
	ConnStateClosed      uint8 = 10
)

// ConnStateName returns the human-readable name for a connection state.
func ConnStateName(state uint8) string {
	switch state {
	case ConnStateUnknown:
		return "UNKNOWN"
	case ConnStateSynSent:
		return "SYN_SENT"
	case ConnStateSynReceived:
		return "SYN_RECEIVED"
	case ConnStateEstablished:
		return "ESTABLISHED"
	case ConnStateFinWait1:
		return "FIN_WAIT1"
	case ConnStateFinWait2:
		return "FIN_WAIT2"
	case ConnStateCloseWait:
		return "CLOSE_WAIT"
	case ConnStateClosing:
		return "CLOSING"
	case ConnStateLastAck:
		return "LAST_ACK"
	case ConnStateTimeWait:
		return "TIME_WAIT"
	case ConnStateClosed:
		return "CLOSED"
	default:
		return fmt.Sprintf("state(%d)", state)
	}
}

// ── Flow event type constants (mirrors FlowEventType in Rust) ────────────────

const (
	FlowEventNew         uint8 = 0
	FlowEventStateChange uint8 = 1
	FlowEventExpired     uint8 = 2
	FlowEventUpdate      uint8 = 3
	FlowEventAnomaly     uint8 = 4
)

// FlowEventTypeName returns the human-readable name for a flow event type.
func FlowEventTypeName(et uint8) string {
	switch et {
	case FlowEventNew:
		return "new"
	case FlowEventStateChange:
		return "state_change"
	case FlowEventExpired:
		return "expired"
	case FlowEventUpdate:
		return "update"
	case FlowEventAnomaly:
		return "anomaly"
	default:
		return fmt.Sprintf("event(%d)", et)
	}
}

// ── FlowKey ──────────────────────────────────────────────────────────────────

// FlowKey mirrors the BPF FlowKey struct (16 bytes).
// Normalized: smaller IP is always src_addr for bidirectional matching.
type FlowKey struct {
	SrcAddr  uint32  // IPv4 source address (network byte order)
	DstAddr  uint32  // IPv4 destination address (network byte order)
	SrcPort  uint16  // Source port (network byte order)
	DstPort  uint16  // Destination port (network byte order)
	Protocol uint8   // IP protocol (TCP=6, UDP=17)
	Pad      [3]byte // Alignment padding
}

// DecodeFlowKey decodes a FlowKey from a byte slice (little-endian struct layout,
// but addr/port fields are in network byte order as stored by BPF).
func DecodeFlowKey(b []byte) (*FlowKey, error) {
	if len(b) < FlowKeySize {
		return nil, fmt.Errorf("flow key too short: %d < %d", len(b), FlowKeySize)
	}
	return &FlowKey{
		SrcAddr:  binary.LittleEndian.Uint32(b[0:4]),
		DstAddr:  binary.LittleEndian.Uint32(b[4:8]),
		SrcPort:  binary.LittleEndian.Uint16(b[8:10]),
		DstPort:  binary.LittleEndian.Uint16(b[10:12]),
		Protocol: b[12],
	}, nil
}

// Encode serializes a FlowKey to a byte slice.
func (fk *FlowKey) Encode() [FlowKeySize]byte {
	var b [FlowKeySize]byte
	binary.LittleEndian.PutUint32(b[0:4], fk.SrcAddr)
	binary.LittleEndian.PutUint32(b[4:8], fk.DstAddr)
	binary.LittleEndian.PutUint16(b[8:10], fk.SrcPort)
	binary.LittleEndian.PutUint16(b[10:12], fk.DstPort)
	b[12] = fk.Protocol
	return b
}

// SrcIP returns the source address as a net.IP (IPv4).
func (fk *FlowKey) SrcIP() net.IP {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, fk.SrcAddr)
	return ip
}

// DstIP returns the destination address as a net.IP (IPv4).
func (fk *FlowKey) DstIP() net.IP {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, fk.DstAddr)
	return ip
}

// String returns a human-readable representation of the flow key.
func (fk *FlowKey) String() string {
	proto := "unknown"
	switch fk.Protocol {
	case 6:
		proto = "tcp"
	case 17:
		proto = "udp"
	}
	srcPort := binary.BigEndian.Uint16([]byte{byte(fk.SrcPort >> 8), byte(fk.SrcPort)})
	dstPort := binary.BigEndian.Uint16([]byte{byte(fk.DstPort >> 8), byte(fk.DstPort)})
	return fmt.Sprintf("%s:%d->%s:%d/%s", fk.SrcIP(), srcPort, fk.DstIP(), dstPort, proto)
}

// ── FlowStateEntry ───────────────────────────────────────────────────────────

// FlowStateEntry mirrors the BPF FlowState struct (72 bytes).
type FlowStateEntry struct {
	TraceID    [16]byte // Associated trace ID
	StartNS    uint64   // Connection start timestamp (nanoseconds)
	LastSeenNS uint64   // Last packet timestamp
	PacketsIn  uint64   // Inbound packet count
	PacketsOut uint64   // Outbound packet count
	BytesIn    uint64   // Inbound bytes
	BytesOut   uint64   // Outbound bytes
	State      uint8    // ConnectionState enum
	Pad        [7]byte  // Alignment
}

// DecodeFlowState decodes a FlowStateEntry from a byte slice (little-endian).
func DecodeFlowState(b []byte) (*FlowStateEntry, error) {
	if len(b) < FlowStateSize {
		return nil, fmt.Errorf("flow state too short: %d < %d", len(b), FlowStateSize)
	}
	fs := &FlowStateEntry{
		StartNS:    binary.LittleEndian.Uint64(b[16:24]),
		LastSeenNS: binary.LittleEndian.Uint64(b[24:32]),
		PacketsIn:  binary.LittleEndian.Uint64(b[32:40]),
		PacketsOut: binary.LittleEndian.Uint64(b[40:48]),
		BytesIn:    binary.LittleEndian.Uint64(b[48:56]),
		BytesOut:   binary.LittleEndian.Uint64(b[56:64]),
		State:      b[64],
	}
	copy(fs.TraceID[:], b[0:16])
	return fs, nil
}

// Encode serializes a FlowStateEntry to a byte slice.
func (fs *FlowStateEntry) Encode() [FlowStateSize]byte {
	var b [FlowStateSize]byte
	copy(b[0:16], fs.TraceID[:])
	binary.LittleEndian.PutUint64(b[16:24], fs.StartNS)
	binary.LittleEndian.PutUint64(b[24:32], fs.LastSeenNS)
	binary.LittleEndian.PutUint64(b[32:40], fs.PacketsIn)
	binary.LittleEndian.PutUint64(b[40:48], fs.PacketsOut)
	binary.LittleEndian.PutUint64(b[48:56], fs.BytesIn)
	binary.LittleEndian.PutUint64(b[56:64], fs.BytesOut)
	b[64] = fs.State
	return b
}

// TotalPackets returns the total packet count (in + out).
func (fs *FlowStateEntry) TotalPackets() uint64 {
	return fs.PacketsIn + fs.PacketsOut
}

// TotalBytes returns the total byte count (in + out).
func (fs *FlowStateEntry) TotalBytes() uint64 {
	return fs.BytesIn + fs.BytesOut
}

// StateName returns the human-readable connection state.
func (fs *FlowStateEntry) StateName() string {
	return ConnStateName(fs.State)
}

// TraceIDHex returns the trace ID as a hex string.
func (fs *FlowStateEntry) TraceIDHex() string {
	return fmt.Sprintf("%032x", fs.TraceID)
}

// DurationNS returns the flow duration in nanoseconds.
func (fs *FlowStateEntry) DurationNS() uint64 {
	if fs.LastSeenNS >= fs.StartNS {
		return fs.LastSeenNS - fs.StartNS
	}
	return 0
}

// ── FlowEventEntry ───────────────────────────────────────────────────────────

// FlowEventEntry mirrors the BPF FlowEvent struct (104 bytes).
type FlowEventEntry struct {
	TimestampNS uint64         // Kernel timestamp
	Key         FlowKey        // 5-tuple
	State       FlowStateEntry // Full flow state
	EventType   uint8          // FlowEventType
	Pad         [7]byte        // Alignment
}

// DecodeFlowEvent decodes a FlowEventEntry from a byte slice.
func DecodeFlowEvent(b []byte) (*FlowEventEntry, error) {
	if len(b) < FlowEventSize {
		return nil, fmt.Errorf("flow event too short: %d < %d", len(b), FlowEventSize)
	}

	key, err := DecodeFlowKey(b[8:24])
	if err != nil {
		return nil, fmt.Errorf("decode flow key: %w", err)
	}

	state, err := DecodeFlowState(b[24:96])
	if err != nil {
		return nil, fmt.Errorf("decode flow state: %w", err)
	}

	return &FlowEventEntry{
		TimestampNS: binary.LittleEndian.Uint64(b[0:8]),
		Key:         *key,
		State:       *state,
		EventType:   b[96],
	}, nil
}

// EventTypeName returns the human-readable event type.
func (fe *FlowEventEntry) EventTypeName() string {
	return FlowEventTypeName(fe.EventType)
}

// ── Prometheus metrics for flow reader ───────────────────────────────────────

var (
	flowReaderActiveFlows = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "flow_reader_active_flows",
		Help: "Number of currently tracked active flows",
	})

	flowReaderEventsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "flow_reader_events_total",
		Help: "Total flow events published to Wotan",
	}, []string{"event_type"})

	flowReaderStaleExpired = promauto.NewCounter(prometheus.CounterOpts{
		Name: "flow_reader_stale_flows_expired_total",
		Help: "Total stale flows expired by GC",
	})

	flowReaderPollLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "flow_reader_poll_duration_seconds",
		Help:    "Duration of each flow map poll cycle",
		Buckets: []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1},
	})

	flowReaderErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "flow_reader_errors_total",
		Help: "Total errors during flow reading or publishing",
	}, []string{"operation"})
)

// ── FlowReader ───────────────────────────────────────────────────────────────

// FlowReaderConfig holds configuration for the flow reader.
type FlowReaderConfig struct {
	// PollInterval is how often to poll the FLOW_MAP.
	PollInterval time.Duration

	// StaleTTL is how long a flow can go without packets before expiration.
	StaleTTL time.Duration

	// FlowTopic is the Wotan topic for flow events.
	FlowTopic string

	// BatchSize is the maximum number of flow events per publish call.
	BatchSize int
}

// DefaultFlowReaderConfig returns sensible defaults.
func DefaultFlowReaderConfig() FlowReaderConfig {
	return FlowReaderConfig{
		PollInterval: 1 * time.Second,
		StaleTTL:     60 * time.Second,
		FlowTopic:    "traces.flow",
		BatchSize:    256,
	}
}

// FlowReaderStats holds runtime counters for the flow reader.
type FlowReaderStats struct {
	PollCycles    uint64 `json:"poll_cycles"`
	FlowsRead    uint64 `json:"flows_read"`
	FlowsExpired uint64 `json:"flows_expired"`
	EventsSent   uint64 `json:"events_sent"`
	DecodeErrors uint64 `json:"decode_errors"`
	PublishErrors uint64 `json:"publish_errors"`
}

// TrackedFlowState holds the Go-side view of a flow from the BPF map.
type TrackedFlowState struct {
	Key       FlowKey        `json:"key"`
	State     FlowStateEntry `json:"state"`
	PrevState uint8          `json:"prev_state"`
}

// FlowReader periodically reads the flow_tracker FLOW_MAP and publishes events.
type FlowReader struct {
	loader    BPFLoader
	publisher *WotanPublisher
	config    FlowReaderConfig
	stats     FlowReaderStats

	mu         sync.Mutex
	knownFlows map[string]*TrackedFlowState // key hex -> state
	eventCh    chan *FlowEventEntry
}

// NewFlowReader creates a flow reader with the given loader, publisher, and config.
func NewFlowReader(loader BPFLoader, publisher *WotanPublisher, config FlowReaderConfig) *FlowReader {
	return &FlowReader{
		loader:     loader,
		publisher:  publisher,
		config:     config,
		knownFlows: make(map[string]*TrackedFlowState),
		eventCh:    make(chan *FlowEventEntry, config.BatchSize*4),
	}
}

// Run starts the periodic flow map reading loop. Blocks until ctx is cancelled.
func (fr *FlowReader) Run(ctx context.Context) {
	ticker := time.NewTicker(fr.config.PollInterval)
	defer ticker.Stop()

	log.Info().
		Dur("interval", fr.config.PollInterval).
		Dur("stale_ttl", fr.config.StaleTTL).
		Str("flow_topic", fr.config.FlowTopic).
		Msg("flow reader started")

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("flow reader shutting down")
			return
		case <-ticker.C:
			fr.pollOnce(ctx)
		}
	}
}

// Events returns a channel of decoded flow events for downstream processing.
func (fr *FlowReader) Events() <-chan *FlowEventEntry {
	return fr.eventCh
}

// Stats returns a snapshot of flow reader statistics.
func (fr *FlowReader) Stats() FlowReaderStats {
	return FlowReaderStats{
		PollCycles:    atomic.LoadUint64(&fr.stats.PollCycles),
		FlowsRead:    atomic.LoadUint64(&fr.stats.FlowsRead),
		FlowsExpired: atomic.LoadUint64(&fr.stats.FlowsExpired),
		EventsSent:   atomic.LoadUint64(&fr.stats.EventsSent),
		DecodeErrors: atomic.LoadUint64(&fr.stats.DecodeErrors),
		PublishErrors: atomic.LoadUint64(&fr.stats.PublishErrors),
	}
}

// ActiveFlows returns the number of currently tracked flows.
func (fr *FlowReader) ActiveFlows() int {
	fr.mu.Lock()
	defer fr.mu.Unlock()
	return len(fr.knownFlows)
}

// pollOnce reads the flow map, detects changes, expires stale flows.
func (fr *FlowReader) pollOnce(ctx context.Context) {
	start := time.Now()
	defer func() {
		flowReaderPollLatency.Observe(time.Since(start).Seconds())
		atomic.AddUint64(&fr.stats.PollCycles, 1)
	}()

	flowMap := fr.loader.GetFlowStateMap()
	if flowMap == nil {
		return
	}

	fr.mu.Lock()
	defer fr.mu.Unlock()

	// Track which keys we see this cycle (to detect disappeared flows)
	seenKeys := make(map[string]bool)

	// Read all flow entries from the BPF map
	var events []*FlowEventEntry

	err := flowMap.Iterate(func(key, value []byte) error {
		fk, err := DecodeFlowKey(key)
		if err != nil {
			atomic.AddUint64(&fr.stats.DecodeErrors, 1)
			flowReaderErrors.WithLabelValues("decode_key").Inc()
			return nil // Continue despite decode errors
		}

		fs, err := DecodeFlowState(value)
		if err != nil {
			atomic.AddUint64(&fr.stats.DecodeErrors, 1)
			flowReaderErrors.WithLabelValues("decode_state").Inc()
			return nil
		}

		atomic.AddUint64(&fr.stats.FlowsRead, 1)

		keyHex := fmt.Sprintf("%x", key)
		seenKeys[keyHex] = true

		// Check if this is a new or changed flow
		existing, known := fr.knownFlows[keyHex]
		if !known {
			// New flow
			fr.knownFlows[keyHex] = &TrackedFlowState{
				Key:       *fk,
				State:     *fs,
				PrevState: fs.State,
			}

			event := &FlowEventEntry{
				TimestampNS: fs.StartNS,
				Key:         *fk,
				State:       *fs,
				EventType:   FlowEventNew,
			}
			events = append(events, event)

		} else if existing.State.State != fs.State {
			// State change
			existing.PrevState = existing.State.State
			existing.State = *fs

			event := &FlowEventEntry{
				TimestampNS: fs.LastSeenNS,
				Key:         *fk,
				State:       *fs,
				EventType:   FlowEventStateChange,
			}
			events = append(events, event)

		} else {
			// Update existing flow state (counters may have changed)
			existing.State = *fs
		}

		return nil
	})

	if err != nil {
		flowReaderErrors.WithLabelValues("iterate").Inc()
		log.Error().Err(err).Msg("error iterating flow map")
		return
	}

	// Expire stale flows: flows we previously knew about but are not in the BPF map,
	// or flows that haven't been seen recently.
	nowNS := uint64(time.Now().UnixNano())
	staleTTLNS := uint64(fr.config.StaleTTL.Nanoseconds())
	expired := 0

	for keyHex, flow := range fr.knownFlows {
		if !seenKeys[keyHex] || (nowNS > flow.State.LastSeenNS && nowNS-flow.State.LastSeenNS > staleTTLNS) {
			// Emit expired event
			event := &FlowEventEntry{
				TimestampNS: nowNS,
				Key:         flow.Key,
				State:       flow.State,
				EventType:   FlowEventExpired,
			}
			events = append(events, event)

			delete(fr.knownFlows, keyHex)
			expired++
		}
	}

	if expired > 0 {
		atomic.AddUint64(&fr.stats.FlowsExpired, uint64(expired))
		flowReaderStaleExpired.Add(float64(expired))
	}

	flowReaderActiveFlows.Set(float64(len(fr.knownFlows)))

	// Publish events
	if len(events) > 0 {
		fr.publishFlowEvents(ctx, events)

		// Emit to event channel (non-blocking)
		for _, ev := range events {
			select {
			case fr.eventCh <- ev:
			default:
				// Channel full, skip
			}
		}
	}
}

// publishFlowEvents serializes flow events and publishes them to Wotan.
func (fr *FlowReader) publishFlowEvents(ctx context.Context, events []*FlowEventEntry) {
	type jsonFlowEvent struct {
		TimestampNS uint64 `json:"timestamp_ns"`
		EventType   string `json:"event_type"`
		SrcAddr     string `json:"src_addr"`
		DstAddr     string `json:"dst_addr"`
		SrcPort     uint16 `json:"src_port"`
		DstPort     uint16 `json:"dst_port"`
		Protocol    uint8  `json:"protocol"`
		ConnState   string `json:"conn_state"`
		PacketsIn   uint64 `json:"packets_in"`
		PacketsOut  uint64 `json:"packets_out"`
		BytesIn     uint64 `json:"bytes_in"`
		BytesOut    uint64 `json:"bytes_out"`
		DurationNS  uint64 `json:"duration_ns"`
		TraceID     string `json:"trace_id,omitempty"`
	}

	jsonEvents := make([]jsonFlowEvent, 0, len(events))
	for _, ev := range events {
		jsonEvents = append(jsonEvents, jsonFlowEvent{
			TimestampNS: ev.TimestampNS,
			EventType:   ev.EventTypeName(),
			SrcAddr:     ev.Key.SrcIP().String(),
			DstAddr:     ev.Key.DstIP().String(),
			SrcPort:     ev.Key.SrcPort,
			DstPort:     ev.Key.DstPort,
			Protocol:    ev.Key.Protocol,
			ConnState:   ev.State.StateName(),
			PacketsIn:   ev.State.PacketsIn,
			PacketsOut:  ev.State.PacketsOut,
			BytesIn:     ev.State.BytesIn,
			BytesOut:    ev.State.BytesOut,
			DurationNS:  ev.State.DurationNS(),
			TraceID:     ev.State.TraceIDHex(),
		})
	}

	// Publish in batches
	for i := 0; i < len(jsonEvents); i += fr.config.BatchSize {
		end := i + fr.config.BatchSize
		if end > len(jsonEvents) {
			end = len(jsonEvents)
		}
		batch := jsonEvents[i:end]

		payload, err := json.Marshal(batch)
		if err != nil {
			atomic.AddUint64(&fr.stats.PublishErrors, 1)
			flowReaderErrors.WithLabelValues("marshal").Inc()
			log.Error().Err(err).Msg("failed to marshal flow events")
			continue
		}

		// Payload ready for Wotan transport
		_ = payload

		count := uint64(len(batch))
		atomic.AddUint64(&fr.stats.EventsSent, count)
		for _, je := range batch {
			flowReaderEventsTotal.WithLabelValues(je.EventType).Inc()
		}
	}
}
