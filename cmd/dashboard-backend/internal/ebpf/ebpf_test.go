package ebpf

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	busboyClient "unheaded/pkg/busboy-client"
)

// === Type Parsing Tests ===

func TestParsePacketEvent(t *testing.T) {
	raw := `{"timestamp_ns":1000000000,"trace_id":{"high":1,"low":2},"flow_key":{"src_addr":"10.0.0.1","dst_addr":"10.0.0.2","src_port":8080,"dst_port":443,"protocol":6},"packet_len":1500,"action":"pass","direction":"ingress"}`
	e, err := ParsePacketEvent([]byte(raw))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if e.PacketLen != 1500 {
		t.Errorf("packet_len = %d, want 1500", e.PacketLen)
	}
	if e.FlowKey.SrcAddr != "10.0.0.1" {
		t.Errorf("src_addr = %q, want 10.0.0.1", e.FlowKey.SrcAddr)
	}
	if e.Action != ActionPass {
		t.Errorf("action = %q, want pass", e.Action)
	}
	if e.Direction != DirectionIngress {
		t.Errorf("direction = %q, want ingress", e.Direction)
	}
	if e.TraceID.High != 1 || e.TraceID.Low != 2 {
		t.Errorf("trace_id = %v, want {1,2}", e.TraceID)
	}
}

func TestParseFlowEvent(t *testing.T) {
	raw := `{"timestamp_ns":2000000000,"flow_key":{"src_addr":"10.0.0.1","dst_addr":"10.0.0.2","src_port":8080,"dst_port":443,"protocol":6},"flow_state":{"trace_id":{"high":1,"low":2},"start_ns":1000000000,"last_seen_ns":2000000000,"packets_in":10,"packets_out":5,"bytes_in":15000,"bytes_out":2500,"state":"established"},"event_type":"state_change"}`
	e, err := ParseFlowEvent([]byte(raw))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if e.EventType != FlowStateChange {
		t.Errorf("event_type = %q, want state_change", e.EventType)
	}
	if e.FlowState.State != StateEstablished {
		t.Errorf("state = %q, want established", e.FlowState.State)
	}
	if e.FlowState.PacketsIn != 10 {
		t.Errorf("packets_in = %d, want 10", e.FlowState.PacketsIn)
	}
}

func TestParseLatencyEvent(t *testing.T) {
	raw := `{"timestamp_ns":3000000000,"trace_id":{"high":0,"low":42},"pid":1234,"tid":1234,"latency_ns":500000,"operation":"tcp_send"}`
	e, err := ParseLatencyEvent([]byte(raw))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if e.Operation != OpTcpSend {
		t.Errorf("operation = %q, want tcp_send", e.Operation)
	}
	if e.LatencyNs != 500000 {
		t.Errorf("latency_ns = %d, want 500000", e.LatencyNs)
	}
	if e.LatencyDuration() != 500*time.Microsecond {
		t.Errorf("LatencyDuration() = %v, want 500µs", e.LatencyDuration())
	}
}

func TestParseSyscallEvent(t *testing.T) {
	raw := `{"timestamp_ns":4000000000,"pid":100,"tid":100,"uid":1000,"gid":1000,"syscall_nr":1,"args":[0,0,0,0,0,0],"ret":0,"comm":"curl","event_type":"exit"}`
	e, err := ParseSyscallEvent([]byte(raw))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if e.Comm != "curl" {
		t.Errorf("comm = %q, want curl", e.Comm)
	}
	if e.EventType != SyscallExit {
		t.Errorf("event_type = %q, want exit", e.EventType)
	}
}

func TestParseInvalidJSON(t *testing.T) {
	if _, err := ParsePacketEvent([]byte("not json")); err == nil {
		t.Error("expected error for invalid JSON")
	}
	if _, err := ParseFlowEvent([]byte("{")); err == nil {
		t.Error("expected error for incomplete JSON")
	}
}

// === FlowGraph Tests ===

