package kenoma

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

	if cfg.ObservationInterval != 30*time.Second {
		t.Errorf("expected ObservationInterval 30s, got %v", cfg.ObservationInterval)
	}
	if cfg.DriftCheckInterval != 60*time.Second {
		t.Errorf("expected DriftCheckInterval 60s, got %v", cfg.DriftCheckInterval)
	}
	if cfg.RetentionPeriod != 24*time.Hour {
		t.Errorf("expected RetentionPeriod 24h, got %v", cfg.RetentionPeriod)
	}
	if cfg.WotanTopic != "kenoma.observations" {
		t.Errorf("expected WotanTopic 'kenoma.observations', got %s", cfg.WotanTopic)
	}
}

func TestNewService(t *testing.T) {
	svc := NewService(nil, nil, nil)
	if svc == nil {
		t.Fatal("NewService returned nil")
	}
	if svc.observations == nil {
		t.Error("observations map not initialized")
	}
	if svc.drifts == nil {
		t.Error("drifts map not initialized")
	}
	if svc.observers == nil {
		t.Error("observers map not initialized")
	}
}

// ============================================================================
// Observation Tests
// ============================================================================

func TestObserve_Basic(t *testing.T) {
	svc := NewService(nil, nil, nil)

	state := map[string]interface{}{
		"port":     8080,
		"replicas": 3,
	}

	obs, err := svc.Observe(context.Background(), "service-1", EntityService, state)
	if err != nil {
		t.Fatalf("Observe failed: %v", err)
	}

	if obs.ID == "" {
		t.Error("expected ID to be generated")
	}
	if obs.EntityID != "service-1" {
		t.Errorf("expected EntityID 'service-1', got %s", obs.EntityID)
	}
	if obs.EntityType != EntityService {
		t.Errorf("expected EntityType service, got %s", obs.EntityType)
	}
	if obs.Hash == "" {
		t.Error("expected hash to be computed")
	}
	if !obs.Healthy {
		t.Error("expected healthy to be true by default")
	}
}

func TestObserve_WithHealthChecker(t *testing.T) {
	svc := NewService(nil, nil, nil)

	// Register a health checker that marks unhealthy if replicas < 2
	svc.RegisterHealthChecker(EntityService, func(ctx context.Context, obs *Observation) (bool, string) {
		replicas, ok := obs.State["replicas"].(int)
		if !ok {
			return false, "replicas not specified"
		}
		if replicas < 2 {
			return false, "insufficient replicas"
		}
		return true, ""
	})

	// Healthy observation
	healthyState := map[string]interface{}{"replicas": 3}
	obs1, _ := svc.Observe(context.Background(), "service-1", EntityService, healthyState)
	if !obs1.Healthy {
		t.Error("expected healthy with 3 replicas")
	}

	// Unhealthy observation
	unhealthyState := map[string]interface{}{"replicas": 1}
	obs2, _ := svc.Observe(context.Background(), "service-2", EntityService, unhealthyState)
	if obs2.Healthy {
		t.Error("expected unhealthy with 1 replica")
	}
	if obs2.HealthReason != "insufficient replicas" {
		t.Errorf("expected reason 'insufficient replicas', got %s", obs2.HealthReason)
	}
}

func TestGetObservation(t *testing.T) {
	svc := NewService(nil, nil, nil)

	state := map[string]interface{}{"port": 8080}
	svc.Observe(context.Background(), "service-1", EntityService, state)

	obs, ok := svc.GetObservation("service-1")
	if !ok {
		t.Error("expected to find observation")
	}
	if obs.State["port"] != 8080 {
		t.Error("expected port 8080")
	}
}

func TestGetObservation_NotFound(t *testing.T) {
	svc := NewService(nil, nil, nil)

	_, ok := svc.GetObservation("nonexistent")
	if ok {
		t.Error("expected not to find observation")
	}
}

func TestGetActualState(t *testing.T) {
	svc := NewService(nil, nil, nil)

	state := map[string]interface{}{
		"port":     8080,
		"replicas": 3,
	}
	svc.Observe(context.Background(), "service-1", EntityService, state)

	actualState, err := svc.GetActualState("service-1")
	if err != nil {
		t.Fatalf("GetActualState failed: %v", err)
	}

	if actualState["port"] != 8080 {
		t.Errorf("expected port 8080, got %v", actualState["port"])
	}
}

func TestGetActualState_NotFound(t *testing.T) {
	svc := NewService(nil, nil, nil)

	_, err := svc.GetActualState("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent entity")
	}
}

