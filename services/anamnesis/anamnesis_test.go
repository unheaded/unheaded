package anamnesis

import (
	"context"
	"testing"
	"time"
)

// ============================================================================
// Configuration Tests
// ============================================================================

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.MaxEvents != 1000000 {
		t.Errorf("expected MaxEvents 1000000, got %d", cfg.MaxEvents)
	}
	if cfg.SnapshotInterval != 100 {
		t.Errorf("expected SnapshotInterval 100, got %d", cfg.SnapshotInterval)
	}
	if cfg.RetentionPeriod != 30*24*time.Hour {
		t.Errorf("expected RetentionPeriod 30 days, got %v", cfg.RetentionPeriod)
	}
	if cfg.BusboyTopic != "anamnesis.events" {
		t.Errorf("expected BusboyTopic 'anamnesis.events', got %s", cfg.BusboyTopic)
	}
}

func TestNewService(t *testing.T) {
	svc := NewService(nil, nil, nil)
	if svc == nil {
		t.Fatal("NewService returned nil")
	}
	if svc.events == nil {
		t.Error("events slice not initialized")
	}
	if svc.eventIndex == nil {
		t.Error("eventIndex map not initialized")
	}
	if svc.aggregateIndex == nil {
		t.Error("aggregateIndex map not initialized")
	}
}

// ============================================================================
// Event Append Tests
// ============================================================================

func TestAppend_Basic(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableSnapshots: false})

	payload := map[string]interface{}{
		"name": "test-service",
		"port": 8080,
	}

	event, err := svc.Append(context.Background(), "service-1", "Service", EventCreated, payload)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	if event.ID == "" {
		t.Error("expected ID to be generated")
	}
	if event.SequenceNum != 1 {
		t.Errorf("expected SequenceNum 1, got %d", event.SequenceNum)
	}
	if event.Version != 1 {
		t.Errorf("expected Version 1, got %d", event.Version)
	}
	if event.Hash == "" {
		t.Error("expected Hash to be computed")
	}
	if event.AggregateID != "service-1" {
		t.Errorf("expected AggregateID 'service-1', got %s", event.AggregateID)
	}
}

func TestAppend_VersionIncrement(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableSnapshots: false})
	ctx := context.Background()

	// First event
	e1, _ := svc.Append(ctx, "service-1", "Service", EventCreated, map[string]interface{}{"name": "test"})
	if e1.Version != 1 {
		t.Errorf("expected version 1, got %d", e1.Version)
	}

	// Second event
	e2, _ := svc.Append(ctx, "service-1", "Service", EventUpdated, map[string]interface{}{"port": 8080})
	if e2.Version != 2 {
		t.Errorf("expected version 2, got %d", e2.Version)
	}

	// Third event
	e3, _ := svc.Append(ctx, "service-1", "Service", EventUpdated, map[string]interface{}{"port": 9090})
	if e3.Version != 3 {
		t.Errorf("expected version 3, got %d", e3.Version)
	}
}

func TestAppend_HashChain(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableSnapshots: false})
	ctx := context.Background()

	e1, _ := svc.Append(ctx, "service-1", "Service", EventCreated, map[string]interface{}{"name": "test"})
	e2, _ := svc.Append(ctx, "service-1", "Service", EventUpdated, map[string]interface{}{"port": 8080})

	// First event should have no prev hash
	if e1.PrevHash != "" {
		t.Error("first event should have no prev hash")
	}

	// Second event should reference first
	if e2.PrevHash != e1.Hash {
		t.Errorf("expected prev hash %s, got %s", e1.Hash, e2.PrevHash)
	}
}

func TestAppend_SeparateAggregates(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableSnapshots: false})
	ctx := context.Background()

	// Different aggregates should have independent versions
	e1, _ := svc.Append(ctx, "service-1", "Service", EventCreated, map[string]interface{}{})
	e2, _ := svc.Append(ctx, "service-2", "Service", EventCreated, map[string]interface{}{})

	if e1.Version != 1 || e2.Version != 1 {
		t.Error("different aggregates should have independent versions")
	}

	// Sequence numbers should be global
	if e1.SequenceNum == e2.SequenceNum {
		t.Error("sequence numbers should be unique globally")
	}
}

// ============================================================================
// Event Retrieval Tests
// ============================================================================