func TestFlowGraph_IngestPacket(t *testing.T) {
	fg := NewFlowGraph(FlowGraphConfig{TTL: 30 * time.Second, MaxFlows: 100})

	e := &PacketEvent{
		TimestampNs: uint64(time.Now().UnixNano()),
		FlowKey:     FlowKey{SrcAddr: "10.0.0.1", DstAddr: "10.0.0.2", SrcPort: 8080, DstPort: 443, Protocol: 6},
		PacketLen:   1500,
		Direction:   DirectionIngress,
	}

	fg.IngestPacket(e)
	if fg.ActiveFlowCount() != 1 {
		t.Fatalf("active flows = %d, want 1", fg.ActiveFlowCount())
	}

	flows := fg.GetActiveFlows()
	if flows[0].PacketsIn != 1 {
		t.Errorf("packets_in = %d, want 1", flows[0].PacketsIn)
	}
	if flows[0].BytesIn != 1500 {
		t.Errorf("bytes_in = %d, want 1500", flows[0].BytesIn)
	}

	// Add egress packet to same flow
	e2 := &PacketEvent{
		TimestampNs: uint64(time.Now().UnixNano()),
		FlowKey:     e.FlowKey,
		PacketLen:   500,
		Direction:   DirectionEgress,
	}
	fg.IngestPacket(e2)
	if fg.ActiveFlowCount() != 1 {
		t.Fatalf("active flows = %d, want 1 (same flow)", fg.ActiveFlowCount())
	}

	flows = fg.GetActiveFlows()
	if flows[0].PacketsOut != 1 || flows[0].BytesOut != 500 {
		t.Errorf("egress: packets=%d bytes=%d, want 1/500", flows[0].PacketsOut, flows[0].BytesOut)
	}
}

func TestFlowGraph_IngestFlow(t *testing.T) {
	fg := NewFlowGraph(FlowGraphConfig{})

	e := &FlowEvent{
		TimestampNs: uint64(time.Now().UnixNano()),
		FlowKey:     FlowKey{SrcAddr: "10.0.0.1", DstAddr: "10.0.0.2", SrcPort: 8080, DstPort: 443, Protocol: 6},
		FlowState:   FlowState{State: StateEstablished, PacketsIn: 10, BytesIn: 15000},
		EventType:   FlowNew,
	}
	fg.IngestFlow(e)
	if fg.ActiveFlowCount() != 1 {
		t.Fatalf("active flows = %d, want 1", fg.ActiveFlowCount())
	}

	// Close the flow
	e2 := &FlowEvent{
		TimestampNs: uint64(time.Now().UnixNano()),
		FlowKey:     e.FlowKey,
		FlowState:   FlowState{State: StateClosed},
		EventType:   FlowExpired,
	}
	fg.IngestFlow(e2)
	if fg.ActiveFlowCount() != 0 {
		t.Fatalf("active flows = %d, want 0 after close", fg.ActiveFlowCount())
	}
}

func TestFlowGraph_Expire(t *testing.T) {
	fg := NewFlowGraph(FlowGraphConfig{TTL: 10 * time.Millisecond})

	e := &PacketEvent{
		TimestampNs: uint64(time.Now().Add(-50 * time.Millisecond).UnixNano()),
		FlowKey:     FlowKey{SrcAddr: "10.0.0.1", DstAddr: "10.0.0.2", SrcPort: 1, DstPort: 2, Protocol: 6},
		PacketLen:   100,
		Direction:   DirectionIngress,
	}
	fg.IngestPacket(e)

	expired := fg.Expire()
	if expired != 1 {
		t.Errorf("expired = %d, want 1", expired)
	}
	if fg.ActiveFlowCount() != 0 {
		t.Errorf("active flows = %d, want 0 after expire", fg.ActiveFlowCount())
	}
}

func TestFlowGraph_MaxFlows(t *testing.T) {
	fg := NewFlowGraph(FlowGraphConfig{MaxFlows: 2, TTL: time.Minute})

	for i := 0; i < 3; i++ {
		e := &PacketEvent{
			TimestampNs: uint64(time.Now().UnixNano()),
			FlowKey:     FlowKey{SrcAddr: "10.0.0.1", DstAddr: "10.0.0.2", SrcPort: uint16(i + 1), DstPort: 443, Protocol: 6},
			PacketLen:   100,
			Direction:   DirectionIngress,
		}
		fg.IngestPacket(e)
	}

	if fg.ActiveFlowCount() > 2 {
		t.Errorf("active flows = %d, want <= 2 (max)", fg.ActiveFlowCount())
	}
}

