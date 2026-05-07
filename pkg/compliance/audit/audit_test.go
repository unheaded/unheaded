// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package audit

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// InMemoryAuditLogger — Log
// ---------------------------------------------------------------------------

func TestInMemoryAuditLogger_Log_AssignsIDAndTimestamp(t *testing.T) {
	logger := NewInMemoryAuditLogger(100)
	event := &AuditEvent{Type: EventControlChecked, Action: "check"}

	if err := logger.Log(context.Background(), event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if event.ID == "" {
		t.Error("expected ID to be assigned")
	}
	if !strings.HasPrefix(event.ID, "AE-") {
		t.Errorf("expected ID prefix AE-, got %s", event.ID)
	}
	if event.Timestamp.IsZero() {
		t.Error("expected timestamp to be assigned")
	}
}

func TestInMemoryAuditLogger_Log_PreservesExistingIDAndTimestamp(t *testing.T) {
	logger := NewInMemoryAuditLogger(100)
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	event := &AuditEvent{
		ID:        "custom-id",
		Timestamp: ts,
		Type:      EventUserAction,
	}

	if err := logger.Log(context.Background(), event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if event.ID != "custom-id" {
		t.Errorf("expected ID custom-id, got %s", event.ID)
	}
	if !event.Timestamp.Equal(ts) {
		t.Errorf("expected timestamp to be preserved")
	}
}

func TestInMemoryAuditLogger_Log_TrimsToMaxSize(t *testing.T) {
	maxSize := 3
	logger := NewInMemoryAuditLogger(maxSize)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		_ = logger.Log(ctx, &AuditEvent{Type: EventUserAction, Action: "a"})
	}

	events, _ := logger.Query(ctx, AuditFilter{})
	if len(events) != maxSize {
		t.Errorf("expected %d events after trim, got %d", maxSize, len(events))
	}
}

func TestInMemoryAuditLogger_Log_ConcurrentSafety(t *testing.T) {
	logger := NewInMemoryAuditLogger(1000)
	ctx := context.Background()
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = logger.Log(ctx, &AuditEvent{Type: EventUserAction})
		}()
	}
	wg.Wait()

	events, _ := logger.Query(ctx, AuditFilter{})
	if len(events) != 50 {
		t.Errorf("expected 50 events, got %d", len(events))
	}
}

// ---------------------------------------------------------------------------
// InMemoryAuditLogger — Query with filters
// ---------------------------------------------------------------------------

func seedLogger(t *testing.T) *InMemoryAuditLogger {
	t.Helper()
	logger := NewInMemoryAuditLogger(100)
	ctx := context.Background()

	now := time.Now()
	events := []*AuditEvent{
		{ID: "e1", Timestamp: now.Add(-3 * time.Hour), Type: EventControlChecked, Severity: SeverityInfo, Actor: "alice", Resource: "ctrl", ResourceID: "C1", CorrelationID: "corr-1"},
		{ID: "e2", Timestamp: now.Add(-2 * time.Hour), Type: EventEvidenceCollected, Severity: SeverityWarning, Actor: "bob", Resource: "evidence", ResourceID: "EV1", CorrelationID: "corr-1"},
		{ID: "e3", Timestamp: now.Add(-1 * time.Hour), Type: EventReportGenerated, Severity: SeverityError, Actor: "alice", Resource: "report", ResourceID: "R1", CorrelationID: "corr-2"},
		{ID: "e4", Timestamp: now, Type: EventError, Severity: SeverityCritical, Actor: "system", Resource: "ctrl", ResourceID: "C2", CorrelationID: "corr-2"},
	}

	for _, e := range events {
		if err := logger.Log(ctx, e); err != nil {
			t.Fatalf("seed log error: %v", err)
		}
	}
	return logger
}

