// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"unheaded/pkg/audit"
)

// --- helpers ---

func makeEvent(id string, ts time.Time, eventType audit.EventType, sev audit.Severity, action string, outcome audit.Outcome) *audit.AuditEvent {
	return &audit.AuditEvent{
		ID:        id,
		Timestamp: ts,
		Type:      eventType,
		Severity:  sev,
		Action:    action,
		Outcome:   outcome,
		Actor:     &audit.Actor{ID: "actor-1", Type: "user", Name: "Alice", Roles: []string{"admin"}},
		Resource:  &audit.Resource{ID: "res-1", Type: "document", Name: "report.pdf", Path: "/docs/report.pdf"},
		Source:    &audit.Source{IP: "10.0.0.1", Service: "gateway", RequestID: "req-1", UserAgent: "test-agent"},
		Hash:      "hash-" + id,
	}
}

func makeMinimalEvent(id string) *audit.AuditEvent {
	return &audit.AuditEvent{
		ID:        id,
		Timestamp: time.Now().UTC(),
		Type:      audit.EventTypeSystem,
		Severity:  audit.SeverityInfo,
		Action:    "test",
		Outcome:   audit.OutcomeSuccess,
		Actor:     &audit.Actor{ID: "actor-1"},
		Hash:      "hash-" + id,
	}
}

// ===================== MemoryStorage Tests =====================

func TestNewMemoryStorage(t *testing.T) {
	s := NewMemoryStorage()
	if s == nil {
		t.Fatal("NewMemoryStorage returned nil")
	}
	if len(s.events) != 0 {
		t.Errorf("expected empty events, got %d", len(s.events))
	}
	if len(s.byID) != 0 {
		t.Errorf("expected empty byID, got %d", len(s.byID))
	}
}

func TestMemoryStorage_Store(t *testing.T) {
	s := NewMemoryStorage()
	ctx := context.Background()
	ev := makeMinimalEvent("e1")

	if err := s.Store(ctx, ev); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Duplicate should fail
	if err := s.Store(ctx, ev); err == nil {
		t.Fatal("expected error on duplicate Store, got nil")
	}
}

func TestMemoryStorage_StoreBatch(t *testing.T) {
	s := NewMemoryStorage()
	ctx := context.Background()

	events := []*audit.AuditEvent{
		makeMinimalEvent("e1"),
		makeMinimalEvent("e2"),
		makeMinimalEvent("e3"),
	}

	if err := s.StoreBatch(ctx, events); err != nil {
		t.Fatalf("StoreBatch failed: %v", err)
	}

	count, _ := s.Count(ctx, &audit.AuditQuery{})
	if count != 3 {
		t.Errorf("expected 3 events, got %d", count)
	}

	// Duplicate in batch should fail
	if err := s.StoreBatch(ctx, events[:1]); err == nil {
		t.Fatal("expected error on duplicate StoreBatch")
	}
}

func TestMemoryStorage_Get(t *testing.T) {
	s := NewMemoryStorage()
	ctx := context.Background()
	ev := makeMinimalEvent("e1")

	_ = s.Store(ctx, ev)

	got, err := s.Get(ctx, "e1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.ID != "e1" {
		t.Errorf("expected ID e1, got %s", got.ID)
	}

	// Not found
	_, err = s.Get(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent Get")
	}
}

func TestMemoryStorage_GetLastHash(t *testing.T) {
	s := NewMemoryStorage()
	ctx := context.Background()

	hash, err := s.GetLastHash(ctx)
	if err != nil {
		t.Fatalf("GetLastHash failed: %v", err)
	}
	if hash != "" {
		t.Errorf("expected empty hash, got %s", hash)
	}

	ev := makeMinimalEvent("e1")
	ev.Hash = "abc123"
	_ = s.Store(ctx, ev)

	hash, err = s.GetLastHash(ctx)
	if err != nil {
		t.Fatalf("GetLastHash failed: %v", err)
	}
	if hash != "abc123" {
		t.Errorf("expected abc123, got %s", hash)
	}
}