func TestFlowGraph_Stats(t *testing.T) {
	fg := NewFlowGraph(FlowGraphConfig{})

	e := &PacketEvent{
		TimestampNs: uint64(time.Now().UnixNano()),
		FlowKey:     FlowKey{SrcAddr: "10.0.0.1", DstAddr: "10.0.0.2", SrcPort: 80, DstPort: 443, Protocol: 6},
		PacketLen:   1000,
		Direction:   DirectionIngress,
	}
	fg.IngestPacket(e)
	fg.IngestPacket(e)

	stats := fg.Stats()
	if stats.TotalPackets != 2 {
		t.Errorf("total_packets = %d, want 2", stats.TotalPackets)
	}
	if stats.TotalBytes != 2000 {
		t.Errorf("total_bytes = %d, want 2000", stats.TotalBytes)
	}
	if stats.TotalFlowsSeen != 1 {
		t.Errorf("total_flows_seen = %d, want 1", stats.TotalFlowsSeen)
	}
}

// === LatencyHistogram Tests ===

func TestLatencyHistogram_Ingest(t *testing.T) {
	lh := NewLatencyHistogram(LatencyHistogramConfig{
		Windows: []time.Duration{1 * time.Second, 10 * time.Second},
	})

	now := time.Now()
	for i := 0; i < 100; i++ {
		lh.Ingest(&LatencyEvent{
			TimestampNs: uint64(now.UnixNano()),
			LatencyNs:   uint64(i * 1000), // 0µs to 99µs
			Operation:   OpTcpSend,
		})
	}

	result := lh.GetPercentiles(OpTcpSend, 1*time.Second)
	if result.SampleCount != 100 {
		t.Fatalf("sample_count = %d, want 100", result.SampleCount)
	}
	if result.P50Ns == 0 {
		t.Error("P50 should be > 0")
	}
	if result.P90Ns <= result.P50Ns {
		t.Errorf("P90 (%d) should be > P50 (%d)", result.P90Ns, result.P50Ns)
	}
	if result.P99Ns <= result.P90Ns {
		t.Errorf("P99 (%d) should be > P90 (%d)", result.P99Ns, result.P90Ns)
	}
	if result.MinNs != 0 {
		t.Errorf("min = %d, want 0", result.MinNs)
	}
	if result.MaxNs != 99000 {
		t.Errorf("max = %d, want 99000", result.MaxNs)
	}
}

func TestLatencyHistogram_Expire(t *testing.T) {
	lh := NewLatencyHistogram(LatencyHistogramConfig{
		Windows: []time.Duration{50 * time.Millisecond},
	})

	old := time.Now().Add(-100 * time.Millisecond)
	lh.Ingest(&LatencyEvent{
		TimestampNs: uint64(old.UnixNano()),
		LatencyNs:   1000,
		Operation:   OpTcpRecv,
	})

	lh.Expire()
	result := lh.GetPercentiles(OpTcpRecv, 50*time.Millisecond)
	if result.SampleCount != 0 {
		t.Errorf("sample_count = %d after expire, want 0", result.SampleCount)
	}
}

func TestLatencyHistogram_MultipleOps(t *testing.T) {
	lh := NewLatencyHistogram(LatencyHistogramConfig{})

	now := time.Now()
	lh.Ingest(&LatencyEvent{TimestampNs: uint64(now.UnixNano()), LatencyNs: 1000, Operation: OpTcpSend})
	lh.Ingest(&LatencyEvent{TimestampNs: uint64(now.UnixNano()), LatencyNs: 2000, Operation: OpTcpRecv})
	lh.Ingest(&LatencyEvent{TimestampNs: uint64(now.UnixNano()), LatencyNs: 3000, Operation: OpTcpConnect})

	all := lh.GetAllPercentiles()
	if len(all) != 4 {
		t.Fatalf("expected 4 operations, got %d", len(all))
	}
	if all[OpTcpSend][0].SampleCount != 1 {
		t.Errorf("tcp_send sample_count = %d, want 1", all[OpTcpSend][0].SampleCount)
	}
	if all[OpTcpAccept][0].SampleCount != 0 {
		t.Errorf("tcp_accept sample_count = %d, want 0", all[OpTcpAccept][0].SampleCount)
	}
}

// === EventRing Tests ===

func TestEventRing_AddAndRecent(t *testing.T) {
	r := NewEventRing(5)

	for i := 0; i < 10; i++ {
		r.Add(EventEnvelope{Type: "test", Timestamp: time.Now()})
	}

	if r.Count() != 5 {
		t.Errorf("count = %d, want 5 (capped)", r.Count())
	}

	recent := r.Recent(3)
	if len(recent) != 3 {
		t.Errorf("recent(3) = %d items, want 3", len(recent))
	}
}

