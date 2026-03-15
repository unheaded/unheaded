// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package dos

import (
	"os"
	"testing"
	"time"

	"unheaded/pkg/logger"
)

func TestNewDropRateTracker(t *testing.T) {
	tests := []struct {
		name        string
		windowSize  time.Duration
		expectError bool
	}{
		{
			name:        "valid window size",
			windowSize:  1 * time.Second,
			expectError: false,
		},
		{
			name:        "zero window size",
			windowSize:  0,
			expectError: true,
		},
		{
			name:        "negative window size",
			windowSize:  -1 * time.Second,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracker, err := NewDropRateTracker(tt.windowSize)

			if tt.expectError && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.expectError && tracker == nil {
				t.Errorf("expected tracker, got nil")
			}
		})
	}
}

func TestDropRateCalculation(t *testing.T) {
	tracker, _ := NewDropRateTracker(1 * time.Second)

	// Record 10 drops
	for i := 0; i < 10; i++ {
		tracker.RecordDrop()
	}

	// Get drop rate - should be approximately 10 drops/sec
	rate := tracker.GetDropRate()
	if rate < 9.0 || rate > 11.0 {
		t.Errorf("expected drop rate around 10 drops/sec, got %.2f", rate)
	}
}

func TestRingBufferMonitorCreation(t *testing.T) {
	log := logger.New(os.Stdout)

	tests := []struct {
		name        string
		log         *logger.Logger
		expectError bool
	}{
		{
			name:        "valid ring buffer monitor",
			log:         log,
			expectError: false,
		},
		{
			name:        "nil logger",
			log:         nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rbm, err := NewRingBufferMonitor(tt.log)

			if tt.expectError && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.expectError && rbm == nil {
				t.Errorf("expected monitor, got nil")
			}
		})
	}
}

func TestBackpressureThresholds(t *testing.T) {
	log := logger.New(os.Stdout)
	rbm, _ := NewRingBufferMonitor(log)

	// Record many drops to trigger backpressure
	for i := 0; i < 20; i++ {
		rbm.RecordDrop()
	}

	level := rbm.GetBackpressureLevel()
	if level != BackpressureHigh {
		t.Errorf("expected BackpressureHigh, got %d", level)
	}
}

func TestQoSAwarePriority(t *testing.T) {
	log := logger.New(os.Stdout)
	rbm, _ := NewRingBufferMonitor(log)

	// Trigger backpressure
	for i := 0; i < 20; i++ {
		rbm.RecordDrop()
	}

	// QoS 4-5 should be dropped under high backpressure
	dropped4 := rbm.DropWithQoSAwareness(4)
	dropped5 := rbm.DropWithQoSAwareness(5)

	if !dropped4 || !dropped5 {
		t.Errorf("expected QoS 4-5 to be dropped under backpressure")
	}

	// QoS 0-3 should not be dropped
	dropped0 := rbm.DropWithQoSAwareness(0)
	dropped3 := rbm.DropWithQoSAwareness(3)

	if dropped0 || dropped3 {
		t.Errorf("expected QoS 0-3 to not be dropped")
	}
}

func TestMapUsageTracker(t *testing.T) {
	tracker, _ := NewMapUsageTracker("test_map", 1000)

	tests := []struct {
		name     string
		usage    uint64
		shouldOK bool
	}{
		{
			name:     "50% usage",
			usage:    500,
			shouldOK: true,
		},
		{
			name:     "80% usage",
			usage:    800,
			shouldOK: true,
		},
		{
			name:     "90% usage",
			usage:    900,
			shouldOK: true,
		},
		{
			name:     "95% usage",
			usage:    950,
			shouldOK: true,
		},
		{
			name:     "exceed max",
			usage:    1001,
			shouldOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, err := tracker.UpdateUsage(tt.usage)

			if tt.shouldOK && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.shouldOK && err == nil {
				t.Errorf("expected error, got nil")
			}
		})
	}
}