func TestMemoryStorage_Query_AllEvents(t *testing.T) {
	s := NewMemoryStorage()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		_ = s.Store(ctx, makeMinimalEvent(fmt.Sprintf("e%d", i)))
	}

	events, err := s.Query(ctx, &audit.AuditQuery{})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(events) != 5 {
		t.Errorf("expected 5 events, got %d", len(events))
	}
}

func TestMemoryStorage_Query_ByTimeRange(t *testing.T) {
	s := NewMemoryStorage()
	ctx := context.Background()

	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		ev := makeEvent(fmt.Sprintf("e%d", i), base.Add(time.Duration(i)*time.Hour), audit.EventTypeSystem, audit.SeverityInfo, "test", audit.OutcomeSuccess)
		_ = s.Store(ctx, ev)
	}

	events, err := s.Query(ctx, &audit.AuditQuery{
		StartTime: base.Add(1 * time.Hour),
		EndTime:   base.Add(3 * time.Hour),
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(events) != 3 {
		t.Errorf("expected 3 events in range, got %d", len(events))
	}
}

func TestMemoryStorage_Query_ByTypes(t *testing.T) {
	s := NewMemoryStorage()
	ctx := context.Background()

	ts := time.Now().UTC()
	_ = s.Store(ctx, makeEvent("e1", ts, audit.EventTypeAuthentication, audit.SeverityInfo, "login", audit.OutcomeSuccess))
	_ = s.Store(ctx, makeEvent("e2", ts, audit.EventTypeSystem, audit.SeverityInfo, "start", audit.OutcomeSuccess))
	_ = s.Store(ctx, makeEvent("e3", ts, audit.EventTypeAuthentication, audit.SeverityInfo, "logout", audit.OutcomeSuccess))

	events, err := s.Query(ctx, &audit.AuditQuery{
		Types: []audit.EventType{audit.EventTypeAuthentication},
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 authentication events, got %d", len(events))
	}
}

func TestMemoryStorage_Query_BySeverity(t *testing.T) {
	s := NewMemoryStorage()
	ctx := context.Background()
	ts := time.Now().UTC()

	_ = s.Store(ctx, makeEvent("e1", ts, audit.EventTypeSystem, audit.SeverityInfo, "test", audit.OutcomeSuccess))
	_ = s.Store(ctx, makeEvent("e2", ts, audit.EventTypeSystem, audit.SeverityCritical, "test", audit.OutcomeSuccess))
	_ = s.Store(ctx, makeEvent("e3", ts, audit.EventTypeSystem, audit.SeverityWarning, "test", audit.OutcomeSuccess))

	events, err := s.Query(ctx, &audit.AuditQuery{
		Severities: []audit.Severity{audit.SeverityCritical, audit.SeverityWarning},
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}
}

func TestMemoryStorage_Query_ByActorIDs(t *testing.T) {
	s := NewMemoryStorage()
	ctx := context.Background()

	ev1 := makeMinimalEvent("e1")
	ev1.Actor = &audit.Actor{ID: "alice"}
	ev2 := makeMinimalEvent("e2")
	ev2.Actor = &audit.Actor{ID: "bob"}
	ev3 := makeMinimalEvent("e3")
	ev3.Actor = &audit.Actor{ID: "alice"}

	_ = s.Store(ctx, ev1)
	_ = s.Store(ctx, ev2)
	_ = s.Store(ctx, ev3)

	events, err := s.Query(ctx, &audit.AuditQuery{
		ActorIDs: []string{"alice"},
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2, got %d", len(events))
	}
}

func TestMemoryStorage_Query_ByActions(t *testing.T) {
	s := NewMemoryStorage()
	ctx := context.Background()
	ts := time.Now().UTC()

	_ = s.Store(ctx, makeEvent("e1", ts, audit.EventTypeSystem, audit.SeverityInfo, "login", audit.OutcomeSuccess))
	_ = s.Store(ctx, makeEvent("e2", ts, audit.EventTypeSystem, audit.SeverityInfo, "logout", audit.OutcomeSuccess))
	_ = s.Store(ctx, makeEvent("e3", ts, audit.EventTypeSystem, audit.SeverityInfo, "login", audit.OutcomeFailure))

	events, err := s.Query(ctx, &audit.AuditQuery{
		Actions: []string{"login"},
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2, got %d", len(events))
	}
}

func TestMemoryStorage_Query_ByOutcomes(t *testing.T) {
	s := NewMemoryStorage()
	ctx := context.Background()
	ts := time.Now().UTC()

	_ = s.Store(ctx, makeEvent("e1", ts, audit.EventTypeSystem, audit.SeverityInfo, "test", audit.OutcomeSuccess))
	_ = s.Store(ctx, makeEvent("e2", ts, audit.EventTypeSystem, audit.SeverityInfo, "test", audit.OutcomeFailure))

	events, err := s.Query(ctx, &audit.AuditQuery{
		Outcomes: []audit.Outcome{audit.OutcomeFailure},
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1, got %d", len(events))
	}
}

func TestMemoryStorage_Query_ByResourceTypes(t *testing.T) {
	s := NewMemoryStorage()
	ctx := context.Background()

	ev1 := makeMinimalEvent("e1")
	ev1.Resource = &audit.Resource{ID: "r1", Type: "document"}
	ev2 := makeMinimalEvent("e2")
	ev2.Resource = &audit.Resource{ID: "r2", Type: "config"}
	ev3 := makeMinimalEvent("e3")
	ev3.Resource = &audit.Resource{ID: "r3", Type: "document"}

	_ = s.Store(ctx, ev1)
	_ = s.Store(ctx, ev2)
	_ = s.Store(ctx, ev3)

	events, err := s.Query(ctx, &audit.AuditQuery{
		ResourceTypes: []string{"document"},
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2, got %d", len(events))
	}
}

func TestMemoryStorage_Query_Pagination(t *testing.T) {
	s := NewMemoryStorage()
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		_ = s.Store(ctx, makeMinimalEvent(fmt.Sprintf("e%d", i)))
	}

	events, err := s.Query(ctx, &audit.AuditQuery{Limit: 3, Offset: 2})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(events) != 3 {
		t.Errorf("expected 3, got %d", len(events))
	}

	// Offset past end
	events, err = s.Query(ctx, &audit.AuditQuery{Offset: 100})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0, got %d", len(events))
	}
}

func TestMemoryStorage_Query_OrderByTimestamp(t *testing.T) {
	s := NewMemoryStorage()
	ctx := context.Background()
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	for i := 0; i < 5; i++ {
		ev := makeEvent(fmt.Sprintf("e%d", i), base.Add(time.Duration(i)*time.Hour), audit.EventTypeSystem, audit.SeverityInfo, "test", audit.OutcomeSuccess)
		_ = s.Store(ctx, ev)
	}

	// Ascending
	events, _ := s.Query(ctx, &audit.AuditQuery{OrderBy: "timestamp", Descending: false})
	for i := 1; i < len(events); i++ {
		if events[i].Timestamp.Before(events[i-1].Timestamp) {
			t.Errorf("events not in ascending order at index %d", i)
		}
	}

	// Descending
	events, _ = s.Query(ctx, &audit.AuditQuery{OrderBy: "timestamp", Descending: true})
	for i := 1; i < len(events); i++ {
		if events[i].Timestamp.After(events[i-1].Timestamp) {
			t.Errorf("events not in descending order at index %d", i)
		}
	}
}

func TestMemoryStorage_Query_OrderBySeverity(t *testing.T) {
	s := NewMemoryStorage()
	ctx := context.Background()
	ts := time.Now().UTC()

	_ = s.Store(ctx, makeEvent("e1", ts, audit.EventTypeSystem, audit.SeverityWarning, "test", audit.OutcomeSuccess))
	_ = s.Store(ctx, makeEvent("e2", ts, audit.EventTypeSystem, audit.SeverityCritical, "test", audit.OutcomeSuccess))
	_ = s.Store(ctx, makeEvent("e3", ts, audit.EventTypeSystem, audit.SeverityInfo, "test", audit.OutcomeSuccess))

	events, _ := s.Query(ctx, &audit.AuditQuery{OrderBy: "severity", Descending: true})
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if events[0].Severity != audit.SeverityCritical {
		t.Errorf("expected critical first, got %s", events[0].Severity)
	}
}

func TestMemoryStorage_Count(t *testing.T) {
	s := NewMemoryStorage()
	ctx := context.Background()
	ts := time.Now().UTC()

	_ = s.Store(ctx, makeEvent("e1", ts, audit.EventTypeAuthentication, audit.SeverityInfo, "login", audit.OutcomeSuccess))
	_ = s.Store(ctx, makeEvent("e2", ts, audit.EventTypeSystem, audit.SeverityInfo, "start", audit.OutcomeSuccess))

	total, _ := s.Count(ctx, &audit.AuditQuery{})
	if total != 2 {
		t.Errorf("expected 2, got %d", total)
	}

	auth, _ := s.Count(ctx, &audit.AuditQuery{Types: []audit.EventType{audit.EventTypeAuthentication}})
	if auth != 1 {
		t.Errorf("expected 1 auth event, got %d", auth)
	}
}

func TestMemoryStorage_Close(t *testing.T) {
	s := NewMemoryStorage()
	if err := s.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if s.events != nil {
		t.Error("expected events to be nil after Close")
	}
}

func TestSeverityOrder(t *testing.T) {
	tests := []struct {
		sev      audit.Severity
		expected int
	}{
		{audit.SeverityInfo, 1},
		{audit.SeverityWarning, 2},
		{audit.SeverityError, 3},
		{audit.SeverityCritical, 4},
		{audit.Severity("unknown"), 0},
	}
	for _, tt := range tests {
		got := severityOrder(tt.sev)
		if got != tt.expected {
			t.Errorf("severityOrder(%s) = %d, want %d", tt.sev, got, tt.expected)
		}
	}
}

// ===================== RetentionCleaner Tests =====================

func TestRetentionCleaner_StartStop(t *testing.T) {
	s := NewMemoryStorage()
	cleaner := NewRetentionCleaner(s, 30, 100*time.Millisecond)
	cleaner.Start()
	time.Sleep(250 * time.Millisecond)
	cleaner.Stop()
	// If it doesn't hang, the test passes
}

// ===================== FileStorage Tests =====================

func TestNewFileStorage(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStorage(&FileStorageConfig{
		BaseDir:       dir,
		MaxFileSize:   1024 * 1024,
		RotateDaily:   false,
		RetentionDays: 30,
	})
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}
	defer fs.Close()

	if fs.baseDir != dir {
		t.Errorf("expected baseDir %s, got %s", dir, fs.baseDir)
	}
}

func TestNewFileStorage_MissingBaseDir(t *testing.T) {
	_, err := NewFileStorage(&FileStorageConfig{BaseDir: ""})
	if err == nil {
		t.Fatal("expected error for empty BaseDir")
	}
}

func TestNewFileStorage_Defaults(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStorage(&FileStorageConfig{
		BaseDir: dir,
	})
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}
	defer fs.Close()

	if fs.maxFileSize != 100*1024*1024 {
		t.Errorf("expected default max file size 100MB, got %d", fs.maxFileSize)
	}
	if fs.retentionDays != 90 {
		t.Errorf("expected default retention 90 days, got %d", fs.retentionDays)
	}
}

func TestFileStorage_StoreAndGet(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStorage(&FileStorageConfig{BaseDir: dir})
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}
	defer fs.Close()

	ctx := context.Background()
	ev := makeEvent("fs-e1", time.Now().UTC(), audit.EventTypeSystem, audit.SeverityInfo, "test", audit.OutcomeSuccess)

	if err := fs.Store(ctx, ev); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	got, err := fs.Get(ctx, "fs-e1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.ID != "fs-e1" {
		t.Errorf("expected ID fs-e1, got %s", got.ID)
	}
}

func TestFileStorage_GetNotFound(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStorage(&FileStorageConfig{BaseDir: dir})
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}
	defer fs.Close()

	_, err = fs.Get(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent event")
	}
}

func TestFileStorage_StoreBatch(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStorage(&FileStorageConfig{BaseDir: dir})
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}
	defer fs.Close()

	ctx := context.Background()
	events := []*audit.AuditEvent{
		makeEvent("b1", time.Now().UTC(), audit.EventTypeSystem, audit.SeverityInfo, "test1", audit.OutcomeSuccess),
		makeEvent("b2", time.Now().UTC(), audit.EventTypeSystem, audit.SeverityInfo, "test2", audit.OutcomeSuccess),
	}

	if err := fs.StoreBatch(ctx, events); err != nil {
		t.Fatalf("StoreBatch failed: %v", err)
	}

	count, _ := fs.Count(ctx, &audit.AuditQuery{})
	if count != 2 {
		t.Errorf("expected count 2, got %d", count)
	}
}

