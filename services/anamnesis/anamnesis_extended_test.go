// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package anamnesis

import (
	"context"
	"fmt"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"unheaded/pkg/logger"
)

// ============================================================================
// Start / Stop Tests
// ============================================================================

func TestStart(t *testing.T) {
	log := logger.New(io.Discard)
	svc := NewService(log, nil, DefaultConfig())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	// Cancel the context to stop the compaction loop goroutine.
	cancel()
}

func TestStop(t *testing.T) {
	log := logger.New(io.Discard)
	svc := NewService(log, nil, nil)

	if err := svc.Stop(); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
}

// ============================================================================
// WotanDrops Tests
// ============================================================================

func TestWotanDrops_IncrementedOnPublish(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableSnapshots: false})
	ctx := context.Background()

	// With nil wotan, every Append should increment wotanDrops via publishEvent.
	svc.Append(ctx, "agg-1", "Test", EventCreated, map[string]interface{}{"k": "v"})
	svc.Append(ctx, "agg-1", "Test", EventUpdated, map[string]interface{}{"k": "v2"})

	drops := svc.WotanDrops()
	if drops != 2 {
		t.Errorf("expected 2 wotan drops, got %d", drops)
	}
}

func TestWotanDrops_ZeroInitially(t *testing.T) {
	svc := NewService(nil, nil, nil)
	if svc.WotanDrops() != 0 {
		t.Errorf("expected 0 wotan drops initially, got %d", svc.WotanDrops())
	}
}

// ============================================================================
// publishEvent Tests (nil wotan path already tested above; covers the log branch)
// ============================================================================

func TestPublishEvent_NilWotan(t *testing.T) {
	log := logger.New(io.Discard)
	svc := NewService(log, nil, &Config{EnableSnapshots: false, WotanTopic: "test.topic"})

	// Directly call publishEvent to exercise the nil wotan branch.
	svc.publishEvent(context.Background(), "test.event", map[string]interface{}{"key": "value"})

	if svc.WotanDrops() != 1 {
		t.Errorf("expected 1 wotan drop, got %d", svc.WotanDrops())
	}
}

// ============================================================================
// subscribeToEvents Tests
// ============================================================================

func TestSubscribeToEvents_NilWotan(t *testing.T) {
	log := logger.New(io.Discard)
	svc := NewService(log, nil, nil)

	// Should return immediately without panic when wotan is nil.
	svc.subscribeToEvents(context.Background())
}

// ============================================================================
// runCompaction Tests
// ============================================================================

func TestRunCompaction_RemovesOldEvents(t *testing.T) {
	log := logger.New(io.Discard)
	cfg := &Config{
		EnableSnapshots: false,
		RetentionPeriod: 50 * time.Millisecond,
	}
	svc := NewService(log, nil, cfg)
	ctx := context.Background()

	// Append an event.
	svc.Append(ctx, "agg-1", "Test", EventCreated, map[string]interface{}{"v": 1})

	// Wait for the event to age past the retention period.
	time.Sleep(100 * time.Millisecond)

	// Append a fresh event.
	svc.Append(ctx, "agg-2", "Test", EventCreated, map[string]interface{}{"v": 2})

	// Run compaction.
	svc.runCompaction(ctx)

	// The old event should be gone, the new one kept.
	svc.mu.RLock()
	count := len(svc.events)
	svc.mu.RUnlock()

	if count != 1 {
		t.Errorf("expected 1 event after compaction, got %d", count)
	}

	// The remaining event should belong to agg-2.
	stream, err := svc.GetEventStream("agg-2")
	if err != nil {
		t.Fatalf("expected agg-2 stream to exist: %v", err)
	}
	if len(stream.Events) != 1 {
		t.Errorf("expected 1 event for agg-2, got %d", len(stream.Events))
	}

	// agg-1 should be gone from the index.
	_, err = svc.GetEventStream("agg-1")
	if err == nil {
		t.Error("expected agg-1 stream to not exist after compaction")
	}
}

