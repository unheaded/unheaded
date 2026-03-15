// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package main

import (
	"encoding/json"
	"testing"
	"time"
)

// ============================================================================
// TIMELINE MANAGER TESTS
// ============================================================================

func TestNewTimelineManager(t *testing.T) {
	broadcast := func(eventType string, data interface{}) {}

	tm := NewTimelineManager(broadcast)
	if tm == nil {
		t.Fatal("expected non-nil TimelineManager")
	}

	if tm.tasks == nil {
		t.Error("expected tasks map to be initialized")
	}
}

func TestTimelineManager_HandleTimelineUpdate_Event(t *testing.T) {
	var receivedEvent string
	broadcast := func(eventType string, data interface{}) {
		receivedEvent = eventType
	}

	tm := NewTimelineManager(broadcast)

	// Test event notification
	eventPayload := `{"event":"timeline_reloaded","timestamp":"2026-01-30T10:00:00Z","source":"timeguru"}`

	err := tm.HandleTimelineUpdate([]byte(eventPayload))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedEvent != "timeline.event" {
		t.Errorf("expected timeline.event, got %s", receivedEvent)
	}
}

func TestTimelineManager_HandleTimelineUpdate_FullTimeline(t *testing.T) {
	receivedEvents := make([]string, 0)
	broadcast := func(eventType string, data interface{}) {
		receivedEvents = append(receivedEvents, eventType)
		_ = data // Acknowledge data for potential future use
	}

	tm := NewTimelineManager(broadcast)

	// Full timeline payload
	timeline := Timeline{
		Version:     "1.0.0",
		LastUpdated: time.Now(),
		Status:      "alpha",
		Phases: []*Phase{
			{
				ID:       "phase-1",
				Name:     "Alpha",
				Status:   "in_progress",
				Progress: 50,
			},
		},
		Milestones: []*Milestone{
			{
				ID:       "milestone-1",
				Name:     "Core Services",
				Status:   "in_progress",
				Progress: 75,
				Owner:    "Team",
				Tasks:    []string{"Task 1", "Task 2"},
			},
		},
	}

	payload, _ := json.Marshal(timeline)

	err := tm.HandleTimelineUpdate(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check that timeline.updated was one of the events
	hasTimelineUpdated := false
	for _, event := range receivedEvents {
		if event == "timeline.updated" {
			hasTimelineUpdated = true
			break
		}
	}
	if !hasTimelineUpdated {
		t.Errorf("expected timeline.updated in events, got %v", receivedEvents)
	}

	// Check tasks were created
	tasks := tm.GetTimelineTasks()
	if len(tasks) == 0 {
		t.Error("expected tasks to be created from timeline")
	}

	// Should have: 1 phase card + 1 milestone card + 2 sub-tasks = 4 tasks
	if len(tasks) != 4 {
		t.Errorf("expected 4 tasks, got %d", len(tasks))
	}
}

func TestTimelineManager_UpdateTimeline_NilTimeline(t *testing.T) {
	tm := NewTimelineManager(nil)

	err := tm.UpdateTimeline(nil)
	if err != ErrNilTimeline {
		t.Errorf("expected ErrNilTimeline, got %v", err)
	}
}

func TestTimelineManager_ConvertMilestoneStatus(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"completed", "done"},
		{"in_progress", "in-progress"},
		{"blocked", "in-progress"},
		{"pending", "todo"},
		{"planned", "todo"},
		{"unknown", "todo"},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := mapMilestoneStatusToTaskStatus(tc.input)
			if result != tc.expected {
				t.Errorf("expected %s, got %s", tc.expected, result)
			}
		})
	}
}

func TestTimelineManager_ConvertPhaseStatus(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"completed", "done"},
		{"in_progress", "in-progress"},
		{"planned", "todo"},
		{"unknown", "todo"},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := mapPhaseStatusToTaskStatus(tc.input)
			if result != tc.expected {
				t.Errorf("expected %s, got %s", tc.expected, result)
			}
		})
	}
}

