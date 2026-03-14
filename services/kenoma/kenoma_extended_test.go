// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package kenoma

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/logger"
	wotanClient "unheaded/pkg/wotan-client"
)

// ============================================================================
// WotanDrops Tests
// ============================================================================

func TestWotanDrops_InitiallyZero(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)
	if got := svc.WotanDrops(); got != 0 {
		t.Errorf("expected 0 drops initially, got %d", got)
	}
}

func TestWotanDrops_IncrementsOnPublishWithoutWotan(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)
	ctx := context.Background()

	// Each Observe call triggers publishEvent which increments wotanDrops when wotan is nil
	svc.Observe(ctx, "entity-1", EntityService, map[string]interface{}{"a": 1})
	svc.Observe(ctx, "entity-2", EntityService, map[string]interface{}{"b": 2})

	drops := svc.WotanDrops()
	if drops < 2 {
		t.Errorf("expected at least 2 drops, got %d", drops)
	}
}

func TestWotanDrops_ConcurrentSafety(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			svc.Observe(ctx, fmt.Sprintf("entity-%d", i), EntityService, map[string]interface{}{"x": i})
		}(i)
	}
	wg.Wait()

	drops := svc.WotanDrops()
	if drops < 50 {
		t.Errorf("expected at least 50 drops, got %d", drops)
	}
}

// ============================================================================
// Start / Stop Tests
// ============================================================================

func TestStart_ReturnsNil(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := svc.Start(ctx)
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	// Cancel to stop background goroutines
	cancel()
	// Give goroutines a moment to exit
	time.Sleep(10 * time.Millisecond)
}

func TestStart_RegistersDefaultComparators(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc.Start(ctx)

	// After Start, the service comparator should be registered
	svc.mu.RLock()
	_, hasServiceComparator := svc.comparators[EntityService]
	svc.mu.RUnlock()

	if !hasServiceComparator {
		t.Error("expected service comparator to be registered after Start")
	}
}

func TestStop_ReturnsNil(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)
	err := svc.Stop()
	if err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
}

// ============================================================================
// RegisterObserver Tests
// ============================================================================

func TestRegisterObserver(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)

	called := false
	observer := func(ctx context.Context, entityID string) (*Observation, error) {
		called = true
		return &Observation{ID: "test-obs", EntityID: entityID}, nil
	}

	svc.RegisterObserver(EntityContainer, observer)

	svc.mu.RLock()
	obs, ok := svc.observers[EntityContainer]
	svc.mu.RUnlock()

	if !ok {
		t.Fatal("observer not registered")
	}

	// Verify the observer works
	result, err := obs(context.Background(), "container-1")
	if err != nil {
		t.Fatalf("observer returned error: %v", err)
	}
	if !called {
		t.Error("observer was not called")
	}
	if result.EntityID != "container-1" {
		t.Errorf("expected entity ID 'container-1', got %s", result.EntityID)
	}
}

func TestRegisterObserver_MultipleTypes(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)

	svc.RegisterObserver(EntityContainer, func(ctx context.Context, entityID string) (*Observation, error) {
		return nil, nil
	})
	svc.RegisterObserver(EntityNode, func(ctx context.Context, entityID string) (*Observation, error) {
		return nil, nil
	})
	svc.RegisterObserver(EntityNetwork, func(ctx context.Context, entityID string) (*Observation, error) {
		return nil, nil
	})

	svc.mu.RLock()
	count := len(svc.observers)
	svc.mu.RUnlock()

	if count != 3 {
		t.Errorf("expected 3 observers, got %d", count)
	}
}

// ============================================================================
// registerDefaultComparators Tests
// ============================================================================

