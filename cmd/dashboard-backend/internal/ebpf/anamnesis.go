// Anamnesis event ingestion for the dashboard backend.
//
// This module subscribes to anamnesis.* Wotan topics (published by
// trace-collector-go), decodes Anamnesis events, builds flow records,
// computes per-hop latency, and emits events to the WebSocket broadcaster.
//
// Per RFC 9669: we use "BPF" (not "eBPF") throughout.
package ebpf

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	pkgebpf "unheaded/pkg/ebpf"
	"unheaded/pkg/logger"
)

// ── Anamnesis topic names ───────────────────────────────────────────────────

const (
	AnamnesisTopicBirth   = "anamnesis.birth"
	AnamnesisTopicHop     = "anamnesis.hop"
	AnamnesisTopicDeath   = "anamnesis.death"
	AnamnesisTopicAnomaly = "anamnesis.anomaly"
	AnamnesisTopicChaos   = "anamnesis.chaos"
)

var anamnesisTopics = []string{
	AnamnesisTopicBirth,
	AnamnesisTopicHop,
	AnamnesisTopicDeath,
	AnamnesisTopicAnomaly,
	AnamnesisTopicChaos,
}

// ── AnamnesisFlow ───────────────────────────────────────────────────────────

// AnamnesisFlow accumulates Anamnesis events for a single flow (flow_label).
type AnamnesisFlow struct {
	FlowLabel  uint16                    `json:"flow_label"`
	Birth      *pkgebpf.AnamnesisEvent   `json:"birth,omitempty"`
	Hops       []*pkgebpf.AnamnesisEvent `json:"hops"`
	Death      *pkgebpf.AnamnesisEvent   `json:"death,omitempty"`
	Anomalies  []*pkgebpf.AnamnesisEvent `json:"anomalies,omitempty"`
	StartNs    uint64                    `json:"start_ns"`
	EndNs      uint64                    `json:"end_ns"`
	LatencyNs  uint64                    `json:"latency_ns"`
	HopCount   int                       `json:"hop_count"`
	Complete   bool                      `json:"complete"`
	HopLatency []HopLatency              `json:"hop_latency,omitempty"`
}

// HopLatency records the latency between consecutive hops.
type HopLatency struct {
	FromHopID uint8  `json:"from_hop_id"`
	ToHopID   uint8  `json:"to_hop_id"`
	LatencyNs uint64 `json:"latency_ns"`
}

// computeHopLatency calculates per-hop latency from consecutive event timestamps.
func (af *AnamnesisFlow) computeHopLatency() {
	var events []*pkgebpf.AnamnesisEvent
	if af.Birth != nil {
		events = append(events, af.Birth)
	}
	events = append(events, af.Hops...)
	if af.Death != nil {
		events = append(events, af.Death)
	}

	af.HopLatency = nil
	for i := 1; i < len(events); i++ {
		prev := events[i-1]
		curr := events[i]
		if curr.TimestampNs >= prev.TimestampNs {
			af.HopLatency = append(af.HopLatency, HopLatency{
				FromHopID: prev.HopID,
				ToHopID:   curr.HopID,
				LatencyNs: curr.TimestampNs - prev.TimestampNs,
			})
		}
	}
}

// ── Flow Builder ────────────────────────────────────────────────────────────

// FlowBuilderConfig configures the Anamnesis flow builder.
type FlowBuilderConfig struct {
	TTL      time.Duration // How long incomplete flows persist (default: 60s)
	MaxFlows int           // Max in-flight flows (default: 65536)
}

// FlowBuilder accumulates Anamnesis events by flow_label into AnamnesisFlows.
type FlowBuilder struct {
	config    FlowBuilderConfig
	flows     map[uint16]*AnamnesisFlow
	mu        sync.RWMutex
	completed chan *AnamnesisFlow

	// Stats
	totalEvents    atomic.Int64
	totalFlows     atomic.Int64
	completedFlows atomic.Int64
}

// NewFlowBuilder creates a new flow builder.
func NewFlowBuilder(config FlowBuilderConfig) *FlowBuilder {
	if config.TTL == 0 {
		config.TTL = 60 * time.Second
	}
	if config.MaxFlows == 0 {
		config.MaxFlows = 65536
	}
	return &FlowBuilder{
		config:    config,
		flows:     make(map[uint16]*AnamnesisFlow),
		completed: make(chan *AnamnesisFlow, 4096),
	}
}

