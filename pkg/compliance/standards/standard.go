// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package standards provides compliance standard definitions and implementations.
package standards

import (
	"context"

	"unheaded/pkg/compliance/controls"
)

// Standard defines the interface for a compliance standard.
type Standard interface {
	// ID returns the unique identifier for the standard.
	ID() string

	// Name returns the human-readable name of the standard.
	Name() string

	// Version returns the version of the standard.
	Version() string

	// Description returns a description of the standard.
	Description() string

	// Controls returns all controls defined by this standard.
	Controls() []*controls.Control

	// GetControl returns a specific control by ID.
	GetControl(id string) (*controls.Control, bool)

	// Families returns the control families in this standard.
	Families() []*controls.ControlFamily

	// Validate validates that the standard is properly configured.
	Validate() error
}

// StandardRegistry manages registered compliance standards.
type StandardRegistry struct {
	standards map[string]Standard
}

// NewStandardRegistry creates a new standard registry.
func NewStandardRegistry() *StandardRegistry {
	return &StandardRegistry{
		standards: make(map[string]Standard),
	}
}

// Register registers a compliance standard.
func (r *StandardRegistry) Register(s Standard) {
	r.standards[s.ID()] = s
}

// Get returns a standard by ID.
func (r *StandardRegistry) Get(id string) (Standard, bool) {
	s, ok := r.standards[id]
	return s, ok
}

// List returns all registered standards.
func (r *StandardRegistry) List() []Standard {
	result := make([]Standard, 0, len(r.standards))
	for _, s := range r.standards {
		result = append(result, s)
	}
	return result
}

// BaseStandard provides a base implementation for compliance standards.
type BaseStandard struct {
	id          string
	name        string
	version     string
	description string
	controls    []*controls.Control
	controlMap  map[string]*controls.Control
	families    []*controls.ControlFamily
}

// NewBaseStandard creates a new base standard.
func NewBaseStandard(id, name, version, description string) *BaseStandard {
	return &BaseStandard{
		id:          id,
		name:        name,
		version:     version,
		description: description,
		controlMap:  make(map[string]*controls.Control),
	}
}

func (s *BaseStandard) ID() string          { return s.id }
func (s *BaseStandard) Name() string        { return s.name }
func (s *BaseStandard) Version() string     { return s.version }
func (s *BaseStandard) Description() string { return s.description }

func (s *BaseStandard) Controls() []*controls.Control {
	return s.controls
}

func (s *BaseStandard) GetControl(id string) (*controls.Control, bool) {
	c, ok := s.controlMap[id]
	return c, ok
}

func (s *BaseStandard) Families() []*controls.ControlFamily {
	return s.families
}

func (s *BaseStandard) AddControl(c *controls.Control) {
	s.controls = append(s.controls, c)
	s.controlMap[c.ID] = c
}

func (s *BaseStandard) AddFamily(f *controls.ControlFamily) {
	s.families = append(s.families, f)
}

func (s *BaseStandard) Validate() error {
	return nil
}

// StandardAssessor assesses compliance against a standard.
type StandardAssessor struct {
	standard Standard
	checker  *controls.CheckRunner
}

// NewStandardAssessor creates a new standard assessor.
func NewStandardAssessor(standard Standard, checker *controls.CheckRunner) *StandardAssessor {
	return &StandardAssessor{
		standard: standard,
		checker:  checker,
	}
}

// AssessmentResult contains the results of a standard assessment.
type AssessmentResult struct {
	Standard       string                    `json:"standard"`
	Version        string                    `json:"version"`
	TotalControls  int                       `json:"total_controls"`
	Compliant      int                       `json:"compliant"`
	NonCompliant   int                       `json:"non_compliant"`
	Partial        int                       `json:"partial"`
	NotAssessed    int                       `json:"not_assessed"`
	NotApplicable  int                       `json:"not_applicable"`
	ComplianceRate float64                   `json:"compliance_rate"`
	Results        []*controls.ControlResult `json:"results"`
}

// Assess runs an assessment against the standard.
func (a *StandardAssessor) Assess(ctx context.Context) (*AssessmentResult, error) {
	allControls := a.standard.Controls()
	results, err := a.checker.RunChecks(ctx, allControls)
	if err != nil {
		return nil, err
	}

	result := &AssessmentResult{
		Standard:      a.standard.ID(),
		Version:       a.standard.Version(),
		TotalControls: len(allControls),
		Results:       results,
	}

	for _, r := range results {
		switch r.Status {
		case controls.StatusCompliant:
			result.Compliant++
		case controls.StatusNonCompliant:
			result.NonCompliant++
		case controls.StatusPartial:
			result.Partial++
		case controls.StatusNotAssessed:
			result.NotAssessed++
		case controls.StatusNotApplicable:
			result.NotApplicable++
		}
	}

	assessable := result.TotalControls - result.NotApplicable - result.NotAssessed
	if assessable > 0 {
		result.ComplianceRate = float64(result.Compliant) / float64(assessable)
	}

	return result, nil
}