func TestEventRing_RecentMoreThanSize(t *testing.T) {
	r := NewEventRing(3)
	r.Add(EventEnvelope{Type: "a"})
	r.Add(EventEnvelope{Type: "b"})

	recent := r.Recent(10) // Ask for more than exist
	if len(recent) != 2 {
		t.Errorf("recent(10) = %d items, want 2", len(recent))
	}
}

// === Ingestor Tests ===

// mockBusboyClient implements BusboySubscriber for testing.
type mockBusboyClient struct {
	channels map[string]chan *busboyClient.Message
	mu       sync.Mutex
}

func newMockBusboyClient() *mockBusboyClient {
	return &mockBusboyClient{
		channels: make(map[string]chan *busboyClient.Message),
	}
}

func (m *mockBusboyClient) Subscribe(_ context.Context, topic, _ string) (*busboyClient.Subscriber, error) {
	return &busboyClient.Subscriber{SubscriberID: "test", Status: "active"}, nil
}

func (m *mockBusboyClient) StreamMessages(_ context.Context, topic string) (<-chan *busboyClient.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch := make(chan *busboyClient.Message, 100)
	m.channels[topic] = ch
	return ch, nil
}

func (m *mockBusboyClient) Close() error { return nil }

func (m *mockBusboyClient) send(topic string, payload string) {
	m.mu.Lock()
	ch, ok := m.channels[topic]
	m.mu.Unlock()
	if ok {
		ch <- &busboyClient.Message{
			Topic:   topic,
			Payload: payload,
		}
	}
}