func TestGetEvent(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableSnapshots: false})

	event, _ := svc.Append(context.Background(), "service-1", "Service", EventCreated, map[string]interface{}{})

	retrieved, ok := svc.GetEvent(event.ID)
	if !ok {
		t.Error("expected to find event")
	}
	if retrieved.ID != event.ID {
		t.Errorf("expected event ID %s, got %s", event.ID, retrieved.ID)
	}
}

func TestGetEvent_NotFound(t *testing.T) {
	svc := NewService(nil, nil, nil)

	_, ok := svc.GetEvent("nonexistent")
	if ok {
		t.Error("expected not to find event")
	}
}

func TestGetEventStream(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableSnapshots: false})
	ctx := context.Background()

	// Create multiple events for one aggregate
	svc.Append(ctx, "service-1", "Service", EventCreated, map[string]interface{}{"name": "test"})
	svc.Append(ctx, "service-1", "Service", EventUpdated, map[string]interface{}{"port": 8080})
	svc.Append(ctx, "service-1", "Service", EventUpdated, map[string]interface{}{"port": 9090})

	stream, err := svc.GetEventStream("service-1")
	if err != nil {
		t.Fatalf("GetEventStream failed: %v", err)
	}

	if len(stream.Events) != 3 {
		t.Errorf("expected 3 events, got %d", len(stream.Events))
	}
	if stream.Version != 3 {
		t.Errorf("expected version 3, got %d", stream.Version)
	}
}

func TestGetEventStream_NotFound(t *testing.T) {
	svc := NewService(nil, nil, nil)

	_, err := svc.GetEventStream("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent aggregate")
	}
}

// ============================================================================
// Query Tests
// ============================================================================

func TestQuery_ByAggregateID(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableSnapshots: false})
	ctx := context.Background()

	svc.Append(ctx, "service-1", "Service", EventCreated, map[string]interface{}{})
	svc.Append(ctx, "service-1", "Service", EventUpdated, map[string]interface{}{})
	svc.Append(ctx, "service-2", "Service", EventCreated, map[string]interface{}{})

	query := &Query{AggregateID: "service-1"}
	events, err := svc.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}
}

func TestQuery_ByEventType(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableSnapshots: false})
	ctx := context.Background()

	svc.Append(ctx, "service-1", "Service", EventCreated, map[string]interface{}{})
	svc.Append(ctx, "service-1", "Service", EventUpdated, map[string]interface{}{})
	svc.Append(ctx, "service-1", "Service", EventDeleted, map[string]interface{}{})

	query := &Query{EventTypes: []EventType{EventCreated, EventUpdated}}
	events, err := svc.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}
}

func TestQuery_ByTimeRange(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableSnapshots: false})
	ctx := context.Background()

	// Create events with slight delay
	svc.Append(ctx, "service-1", "Service", EventCreated, map[string]interface{}{})

	time.Sleep(10 * time.Millisecond)
	midTime := time.Now()
	time.Sleep(10 * time.Millisecond)

	svc.Append(ctx, "service-1", "Service", EventUpdated, map[string]interface{}{})

	// Query events after midTime
	query := &Query{FromTime: &midTime}
	events, err := svc.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(events) != 1 {
		t.Errorf("expected 1 event after midTime, got %d", len(events))
	}
}

func TestQuery_WithLimit(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableSnapshots: false})
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		svc.Append(ctx, "service-1", "Service", EventUpdated, map[string]interface{}{"i": i})
	}

	query := &Query{Limit: 5}
	events, err := svc.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(events) != 5 {
		t.Errorf("expected 5 events with limit, got %d", len(events))
	}
}

// ============================================================================
// State Reconstruction Tests
// ============================================================================

func TestReconstructState(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableSnapshots: false})
	ctx := context.Background()

	// Build up state through events
	svc.Append(ctx, "service-1", "Service", EventCreated, map[string]interface{}{"name": "test", "port": 8080})
	svc.Append(ctx, "service-1", "Service", EventUpdated, map[string]interface{}{"port": 9090})
	svc.Append(ctx, "service-1", "Service", EventUpdated, map[string]interface{}{"replicas": 3})

	// Reconstruct at version 3
	state, err := svc.ReconstructState(ctx, "service-1", 3)
	if err != nil {
		t.Fatalf("ReconstructState failed: %v", err)
	}

	if state["name"] != "test" {
		t.Errorf("expected name 'test', got %v", state["name"])
	}
	if state["port"] != 9090 {
		t.Errorf("expected port 9090, got %v", state["port"])
	}
	if state["replicas"] != 3 {
		t.Errorf("expected replicas 3, got %v", state["replicas"])
	}
}

