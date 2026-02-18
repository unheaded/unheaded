// End-to-end BPF → Dashboard pipeline test (Step 1.7).
//
// This test verifies the full Anamnesis event pipeline:
//
//	BPF ring buffer → trace-collector → Wotan → dashboard-backend → WebSocket
//
// Uses mock Wotan subscriber to inject events and verifies flow correlation,
// hop latency, Monad CRC, and WebSocket broadcast through the full pipeline.
//
// IMPORTANT: Because the AnamnesisIngestor reads from 5 independent topic
// channels (birth, hop, death, anomaly, chaos) via separate goroutines,
// events must be sent in phases with inter-phase waits to guarantee
// ordering within a flow.  The real BPF pipeline has the same characteristic —
// events from different ring buffers can arrive out of order.
//
// Per RFC 9669: we use "BPF" (not "eBPF") throughout.
package ebpf

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	pkgebpf "unheaded/pkg/ebpf"
)

// ── Event factory ───────────────────────────────────────────────────────────

// makeE2EEvent builds an AnamnesisEvent with a valid Monad (including CRC).
func makeE2EEvent(evType pkgebpf.EventType, hopID uint8, flowLabel uint16, ts uint64) *pkgebpf.AnamnesisEvent {
	monad := pkgebpf.MonadState{
		Version:      pkgebpf.MonadVersion,
		SrcServiceID: 0x0A,
		DstServiceID: 0x0B,
		HopCount:     hopID,
		QosClass:     0x01,
		FlowAction:   0x01, // forward
		CircuitState: 0x01, // closed
		Flags:        pkgebpf.FlagTraced,
		LatencyHint:  100,
		DeployRing:   0x01, // production
		Scratch:      [4]uint8{0xCA, 0xFE, 0xBA, 0xBE},
	}
	monad.RecomputeChecksum()

	return &pkgebpf.AnamnesisEvent{
		TimestampNs: ts,
		EventType:   evType,
		HopID:       hopID,
		FlowLabelLo: flowLabel,
		Monad:       monad,
	}
}

// topicForEventType returns the Wotan topic for a given event type.
func topicForEventType(evType pkgebpf.EventType) string {
	switch evType {
	case pkgebpf.EventBirth:
		return AnamnesisTopicBirth
	case pkgebpf.EventHop:
		return AnamnesisTopicHop
	case pkgebpf.EventDeath:
		return AnamnesisTopicDeath
	case pkgebpf.EventAnomaly:
		return AnamnesisTopicAnomaly
	case pkgebpf.EventChaos:
		return AnamnesisTopicChaos
	default:
		return ""
	}
}

// ── Broadcast collector ─────────────────────────────────────────────────────

// wsBroadcastCollector captures WebSocket broadcast messages for verification.
type wsBroadcastCollector struct {
	mu       sync.Mutex
	messages []json.RawMessage
}

func (c *wsBroadcastCollector) broadcast(data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	msg := make(json.RawMessage, len(data))
	copy(msg, data)
	c.messages = append(c.messages, msg)
}

func (c *wsBroadcastCollector) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.messages)
}

func (c *wsBroadcastCollector) getMessages() []json.RawMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]json.RawMessage, len(c.messages))
	copy(out, c.messages)
	return out
}

// parseWSMsg extracts the type field from a WebSocket message.
func parseWSMsg(raw json.RawMessage) (string, json.RawMessage, error) {
	var msg struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		return "", nil, err
	}
	return msg.Type, msg.Data, nil
}