func TestTimelineManager_TaskChanged(t *testing.T) {
	task1 := &Task{
		ID:       "test-1",
		Title:    "Test Task",
		Status:   "todo",
		Progress: 0,
	}

	task2 := &Task{
		ID:       "test-1",
		Title:    "Test Task",
		Status:   "todo",
		Progress: 0,
	}

	// Identical tasks
	if taskChanged(task1, task2) {
		t.Error("expected identical tasks to not be changed")
	}

	// Different title
	task2.Title = "Different Title"
	if !taskChanged(task1, task2) {
		t.Error("expected different titles to be changed")
	}

	// Nil handling
	if !taskChanged(nil, task2) {
		t.Error("expected nil comparison to show changed")
	}

	if !taskChanged(task1, nil) {
		t.Error("expected nil comparison to show changed")
	}

	if taskChanged(nil, nil) {
		t.Error("expected both nil to not be changed")
	}
}

func TestTimelineManager_HandleEmptyPayload(t *testing.T) {
	tm := NewTimelineManager(nil)

	err := tm.HandleTimelineUpdate([]byte{})
	if err != ErrInvalidTimeline {
		t.Errorf("expected ErrInvalidTimeline, got %v", err)
	}
}

func TestTimelineManager_GetTimeline_Empty(t *testing.T) {
	tm := NewTimelineManager(nil)

	tl := tm.GetTimeline()
	if tl != nil {
		t.Error("expected nil timeline before update")
	}
}

func TestTimelineManager_GetTimelineTasks_Empty(t *testing.T) {
	tm := NewTimelineManager(nil)

	tasks := tm.GetTimelineTasks()
	if len(tasks) != 0 {
		t.Error("expected empty tasks before update")
	}
}

// ============================================================================
// INTEGRATION TESTS
// ============================================================================

func TestTimelineManager_FullWorkflow(t *testing.T) {
	eventLog := make([]string, 0)
	broadcast := func(eventType string, data interface{}) {
		eventLog = append(eventLog, eventType)
	}

	tm := NewTimelineManager(broadcast)

	// 1. Initial timeline
	timeline1 := &Timeline{
		Version: "1.0.0",
		Status:  "alpha",
		Phases: []*Phase{
			{ID: "phase-1", Name: "Alpha", Status: "in_progress"},
		},
		Milestones: []*Milestone{
			{ID: "ms-1", Name: "Milestone 1", Status: "in_progress"},
		},
	}

	err := tm.UpdateTimeline(timeline1)
	if err != nil {
		t.Fatalf("first update failed: %v", err)
	}

	tasks1 := tm.GetTimelineTasks()
	// 1 phase + 1 milestone = 2
	if len(tasks1) != 2 {
		t.Errorf("expected 2 tasks after first update, got %d", len(tasks1))
	}

	// 2. Update timeline with changes
	timeline2 := &Timeline{
		Version: "1.0.1",
		Status:  "alpha",
		Phases: []*Phase{
			{ID: "phase-1", Name: "Alpha", Status: "completed"}, // Status changed
		},
		Milestones: []*Milestone{
			{ID: "ms-1", Name: "Milestone 1", Status: "completed"}, // Status changed
			{ID: "ms-2", Name: "Milestone 2", Status: "pending"},   // New milestone
		},
	}

	err = tm.UpdateTimeline(timeline2)
	if err != nil {
		t.Fatalf("second update failed: %v", err)
	}

	tasks2 := tm.GetTimelineTasks()
	// 1 phase + 2 milestones = 3
	if len(tasks2) != 3 {
		t.Errorf("expected 3 tasks after second update, got %d", len(tasks2))
	}

	// Check event log
	if len(eventLog) < 2 {
		t.Errorf("expected at least 2 events, got %d", len(eventLog))
	}
}