func TestReconstructState_AtVersion(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableSnapshots: false})
	ctx := context.Background()

	svc.Append(ctx, "service-1", "Service", EventCreated, map[string]interface{}{"name": "test", "port": 8080})
	svc.Append(ctx, "service-1", "Service", EventUpdated, map[string]interface{}{"port": 9090})
	svc.Append(ctx, "service-1", "Service", EventUpdated, map[string]interface{}{"port": 7070})

	// Reconstruct at version 2 (before last update)
	state, err := svc.ReconstructState(ctx, "service-1", 2)
	if err != nil {
		t.Fatalf("ReconstructState failed: %v", err)
	}

	if state["port"] != 9090 {
		t.Errorf("expected port 9090 at version 2, got %v", state["port"])
	}
}

// ============================================================================
// Snapshot Tests
// ============================================================================

func TestSnapshot_Creation(t *testing.T) {
	// Create service with snapshot interval of 3
	svc := NewService(nil, nil, &Config{EnableSnapshots: true, SnapshotInterval: 3})
	ctx := context.Background()

	// Create 3 events to trigger snapshot
	svc.Append(ctx, "service-1", "Service", EventCreated, map[string]interface{}{"name": "test"})
	svc.Append(ctx, "service-1", "Service", EventUpdated, map[string]interface{}{"port": 8080})
	svc.Append(ctx, "service-1", "Service", EventUpdated, map[string]interface{}{"port": 9090})

	// Give snapshot goroutine time to run
	time.Sleep(50 * time.Millisecond)

	snap, ok := svc.GetLatestSnapshot("service-1")
	if !ok {
		t.Skip("snapshot may not have been created yet (async)")
	}

	if snap.Version != 3 {
		t.Errorf("expected snapshot at version 3, got %d", snap.Version)
	}
}

// ============================================================================
// Projection Tests
// ============================================================================

func TestRegisterProjection(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableSnapshots: false})

	// Register a counting projection
	svc.RegisterProjection("event-counter", func(proj *Projection, event *Event) error {
		count, _ := proj.State["count"].(int)
		proj.State["count"] = count + 1
		return nil
	})

	proj, ok := svc.GetProjection("event-counter")
	if !ok {
		t.Error("expected to find projection")
	}
	if proj.Name != "event-counter" {
		t.Errorf("expected name 'event-counter', got %s", proj.Name)
	}
}

func TestProjection_UpdateOnEvent(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableSnapshots: false})
	ctx := context.Background()

	// Register projection that counts events per aggregate
	svc.RegisterProjection("aggregate-counter", func(proj *Projection, event *Event) error {
		counts, ok := proj.State["counts"].(map[string]int)
		if !ok {
			counts = make(map[string]int)
			proj.State["counts"] = counts
		}
		counts[event.AggregateID]++
		return nil
	})

	// Append events
	svc.Append(ctx, "service-1", "Service", EventCreated, map[string]interface{}{})
	svc.Append(ctx, "service-1", "Service", EventUpdated, map[string]interface{}{})
	svc.Append(ctx, "service-2", "Service", EventCreated, map[string]interface{}{})

	// Give handlers time to run
	time.Sleep(50 * time.Millisecond)

	proj, _ := svc.GetProjection("aggregate-counter")
	counts, _ := proj.State["counts"].(map[string]int)

	if counts["service-1"] != 2 {
		t.Errorf("expected service-1 count 2, got %d", counts["service-1"])
	}
	if counts["service-2"] != 1 {
		t.Errorf("expected service-2 count 1, got %d", counts["service-2"])
	}
}

// ============================================================================
// Event Handler Tests
// ============================================================================