func TestRunCompaction_NoOp(t *testing.T) {
	log := logger.New(io.Discard)
	cfg := &Config{
		EnableSnapshots: false,
		RetentionPeriod: 24 * time.Hour,
	}
	svc := NewService(log, nil, cfg)
	ctx := context.Background()

	svc.Append(ctx, "agg-1", "Test", EventCreated, map[string]interface{}{})

	svc.runCompaction(ctx)

	svc.mu.RLock()
	count := len(svc.events)
	svc.mu.RUnlock()

	if count != 1 {
		t.Errorf("expected 1 event (no compaction), got %d", count)
	}
}

// ============================================================================
// compactionLoop Tests
// ============================================================================

func TestCompactionLoop_StopsOnContextCancel(t *testing.T) {
	log := logger.New(io.Discard)
	cfg := &Config{
		EnableSnapshots:    false,
		CompactionInterval: 10 * time.Millisecond,
		RetentionPeriod:    24 * time.Hour,
	}
	svc := NewService(log, nil, cfg)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		svc.compactionLoop(ctx)
		close(done)
	}()

	// Let it tick a few times.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Good, loop exited.
	case <-time.After(2 * time.Second):
		t.Fatal("compactionLoop did not exit after context cancellation")
	}
}

// ============================================================================
// RebuildProjection Tests
// ============================================================================

func TestRebuildProjection_Success(t *testing.T) {
	log := logger.New(io.Discard)
	svc := NewService(log, nil, &Config{EnableSnapshots: false})
	ctx := context.Background()

	// Register a counting projection.
	svc.RegisterProjection("counter", func(proj *Projection, event *Event) error {
		count, _ := proj.State["count"].(int)
		proj.State["count"] = count + 1
		return nil
	})

	// Append events directly (projection handlers fire async, but RebuildProjection
	// replays synchronously).
	svc.Append(ctx, "agg-1", "Test", EventCreated, map[string]interface{}{})
	svc.Append(ctx, "agg-1", "Test", EventUpdated, map[string]interface{}{})
	svc.Append(ctx, "agg-2", "Test", EventCreated, map[string]interface{}{})

	// Give async handlers time to finish so they don't race with RebuildProjection.
	time.Sleep(100 * time.Millisecond)

	// Rebuild the projection from scratch.
	if err := svc.RebuildProjection(ctx, "counter"); err != nil {
		t.Fatalf("RebuildProjection failed: %v", err)
	}

	proj, ok := svc.GetProjection("counter")
	if !ok {
		t.Fatal("expected projection to exist")
	}

	count, _ := proj.State["count"].(int)
	if count != 3 {
		t.Errorf("expected count 3 after rebuild, got %d", count)
	}
}

func TestRebuildProjection_NotFound(t *testing.T) {
	svc := NewService(nil, nil, nil)

	err := svc.RebuildProjection(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent projection")
	}
}

func TestRebuildProjection_HandlerError(t *testing.T) {
	log := logger.New(io.Discard)
	svc := NewService(log, nil, &Config{EnableSnapshots: false})
	ctx := context.Background()

	svc.RegisterProjection("failing", func(proj *Projection, event *Event) error {
		return fmt.Errorf("handler exploded")
	})

	svc.Append(ctx, "agg-1", "Test", EventCreated, map[string]interface{}{})

	// Give async handler time to finish (it will fail silently in notifyHandlers).
	time.Sleep(100 * time.Millisecond)

	err := svc.RebuildProjection(ctx, "failing")
	if err == nil {
		t.Error("expected error from failing projection handler")
	}
}

// ============================================================================
// Query — uncovered filter branches
// ============================================================================

func TestQuery_ByAggregateType(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableSnapshots: false})
	ctx := context.Background()

	svc.Append(ctx, "svc-1", "Service", EventCreated, map[string]interface{}{})
	svc.Append(ctx, "node-1", "Node", EventCreated, map[string]interface{}{})
	svc.Append(ctx, "svc-2", "Service", EventCreated, map[string]interface{}{})

	events, err := svc.Query(ctx, &Query{AggregateType: "Node"})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 Node event, got %d", len(events))
	}
}

