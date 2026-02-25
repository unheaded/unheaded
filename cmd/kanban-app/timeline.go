// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package main - Timeline integration for THE META MOMENT
// Kanban tracks its own timeline in real-time via Wotan
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// TopicTimelineUpdates is defined in wotan.go

// Timeline errors
var (
	ErrInvalidTimeline  = errors.New("invalid timeline data")
	ErrNilTimeline      = errors.New("timeline cannot be nil")
	ErrTimelineParsing  = errors.New("failed to parse timeline")
)

// TimelineEvent represents an event from Timeguru
type TimelineEvent struct {
	Event     string    `json:"event"`
	Timestamp time.Time `json:"timestamp"`
	Source    string    `json:"source"`
}

// Timeline represents the full timeline structure from Timeguru
type Timeline struct {
	Version     string            `json:"version"`
	LastUpdated time.Time         `json:"last_updated"`
	Status      string            `json:"status"`
	Vision      string            `json:"vision,omitempty"`
	Phases      []*Phase          `json:"phases"`
	Milestones  []*Milestone      `json:"milestones"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// Phase represents a project phase (maps to a column concept)
type Phase struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	StartDate   time.Time `json:"start_date,omitempty"`
	EndDate     time.Time `json:"end_date,omitempty"`
	Progress    int       `json:"progress"`
	Description string    `json:"description,omitempty"`
	Milestones  []string  `json:"milestones,omitempty"`
}

// Milestone represents a project milestone (maps to a card)
type Milestone struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Status       string    `json:"status"`
	ETA          time.Time `json:"eta,omitempty"`
	Progress     int       `json:"progress"`
	Owner        string    `json:"owner,omitempty"`
	Risk         string    `json:"risk,omitempty"`
	Description  string    `json:"description,omitempty"`
	Tasks        []string  `json:"tasks,omitempty"`
	Dependencies []string  `json:"dependencies,omitempty"`
}

// TimelineManager manages timeline state and conversion to tasks
type TimelineManager struct {
	timeline *Timeline
	tasks    map[string]*Task
	mu       sync.RWMutex

	// Callback for broadcasting updates
	broadcast func(eventType string, data interface{})
}

// NewTimelineManager creates a new TimelineManager
func NewTimelineManager(broadcast func(string, interface{})) *TimelineManager {
	return &TimelineManager{
		tasks:     make(map[string]*Task),
		broadcast: broadcast,
	}
}

// HandleTimelineUpdate processes a timeline update event from Wotan
func (tm *TimelineManager) HandleTimelineUpdate(payload []byte) error {
	if len(payload) == 0 {
		return ErrInvalidTimeline
	}

	// First try to parse as TimelineEvent (status update notification)
	var event TimelineEvent
	if err := json.Unmarshal(payload, &event); err == nil && event.Event != "" {
		log.Info().
			Str("event", event.Event).
			Str("source", event.Source).
			Msg("received timeline event notification")

		// Notify frontend of the update event
		if tm.broadcast != nil {
			tm.broadcast("timeline.event", map[string]interface{}{
				"event":     event.Event,
				"timestamp": event.Timestamp,
				"source":    event.Source,
			})
		}

		return nil
	}

	// Try to parse as full Timeline
	var timeline Timeline
	if err := json.Unmarshal(payload, &timeline); err != nil {
		return fmt.Errorf("%w: %v", ErrTimelineParsing, err)
	}

	return tm.UpdateTimeline(&timeline)
}

// UpdateTimeline updates the internal timeline and generates tasks
func (tm *TimelineManager) UpdateTimeline(timeline *Timeline) error {
	if timeline == nil {
		return ErrNilTimeline
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.timeline = timeline

	// Convert milestones to tasks
	newTasks := tm.convertMilestonesToTasks(timeline)

	// Track changes for broadcasting
	var added, updated, removed []*Task

	// Find new and updated tasks
	for id, task := range newTasks {
		if existing, ok := tm.tasks[id]; ok {
			// Check if changed
			if taskChanged(existing, task) {
				updated = append(updated, task)
			}
		} else {
			added = append(added, task)
		}
	}

	// Find removed tasks
	for id, task := range tm.tasks {
		if _, ok := newTasks[id]; !ok {
			removed = append(removed, task)
		}
	}

	// Update local store
	tm.tasks = newTasks

	// Broadcast changes
	if tm.broadcast != nil {
		// Broadcast full timeline refresh
		tm.broadcast("timeline.updated", map[string]interface{}{
			"tasks":        taskMapToSlice(newTasks),
			"phases":       timeline.Phases,
			"milestones":   timeline.Milestones,
			"status":       timeline.Status,
			"last_updated": timeline.LastUpdated,
			"added":        len(added),
			"updated":      len(updated),
			"removed":      len(removed),
		})

		// Also broadcast individual changes for granular updates
		for _, task := range added {
			tm.broadcast("timeline.task.created", task)
		}
		for _, task := range updated {
			tm.broadcast("timeline.task.updated", task)
		}
		for _, task := range removed {
			tm.broadcast("timeline.task.removed", task)
		}
	}

	log.Info().
		Int("phases", len(timeline.Phases)).
		Int("milestones", len(timeline.Milestones)).
		Int("tasks", len(newTasks)).
		Int("added", len(added)).
		Int("updated", len(updated)).
		Int("removed", len(removed)).
		Msg("timeline updated")

	return nil
}

// GetTimelineTasks returns all tasks derived from the timeline
func (tm *TimelineManager) GetTimelineTasks() []*Task {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	return taskMapToSlice(tm.tasks)
}

// GetTimeline returns the current timeline
func (tm *TimelineManager) GetTimeline() *Timeline {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.timeline
}

// convertMilestonesToTasks converts timeline milestones to Kanban tasks
func (tm *TimelineManager) convertMilestonesToTasks(timeline *Timeline) map[string]*Task {
	tasks := make(map[string]*Task)
	now := time.Now()

	// Create a phase map for quick lookup
	phaseMap := make(map[string]*Phase)
	for _, phase := range timeline.Phases {
		phaseMap[phase.ID] = phase
	}

	// Convert each milestone to a task
	for _, m := range timeline.Milestones {
		task := &Task{
			ID:          fmt.Sprintf("tl-%s", m.ID),
			Title:       m.Name,
			Description: m.Description,
			Status:      mapMilestoneStatusToTaskStatus(m.Status),
			Type:        "milestone",
			Owner:       m.Owner,
			Progress:    m.Progress,
			CreatedAt:   now,
			UpdatedAt:   timeline.LastUpdated,
		}

		// Find parent phase for this milestone
		for _, phase := range timeline.Phases {
			for _, msID := range phase.Milestones {
				if msID == m.ID {
					// Add phase info to description
					if task.Description == "" {
						task.Description = fmt.Sprintf("Phase: %s", phase.Name)
					} else {
						task.Description = fmt.Sprintf("Phase: %s\n%s", phase.Name, task.Description)
					}
					break
				}
			}
		}

		// Add ETA to description if present
		if !m.ETA.IsZero() {
			task.Description = fmt.Sprintf("%s\nETA: %s", task.Description, m.ETA.Format("Jan 2, 2006"))
		}

		// Add risk level if present
		if m.Risk != "" {
			task.Description = fmt.Sprintf("%s\nRisk: %s", task.Description, cases.Title(language.English).String(m.Risk))
		}

		tasks[task.ID] = task

		// Also create tasks for each sub-task of the milestone
		for i, taskName := range m.Tasks {
			subTask := &Task{
				ID:          fmt.Sprintf("tl-%s-task-%d", m.ID, i),
				Title:       taskName,
				Description: fmt.Sprintf("Sub-task of: %s", m.Name),
				Status:      mapMilestoneStatusToTaskStatus(m.Status),
				Type:        "task",
				Owner:       m.Owner,
				Progress:    0, // Individual tasks don't have progress
				CreatedAt:   now,
				UpdatedAt:   timeline.LastUpdated,
			}

			// If parent milestone is complete, mark sub-tasks as complete
			if m.Status == "completed" || m.Progress == 100 {
				subTask.Status = "done"
				subTask.Progress = 100
			}

			tasks[subTask.ID] = subTask
		}
	}

	// Also create phase cards as epic/milestone markers
	for _, phase := range timeline.Phases {
		task := &Task{
			ID:          fmt.Sprintf("tl-phase-%s", phase.ID),
			Title:       fmt.Sprintf("[PHASE] %s", phase.Name),
			Description: phase.Description,
			Status:      mapPhaseStatusToTaskStatus(phase.Status),
			Type:        "epic",
			Progress:    phase.Progress,
			CreatedAt:   now,
			UpdatedAt:   timeline.LastUpdated,
		}

		tasks[task.ID] = task
	}

	return tasks
}

// mapMilestoneStatusToTaskStatus converts timeline milestone status to Kanban task status
func mapMilestoneStatusToTaskStatus(status string) string {
	switch status {
	case "completed":
		return "done"
	case "in_progress":
		return "in-progress"
	case "blocked":
		return "in-progress" // Show blocked items in progress column with visual indicator
	case "pending", "planned":
		return "todo"
	default:
		return "todo"
	}
}

// mapPhaseStatusToTaskStatus converts timeline phase status to Kanban task status
func mapPhaseStatusToTaskStatus(status string) string {
	switch status {
	case "completed":
		return "done"
	case "in_progress":
		return "in-progress"
	case "planned":
		return "todo"
	default:
		return "todo"
	}
}

// taskChanged checks if two tasks are different
func taskChanged(a, b *Task) bool {
	if a == nil || b == nil {
		return a != b
	}

	return a.Title != b.Title ||
		a.Description != b.Description ||
		a.Status != b.Status ||
		a.Progress != b.Progress ||
		a.Owner != b.Owner
}

// taskMapToSlice converts task map to slice
func taskMapToSlice(tasks map[string]*Task) []*Task {
	result := make([]*Task, 0, len(tasks))
	for _, task := range tasks {
		result = append(result, task)
	}
	return result
}