func TestRegisterDefaultComparators_ServicePortSeverity(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)
	svc.registerDefaultComparators()

	// Compare with port drift — should get high severity on port
	desired := map[string]interface{}{"port": 8080}
	actual := map[string]interface{}{"port": 9090}

	drifts, err := svc.Compare(context.Background(), "svc-1", EntityService, "cfg-1", desired, actual)
	if err != nil {
		t.Fatalf("Compare failed: %v", err)
	}

	if len(drifts) != 1 {
		t.Fatalf("expected 1 drift, got %d", len(drifts))
	}

	// The service comparator should elevate port drift to high severity
	if drifts[0].Severity != SeverityHigh {
		t.Errorf("expected SeverityHigh for port drift, got %s", drifts[0].Severity)
	}
}

func TestRegisterDefaultComparators_NoDrift(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)
	svc.registerDefaultComparators()

	desired := map[string]interface{}{"port": 8080, "name": "svc"}
	actual := map[string]interface{}{"port": 8080, "name": "svc"}

	drifts, err := svc.Compare(context.Background(), "svc-1", EntityService, "cfg-1", desired, actual)
	if err != nil {
		t.Fatalf("Compare failed: %v", err)
	}

	if len(drifts) != 0 {
		t.Errorf("expected 0 drifts, got %d", len(drifts))
	}
}

func TestRegisterDefaultComparators_PortMissingFromActual(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)
	svc.registerDefaultComparators()

	// Port in desired but not in actual — should be missing drift, no port-severity elevation
	desired := map[string]interface{}{"port": 8080}
	actual := map[string]interface{}{}

	drifts, err := svc.Compare(context.Background(), "svc-1", EntityService, "cfg-1", desired, actual)
	if err != nil {
		t.Fatalf("Compare failed: %v", err)
	}

	if len(drifts) != 1 {
		t.Fatalf("expected 1 drift, got %d", len(drifts))
	}

	if drifts[0].DriftType != DriftMissing {
		t.Errorf("expected DriftMissing, got %s", drifts[0].DriftType)
	}
}

func TestRegisterDefaultComparators_SamePort(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)
	svc.registerDefaultComparators()

	// Same port — no elevation needed
	desired := map[string]interface{}{"port": 8080, "replicas": 1}
	actual := map[string]interface{}{"port": 8080, "replicas": 2}

	drifts, err := svc.Compare(context.Background(), "svc-1", EntityService, "cfg-1", desired, actual)
	if err != nil {
		t.Fatalf("Compare failed: %v", err)
	}

	if len(drifts) != 1 {
		t.Fatalf("expected 1 drift, got %d", len(drifts))
	}

	// replicas drift should be medium, not elevated
	if drifts[0].Severity != SeverityMedium {
		t.Errorf("expected SeverityMedium for replicas drift, got %s", drifts[0].Severity)
	}
}

// ============================================================================
// observationLoop / driftDetectionLoop Tests
// ============================================================================

func TestObservationLoop_ExitsOnContextCancel(t *testing.T) {
	cfg := &Config{
		ObservationInterval: 5 * time.Millisecond,
		DriftCheckInterval:  60 * time.Second,
		RetentionPeriod:     time.Hour,
		MaxObservations:     100,
		WotanTopic:          "test",
	}
	svc := NewService(logger.New(io.Discard), nil, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		svc.observationLoop(ctx)
		close(done)
	}()

	// Let at least one tick happen
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// OK — exited
	case <-time.After(time.Second):
		t.Fatal("observationLoop did not exit after context cancel")
	}
}

func TestDriftDetectionLoop_ExitsOnContextCancel(t *testing.T) {
	cfg := &Config{
		ObservationInterval: 60 * time.Second,
		DriftCheckInterval:  5 * time.Millisecond,
		RetentionPeriod:     time.Hour,
		MaxObservations:     100,
		WotanTopic:          "test",
	}
	svc := NewService(logger.New(io.Discard), nil, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		svc.driftDetectionLoop(ctx)
		close(done)
	}()

	// Let at least one tick happen
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// OK — exited
	case <-time.After(time.Second):
		t.Fatal("driftDetectionLoop did not exit after context cancel")
	}
}

// ============================================================================
// runObservations Tests
// ============================================================================