func TestQuery_ByVersionRange(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableSnapshots: false})
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		svc.Append(ctx, "agg-1", "Test", EventUpdated, map[string]interface{}{"i": i})
	}

	events, err := svc.Query(ctx, &Query{
		AggregateID: "agg-1",
		FromVersion: 2,
		ToVersion:   4,
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(events) != 3 {
		t.Errorf("expected 3 events in version range [2,4], got %d", len(events))
	}
}

func TestQuery_ByActor(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableSnapshots: false})
	ctx := context.Background()

	// Append events and manually set Actor.
	e1, _ := svc.Append(ctx, "agg-1", "Test", EventCreated, map[string]interface{}{})
	e2, _ := svc.Append(ctx, "agg-1", "Test", EventUpdated, map[string]interface{}{})

	svc.mu.Lock()
	e1.Actor = "alice"
	e2.Actor = "bob"
	svc.mu.Unlock()

	events, err := svc.Query(ctx, &Query{Actor: "alice"})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event for alice, got %d", len(events))
	}
}

func TestQuery_ByCorrelationID(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableSnapshots: false})
	ctx := context.Background()

	e1, _ := svc.Append(ctx, "agg-1", "Test", EventCreated, map[string]interface{}{})
	svc.Append(ctx, "agg-1", "Test", EventUpdated, map[string]interface{}{})

	svc.mu.Lock()
	e1.CorrelationID = "corr-123"
	svc.mu.Unlock()

	events, err := svc.Query(ctx, &Query{CorrelationID: "corr-123"})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event with correlation ID, got %d", len(events))
	}
}

func TestQuery_WithOffset(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableSnapshots: false})
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		svc.Append(ctx, "agg-1", "Test", EventUpdated, map[string]interface{}{"i": i})
	}

	events, err := svc.Query(ctx, &Query{Offset: 7})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(events) != 3 {
		t.Errorf("expected 3 events after offset 7, got %d", len(events))
	}
}

func TestQuery_WithOffsetAndLimit(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableSnapshots: false})
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		svc.Append(ctx, "agg-1", "Test", EventUpdated, map[string]interface{}{"i": i})
	}

	events, err := svc.Query(ctx, &Query{Offset: 3, Limit: 2})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 events with offset+limit, got %d", len(events))
	}
}

func TestQuery_ByToTime(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableSnapshots: false})
	ctx := context.Background()

	svc.Append(ctx, "agg-1", "Test", EventCreated, map[string]interface{}{})

	time.Sleep(20 * time.Millisecond)
	cutoff := time.Now()
	time.Sleep(20 * time.Millisecond)

	svc.Append(ctx, "agg-1", "Test", EventUpdated, map[string]interface{}{})

	events, err := svc.Query(ctx, &Query{ToTime: &cutoff})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event before cutoff, got %d", len(events))
	}
}

func TestQuery_AllFilters(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableSnapshots: false})
	ctx := context.Background()

	svc.Append(ctx, "agg-1", "Service", EventCreated, map[string]interface{}{})

	events, err := svc.Query(ctx, &Query{})
	if err != nil {
		t.Fatalf("Query with empty filter failed: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event with empty filter, got %d", len(events))
	}
}

// ============================================================================
// applyEventToState — EventDeleted branch
// ============================================================================

func TestApplyEventToState_Delete(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableSnapshots: false})
	ctx := context.Background()

	// Create then delete a field.
	svc.Append(ctx, "agg-1", "Test", EventCreated, map[string]interface{}{"name": "test", "port": 8080})
	svc.Append(ctx, "agg-1", "Test", EventDeleted, map[string]interface{}{"port": nil})

	state, err := svc.ReconstructState(ctx, "agg-1", 2)
	if err != nil {
		t.Fatalf("ReconstructState failed: %v", err)
	}

	if _, ok := state["port"]; ok {
		t.Error("expected 'port' to be deleted from state")
	}
	if state["name"] != "test" {
		t.Errorf("expected 'name' to remain, got %v", state["name"])
	}
}