// ============================================================================
// Drift Detection Tests
// ============================================================================

func TestCompare_NoDrift(t *testing.T) {
	svc := NewService(nil, nil, nil)

	desired := map[string]interface{}{
		"port":     8080,
		"replicas": 3,
	}
	actual := map[string]interface{}{
		"port":     8080,
		"replicas": 3,
	}

	drifts, err := svc.Compare(context.Background(), "service-1", EntityService, "cfg-1", desired, actual)
	if err != nil {
		t.Fatalf("Compare failed: %v", err)
	}

	if len(drifts) != 0 {
		t.Errorf("expected no drift, got %d drifts", len(drifts))
	}
}

func TestCompare_ModifiedValue(t *testing.T) {
	svc := NewService(nil, nil, nil)

	desired := map[string]interface{}{
		"port": 8080,
	}
	actual := map[string]interface{}{
		"port": 9090,
	}

	drifts, err := svc.Compare(context.Background(), "service-1", EntityService, "cfg-1", desired, actual)
	if err != nil {
		t.Fatalf("Compare failed: %v", err)
	}

	if len(drifts) != 1 {
		t.Fatalf("expected 1 drift, got %d", len(drifts))
	}

	drift := drifts[0]
	if drift.DriftType != DriftModified {
		t.Errorf("expected DriftModified, got %s", drift.DriftType)
	}
	if drift.Path != "port" {
		t.Errorf("expected path 'port', got %s", drift.Path)
	}
	if drift.DesiredValue != 8080 {
		t.Errorf("expected desired value 8080, got %v", drift.DesiredValue)
	}
	if drift.ActualValue != 9090 {
		t.Errorf("expected actual value 9090, got %v", drift.ActualValue)
	}
}

func TestCompare_MissingValue(t *testing.T) {
	svc := NewService(nil, nil, nil)

	desired := map[string]interface{}{
		"port":     8080,
		"replicas": 3,
	}
	actual := map[string]interface{}{
		"port": 8080,
		// replicas missing
	}

	drifts, err := svc.Compare(context.Background(), "service-1", EntityService, "cfg-1", desired, actual)
	if err != nil {
		t.Fatalf("Compare failed: %v", err)
	}

	if len(drifts) != 1 {
		t.Fatalf("expected 1 drift, got %d", len(drifts))
	}

	drift := drifts[0]
	if drift.DriftType != DriftMissing {
		t.Errorf("expected DriftMissing, got %s", drift.DriftType)
	}
	if drift.Path != "replicas" {
		t.Errorf("expected path 'replicas', got %s", drift.Path)
	}
}

func TestCompare_ExtraValue(t *testing.T) {
	svc := NewService(nil, nil, nil)

	desired := map[string]interface{}{
		"port": 8080,
	}
	actual := map[string]interface{}{
		"port":  8080,
		"extra": "value", // Not in desired
	}

	drifts, err := svc.Compare(context.Background(), "service-1", EntityService, "cfg-1", desired, actual)
	if err != nil {
		t.Fatalf("Compare failed: %v", err)
	}

	if len(drifts) != 1 {
		t.Fatalf("expected 1 drift, got %d", len(drifts))
	}

	drift := drifts[0]
	if drift.DriftType != DriftExtra {
		t.Errorf("expected DriftExtra, got %s", drift.DriftType)
	}
}

func TestCompare_MultipleDrifts(t *testing.T) {
	svc := NewService(nil, nil, nil)

	desired := map[string]interface{}{
		"port":     8080,
		"replicas": 3,
		"env":      "prod",
	}
	actual := map[string]interface{}{
		"port":  9090, // Modified
		"extra": "x",  // Extra
		// replicas and env missing
	}

	drifts, err := svc.Compare(context.Background(), "service-1", EntityService, "cfg-1", desired, actual)
	if err != nil {
		t.Fatalf("Compare failed: %v", err)
	}

	if len(drifts) != 4 {
		t.Errorf("expected 4 drifts, got %d", len(drifts))
	}

	// Check we have the expected drift types
	typeCount := make(map[DriftType]int)
	for _, d := range drifts {
		typeCount[d.DriftType]++
	}

	if typeCount[DriftModified] != 1 {
		t.Errorf("expected 1 modified drift, got %d", typeCount[DriftModified])
	}
	if typeCount[DriftMissing] != 2 {
		t.Errorf("expected 2 missing drifts, got %d", typeCount[DriftMissing])
	}
	if typeCount[DriftExtra] != 1 {
		t.Errorf("expected 1 extra drift, got %d", typeCount[DriftExtra])
	}
}

