package ebpf

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	wotanClient "unheaded/pkg/wotan-client"
	"unheaded/pkg/logger"
)

// IngestorConfig configures the eBPF event ingestor.
type IngestorConfig struct {
	// FlowTTL is the TTL for active flows (default: 30s).
	FlowTTL time.Duration
	// MaxFlows is the maximum tracked flows (default: 65536).
	MaxFlows int
	// LatencyWindows are the time windows for latency percentiles.
	LatencyWindows []time.Duration
	// MaxLatencySamples per window per operation (default: 10000).
	MaxLatencySamples int
	// ExpireInterval is how often to run TTL expiration (default: 5s).
	ExpireInterval time.Duration
	// EventBufferSize is the size of the recent events ring buffer.
	EventBufferSize int
}

// DefaultIngestorConfig returns sensible defaults.
func DefaultIngestorConfig() IngestorConfig {
	return IngestorConfig{
		FlowTTL:           30 * time.Second,
		MaxFlows:          65536,
		LatencyWindows:    []time.Duration{1 * time.Second, 10 * time.Second, 60 * time.Second},
		MaxLatencySamples: 10000,
		ExpireInterval:    5 * time.Second,
		EventBufferSize:   1000,
	}
}

// WotanSubscriber abstracts the Wotan client interface needed by the ingestor.
// Both wotanClient.Client and wotanClient.TopicStreamClient satisfy this.
type WotanSubscriber interface {
	Subscribe(ctx context.Context, topic, displayName string) (*wotanClient.Subscriber, error)
	StreamMessages(ctx context.Context, topic string) (<-chan *wotanClient.Message, error)
	Close() error
}

// Ingestor subscribes to eBPF Wotan topics and feeds events into
// the flow graph, latency histogram, and event buffer.
type Ingestor struct {
	config    IngestorConfig
	log       *logger.Logger
	client    WotanSubscriber
	flows     *FlowGraph
	latency   *LatencyHistogram
	events    *EventRing
	listeners []func(EventEnvelope)
	listMu    sync.RWMutex

	// Counters
	packetsIngested atomic.Int64
	flowsIngested   atomic.Int64
	latencyIngested atomic.Int64
	syscallIngested atomic.Int64
	parseErrors     atomic.Int64
}

// EventEnvelope wraps a typed eBPF event for listener dispatch.
type EventEnvelope struct {
	Topic     string      `json:"topic"`
	Type      string      `json:"type"` // "packet", "flow", "latency", "syscall"
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data"`
}

// EventRing is a bounded ring buffer of recent eBPF events.
type EventRing struct {
	events []EventEnvelope
	size   int
	mu     sync.RWMutex
}

// NewEventRing creates a new event ring buffer.
func NewEventRing(size int) *EventRing {
	return &EventRing{
		events: make([]EventEnvelope, 0, size),
		size:   size,
	}
}

// Add appends an event, evicting the oldest if full.
func (r *EventRing) Add(e EventEnvelope) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
	if len(r.events) > r.size {
		r.events = r.events[len(r.events)-r.size:]
	}
}

// Recent returns up to n most recent events, newest first.
func (r *EventRing) Recent(n int) []EventEnvelope {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if n > len(r.events) {
		n = len(r.events)
	}
	if n == 0 {
		return nil
	}
	out := make([]EventEnvelope, n)
	for i := 0; i < n; i++ {
		out[i] = r.events[len(r.events)-1-i]
	}
	return out
}

// Count returns the number of buffered events.
func (r *EventRing) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.events)
}

// NewIngestor creates a new eBPF event ingestor.
func NewIngestor(config IngestorConfig, client WotanSubscriber, log *logger.Logger) *Ingestor {
	if log == nil {
		log = logger.New(io.Discard)
	}
	return &Ingestor{
		config: config,
		log:    log,
		client: client,
		flows: NewFlowGraph(FlowGraphConfig{
			TTL:      config.FlowTTL,
			MaxFlows: config.MaxFlows,
		}),
		latency: NewLatencyHistogram(LatencyHistogramConfig{
			Windows:    config.LatencyWindows,
			MaxSamples: config.MaxLatencySamples,
		}),
		events: NewEventRing(config.EventBufferSize),
	}
}

// OnEvent registers a listener for all incoming eBPF events.
func (ing *Ingestor) OnEvent(fn func(EventEnvelope)) {
	ing.listMu.Lock()
	defer ing.listMu.Unlock()
	ing.listeners = append(ing.listeners, fn)
}

// Start subscribes to eBPF topics and begins ingesting events.
func (ing *Ingestor) Start(ctx context.Context) error {
	topics := []string{
		"ebpf.packet.events",
		"ebpf.flow.events",
		"ebpf.latency.events",
		"ebpf.syscall.events",
	}

	for _, topic := range topics {
		if _, err := ing.client.Subscribe(ctx, topic, "dashboard-backend"); err != nil {
			ing.log.Warn().Err(err).Str("topic", topic).Msg("failed to subscribe to ebpf topic")
			continue
		}
		ing.log.Info().Str("topic", topic).Msg("subscribed to ebpf topic")
	}

	// Start streaming each topic
	for _, topic := range topics {
		go ing.streamTopic(ctx, topic)
	}

	// Start expiration loop
	go ing.expirationLoop(ctx)

	return nil
}