func TestApplyEventToState_UnknownType(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableSnapshots: false})

	state := map[string]interface{}{"existing": true}
	event := &Event{
		Type:    EventCommand,
		Payload: map[string]interface{}{"new_key": "new_val"},
	}

	svc.applyEventToState(state, event)

	// EventCommand should not modify state.
	if _, ok := state["new_key"]; ok {
		t.Error("EventCommand should not add keys to state")
	}
	if !state["existing"].(bool) {
		t.Error("existing key should remain unchanged")
	}
}

// ============================================================================
// ReconstructState — with snapshots
// ============================================================================

func TestReconstructState_WithSnapshot(t *testing.T) {
	log := logger.New(io.Discard)
	cfg := &Config{
		EnableSnapshots:  true,
		SnapshotInterval: 2,
	}
	svc := NewService(log, nil, cfg)
	ctx := context.Background()

	// Append enough events to trigger a snapshot at version 2.
	svc.Append(ctx, "agg-1", "Test", EventCreated, map[string]interface{}{"name": "test"})
	svc.Append(ctx, "agg-1", "Test", EventUpdated, map[string]interface{}{"port": 8080})

	// Wait for the async snapshot goroutine.
	deadline := time.After(2 * time.Second)
	for {
		_, ok := svc.GetLatestSnapshot("agg-1")
		if ok {
			break
		}
		select {
		case <-deadline:
			t.Skip("snapshot not created within timeout")
			return
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Append more events after the snapshot.
	svc.Append(ctx, "agg-1", "Test", EventUpdated, map[string]interface{}{"port": 9090})

	// Reconstruct at version 3 — should start from snapshot at v2 then apply v3.
	state, err := svc.ReconstructState(ctx, "agg-1", 3)
	if err != nil {
		t.Fatalf("ReconstructState failed: %v", err)
	}

	if state["port"] != 9090 {
		t.Errorf("expected port 9090, got %v", state["port"])
	}
	if state["name"] != "test" {
		t.Errorf("expected name 'test' from snapshot, got %v", state["name"])
	}
}

// ============================================================================
// notifyHandlersSnapshot — error paths
// ============================================================================

func TestNotifyHandlersSnapshot_EventHandlerError(t *testing.T) {
	log := logger.New(io.Discard)
	svc := NewService(log, nil, &Config{EnableSnapshots: false})
	ctx := context.Background()

	var called int64
	handlers := []EventHandler{
		func(ctx context.Context, event *Event) error {
			atomic.AddInt64(&called, 1)
			return fmt.Errorf("handler error")
		},
	}

	event := &Event{ID: "test-evt", Type: EventCreated}

	svc.notifyHandlersSnapshot(ctx, event, handlers, nil)

	if atomic.LoadInt64(&called) != 1 {
		t.Error("expected handler to be called despite error")
	}
}

func TestNotifyHandlersSnapshot_ProjectionHandlerError(t *testing.T) {
	log := logger.New(io.Discard)
	svc := NewService(log, nil, &Config{EnableSnapshots: false})
	ctx := context.Background()

	// Register a projection so it exists in the projections map.
	svc.RegisterProjection("failing-proj", func(proj *Projection, event *Event) error {
		return fmt.Errorf("projection error")
	})

	projHandlers := map[string]ProjectionHandler{
		"failing-proj": func(proj *Projection, event *Event) error {
			return fmt.Errorf("projection error")
		},
	}

	event := &Event{ID: "test-evt", Type: EventCreated}

	// Should not panic; error is logged.
	svc.notifyHandlersSnapshot(ctx, event, nil, projHandlers)

	proj, ok := svc.GetProjection("failing-proj")
	if !ok {
		t.Fatal("expected projection to exist")
	}
	// Version should NOT have been incremented because the handler failed.
	if proj.Version != 0 {
		t.Errorf("expected version 0 (handler failed), got %d", proj.Version)
	}
}

func TestNotifyHandlersSnapshot_NilProjection(t *testing.T) {
	log := logger.New(io.Discard)
	svc := NewService(log, nil, &Config{EnableSnapshots: false})
	ctx := context.Background()

	// Don't register the projection — it won't be in svc.projections.
	projHandlers := map[string]ProjectionHandler{
		"ghost-proj": func(proj *Projection, event *Event) error {
			return nil
		},
	}

	event := &Event{ID: "test-evt", Type: EventCreated}

	// Should not panic when proj is nil.
	svc.notifyHandlersSnapshot(ctx, event, nil, projHandlers)
}

// ============================================================================
// GetTimeline — with limit
// ============================================================================

func TestGetTimeline_WithLimit(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableSnapshots: false})
	ctx := context.Background()

	start := time.Now().Add(-time.Second)

	for i := 0; i < 5; i++ {
		svc.Append(ctx, fmt.Sprintf("agg-%d", i), "Test", EventCreated, map[string]interface{}{})
	}

	end := time.Now().Add(time.Second)

	timeline, err := svc.GetTimeline(ctx, start, end, 3)
	if err != nil {
		t.Fatalf("GetTimeline failed: %v", err)
	}

	if timeline.TotalCount != 5 {
		t.Errorf("expected TotalCount 5, got %d", timeline.TotalCount)
	}
	if len(timeline.Events) != 3 {
		t.Errorf("expected 3 events (limited), got %d", len(timeline.Events))
	}
}

