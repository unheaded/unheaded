// Package state provides state management tests for the Cuirass control plane
package state

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// MANAGER TESTS - THE CORE HEART'S TRIALS
// ============================================================================

func TestNewManager_DefaultConfig(t *testing.T) {
	m := NewManager(ManagerConfig{})
	if m == nil {
		t.Fatal("NewManager() returned nil")
	}
	if m.pollInterval != 30*time.Second {
		t.Errorf("expected default poll interval 30s, got %v", m.pollInterval)
	}
	if m.closed {
		t.Error("new manager should not be closed")
	}
}

func TestNewManager_CustomConfig(t *testing.T) {
	cfg := ManagerConfig{
		PollInterval: 10 * time.Second,
		OnDrift: func(dr DriftReport) {
			// Callback provided to verify it gets set
		},
	}

	m := NewManager(cfg)
	if m.pollInterval != 10*time.Second {
		t.Errorf("expected poll interval 10s, got %v", m.pollInterval)
	}
	if m.onDrift == nil {
		t.Error("expected onDrift callback to be set")
	}
}

// ============================================================================
// DESIRED STATE TESTS
// ============================================================================

func TestManager_SetDesiredState_Valid(t *testing.T) {
	m := NewManager(ManagerConfig{})
	ctx := context.Background()

	spec := &ContainerSpec{
		ID:    "citadel-1",
		Name:  "gateway-citadel",
		Image: "nixos:latest",
		Type:  "gateway",
		CPU:   2,
		Memory: 1024,
	}

	err := m.SetDesiredState(ctx, spec)
	if err != nil {
		t.Fatalf("SetDesiredState() error: %v", err)
	}

	// Verify it was stored
	got, err := m.GetDesiredState(ctx, "citadel-1")
	if err != nil {
		t.Fatalf("GetDesiredState() error: %v", err)
	}
	if got.Name != "gateway-citadel" {
		t.Errorf("expected name 'gateway-citadel', got %q", got.Name)
	}
}

func TestManager_SetDesiredState_NilSpec(t *testing.T) {
	m := NewManager(ManagerConfig{})
	ctx := context.Background()

	err := m.SetDesiredState(ctx, nil)
	if err == nil {
		t.Error("expected error for nil spec, got nil")
	}
}

func TestManager_SetDesiredState_EmptyID(t *testing.T) {
	m := NewManager(ManagerConfig{})
	ctx := context.Background()

	spec := &ContainerSpec{
		ID:    "",
		Name:  "test",
		Image: "nixos:latest",
	}

	err := m.SetDesiredState(ctx, spec)
	if err == nil {
		t.Error("expected error for empty ID, got nil")
	}
}

func TestManager_SetDesiredState_EmptyName(t *testing.T) {
	m := NewManager(ManagerConfig{})
	ctx := context.Background()

	spec := &ContainerSpec{
		ID:    "test-1",
		Name:  "",
		Image: "nixos:latest",
	}

	err := m.SetDesiredState(ctx, spec)
	if err == nil {
		t.Error("expected error for empty name, got nil")
	}
}

func TestManager_SetDesiredState_Closed(t *testing.T) {
	m := NewManager(ManagerConfig{})
	ctx := context.Background()
	m.Close()

	spec := &ContainerSpec{
		ID:    "citadel-1",
		Name:  "test",
		Image: "nixos:latest",
	}

	err := m.SetDesiredState(ctx, spec)
	if err != ErrStateClosed {
		t.Errorf("expected ErrStateClosed, got %v", err)
	}
}

func TestManager_RemoveDesiredState_Valid(t *testing.T) {
	m := NewManager(ManagerConfig{})
	ctx := context.Background()

	// Add first
	spec := &ContainerSpec{
		ID:    "citadel-1",
		Name:  "test",
		Image: "nixos:latest",
	}
	m.SetDesiredState(ctx, spec)

	// Remove
	err := m.RemoveDesiredState(ctx, "citadel-1")
	if err != nil {
		t.Fatalf("RemoveDesiredState() error: %v", err)
	}

	// Verify removed
	_, err = m.GetDesiredState(ctx, "citadel-1")
	if err != ErrContainerNotFound {
		t.Errorf("expected ErrContainerNotFound, got %v", err)
	}
}

