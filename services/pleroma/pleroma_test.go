// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package pleroma

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

	if cfg.MaxConfigurations != 50000 {
		t.Errorf("expected MaxConfigurations 50000, got %d", cfg.MaxConfigurations)
	}
	if cfg.ValidationTimeout != 10*time.Second {
		t.Errorf("expected ValidationTimeout 10s, got %v", cfg.ValidationTimeout)
	}
	if cfg.ReconcileInterval != 30*time.Second {
		t.Errorf("expected ReconcileInterval 30s, got %v", cfg.ReconcileInterval)
	}
	if cfg.WotanTopic != "pleroma.configurations" {
		t.Errorf("expected WotanTopic 'pleroma.configurations', got %s", cfg.WotanTopic)
	}
}

func TestNewService(t *testing.T) {
	svc := NewService(nil, nil, nil)
	if svc == nil {
		t.Fatal("NewService returned nil")
	}
	if svc.configurations == nil {
		t.Error("configurations map not initialized")
	}
	if svc.reconciliations == nil {
		t.Error("reconciliations map not initialized")
	}
	if svc.validators == nil {
		t.Error("validators map not initialized")
	}
}

// ============================================================================
// Apply Tests
// ============================================================================

func TestApply_Basic(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableValidation: false})

	cfg := &Configuration{
		Name:      "test-service",
		Namespace: "default",
		Kind:      KindService,
		Spec: map[string]interface{}{
			"port": 8080,
		},
	}

	result, err := svc.Apply(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	if result.ID == "" {
		t.Error("expected ID to be generated")
	}
	if result.Version != 1 {
		t.Errorf("expected version 1, got %d", result.Version)
	}
	if result.Hash == "" {
		t.Error("expected hash to be computed")
	}
	if result.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
	if result.UpdatedAt.IsZero() {
		t.Error("expected UpdatedAt to be set")
	}
}

func TestApply_VersionIncrement(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableValidation: false})

	cfg := &Configuration{
		ID:        "test-cfg-1",
		Name:      "test-service",
		Namespace: "default",
		Kind:      KindService,
		Spec: map[string]interface{}{
			"port": 8080,
		},
	}

	// First apply
	result1, err := svc.Apply(context.Background(), cfg)
	if err != nil {
		t.Fatalf("First Apply failed: %v", err)
	}
	if result1.Version != 1 {
		t.Errorf("expected version 1, got %d", result1.Version)
	}

	// Second apply (update)
	cfg.Spec["port"] = 9090
	result2, err := svc.Apply(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Second Apply failed: %v", err)
	}
	if result2.Version != 2 {
		t.Errorf("expected version 2, got %d", result2.Version)
	}
}

func TestApply_NilConfig(t *testing.T) {
	svc := NewService(nil, nil, nil)

	_, err := svc.Apply(context.Background(), nil)
	if err == nil {
		t.Error("expected error for nil configuration")
	}
}

func TestApply_WithValidation_MissingName(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableValidation: true})

	cfg := &Configuration{
		Namespace: "default",
		Kind:      KindService,
		Spec:      map[string]interface{}{},
	}

	_, err := svc.Apply(context.Background(), cfg)
	if err == nil {
		t.Error("expected validation error for missing name")
	}
}

func TestApply_WithValidation_MissingNamespace(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableValidation: true})

	cfg := &Configuration{
		Name: "test",
		Kind: KindService,
		Spec: map[string]interface{}{},
	}

	_, err := svc.Apply(context.Background(), cfg)
	if err == nil {
		t.Error("expected validation error for missing namespace")
	}
}

// ============================================================================
// Get Tests
// ============================================================================

func TestGet(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableValidation: false})

	cfg := &Configuration{
		ID:        "test-get-1",
		Name:      "test-service",
		Namespace: "default",
		Kind:      KindService,
		Spec:      map[string]interface{}{},
	}

	svc.Apply(context.Background(), cfg)

	result, ok := svc.Get("test-get-1")
	if !ok {
		t.Error("expected to find configuration")
	}
	if result.Name != "test-service" {
		t.Errorf("expected name 'test-service', got %s", result.Name)
	}
}

func TestGet_NotFound(t *testing.T) {
	svc := NewService(nil, nil, nil)

	_, ok := svc.Get("nonexistent")
	if ok {
		t.Error("expected not to find configuration")
	}
}

func TestGetByName(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableValidation: false})

	cfg := &Configuration{
		Name:      "test-service",
		Namespace: "production",
		Kind:      KindService,
		Spec:      map[string]interface{}{},
		Status:    StatusActive,
	}

	svc.Apply(context.Background(), cfg)

	result, ok := svc.GetByName("production", "test-service")
	if !ok {
		t.Error("expected to find configuration by name")
	}
	if result.Namespace != "production" {
		t.Errorf("expected namespace 'production', got %s", result.Namespace)
	}
}

// ============================================================================
// List Tests
// ============================================================================