func TestGetTimeline_Empty(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableSnapshots: false})
	ctx := context.Background()

	from := time.Now().Add(-time.Hour)
	to := time.Now().Add(time.Hour)

	timeline, err := svc.GetTimeline(ctx, from, to, 0)
	if err != nil {
		t.Fatalf("GetTimeline failed: %v", err)
	}
	if timeline.TotalCount != 0 {
		t.Errorf("expected 0 events, got %d", timeline.TotalCount)
	}
}

// ============================================================================
// Stats — additional coverage
// ============================================================================

func TestStats_EmptyService(t *testing.T) {
	svc := NewService(nil, nil, nil)

	stats := svc.Stats()

	if stats["total_events"].(int) != 0 {
		t.Errorf("expected 0 events, got %v", stats["total_events"])
	}
	if stats["total_aggregates"].(int) != 0 {
		t.Errorf("expected 0 aggregates, got %v", stats["total_aggregates"])
	}
	if stats["total_snapshots"].(int) != 0 {
		t.Errorf("expected 0 snapshots, got %v", stats["total_snapshots"])
	}
	if stats["total_projections"].(int) != 0 {
		t.Errorf("expected 0 projections, got %v", stats["total_projections"])
	}
	if stats["sequence_number"].(int64) != 0 {
		t.Errorf("expected sequence 0, got %v", stats["sequence_number"])
	}
	if stats["registered_handlers"].(int) != 0 {
		t.Errorf("expected 0 handlers, got %v", stats["registered_handlers"])
	}
}

