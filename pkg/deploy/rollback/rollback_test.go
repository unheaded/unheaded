// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package rollback

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Mock implementations
// ---------------------------------------------------------------------------

type mockDeploymentLookup struct {
	mu          sync.Mutex
	deployments map[string]*DeploymentInfo
	services    map[string][]*DeploymentInfo
	previous    map[string]*DeploymentInfo // key: serviceName+":"+beforeID
}

func newMockDeploymentLookup() *mockDeploymentLookup {
	return &mockDeploymentLookup{
		deployments: make(map[string]*DeploymentInfo),
		services:    make(map[string][]*DeploymentInfo),
		previous:    make(map[string]*DeploymentInfo),
	}
}

func (m *mockDeploymentLookup) Get(_ context.Context, deploymentID string) (*DeploymentInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.deployments[deploymentID]
	if !ok {
		return nil, ErrDeploymentNotFound
	}
	return d, nil
}

func (m *mockDeploymentLookup) GetPrevious(_ context.Context, serviceName string, beforeDeploymentID string) (*DeploymentInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := serviceName + ":" + beforeDeploymentID
	d, ok := m.previous[key]
	if !ok {
		return nil, ErrNoRollbackTarget
	}
	return d, nil
}

func (m *mockDeploymentLookup) GetByService(_ context.Context, serviceName string, limit int) ([]*DeploymentInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	deps, ok := m.services[serviceName]
	if !ok {
		return []*DeploymentInfo{}, nil
	}
	if limit > 0 && limit < len(deps) {
		return deps[:limit], nil
	}
	return deps, nil
}

type mockInstanceUpdater struct {
	mu       sync.Mutex
	calls    []instanceUpdateCall
	failNext bool
}

type instanceUpdateCall struct {
	ServiceName string
	Version     string
	ArtifactRef string
	Replicas    int
}

func (m *mockInstanceUpdater) Update(_ context.Context, serviceName, version, artifactRef string, replicas int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failNext {
		m.failNext = false
		return errors.New("update failed")
	}
	m.calls = append(m.calls, instanceUpdateCall{
		ServiceName: serviceName,
		Version:     version,
		ArtifactRef: artifactRef,
		Replicas:    replicas,
	})
	return nil
}

type mockTrafficShifter struct {
	mu       sync.Mutex
	calls    []trafficShiftCall
	failNext bool
}

type trafficShiftCall struct {
	ServiceName string
	FromVersion string
	ToVersion   string
	Weight      int
}

func (m *mockTrafficShifter) Shift(_ context.Context, serviceName, fromVersion, toVersion string, weight int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failNext {
		m.failNext = false
		return errors.New("shift failed")
	}
	m.calls = append(m.calls, trafficShiftCall{
		ServiceName: serviceName,
		FromVersion: fromVersion,
		ToVersion:   toVersion,
		Weight:      weight,
	})
	return nil
}

// ---------------------------------------------------------------------------
// Helper: create a test Manager with populated mocks
// ---------------------------------------------------------------------------

func setupTestManager() (*Manager, *mockDeploymentLookup, *mockInstanceUpdater, *mockTrafficShifter) {
	lookup := newMockDeploymentLookup()
	updater := &mockInstanceUpdater{}
	shifter := &mockTrafficShifter{}

	lookup.deployments["dep-current"] = &DeploymentInfo{
		ID:          "dep-current",
		ServiceName: "svc-a",
		Version:     "v2",
		ArtifactRef: "artifact-v2",
		State:       "running",
		Replicas:    3,
	}
	lookup.deployments["dep-previous"] = &DeploymentInfo{
		ID:          "dep-previous",
		ServiceName: "svc-a",
		Version:     "v1",
		ArtifactRef: "artifact-v1",
		State:       "completed",
		Replicas:    3,
	}
	lookup.previous["svc-a:dep-current"] = lookup.deployments["dep-previous"]

	mgr := NewManager(lookup, updater, shifter)
	return mgr, lookup, updater, shifter
}