func TestList_ByNamespace(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableValidation: false})

	// Create configs in different namespaces
	svc.Apply(context.Background(), &Configuration{
		Name:      "svc1",
		Namespace: "prod",
		Kind:      KindService,
		Spec:      map[string]interface{}{},
		Status:    StatusActive,
	})
	svc.Apply(context.Background(), &Configuration{
		Name:      "svc2",
		Namespace: "prod",
		Kind:      KindService,
		Spec:      map[string]interface{}{},
		Status:    StatusActive,
	})
	svc.Apply(context.Background(), &Configuration{
		Name:      "svc3",
		Namespace: "dev",
		Kind:      KindService,
		Spec:      map[string]interface{}{},
		Status:    StatusActive,
	})

	results := svc.List("prod", "", nil)
	if len(results) != 2 {
		t.Errorf("expected 2 results for prod namespace, got %d", len(results))
	}
}

func TestList_ByKind(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableValidation: false})

	svc.Apply(context.Background(), &Configuration{
		Name:      "svc1",
		Namespace: "default",
		Kind:      KindService,
		Spec:      map[string]interface{}{},
		Status:    StatusActive,
	})
	svc.Apply(context.Background(), &Configuration{
		Name:      "net1",
		Namespace: "default",
		Kind:      KindNetwork,
		Spec:      map[string]interface{}{},
		Status:    StatusActive,
	})

	results := svc.List("", KindService, nil)
	if len(results) != 1 {
		t.Errorf("expected 1 service, got %d", len(results))
	}
}

func TestList_ByLabels(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableValidation: false})

	svc.Apply(context.Background(), &Configuration{
		Name:      "svc1",
		Namespace: "default",
		Kind:      KindService,
		Spec:      map[string]interface{}{},
		Labels:    map[string]string{"env": "prod", "team": "platform"},
		Status:    StatusActive,
	})
	svc.Apply(context.Background(), &Configuration{
		Name:      "svc2",
		Namespace: "default",
		Kind:      KindService,
		Spec:      map[string]interface{}{},
		Labels:    map[string]string{"env": "dev", "team": "platform"},
		Status:    StatusActive,
	})

	results := svc.List("", "", map[string]string{"env": "prod"})
	if len(results) != 1 {
		t.Errorf("expected 1 result with env=prod, got %d", len(results))
	}
}

// ============================================================================
// Delete Tests
// ============================================================================

func TestDelete(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableValidation: false})

	cfg := &Configuration{
		ID:        "test-delete-1",
		Name:      "test-service",
		Namespace: "default",
		Kind:      KindService,
		Spec:      map[string]interface{}{},
		Status:    StatusActive,
	}

	svc.Apply(context.Background(), cfg)

	err := svc.Delete(context.Background(), "test-delete-1")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	result, ok := svc.Get("test-delete-1")
	if !ok {
		t.Fatal("expected config to still exist")
	}
	if result.Status != StatusDeleted {
		t.Errorf("expected status Deleted, got %s", result.Status)
	}
}

func TestDelete_NotFound(t *testing.T) {
	svc := NewService(nil, nil, nil)

	err := svc.Delete(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent configuration")
	}
}

// ============================================================================
// Validator Tests
// ============================================================================

func TestRegisterValidator(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableValidation: true})

	customValidator := func(ctx context.Context, cfg *Configuration) *Validation {
		v := &Validation{ConfigID: cfg.ID, Valid: true}
		if cfg.Spec["required_field"] == nil {
			v.Valid = false
			v.Errors = append(v.Errors, ValidationError{
				Field:   "spec.required_field",
				Message: "required_field is required",
				Code:    "required",
			})
		}
		return v
	}

	svc.RegisterValidator(KindCustom, customValidator)

	// Test that validator is called
	cfg := &Configuration{
		Name:      "test",
		Namespace: "default",
		Kind:      KindCustom,
		Spec:      map[string]interface{}{},
	}

	_, err := svc.Apply(context.Background(), cfg)
	if err == nil {
		t.Error("expected validation to fail")
	}

	// Now with required field
	cfg.Spec["required_field"] = "value"
	_, err = svc.Apply(context.Background(), cfg)
	if err != nil {
		t.Errorf("expected validation to pass: %v", err)
	}
}

// ============================================================================
// GetDesiredState Tests
// ============================================================================

func TestGetDesiredState(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableValidation: false})

	cfg := &Configuration{
		ID:        "test-state-1",
		Name:      "test-service",
		Namespace: "default",
		Kind:      KindService,
		Spec: map[string]interface{}{
			"port":     8080,
			"replicas": 3,
		},
	}

	svc.Apply(context.Background(), cfg)

	state, err := svc.GetDesiredState("test-state-1")
	if err != nil {
		t.Fatalf("GetDesiredState failed: %v", err)
	}

	if state["port"] != 8080 {
		t.Errorf("expected port 8080, got %v", state["port"])
	}
	if state["replicas"] != 3 {
		t.Errorf("expected replicas 3, got %v", state["replicas"])
	}
}

func TestGetDesiredState_NotFound(t *testing.T) {
	svc := NewService(nil, nil, nil)

	_, err := svc.GetDesiredState("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent configuration")
	}
}