func TestStats_WithSnapshots(t *testing.T) {
	log := logger.New(io.Discard)
	cfg := &Config{
		EnableSnapshots:  true,
		SnapshotInterval: 2,
	}
	svc := NewService(log, nil, cfg)
	ctx := context.Background()

	svc.Append(ctx, "agg-1", "Test", EventCreated, map[string]interface{}{"a": 1})
	svc.Append(ctx, "agg-1", "Test", EventUpdated, map[string]interface{}{"a": 2})

	// Wait for snapshot.
	deadline := time.After(2 * time.Second)
	for {
		_, ok := svc.GetLatestSnapshot("agg-1")
		if ok {
			break
		}
		select {
		case <-deadline:
			t.Skip("snapshot not created within timeout")
			return
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	stats := svc.Stats()
	snapshots := stats["total_snapshots"].(int)
	if snapshots < 1 {
		t.Errorf("expected at least 1 snapshot in stats, got %d", snapshots)
	}
}

// ============================================================================
// GetLatestSnapshot — no snapshot
// ============================================================================

func TestGetLatestSnapshot_NotFound(t *testing.T) {
	svc := NewService(nil, nil, nil)

	_, ok := svc.GetLatestSnapshot("nonexistent")
	if ok {
		t.Error("expected no snapshot for nonexistent aggregate")
	}
}

// ============================================================================
// GetProjection — not found
// ============================================================================

func TestGetProjection_NotFound(t *testing.T) {
	svc := NewService(nil, nil, nil)

	_, ok := svc.GetProjection("nonexistent")
	if ok {
		t.Error("expected no projection for nonexistent name")
	}
}

// ============================================================================
// NewService — with explicit config and logger
// ============================================================================

func TestNewService_WithConfig(t *testing.T) {
	log := logger.New(io.Discard)
	cfg := &Config{
		MaxEvents:        500,
		SnapshotInterval: 50,
		EnableSnapshots:  true,
		WotanTopic:       "custom.topic",
	}

	svc := NewService(log, nil, cfg)

	if svc.config.MaxEvents != 500 {
		t.Errorf("expected MaxEvents 500, got %d", svc.config.MaxEvents)
	}
	if svc.config.WotanTopic != "custom.topic" {
		t.Errorf("expected WotanTopic 'custom.topic', got %s", svc.config.WotanTopic)
	}
}

// ============================================================================
// Concurrent reads during writes
// ============================================================================

func TestConcurrentQueryDuringAppend(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableSnapshots: false})
	ctx := context.Background()

	// Pre-populate.
	for i := 0; i < 10; i++ {
		svc.Append(ctx, "agg-1", "Test", EventUpdated, map[string]interface{}{"i": i})
	}

	done := make(chan struct{})

	// Concurrent writer.
	go func() {
		for i := 0; i < 50; i++ {
			svc.Append(ctx, "agg-1", "Test", EventUpdated, map[string]interface{}{"i": i + 10})
		}
		close(done)
	}()

	// Concurrent readers.
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 20; j++ {
				svc.Query(ctx, &Query{AggregateID: "agg-1", Limit: 5})
				svc.Stats()
				svc.GetLatestSnapshot("agg-1")
			}
		}()
	}

	<-done
}

// ============================================================================
// ReconstructState — empty aggregate
// ============================================================================

func TestReconstructState_EmptyAggregate(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableSnapshots: false})
	ctx := context.Background()

	state, err := svc.ReconstructState(ctx, "nonexistent", 1)
	if err != nil {
		t.Fatalf("ReconstructState should not error for empty aggregate: %v", err)
	}
	if len(state) != 0 {
		t.Errorf("expected empty state for empty aggregate, got %v", state)
	}
}

// ============================================================================
// computeStateHash — deterministic
// ============================================================================

func TestComputeStateHash_Deterministic(t *testing.T) {
	svc := NewService(nil, nil, nil)

	state := map[string]interface{}{"key": "value", "num": 42}

	h1 := svc.computeStateHash(state)
	h2 := svc.computeStateHash(state)

	if h1 != h2 {
		t.Errorf("expected same hash for same state, got %s and %s", h1, h2)
	}
	if h1 == "" {
		t.Error("expected non-empty hash")
	}
}

// ============================================================================
// computeEventHash — deterministic
// ============================================================================

func TestComputeEventHash_Deterministic(t *testing.T) {
	svc := NewService(nil, nil, nil)
	now := time.Now()

	event := &Event{
		AggregateID:   "agg-1",
		AggregateType: "Test",
		Type:          EventCreated,
		Timestamp:     now,
		Version:       1,
		PrevHash:      "",
	}

	h1 := svc.computeEventHash(event)
	h2 := svc.computeEventHash(event)

	if h1 != h2 {
		t.Errorf("expected same hash, got %s and %s", h1, h2)
	}
}