// Ingest adds an Anamnesis event to the flow builder.
func (fb *FlowBuilder) Ingest(ev *pkgebpf.AnamnesisEvent) {
	fb.totalEvents.Add(1)

	fb.mu.Lock()
	defer fb.mu.Unlock()

	fl := ev.FlowLabelLo
	flow, exists := fb.flows[fl]
	if !exists {
		fb.totalFlows.Add(1)
		flow = &AnamnesisFlow{
			FlowLabel: fl,
			StartNs:   ev.TimestampNs,
		}
		fb.flows[fl] = flow
	}

	switch ev.EventType {
	case pkgebpf.EventBirth:
		flow.Birth = ev
		flow.StartNs = ev.TimestampNs
	case pkgebpf.EventHop:
		flow.Hops = append(flow.Hops, ev)
		flow.HopCount = len(flow.Hops)
	case pkgebpf.EventDeath:
		flow.Death = ev
		flow.EndNs = ev.TimestampNs
		if flow.StartNs > 0 {
			flow.LatencyNs = ev.TimestampNs - flow.StartNs
		}
		flow.Complete = true
	case pkgebpf.EventAnomaly, pkgebpf.EventChaos:
		flow.Anomalies = append(flow.Anomalies, ev)
	}

	if flow.Complete {
		flow.computeHopLatency()
		delete(fb.flows, fl)
		fb.completedFlows.Add(1)
		select {
		case fb.completed <- flow:
		default:
			// Channel full, drop
		}
	}
}

// CompletedFlows returns the channel of completed flows.
func (fb *FlowBuilder) CompletedFlows() <-chan *AnamnesisFlow {
	return fb.completed
}

// GC removes incomplete flows older than TTL.
func (fb *FlowBuilder) GC(nowNs uint64) int {
	fb.mu.Lock()
	defer fb.mu.Unlock()

	ttlNs := uint64(fb.config.TTL.Nanoseconds())
	removed := 0
	for fl, flow := range fb.flows {
		if nowNs-flow.StartNs > ttlNs {
			delete(fb.flows, fl)
			removed++
		}
	}
	return removed
}

// InFlightCount returns the number of incomplete flows.
func (fb *FlowBuilder) InFlightCount() int {
	fb.mu.RLock()
	defer fb.mu.RUnlock()
	return len(fb.flows)
}

// Stats returns flow builder statistics.
func (fb *FlowBuilder) Stats() FlowBuilderStats {
	return FlowBuilderStats{
		TotalEvents:    fb.totalEvents.Load(),
		TotalFlows:     fb.totalFlows.Load(),
		CompletedFlows: fb.completedFlows.Load(),
		InFlightFlows:  fb.InFlightCount(),
	}
}

// FlowBuilderStats holds flow builder statistics.
type FlowBuilderStats struct {
	TotalEvents    int64 `json:"total_events"`
	TotalFlows     int64 `json:"total_flows"`
	CompletedFlows int64 `json:"completed_flows"`
	InFlightFlows  int   `json:"in_flight_flows"`
}

// ── Monad decoder ───────────────────────────────────────────────────────────

// DecodedMonad provides a dashboard-friendly view of the Monad register state.
type DecodedMonad struct {
	Version      uint8    `json:"version"`
	SrcServiceID uint8    `json:"src_service_id"`
	DstServiceID uint8    `json:"dst_service_id"`
	HopCount     uint8    `json:"hop_count"`
	QosClass     uint8    `json:"qos_class"`
	FlowAction   string   `json:"flow_action"`
	CircuitState string   `json:"circuit_state"`
	Flags        []string `json:"flags"`
	LatencyHint  uint16   `json:"latency_hint_us"`
	DeployRing   string   `json:"deploy_ring"`
	ScratchR0    uint16   `json:"scratch_r0"`
	ScratchR1    uint16   `json:"scratch_r1"`
	ChecksumOK  bool     `json:"checksum_ok"`
}