func TestFileStorage_Query(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStorage(&FileStorageConfig{BaseDir: dir})
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}
	defer fs.Close()

	ctx := context.Background()
	base := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	_ = fs.Store(ctx, makeEvent("q1", base, audit.EventTypeAuthentication, audit.SeverityInfo, "login", audit.OutcomeSuccess))
	_ = fs.Store(ctx, makeEvent("q2", base.Add(time.Hour), audit.EventTypeSystem, audit.SeverityWarning, "start", audit.OutcomeSuccess))
	_ = fs.Store(ctx, makeEvent("q3", base.Add(2*time.Hour), audit.EventTypeAuthentication, audit.SeverityInfo, "logout", audit.OutcomeSuccess))

	// Query all
	events, err := fs.Query(ctx, &audit.AuditQuery{})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(events) != 3 {
		t.Errorf("expected 3 events, got %d", len(events))
	}

	// Query by type
	events, err = fs.Query(ctx, &audit.AuditQuery{
		Types: []audit.EventType{audit.EventTypeAuthentication},
	})
	if err != nil {
		t.Fatalf("Query by type failed: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 auth events, got %d", len(events))
	}

	// Query by time range
	events, err = fs.Query(ctx, &audit.AuditQuery{
		StartTime: base.Add(30 * time.Minute),
		EndTime:   base.Add(3 * time.Hour),
	})
	if err != nil {
		t.Fatalf("Query by time failed: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 events in range, got %d", len(events))
	}
}

