// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package audit provides audit logging for compliance activities.
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// EventType represents the type of audit event.
type EventType string

const (
	EventAssessmentStarted   EventType = "assessment_started"
	EventAssessmentCompleted EventType = "assessment_completed"
	EventControlChecked      EventType = "control_checked"
	EventEvidenceCollected   EventType = "evidence_collected"
	EventEvidenceDeleted     EventType = "evidence_deleted"
	EventReportGenerated     EventType = "report_generated"
	EventReportExported      EventType = "report_exported"
	EventRemediationCreated  EventType = "remediation_created"
	EventRemediationUpdated  EventType = "remediation_updated"
	EventRemediationClosed   EventType = "remediation_closed"
	EventConfigChanged       EventType = "config_changed"
	EventUserAction          EventType = "user_action"
	EventSystemAction        EventType = "system_action"
	EventError               EventType = "error"
)

// Severity represents the severity level of an audit event.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityError    Severity = "error"
	SeverityCritical Severity = "critical"
)

// AuditEvent represents a single audit event.
type AuditEvent struct {
	ID          string                 `json:"id"`
	Timestamp   time.Time              `json:"timestamp"`
	Type        EventType              `json:"type"`
	Severity    Severity               `json:"severity"`
	Actor       string                 `json:"actor"`
	Action      string                 `json:"action"`
	Resource    string                 `json:"resource"`
	ResourceID  string                 `json:"resource_id"`
	Description string                 `json:"description"`
	Outcome     string                 `json:"outcome"`
	Metadata    map[string]interface{} `json:"metadata"`
	ClientIP    string                 `json:"client_ip,omitempty"`
	UserAgent   string                 `json:"user_agent,omitempty"`
	SessionID   string                 `json:"session_id,omitempty"`
	CorrelationID string               `json:"correlation_id,omitempty"`
}

// AuditLogger defines the interface for audit logging.
type AuditLogger interface {
	Log(ctx context.Context, event *AuditEvent) error
	Query(ctx context.Context, filter AuditFilter) ([]*AuditEvent, error)
	Export(ctx context.Context, filter AuditFilter) ([]byte, error)
}

// AuditFilter defines criteria for filtering audit events.
type AuditFilter struct {
	StartTime     *time.Time
	EndTime       *time.Time
	Types         []EventType
	Severities    []Severity
	Actors        []string
	Resources     []string
	ResourceIDs   []string
	CorrelationID string
	Limit         int
	Offset        int
}

// InMemoryAuditLogger implements AuditLogger with in-memory storage.
type InMemoryAuditLogger struct {
	mu     sync.RWMutex
	events []*AuditEvent
	maxSize int
}

// NewInMemoryAuditLogger creates a new in-memory audit logger.
func NewInMemoryAuditLogger(maxSize int) *InMemoryAuditLogger {
	return &InMemoryAuditLogger{
		events:  make([]*AuditEvent, 0),
		maxSize: maxSize,
	}
}

func (l *InMemoryAuditLogger) Log(ctx context.Context, event *AuditEvent) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if event.ID == "" {
		event.ID = fmt.Sprintf("AE-%d", time.Now().UnixNano())
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	l.events = append(l.events, event)

	// Trim if over max size
	if len(l.events) > l.maxSize {
		l.events = l.events[len(l.events)-l.maxSize:]
	}

	return nil
}

func (l *InMemoryAuditLogger) Query(ctx context.Context, filter AuditFilter) ([]*AuditEvent, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var results []*AuditEvent

	for _, event := range l.events {
		if l.matchesFilter(event, filter) {
			results = append(results, event)
		}
	}

	// Apply limit and offset
	if filter.Offset > 0 && filter.Offset < len(results) {
		results = results[filter.Offset:]
	}
	if filter.Limit > 0 && filter.Limit < len(results) {
		results = results[:filter.Limit]
	}

	return results, nil
}

func (l *InMemoryAuditLogger) Export(ctx context.Context, filter AuditFilter) ([]byte, error) {
	events, err := l.Query(ctx, filter)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(events, "", "  ")
}