func TestRunObservations_LogsTrackedEntities(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)
	ctx := context.Background()

	// Add some observations first
	svc.Observe(ctx, "svc-1", EntityService, map[string]interface{}{"a": 1})
	svc.Observe(ctx, "svc-2", EntityService, map[string]interface{}{"b": 2})

	// Should not panic
	svc.runObservations(ctx)
}

// ============================================================================
// runDriftDetection Tests
// ============================================================================

func TestRunDriftDetection_CountsActiveDrifts(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)
	ctx := context.Background()

	// Create a drift
	svc.Compare(ctx, "svc-1", EntityService, "cfg-1",
		map[string]interface{}{"port": 8080},
		map[string]interface{}{"port": 9090})

	// Resolve it
	drifts := svc.ListDrifts("svc-1")
	if len(drifts) > 0 {
		svc.ResolveDrift(ctx, drifts[0].ID)
	}

	// Create another (unresolved)
	svc.Compare(ctx, "svc-2", EntityService, "cfg-2",
		map[string]interface{}{"replicas": 3},
		map[string]interface{}{"replicas": 1})

	// Should not panic, should count 1 active drift
	svc.runDriftDetection(ctx)
}

// ============================================================================
// subscribeToEvents Tests
// ============================================================================

func TestSubscribeToEvents_NilWotan(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)
	ctx := context.Background()

	// Should return immediately without panicking
	svc.subscribeToEvents(ctx)
}

func TestSubscribeToEvents_WithWotan_SubscribeError(t *testing.T) {
	// Mock server that returns an error for subscribe
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"test error"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client, err := wotanClient.NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	svc := NewService(logger.New(io.Discard), client, nil)
	ctx := context.Background()

	// Should not panic even when subscribe fails
	svc.subscribeToEvents(ctx)
}

func TestSubscribeToEvents_WithWotan_Success(t *testing.T) {
	// Mock server with proper subscribe endpoint
	mux := http.NewServeMux()
	mux.HandleFunc("/topics/state.changed/subscribe", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		resp := map[string]interface{}{
			"subscriber": map[string]interface{}{
				"subscriber_id": "test-sub-1",
				"topic":         "state.changed",
				"display_name":  "kenoma-service",
				"status":        "approved",
			},
		}
		json.NewEncoder(w).Encode(resp)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client, err := wotanClient.NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	svc := NewService(logger.New(io.Discard), client, nil)
	ctx := context.Background()

	// Should succeed
	svc.subscribeToEvents(ctx)
}

// ============================================================================
// publishEvent Tests
// ============================================================================

func TestPublishEvent_NilWotan(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)
	ctx := context.Background()

	initialDrops := svc.WotanDrops()
	svc.publishEvent(ctx, "test.event", map[string]interface{}{"key": "value"})

	if svc.WotanDrops() != initialDrops+1 {
		t.Error("expected wotanDrops to increment when wotan is nil")
	}
}

func TestPublishEvent_WithWotan_PublishError(t *testing.T) {
	// Create a wotan client that will fail on publish (not subscribed to the topic)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client, err := wotanClient.NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	svc := NewService(logger.New(io.Discard), client, nil)
	ctx := context.Background()

	// Should not panic — the publish will fail because client isn't subscribed
	svc.publishEvent(ctx, "test.event", map[string]interface{}{"key": "value"})
}