func TestManager_RemoveDesiredState_EmptyID(t *testing.T) {
	m := NewManager(ManagerConfig{})
	ctx := context.Background()

	err := m.RemoveDesiredState(ctx, "")
	if err != ErrEmptyID {
		t.Errorf("expected ErrEmptyID, got %v", err)
	}
}

func TestManager_GetDesiredState_NotFound(t *testing.T) {
	m := NewManager(ManagerConfig{})
	ctx := context.Background()

	_, err := m.GetDesiredState(ctx, "nonexistent")
	if err != ErrContainerNotFound {
		t.Errorf("expected ErrContainerNotFound, got %v", err)
	}
}

func TestManager_ListDesiredStates(t *testing.T) {
	m := NewManager(ManagerConfig{})
	ctx := context.Background()

	specs := []*ContainerSpec{
		{ID: "citadel-1", Name: "gateway", Image: "nixos:latest"},
		{ID: "citadel-2", Name: "worker", Image: "nixos:latest"},
		{ID: "citadel-3", Name: "service", Image: "nixos:latest"},
	}

	for _, spec := range specs {
		m.SetDesiredState(ctx, spec)
	}

	list, err := m.ListDesiredStates(ctx)
	if err != nil {
		t.Fatalf("ListDesiredStates() error: %v", err)
	}
	if len(list) != 3 {
		t.Errorf("expected 3 specs, got %d", len(list))
	}
}

// ============================================================================
// ACTUAL STATE TESTS
// ============================================================================

func TestManager_UpdateActualState_Valid(t *testing.T) {
	m := NewManager(ManagerConfig{})
	ctx := context.Background()

	state := &ContainerState{
		ID:        "citadel-1",
		Status:    StatusRunning,
		IPAddress: "10.0.1.100",
		CPUUsage:  25.5,
		Health:    HealthHealthy,
	}

	err := m.UpdateActualState(ctx, state)
	if err != nil {
		t.Fatalf("UpdateActualState() error: %v", err)
	}

	got, err := m.GetActualState(ctx, "citadel-1")
	if err != nil {
		t.Fatalf("GetActualState() error: %v", err)
	}
	if got.Status != StatusRunning {
		t.Errorf("expected status Running, got %v", got.Status)
	}
}

func TestManager_UpdateActualState_NilState(t *testing.T) {
	m := NewManager(ManagerConfig{})
	ctx := context.Background()

	err := m.UpdateActualState(ctx, nil)
	if err != ErrNilState {
		t.Errorf("expected ErrNilState, got %v", err)
	}
}

func TestManager_UpdateActualState_EmptyID(t *testing.T) {
	m := NewManager(ManagerConfig{})
	ctx := context.Background()

	state := &ContainerState{
		ID:     "",
		Status: StatusRunning,
	}

	err := m.UpdateActualState(ctx, state)
	if err != ErrEmptyID {
		t.Errorf("expected ErrEmptyID, got %v", err)
	}
}

func TestManager_GetActualState_NotFound(t *testing.T) {
	m := NewManager(ManagerConfig{})
	ctx := context.Background()

	_, err := m.GetActualState(ctx, "nonexistent")
	if err != ErrContainerNotFound {
		t.Errorf("expected ErrContainerNotFound, got %v", err)
	}
}

func TestManager_ListActualStates(t *testing.T) {
	m := NewManager(ManagerConfig{})
	ctx := context.Background()

	states := []*ContainerState{
		{ID: "citadel-1", Status: StatusRunning, Health: HealthHealthy},
		{ID: "citadel-2", Status: StatusRunning, Health: HealthHealthy},
	}

	for _, s := range states {
		m.UpdateActualState(ctx, s)
	}

	list, err := m.ListActualStates(ctx)
	if err != nil {
		t.Fatalf("ListActualStates() error: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 states, got %d", len(list))
	}
}

// ============================================================================
// DRIFT DETECTION TESTS - THE HEART SEES ALL
// ============================================================================

