// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package controls provides control definitions and management for compliance frameworks.
package controls

import (
	"time"
)

// ControlStatus represents the current status of a control.
type ControlStatus string

const (
	StatusNotAssessed  ControlStatus = "not_assessed"
	StatusCompliant    ControlStatus = "compliant"
	StatusNonCompliant ControlStatus = "non_compliant"
	StatusPartial      ControlStatus = "partial"
	StatusNotApplicable ControlStatus = "not_applicable"
)

// ControlCategory represents the category of a control.
type ControlCategory string

const (
	CategoryAccessControl      ControlCategory = "access_control"
	CategoryAuditLogging       ControlCategory = "audit_logging"
	CategoryDataProtection     ControlCategory = "data_protection"
	CategoryIncidentResponse   ControlCategory = "incident_response"
	CategoryRiskManagement     ControlCategory = "risk_management"
	CategorySecurityAwareness  ControlCategory = "security_awareness"
	CategorySystemIntegrity    ControlCategory = "system_integrity"
	CategoryNetworkSecurity    ControlCategory = "network_security"
	CategoryChangeManagement   ControlCategory = "change_management"
	CategoryVendorManagement   ControlCategory = "vendor_management"
	CategoryPhysicalSecurity   ControlCategory = "physical_security"
	CategoryPrivacy            ControlCategory = "privacy"
)

// Severity represents the severity level of a control failure.
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
	SeverityInfo     Severity = "info"
)

// Control represents a compliance control definition.
type Control struct {
	ID            string            `json:"id"`
	Standard      string            `json:"standard"`
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	Category      ControlCategory   `json:"category"`
	Severity      Severity          `json:"severity"`
	Requirements  []string          `json:"requirements"`
	TestProcedure string            `json:"test_procedure"`
	Guidance      string            `json:"guidance"`
	References    []string          `json:"references"`
	Mappings      map[string]string `json:"mappings"` // Cross-framework mappings
	Automated     bool              `json:"automated"`
	Frequency     time.Duration     `json:"frequency"`
	Owner         string            `json:"owner"`
	Tags          []string          `json:"tags"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

// ControlResult represents the result of a control assessment.
type ControlResult struct {
	ControlID     string        `json:"control_id"`
	Status        ControlStatus `json:"status"`
	Score         float64       `json:"score"` // 0.0 to 1.0
	Findings      []Finding     `json:"findings"`
	Evidence      []string      `json:"evidence_ids"`
	TestedAt      time.Time     `json:"tested_at"`
	TestedBy      string        `json:"tested_by"`
	Notes         string        `json:"notes"`
	RemediationID string        `json:"remediation_id,omitempty"`
}

// Finding represents a specific finding from a control assessment.
type Finding struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Severity    Severity  `json:"severity"`
	Resource    string    `json:"resource"`
	Location    string    `json:"location"`
	Remediation string    `json:"remediation"`
	DetectedAt  time.Time `json:"detected_at"`
}

// Remediation represents a remediation action for a control finding.
type Remediation struct {
	ID           string            `json:"id"`
	ControlID    string            `json:"control_id"`
	FindingID    string            `json:"finding_id"`
	Title        string            `json:"title"`
	Description  string            `json:"description"`
	Status       RemediationStatus `json:"status"`
	Priority     Severity          `json:"priority"`
	Owner        string            `json:"owner"`
	DueDate      time.Time         `json:"due_date"`
	CompletedAt  *time.Time        `json:"completed_at,omitempty"`
	Notes        string            `json:"notes"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

// RemediationStatus represents the status of a remediation action.
type RemediationStatus string

const (
	RemediationOpen       RemediationStatus = "open"
	RemediationInProgress RemediationStatus = "in_progress"
	RemediationCompleted  RemediationStatus = "completed"
	RemediationAccepted   RemediationStatus = "risk_accepted"
	RemediationDeferred   RemediationStatus = "deferred"
)

// ControlFamily represents a group of related controls.
type ControlFamily struct {
	ID          string     `json:"id"`
	Standard    string     `json:"standard"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Controls    []*Control `json:"controls"`
}

// NewControl creates a new control with default values.
func NewControl(id, standard, name, description string) *Control {
	now := time.Now()
	return &Control{
		ID:          id,
		Standard:    standard,
		Name:        name,
		Description: description,
		Mappings:    make(map[string]string),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// WithCategory sets the control category.
func (c *Control) WithCategory(cat ControlCategory) *Control {
	c.Category = cat
	return c
}

// WithSeverity sets the control severity.
func (c *Control) WithSeverity(sev Severity) *Control {
	c.Severity = sev
	return c
}

// WithRequirements sets the control requirements.
func (c *Control) WithRequirements(reqs ...string) *Control {
	c.Requirements = reqs
	return c
}

// AddMapping adds a cross-framework mapping.
func (c *Control) AddMapping(framework, controlID string) *Control {
	c.Mappings[framework] = controlID
	return c
}