func TestIngestor_PacketIngestion(t *testing.T) {
	mock := newMockBusboyClient()
	config := DefaultIngestorConfig()
	config.ExpireInterval = time.Hour // Don't expire during test
	ing := NewIngestor(config, mock, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var received []EventEnvelope
	var mu sync.Mutex
	ing.OnEvent(func(e EventEnvelope) {
		mu.Lock()
		received = append(received, e)
		mu.Unlock()
	})

	if err := ing.Start(ctx); err != nil {
		t.Fatalf("start error: %v", err)
	}

	// Give goroutines time to start streaming
	time.Sleep(50 * time.Millisecond)

	pkt := PacketEvent{
		TimestampNs: uint64(time.Now().UnixNano()),
		TraceID:     TraceID{High: 1, Low: 2},
		FlowKey:     FlowKey{SrcAddr: "10.0.0.1", DstAddr: "10.0.0.2", SrcPort: 80, DstPort: 443, Protocol: 6},
		PacketLen:   1500,
		Action:      ActionPass,
		Direction:   DirectionIngress,
	}
	data, _ := json.Marshal(pkt)
	mock.send("ebpf.packet.events", string(data))

	// Wait for processing
	time.Sleep(50 * time.Millisecond)

	stats := ing.Stats()
	if stats.PacketsIngested != 1 {
		t.Errorf("packets_ingested = %d, want 1", stats.PacketsIngested)
	}
	if ing.FlowGraph().ActiveFlowCount() != 1 {
		t.Errorf("active flows = %d, want 1", ing.FlowGraph().ActiveFlowCount())
	}

	mu.Lock()
	if len(received) != 1 {
		t.Errorf("listener received %d events, want 1", len(received))
	} else if received[0].Type != "packet" {
		t.Errorf("event type = %q, want packet", received[0].Type)
	}
	mu.Unlock()
}

func TestIngestor_LatencyIngestion(t *testing.T) {
	mock := newMockBusboyClient()
	config := DefaultIngestorConfig()
	config.ExpireInterval = time.Hour
	ing := NewIngestor(config, mock, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := ing.Start(ctx); err != nil {
		t.Fatalf("start error: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	lat := LatencyEvent{
		TimestampNs: uint64(time.Now().UnixNano()),
		LatencyNs:   500000,
		Operation:   OpTcpSend,
		PID:         1234,
	}
	data, _ := json.Marshal(lat)
	mock.send("ebpf.latency.events", string(data))
	time.Sleep(50 * time.Millisecond)

	stats := ing.Stats()
	if stats.LatencyIngested != 1 {
		t.Errorf("latency_ingested = %d, want 1", stats.LatencyIngested)
	}

	p := ing.LatencyHistogram().GetPercentiles(OpTcpSend, 1*time.Second)
	if p.SampleCount != 1 {
		t.Errorf("sample_count = %d, want 1", p.SampleCount)
	}
	if p.P50Ns != 500000 {
		t.Errorf("P50 = %d, want 500000", p.P50Ns)
	}
}

func TestIngestor_FlowIngestion(t *testing.T) {
	mock := newMockBusboyClient()
	config := DefaultIngestorConfig()
	config.ExpireInterval = time.Hour
	ing := NewIngestor(config, mock, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := ing.Start(ctx); err != nil {
		t.Fatalf("start error: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	fe := FlowEvent{
		TimestampNs: uint64(time.Now().UnixNano()),
		FlowKey:     FlowKey{SrcAddr: "10.0.0.1", DstAddr: "10.0.0.2", SrcPort: 80, DstPort: 443, Protocol: 6},
		FlowState:   FlowState{State: StateEstablished, PacketsIn: 100, BytesIn: 150000},
		EventType:   FlowNew,
	}
	data, _ := json.Marshal(fe)
	mock.send("ebpf.flow.events", string(data))
	time.Sleep(50 * time.Millisecond)

	stats := ing.Stats()
	if stats.FlowsIngested != 1 {
		t.Errorf("flows_ingested = %d, want 1", stats.FlowsIngested)
	}
	if ing.FlowGraph().ActiveFlowCount() != 1 {
		t.Errorf("active flows = %d, want 1", ing.FlowGraph().ActiveFlowCount())
	}
}

func TestIngestor_ParseErrors(t *testing.T) {
	mock := newMockBusboyClient()
	config := DefaultIngestorConfig()
	config.ExpireInterval = time.Hour
	ing := NewIngestor(config, mock, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := ing.Start(ctx); err != nil {
		t.Fatalf("start error: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	mock.send("ebpf.packet.events", "not valid json")
	time.Sleep(50 * time.Millisecond)

	stats := ing.Stats()
	if stats.ParseErrors != 1 {
		t.Errorf("parse_errors = %d, want 1", stats.ParseErrors)
	}
}

func TestIngestor_RecentEvents(t *testing.T) {
	mock := newMockBusboyClient()
	config := DefaultIngestorConfig()
	config.EventBufferSize = 5
	config.ExpireInterval = time.Hour
	ing := NewIngestor(config, mock, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := ing.Start(ctx); err != nil {
		t.Fatalf("start error: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	// Send 3 packets
	for i := 0; i < 3; i++ {
		pkt := PacketEvent{
			TimestampNs: uint64(time.Now().UnixNano()),
			FlowKey:     FlowKey{SrcAddr: "10.0.0.1", DstAddr: "10.0.0.2", SrcPort: uint16(i + 1), DstPort: 443, Protocol: 6},
			PacketLen:   100,
			Direction:   DirectionIngress,
		}
		data, _ := json.Marshal(pkt)
		mock.send("ebpf.packet.events", string(data))
	}
	time.Sleep(100 * time.Millisecond)

	recent := ing.RecentEvents(10)
	if len(recent) != 3 {
		t.Errorf("recent events = %d, want 3", len(recent))
	}
}

// === Concurrency Tests ===

func TestFlowGraph_Concurrent(t *testing.T) {
	fg := NewFlowGraph(FlowGraphConfig{MaxFlows: 100})

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				fg.IngestPacket(&PacketEvent{
					TimestampNs: uint64(time.Now().UnixNano()),
					FlowKey:     FlowKey{SrcAddr: "10.0.0.1", DstAddr: "10.0.0.2", SrcPort: uint16(id), DstPort: uint16(j), Protocol: 6},
					PacketLen:   100,
					Direction:   DirectionIngress,
				})
			}
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = fg.GetActiveFlows()
				_ = fg.Stats()
				fg.Expire()
			}
		}()
	}

	wg.Wait()
}

func TestLatencyHistogram_Concurrent(t *testing.T) {
	lh := NewLatencyHistogram(LatencyHistogramConfig{})

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				lh.Ingest(&LatencyEvent{
					TimestampNs: uint64(time.Now().UnixNano()),
					LatencyNs:   uint64(j * 1000),
					Operation:   OpTcpSend,
				})
			}
		}()
	}

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = lh.GetPercentiles(OpTcpSend, 1*time.Second)
				_ = lh.GetAllPercentiles()
				lh.Expire()
			}
		}()
	}

	wg.Wait()
}