// waitForIngested polls until the ingestor has ingested at least n events.
func waitForIngested(t *testing.T, ing *AnamnesisIngestor, n int64, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		if ing.Stats().EventsIngested >= n {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timeout waiting for %d ingested events (have %d)",
				n, ing.Stats().EventsIngested)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// waitForCompleted polls until the flow builder has completed at least n flows.
func waitForCompleted(t *testing.T, ing *AnamnesisIngestor, n int64, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		if ing.Stats().FlowBuilder.CompletedFlows >= n {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timeout waiting for %d completed flows (have %d)",
				n, ing.Stats().FlowBuilder.CompletedFlows)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// ============================================================================
// E2E TEST: Full Pipeline (100 flows)
// ============================================================================

// TestE2E_BPFDashboard_FullPipeline tests the full Anamnesis pipeline with 100
// concurrent flows: trace-collector publishes events → dashboard-backend ingests
// → flow correlation → WebSocket broadcast.
//
// Events are sent in phases (all BIRTHs → all HOPs → all DEATHs) with waits
// between phases to guarantee correct ordering across the independent topic
// goroutines.
func TestE2E_BPFDashboard_FullPipeline(t *testing.T) {
	const numFlows = 100
	const hopsPerFlow = 2 // BIRTH → HOP → HOP → DEATH

	wotan := newMockWotanSubscriber()
	fb := NewFlowBuilder(FlowBuilderConfig{TTL: 30 * time.Second})
	collector := &wsBroadcastCollector{}
	ingestor := NewAnamnesisIngestor(wotan, fb, collector.broadcast, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := ingestor.Start(ctx); err != nil {
		t.Fatalf("ingestor.Start: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	t.Logf("Publishing %d flows (%d events each)...", numFlows, 2+hopsPerFlow)

	// Phase 1: Send all BIRTHs
	for i := 0; i < numFlows; i++ {
		fl := uint16(0x1000 + i)
		baseTs := uint64(i) * 10000
		wotan.send(AnamnesisTopicBirth, makeE2EEvent(pkgebpf.EventBirth, 0, fl, baseTs))
	}
	waitForIngested(t, ingestor, int64(numFlows), 5*time.Second)

	// Phase 2: Send all HOPs (2 per flow)
	for hop := 1; hop <= hopsPerFlow; hop++ {
		for i := 0; i < numFlows; i++ {
			fl := uint16(0x1000 + i)
			baseTs := uint64(i)*10000 + uint64(hop)*1000
			wotan.send(AnamnesisTopicHop, makeE2EEvent(pkgebpf.EventHop, uint8(hop), fl, baseTs))
		}
		waitForIngested(t, ingestor, int64(numFlows+numFlows*hop), 5*time.Second)
	}

	// Phase 3: Send all DEATHs
	for i := 0; i < numFlows; i++ {
		fl := uint16(0x1000 + i)
		baseTs := uint64(i)*10000 + uint64(hopsPerFlow+1)*1000
		wotan.send(AnamnesisTopicDeath, makeE2EEvent(pkgebpf.EventDeath, uint8(hopsPerFlow+1), fl, baseTs))
	}

	totalEvents := int64(numFlows * (2 + hopsPerFlow))
	waitForIngested(t, ingestor, totalEvents, 5*time.Second)
	waitForCompleted(t, ingestor, int64(numFlows), 5*time.Second)

	// ── Assertions ──────────────────────────────────────────────────────

	stats := ingestor.Stats()

	t.Logf("Events ingested: %d", stats.EventsIngested)
	if stats.EventsIngested != totalEvents {
		t.Errorf("EventsIngested = %d, want %d", stats.EventsIngested, totalEvents)
	}

	t.Logf("Completed flows: %d", stats.FlowBuilder.CompletedFlows)
	if stats.FlowBuilder.CompletedFlows != int64(numFlows) {
		t.Errorf("CompletedFlows = %d, want %d", stats.FlowBuilder.CompletedFlows, numFlows)
	}

	if stats.ParseErrors != 0 {
		t.Errorf("ParseErrors = %d, want 0", stats.ParseErrors)
	}

	if stats.FlowBuilder.InFlightFlows != 0 {
		t.Errorf("InFlightFlows = %d, want 0", stats.FlowBuilder.InFlightFlows)
	}

	// WebSocket broadcasts: hopsPerFlow hop msgs + 1 flow msg per flow
	expectedBroadcasts := numFlows * (hopsPerFlow + 1)
	bcCount := collector.count()
	t.Logf("WebSocket broadcasts: %d (expected %d)", bcCount, expectedBroadcasts)
	if bcCount < expectedBroadcasts {
		t.Errorf("broadcasts = %d, want >= %d", bcCount, expectedBroadcasts)
	}

	// Verify broadcast message types
	msgs := collector.getMessages()
	typeCounts := map[string]int{}
	for _, raw := range msgs {
		msgType, _, err := parseWSMsg(raw)
		if err != nil {
			t.Errorf("failed to parse WS message: %v", err)
			continue
		}
		typeCounts[msgType]++
	}

	t.Logf("WS message types: %v", typeCounts)
	if typeCounts["hop"] < numFlows*hopsPerFlow {
		t.Errorf("hop messages = %d, want >= %d", typeCounts["hop"], numFlows*hopsPerFlow)
	}
	if typeCounts["flow"] < numFlows {
		t.Errorf("flow messages = %d, want >= %d", typeCounts["flow"], numFlows)
	}

	t.Log("Full pipeline E2E test PASSED")
}

// ============================================================================
// E2E TEST: Flow Correlation (varying hop counts)
// ============================================================================

// TestE2E_BPFDashboard_FlowCorrelation verifies that flow records have correct
// hop counts, latency calculations, and per-hop latency breakdown.
func TestE2E_BPFDashboard_FlowCorrelation(t *testing.T) {
	const numFlows = 50

	wotan := newMockWotanSubscriber()
	fb := NewFlowBuilder(FlowBuilderConfig{})
	collector := &wsBroadcastCollector{}
	ingestor := NewAnamnesisIngestor(wotan, fb, collector.broadcast, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := ingestor.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	// Publish flows with varying hop counts (1-5)
	type flowSpec struct {
		flowLabel uint16
		hopCount  int
	}
	var specs []flowSpec
	for i := 0; i < numFlows; i++ {
		specs = append(specs, flowSpec{
			flowLabel: uint16(0x2000 + i),
			hopCount:  1 + (i % 5),
		})
	}

	// Phase 1: Send all BIRTHs
	for _, spec := range specs {
		baseTs := uint64(spec.flowLabel) * 100000
		wotan.send(AnamnesisTopicBirth,
			makeE2EEvent(pkgebpf.EventBirth, 0, spec.flowLabel, baseTs))
	}
	waitForIngested(t, ingestor, int64(numFlows), 5*time.Second)

	// Phase 2: Send HOPs in order (hop 1 for all, then hop 2 for those that need it, etc.)
	maxHops := 5
	ingested := int64(numFlows) // already ingested BIRTHs
	for hop := 1; hop <= maxHops; hop++ {
		for _, spec := range specs {
			if hop > spec.hopCount {
				continue
			}
			baseTs := uint64(spec.flowLabel)*100000 + uint64(hop)*1000
			wotan.send(AnamnesisTopicHop,
				makeE2EEvent(pkgebpf.EventHop, uint8(hop), spec.flowLabel, baseTs))
			ingested++
		}
		waitForIngested(t, ingestor, ingested, 5*time.Second)
	}

	// Phase 3: Send all DEATHs
	for _, spec := range specs {
		baseTs := uint64(spec.flowLabel)*100000 + uint64(spec.hopCount+1)*1000
		wotan.send(AnamnesisTopicDeath,
			makeE2EEvent(pkgebpf.EventDeath, uint8(spec.hopCount+1), spec.flowLabel, baseTs))
	}
	waitForCompleted(t, ingestor, int64(numFlows), 10*time.Second)

	// Parse flow messages from WS broadcasts
	msgs := collector.getMessages()
	var flows []*AnamnesisFlow
	for _, raw := range msgs {
		msgType, data, err := parseWSMsg(raw)
		if err != nil || msgType != "flow" {
			continue
		}
		var flow AnamnesisFlow
		if err := json.Unmarshal(data, &flow); err != nil {
			t.Errorf("unmarshal flow: %v", err)
			continue
		}
		flows = append(flows, &flow)
	}

	if len(flows) < numFlows {
		t.Fatalf("parsed flows = %d, want >= %d", len(flows), numFlows)
	}

	expectedHops := make(map[uint16]int)
	for _, spec := range specs {
		expectedHops[spec.flowLabel] = spec.hopCount
	}

	verified := 0
	for _, flow := range flows {
		expected, ok := expectedHops[flow.FlowLabel]
		if !ok {
			continue
		}

		if flow.HopCount != expected {
			t.Errorf("flow 0x%04x: HopCount = %d, want %d", flow.FlowLabel, flow.HopCount, expected)
		}
		if !flow.Complete {
			t.Errorf("flow 0x%04x: not complete", flow.FlowLabel)
		}

		expectedLatency := uint64(expected+1) * 1000
		if flow.LatencyNs != expectedLatency {
			t.Errorf("flow 0x%04x: LatencyNs = %d, want %d", flow.FlowLabel, flow.LatencyNs, expectedLatency)
		}

		// Hop latency entries = hopCount + 1 (birth→hop₁, ..., hopₙ→death)
		expectedHopLatencyLen := expected + 1
		if len(flow.HopLatency) != expectedHopLatencyLen {
			t.Errorf("flow 0x%04x: HopLatency len = %d, want %d",
				flow.FlowLabel, len(flow.HopLatency), expectedHopLatencyLen)
		}

		for j, hl := range flow.HopLatency {
			if hl.LatencyNs != 1000 {
				t.Errorf("flow 0x%04x hop[%d]: latency = %d, want 1000",
					flow.FlowLabel, j, hl.LatencyNs)
			}
		}

		verified++
	}

	t.Logf("Verified %d/%d flows with correct hop counts and latency", verified, numFlows)
	if verified < numFlows {
		t.Errorf("only verified %d flows, want %d", verified, numFlows)
	}
}

// ============================================================================
// E2E TEST: Monad CRC-16 Validation
// ============================================================================

// TestE2E_BPFDashboard_MonadCRC verifies that Monad CRC-16 is valid at each hop.
func TestE2E_BPFDashboard_MonadCRC(t *testing.T) {
	const numFlows = 20

	wotan := newMockWotanSubscriber()
	fb := NewFlowBuilder(FlowBuilderConfig{})

	var allEvents []*pkgebpf.AnamnesisEvent
	var eventMu sync.Mutex

	collector := &wsBroadcastCollector{}
	ingestor := NewAnamnesisIngestor(wotan, fb, collector.broadcast, nil)
	ingestor.OnEvent(func(ev *pkgebpf.AnamnesisEvent) {
		eventMu.Lock()
		allEvents = append(allEvents, ev)
		eventMu.Unlock()
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := ingestor.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	// Publish flows with valid CRC (phased for ordering)
	for i := 0; i < numFlows; i++ {
		fl := uint16(0x3000 + i)
		base := uint64(i) * 100000
		wotan.send(AnamnesisTopicBirth, makeE2EEvent(pkgebpf.EventBirth, 0, fl, base))
	}
	waitForIngested(t, ingestor, int64(numFlows), 5*time.Second)

	for i := 0; i < numFlows; i++ {
		fl := uint16(0x3000 + i)
		base := uint64(i)*100000 + 1000
		wotan.send(AnamnesisTopicHop, makeE2EEvent(pkgebpf.EventHop, 1, fl, base))
	}
	waitForIngested(t, ingestor, int64(numFlows*2), 5*time.Second)

	for i := 0; i < numFlows; i++ {
		fl := uint16(0x3000 + i)
		base := uint64(i)*100000 + 2000
		wotan.send(AnamnesisTopicDeath, makeE2EEvent(pkgebpf.EventDeath, 2, fl, base))
	}
	waitForIngested(t, ingestor, int64(numFlows*3), 5*time.Second)
	time.Sleep(50 * time.Millisecond)

	eventMu.Lock()
	events := make([]*pkgebpf.AnamnesisEvent, len(allEvents))
	copy(events, allEvents)
	eventMu.Unlock()

	// Verify Monad CRC-16 at each hop
	crcValid := 0
	crcInvalid := 0
	for _, ev := range events {
		decoded := DecodeMonadForDashboard(&ev.Monad)
		if decoded.ChecksumOK {
			crcValid++
		} else {
			crcInvalid++
			t.Errorf("event type=%s flowLabel=0x%04x hopID=%d: CRC invalid",
				ev.EventType, ev.FlowLabelLo, ev.HopID)
		}
	}

	t.Logf("CRC check: %d valid, %d invalid out of %d events", crcValid, crcInvalid, len(events))
	if crcInvalid != 0 {
		t.Errorf("found %d events with invalid CRC", crcInvalid)
	}
	if crcValid != numFlows*3 {
		t.Errorf("crcValid = %d, want %d", crcValid, numFlows*3)
	}
}

// ============================================================================
// E2E TEST: Trace ID Correlation (interleaved traffic)
// ============================================================================

// TestE2E_BPFDashboard_TraceCorrelation verifies that trace_ids (flow labels)
// correlate across BIRTH, HOP, and DEATH events even when interleaved.
func TestE2E_BPFDashboard_TraceCorrelation(t *testing.T) {
	const numFlows = 30

	wotan := newMockWotanSubscriber()
	fb := NewFlowBuilder(FlowBuilderConfig{})

	var allEvents []*pkgebpf.AnamnesisEvent
	var eventMu sync.Mutex

	collector := &wsBroadcastCollector{}
	ingestor := NewAnamnesisIngestor(wotan, fb, collector.broadcast, nil)
	ingestor.OnEvent(func(ev *pkgebpf.AnamnesisEvent) {
		eventMu.Lock()
		allEvents = append(allEvents, ev)
		eventMu.Unlock()
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := ingestor.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	// Phased, interleaved delivery:
	// Phase 1: all BIRTHs (interleaved across flows)
	for i := 0; i < numFlows; i++ {
		fl := uint16(0x4000 + i)
		base := uint64(i) * 100000
		wotan.send(AnamnesisTopicBirth, makeE2EEvent(pkgebpf.EventBirth, 0, fl, base))
	}
	waitForIngested(t, ingestor, int64(numFlows), 5*time.Second)

	// Phase 2: all HOP1s
	for i := 0; i < numFlows; i++ {
		fl := uint16(0x4000 + i)
		base := uint64(i)*100000 + 500
		wotan.send(AnamnesisTopicHop, makeE2EEvent(pkgebpf.EventHop, 1, fl, base))
	}
	waitForIngested(t, ingestor, int64(numFlows*2), 5*time.Second)

	// Phase 3: all HOP2s
	for i := 0; i < numFlows; i++ {
		fl := uint16(0x4000 + i)
		base := uint64(i)*100000 + 1000
		wotan.send(AnamnesisTopicHop, makeE2EEvent(pkgebpf.EventHop, 2, fl, base))
	}
	waitForIngested(t, ingestor, int64(numFlows*3), 5*time.Second)

	// Phase 4: all DEATHs
	for i := 0; i < numFlows; i++ {
		fl := uint16(0x4000 + i)
		base := uint64(i)*100000 + 1500
		wotan.send(AnamnesisTopicDeath, makeE2EEvent(pkgebpf.EventDeath, 3, fl, base))
	}
	waitForCompleted(t, ingestor, int64(numFlows), 10*time.Second)
	time.Sleep(50 * time.Millisecond)

	eventMu.Lock()
	evCopy := make([]*pkgebpf.AnamnesisEvent, len(allEvents))
	copy(evCopy, allEvents)
	eventMu.Unlock()

	// Group events by flow_label
	flowEvents := make(map[uint16][]*pkgebpf.AnamnesisEvent)
	for _, ev := range evCopy {
		flowEvents[ev.FlowLabelLo] = append(flowEvents[ev.FlowLabelLo], ev)
	}

	t.Logf("Unique flow labels: %d", len(flowEvents))
	if len(flowEvents) != numFlows {
		t.Errorf("unique flow labels = %d, want %d", len(flowEvents), numFlows)
	}

	// Each flow should have exactly 4 events: BIRTH + 2 HOPs + DEATH
	correlated := 0
	for fl, evs := range flowEvents {
		if len(evs) != 4 {
			t.Errorf("flow 0x%04x: %d events, want 4", fl, len(evs))
			continue
		}

		typeCounts := map[pkgebpf.EventType]int{}
		for _, ev := range evs {
			typeCounts[ev.EventType]++
		}

		if typeCounts[pkgebpf.EventBirth] != 1 {
			t.Errorf("flow 0x%04x: BIRTH count = %d, want 1", fl, typeCounts[pkgebpf.EventBirth])
		}
		if typeCounts[pkgebpf.EventHop] != 2 {
			t.Errorf("flow 0x%04x: HOP count = %d, want 2", fl, typeCounts[pkgebpf.EventHop])
		}
		if typeCounts[pkgebpf.EventDeath] != 1 {
			t.Errorf("flow 0x%04x: DEATH count = %d, want 1", fl, typeCounts[pkgebpf.EventDeath])
		}

		// Verify all events share the same src/dst service IDs
		for _, ev := range evs {
			if ev.Monad.SrcServiceID != 0x0A {
				t.Errorf("flow 0x%04x: SrcServiceID = 0x%02x, want 0x0A", fl, ev.Monad.SrcServiceID)
			}
			if ev.Monad.DstServiceID != 0x0B {
				t.Errorf("flow 0x%04x: DstServiceID = 0x%02x, want 0x0B", fl, ev.Monad.DstServiceID)
			}
		}

		correlated++
	}

	t.Logf("Correlated flows: %d/%d", correlated, numFlows)
	if correlated != numFlows {
		t.Errorf("correlated = %d, want %d", correlated, numFlows)
	}
}

// ============================================================================
// E2E TEST: Anomaly + Chaos Events
// ============================================================================

// TestE2E_BPFDashboard_AnomalyAndChaos verifies anomaly and chaos events
// are correctly tracked in flows and broadcast via WebSocket.
func TestE2E_BPFDashboard_AnomalyAndChaos(t *testing.T) {
	const numFlows = 10

	wotan := newMockWotanSubscriber()
	fb := NewFlowBuilder(FlowBuilderConfig{})
	collector := &wsBroadcastCollector{}
	ingestor := NewAnamnesisIngestor(wotan, fb, collector.broadcast, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := ingestor.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	// Phased: BIRTH → HOP → anomaly/chaos → DEATH
	// Phase 1: BIRTHs
	for i := 0; i < numFlows; i++ {
		fl := uint16(0x5000 + i)
		base := uint64(i) * 100000
		wotan.send(AnamnesisTopicBirth, makeE2EEvent(pkgebpf.EventBirth, 0, fl, base))
	}
	waitForIngested(t, ingestor, int64(numFlows), 5*time.Second)

	// Phase 2: HOPs
	for i := 0; i < numFlows; i++ {
		fl := uint16(0x5000 + i)
		base := uint64(i)*100000 + 500
		wotan.send(AnamnesisTopicHop, makeE2EEvent(pkgebpf.EventHop, 1, fl, base))
	}
	waitForIngested(t, ingestor, int64(numFlows*2), 5*time.Second)

	// Phase 3: Anomaly (even) or Chaos (odd)
	for i := 0; i < numFlows; i++ {
		fl := uint16(0x5000 + i)
		base := uint64(i)*100000 + 750
		if i%2 == 0 {
			wotan.send(AnamnesisTopicAnomaly, makeE2EEvent(pkgebpf.EventAnomaly, 1, fl, base))
		} else {
			wotan.send(AnamnesisTopicChaos, makeE2EEvent(pkgebpf.EventChaos, 1, fl, base))
		}
	}
	waitForIngested(t, ingestor, int64(numFlows*3), 5*time.Second)

	// Phase 4: DEATHs
	for i := 0; i < numFlows; i++ {
		fl := uint16(0x5000 + i)
		base := uint64(i)*100000 + 1000
		wotan.send(AnamnesisTopicDeath, makeE2EEvent(pkgebpf.EventDeath, 2, fl, base))
	}
	waitForCompleted(t, ingestor, int64(numFlows), 5*time.Second)
	time.Sleep(100 * time.Millisecond)

	// Verify WS message types include anomaly and chaos
	msgs := collector.getMessages()
	typeCounts := map[string]int{}
	for _, raw := range msgs {
		msgType, _, err := parseWSMsg(raw)
		if err != nil {
			continue
		}
		typeCounts[msgType]++
	}

	t.Logf("WS message types: %v", typeCounts)

	if typeCounts["anomaly"] < numFlows/2 {
		t.Errorf("anomaly WS messages = %d, want >= %d", typeCounts["anomaly"], numFlows/2)
	}
	if typeCounts["chaos"] < numFlows/2 {
		t.Errorf("chaos WS messages = %d, want >= %d", typeCounts["chaos"], numFlows/2)
	}

	// Verify completed flow records contain anomalies
	flowsWithAnomalies := 0
	for _, raw := range msgs {
		msgType, data, err := parseWSMsg(raw)
		if err != nil || msgType != "flow" {
			continue
		}
		var flow AnamnesisFlow
		if err := json.Unmarshal(data, &flow); err != nil {
			continue
		}
		if len(flow.Anomalies) > 0 {
			flowsWithAnomalies++
		}
	}

	t.Logf("Flows with anomalies: %d/%d", flowsWithAnomalies, numFlows)
	if flowsWithAnomalies != numFlows {
		t.Errorf("flowsWithAnomalies = %d, want %d", flowsWithAnomalies, numFlows)
	}
}

// ============================================================================
// E2E TEST: Monad Decode (all combinations)
// ============================================================================

// TestE2E_BPFDashboard_MonadDecode verifies the Monad decoder produces correct
// dashboard-friendly output for all flow actions, circuit states, and deploy rings.
func TestE2E_BPFDashboard_MonadDecode(t *testing.T) {
	tests := []struct {
		name         string
		flowAction   uint8
		circuitState uint8
		deployRing   uint8
		flags        uint8
		wantAction   string
		wantCircuit  string
		wantRing     string
		wantFlags    int
	}{
		{"forward/closed/prod", 0x01, 0x01, 0x01, pkgebpf.FlagTraced, "forward", "closed", "production", 1},
		{"trace/open/canary", 0x02, 0x02, 0x02, pkgebpf.FlagTraced | pkgebpf.FlagCanary, "trace", "open", "canary", 2},
		{"sample/half_open/staging", 0x03, 0x03, 0x03, pkgebpf.FlagSampled, "sample", "half_open", "staging", 1},
		{"mirror/closed/dev", 0x04, 0x01, 0x04, pkgebpf.FlagMirror, "mirror", "closed", "development", 1},
		{"drop/open/prod", 0x05, 0x02, 0x01, pkgebpf.FlagChaos | pkgebpf.FlagEncrypt, "drop", "open", "production", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &pkgebpf.MonadState{
				Version:      pkgebpf.MonadVersion,
				SrcServiceID: 0x0A,
				DstServiceID: 0x0B,
				FlowAction:   tt.flowAction,
				CircuitState: tt.circuitState,
				DeployRing:   tt.deployRing,
				Flags:        tt.flags,
				Scratch:      [4]uint8{0xDE, 0xAD, 0xBE, 0xEF},
			}
			m.RecomputeChecksum()

			decoded := DecodeMonadForDashboard(m)

			if decoded.FlowAction != tt.wantAction {
				t.Errorf("FlowAction = %q, want %q", decoded.FlowAction, tt.wantAction)
			}
			if decoded.CircuitState != tt.wantCircuit {
				t.Errorf("CircuitState = %q, want %q", decoded.CircuitState, tt.wantCircuit)
			}
			if decoded.DeployRing != tt.wantRing {
				t.Errorf("DeployRing = %q, want %q", decoded.DeployRing, tt.wantRing)
			}
			if len(decoded.Flags) != tt.wantFlags {
				t.Errorf("Flags count = %d, want %d (flags=%v)", len(decoded.Flags), tt.wantFlags, decoded.Flags)
			}
			if !decoded.ChecksumOK {
				t.Error("ChecksumOK = false, want true")
			}
			if decoded.ScratchR0 != 0xDEAD {
				t.Errorf("ScratchR0 = 0x%04x, want 0xDEAD", decoded.ScratchR0)
			}
			if decoded.ScratchR1 != 0xBEEF {
				t.Errorf("ScratchR1 = 0x%04x, want 0xBEEF", decoded.ScratchR1)
			}
		})
	}
}

// ============================================================================
// E2E TEST: Throughput
// ============================================================================

// TestE2E_BPFDashboard_Throughput benchmarks the pipeline processing throughput
// using phased event delivery.
func TestE2E_BPFDashboard_Throughput(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping throughput test in short mode")
	}

	const numFlows = 1000

	wotan := newMockWotanSubscriber()
	fb := NewFlowBuilder(FlowBuilderConfig{})
	collector := &wsBroadcastCollector{}
	ingestor := NewAnamnesisIngestor(wotan, fb, collector.broadcast, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := ingestor.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	start := time.Now()

	// Phase 1: BIRTHs
	for i := 0; i < numFlows; i++ {
		fl := uint16(i)
		base := uint64(i) * 10000
		wotan.send(AnamnesisTopicBirth, makeE2EEvent(pkgebpf.EventBirth, 0, fl, base))
	}
	waitForIngested(t, ingestor, int64(numFlows), 10*time.Second)

	// Phase 2: HOPs
	for i := 0; i < numFlows; i++ {
		fl := uint16(i)
		base := uint64(i)*10000 + 1000
		wotan.send(AnamnesisTopicHop, makeE2EEvent(pkgebpf.EventHop, 1, fl, base))
	}
	waitForIngested(t, ingestor, int64(numFlows*2), 10*time.Second)

	// Phase 3: DEATHs
	for i := 0; i < numFlows; i++ {
		fl := uint16(i)
		base := uint64(i)*10000 + 2000
		wotan.send(AnamnesisTopicDeath, makeE2EEvent(pkgebpf.EventDeath, 2, fl, base))
	}
	waitForCompleted(t, ingestor, int64(numFlows), 30*time.Second)

	elapsed := time.Since(start)
	eventsPerSec := float64(numFlows*3) / elapsed.Seconds()

	t.Logf("Processed %d flows (%d events) in %v", numFlows, numFlows*3, elapsed)
	t.Logf("Throughput: %.0f events/sec", eventsPerSec)

	stats := ingestor.Stats()
	if stats.FlowBuilder.CompletedFlows != int64(numFlows) {
		t.Errorf("CompletedFlows = %d, want %d", stats.FlowBuilder.CompletedFlows, numFlows)
	}

	fmt.Printf("BPF Dashboard E2E throughput: %.0f events/sec (%d flows in %v)\n",
		eventsPerSec, numFlows, elapsed.Round(time.Millisecond))
}