func TestMapUsageAlerts(t *testing.T) {
	tracker, _ := NewMapUsageTracker("test_map", 1000)

	// Update to 80%
	alert80, alert90, alert95, _ := tracker.UpdateUsage(800)
	if !alert80 {
		t.Errorf("expected 80%% alert")
	}

	// Update to 90% (should alert for 90%)
	tracker2, _ := NewMapUsageTracker("test_map2", 1000)
	tracker2.UpdateUsage(800) // trigger 80% alert first
	alert80, alert90, alert95, _ = tracker2.UpdateUsage(900)
	if !alert90 {
		t.Errorf("expected 90%% alert")
	}

	// Update to 95% (should alert for 95%)
	tracker3, _ := NewMapUsageTracker("test_map3", 1000)
	tracker3.UpdateUsage(800)
	tracker3.UpdateUsage(900)
	alert80, alert90, alert95, _ = tracker3.UpdateUsage(950)
	if !alert95 {
		t.Errorf("expected 95%% alert")
	}
}

func TestBPFMapLimits(t *testing.T) {
	log := logger.New(os.Stdout)
	bml, _ := NewBPFMapLimits(log, 1000000)

	// Register a map
	if err := bml.RegisterMap("flow_table", 100000); err != nil {
		t.Errorf("failed to register map: %v", err)
	}

	// Try to register duplicate
	err := bml.RegisterMap("flow_table", 100000)
	if err == nil {
		t.Errorf("expected error for duplicate map registration")
	}

	// Update usage
	if err := bml.UpdateMapUsage("flow_table", 50000); err != nil {
		t.Errorf("failed to update map usage: %v", err)
	}

	// Update usage to trigger alert
	if err := bml.UpdateMapUsage("flow_table", 80000); err != nil {
		t.Errorf("failed to update map usage: %v", err)
	}
}

func TestMapEviction(t *testing.T) {
	log := logger.New(os.Stdout)
	bml, _ := NewBPFMapLimits(log, 1000000)
	bml.RegisterMap("test_map", 1000)

	// Set initial usage
	bml.UpdateMapUsage("test_map", 500)

	// Evict entries
	if err := bml.EvictLowestQoS("test_map", 100); err != nil {
		t.Errorf("failed to evict: %v", err)
	}

	// Verify usage decreased
	tracker := bml.mapTrackers["test_map"]
	if tracker.currentUsage != 400 {
		t.Errorf("expected usage 400 after eviction, got %d", tracker.currentUsage)
	}
}

func TestSizeLimitEnforcer(t *testing.T) {
	log := logger.New(os.Stdout)
	sle, _ := NewSizeLimitEnforcer(log, 1024*1024, 100*1024*1024)

	tests := []struct {
		name      string
		flowID    string
		size      uint64
		shouldOK  bool
	}{
		{
			name:     "valid allocation",
			flowID:   "flow1",
			size:     512 * 1024,
			shouldOK: true,
		},
		{
			name:     "exceed per-flow limit",
			flowID:   "flow1",
			size:     600 * 1024,
			shouldOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sle.CheckAndRecordSize(tt.flowID, tt.size)

			if tt.shouldOK && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.shouldOK && err == nil {
				t.Errorf("expected error, got nil")
			}
		})
	}
}