// ============================================================================
// Drift Management Tests
// ============================================================================

func TestGetDrift(t *testing.T) {
	svc := NewService(nil, nil, nil)

	desired := map[string]interface{}{"port": 8080}
	actual := map[string]interface{}{"port": 9090}

	drifts, _ := svc.Compare(context.Background(), "service-1", EntityService, "cfg-1", desired, actual)

	drift, ok := svc.GetDrift(drifts[0].ID)
	if !ok {
		t.Error("expected to find drift")
	}
	if drift.EntityID != "service-1" {
		t.Errorf("expected EntityID 'service-1', got %s", drift.EntityID)
	}
}

func TestListDrifts(t *testing.T) {
	svc := NewService(nil, nil, nil)

	// Create multiple drifts
	desired := map[string]interface{}{"port": 8080, "replicas": 3}
	actual := map[string]interface{}{"port": 9090}

	svc.Compare(context.Background(), "service-1", EntityService, "cfg-1", desired, actual)

	drifts := svc.ListDrifts("service-1")
	if len(drifts) < 2 {
		t.Errorf("expected at least 2 drifts, got %d", len(drifts))
	}
}

func TestListAllDrifts(t *testing.T) {
	svc := NewService(nil, nil, nil)

	// Create drifts for multiple entities
	svc.Compare(context.Background(), "service-1", EntityService, "cfg-1",
		map[string]interface{}{"port": 8080},
		map[string]interface{}{"port": 9090})

	svc.Compare(context.Background(), "service-2", EntityService, "cfg-2",
		map[string]interface{}{"replicas": 3},
		map[string]interface{}{"replicas": 1})

	drifts := svc.ListAllDrifts()
	if len(drifts) != 2 {
		t.Errorf("expected 2 drifts, got %d", len(drifts))
	}
}

func TestResolveDrift(t *testing.T) {
	svc := NewService(nil, nil, nil)

	desired := map[string]interface{}{"port": 8080}
	actual := map[string]interface{}{"port": 9090}

	drifts, _ := svc.Compare(context.Background(), "service-1", EntityService, "cfg-1", desired, actual)
	driftID := drifts[0].ID

	err := svc.ResolveDrift(context.Background(), driftID)
	if err != nil {
		t.Fatalf("ResolveDrift failed: %v", err)
	}

	drift, _ := svc.GetDrift(driftID)
	if drift.ResolvedAt == nil {
		t.Error("expected ResolvedAt to be set")
	}

	// Should not appear in active drifts
	activeDrifts := svc.ListDrifts("service-1")
	for _, d := range activeDrifts {
		if d.ID == driftID {
			t.Error("resolved drift should not appear in active drifts")
		}
	}
}

func TestResolveDrift_NotFound(t *testing.T) {
	svc := NewService(nil, nil, nil)

	err := svc.ResolveDrift(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent drift")
	}
}

// ============================================================================
// Severity Tests
// ============================================================================

func TestAssessSeverity(t *testing.T) {
	svc := NewService(nil, nil, nil)

	tests := []struct {
		driftType DriftType
		expected  DriftSeverity
	}{
		{DriftMissing, SeverityHigh},
		{DriftUnhealthy, SeverityCritical},
		{DriftModified, SeverityMedium},
		{DriftExtra, SeverityLow},
		{DriftOutdated, SeverityMedium},
	}

	for _, tt := range tests {
		drift := &Drift{DriftType: tt.driftType}
		severity := svc.assessSeverity(drift)
		if severity != tt.expected {
			t.Errorf("for %s, expected severity %s, got %s", tt.driftType, tt.expected, severity)
		}
	}
}

// ============================================================================
// Custom Comparator Tests
// ============================================================================

