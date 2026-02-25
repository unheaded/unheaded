// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package prefetch

import (
	"testing"
)

func TestPrefetchHintCreation(t *testing.T) {
	hint := &PrefetchHint{
		FlowLabel: 0x12345678,
		BaseAddr:  0x80000000,
		PageCount: 4,
		ID:        1,
	}

	if hint.FlowLabel != 0x12345678 {
		t.Errorf("Expected flow label %d, got %d", 0x12345678, hint.FlowLabel)
	}
	if hint.BaseAddr != 0x80000000 {
		t.Errorf("Expected base addr %d, got %d", 0x80000000, hint.BaseAddr)
	}
	if hint.PageCount != 4 {
		t.Errorf("Expected page count 4, got %d", hint.PageCount)
	}
}

func TestPrefetchManagerAddHint(t *testing.T) {
	pm := NewPrefetchManager()

	hintID, err := pm.AddHint(0x12345678, 0x80000000, 4)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if hintID != 1 {
		t.Errorf("Expected hint ID 1, got %d", hintID)
	}

	if pm.OutstandingCount() != 1 {
		t.Errorf("Expected 1 outstanding hint, got %d", pm.OutstandingCount())
	}
}

func TestPrefetchManagerGetHint(t *testing.T) {
	pm := NewPrefetchManager()

	hintID, _ := pm.AddHint(0x12345678, 0x80000000, 4)

	hint, exists := pm.GetHint(hintID)
	if !exists {
		t.Errorf("Expected hint to exist")
	}

	if hint.FlowLabel != 0x12345678 {
		t.Errorf("Expected flow label %d, got %d", 0x12345678, hint.FlowLabel)
	}
}

func TestPrefetchManagerGetHintNotFound(t *testing.T) {
	pm := NewPrefetchManager()

	_, exists := pm.GetHint(999)
	if exists {
		t.Errorf("Expected hint to not exist")
	}
}

func TestPrefetchManagerCancelHint(t *testing.T) {
	pm := NewPrefetchManager()

	hintID, _ := pm.AddHint(0x12345678, 0x80000000, 4)

	if pm.OutstandingCount() != 1 {
		t.Errorf("Expected 1 outstanding hint before cancel")
	}

	err := pm.CancelHint(hintID)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if pm.OutstandingCount() != 0 {
		t.Errorf("Expected 0 outstanding hints after cancel")
	}

	_, exists := pm.GetHint(hintID)
	if exists {
		t.Errorf("Expected hint to not exist after cancel")
	}
}

func TestPrefetchManagerCancelHintNotFound(t *testing.T) {
	pm := NewPrefetchManager()

	err := pm.CancelHint(999)
	if err == nil {
		t.Errorf("Expected error for non-existent hint")
	}
}

func TestPrefetchManagerMaxOutstanding(t *testing.T) {
	pm := NewPrefetchManagerWithLimits(3, 2)

	// Add 3 hints (max global)
	for i := 0; i < 3; i++ {
		_, err := pm.AddHint(uint32(i), 0x80000000, 4)
		if err != nil {
			t.Errorf("Unexpected error at %d: %v", i, err)
		}
	}

	// Try to add 4th (should fail)
	_, err := pm.AddHint(99, 0x80000000, 4)
	if err == nil {
		t.Errorf("Expected error when exceeding max outstanding")
	}
}

func TestPrefetchManagerMaxPerFlow(t *testing.T) {
	pm := NewPrefetchManagerWithLimits(10, 2)

	flowLabel := uint32(0x12345678)

	// Add 2 hints for same flow (max per flow)
	for i := 0; i < 2; i++ {
		_, err := pm.AddHint(flowLabel, uint32(0x80000000+i*4096), 4)
		if err != nil {
			t.Errorf("Unexpected error at %d: %v", i, err)
		}
	}

	// Try to add 3rd hint for same flow (should fail)
	_, err := pm.AddHint(flowLabel, 0x80010000, 4)
	if err == nil {
		t.Errorf("Expected error when exceeding max per flow")
	}

	// But different flow should work
	_, err = pm.AddHint(0xABCDEF00, 0x80010000, 4)
	if err != nil {
		t.Errorf("Unexpected error for different flow: %v", err)
	}
}