func TestSizeLimitGlobalLimit(t *testing.T) {
	log := logger.New(os.Stdout)
	// per-flow limit of 16MB so individual 15MB allocations fit within it
	sle, _ := NewSizeLimitEnforcer(log, 16*1024*1024, 20*1024*1024)

	// Add flow1 with 15MB
	sle.CheckAndRecordSize("flow1", 15*1024*1024)

	// Try to add flow2 with 10MB (would exceed global 20MB)
	err := sle.CheckAndRecordSize("flow2", 10*1024*1024)
	if err == nil {
		t.Errorf("expected error for exceeding global limit")
	}

	// Add flow2 with 5MB (should work)
	err = sle.CheckAndRecordSize("flow2", 5*1024*1024)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFlowUsageTracking(t *testing.T) {
	log := logger.New(os.Stdout)
	sle, _ := NewSizeLimitEnforcer(log, 1024*1024, 100*1024*1024)

	sle.CheckAndRecordSize("flow1", 512*1024)

	usage, _ := sle.GetFlowUsage("flow1")
	if usage != 512*1024 {
		t.Errorf("expected usage 512KB, got %d", usage)
	}

	globalUsage := sle.GetGlobalUsage()
	if globalUsage != 512*1024 {
		t.Errorf("expected global usage 512KB, got %d", globalUsage)
	}
}

func TestFlowUsageReset(t *testing.T) {
	log := logger.New(os.Stdout)
	sle, _ := NewSizeLimitEnforcer(log, 1024*1024, 100*1024*1024)

	sle.CheckAndRecordSize("flow1", 512*1024)
	sle.ResetFlowUsage("flow1")

	globalUsage := sle.GetGlobalUsage()
	if globalUsage != 0 {
		t.Errorf("expected global usage 0 after reset, got %d", globalUsage)
	}
}

func TestCompressionGuard(t *testing.T) {
	log := logger.New(os.Stdout)
	cg, _ := NewCompressionGuard(log)

	// Set compression flag for sensitive entry
	if err := cg.SetCompressionFlag("secret_key", CompressDisabled); err != nil {
		t.Errorf("failed to set compression flag: %v", err)
	}

	// Retrieve flag
	flag, _ := cg.GetCompressionFlag("secret_key")
	if flag != CompressDisabled {
		t.Errorf("expected CompressDisabled, got %d", flag)
	}

	// Unknown entry should default to enabled
	flag, _ = cg.GetCompressionFlag("unknown")
	if flag != CompressEnabled {
		t.Errorf("expected CompressEnabled for unknown entry, got %d", flag)
	}
}

func TestCompressionContexts(t *testing.T) {
	log := logger.New(os.Stdout)
	cg, _ := NewCompressionGuard(log)

	cg.SetTrustedContext(true)
	cg.SetUntrustedContext(false)

	// In test, we just verify no panics occur
	if cg.trustedContext != true {
		t.Errorf("expected trusted context true")
	}
	if cg.untrustedContext != false {
		t.Errorf("expected untrusted context false")
	}
}

func TestExtensionLimits(t *testing.T) {
	log := logger.New(os.Stdout)
	el, _ := NewExtensionLimits(log, 4)

	tests := []struct {
		name      string
		count     int
		shouldOK  bool
	}{
		{
			name:     "valid count",
			count:    2,
			shouldOK: true,
		},
		{
			name:     "max count",
			count:    4,
			shouldOK: true,
		},
		{
			name:     "exceed limit",
			count:    5,
			shouldOK: false,
		},
		{
			name:     "negative count",
			count:    -1,
			shouldOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := el.ValidateTLVCount(tt.count)

			if tt.shouldOK && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.shouldOK && err == nil {
				t.Errorf("expected error, got nil")
			}
		})
	}
}

func TestUnknownTLVProcessing(t *testing.T) {
	log := logger.New(os.Stdout)
	el, _ := NewExtensionLimits(log, 4)

	// Process unknown TLVs
	for i := 0; i < 10; i++ {
		if err := el.ProcessUnknownTLV(999 + i); err != nil {
			t.Errorf("failed to process unknown TLV: %v", err)
		}
	}

	count := el.GetUnknownSkipCount()
	if count != 10 {
		t.Errorf("expected 10 unknown TLV skips, got %d", count)
	}
}

func TestConcurrentMapUpdates(t *testing.T) {
	log := logger.New(os.Stdout)
	bml, _ := NewBPFMapLimits(log, 1000000)
	bml.RegisterMap("concurrent_map", 10000)

	done := make(chan bool, 10)

	// Concurrent updates
	for i := 0; i < 10; i++ {
		go func(idx int) {
			defer func() { done <- true }()
			for j := 0; j < 100; j++ {
				bml.UpdateMapUsage("concurrent_map", uint64((idx*100+j)%10000))
			}
		}(i)
	}

	// Wait for completion
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestConcurrentFlowTracking(t *testing.T) {
	log := logger.New(os.Stdout)
	sle, _ := NewSizeLimitEnforcer(log, 10*1024*1024, 100*1024*1024)

	done := make(chan bool, 10)

	// Concurrent flow updates
	for i := 0; i < 10; i++ {
		go func(idx int) {
			defer func() { done <- true }()
			flowID := "flow-" + string(rune('0'+idx))
			sle.CheckAndRecordSize(flowID, 512*1024)
		}(i)
	}

	// Wait for completion
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all flows recorded
	global := sle.GetGlobalUsage()
	expected := uint64(10 * 512 * 1024)
	if global != expected {
		t.Errorf("expected global usage %d, got %d", expected, global)
	}
}
