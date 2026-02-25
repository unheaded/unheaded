// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package timeline

import (
	"testing"
	"time"
)

// ============================================================================
// DATA MODEL TESTS (RED PHASE - these will fail until we implement)
// ============================================================================

func TestMilestone_Validate_HappyPath(t *testing.T) {
	m := &Milestone{
		ID:          "milestone-1",
		Name:        "eBPF Foundation",
		Status:      "in_progress",
		ETA:         time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC),
		Progress:    25,
		Owner:       "Agent 5",
		Risk:        "medium",
		Description: "Build eBPF programs for packet tracing",
		Tasks:       []string{"packet_marker", "flow_tracker", "latency_probe"},
	}

	err := m.Validate()
	if err != nil {
		t.Errorf("Validate() expected no error, got: %v", err)
	}
}

func TestMilestone_Validate_NilReceiver(t *testing.T) {
	var m *Milestone
	err := m.Validate()
	if err == nil {
		t.Error("Validate() expected error for nil receiver, got nil")
	}
}

func TestMilestone_Validate_EmptyID(t *testing.T) {
	m := &Milestone{
		ID:       "",
		Name:     "Test",
		Status:   "pending",
		Progress: 0,
	}
	err := m.Validate()
	if err == nil {
		t.Error("Validate() expected error for empty ID, got nil")
	}
}

func TestMilestone_Validate_EmptyName(t *testing.T) {
	m := &Milestone{
		ID:       "test-1",
		Name:     "",
		Status:   "pending",
		Progress: 0,
	}
	err := m.Validate()
	if err == nil {
		t.Error("Validate() expected error for empty name, got nil")
	}
}

func TestMilestone_Validate_InvalidStatus(t *testing.T) {
	tests := []struct {
		name   string
		status string
	}{
		{"empty status", ""},
		{"invalid status", "invalid"},
		{"uppercase", "PENDING"},
		{"typo", "complated"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Milestone{
				ID:       "test-1",
				Name:     "Test",
				Status:   tt.status,
				Progress: 0,
			}
			err := m.Validate()
			if err == nil {
				t.Errorf("Validate() expected error for status %q, got nil", tt.status)
			}
		})
	}
}

func TestMilestone_Validate_InvalidProgress(t *testing.T) {
	tests := []struct {
		name     string
		progress int
	}{
		{"negative", -1},
		{"over 100", 101},
		{"way over", 500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Milestone{
				ID:       "test-1",
				Name:     "Test",
				Status:   "pending",
				Progress: tt.progress,
			}
			err := m.Validate()
			if err == nil {
				t.Errorf("Validate() expected error for progress %d, got nil", tt.progress)
			}
		})
	}
}

func TestMilestone_Validate_InvalidRisk(t *testing.T) {
	tests := []struct {
		name string
		risk string
	}{
		{"invalid", "invalid"},
		{"uppercase", "HIGH"},
		{"empty is valid", ""}, // empty risk is allowed
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Milestone{
				ID:       "test-1",
				Name:     "Test",
				Status:   "pending",
				Progress: 0,
				Risk:     tt.risk,
			}
			err := m.Validate()
			// empty is allowed, others should error
			if tt.risk == "" && err != nil {
				t.Errorf("Validate() expected no error for empty risk, got: %v", err)
			} else if tt.risk != "" && err == nil {
				t.Errorf("Validate() expected error for risk %q, got nil", tt.risk)
			}
		})
	}
}

func TestPhase_Validate_HappyPath(t *testing.T) {
	p := &Phase{
		ID:         "phase-1",
		Name:       "Alpha",
		Status:     "in_progress",
		StartDate:  time.Date(2026, 1, 26, 0, 0, 0, 0, time.UTC),
		EndDate:    time.Date(2026, 2, 8, 0, 0, 0, 0, time.UTC),
		Progress:   25,
		Milestones: []string{"milestone-1", "milestone-2"},
	}

	err := p.Validate()
	if err != nil {
		t.Errorf("Validate() expected no error, got: %v", err)
	}
}

func TestPhase_Validate_NilReceiver(t *testing.T) {
	var p *Phase
	err := p.Validate()
	if err == nil {
		t.Error("Validate() expected error for nil receiver, got nil")
	}
}

func TestPhase_Validate_InvalidFields(t *testing.T) {
	tests := []struct {
		name  string
		phase Phase
	}{
		{
			"empty ID",
			Phase{ID: "", Name: "Test", Status: "pending", Progress: 0},
		},
		{
			"empty name",
			Phase{ID: "test-1", Name: "", Status: "pending", Progress: 0},
		},
		{
			"invalid status",
			Phase{ID: "test-1", Name: "Test", Status: "invalid", Progress: 0},
		},
		{
			"negative progress",
			Phase{ID: "test-1", Name: "Test", Status: "pending", Progress: -1},
		},
		{
			"over 100 progress",
			Phase{ID: "test-1", Name: "Test", Status: "pending", Progress: 150},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.phase.Validate()
			if err == nil {
				t.Error("Validate() expected error, got nil")
			}
		})
	}
}