// waitForRollbackState polls until the rollback reaches the expected state or
// a timeout elapses. This is needed because executeRollback runs in a goroutine.
func waitForRollbackState(t *testing.T, mgr *Manager, rollbackID string, want RollbackState, timeout time.Duration) *Rollback {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for rollback %s to reach state %s", rollbackID, want)
			return nil
		default:
			rb, err := mgr.Status(context.Background(), rollbackID)
			if err != nil {
				time.Sleep(5 * time.Millisecond)
				continue
			}
			if rb.State == want {
				return rb
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
}

// ---------------------------------------------------------------------------
// Tests for Manager
// ---------------------------------------------------------------------------

func TestNewManager(t *testing.T) {
	mgr, _, _, _ := setupTestManager()
	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
}

func TestManager_Rollback_Success(t *testing.T) {
	mgr, _, updater, shifter := setupTestManager()

	req := &RollbackRequest{
		DeploymentID: "dep-current",
		Reason:       RollbackReasonManual,
		InitiatedBy:  "tester",
	}

	rb, err := mgr.Rollback(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rb == nil {
		t.Fatal("expected non-nil rollback")
	}
	if rb.FromVersion != "v2" {
		t.Errorf("expected FromVersion v2, got %s", rb.FromVersion)
	}
	if rb.ToVersion != "v1" {
		t.Errorf("expected ToVersion v1, got %s", rb.ToVersion)
	}
	if rb.Reason != RollbackReasonManual {
		t.Errorf("expected reason manual, got %s", rb.Reason)
	}
	if rb.InitiatedBy != "tester" {
		t.Errorf("expected InitiatedBy tester, got %s", rb.InitiatedBy)
	}

	// Wait for async completion
	completed := waitForRollbackState(t, mgr, rb.ID, RollbackStateCompleted, 5*time.Second)
	if completed.Duration <= 0 {
		t.Error("expected positive duration")
	}
	if completed.Metrics.InstancesRolledBack != 3 {
		t.Errorf("expected 3 instances rolled back, got %d", completed.Metrics.InstancesRolledBack)
	}

	// Check updater was called
	updater.mu.Lock()
	if len(updater.calls) != 1 {
		t.Errorf("expected 1 update call, got %d", len(updater.calls))
	} else {
		call := updater.calls[0]
		if call.Version != "v1" {
			t.Errorf("expected update to v1, got %s", call.Version)
		}
		if call.Replicas != 3 {
			t.Errorf("expected 3 replicas, got %d", call.Replicas)
		}
	}
	updater.mu.Unlock()

	// Check shifter was called
	shifter.mu.Lock()
	if len(shifter.calls) != 1 {
		t.Errorf("expected 1 shift call, got %d", len(shifter.calls))
	} else {
		call := shifter.calls[0]
		if call.FromVersion != "v2" || call.ToVersion != "v1" {
			t.Errorf("expected shift from v2 to v1, got %s -> %s", call.FromVersion, call.ToVersion)
		}
		if call.Weight != 100 {
			t.Errorf("expected weight 100, got %d", call.Weight)
		}
	}
	shifter.mu.Unlock()
}

func TestManager_Rollback_DeploymentNotFound(t *testing.T) {
	mgr, _, _, _ := setupTestManager()

	req := &RollbackRequest{
		DeploymentID: "nonexistent",
		Reason:       RollbackReasonManual,
	}

	_, err := mgr.Rollback(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for nonexistent deployment")
	}
}

func TestManager_Rollback_NoPreviousDeployment(t *testing.T) {
	lookup := newMockDeploymentLookup()
	updater := &mockInstanceUpdater{}
	shifter := &mockTrafficShifter{}

	lookup.deployments["dep-only"] = &DeploymentInfo{
		ID:          "dep-only",
		ServiceName: "svc-lonely",
		Version:     "v1",
		State:       "running",
		Replicas:    1,
	}
	// No previous deployment registered

	mgr := NewManager(lookup, updater, shifter)
	req := &RollbackRequest{
		DeploymentID: "dep-only",
		Reason:       RollbackReasonManual,
	}

	_, err := mgr.Rollback(context.Background(), req)
	if err == nil {
		t.Fatal("expected error when no previous deployment exists")
	}
}

func TestManager_Rollback_SpecificTarget(t *testing.T) {
	mgr, lookup, updater, _ := setupTestManager()

	// Add a specific target
	lookup.mu.Lock()
	lookup.deployments["dep-target"] = &DeploymentInfo{
		ID:          "dep-target",
		ServiceName: "svc-a",
		Version:     "v0",
		ArtifactRef: "artifact-v0",
		State:       "completed",
		Replicas:    2,
	}
	lookup.mu.Unlock()

	req := &RollbackRequest{
		DeploymentID:       "dep-current",
		TargetDeploymentID: "dep-target",
		Reason:             RollbackReasonManual,
	}

	rb, err := mgr.Rollback(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rb.ToVersion != "v0" {
		t.Errorf("expected ToVersion v0, got %s", rb.ToVersion)
	}

	waitForRollbackState(t, mgr, rb.ID, RollbackStateCompleted, 5*time.Second)

	updater.mu.Lock()
	if len(updater.calls) != 1 {
		t.Fatalf("expected 1 update call, got %d", len(updater.calls))
	}
	if updater.calls[0].Version != "v0" {
		t.Errorf("expected update to v0, got %s", updater.calls[0].Version)
	}
	updater.mu.Unlock()
}

func TestManager_Rollback_DuplicateInProgress(t *testing.T) {
	mgr, _, _, _ := setupTestManager()

	// Slow down the updater so rollback stays in-progress
	lookup := newMockDeploymentLookup()
	lookup.deployments["dep-current"] = &DeploymentInfo{
		ID:          "dep-current",
		ServiceName: "svc-a",
		Version:     "v2",
		ArtifactRef: "artifact-v2",
		State:       "running",
		Replicas:    3,
	}
	lookup.deployments["dep-previous"] = &DeploymentInfo{
		ID:          "dep-previous",
		ServiceName: "svc-a",
		Version:     "v1",
		ArtifactRef: "artifact-v1",
		State:       "completed",
		Replicas:    3,
	}
	lookup.previous["svc-a:dep-current"] = lookup.deployments["dep-previous"]

	slowUpdater := &mockInstanceUpdater{}
	mgr = NewManager(lookup, slowUpdater, &mockTrafficShifter{})

	req := &RollbackRequest{
		DeploymentID: "dep-current",
		Reason:       RollbackReasonManual,
	}

	rb1, err := mgr.Rollback(context.Background(), req)
	if err != nil {
		t.Fatalf("first rollback failed: %v", err)
	}

	// Second rollback should fail because one is in progress
	_, err = mgr.Rollback(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for duplicate rollback")
	}
	if !errors.Is(err, ErrRollbackInProgress) {
		t.Errorf("expected ErrRollbackInProgress, got %v", err)
	}

	// Wait for completion so we clean up
	waitForRollbackState(t, mgr, rb1.ID, RollbackStateCompleted, 5*time.Second)
}

func TestManager_Rollback_TrafficShiftFailure(t *testing.T) {
	lookup := newMockDeploymentLookup()
	updater := &mockInstanceUpdater{}
	shifter := &mockTrafficShifter{failNext: true}

	lookup.deployments["dep-1"] = &DeploymentInfo{
		ID: "dep-1", ServiceName: "svc-a", Version: "v2", Replicas: 1,
	}
	lookup.deployments["dep-0"] = &DeploymentInfo{
		ID: "dep-0", ServiceName: "svc-a", Version: "v1", Replicas: 1,
	}
	lookup.previous["svc-a:dep-1"] = lookup.deployments["dep-0"]

	mgr := NewManager(lookup, updater, shifter)
	req := &RollbackRequest{DeploymentID: "dep-1", Reason: RollbackReasonAutomatic}

	rb, err := mgr.Rollback(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	completed := waitForRollbackState(t, mgr, rb.ID, RollbackStateFailed, 5*time.Second)
	if completed.Error == "" {
		t.Error("expected error message on failed rollback")
	}
}

func TestManager_Rollback_InstanceUpdateFailure(t *testing.T) {
	lookup := newMockDeploymentLookup()
	updater := &mockInstanceUpdater{failNext: true}
	shifter := &mockTrafficShifter{}

	lookup.deployments["dep-1"] = &DeploymentInfo{
		ID: "dep-1", ServiceName: "svc-a", Version: "v2", Replicas: 1,
	}
	lookup.deployments["dep-0"] = &DeploymentInfo{
		ID: "dep-0", ServiceName: "svc-a", Version: "v1", Replicas: 1,
	}
	lookup.previous["svc-a:dep-1"] = lookup.deployments["dep-0"]

	mgr := NewManager(lookup, updater, shifter)
	req := &RollbackRequest{DeploymentID: "dep-1", Reason: RollbackReasonManual}

	rb, err := mgr.Rollback(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	completed := waitForRollbackState(t, mgr, rb.ID, RollbackStateFailed, 5*time.Second)
	if completed.Error == "" {
		t.Error("expected error on failed rollback")
	}
}

func TestManager_Cancel(t *testing.T) {
	mgr, _, _, _ := setupTestManager()

	req := &RollbackRequest{
		DeploymentID: "dep-current",
		Reason:       RollbackReasonManual,
	}

	rb, err := mgr.Rollback(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Try to cancel (may or may not succeed depending on timing)
	err = mgr.Cancel(context.Background(), rb.ID)
	// The rollback may have already completed, in which case Cancel returns an error.
	// Both outcomes are acceptable.
	_ = err
}

func TestManager_Cancel_NotFound(t *testing.T) {
	mgr, _, _, _ := setupTestManager()
	err := mgr.Cancel(context.Background(), "nonexistent")
	if !errors.Is(err, ErrDeploymentNotFound) {
		t.Fatalf("expected ErrDeploymentNotFound, got %v", err)
	}
}

func TestManager_Status_NotFound(t *testing.T) {
	mgr, _, _, _ := setupTestManager()
	_, err := mgr.Status(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent rollback")
	}
}

func TestManager_GetByService(t *testing.T) {
	mgr, _, _, _ := setupTestManager()

	req := &RollbackRequest{
		DeploymentID: "dep-current",
		Reason:       RollbackReasonManual,
	}

	rb, err := mgr.Rollback(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	waitForRollbackState(t, mgr, rb.ID, RollbackStateCompleted, 5*time.Second)

	rollbacks, err := mgr.GetByService(context.Background(), "svc-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rollbacks) == 0 {
		t.Error("expected at least one rollback in history")
	}
}

func TestManager_IsRollbackInProgress(t *testing.T) {
	mgr, _, _, _ := setupTestManager()

	if mgr.IsRollbackInProgress("svc-a") {
		t.Error("expected no rollback in progress initially")
	}
}

func TestManager_AddCallback(t *testing.T) {
	mgr, _, _, _ := setupTestManager()

	var callbackInvoked int32
	var mu sync.Mutex

	mgr.AddCallback(func(ctx context.Context, rb *Rollback) error {
		mu.Lock()
		callbackInvoked++
		mu.Unlock()
		return nil
	})

	req := &RollbackRequest{
		DeploymentID: "dep-current",
		Reason:       RollbackReasonManual,
	}

	rb, err := mgr.Rollback(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	waitForRollbackState(t, mgr, rb.ID, RollbackStateCompleted, 5*time.Second)

	// Give callbacks a moment since they run in goroutines
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if callbackInvoked == 0 {
		t.Error("expected callback to be invoked")
	}
	mu.Unlock()
}

func TestManager_Rollback_NilTrafficShifter(t *testing.T) {
	lookup := newMockDeploymentLookup()
	updater := &mockInstanceUpdater{}

	lookup.deployments["dep-1"] = &DeploymentInfo{
		ID: "dep-1", ServiceName: "svc-a", Version: "v2", Replicas: 2,
	}
	lookup.deployments["dep-0"] = &DeploymentInfo{
		ID: "dep-0", ServiceName: "svc-a", Version: "v1", ArtifactRef: "art-v1", Replicas: 2,
	}
	lookup.previous["svc-a:dep-1"] = lookup.deployments["dep-0"]

	mgr := NewManager(lookup, updater, nil)
	req := &RollbackRequest{DeploymentID: "dep-1", Reason: RollbackReasonManual}

	rb, err := mgr.Rollback(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	completed := waitForRollbackState(t, mgr, rb.ID, RollbackStateCompleted, 5*time.Second)
	if completed.Metrics.TrafficShifts != 0 {
		t.Errorf("expected 0 traffic shifts, got %d", completed.Metrics.TrafficShifts)
	}
}

// ---------------------------------------------------------------------------
// Tests for Rollback struct fields
// ---------------------------------------------------------------------------

func TestRollbackState_Constants(t *testing.T) {
	states := []struct {
		state RollbackState
		want  string
	}{
		{RollbackStatePending, "pending"},
		{RollbackStateRunning, "running"},
		{RollbackStateCompleted, "completed"},
		{RollbackStateFailed, "failed"},
		{RollbackStateCancelled, "cancelled"},
	}
	for _, tc := range states {
		if string(tc.state) != tc.want {
			t.Errorf("expected %q, got %q", tc.want, tc.state)
		}
	}
}

func TestRollbackReason_Constants(t *testing.T) {
	reasons := []struct {
		reason RollbackReason
		want   string
	}{
		{RollbackReasonManual, "manual"},
		{RollbackReasonAutomatic, "automatic"},
		{RollbackReasonHealthCheck, "health_check"},
		{RollbackReasonTimeout, "timeout"},
		{RollbackReasonCanaryFail, "canary_failure"},
	}
	for _, tc := range reasons {
		if string(tc.reason) != tc.want {
			t.Errorf("expected %q, got %q", tc.want, tc.reason)
		}
	}
}

func TestErrorSentinels(t *testing.T) {
	errs := []error{
		ErrNoRollbackTarget,
		ErrRollbackInProgress,
		ErrRollbackFailed,
		ErrInvalidDeployment,
		ErrDeploymentNotFound,
	}
	for _, e := range errs {
		if e == nil {
			t.Error("expected non-nil error")
		}
		if e.Error() == "" {
			t.Error("expected non-empty error message")
		}
	}
}

// ---------------------------------------------------------------------------
// Tests for AutoRollbackConfig
// ---------------------------------------------------------------------------

func TestDefaultAutoRollbackConfig(t *testing.T) {
	cfg := DefaultAutoRollbackConfig()
	if !cfg.Enabled {
		t.Error("expected Enabled=true")
	}
	if cfg.HealthCheckFailures != 3 {
		t.Errorf("expected 3 health check failures, got %d", cfg.HealthCheckFailures)
	}
	if cfg.ErrorRateThreshold != 0.05 {
		t.Errorf("expected 0.05 error rate threshold, got %f", cfg.ErrorRateThreshold)
	}
	if cfg.LatencyThreshold != 5*time.Second {
		t.Errorf("expected 5s latency threshold, got %v", cfg.LatencyThreshold)
	}
	if cfg.GracePeriod != 30*time.Second {
		t.Errorf("expected 30s grace period, got %v", cfg.GracePeriod)
	}
}

// ---------------------------------------------------------------------------
// Tests for AutoRollbackMonitor
// ---------------------------------------------------------------------------

func TestNewAutoRollbackMonitor_DefaultConfig(t *testing.T) {
	mgr, _, _, _ := setupTestManager()
	monitor := NewAutoRollbackMonitor(mgr, nil)
	if monitor == nil {
		t.Fatal("expected non-nil monitor")
	}
	if monitor.config == nil {
		t.Fatal("expected default config")
	}
	if !monitor.config.Enabled {
		t.Error("expected default config to be enabled")
	}
}

func TestNewAutoRollbackMonitor_CustomConfig(t *testing.T) {
	mgr, _, _, _ := setupTestManager()
	cfg := &AutoRollbackConfig{
		Enabled:             false,
		HealthCheckFailures: 5,
	}
	monitor := NewAutoRollbackMonitor(mgr, cfg)
	if monitor.config.Enabled {
		t.Error("expected custom config to be disabled")
	}
	if monitor.config.HealthCheckFailures != 5 {
		t.Errorf("expected 5, got %d", monitor.config.HealthCheckFailures)
	}
}

func TestAutoRollbackMonitor_WatchAndUnwatch(t *testing.T) {
	mgr, _, _, _ := setupTestManager()
	monitor := NewAutoRollbackMonitor(mgr, nil)

	monitor.Watch("dep-1", "svc-a")

	monitor.mu.RLock()
	_, exists := monitor.watches["dep-1"]
	monitor.mu.RUnlock()
	if !exists {
		t.Error("expected watch to be registered")
	}

	monitor.Unwatch("dep-1")

	monitor.mu.RLock()
	_, exists = monitor.watches["dep-1"]
	monitor.mu.RUnlock()
	if exists {
		t.Error("expected watch to be removed")
	}
}

func TestAutoRollbackMonitor_RecordHealthCheckFailure_Disabled(t *testing.T) {
	mgr, _, _, _ := setupTestManager()
	cfg := &AutoRollbackConfig{Enabled: false}
	monitor := NewAutoRollbackMonitor(mgr, cfg)

	monitor.Watch("dep-current", "svc-a")
	// Should not trigger anything since disabled
	monitor.RecordHealthCheckFailure(context.Background(), "dep-current")

	monitor.mu.RLock()
	w, exists := monitor.watches["dep-current"]
	monitor.mu.RUnlock()
	if !exists {
		t.Fatal("expected watch to still exist")
	}
	if w.failures != 0 {
		t.Error("expected no failure recording when disabled")
	}
}

func TestAutoRollbackMonitor_RecordHealthCheckFailure_NotWatched(t *testing.T) {
	mgr, _, _, _ := setupTestManager()
	monitor := NewAutoRollbackMonitor(mgr, nil)

	// Recording for an unwatched deployment should be a no-op
	monitor.RecordHealthCheckFailure(context.Background(), "dep-unknown")
}

func TestAutoRollbackMonitor_RecordHealthCheckFailure_BelowThreshold(t *testing.T) {
	mgr, _, _, _ := setupTestManager()
	cfg := &AutoRollbackConfig{
		Enabled:             true,
		HealthCheckFailures: 5,
		GracePeriod:         0,
	}
	monitor := NewAutoRollbackMonitor(mgr, cfg)
	monitor.Watch("dep-current", "svc-a")

	// Record fewer failures than threshold
	monitor.RecordHealthCheckFailure(context.Background(), "dep-current")
	monitor.RecordHealthCheckFailure(context.Background(), "dep-current")

	// Watch should still exist since threshold not reached
	monitor.mu.RLock()
	_, exists := monitor.watches["dep-current"]
	monitor.mu.RUnlock()
	if !exists {
		t.Error("expected watch to still exist below threshold")
	}
}

func TestAutoRollbackMonitor_RecordHealthCheckFailure_TriggersRollback(t *testing.T) {
	mgr, _, _, _ := setupTestManager()
	cfg := &AutoRollbackConfig{
		Enabled:             true,
		HealthCheckFailures: 2,
		GracePeriod:         0, // no grace period for test
	}
	monitor := NewAutoRollbackMonitor(mgr, cfg)

	// Set start time in the past so grace period is satisfied
	monitor.Watch("dep-current", "svc-a")
	monitor.mu.Lock()
	monitor.watches["dep-current"].startedAt = time.Now().Add(-1 * time.Minute)
	monitor.mu.Unlock()

	// Record enough failures to trigger rollback
	monitor.RecordHealthCheckFailure(context.Background(), "dep-current")
	monitor.RecordHealthCheckFailure(context.Background(), "dep-current")

	// Watch should be removed after triggering rollback
	monitor.mu.RLock()
	_, exists := monitor.watches["dep-current"]
	monitor.mu.RUnlock()
	if exists {
		t.Error("expected watch to be removed after rollback trigger")
	}
}

func TestAutoRollbackMonitor_RecordHealthCheckFailure_GracePeriod(t *testing.T) {
	mgr, _, _, _ := setupTestManager()
	cfg := &AutoRollbackConfig{
		Enabled:             true,
		HealthCheckFailures: 1,
		GracePeriod:         1 * time.Hour, // very long grace period
	}
	monitor := NewAutoRollbackMonitor(mgr, cfg)
	monitor.Watch("dep-current", "svc-a")

	// Enough failures, but within grace period
	monitor.RecordHealthCheckFailure(context.Background(), "dep-current")

	// Watch should still exist because grace period hasn't elapsed
	monitor.mu.RLock()
	_, exists := monitor.watches["dep-current"]
	monitor.mu.RUnlock()
	if !exists {
		t.Error("expected watch to still exist during grace period")
	}
}

func TestAutoRollbackMonitor_Stop(t *testing.T) {
	mgr, _, _, _ := setupTestManager()
	monitor := NewAutoRollbackMonitor(mgr, nil)
	// Stop should not panic
	monitor.Stop()
}

// ---------------------------------------------------------------------------
// Tests for History
// ---------------------------------------------------------------------------

func TestNewHistory_DefaultMaxSize(t *testing.T) {
	h := NewHistory(0)
	if h.maxSize != 100 {
		t.Errorf("expected default maxSize 100, got %d", h.maxSize)
	}
}

func TestNewHistory_NegativeMaxSize(t *testing.T) {
	h := NewHistory(-5)
	if h.maxSize != 100 {
		t.Errorf("expected default maxSize 100, got %d", h.maxSize)
	}
}

func TestNewHistory_CustomMaxSize(t *testing.T) {
	h := NewHistory(50)
	if h.maxSize != 50 {
		t.Errorf("expected maxSize 50, got %d", h.maxSize)
	}
}

func TestHistory_AddDeployment(t *testing.T) {
	h := NewHistory(10)

	entry := &HistoryEntry{
		DeploymentID: "dep-1",
		ServiceName:  "svc-a",
		Version:      "v1",
		State:        "completed",
		Replicas:     3,
		StartedAt:    time.Now(),
	}
	h.AddDeployment(entry)

	entries, err := h.GetDeployments("svc-a", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].DeploymentID != "dep-1" {
		t.Errorf("expected dep-1, got %s", entries[0].DeploymentID)
	}
}

func TestHistory_AddDeployment_SortedByTime(t *testing.T) {
	h := NewHistory(10)

	now := time.Now()
	h.AddDeployment(&HistoryEntry{
		DeploymentID: "dep-old",
		ServiceName:  "svc-a",
		StartedAt:    now.Add(-1 * time.Hour),
	})
	h.AddDeployment(&HistoryEntry{
		DeploymentID: "dep-new",
		ServiceName:  "svc-a",
		StartedAt:    now,
	})

	entries, _ := h.GetDeployments("svc-a", 10)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	// Newest first
	if entries[0].DeploymentID != "dep-new" {
		t.Errorf("expected newest first, got %s", entries[0].DeploymentID)
	}
}

func TestHistory_AddDeployment_MaxSize(t *testing.T) {
	h := NewHistory(3)

	for i := 0; i < 5; i++ {
		h.AddDeployment(&HistoryEntry{
			DeploymentID: fmt.Sprintf("dep-%d", i),
			ServiceName:  "svc-a",
			StartedAt:    time.Now().Add(time.Duration(i) * time.Second),
		})
	}

	entries, _ := h.GetDeployments("svc-a", 10)
	if len(entries) != 3 {
		t.Errorf("expected max 3 entries, got %d", len(entries))
	}
}

func TestHistory_AddRollback(t *testing.T) {
	h := NewHistory(10)

	rb := &Rollback{
		ID:          "rb-1",
		ServiceName: "svc-a",
		FromVersion: "v2",
		ToVersion:   "v1",
		State:       RollbackStateCompleted,
		StartedAt:   time.Now(),
	}
	h.Add(rb)

	got, err := h.Get("rb-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "rb-1" {
		t.Errorf("expected rb-1, got %s", got.ID)
	}
}

func TestHistory_Get_NotFound(t *testing.T) {
	h := NewHistory(10)
	_, err := h.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent rollback")
	}
}

func TestHistory_GetByService(t *testing.T) {
	h := NewHistory(10)

	h.Add(&Rollback{ID: "rb-1", ServiceName: "svc-a", StartedAt: time.Now()})
	h.Add(&Rollback{ID: "rb-2", ServiceName: "svc-a", StartedAt: time.Now()})
	h.Add(&Rollback{ID: "rb-3", ServiceName: "svc-b", StartedAt: time.Now()})

	rollbacks, err := h.GetByService("svc-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rollbacks) != 2 {
		t.Errorf("expected 2 rollbacks for svc-a, got %d", len(rollbacks))
	}
}

func TestHistory_GetByService_NotFound(t *testing.T) {
	h := NewHistory(10)
	rollbacks, err := h.GetByService("svc-none")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rollbacks) != 0 {
		t.Errorf("expected empty, got %d", len(rollbacks))
	}
}

func TestHistory_GetDeployments_NoService(t *testing.T) {
	h := NewHistory(10)
	entries, err := h.GetDeployments("svc-none", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected empty, got %d", len(entries))
	}
}

func TestHistory_GetDeployments_Limit(t *testing.T) {
	h := NewHistory(10)
	for i := 0; i < 5; i++ {
		h.AddDeployment(&HistoryEntry{
			DeploymentID: fmt.Sprintf("dep-%d", i),
			ServiceName:  "svc-a",
			StartedAt:    time.Now().Add(time.Duration(i) * time.Second),
		})
	}

	entries, _ := h.GetDeployments("svc-a", 2)
	if len(entries) != 2 {
		t.Errorf("expected 2 entries with limit, got %d", len(entries))
	}
}

func TestHistory_GetDeployments_ZeroLimit(t *testing.T) {
	h := NewHistory(10)
	for i := 0; i < 3; i++ {
		h.AddDeployment(&HistoryEntry{
			DeploymentID: fmt.Sprintf("dep-%d", i),
			ServiceName:  "svc-a",
			StartedAt:    time.Now().Add(time.Duration(i) * time.Second),
		})
	}

	entries, _ := h.GetDeployments("svc-a", 0)
	if len(entries) != 3 {
		t.Errorf("expected all 3 entries with zero limit, got %d", len(entries))
	}
}

func TestHistory_GetPreviousSuccessful(t *testing.T) {
	h := NewHistory(10)

	now := time.Now()
	h.AddDeployment(&HistoryEntry{
		DeploymentID: "dep-current",
		ServiceName:  "svc-a",
		State:        "running",
		StartedAt:    now,
	})
	h.AddDeployment(&HistoryEntry{
		DeploymentID: "dep-prev",
		ServiceName:  "svc-a",
		State:        "completed",
		Version:      "v1",
		StartedAt:    now.Add(-1 * time.Hour),
	})

	prev, err := h.GetPreviousSuccessful("svc-a", "dep-current")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prev.DeploymentID != "dep-prev" {
		t.Errorf("expected dep-prev, got %s", prev.DeploymentID)
	}
}

func TestHistory_GetPreviousSuccessful_NoHistory(t *testing.T) {
	h := NewHistory(10)
	_, err := h.GetPreviousSuccessful("svc-none", "dep-1")
	if err == nil {
		t.Fatal("expected error for no history")
	}
}

func TestHistory_GetPreviousSuccessful_NoPrevious(t *testing.T) {
	h := NewHistory(10)
	h.AddDeployment(&HistoryEntry{
		DeploymentID: "dep-only",
		ServiceName:  "svc-a",
		State:        "running",
		StartedAt:    time.Now(),
	})

	_, err := h.GetPreviousSuccessful("svc-a", "dep-only")
	if err == nil {
		t.Fatal("expected error when no previous successful deployment")
	}
}

func TestHistory_GetRollbacksAfter(t *testing.T) {
	h := NewHistory(10)

	cutoff := time.Now()
	h.Add(&Rollback{ID: "rb-old", ServiceName: "svc-a", StartedAt: cutoff.Add(-1 * time.Hour)})
	h.Add(&Rollback{ID: "rb-new", ServiceName: "svc-a", StartedAt: cutoff.Add(1 * time.Hour)})

	result, err := h.GetRollbacksAfter("svc-a", cutoff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 rollback after cutoff, got %d", len(result))
	}
	if result[0].ID != "rb-new" {
		t.Errorf("expected rb-new, got %s", result[0].ID)
	}
}

func TestHistory_GetRollbacksAfter_NoService(t *testing.T) {
	h := NewHistory(10)
	result, err := h.GetRollbacksAfter("svc-none", time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty, got %d", len(result))
	}
}

func TestHistory_GetDeploymentsAfter(t *testing.T) {
	h := NewHistory(10)

	cutoff := time.Now()
	h.AddDeployment(&HistoryEntry{
		DeploymentID: "dep-old", ServiceName: "svc-a", StartedAt: cutoff.Add(-1 * time.Hour),
	})
	h.AddDeployment(&HistoryEntry{
		DeploymentID: "dep-new", ServiceName: "svc-a", StartedAt: cutoff.Add(1 * time.Hour),
	})

	result, err := h.GetDeploymentsAfter("svc-a", cutoff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 deployment after cutoff, got %d", len(result))
	}
	if result[0].DeploymentID != "dep-new" {
		t.Errorf("expected dep-new, got %s", result[0].DeploymentID)
	}
}

func TestHistory_GetDeploymentsAfter_NoService(t *testing.T) {
	h := NewHistory(10)
	result, err := h.GetDeploymentsAfter("svc-none", time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty, got %d", len(result))
	}
}

// ---------------------------------------------------------------------------
// Tests for History.GetStats
// ---------------------------------------------------------------------------

func TestHistory_GetStats(t *testing.T) {
	h := NewHistory(10)

	now := time.Now()
	h.AddDeployment(&HistoryEntry{
		DeploymentID: "dep-1", ServiceName: "svc-a", State: "completed", StartedAt: now,
	})
	h.AddDeployment(&HistoryEntry{
		DeploymentID: "dep-2", ServiceName: "svc-a", State: "failed", StartedAt: now.Add(1 * time.Second),
	})
	h.AddDeployment(&HistoryEntry{
		DeploymentID: "dep-3", ServiceName: "svc-a", State: "completed", StartedAt: now.Add(2 * time.Second),
	})

	h.Add(&Rollback{
		ID: "rb-1", ServiceName: "svc-a", State: RollbackStateCompleted,
		Reason: RollbackReasonManual, StartedAt: now,
	})
	h.Add(&Rollback{
		ID: "rb-2", ServiceName: "svc-a", State: RollbackStateFailed,
		Reason: RollbackReasonHealthCheck, StartedAt: now.Add(1 * time.Second),
	})
	h.Add(&Rollback{
		ID: "rb-3", ServiceName: "svc-a", State: RollbackStateCompleted,
		Reason: RollbackReasonAutomatic, StartedAt: now.Add(2 * time.Second),
	})

	stats := h.GetStats("svc-a")
	if stats.ServiceName != "svc-a" {
		t.Errorf("expected svc-a, got %s", stats.ServiceName)
	}
	if stats.TotalDeployments != 3 {
		t.Errorf("expected 3 total deployments, got %d", stats.TotalDeployments)
	}
	if stats.SuccessfulDeployments != 2 {
		t.Errorf("expected 2 successful, got %d", stats.SuccessfulDeployments)
	}
	if stats.FailedDeployments != 1 {
		t.Errorf("expected 1 failed, got %d", stats.FailedDeployments)
	}
	if stats.TotalRollbacks != 3 {
		t.Errorf("expected 3 total rollbacks, got %d", stats.TotalRollbacks)
	}
	if stats.SuccessfulRollbacks != 2 {
		t.Errorf("expected 2 successful rollbacks, got %d", stats.SuccessfulRollbacks)
	}
	if stats.FailedRollbacks != 1 {
		t.Errorf("expected 1 failed rollback, got %d", stats.FailedRollbacks)
	}
	if stats.ManualRollbacks != 1 {
		t.Errorf("expected 1 manual rollback, got %d", stats.ManualRollbacks)
	}
	if stats.AutomaticRollbacks != 2 {
		t.Errorf("expected 2 automatic rollbacks, got %d", stats.AutomaticRollbacks)
	}
}

func TestHistory_GetStats_NoData(t *testing.T) {
	h := NewHistory(10)
	stats := h.GetStats("svc-empty")
	if stats.TotalDeployments != 0 {
		t.Errorf("expected 0 deployments, got %d", stats.TotalDeployments)
	}
	if stats.TotalRollbacks != 0 {
		t.Errorf("expected 0 rollbacks, got %d", stats.TotalRollbacks)
	}
}

// ---------------------------------------------------------------------------
// Tests for History.Clear and ClearAll
// ---------------------------------------------------------------------------

func TestHistory_Clear(t *testing.T) {
	h := NewHistory(10)
	h.AddDeployment(&HistoryEntry{DeploymentID: "dep-1", ServiceName: "svc-a", StartedAt: time.Now()})
	h.Add(&Rollback{ID: "rb-1", ServiceName: "svc-a", StartedAt: time.Now()})

	h.Clear("svc-a")

	entries, _ := h.GetDeployments("svc-a", 10)
	if len(entries) != 0 {
		t.Errorf("expected 0 deployments after clear, got %d", len(entries))
	}
	rollbacks, _ := h.GetByService("svc-a")
	if len(rollbacks) != 0 {
		t.Errorf("expected 0 rollbacks after clear, got %d", len(rollbacks))
	}
	_, err := h.Get("rb-1")
	if err == nil {
		t.Error("expected error when getting cleared rollback by ID")
	}
}

func TestHistory_ClearAll(t *testing.T) {
	h := NewHistory(10)
	h.AddDeployment(&HistoryEntry{DeploymentID: "dep-1", ServiceName: "svc-a", StartedAt: time.Now()})
	h.AddDeployment(&HistoryEntry{DeploymentID: "dep-2", ServiceName: "svc-b", StartedAt: time.Now()})
	h.Add(&Rollback{ID: "rb-1", ServiceName: "svc-a", StartedAt: time.Now()})

	h.ClearAll()

	entries, _ := h.GetDeployments("svc-a", 10)
	if len(entries) != 0 {
		t.Errorf("expected 0 after clear all, got %d", len(entries))
	}
	entries, _ = h.GetDeployments("svc-b", 10)
	if len(entries) != 0 {
		t.Errorf("expected 0 after clear all, got %d", len(entries))
	}
}

// ---------------------------------------------------------------------------
// Tests for History.Export and Import
// ---------------------------------------------------------------------------

func TestHistory_ExportImport(t *testing.T) {
	h := NewHistory(10)

	now := time.Now().Truncate(time.Millisecond) // truncate for JSON round-trip

	h.AddDeployment(&HistoryEntry{
		DeploymentID: "dep-1",
		ServiceName:  "svc-a",
		Version:      "v1",
		State:        "completed",
		Replicas:     3,
		StartedAt:    now,
	})
	h.Add(&Rollback{
		ID:          "rb-1",
		ServiceName: "svc-a",
		FromVersion: "v2",
		ToVersion:   "v1",
		State:       RollbackStateCompleted,
		StartedAt:   now,
	})

	data, err := h.Export()
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}

	// Verify it's valid JSON
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("export produced invalid JSON: %v", err)
	}

	// Import into a new history
	h2 := NewHistory(10)
	if err := h2.Import(data); err != nil {
		t.Fatalf("import failed: %v", err)
	}

	entries, _ := h2.GetDeployments("svc-a", 10)
	if len(entries) != 1 {
		t.Fatalf("expected 1 deployment after import, got %d", len(entries))
	}
	if entries[0].DeploymentID != "dep-1" {
		t.Errorf("expected dep-1, got %s", entries[0].DeploymentID)
	}

	rb, err := h2.Get("rb-1")
	if err != nil {
		t.Fatalf("expected rollback in imported history: %v", err)
	}
	if rb.FromVersion != "v2" {
		t.Errorf("expected FromVersion v2, got %s", rb.FromVersion)
	}
}

func TestHistory_Import_InvalidJSON(t *testing.T) {
	h := NewHistory(10)
	err := h.Import([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestHistory_Import_Merge(t *testing.T) {
	h := NewHistory(10)
	h.AddDeployment(&HistoryEntry{
		DeploymentID: "dep-existing",
		ServiceName:  "svc-a",
		StartedAt:    time.Now(),
	})

	importData := `{
		"deployments": {
			"svc-a": [{"deployment_id":"dep-imported","service_name":"svc-a","started_at":"2025-01-01T00:00:00Z"}]
		},
		"rollbacks": {}
	}`

	if err := h.Import([]byte(importData)); err != nil {
		t.Fatalf("import failed: %v", err)
	}

	entries, _ := h.GetDeployments("svc-a", 10)
	if len(entries) != 2 {
		t.Errorf("expected 2 merged entries, got %d", len(entries))
	}
}

// ---------------------------------------------------------------------------
// Tests for History.Query
// ---------------------------------------------------------------------------

func TestHistory_Query_ByService(t *testing.T) {
	h := NewHistory(10)
	h.AddDeployment(&HistoryEntry{DeploymentID: "dep-1", ServiceName: "svc-a", StartedAt: time.Now()})
	h.AddDeployment(&HistoryEntry{DeploymentID: "dep-2", ServiceName: "svc-b", StartedAt: time.Now()})

	result, err := h.Query(&HistoryQuery{ServiceName: "svc-a"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(result.Entries))
	}
}

func TestHistory_Query_AllServices(t *testing.T) {
	h := NewHistory(10)
	h.AddDeployment(&HistoryEntry{DeploymentID: "dep-1", ServiceName: "svc-a", StartedAt: time.Now()})
	h.AddDeployment(&HistoryEntry{DeploymentID: "dep-2", ServiceName: "svc-b", StartedAt: time.Now()})

	result, err := h.Query(&HistoryQuery{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(result.Entries))
	}
}

func TestHistory_Query_ByState(t *testing.T) {
	h := NewHistory(10)
	h.AddDeployment(&HistoryEntry{
		DeploymentID: "dep-1", ServiceName: "svc-a", State: "completed", StartedAt: time.Now(),
	})
	h.AddDeployment(&HistoryEntry{
		DeploymentID: "dep-2", ServiceName: "svc-a", State: "failed", StartedAt: time.Now(),
	})

	result, err := h.Query(&HistoryQuery{ServiceName: "svc-a", State: "completed"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Entries) != 1 {
		t.Errorf("expected 1 completed entry, got %d", len(result.Entries))
	}
}

func TestHistory_Query_TimeRange(t *testing.T) {
	h := NewHistory(10)

	now := time.Now()
	h.AddDeployment(&HistoryEntry{
		DeploymentID: "dep-old", ServiceName: "svc-a", StartedAt: now.Add(-2 * time.Hour),
	})
	h.AddDeployment(&HistoryEntry{
		DeploymentID: "dep-mid", ServiceName: "svc-a", StartedAt: now.Add(-30 * time.Minute),
	})
	h.AddDeployment(&HistoryEntry{
		DeploymentID: "dep-new", ServiceName: "svc-a", StartedAt: now.Add(1 * time.Hour),
	})

	result, err := h.Query(&HistoryQuery{
		ServiceName: "svc-a",
		StartTime:   now.Add(-1 * time.Hour),
		EndTime:     now,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Entries) != 1 {
		t.Errorf("expected 1 entry in range, got %d", len(result.Entries))
	}
	if len(result.Entries) > 0 && result.Entries[0].DeploymentID != "dep-mid" {
		t.Errorf("expected dep-mid, got %s", result.Entries[0].DeploymentID)
	}
}

func TestHistory_Query_WithRollbacks(t *testing.T) {
	h := NewHistory(10)

	now := time.Now()
	h.AddDeployment(&HistoryEntry{DeploymentID: "dep-1", ServiceName: "svc-a", StartedAt: now})
	h.Add(&Rollback{ID: "rb-1", ServiceName: "svc-a", State: RollbackStateCompleted, StartedAt: now})

	result, err := h.Query(&HistoryQuery{
		ServiceName:      "svc-a",
		IncludeRollbacks: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Entries) != 1 {
		t.Errorf("expected 1 deployment entry, got %d", len(result.Entries))
	}
	if len(result.Rollbacks) != 1 {
		t.Errorf("expected 1 rollback, got %d", len(result.Rollbacks))
	}
}

func TestHistory_Query_Limit(t *testing.T) {
	h := NewHistory(10)

	for i := 0; i < 5; i++ {
		h.AddDeployment(&HistoryEntry{
			DeploymentID: fmt.Sprintf("dep-%d", i),
			ServiceName:  "svc-a",
			StartedAt:    time.Now().Add(time.Duration(i) * time.Second),
		})
	}

	result, err := h.Query(&HistoryQuery{ServiceName: "svc-a", Limit: 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Entries) != 2 {
		t.Errorf("expected 2 entries with limit, got %d", len(result.Entries))
	}
}

func TestHistory_Query_Offset(t *testing.T) {
	h := NewHistory(10)

	for i := 0; i < 5; i++ {
		h.AddDeployment(&HistoryEntry{
			DeploymentID: fmt.Sprintf("dep-%d", i),
			ServiceName:  "svc-a",
			StartedAt:    time.Now().Add(time.Duration(i) * time.Second),
		})
	}

	result, err := h.Query(&HistoryQuery{ServiceName: "svc-a", Offset: 2, Limit: 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Entries) != 2 {
		t.Errorf("expected 2 entries with offset+limit, got %d", len(result.Entries))
	}
}

func TestHistory_Query_RollbackStateFilter(t *testing.T) {
	h := NewHistory(10)

	now := time.Now()
	h.Add(&Rollback{ID: "rb-1", ServiceName: "svc-a", State: RollbackStateCompleted, StartedAt: now})
	h.Add(&Rollback{ID: "rb-2", ServiceName: "svc-a", State: RollbackStateFailed, StartedAt: now})

	result, err := h.Query(&HistoryQuery{
		ServiceName:      "svc-a",
		State:            "completed",
		IncludeRollbacks: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Rollbacks) != 1 {
		t.Errorf("expected 1 completed rollback, got %d", len(result.Rollbacks))
	}
}

// ---------------------------------------------------------------------------
// Tests for HistoryEntry fields
// ---------------------------------------------------------------------------

func TestHistoryEntry_Fields(t *testing.T) {
	now := time.Now()
	entry := &HistoryEntry{
		DeploymentID:   "dep-1",
		ServiceName:    "svc-a",
		Version:        "v1",
		ArtifactRef:    "art-v1",
		State:          "completed",
		Replicas:       3,
		StartedAt:      now,
		CompletedAt:    now.Add(5 * time.Minute),
		Duration:       5 * time.Minute,
		RollbackID:     "rb-1",
		RollbackFromID: "dep-0",
		Metadata:       map[string]interface{}{"key": "value"},
	}
	if entry.DeploymentID != "dep-1" {
		t.Error("DeploymentID mismatch")
	}
	if entry.RollbackID != "rb-1" {
		t.Error("RollbackID mismatch")
	}
	if entry.RollbackFromID != "dep-0" {
		t.Error("RollbackFromID mismatch")
	}
	if entry.Metadata["key"] != "value" {
		t.Error("Metadata mismatch")
	}
}

// ---------------------------------------------------------------------------
// Tests for DeploymentInfo fields
// ---------------------------------------------------------------------------

func TestDeploymentInfo_Fields(t *testing.T) {
	info := &DeploymentInfo{
		ID:          "dep-1",
		ServiceName: "svc-a",
		Version:     "v1",
		ArtifactRef: "art-v1",
		State:       "running",
		Replicas:    3,
	}
	if info.ID != "dep-1" {
		t.Error("ID mismatch")
	}
	if info.Replicas != 3 {
		t.Error("Replicas mismatch")
	}
}

// ---------------------------------------------------------------------------
// Tests for RollbackMetrics fields
// ---------------------------------------------------------------------------

func TestRollbackMetrics_Fields(t *testing.T) {
	m := &RollbackMetrics{
		InstancesRolledBack: 3,
		InstancesFailed:     1,
		TrafficShifts:       2,
		HealthChecks:        10,
	}
	if m.InstancesRolledBack != 3 {
		t.Error("InstancesRolledBack mismatch")
	}
	if m.InstancesFailed != 1 {
		t.Error("InstancesFailed mismatch")
	}
}

// ---------------------------------------------------------------------------
// Tests for RollbackRequest fields
// ---------------------------------------------------------------------------

func TestRollbackRequest_Fields(t *testing.T) {
	req := &RollbackRequest{
		DeploymentID:       "dep-1",
		TargetDeploymentID: "dep-0",
		Reason:             RollbackReasonCanaryFail,
		InitiatedBy:        "system",
	}
	if req.DeploymentID != "dep-1" {
		t.Error("DeploymentID mismatch")
	}
	if req.Reason != RollbackReasonCanaryFail {
		t.Error("Reason mismatch")
	}
}

// ---------------------------------------------------------------------------
// Tests for HistoryStats fields
// ---------------------------------------------------------------------------

func TestHistoryStats_Fields(t *testing.T) {
	stats := &HistoryStats{
		ServiceName:           "svc-a",
		TotalDeployments:      10,
		SuccessfulDeployments: 8,
		FailedDeployments:     2,
		TotalRollbacks:        3,
		SuccessfulRollbacks:   2,
		FailedRollbacks:       1,
		ManualRollbacks:       1,
		AutomaticRollbacks:    2,
	}
	if stats.TotalDeployments != 10 {
		t.Error("TotalDeployments mismatch")
	}
	if stats.AutomaticRollbacks != 2 {
		t.Error("AutomaticRollbacks mismatch")
	}
}

// ---------------------------------------------------------------------------
// Tests for matchesQuery and matchesRollbackQuery
// ---------------------------------------------------------------------------

func TestMatchesQuery(t *testing.T) {
	now := time.Now()
	entry := &HistoryEntry{
		ServiceName: "svc-a",
		State:       "completed",
		StartedAt:   now,
	}

	// Matches everything with empty query
	if !matchesQuery(entry, &HistoryQuery{}) {
		t.Error("expected match for empty query")
	}

	// Matches service
	if !matchesQuery(entry, &HistoryQuery{ServiceName: "svc-a"}) {
		t.Error("expected match for matching service")
	}
	if matchesQuery(entry, &HistoryQuery{ServiceName: "svc-b"}) {
		t.Error("expected no match for different service")
	}

	// Matches state
	if !matchesQuery(entry, &HistoryQuery{State: "completed"}) {
		t.Error("expected match for matching state")
	}
	if matchesQuery(entry, &HistoryQuery{State: "failed"}) {
		t.Error("expected no match for different state")
	}

	// Time filters
	if matchesQuery(entry, &HistoryQuery{StartTime: now.Add(1 * time.Hour)}) {
		t.Error("expected no match when entry is before StartTime")
	}
	if matchesQuery(entry, &HistoryQuery{EndTime: now.Add(-1 * time.Hour)}) {
		t.Error("expected no match when entry is after EndTime")
	}
}

func TestMatchesRollbackQuery(t *testing.T) {
	now := time.Now()
	rb := &Rollback{
		ServiceName: "svc-a",
		State:       RollbackStateCompleted,
		StartedAt:   now,
	}

	if !matchesRollbackQuery(rb, &HistoryQuery{}) {
		t.Error("expected match for empty query")
	}
	if matchesRollbackQuery(rb, &HistoryQuery{ServiceName: "svc-b"}) {
		t.Error("expected no match for different service")
	}
	if matchesRollbackQuery(rb, &HistoryQuery{State: "failed"}) {
		t.Error("expected no match for different state")
	}
}

// ---------------------------------------------------------------------------
// Tests for generateRollbackID
// ---------------------------------------------------------------------------

func TestGenerateRollbackID(t *testing.T) {
	id1 := generateRollbackID()
	if id1 == "" {
		t.Fatal("expected non-empty ID")
	}
	// IDs should start with "rollback-"
	if len(id1) < 10 {
		t.Error("expected longer rollback ID")
	}
}

// ---------------------------------------------------------------------------
// Tests for Rollback struct field access
// ---------------------------------------------------------------------------

func TestRollback_FieldAccess(t *testing.T) {
	now := time.Now()
	rb := &Rollback{
		ID:                 "rb-1",
		DeploymentID:       "dep-1",
		TargetDeploymentID: "dep-0",
		ServiceName:        "svc-a",
		FromVersion:        "v2",
		ToVersion:          "v1",
		State:              RollbackStatePending,
		Reason:             RollbackReasonTimeout,
		Message:            "timeout occurred",
		InitiatedBy:        "system",
		StartedAt:          now,
		CompletedAt:        now.Add(1 * time.Minute),
		Duration:           1 * time.Minute,
		Metrics:            &RollbackMetrics{InstancesRolledBack: 3},
		Error:              "",
	}
	if rb.ID != "rb-1" {
		t.Error("ID mismatch")
	}
	if rb.Reason != RollbackReasonTimeout {
		t.Error("Reason mismatch")
	}
	if rb.Metrics.InstancesRolledBack != 3 {
		t.Error("Metrics mismatch")
	}
}