func TestRegisterComparator(t *testing.T) {
	svc := NewService(nil, nil, nil)

	// Register a custom comparator that treats 8080 and 80 as equivalent
	svc.RegisterComparator(EntityLoadBalancer, func(desired, actual map[string]interface{}) []Drift {
		var drifts []Drift
		for key, desiredVal := range desired {
			actualVal, exists := actual[key]
			if !exists {
				drifts = append(drifts, Drift{
					DriftType:    DriftMissing,
					Path:         key,
					DesiredValue: desiredVal,
				})
				continue
			}

			// Special case: treat port 80 and 8080 as equivalent
			if key == "port" {
				dPort, _ := desiredVal.(int)
				aPort, _ := actualVal.(int)
				if (dPort == 80 || dPort == 8080) && (aPort == 80 || aPort == 8080) {
					continue // No drift
				}
			}

			if desiredVal != actualVal {
				drifts = append(drifts, Drift{
					DriftType:    DriftModified,
					Path:         key,
					DesiredValue: desiredVal,
					ActualValue:  actualVal,
				})
			}
		}
		return drifts
	})

	// Test that 80 and 8080 are treated as equivalent
	drifts, _ := svc.Compare(context.Background(), "lb-1", EntityLoadBalancer, "cfg-1",
		map[string]interface{}{"port": 80},
		map[string]interface{}{"port": 8080})

	if len(drifts) != 0 {
		t.Errorf("expected no drifts (80 == 8080 for load balancer), got %d", len(drifts))
	}
}

// ============================================================================
// Stats Tests
// ============================================================================

func TestStats(t *testing.T) {
	svc := NewService(nil, nil, nil)

	// Create some observations
	svc.Observe(context.Background(), "service-1", EntityService, map[string]interface{}{"port": 8080})
	svc.Observe(context.Background(), "node-1", EntityNode, map[string]interface{}{"cpu": 4})

	// Create some drifts
	svc.Compare(context.Background(), "service-1", EntityService, "cfg-1",
		map[string]interface{}{"port": 8080},
		map[string]interface{}{"port": 9090})

	stats := svc.Stats()

	if stats["total_observations"].(int) != 2 {
		t.Errorf("expected 2 observations, got %v", stats["total_observations"])
	}
	if stats["total_drifts"].(int) != 1 {
		t.Errorf("expected 1 drift, got %v", stats["total_drifts"])
	}
	if stats["active_drifts"].(int) != 1 {
		t.Errorf("expected 1 active drift, got %v", stats["active_drifts"])
	}
}

// ============================================================================
// Concurrent Access Tests
// ============================================================================

func TestConcurrentObserve(t *testing.T) {
	svc := NewService(nil, nil, nil)
	ctx := context.Background()

	done := make(chan bool, 100)

	for i := 0; i < 100; i++ {
		go func(i int) {
			state := map[string]interface{}{"instance": i}
			_, _ = svc.Observe(ctx, "service-concurrent", EntityService, state)
			done <- true
		}(i)
	}

	for i := 0; i < 100; i++ {
		<-done
	}

	// Should have one observation (the latest)
	_, ok := svc.GetObservation("service-concurrent")
	if !ok {
		t.Error("expected observation to exist")
	}
}

func TestConcurrentCompare(t *testing.T) {
	svc := NewService(nil, nil, nil)
	ctx := context.Background()

	done := make(chan bool, 50)

	for i := 0; i < 50; i++ {
		go func(i int) {
			desired := map[string]interface{}{"port": 8080}
			actual := map[string]interface{}{"port": 8080 + i}
			_, _ = svc.Compare(ctx, "service-compare", EntityService, "cfg-compare", desired, actual)
			done <- true
		}(i)
	}

	for i := 0; i < 50; i++ {
		<-done
	}

	stats := svc.Stats()
	if stats["total_drifts"].(int) < 49 {
		t.Errorf("expected at least 49 drifts (port 8080 vs others), got %v", stats["total_drifts"])
	}
}

// ============================================================================
// Benchmarks
// ============================================================================

func BenchmarkObserve(b *testing.B) {
	svc := NewService(nil, nil, nil)
	ctx := context.Background()

	state := map[string]interface{}{
		"port":     8080,
		"replicas": 3,
		"env":      "prod",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = svc.Observe(ctx, "bench-service", EntityService, state)
	}
}

func BenchmarkCompare(b *testing.B) {
	svc := NewService(nil, nil, nil)
	ctx := context.Background()

	desired := map[string]interface{}{
		"port":     8080,
		"replicas": 3,
		"env":      "prod",
	}
	actual := map[string]interface{}{
		"port":     9090,
		"replicas": 1,
		"env":      "dev",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = svc.Compare(ctx, "bench-service", EntityService, "bench-cfg", desired, actual)
	}
}

func BenchmarkListDrifts(b *testing.B) {
	svc := NewService(nil, nil, nil)
	ctx := context.Background()

	// Create 100 drifts
	for i := 0; i < 100; i++ {
		svc.Compare(ctx, "bench-service", EntityService, "cfg",
			map[string]interface{}{"val": i},
			map[string]interface{}{"val": i + 1})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = svc.ListDrifts("bench-service")
	}
}