func TestFileStorage_Query_Pagination(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStorage(&FileStorageConfig{BaseDir: dir})
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}
	defer fs.Close()

	ctx := context.Background()
	base := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 10; i++ {
		_ = fs.Store(ctx, makeEvent(fmt.Sprintf("p%d", i), base.Add(time.Duration(i)*time.Minute), audit.EventTypeSystem, audit.SeverityInfo, "test", audit.OutcomeSuccess))
	}

	events, _ := fs.Query(ctx, &audit.AuditQuery{Limit: 3})
	if len(events) != 3 {
		t.Errorf("expected 3, got %d", len(events))
	}

	events, _ = fs.Query(ctx, &audit.AuditQuery{Offset: 7})
	if len(events) != 3 {
		t.Errorf("expected 3, got %d", len(events))
	}
}

func TestFileStorage_Query_Descending(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStorage(&FileStorageConfig{BaseDir: dir})
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}
	defer fs.Close()

	ctx := context.Background()
	base := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		_ = fs.Store(ctx, makeEvent(fmt.Sprintf("d%d", i), base.Add(time.Duration(i)*time.Hour), audit.EventTypeSystem, audit.SeverityInfo, "test", audit.OutcomeSuccess))
	}

	events, _ := fs.Query(ctx, &audit.AuditQuery{Descending: true})
	for i := 1; i < len(events); i++ {
		if events[i].Timestamp.After(events[i-1].Timestamp) {
			t.Errorf("events not in descending order at index %d", i)
		}
	}
}