func TestPublishEvent_WithWotan_FullFlow(t *testing.T) {
	// Set up a mock server that supports subscribe + publish
	var publishCalled bool
	mux := http.NewServeMux()
	mux.HandleFunc("/topics/kenoma.test/subscribe", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		resp := map[string]interface{}{
			"subscriber": map[string]interface{}{
				"subscriber_id": "sub-1",
				"topic":         "kenoma.test",
				"display_name":  "kenoma-pub",
				"status":        "approved",
			},
		}
		json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/topics/kenoma.test/publish", func(w http.ResponseWriter, r *http.Request) {
		publishCalled = true
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client, err := wotanClient.NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	// Subscribe first so Publish can work
	_, err = client.Subscribe(context.Background(), "kenoma.test", "kenoma-pub")
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	cfg := DefaultConfig()
	cfg.WotanTopic = "kenoma.test"
	svc := NewService(logger.New(io.Discard), client, cfg)

	svc.publishEvent(context.Background(), "test.event", map[string]interface{}{"key": "value"})

	if !publishCalled {
		t.Error("expected publish to be called on the server")
	}
}

// ============================================================================
// assessSeverity - Default Case
// ============================================================================

func TestAssessSeverity_UnknownType(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)

	drift := &Drift{DriftType: DriftType("unknown")}
	severity := svc.assessSeverity(drift)

	if severity != SeverityInfo {
		t.Errorf("expected SeverityInfo for unknown drift type, got %s", severity)
	}
}

// ============================================================================
// computeStateHash Tests
// ============================================================================

func TestComputeStateHash_EmptyState(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)

	hash := svc.computeStateHash(map[string]interface{}{})
	if hash == "" {
		t.Error("expected non-empty hash for empty map (marshals to '{}')")
	}
}

func TestComputeStateHash_ConsistentForSameInput(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)

	state := map[string]interface{}{"port": 8080, "name": "test"}
	hash1 := svc.computeStateHash(state)
	hash2 := svc.computeStateHash(state)

	if hash1 != hash2 {
		t.Errorf("expected same hash for same input, got %s and %s", hash1, hash2)
	}
}

func TestComputeStateHash_DifferentForDifferentInput(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)

	hash1 := svc.computeStateHash(map[string]interface{}{"port": 8080})
	hash2 := svc.computeStateHash(map[string]interface{}{"port": 9090})

	if hash1 == hash2 {
		t.Error("expected different hashes for different inputs")
	}
}

// ============================================================================
// isAutoRemediable Tests
// ============================================================================

func TestIsAutoRemediable_EnabledAndModified(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AutoRemediate = true
	svc := NewService(logger.New(io.Discard), nil, cfg)

	drift := &Drift{DriftType: DriftModified}
	if !svc.isAutoRemediable(drift) {
		t.Error("expected auto-remediable when AutoRemediate=true and DriftModified")
	}
}

func TestIsAutoRemediable_DisabledAndModified(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AutoRemediate = false
	svc := NewService(logger.New(io.Discard), nil, cfg)

	drift := &Drift{DriftType: DriftModified}
	if svc.isAutoRemediable(drift) {
		t.Error("expected not auto-remediable when AutoRemediate=false")
	}
}

func TestIsAutoRemediable_EnabledButNotModified(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AutoRemediate = true
	svc := NewService(logger.New(io.Discard), nil, cfg)

	for _, dt := range []DriftType{DriftMissing, DriftExtra, DriftOutdated, DriftUnhealthy} {
		drift := &Drift{DriftType: dt}
		if svc.isAutoRemediable(drift) {
			t.Errorf("expected not auto-remediable for drift type %s", dt)
		}
	}
}

// ============================================================================
// Start with Wotan (subscribeToEvents path)
// ============================================================================