func TestManager_DetectDrift_NoDrift(t *testing.T) {
	m := NewManager(ManagerConfig{})
	ctx := context.Background()

	// Add desired and matching actual
	m.SetDesiredState(ctx, &ContainerSpec{
		ID:    "citadel-1",
		Name:  "gateway",
		Image: "nixos:latest",
	})
	m.UpdateActualState(ctx, &ContainerState{
		ID:     "citadel-1",
		Status: StatusRunning,
		Health: HealthHealthy,
	})

	reports, err := m.DetectDrift(ctx)
	if err != nil {
		t.Fatalf("DetectDrift() error: %v", err)
	}
	if len(reports) != 0 {
		t.Errorf("expected no drift, got %d reports", len(reports))
	}
}

func TestManager_DetectDrift_Missing(t *testing.T) {
	m := NewManager(ManagerConfig{})
	ctx := context.Background()

	// Desired but no actual
	m.SetDesiredState(ctx, &ContainerSpec{
		ID:    "citadel-1",
		Name:  "gateway",
		Image: "nixos:latest",
	})

	reports, err := m.DetectDrift(ctx)
	if err != nil {
		t.Fatalf("DetectDrift() error: %v", err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected 1 drift report, got %d", len(reports))
	}
	if reports[0].DriftType != DriftMissing {
		t.Errorf("expected DriftMissing, got %v", reports[0].DriftType)
	}
	if reports[0].Severity != "high" {
		t.Errorf("expected severity 'high', got %q", reports[0].Severity)
	}
}

func TestManager_DetectDrift_Orphaned(t *testing.T) {
	m := NewManager(ManagerConfig{})
	ctx := context.Background()

	// Actual but no desired
	m.UpdateActualState(ctx, &ContainerState{
		ID:     "citadel-1",
		Status: StatusRunning,
		Health: HealthHealthy,
	})

	reports, err := m.DetectDrift(ctx)
	if err != nil {
		t.Fatalf("DetectDrift() error: %v", err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected 1 drift report, got %d", len(reports))
	}
	if reports[0].DriftType != DriftOrphaned {
		t.Errorf("expected DriftOrphaned, got %v", reports[0].DriftType)
	}
	if reports[0].Severity != "low" {
		t.Errorf("expected severity 'low', got %q", reports[0].Severity)
	}
}

func TestManager_DetectDrift_StatusDrift(t *testing.T) {
	m := NewManager(ManagerConfig{})
	ctx := context.Background()

	m.SetDesiredState(ctx, &ContainerSpec{
		ID:    "citadel-1",
		Name:  "gateway",
		Image: "nixos:latest",
	})
	m.UpdateActualState(ctx, &ContainerState{
		ID:     "citadel-1",
		Status: StatusStopped, // Not running!
		Health: HealthHealthy,
	})

	reports, err := m.DetectDrift(ctx)
	if err != nil {
		t.Fatalf("DetectDrift() error: %v", err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected 1 drift report, got %d", len(reports))
	}
	if reports[0].DriftType != DriftStatus {
		t.Errorf("expected DriftStatus, got %v", reports[0].DriftType)
	}
}

func TestManager_DetectDrift_Degraded(t *testing.T) {
	m := NewManager(ManagerConfig{})
	ctx := context.Background()

	m.SetDesiredState(ctx, &ContainerSpec{
		ID:    "citadel-1",
		Name:  "gateway",
		Image: "nixos:latest",
	})
	m.UpdateActualState(ctx, &ContainerState{
		ID:     "citadel-1",
		Status: StatusRunning,
		Health: HealthUnhealthy, // Unhealthy!
	})

	reports, err := m.DetectDrift(ctx)
	if err != nil {
		t.Fatalf("DetectDrift() error: %v", err)
	}
	// Should have degraded drift report
	hasDegraded := false
	for _, r := range reports {
		if r.DriftType == DriftDegraded {
			hasDegraded = true
			if r.Severity != "medium" {
				t.Errorf("expected severity 'medium', got %q", r.Severity)
			}
		}
	}
	if !hasDegraded {
		t.Error("expected DriftDegraded report")
	}
}

func TestManager_DetectDrift_CallsCallback(t *testing.T) {
	driftReports := make([]DriftReport, 0)
	m := NewManager(ManagerConfig{
		OnDrift: func(dr DriftReport) {
			driftReports = append(driftReports, dr)
		},
	})
	ctx := context.Background()

	// Create a missing drift scenario
	m.SetDesiredState(ctx, &ContainerSpec{
		ID:    "citadel-1",
		Name:  "gateway",
		Image: "nixos:latest",
	})

	m.DetectDrift(ctx)

	if len(driftReports) != 1 {
		t.Errorf("expected callback to be called once, got %d calls", len(driftReports))
	}
}

// ============================================================================
// LIFECYCLE TESTS
// ============================================================================

func TestManager_Close(t *testing.T) {
	m := NewManager(ManagerConfig{})

	err := m.Close()
	if err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	if !m.IsClosed() {
		t.Error("expected manager to be closed")
	}
}

func TestManager_OperationsAfterClose(t *testing.T) {
	m := NewManager(ManagerConfig{})
	ctx := context.Background()
	m.Close()

	tests := []struct {
		name string
		fn   func() error
	}{
		{"SetDesiredState", func() error {
			return m.SetDesiredState(ctx, &ContainerSpec{ID: "test", Name: "test", Image: "test"})
		}},
		{"RemoveDesiredState", func() error {
			return m.RemoveDesiredState(ctx, "test")
		}},
		{"GetDesiredState", func() error {
			_, err := m.GetDesiredState(ctx, "test")
			return err
		}},
		{"ListDesiredStates", func() error {
			_, err := m.ListDesiredStates(ctx)
			return err
		}},
		{"UpdateActualState", func() error {
			return m.UpdateActualState(ctx, &ContainerState{ID: "test", Status: StatusRunning})
		}},
		{"GetActualState", func() error {
			_, err := m.GetActualState(ctx, "test")
			return err
		}},
		{"ListActualStates", func() error {
			_, err := m.ListActualStates(ctx)
			return err
		}},
		{"DetectDrift", func() error {
			_, err := m.DetectDrift(ctx)
			return err
		}},
		{"GetDriftReports", func() error {
			_, err := m.GetDriftReports(ctx)
			return err
		}},
		{"ExportState", func() error {
			_, err := m.ExportState(ctx)
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			if err != ErrStateClosed {
				t.Errorf("expected ErrStateClosed, got %v", err)
			}
		})
	}
}

// ============================================================================
// METRICS TESTS
// ============================================================================

func TestManager_GetMetrics(t *testing.T) {
	m := NewManager(ManagerConfig{})
	ctx := context.Background()

	// Add some state
	m.SetDesiredState(ctx, &ContainerSpec{ID: "c1", Name: "test1", Image: "nixos"})
	m.SetDesiredState(ctx, &ContainerSpec{ID: "c2", Name: "test2", Image: "nixos"})
	m.UpdateActualState(ctx, &ContainerState{ID: "c1", Status: StatusRunning})

	m.DetectDrift(ctx)

	metrics, err := m.GetMetrics(ctx)
	if err != nil {
		t.Fatalf("GetMetrics() error: %v", err)
	}

	if metrics.ContainersDesired != 2 {
		t.Errorf("expected 2 desired containers, got %d", metrics.ContainersDesired)
	}
	if metrics.ContainersActual != 1 {
		t.Errorf("expected 1 actual container, got %d", metrics.ContainersActual)
	}
	if metrics.DriftsDetected != 1 {
		t.Errorf("expected 1 drift, got %d", metrics.DriftsDetected)
	}
}

// ============================================================================
// SERIALIZATION TESTS
// ============================================================================

func TestManager_ExportState(t *testing.T) {
	m := NewManager(ManagerConfig{})
	ctx := context.Background()

	m.SetDesiredState(ctx, &ContainerSpec{
		ID:    "citadel-1",
		Name:  "gateway",
		Image: "nixos:latest",
		CPU:   2,
		Memory: 1024,
	})
	m.UpdateActualState(ctx, &ContainerState{
		ID:        "citadel-1",
		Status:    StatusRunning,
		IPAddress: "10.0.1.100",
		Health:    HealthHealthy,
	})

	data, err := m.ExportState(ctx)
	if err != nil {
		t.Fatalf("ExportState() error: %v", err)
	}

	// Should be valid JSON
	if len(data) == 0 {
		t.Error("ExportState() returned empty data")
	}

	// Should contain our container
	json := string(data)
	if !contains(json, "citadel-1") {
		t.Error("exported state should contain citadel-1")
	}
	if !contains(json, "gateway") {
		t.Error("exported state should contain gateway")
	}
}

// ============================================================================
// CONCURRENCY TESTS - THE ARMOR HOLDS UNDER PRESSURE
// ============================================================================

func TestManager_ConcurrentDesiredStateOperations(t *testing.T) {
	m := NewManager(ManagerConfig{})
	ctx := context.Background()

	const numGoroutines = 100
	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2)

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			spec := &ContainerSpec{
				ID:    fmt.Sprintf("citadel-%d", id),
				Name:  fmt.Sprintf("worker-%d", id),
				Image: "nixos:latest",
			}
			m.SetDesiredState(ctx, spec)
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			m.ListDesiredStates(ctx)
		}()
	}

	wg.Wait()
}