// ============================================================================
// Reconciliation Tests
// ============================================================================

func TestRecordReconciliation(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableValidation: false})

	cfg := &Configuration{
		ID:        "test-reconcile-1",
		Name:      "test-service",
		Namespace: "default",
		Kind:      KindService,
		Spec: map[string]interface{}{
			"port": 8080,
		},
	}

	svc.Apply(context.Background(), cfg)

	actualState := map[string]interface{}{
		"port": 9090, // Different from desired
	}

	differences := []Difference{
		{
			Path:         "port",
			DesiredValue: 8080,
			ActualValue:  9090,
			ChangeType:   ChangeUpdate,
		},
	}

	reconcile, err := svc.RecordReconciliation(context.Background(), "test-reconcile-1", actualState, differences, false, "port mismatch")
	if err != nil {
		t.Fatalf("RecordReconciliation failed: %v", err)
	}

	if reconcile.ID == "" {
		t.Error("expected reconciliation ID")
	}
	if reconcile.Status != ReconcileFailed {
		t.Errorf("expected status Failed, got %s", reconcile.Status)
	}
	if len(reconcile.Differences) != 1 {
		t.Errorf("expected 1 difference, got %d", len(reconcile.Differences))
	}
}

// ============================================================================
// Stats Tests
// ============================================================================

func TestStats(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableValidation: false})

	// Apply some configurations
	svc.Apply(context.Background(), &Configuration{
		Name:      "svc1",
		Namespace: "default",
		Kind:      KindService,
		Spec:      map[string]interface{}{},
		Status:    StatusActive,
	})
	svc.Apply(context.Background(), &Configuration{
		Name:      "net1",
		Namespace: "default",
		Kind:      KindNetwork,
		Spec:      map[string]interface{}{},
		Status:    StatusActive,
	})

	stats := svc.Stats()

	if stats["total_configurations"].(int) != 2 {
		t.Errorf("expected 2 configurations, got %v", stats["total_configurations"])
	}

	byKind := stats["by_kind"].(map[string]int)
	if byKind["service"] != 1 {
		t.Errorf("expected 1 service, got %d", byKind["service"])
	}
}

// ============================================================================
// Hash Tests
// ============================================================================

func TestComputeHash(t *testing.T) {
	svc := NewService(nil, nil, nil)

	spec1 := map[string]interface{}{"port": 8080}
	spec2 := map[string]interface{}{"port": 8080}
	spec3 := map[string]interface{}{"port": 9090}

	hash1 := svc.computeHash(spec1)
	hash2 := svc.computeHash(spec2)
	hash3 := svc.computeHash(spec3)

	if hash1 != hash2 {
		t.Error("identical specs should have same hash")
	}
	if hash1 == hash3 {
		t.Error("different specs should have different hashes")
	}
}

// ============================================================================
// Concurrent Access Tests
// ============================================================================

func TestConcurrentApply(t *testing.T) {
	svc := NewService(nil, nil, &Config{EnableValidation: false})
	ctx := context.Background()

	done := make(chan bool, 100)

	for i := 0; i < 100; i++ {
		go func(i int) {
			cfg := &Configuration{
				Name:      "concurrent-svc",
				Namespace: "default",
				Kind:      KindService,
				Spec: map[string]interface{}{
					"instance": i,
				},
			}
			_, _ = svc.Apply(ctx, cfg)
			done <- true
		}(i)
	}

	for i := 0; i < 100; i++ {
		<-done
	}

	// Should have one configuration with the latest spec
	stats := svc.Stats()
	if stats["total_configurations"].(int) < 1 {
		t.Error("expected at least 1 configuration")
	}
}

// ============================================================================
// Benchmarks
// ============================================================================

func BenchmarkApply(b *testing.B) {
	svc := NewService(nil, nil, &Config{EnableValidation: false})
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cfg := &Configuration{
			Name:      "bench-svc",
			Namespace: "default",
			Kind:      KindService,
			Spec: map[string]interface{}{
				"port": 8080 + i,
			},
		}
		_, _ = svc.Apply(ctx, cfg)
	}
}

func BenchmarkGet(b *testing.B) {
	svc := NewService(nil, nil, &Config{EnableValidation: false})
	ctx := context.Background()

	cfg := &Configuration{
		ID:        "bench-get",
		Name:      "bench-svc",
		Namespace: "default",
		Kind:      KindService,
		Spec:      map[string]interface{}{},
	}
	svc.Apply(ctx, cfg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = svc.Get("bench-get")
	}
}

func BenchmarkList(b *testing.B) {
	svc := NewService(nil, nil, &Config{EnableValidation: false})
	ctx := context.Background()

	// Add 100 configurations
	for i := 0; i < 100; i++ {
		cfg := &Configuration{
			Name:      "bench-svc",
			Namespace: "default",
			Kind:      KindService,
			Spec:      map[string]interface{}{},
			Status:    StatusActive,
		}
		svc.Apply(ctx, cfg)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = svc.List("default", "", nil)
	}
}