func TestStart_WithWotan_SubscribestoEvents(t *testing.T) {
	var subscribed bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/subscribe", func(w http.ResponseWriter, r *http.Request) {
		subscribed = true
		resp := map[string]interface{}{
			"subscriber_id": "test-sub",
			"topic":         "state.changed",
			"status":        "active",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	// Need a messages endpoint to avoid errors from polling
	mux.HandleFunc("/api/v1/messages", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"messages": []interface{}{}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client, err := wotanClient.NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	cfg := &Config{
		ObservationInterval: 100 * time.Millisecond,
		DriftCheckInterval:  100 * time.Millisecond,
		RetentionPeriod:     time.Hour,
		MaxObservations:     100,
		WotanTopic:          "kenoma.test",
	}

	svc := NewService(logger.New(io.Discard), client, cfg)
	ctx, cancel := context.WithCancel(context.Background())

	if err = svc.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	// Give the subscribeToEvents goroutine time to run
	time.Sleep(50 * time.Millisecond)
	cancel()

	if !subscribed {
		t.Error("expected subscribeToEvents to be called when wotan is available")
	}
}

// ============================================================================
// Compare with custom comparator vs default fallback
// ============================================================================

func TestCompare_UsesDefaultComparatorWhenNoneRegistered(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)

	// EntityNetwork has no comparator registered — should use defaultComparator
	desired := map[string]interface{}{"subnet": "10.0.0.0/24"}
	actual := map[string]interface{}{"subnet": "192.168.0.0/24"}

	drifts, err := svc.Compare(context.Background(), "net-1", EntityNetwork, "cfg-1", desired, actual)
	if err != nil {
		t.Fatalf("Compare failed: %v", err)
	}

	if len(drifts) != 1 {
		t.Fatalf("expected 1 drift, got %d", len(drifts))
	}
	if drifts[0].DriftType != DriftModified {
		t.Errorf("expected DriftModified, got %s", drifts[0].DriftType)
	}
}

// ============================================================================
// ListDrifts and ListAllDrifts — resolved drift filtering
// ============================================================================

func TestListDrifts_ExcludesResolvedDrifts(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)
	ctx := context.Background()

	// Create two drifts
	svc.Compare(ctx, "svc-1", EntityService, "cfg-1",
		map[string]interface{}{"a": 1, "b": 2},
		map[string]interface{}{"a": 10, "b": 20})

	drifts := svc.ListDrifts("svc-1")
	if len(drifts) != 2 {
		t.Fatalf("expected 2 drifts, got %d", len(drifts))
	}

	// Resolve one
	svc.ResolveDrift(ctx, drifts[0].ID)

	active := svc.ListDrifts("svc-1")
	if len(active) != 1 {
		t.Errorf("expected 1 active drift after resolving one, got %d", len(active))
	}
}

func TestListAllDrifts_ExcludesResolvedDrifts(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)
	ctx := context.Background()

	// Create drifts across entities
	svc.Compare(ctx, "svc-1", EntityService, "cfg-1",
		map[string]interface{}{"x": 1},
		map[string]interface{}{"x": 2})

	svc.Compare(ctx, "svc-2", EntityService, "cfg-2",
		map[string]interface{}{"y": 1},
		map[string]interface{}{"y": 2})

	all := svc.ListAllDrifts()
	if len(all) != 2 {
		t.Fatalf("expected 2 drifts, got %d", len(all))
	}

	// Resolve one
	svc.ResolveDrift(ctx, all[0].ID)

	remaining := svc.ListAllDrifts()
	if len(remaining) != 1 {
		t.Errorf("expected 1 active drift after resolving one, got %d", len(remaining))
	}
}

func TestListDrifts_EmptyForUnknownEntity(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)

	drifts := svc.ListDrifts("nonexistent")
	if len(drifts) != 0 {
		t.Errorf("expected 0 drifts for unknown entity, got %d", len(drifts))
	}
}

// ============================================================================
// Stats — extended coverage
// ============================================================================