func TestTimeline_Validate_HappyPath(t *testing.T) {
	tl := &Timeline{
		Version:     "1.0.0",
		LastUpdated: time.Now(),
		Status:      "alpha",
		Phases: []*Phase{
			{
				ID:       "phase-1",
				Name:     "Alpha",
				Status:   "in_progress",
				Progress: 25,
			},
		},
		Milestones: []*Milestone{
			{
				ID:       "milestone-1",
				Name:     "eBPF Foundation",
				Status:   "in_progress",
				Progress: 25,
			},
		},
	}

	err := tl.Validate()
	if err != nil {
		t.Errorf("Validate() expected no error, got: %v", err)
	}
}

func TestTimeline_Validate_NilReceiver(t *testing.T) {
	var tl *Timeline
	err := tl.Validate()
	if err == nil {
		t.Error("Validate() expected error for nil receiver, got nil")
	}
}

func TestTimeline_Validate_EmptyVersion(t *testing.T) {
	tl := &Timeline{
		Version:     "",
		LastUpdated: time.Now(),
		Status:      "alpha",
		Phases:      []*Phase{},
		Milestones:  []*Milestone{},
	}

	err := tl.Validate()
	if err == nil {
		t.Error("Validate() expected error for empty version, got nil")
	}
}

func TestTimeline_Validate_InvalidPhase(t *testing.T) {
	tl := &Timeline{
		Version:     "1.0.0",
		LastUpdated: time.Now(),
		Status:      "alpha",
		Phases: []*Phase{
			{ID: "", Name: "Invalid", Status: "pending", Progress: 0}, // invalid: empty ID
		},
		Milestones: []*Milestone{},
	}

	err := tl.Validate()
	if err == nil {
		t.Error("Validate() expected error for invalid phase, got nil")
	}
}

func TestTimeline_Validate_InvalidMilestone(t *testing.T) {
	tl := &Timeline{
		Version:     "1.0.0",
		LastUpdated: time.Now(),
		Status:      "alpha",
		Phases:      []*Phase{},
		Milestones: []*Milestone{
			{ID: "test-1", Name: "", Status: "pending", Progress: 0}, // invalid: empty name
		},
	}

	err := tl.Validate()
	if err == nil {
		t.Error("Validate() expected error for invalid milestone, got nil")
	}
}

// ============================================================================
// HELPER TESTS
// ============================================================================

func TestTimeline_GetMilestoneByID_Found(t *testing.T) {
	tl := &Timeline{
		Version:     "1.0.0",
		LastUpdated: time.Now(),
		Status:      "alpha",
		Milestones: []*Milestone{
			{ID: "milestone-1", Name: "Test 1", Status: "pending", Progress: 0},
			{ID: "milestone-2", Name: "Test 2", Status: "in_progress", Progress: 50},
		},
	}

	m, found := tl.GetMilestoneByID("milestone-2")
	if !found {
		t.Error("GetMilestoneByID() expected to find milestone-2, got not found")
	}
	if m == nil {
		t.Fatal("GetMilestoneByID() returned nil milestone")
	}
	if m.ID != "milestone-2" {
		t.Errorf("GetMilestoneByID() expected ID milestone-2, got %q", m.ID)
	}
}

func TestTimeline_GetMilestoneByID_NotFound(t *testing.T) {
	tl := &Timeline{
		Version:     "1.0.0",
		LastUpdated: time.Now(),
		Status:      "alpha",
		Milestones: []*Milestone{
			{ID: "milestone-1", Name: "Test 1", Status: "pending", Progress: 0},
		},
	}

	m, found := tl.GetMilestoneByID("nonexistent")
	if found {
		t.Error("GetMilestoneByID() expected not found, got found")
	}
	if m != nil {
		t.Error("GetMilestoneByID() expected nil milestone for not found, got non-nil")
	}
}

func TestTimeline_GetMilestoneByID_NilReceiver(t *testing.T) {
	var tl *Timeline
	m, found := tl.GetMilestoneByID("test")
	if found {
		t.Error("GetMilestoneByID() expected not found for nil receiver, got found")
	}
	if m != nil {
		t.Error("GetMilestoneByID() expected nil milestone for nil receiver, got non-nil")
	}
}

func TestTimeline_GetMilestoneByID_EmptyID(t *testing.T) {
	tl := &Timeline{
		Version:     "1.0.0",
		LastUpdated: time.Now(),
		Status:      "alpha",
		Milestones: []*Milestone{
			{ID: "milestone-1", Name: "Test 1", Status: "pending", Progress: 0},
		},
	}

	m, found := tl.GetMilestoneByID("")
	if found {
		t.Error("GetMilestoneByID() expected not found for empty ID, got found")
	}
	if m != nil {
		t.Error("GetMilestoneByID() expected nil milestone for empty ID, got non-nil")
	}
}

// ============================================================================
// CONCURRENCY TESTS
// ============================================================================

func TestTimeline_GetMilestoneByID_ConcurrentAccess(t *testing.T) {
	tl := &Timeline{
		Version:     "1.0.0",
		LastUpdated: time.Now(),
		Status:      "alpha",
		Milestones: []*Milestone{
			{ID: "milestone-1", Name: "Test 1", Status: "pending", Progress: 0},
			{ID: "milestone-2", Name: "Test 2", Status: "in_progress", Progress: 50},
			{ID: "milestone-3", Name: "Test 3", Status: "completed", Progress: 100},
		},
	}

	// Concurrent reads should not race
	const numGoroutines = 100
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id string) {
			_, _ = tl.GetMilestoneByID(id)
			done <- true
		}("milestone-2")
	}

	for i := 0; i < numGoroutines; i++ {
		<-done
	}
}