// streamTopic streams messages from a single topic and dispatches them.
func (ing *Ingestor) streamTopic(ctx context.Context, topic string) {
	ch, err := ing.client.StreamMessages(ctx, topic)
	if err != nil {
		ing.log.Error().Err(err).Str("topic", topic).Msg("failed to stream ebpf topic")
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
			ing.dispatch(topic, msg.Payload)
		}
	}
}

// dispatch parses and routes an event based on the topic.
// Payloads may be single JSON objects or batched arrays from the
// trace-collector's batch publisher. Arrays are unwrapped and each
// element is dispatched individually.
func (ing *Ingestor) dispatch(topic string, payload string) {
	data := []byte(strings.TrimSpace(payload))

	// Detect JSON arrays (batched events) and unwrap them.
	if len(data) > 0 && data[0] == '[' {
		var batch []json.RawMessage
		if err := json.Unmarshal(data, &batch); err != nil {
			ing.parseErrors.Add(1)
			return
		}
		for _, item := range batch {
			ing.dispatchSingle(topic, item)
		}
		return
	}

	ing.dispatchSingle(topic, data)
}

// dispatchSingle parses and routes a single JSON event object.
func (ing *Ingestor) dispatchSingle(topic string, data []byte) {
	switch {
	case strings.Contains(topic, "packet"):
		e, err := ParsePacketEvent(data)
		if err != nil {
			ing.parseErrors.Add(1)
			return
		}
		ing.flows.IngestPacket(e)
		ing.packetsIngested.Add(1)
		ing.emit(EventEnvelope{
			Topic:     topic,
			Type:      "packet",
			Timestamp: e.Time(),
			Data:      e,
		})

	case strings.Contains(topic, "flow"):
		e, err := ParseFlowEvent(data)
		if err != nil {
			ing.parseErrors.Add(1)
			return
		}
		ing.flows.IngestFlow(e)
		ing.flowsIngested.Add(1)
		ing.emit(EventEnvelope{
			Topic:     topic,
			Type:      "flow",
			Timestamp: e.Time(),
			Data:      e,
		})

	case strings.Contains(topic, "latency"):
		e, err := ParseLatencyEvent(data)
		if err != nil {
			ing.parseErrors.Add(1)
			return
		}
		ing.latency.Ingest(e)
		ing.latencyIngested.Add(1)
		ing.emit(EventEnvelope{
			Topic:     topic,
			Type:      "latency",
			Timestamp: e.Time(),
			Data:      e,
		})

	case strings.Contains(topic, "syscall"):
		e, err := ParseSyscallEvent(data)
		if err != nil {
			ing.parseErrors.Add(1)
			return
		}
		ing.syscallIngested.Add(1)
		ing.emit(EventEnvelope{
			Topic:     topic,
			Type:      "syscall",
			Timestamp: e.Time(),
			Data:      e,
		})
	}
}

// emit adds the event to the ring buffer and notifies all listeners.
func (ing *Ingestor) emit(env EventEnvelope) {
	ing.events.Add(env)

	ing.listMu.RLock()
	listeners := make([]func(EventEnvelope), len(ing.listeners))
	copy(listeners, ing.listeners)
	ing.listMu.RUnlock()

	for _, fn := range listeners {
		fn(env)
	}
}

// expirationLoop periodically expires stale flows and latency samples.
func (ing *Ingestor) expirationLoop(ctx context.Context) {
	ticker := time.NewTicker(ing.config.ExpireInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			expired := ing.flows.Expire()
			ing.latency.Expire()
			if expired > 0 {
				ing.log.Debug().Int("expired_flows", expired).Msg("expired stale flows")
			}
		}
	}
}

// FlowGraph returns the underlying flow graph.
func (ing *Ingestor) FlowGraph() *FlowGraph {
	return ing.flows
}

// LatencyHistogram returns the underlying latency histogram.
func (ing *Ingestor) LatencyHistogram() *LatencyHistogram {
	return ing.latency
}

// RecentEvents returns the most recent n events.
func (ing *Ingestor) RecentEvents(n int) []EventEnvelope {
	return ing.events.Recent(n)
}

// Stats returns ingestor statistics.
func (ing *Ingestor) Stats() IngestorStats {
	return IngestorStats{
		PacketsIngested: ing.packetsIngested.Load(),
		FlowsIngested:  ing.flowsIngested.Load(),
		LatencyIngested: ing.latencyIngested.Load(),
		SyscallIngested: ing.syscallIngested.Load(),
		ParseErrors:     ing.parseErrors.Load(),
		FlowStats:       ing.flows.Stats(),
		LatencyStats:    ing.latency.Stats(),
		EventsBuffered:  ing.events.Count(),
	}
}

// IngestorStats holds ingestor statistics for the /api/v1/ebpf/stats endpoint.
type IngestorStats struct {
	PacketsIngested int64          `json:"packets_ingested"`
	FlowsIngested  int64          `json:"flows_ingested"`
	LatencyIngested int64          `json:"latency_ingested"`
	SyscallIngested int64          `json:"syscall_ingested"`
	ParseErrors     int64          `json:"parse_errors"`
	FlowStats       FlowGraphStats `json:"flow_stats"`
	LatencyStats    LatencyStats   `json:"latency_stats"`
	EventsBuffered  int            `json:"events_buffered"`
}

// MarshalJSON implements json.Marshaler for IngestorStats.
func (s IngestorStats) MarshalJSON() ([]byte, error) {
	type Alias IngestorStats
	return json.Marshal(Alias(s))
}