func (l *InMemoryAuditLogger) matchesFilter(event *AuditEvent, filter AuditFilter) bool {
	if filter.StartTime != nil && event.Timestamp.Before(*filter.StartTime) {
		return false
	}
	if filter.EndTime != nil && event.Timestamp.After(*filter.EndTime) {
		return false
	}

	if len(filter.Types) > 0 {
		found := false
		for _, t := range filter.Types {
			if event.Type == t {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if len(filter.Severities) > 0 {
		found := false
		for _, s := range filter.Severities {
			if event.Severity == s {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if len(filter.Actors) > 0 {
		found := false
		for _, a := range filter.Actors {
			if event.Actor == a {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if len(filter.Resources) > 0 {
		found := false
		for _, r := range filter.Resources {
			if event.Resource == r {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if len(filter.ResourceIDs) > 0 {
		found := false
		for _, id := range filter.ResourceIDs {
			if event.ResourceID == id {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if filter.CorrelationID != "" && event.CorrelationID != filter.CorrelationID {
		return false
	}

	return true
}

// AuditLogBuilder helps build audit events fluently.
type AuditLogBuilder struct {
	event *AuditEvent
}

// NewAuditEvent creates a new audit event builder.
func NewAuditEvent(eventType EventType) *AuditLogBuilder {
	return &AuditLogBuilder{
		event: &AuditEvent{
			ID:        fmt.Sprintf("AE-%d", time.Now().UnixNano()),
			Timestamp: time.Now(),
			Type:      eventType,
			Severity:  SeverityInfo,
			Metadata:  make(map[string]interface{}),
		},
	}
}

func (b *AuditLogBuilder) WithSeverity(s Severity) *AuditLogBuilder {
	b.event.Severity = s
	return b
}

func (b *AuditLogBuilder) WithActor(actor string) *AuditLogBuilder {
	b.event.Actor = actor
	return b
}

func (b *AuditLogBuilder) WithAction(action string) *AuditLogBuilder {
	b.event.Action = action
	return b
}

func (b *AuditLogBuilder) WithResource(resource, resourceID string) *AuditLogBuilder {
	b.event.Resource = resource
	b.event.ResourceID = resourceID
	return b
}

func (b *AuditLogBuilder) WithDescription(desc string) *AuditLogBuilder {
	b.event.Description = desc
	return b
}

func (b *AuditLogBuilder) WithOutcome(outcome string) *AuditLogBuilder {
	b.event.Outcome = outcome
	return b
}

func (b *AuditLogBuilder) WithMetadata(key string, value interface{}) *AuditLogBuilder {
	b.event.Metadata[key] = value
	return b
}

func (b *AuditLogBuilder) WithClientInfo(ip, userAgent string) *AuditLogBuilder {
	b.event.ClientIP = ip
	b.event.UserAgent = userAgent
	return b
}

func (b *AuditLogBuilder) WithSession(sessionID string) *AuditLogBuilder {
	b.event.SessionID = sessionID
	return b
}

func (b *AuditLogBuilder) WithCorrelation(correlationID string) *AuditLogBuilder {
	b.event.CorrelationID = correlationID
	return b
}

func (b *AuditLogBuilder) Build() *AuditEvent {
	return b.event
}

// LogTo logs the built event to the specified logger.
func (b *AuditLogBuilder) LogTo(ctx context.Context, logger AuditLogger) error {
	return logger.Log(ctx, b.event)
}

// ComplianceAuditor provides compliance-specific audit logging.
type ComplianceAuditor struct {
	logger        AuditLogger
	defaultActor  string
	correlationID string
}

// NewComplianceAuditor creates a new compliance auditor.
func NewComplianceAuditor(logger AuditLogger) *ComplianceAuditor {
	return &ComplianceAuditor{
		logger: logger,
	}
}

// WithActor sets the default actor for all events.
func (a *ComplianceAuditor) WithActor(actor string) *ComplianceAuditor {
	a.defaultActor = actor
	return a
}

// WithCorrelation sets a correlation ID for related events.
func (a *ComplianceAuditor) WithCorrelation(id string) *ComplianceAuditor {
	a.correlationID = id
	return a
}

// LogAssessmentStart logs the start of an assessment.
func (a *ComplianceAuditor) LogAssessmentStart(ctx context.Context, standard, assessmentID string) error {
	return NewAuditEvent(EventAssessmentStarted).
		WithActor(a.defaultActor).
		WithAction("start_assessment").
		WithResource("assessment", assessmentID).
		WithDescription(fmt.Sprintf("Started compliance assessment for %s", standard)).
		WithOutcome("in_progress").
		WithMetadata("standard", standard).
		WithCorrelation(a.correlationID).
		LogTo(ctx, a.logger)
}

// LogAssessmentComplete logs the completion of an assessment.
func (a *ComplianceAuditor) LogAssessmentComplete(ctx context.Context, assessmentID string, result map[string]interface{}) error {
	event := NewAuditEvent(EventAssessmentCompleted).
		WithActor(a.defaultActor).
		WithAction("complete_assessment").
		WithResource("assessment", assessmentID).
		WithDescription("Completed compliance assessment").
		WithOutcome("success").
		WithCorrelation(a.correlationID)

	for k, v := range result {
		event.WithMetadata(k, v)
	}

	return event.LogTo(ctx, a.logger)
}

// LogControlCheck logs a control check.
func (a *ComplianceAuditor) LogControlCheck(ctx context.Context, controlID, status string, score float64) error {
	return NewAuditEvent(EventControlChecked).
		WithActor(a.defaultActor).
		WithAction("check_control").
		WithResource("control", controlID).
		WithDescription(fmt.Sprintf("Checked control %s: %s", controlID, status)).
		WithOutcome(status).
		WithMetadata("score", score).
		WithCorrelation(a.correlationID).
		LogTo(ctx, a.logger)
}

// LogEvidenceCollection logs evidence collection.
func (a *ComplianceAuditor) LogEvidenceCollection(ctx context.Context, controlID, evidenceID, evidenceType string) error {
	return NewAuditEvent(EventEvidenceCollected).
		WithActor(a.defaultActor).
		WithAction("collect_evidence").
		WithResource("evidence", evidenceID).
		WithDescription(fmt.Sprintf("Collected %s evidence for control %s", evidenceType, controlID)).
		WithOutcome("success").
		WithMetadata("control_id", controlID).
		WithMetadata("evidence_type", evidenceType).
		WithCorrelation(a.correlationID).
		LogTo(ctx, a.logger)
}

// LogReportGeneration logs report generation.
func (a *ComplianceAuditor) LogReportGeneration(ctx context.Context, reportID, reportType string) error {
	return NewAuditEvent(EventReportGenerated).
		WithActor(a.defaultActor).
		WithAction("generate_report").
		WithResource("report", reportID).
		WithDescription(fmt.Sprintf("Generated %s report", reportType)).
		WithOutcome("success").
		WithMetadata("report_type", reportType).
		WithCorrelation(a.correlationID).
		LogTo(ctx, a.logger)
}

// LogRemediationAction logs remediation actions.
func (a *ComplianceAuditor) LogRemediationAction(ctx context.Context, remediationID, action, status string) error {
	var eventType EventType
	switch action {
	case "create":
		eventType = EventRemediationCreated
	case "update":
		eventType = EventRemediationUpdated
	case "close":
		eventType = EventRemediationClosed
	default:
		eventType = EventUserAction
	}

	return NewAuditEvent(eventType).
		WithActor(a.defaultActor).
		WithAction(action + "_remediation").
		WithResource("remediation", remediationID).
		WithDescription(fmt.Sprintf("Remediation %s: %s", remediationID, action)).
		WithOutcome(status).
		WithCorrelation(a.correlationID).
		LogTo(ctx, a.logger)
}

// LogError logs an error event.
func (a *ComplianceAuditor) LogError(ctx context.Context, resource, resourceID string, err error) error {
	return NewAuditEvent(EventError).
		WithSeverity(SeverityError).
		WithActor(a.defaultActor).
		WithAction("error").
		WithResource(resource, resourceID).
		WithDescription(err.Error()).
		WithOutcome("failure").
		WithCorrelation(a.correlationID).
		LogTo(ctx, a.logger)
}