// DecodeMonadForDashboard decodes a MonadState into a dashboard-friendly format.
func DecodeMonadForDashboard(m *pkgebpf.MonadState) *DecodedMonad {
	return &DecodedMonad{
		Version:      m.Version,
		SrcServiceID: m.SrcServiceID,
		DstServiceID: m.DstServiceID,
		HopCount:     m.HopCount,
		QosClass:     m.QosClass,
		FlowAction:   flowActionName(m.FlowAction),
		CircuitState: circuitStateName(m.CircuitState),
		Flags:        m.FlagNames(),
		LatencyHint:  m.LatencyHint,
		DeployRing:   deployRingName(m.DeployRing),
		ScratchR0:    m.ScratchR0(),
		ScratchR1:    m.ScratchR1(),
		ChecksumOK:  m.VerifyChecksum(),
	}
}

func flowActionName(v uint8) string {
	switch v {
	case 0x01:
		return "forward"
	case 0x02:
		return "trace"
	case 0x03:
		return "sample"
	case 0x04:
		return "mirror"
	case 0x05:
		return "drop"
	default:
		return fmt.Sprintf("unknown(%d)", v)
	}
}

func circuitStateName(v uint8) string {
	switch v {
	case 0x01:
		return "closed"
	case 0x02:
		return "open"
	case 0x03:
		return "half_open"
	default:
		return fmt.Sprintf("unknown(%d)", v)
	}
}

func deployRingName(v uint8) string {
	switch v {
	case 0x01:
		return "production"
	case 0x02:
		return "canary"
	case 0x03:
		return "staging"
	case 0x04:
		return "development"
	default:
		return fmt.Sprintf("unknown(%d)", v)
	}
}

// ── WebSocket message types ─────────────────────────────────────────────────

// AnamnesisWSMessage wraps Anamnesis events for WebSocket broadcast.
type AnamnesisWSMessage struct {
	Type  string      `json:"type"`
	Data  interface{} `json:"data"`
}

// NewFlowWSMessage creates a WebSocket message for a completed flow.
func NewFlowWSMessage(flow *AnamnesisFlow) ([]byte, error) {
	msg := AnamnesisWSMessage{
		Type: "flow",
		Data: flow,
	}
	return json.Marshal(msg)
}

// NewHopWSMessage creates a WebSocket message for a hop event.
func NewHopWSMessage(ev *pkgebpf.AnamnesisEvent) ([]byte, error) {
	msg := AnamnesisWSMessage{
		Type: "hop",
		Data: ev,
	}
	return json.Marshal(msg)
}

// NewAnomalyWSMessage creates a WebSocket message for an anomaly/chaos event.
func NewAnomalyWSMessage(ev *pkgebpf.AnamnesisEvent) ([]byte, error) {
	typeName := "anomaly"
	if ev.EventType == pkgebpf.EventChaos {
		typeName = "chaos"
	}
	msg := AnamnesisWSMessage{
		Type: typeName,
		Data: ev,
	}
	return json.Marshal(msg)
}

// ── Anamnesis Ingestor ──────────────────────────────────────────────────────

// AnamnesisIngestor subscribes to anamnesis.* Wotan topics and feeds events
// into the FlowBuilder, computing latency and emitting WebSocket messages.
type AnamnesisIngestor struct {
	log          *logger.Logger
	client       WotanSubscriber
	flowBuilder  *FlowBuilder
	broadcaster  func([]byte) // WebSocket broadcast function
	listeners    []func(*pkgebpf.AnamnesisEvent)
	listMu       sync.RWMutex

	// Stats
	eventsIngested atomic.Int64
	parseErrors    atomic.Int64
}

// NewAnamnesisIngestor creates a new Anamnesis event ingestor.
func NewAnamnesisIngestor(
	client WotanSubscriber,
	flowBuilder *FlowBuilder,
	broadcaster func([]byte),
	log *logger.Logger,
) *AnamnesisIngestor {
	if log == nil {
		log = logger.New(io.Discard)
	}
	return &AnamnesisIngestor{
		log:         log,
		client:      client,
		flowBuilder: flowBuilder,
		broadcaster: broadcaster,
	}
}