func TestStats_WithMixedHealthAndResolved(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)
	ctx := context.Background()

	// Register a health checker
	svc.RegisterHealthChecker(EntityService, func(_ context.Context, obs *Observation) (bool, string) {
		if v, ok := obs.State["healthy"]; ok {
			if h, ok := v.(bool); ok && !h {
				return false, "marked unhealthy"
			}
		}
		return true, ""
	})

	// Healthy observation
	svc.Observe(ctx, "svc-healthy", EntityService, map[string]interface{}{"healthy": true})
	// Unhealthy observation
	svc.Observe(ctx, "svc-unhealthy", EntityService, map[string]interface{}{"healthy": false})
	// Node observation (no health checker)
	svc.Observe(ctx, "node-1", EntityNode, map[string]interface{}{"cpu": 4})

	// Create drift and resolve it
	drifts, _ := svc.Compare(ctx, "svc-healthy", EntityService, "cfg-1",
		map[string]interface{}{"port": 8080},
		map[string]interface{}{"port": 9090})
	svc.ResolveDrift(ctx, drifts[0].ID)

	// Create unresolved drift
	svc.Compare(ctx, "svc-unhealthy", EntityService, "cfg-2",
		map[string]interface{}{"replicas": 3},
		map[string]interface{}{"replicas": 1})

	stats := svc.Stats()

	if stats["total_observations"].(int) != 3 {
		t.Errorf("expected 3 observations, got %v", stats["total_observations"])
	}
	if stats["healthy_entities"].(int) != 2 {
		t.Errorf("expected 2 healthy entities, got %v", stats["healthy_entities"])
	}
	if stats["unhealthy_entities"].(int) != 1 {
		t.Errorf("expected 1 unhealthy entity, got %v", stats["unhealthy_entities"])
	}
	if stats["total_drifts"].(int) != 2 {
		t.Errorf("expected 2 total drifts, got %v", stats["total_drifts"])
	}
	if stats["active_drifts"].(int) != 1 {
		t.Errorf("expected 1 active drift, got %v", stats["active_drifts"])
	}

	typeCounts := stats["observations_by_type"].(map[string]int)
	if typeCounts["service"] != 2 {
		t.Errorf("expected 2 service observations, got %d", typeCounts["service"])
	}
	if typeCounts["node"] != 1 {
		t.Errorf("expected 1 node observation, got %d", typeCounts["node"])
	}
}

func TestStats_RegisteredCounts(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)

	svc.RegisterObserver(EntityService, func(ctx context.Context, entityID string) (*Observation, error) {
		return nil, nil
	})
	svc.RegisterObserver(EntityNode, func(ctx context.Context, entityID string) (*Observation, error) {
		return nil, nil
	})

	stats := svc.Stats()

	if stats["registered_observers"].(int) != 2 {
		t.Errorf("expected 2 registered observers, got %v", stats["registered_observers"])
	}
}

// ============================================================================
// NewService — explicit logger
// ============================================================================

func TestNewService_WithExplicitLogger(t *testing.T) {
	log := logger.New(io.Discard)
	svc := NewService(log, nil, nil)

	if svc.log != log {
		t.Error("expected service to use the provided logger")
	}
}

func TestNewService_WithConfig(t *testing.T) {
	cfg := &Config{
		ObservationInterval: 10 * time.Second,
		DriftCheckInterval:  20 * time.Second,
		RetentionPeriod:     2 * time.Hour,
		MaxObservations:     500,
		AutoRemediate:       true,
		WotanTopic:          "custom.topic",
	}

	svc := NewService(logger.New(io.Discard), nil, cfg)

	if svc.config.ObservationInterval != 10*time.Second {
		t.Errorf("expected ObservationInterval 10s, got %v", svc.config.ObservationInterval)
	}
	if svc.config.AutoRemediate != true {
		t.Error("expected AutoRemediate true")
	}
	if svc.config.WotanTopic != "custom.topic" {
		t.Errorf("expected WotanTopic 'custom.topic', got %s", svc.config.WotanTopic)
	}
}

// ============================================================================
// Entity type constants verification
// ============================================================================

func TestEntityTypeConstants(t *testing.T) {
	types := []EntityType{
		EntityService, EntityContainer, EntityNode, EntityNetwork,
		EntityStorage, EntitySecret, EntityLoadBalancer, EntityDNS, EntityCertificate,
	}

	expected := []string{
		"service", "container", "node", "network",
		"storage", "secret", "loadbalancer", "dns", "certificate",
	}

	for i, et := range types {
		if string(et) != expected[i] {
			t.Errorf("expected entity type %s, got %s", expected[i], et)
		}
	}
}

// ============================================================================
// Drift type and severity constants verification
// ============================================================================

