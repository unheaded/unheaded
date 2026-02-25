// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package timeline

import (
	"errors"
	"fmt"
	"time"
)

// ============================================================================
// ERRORS
// ============================================================================

var (
	ErrNilReceiver     = errors.New("nil receiver")
	ErrEmptyID         = errors.New("ID cannot be empty")
	ErrEmptyName       = errors.New("name cannot be empty")
	ErrEmptyVersion    = errors.New("version cannot be empty")
	ErrInvalidStatus   = errors.New("invalid status")
	ErrInvalidProgress = errors.New("progress must be between 0 and 100")
	ErrInvalidRisk     = errors.New("invalid risk level")
	ErrInvalidPhase    = errors.New("invalid phase")
	ErrInvalidMilestone = errors.New("invalid milestone")
)

// ============================================================================
// MILESTONE
// ============================================================================

// Milestone represents a single project milestone
type Milestone struct {
	ID           string    `json:"id" yaml:"id" toml:"id"`
	Name         string    `json:"name" yaml:"name" toml:"name"`
	Status       string    `json:"status" yaml:"status" toml:"status"` // pending, in_progress, completed, blocked
	ETA          time.Time `json:"eta,omitempty" yaml:"eta,omitempty" toml:"eta,omitempty"`
	Progress     int       `json:"progress" yaml:"progress" toml:"progress"` // 0-100
	Owner        string    `json:"owner,omitempty" yaml:"owner,omitempty" toml:"owner,omitempty"`
	Risk         string    `json:"risk,omitempty" yaml:"risk,omitempty" toml:"risk,omitempty"` // low, medium, high
	Description  string    `json:"description,omitempty" yaml:"description,omitempty" toml:"description,omitempty"`
	Tasks        []string  `json:"tasks,omitempty" yaml:"tasks,omitempty" toml:"tasks,omitempty"`
	Dependencies []string  `json:"dependencies,omitempty" yaml:"dependencies,omitempty" toml:"dependencies,omitempty"`
}

// Validate performs defensive validation on a Milestone
func (m *Milestone) Validate() error {
	if m == nil {
		return ErrNilReceiver
	}

	if m.ID == "" {
		return ErrEmptyID
	}

	if m.Name == "" {
		return ErrEmptyName
	}

	// Valid statuses: pending, in_progress, completed, blocked
	switch m.Status {
	case "pending", "in_progress", "completed", "blocked":
		// valid
	default:
		return fmt.Errorf("%w: %q (must be pending, in_progress, completed, or blocked)", ErrInvalidStatus, m.Status)
	}

	if m.Progress < 0 || m.Progress > 100 {
		return fmt.Errorf("%w: %d", ErrInvalidProgress, m.Progress)
	}

	// Risk is optional, but if present must be valid
	if m.Risk != "" {
		switch m.Risk {
		case "low", "medium", "high":
			// valid
		default:
			return fmt.Errorf("%w: %q (must be low, medium, or high)", ErrInvalidRisk, m.Risk)
		}
	}

	return nil
}

// ============================================================================
// PHASE
// ============================================================================

// Phase represents a project phase (e.g., Alpha, Beta)
type Phase struct {
	ID          string    `json:"id" yaml:"id" toml:"id"`
	Name        string    `json:"name" yaml:"name" toml:"name"`
	Status      string    `json:"status" yaml:"status" toml:"status"` // planned, in_progress, completed
	StartDate   time.Time `json:"start_date,omitempty" yaml:"start_date,omitempty" toml:"start_date,omitempty"`
	EndDate     time.Time `json:"end_date,omitempty" yaml:"end_date,omitempty" toml:"end_date,omitempty"`
	Progress    int       `json:"progress" yaml:"progress" toml:"progress"` // 0-100
	Description string    `json:"description,omitempty" yaml:"description,omitempty" toml:"description,omitempty"`
	Milestones  []string  `json:"milestones,omitempty" yaml:"milestones,omitempty" toml:"milestones,omitempty"` // References to milestone IDs
}

// Validate performs defensive validation on a Phase
func (p *Phase) Validate() error {
	if p == nil {
		return ErrNilReceiver
	}

	if p.ID == "" {
		return ErrEmptyID
	}

	if p.Name == "" {
		return ErrEmptyName
	}

	// Valid statuses: planned, in_progress, completed
	switch p.Status {
	case "planned", "in_progress", "completed":
		// valid
	default:
		return fmt.Errorf("%w: %q (must be planned, in_progress, or completed)", ErrInvalidStatus, p.Status)
	}

	if p.Progress < 0 || p.Progress > 100 {
		return fmt.Errorf("%w: %d", ErrInvalidProgress, p.Progress)
	}

	return nil
}

// ============================================================================
// TIMELINE
// ============================================================================

// Timeline represents the complete project timeline
type Timeline struct {
	Version     string            `json:"version" yaml:"version" toml:"version"`
	LastUpdated time.Time         `json:"last_updated" yaml:"last_updated" toml:"last_updated"`
	Status      string            `json:"status" yaml:"status" toml:"status"` // planning, alpha, beta, mvp, production
	Vision      string            `json:"vision,omitempty" yaml:"vision,omitempty" toml:"vision,omitempty"`
	Phases      []*Phase          `json:"phases" yaml:"phases" toml:"phases"`
	Milestones  []*Milestone      `json:"milestones" yaml:"milestones" toml:"milestones"`
	Metadata    map[string]string `json:"metadata,omitempty" yaml:"metadata,omitempty" toml:"metadata,omitempty"`
}

// Validate performs defensive validation on a Timeline
func (t *Timeline) Validate() error {
	if t == nil {
		return ErrNilReceiver
	}

	if t.Version == "" {
		return ErrEmptyVersion
	}

	// Validate all phases
	for i, phase := range t.Phases {
		if err := phase.Validate(); err != nil {
			return fmt.Errorf("%w at index %d: %v", ErrInvalidPhase, i, err)
		}
	}

	// Validate all milestones
	for i, milestone := range t.Milestones {
		if err := milestone.Validate(); err != nil {
			return fmt.Errorf("%w at index %d: %v", ErrInvalidMilestone, i, err)
		}
	}

	return nil
}

// GetMilestoneByID retrieves a milestone by ID (defensive implementation)
func (t *Timeline) GetMilestoneByID(id string) (*Milestone, bool) {
	if t == nil {
		return nil, false
	}

	if id == "" {
		return nil, false
	}

	for _, m := range t.Milestones {
		if m != nil && m.ID == id {
			return m, true
		}
	}

	return nil, false
}

// GetPhaseByID retrieves a phase by ID (defensive implementation)
func (t *Timeline) GetPhaseByID(id string) (*Phase, bool) {
	if t == nil {
		return nil, false
	}

	if id == "" {
		return nil, false
	}

	for _, p := range t.Phases {
		if p != nil && p.ID == id {
			return p, true
		}
	}

	return nil, false
}