func TestQuery_NoFilter_ReturnsAll(t *testing.T) {
	logger := seedLogger(t)
	events, err := logger.Query(context.Background(), AuditFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 4 {
		t.Errorf("expected 4 events, got %d", len(events))
	}
}

func TestQuery_FilterByType(t *testing.T) {
	logger := seedLogger(t)
	events, err := logger.Query(context.Background(), AuditFilter{
		Types: []EventType{EventControlChecked, EventError},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}
}

func TestQuery_FilterBySeverity(t *testing.T) {
	logger := seedLogger(t)
	events, err := logger.Query(context.Background(), AuditFilter{
		Severities: []Severity{SeverityError, SeverityCritical},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}
}

func TestQuery_FilterByActor(t *testing.T) {
	logger := seedLogger(t)
	events, err := logger.Query(context.Background(), AuditFilter{
		Actors: []string{"alice"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}
}

func TestQuery_FilterByResource(t *testing.T) {
	logger := seedLogger(t)
	events, err := logger.Query(context.Background(), AuditFilter{
		Resources: []string{"ctrl"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}
}

func TestQuery_FilterByResourceID(t *testing.T) {
	logger := seedLogger(t)
	events, err := logger.Query(context.Background(), AuditFilter{
		ResourceIDs: []string{"C1"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
}

func TestQuery_FilterByCorrelationID(t *testing.T) {
	logger := seedLogger(t)
	events, err := logger.Query(context.Background(), AuditFilter{
		CorrelationID: "corr-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}
}

func TestQuery_FilterByTimeRange(t *testing.T) {
	logger := seedLogger(t)
	now := time.Now()
	start := now.Add(-2*time.Hour - 30*time.Minute)
	end := now.Add(-30 * time.Minute)

	events, err := logger.Query(context.Background(), AuditFilter{
		StartTime: &start,
		EndTime:   &end,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}
}

func TestQuery_LimitAndOffset(t *testing.T) {
	logger := seedLogger(t)

	// Limit only
	events, _ := logger.Query(context.Background(), AuditFilter{Limit: 2})
	if len(events) != 2 {
		t.Errorf("expected 2 events with limit, got %d", len(events))
	}

	// Offset + limit
	events, _ = logger.Query(context.Background(), AuditFilter{Offset: 1, Limit: 2})
	if len(events) != 2 {
		t.Errorf("expected 2 events with offset+limit, got %d", len(events))
	}

	// Offset beyond length
	events, _ = logger.Query(context.Background(), AuditFilter{Offset: 100})
	if len(events) != 4 {
		// offset >= len(results) means no slicing occurs
		t.Errorf("expected 4 events with large offset, got %d", len(events))
	}
}

// ---------------------------------------------------------------------------
// InMemoryAuditLogger — Export
// ---------------------------------------------------------------------------

func TestExport_ReturnsValidJSON(t *testing.T) {
	logger := seedLogger(t)
	data, err := logger.Export(context.Background(), AuditFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var events []*AuditEvent
	if err := json.Unmarshal(data, &events); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(events) != 4 {
		t.Errorf("expected 4 events in export, got %d", len(events))
	}
}

func TestExport_WithFilter(t *testing.T) {
	logger := seedLogger(t)
	data, err := logger.Export(context.Background(), AuditFilter{
		Types: []EventType{EventError},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var events []*AuditEvent
	if err := json.Unmarshal(data, &events); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event in filtered export, got %d", len(events))
	}
}

// ---------------------------------------------------------------------------
// AuditLogBuilder
// ---------------------------------------------------------------------------

func TestAuditLogBuilder_Build(t *testing.T) {
	event := NewAuditEvent(EventAssessmentStarted).
		WithSeverity(SeverityWarning).
		WithActor("alice").
		WithAction("start").
		WithResource("assessment", "A1").
		WithDescription("Starting assessment").
		WithOutcome("in_progress").
		WithMetadata("standard", "SOC2").
		WithClientInfo("10.0.0.1", "TestAgent/1.0").
		WithSession("sess-123").
		WithCorrelation("corr-abc").
		Build()

	if event.Type != EventAssessmentStarted {
		t.Errorf("expected type %s, got %s", EventAssessmentStarted, event.Type)
	}
	if event.Severity != SeverityWarning {
		t.Errorf("expected severity %s, got %s", SeverityWarning, event.Severity)
	}
	if event.Actor != "alice" {
		t.Errorf("expected actor alice, got %s", event.Actor)
	}
	if event.Action != "start" {
		t.Errorf("expected action start, got %s", event.Action)
	}
	if event.Resource != "assessment" {
		t.Errorf("expected resource assessment, got %s", event.Resource)
	}
	if event.ResourceID != "A1" {
		t.Errorf("expected resource_id A1, got %s", event.ResourceID)
	}
	if event.Description != "Starting assessment" {
		t.Errorf("unexpected description: %s", event.Description)
	}
	if event.Outcome != "in_progress" {
		t.Errorf("unexpected outcome: %s", event.Outcome)
	}
	if event.Metadata["standard"] != "SOC2" {
		t.Errorf("expected metadata standard=SOC2, got %v", event.Metadata["standard"])
	}
	if event.ClientIP != "10.0.0.1" {
		t.Errorf("unexpected client IP: %s", event.ClientIP)
	}
	if event.UserAgent != "TestAgent/1.0" {
		t.Errorf("unexpected user agent: %s", event.UserAgent)
	}
	if event.SessionID != "sess-123" {
		t.Errorf("unexpected session ID: %s", event.SessionID)
	}
	if event.CorrelationID != "corr-abc" {
		t.Errorf("unexpected correlation ID: %s", event.CorrelationID)
	}
	if event.ID == "" {
		t.Error("expected auto-generated ID")
	}
	if event.Timestamp.IsZero() {
		t.Error("expected auto-generated timestamp")
	}
}

func TestAuditLogBuilder_LogTo(t *testing.T) {
	logger := NewInMemoryAuditLogger(10)
	ctx := context.Background()

	err := NewAuditEvent(EventUserAction).
		WithActor("bob").
		WithAction("click").
		LogTo(ctx, logger)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events, _ := logger.Query(ctx, AuditFilter{Actors: []string{"bob"}})
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
}

// ---------------------------------------------------------------------------
// ComplianceAuditor
// ---------------------------------------------------------------------------

func newTestAuditor() (*ComplianceAuditor, *InMemoryAuditLogger) {
	logger := NewInMemoryAuditLogger(100)
	auditor := NewComplianceAuditor(logger).
		WithActor("test-user").
		WithCorrelation("test-corr")
	return auditor, logger
}

func TestComplianceAuditor_LogAssessmentStart(t *testing.T) {
	auditor, logger := newTestAuditor()
	ctx := context.Background()

	err := auditor.LogAssessmentStart(ctx, "SOC2", "assessment-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events, _ := logger.Query(ctx, AuditFilter{Types: []EventType{EventAssessmentStarted}})
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	e := events[0]
	if e.Actor != "test-user" {
		t.Errorf("expected actor test-user, got %s", e.Actor)
	}
	if e.ResourceID != "assessment-1" {
		t.Errorf("expected resource_id assessment-1, got %s", e.ResourceID)
	}
	if e.CorrelationID != "test-corr" {
		t.Errorf("expected correlation test-corr, got %s", e.CorrelationID)
	}
	if !strings.Contains(e.Description, "SOC2") {
		t.Errorf("expected description to contain SOC2, got %s", e.Description)
	}
	if e.Metadata["standard"] != "SOC2" {
		t.Errorf("expected metadata standard=SOC2")
	}
}

func TestComplianceAuditor_LogAssessmentComplete(t *testing.T) {
	auditor, logger := newTestAuditor()
	ctx := context.Background()

	result := map[string]interface{}{
		"score":  95.5,
		"passed": true,
	}
	err := auditor.LogAssessmentComplete(ctx, "assessment-1", result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events, _ := logger.Query(ctx, AuditFilter{Types: []EventType{EventAssessmentCompleted}})
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	e := events[0]
	if e.Metadata["score"] != 95.5 {
		t.Errorf("expected metadata score=95.5, got %v", e.Metadata["score"])
	}
	if e.Metadata["passed"] != true {
		t.Errorf("expected metadata passed=true, got %v", e.Metadata["passed"])
	}
}

func TestComplianceAuditor_LogControlCheck(t *testing.T) {
	auditor, logger := newTestAuditor()
	ctx := context.Background()

	err := auditor.LogControlCheck(ctx, "CTRL-001", "pass", 0.95)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events, _ := logger.Query(ctx, AuditFilter{Types: []EventType{EventControlChecked}})
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	e := events[0]
	if e.ResourceID != "CTRL-001" {
		t.Errorf("expected resource_id CTRL-001, got %s", e.ResourceID)
	}
	if e.Outcome != "pass" {
		t.Errorf("expected outcome pass, got %s", e.Outcome)
	}
	if e.Metadata["score"] != 0.95 {
		t.Errorf("expected metadata score=0.95, got %v", e.Metadata["score"])
	}
}

func TestComplianceAuditor_LogEvidenceCollection(t *testing.T) {
	auditor, logger := newTestAuditor()
	ctx := context.Background()

	err := auditor.LogEvidenceCollection(ctx, "CTRL-001", "EV-001", "screenshot")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events, _ := logger.Query(ctx, AuditFilter{Types: []EventType{EventEvidenceCollected}})
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	e := events[0]
	if e.ResourceID != "EV-001" {
		t.Errorf("expected resource_id EV-001, got %s", e.ResourceID)
	}
	if e.Metadata["control_id"] != "CTRL-001" {
		t.Errorf("expected metadata control_id=CTRL-001")
	}
	if e.Metadata["evidence_type"] != "screenshot" {
		t.Errorf("expected metadata evidence_type=screenshot")
	}
}

func TestComplianceAuditor_LogReportGeneration(t *testing.T) {
	auditor, logger := newTestAuditor()
	ctx := context.Background()

	err := auditor.LogReportGeneration(ctx, "RPT-001", "executive_summary")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events, _ := logger.Query(ctx, AuditFilter{Types: []EventType{EventReportGenerated}})
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	e := events[0]
	if e.ResourceID != "RPT-001" {
		t.Errorf("expected resource_id RPT-001, got %s", e.ResourceID)
	}
	if e.Metadata["report_type"] != "executive_summary" {
		t.Errorf("expected metadata report_type=executive_summary")
	}
}

func TestComplianceAuditor_LogRemediationAction(t *testing.T) {
	tests := []struct {
		action       string
		expectedType EventType
	}{
		{"create", EventRemediationCreated},
		{"update", EventRemediationUpdated},
		{"close", EventRemediationClosed},
		{"unknown", EventUserAction},
	}

	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			auditor, logger := newTestAuditor()
			ctx := context.Background()

			err := auditor.LogRemediationAction(ctx, "REM-001", tt.action, "success")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			events, _ := logger.Query(ctx, AuditFilter{Types: []EventType{tt.expectedType}})
			if len(events) != 1 {
				t.Fatalf("expected 1 event of type %s, got %d", tt.expectedType, len(events))
			}

			e := events[0]
			if e.Outcome != "success" {
				t.Errorf("expected outcome success, got %s", e.Outcome)
			}
		})
	}
}

func TestComplianceAuditor_LogError(t *testing.T) {
	auditor, logger := newTestAuditor()
	ctx := context.Background()

	err := auditor.LogError(ctx, "assessment", "A1", errors.New("something went wrong"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events, _ := logger.Query(ctx, AuditFilter{Types: []EventType{EventError}})
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	e := events[0]
	if e.Severity != SeverityError {
		t.Errorf("expected severity error, got %s", e.Severity)
	}
	if e.Description != "something went wrong" {
		t.Errorf("expected description 'something went wrong', got %s", e.Description)
	}
	if e.Outcome != "failure" {
		t.Errorf("expected outcome failure, got %s", e.Outcome)
	}
}

// ---------------------------------------------------------------------------
// AuditEvent JSON round-trip
// ---------------------------------------------------------------------------

func TestAuditEvent_JSONRoundTrip(t *testing.T) {
	event := NewAuditEvent(EventControlChecked).
		WithSeverity(SeverityWarning).
		WithActor("alice").
		WithAction("check").
		WithResource("control", "C1").
		WithDescription("Checking control C1").
		WithOutcome("pass").
		WithMetadata("score", 0.9).
		WithClientInfo("10.0.0.1", "TestAgent").
		WithSession("sess-1").
		WithCorrelation("corr-1").
		Build()

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded AuditEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.ID != event.ID {
		t.Errorf("ID mismatch: %s vs %s", decoded.ID, event.ID)
	}
	if decoded.Type != event.Type {
		t.Errorf("Type mismatch: %s vs %s", decoded.Type, event.Type)
	}
	if decoded.Actor != event.Actor {
		t.Errorf("Actor mismatch")
	}
	if decoded.CorrelationID != event.CorrelationID {
		t.Errorf("CorrelationID mismatch")
	}
}