func TestManager_ConcurrentDriftDetection(t *testing.T) {
	m := NewManager(ManagerConfig{})
	ctx := context.Background()

	// Setup some state
	for i := 0; i < 10; i++ {
		m.SetDesiredState(ctx, &ContainerSpec{
			ID:    fmt.Sprintf("citadel-%d", i),
			Name:  fmt.Sprintf("worker-%d", i),
			Image: "nixos:latest",
		})
	}

	const numGoroutines = 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			m.DetectDrift(ctx)
		}()
	}

	wg.Wait()
}

// ============================================================================
// VALIDATION TESTS
// ============================================================================

func TestContainerSpec_Validate_AllFields(t *testing.T) {
	tests := []struct {
		name    string
		spec    *ContainerSpec
		wantErr bool
	}{
		{
			"valid minimal",
			&ContainerSpec{ID: "test", Name: "test", Image: "nixos"},
			false,
		},
		{
			"nil",
			nil,
			true,
		},
		{
			"empty ID",
			&ContainerSpec{ID: "", Name: "test", Image: "nixos"},
			true,
		},
		{
			"empty name",
			&ContainerSpec{ID: "test", Name: "", Image: "nixos"},
			true,
		},
		{
			"empty image",
			&ContainerSpec{ID: "test", Name: "test", Image: ""},
			true,
		},
		{
			"negative CPU",
			&ContainerSpec{ID: "test", Name: "test", Image: "nixos", CPU: -1},
			true,
		},
		{
			"negative memory",
			&ContainerSpec{ID: "test", Name: "test", Image: "nixos", Memory: -1},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ============================================================================
// BENCHMARKS - THE FORGE TESTS STEEL
// ============================================================================

func BenchmarkManager_SetDesiredState(b *testing.B) {
	m := NewManager(ManagerConfig{})
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		spec := &ContainerSpec{
			ID:    fmt.Sprintf("citadel-%d", i%1000),
			Name:  "worker",
			Image: "nixos:latest",
		}
		m.SetDesiredState(ctx, spec)
	}
}

func BenchmarkManager_DetectDrift(b *testing.B) {
	m := NewManager(ManagerConfig{})
	ctx := context.Background()

	// Setup 100 containers, 50 with drift
	for i := 0; i < 100; i++ {
		m.SetDesiredState(ctx, &ContainerSpec{
			ID:    fmt.Sprintf("citadel-%d", i),
			Name:  "worker",
			Image: "nixos:latest",
		})
		if i < 50 {
			m.UpdateActualState(ctx, &ContainerState{
				ID:     fmt.Sprintf("citadel-%d", i),
				Status: StatusRunning,
				Health: HealthHealthy,
			})
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.DetectDrift(ctx)
	}
}

// ============================================================================
// HELPERS
// ============================================================================

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr, 0))
}

func containsAt(s, substr string, start int) bool {
	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