func TestFileStorage_Query_ByActorIDs(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStorage(&FileStorageConfig{BaseDir: dir})
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}
	defer fs.Close()

	ctx := context.Background()
	ts := time.Now().UTC()
	ev1 := makeEvent("a1", ts, audit.EventTypeSystem, audit.SeverityInfo, "test", audit.OutcomeSuccess)
	ev1.Actor = &audit.Actor{ID: "alice"}
	ev2 := makeEvent("a2", ts, audit.EventTypeSystem, audit.SeverityInfo, "test", audit.OutcomeSuccess)
	ev2.Actor = &audit.Actor{ID: "bob"}

	_ = fs.Store(ctx, ev1)
	_ = fs.Store(ctx, ev2)

	events, _ := fs.Query(ctx, &audit.AuditQuery{ActorIDs: []string{"alice"}})
	if len(events) != 1 {
		t.Errorf("expected 1, got %d", len(events))
	}
}

func TestFileStorage_GetLastHash(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStorage(&FileStorageConfig{BaseDir: dir})
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}
	defer fs.Close()

	ctx := context.Background()
	hash, _ := fs.GetLastHash(ctx)
	if hash != "" {
		t.Errorf("expected empty hash initially, got %s", hash)
	}

	ev := makeMinimalEvent("h1")
	ev.Hash = "myhash"
	_ = fs.Store(ctx, ev)

	hash, _ = fs.GetLastHash(ctx)
	if hash != "myhash" {
		t.Errorf("expected myhash, got %s", hash)
	}
}