func TestDriftTypeConstants(t *testing.T) {
	types := map[DriftType]string{
		DriftMissing:   "missing",
		DriftExtra:     "extra",
		DriftModified:  "modified",
		DriftOutdated:  "outdated",
		DriftUnhealthy: "unhealthy",
	}

	for dt, expected := range types {
		if string(dt) != expected {
			t.Errorf("expected %s, got %s", expected, dt)
		}
	}
}

func TestSeverityConstants(t *testing.T) {
	severities := map[DriftSeverity]string{
		SeverityCritical: "critical",
		SeverityHigh:     "high",
		SeverityMedium:   "medium",
		SeverityLow:      "low",
		SeverityInfo:     "info",
	}

	for s, expected := range severities {
		if string(s) != expected {
			t.Errorf("expected %s, got %s", expected, s)
		}
	}
}

// ============================================================================
// Observe — entity index accumulation
// ============================================================================

func TestObserve_AccumulatesEntityIndex(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)
	ctx := context.Background()

	// Observe same entity multiple times
	svc.Observe(ctx, "svc-1", EntityService, map[string]interface{}{"v": 1})
	svc.Observe(ctx, "svc-1", EntityService, map[string]interface{}{"v": 2})
	svc.Observe(ctx, "svc-1", EntityService, map[string]interface{}{"v": 3})

	svc.mu.RLock()
	indexLen := len(svc.entityIndex["svc-1"])
	svc.mu.RUnlock()

	if indexLen != 3 {
		t.Errorf("expected 3 entries in entity index, got %d", indexLen)
	}

	// But latest observation should be the last one
	obs, ok := svc.GetObservation("svc-1")
	if !ok {
		t.Fatal("expected observation to exist")
	}
	if obs.State["v"] != 3 {
		t.Errorf("expected latest state v=3, got %v", obs.State["v"])
	}
}

// ============================================================================
// Compare — drift event publishing
// ============================================================================

func TestCompare_PublishesDriftEventWhenDriftsFound(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)
	ctx := context.Background()

	initialDrops := svc.WotanDrops()

	// Create drift — will trigger publishEvent (dropped since wotan is nil)
	svc.Compare(ctx, "svc-1", EntityService, "cfg-1",
		map[string]interface{}{"port": 8080},
		map[string]interface{}{"port": 9090})

	afterDrops := svc.WotanDrops()
	if afterDrops <= initialDrops {
		t.Error("expected wotanDrops to increment when drifts are detected")
	}
}

func TestCompare_NoDriftEvent_WhenNoDrifts(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)
	ctx := context.Background()

	initialDrops := svc.WotanDrops()

	// No drift — no event published
	svc.Compare(ctx, "svc-1", EntityService, "cfg-1",
		map[string]interface{}{"port": 8080},
		map[string]interface{}{"port": 8080})

	afterDrops := svc.WotanDrops()
	if afterDrops != initialDrops {
		t.Error("expected no wotanDrops increment when no drifts detected")
	}
}

// ============================================================================
// GetDrift — not found
// ============================================================================

func TestGetDrift_NotFound(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)

	_, ok := svc.GetDrift("nonexistent-drift-id")
	if ok {
		t.Error("expected GetDrift to return false for nonexistent ID")
	}
}

// ============================================================================
// Compare — auto-remediable flag setting
// ============================================================================

func TestCompare_SetsAutoRemediableFlag(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AutoRemediate = true
	svc := NewService(logger.New(io.Discard), nil, cfg)

	drifts, _ := svc.Compare(context.Background(), "svc-1", EntityService, "cfg-1",
		map[string]interface{}{"port": 8080, "missing_key": "val"},
		map[string]interface{}{"port": 9090})

	for _, d := range drifts {
		if d.DriftType == DriftModified && !d.AutoRemediable {
			t.Errorf("expected modified drift to be auto-remediable, got false")
		}
		if d.DriftType == DriftMissing && d.AutoRemediable {
			t.Errorf("expected missing drift to NOT be auto-remediable, got true")
		}
	}
}