func TestPrefetchManagerGetFlowHints(t *testing.T) {
	pm := NewPrefetchManager()

	flowLabel := uint32(0x12345678)

	// Add multiple hints for same flow
	hintIDs := make([]uint64, 3)
	for i := 0; i < 3; i++ {
		id, _ := pm.AddHint(flowLabel, uint32(0x80000000+i*4096), uint8(i+1))
		hintIDs[i] = id
	}

	// Get flow hints
	hints := pm.GetFlowHints(flowLabel)
	if len(hints) != 3 {
		t.Errorf("Expected 3 hints, got %d", len(hints))
	}

	// Verify all hints are present
	for i, hint := range hints {
		if hint.PageCount != uint8(i+1) {
			t.Errorf("Expected page count %d, got %d", i+1, hint.PageCount)
		}
	}
}

func TestPrefetchManagerGetFlowHintsNotFound(t *testing.T) {
	pm := NewPrefetchManager()

	hints := pm.GetFlowHints(0xDEADBEEF)
	if hints != nil && len(hints) != 0 {
		t.Errorf("Expected no hints for non-existent flow")
	}
}

func TestPrefetchManagerCancelFlowHints(t *testing.T) {
	pm := NewPrefetchManager()

	flowLabel := uint32(0x12345678)

	// Add multiple hints for same flow
	for i := 0; i < 3; i++ {
		pm.AddHint(flowLabel, uint32(0x80000000+i*4096), 4)
	}

	if pm.OutstandingCount() != 3 {
		t.Errorf("Expected 3 outstanding hints before cancel")
	}

	err := pm.CancelFlowHints(flowLabel)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if pm.OutstandingCount() != 0 {
		t.Errorf("Expected 0 outstanding hints after cancel")
	}

	hints := pm.GetFlowHints(flowLabel)
	if hints != nil && len(hints) != 0 {
		t.Errorf("Expected no hints after cancellation")
	}
}

func TestPrefetchManagerCancelFlowHintsNotFound(t *testing.T) {
	pm := NewPrefetchManager()

	// Should not error on non-existent flow
	err := pm.CancelFlowHints(0xDEADBEEF)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestPrefetchManagerOutstandingCount(t *testing.T) {
	pm := NewPrefetchManager()

	if pm.OutstandingCount() != 0 {
		t.Errorf("Expected 0 outstanding hints initially")
	}

	pm.AddHint(0x11111111, 0x80000000, 4)
	pm.AddHint(0x22222222, 0x80000000, 4)

	if pm.OutstandingCount() != 2 {
		t.Errorf("Expected 2 outstanding hints")
	}
}

func TestPrefetchManagerSetMaxOutstanding(t *testing.T) {
	pm := NewPrefetchManager()

	_ = pm.GetMaxOutstanding()
	newMax := uint16(32)

	pm.SetMaxOutstanding(newMax)

	if pm.GetMaxOutstanding() != newMax {
		t.Errorf("Expected max %d, got %d", newMax, pm.GetMaxOutstanding())
	}

	// Add hints up to new limit
	for i := 0; i < 32; i++ {
		_, err := pm.AddHint(uint32(i), 0x80000000, 4)
		if err != nil {
			t.Errorf("Unexpected error at %d: %v", i, err)
		}
	}

	// Next should fail
	_, err := pm.AddHint(32, 0x80000000, 4)
	if err == nil {
		t.Errorf("Expected error when exceeding new max")
	}
}

func TestPrefetchManagerMultipleFlows(t *testing.T) {
	pm := NewPrefetchManager()

	// Add hints to multiple flows
	flow1 := uint32(0x11111111)
	flow2 := uint32(0x22222222)
	flow3 := uint32(0x33333333)

	pm.AddHint(flow1, 0x80000000, 4)
	pm.AddHint(flow1, 0x80001000, 4)
	pm.AddHint(flow2, 0x80000000, 4)
	pm.AddHint(flow3, 0x80000000, 4)

	if pm.OutstandingCount() != 4 {
		t.Errorf("Expected 4 outstanding hints")
	}

	// Get hints per flow
	hints1 := pm.GetFlowHints(flow1)
	hints2 := pm.GetFlowHints(flow2)
	hints3 := pm.GetFlowHints(flow3)

	if len(hints1) != 2 {
		t.Errorf("Expected 2 hints for flow1, got %d", len(hints1))
	}
	if len(hints2) != 1 {
		t.Errorf("Expected 1 hint for flow2, got %d", len(hints2))
	}
	if len(hints3) != 1 {
		t.Errorf("Expected 1 hint for flow3, got %d", len(hints3))
	}

	// Cancel flow1
	pm.CancelFlowHints(flow1)

	if pm.OutstandingCount() != 2 {
		t.Errorf("Expected 2 outstanding hints after canceling flow1")
	}
}