func TestFileStorage_Count(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStorage(&FileStorageConfig{BaseDir: dir})
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}
	defer fs.Close()

	ctx := context.Background()
	_ = fs.Store(ctx, makeMinimalEvent("c1"))
	_ = fs.Store(ctx, makeMinimalEvent("c2"))

	count, err := fs.Count(ctx, &audit.AuditQuery{})
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2, got %d", count)
	}
}

func TestFileStorage_Rotate(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStorage(&FileStorageConfig{
		BaseDir:     dir,
		MaxFileSize: 50, // very small to trigger rotation
	})
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}
	defer fs.Close()

	ctx := context.Background()

	// Store events to trigger rotation
	for i := 0; i < 5; i++ {
		ev := makeEvent(fmt.Sprintf("rot%d", i), time.Now().UTC(), audit.EventTypeSystem, audit.SeverityInfo, "test-rotation", audit.OutcomeSuccess)
		_ = fs.Store(ctx, ev)
	}

	// Verify files were created
	files, _ := filepath.Glob(filepath.Join(dir, "audit-*.log"))
	if len(files) < 1 {
		t.Error("expected at least one log file")
	}
}

func TestFileStorage_ManualRotate(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStorage(&FileStorageConfig{BaseDir: dir})
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}
	defer fs.Close()

	if err := fs.Rotate(); err != nil {
		t.Fatalf("Rotate failed: %v", err)
	}
}

func TestFileStorage_Close(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStorage(&FileStorageConfig{BaseDir: dir})
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}

	if err := fs.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

// ===================== FileReader Tests =====================