// OnEvent registers a listener for raw Anamnesis events.
func (ai *AnamnesisIngestor) OnEvent(fn func(*pkgebpf.AnamnesisEvent)) {
	ai.listMu.Lock()
	defer ai.listMu.Unlock()
	ai.listeners = append(ai.listeners, fn)
}

// Start subscribes to all anamnesis topics and begins streaming.
func (ai *AnamnesisIngestor) Start(ctx context.Context) error {
	for _, topic := range anamnesisTopics {
		if _, err := ai.client.Subscribe(ctx, topic, "dashboard-backend"); err != nil {
			ai.log.Warn().Err(err).Str("topic", topic).Msg("failed to subscribe to anamnesis topic")
			continue
		}
		ai.log.Info().Str("topic", topic).Msg("subscribed to anamnesis topic")
	}

	for _, topic := range anamnesisTopics {
		go ai.streamTopic(ctx, topic)
	}

	// Process completed flows and broadcast to WebSocket
	go ai.broadcastFlows(ctx)

	// GC loop for stale flows
	go ai.gcLoop(ctx)

	return nil
}

func (ai *AnamnesisIngestor) streamTopic(ctx context.Context, topic string) {
	ch, err := ai.client.StreamMessages(ctx, topic)
	if err != nil {
		ai.log.Error().Err(err).Str("topic", topic).Msg("failed to stream anamnesis topic")
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			if msg == nil {
				continue
			}
			ai.handleMessage(topic, msg.Payload)
		}
	}
}

func (ai *AnamnesisIngestor) handleMessage(topic, payload string) {
	// The payload is a JSON-encoded AnamnesisEvent (or array of them)
	data := []byte(payload)

	// Try single event first
	var ev pkgebpf.AnamnesisEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		// Try array
		var events []*pkgebpf.AnamnesisEvent
		if err2 := json.Unmarshal(data, &events); err2 != nil {
			ai.parseErrors.Add(1)
			return
		}
		for _, e := range events {
			ai.processEvent(e)
		}
		return
	}
	ai.processEvent(&ev)
}

func (ai *AnamnesisIngestor) processEvent(ev *pkgebpf.AnamnesisEvent) {
	ai.eventsIngested.Add(1)

	// Feed into flow builder
	ai.flowBuilder.Ingest(ev)

	// Notify listeners
	ai.listMu.RLock()
	listeners := make([]func(*pkgebpf.AnamnesisEvent), len(ai.listeners))
	copy(listeners, ai.listeners)
	ai.listMu.RUnlock()

	for _, fn := range listeners {
		fn(ev)
	}

	// Broadcast hop/anomaly events immediately to WebSocket
	if ai.broadcaster != nil {
		var wsData []byte
		var err error

		switch ev.EventType {
		case pkgebpf.EventHop:
			wsData, err = NewHopWSMessage(ev)
		case pkgebpf.EventAnomaly, pkgebpf.EventChaos:
			wsData, err = NewAnomalyWSMessage(ev)
		}

		if err == nil && wsData != nil {
			ai.broadcaster(wsData)
		}
	}
}

func (ai *AnamnesisIngestor) broadcastFlows(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case flow, ok := <-ai.flowBuilder.CompletedFlows():
			if !ok {
				return
			}
			if ai.broadcaster == nil {
				continue
			}
			wsData, err := NewFlowWSMessage(flow)
			if err != nil {
				continue
			}
			ai.broadcaster(wsData)
		}
	}
}

func (ai *AnamnesisIngestor) gcLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ai.flowBuilder.GC(uint64(time.Now().UnixNano()))
		}
	}
}

// Stats returns Anamnesis ingestor statistics.
func (ai *AnamnesisIngestor) Stats() AnamnesisIngestorStats {
	return AnamnesisIngestorStats{
		EventsIngested: ai.eventsIngested.Load(),
		ParseErrors:    ai.parseErrors.Load(),
		FlowBuilder:    ai.flowBuilder.Stats(),
	}
}

// AnamnesisIngestorStats holds ingestor statistics.
type AnamnesisIngestorStats struct {
	EventsIngested int64            `json:"events_ingested"`
	ParseErrors    int64            `json:"parse_errors"`
	FlowBuilder    FlowBuilderStats `json:"flow_builder"`
}