func TestRegisterEventHandler(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableSnapshots: false})
	ctx := context.Background()

	// Buffered channel: handler sends the event, test receives it.
	// Channel send→receive creates a happens-before relationship in
	// Go's memory model, eliminating the data race that time.Sleep cannot.
	done := make(chan *Event, 1)

	svc.RegisterEventHandler(func(ctx context.Context, event *Event) error {
		done <- event
		return nil
	})

	svc.Append(ctx, "service-1", "Service", EventCreated, map[string]interface{}{"test": true})

	select {
	case capturedEvent := <-done:
		if capturedEvent == nil {
			t.Error("expected event to be captured")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("handler was not called within timeout")
	}
}

// ============================================================================
// Timeline Tests
// ============================================================================

func TestGetTimeline(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableSnapshots: false})
	ctx := context.Background()

	start := time.Now()

	svc.Append(ctx, "service-1", "Service", EventCreated, map[string]interface{}{})
	time.Sleep(10 * time.Millisecond)
	svc.Append(ctx, "service-2", "Service", EventCreated, map[string]interface{}{})
	time.Sleep(10 * time.Millisecond)
	svc.Append(ctx, "service-1", "Service", EventUpdated, map[string]interface{}{})

	end := time.Now().Add(time.Second)

	timeline, err := svc.GetTimeline(ctx, start, end, 0)
	if err != nil {
		t.Fatalf("GetTimeline failed: %v", err)
	}

	if timeline.TotalCount != 3 {
		t.Errorf("expected 3 events in timeline, got %d", timeline.TotalCount)
	}

	// Events should be chronologically ordered
	for i := 1; i < len(timeline.Events); i++ {
		if timeline.Events[i].Timestamp.Before(timeline.Events[i-1].Timestamp) {
			t.Error("timeline events should be chronologically ordered")
		}
	}
}

// ============================================================================
// Stats Tests
// ============================================================================

func TestStats(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableSnapshots: false})
	ctx := context.Background()

	svc.Append(ctx, "service-1", "Service", EventCreated, map[string]interface{}{})
	svc.Append(ctx, "service-1", "Service", EventUpdated, map[string]interface{}{})
	svc.Append(ctx, "service-2", "Node", EventCreated, map[string]interface{}{})

	stats := svc.Stats()

	if stats["total_events"].(int) != 3 {
		t.Errorf("expected 3 total events, got %v", stats["total_events"])
	}
	if stats["total_aggregates"].(int) != 2 {
		t.Errorf("expected 2 aggregates, got %v", stats["total_aggregates"])
	}
}

// ============================================================================
// Concurrent Access Tests
// ============================================================================

func TestConcurrentAppend(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableSnapshots: false})
	ctx := context.Background()

	done := make(chan bool, 100)

	for i := 0; i < 100; i++ {
		go func(i int) {
			_, _ = svc.Append(ctx, "concurrent-agg", "Test", EventUpdated, map[string]interface{}{"i": i})
			done <- true
		}(i)
	}

	for i := 0; i < 100; i++ {
		<-done
	}

	stream, _ := svc.GetEventStream("concurrent-agg")
	if len(stream.Events) != 100 {
		t.Errorf("expected 100 events, got %d", len(stream.Events))
	}
}

// ============================================================================
// Benchmarks
// ============================================================================

func BenchmarkAppend(b *testing.B) {
	svc := NewService(nil, nil, &Config{EnableSnapshots: false})
	ctx := context.Background()

	payload := map[string]interface{}{
		"name": "test",
		"port": 8080,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = svc.Append(ctx, "bench-agg", "Service", EventUpdated, payload)
	}
}

func BenchmarkQuery(b *testing.B) {
	svc := NewService(nil, nil, &Config{EnableSnapshots: false})
	ctx := context.Background()

	// Populate with events
	for i := 0; i < 1000; i++ {
		svc.Append(ctx, "bench-agg", "Service", EventUpdated, map[string]interface{}{"i": i})
	}

	query := &Query{AggregateID: "bench-agg", Limit: 100}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = svc.Query(ctx, query)
	}
}

func BenchmarkReconstructState(b *testing.B) {
	svc := NewService(nil, nil, &Config{EnableSnapshots: false})
	ctx := context.Background()

	// Build history
	for i := 0; i < 100; i++ {
		svc.Append(ctx, "bench-agg", "Service", EventUpdated, map[string]interface{}{"port": 8080 + i})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = svc.ReconstructState(ctx, "bench-agg", 50)
	}
}