func TestFileReader(t *testing.T) {
	dir := t.TempDir()

	// Write some events to a file
	filename := filepath.Join(dir, "audit-2025-06-15.log")
	f, err := os.Create(filename)
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	events := []*audit.AuditEvent{
		makeMinimalEvent("fr1"),
		makeMinimalEvent("fr2"),
		makeMinimalEvent("fr3"),
	}
	for _, ev := range events {
		data, _ := json.Marshal(ev)
		f.Write(data)
		f.WriteString("\n")
	}
	f.Close()

	reader, err := NewFileReader(dir)
	if err != nil {
		t.Fatalf("NewFileReader failed: %v", err)
	}
	defer reader.Close()

	var count int
	for {
		ev, err := reader.Next()
		if err != nil {
			break
		}
		if ev != nil {
			count++
		}
	}
	if count != 3 {
		t.Errorf("expected 3 events, got %d", count)
	}
}

func TestFileReader_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	reader, err := NewFileReader(dir)
	if err != nil {
		t.Fatalf("NewFileReader failed: %v", err)
	}
	defer reader.Close()

	_, err = reader.Next()
	if err == nil {
		t.Error("expected error (EOF) for empty dir")
	}
}

func TestFileReader_MalformedLines(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "audit-2025-06-15.log")
	f, _ := os.Create(filename)
	f.WriteString("not json\n")
	ev := makeMinimalEvent("ok1")
	data, _ := json.Marshal(ev)
	f.Write(data)
	f.WriteString("\n")
	f.WriteString("also bad\n")
	f.Close()

	reader, _ := NewFileReader(dir)
	defer reader.Close()

	got, err := reader.Next()
	if err != nil {
		t.Fatalf("Next failed: %v", err)
	}
	if got.ID != "ok1" {
		t.Errorf("expected ok1, got %s", got.ID)
	}
}

// ===================== NewFileStorageFromConfig Tests =====================

func TestNewFileStorageFromConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := map[string]interface{}{
		"base_dir":       dir,
		"max_file_size":  float64(500000),
		"rotate_daily":   true,
		"retention_days": float64(60),
	}

	s, err := NewFileStorageFromConfig(cfg)
	if err != nil {
		t.Fatalf("NewFileStorageFromConfig failed: %v", err)
	}
	defer s.Close()
}

func TestNewFileStorageFromConfig_Empty(t *testing.T) {
	cfg := map[string]interface{}{}
	_, err := NewFileStorageFromConfig(cfg)
	if err == nil {
		t.Fatal("expected error for empty config")
	}
}

// ===================== matchesQuery (file.go) Tests =====================

func TestMatchesQuery(t *testing.T) {
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	ev := makeEvent("mq1", base, audit.EventTypeAuthentication, audit.SeverityInfo, "login", audit.OutcomeSuccess)

	// Empty query matches all
	if !matchesQuery(ev, &audit.AuditQuery{}) {
		t.Error("empty query should match")
	}

	// Time range - before
	if matchesQuery(ev, &audit.AuditQuery{StartTime: base.Add(time.Hour)}) {
		t.Error("event before start time should not match")
	}

	// Time range - after
	if matchesQuery(ev, &audit.AuditQuery{EndTime: base.Add(-time.Hour)}) {
		t.Error("event after end time should not match")
	}

	// Type mismatch
	if matchesQuery(ev, &audit.AuditQuery{Types: []audit.EventType{audit.EventTypeSystem}}) {
		t.Error("wrong type should not match")
	}

	// Type match
	if !matchesQuery(ev, &audit.AuditQuery{Types: []audit.EventType{audit.EventTypeAuthentication}}) {
		t.Error("correct type should match")
	}

	// Actor mismatch
	if matchesQuery(ev, &audit.AuditQuery{ActorIDs: []string{"nobody"}}) {
		t.Error("wrong actor should not match")
	}

	// Actor match
	if !matchesQuery(ev, &audit.AuditQuery{ActorIDs: []string{"actor-1"}}) {
		t.Error("correct actor should match")
	}
}
