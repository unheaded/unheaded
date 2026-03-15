// SPDX-License-Identifier: GPL-3.0-or-later
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

// ===================== FileStorage loadLastHash Tests =====================

func TestFileStorage_LoadLastHash_ExistingData(t *testing.T) {
	dir := t.TempDir()

	// Create a log file with a known event as the last line
	filename := filepath.Join(dir, "audit-2025-06-15.log")
	ev := makeEvent("lh1", time.Now().UTC(), audit.EventTypeSystem, audit.SeverityInfo, "test", audit.OutcomeSuccess)
	ev.Hash = "loadlasthash-test"
	data, _ := json.Marshal(ev)

	if err := os.WriteFile(filename, append(data, '\n'), 0640); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Create new FileStorage which should load the last hash
	fs, err := NewFileStorage(&FileStorageConfig{BaseDir: dir})
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}
	defer fs.Close()

	hash, _ := fs.GetLastHash(context.Background())
	if hash != "loadlasthash-test" {
		t.Errorf("expected hash loadlasthash-test, got %s", hash)
	}
}

func TestFileStorage_LoadLastHash_MultipleLines(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "audit-2025-06-15.log")

	f, err := os.Create(filename)
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	// Write multiple events, the last one should be used
	for i := 0; i < 3; i++ {
		ev := makeEvent(fmt.Sprintf("multi%d", i), time.Now().UTC(), audit.EventTypeSystem, audit.SeverityInfo, "test", audit.OutcomeSuccess)
		ev.Hash = fmt.Sprintf("hash-%d", i)
		data, _ := json.Marshal(ev)
		f.Write(data)
		f.WriteString("\n")
	}
	f.Close()

	fs, err := NewFileStorage(&FileStorageConfig{BaseDir: dir})
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}
	defer fs.Close()

	hash, _ := fs.GetLastHash(context.Background())
	if hash != "hash-2" {
		t.Errorf("expected hash-2, got %s", hash)
	}
}

func TestFileStorage_LoadLastHash_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "audit-2025-06-15.log")
	os.WriteFile(filename, []byte(""), 0640)

	fs, err := NewFileStorage(&FileStorageConfig{BaseDir: dir})
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}
	defer fs.Close()

	hash, _ := fs.GetLastHash(context.Background())
	if hash != "" {
		t.Errorf("expected empty hash for empty file, got %s", hash)
	}
}

func TestFileStorage_LoadLastHash_MalformedLastLine(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "audit-2025-06-15.log")
	os.WriteFile(filename, []byte("this is not json\n"), 0640)

	// Should still create, just with empty hash (loadLastHash error ignored)
	fs, err := NewFileStorage(&FileStorageConfig{BaseDir: dir})
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}
	defer fs.Close()

	hash, _ := fs.GetLastHash(context.Background())
	if hash != "" {
		t.Errorf("expected empty hash for malformed file, got %s", hash)
	}
}

// ===================== FileStorage Rotation Tests =====================

func TestFileStorage_SizeBasedRotation(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStorage(&FileStorageConfig{
		BaseDir:     dir,
		MaxFileSize: 100, // very small to trigger rotation
	})
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}
	defer fs.Close()

	ctx := context.Background()

	// Store enough events to trigger rotation
	for i := 0; i < 10; i++ {
		ev := makeEvent(fmt.Sprintf("rot-sz-%d", i), time.Now().UTC(), audit.EventTypeSystem, audit.SeverityInfo, "test-size-rotation", audit.OutcomeSuccess)
		_ = fs.Store(ctx, ev)
	}

	// Verify multiple files were created
	files, _ := filepath.Glob(filepath.Join(dir, "audit-*.log"))
	if len(files) < 2 {
		t.Errorf("expected at least 2 log files after rotation, got %d", len(files))
	}
}

func TestFileStorage_ManualRotate_WithData(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStorage(&FileStorageConfig{BaseDir: dir})
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}
	defer fs.Close()

	ctx := context.Background()

	// Store an event, then rotate, then store another
	ev1 := makeEvent("pre-rotate", time.Now().UTC(), audit.EventTypeSystem, audit.SeverityInfo, "test", audit.OutcomeSuccess)
	_ = fs.Store(ctx, ev1)

	if err := fs.Rotate(); err != nil {
		t.Fatalf("Rotate failed: %v", err)
	}

	ev2 := makeEvent("post-rotate", time.Now().UTC(), audit.EventTypeSystem, audit.SeverityInfo, "test", audit.OutcomeSuccess)
	_ = fs.Store(ctx, ev2)

	// Both events should still be retrievable
	got1, err := fs.Get(ctx, "pre-rotate")
	if err != nil {
		t.Errorf("Get pre-rotate failed: %v", err)
	} else if got1.ID != "pre-rotate" {
		t.Errorf("expected pre-rotate, got %s", got1.ID)
	}

	got2, err := fs.Get(ctx, "post-rotate")
	if err != nil {
		t.Errorf("Get post-rotate failed: %v", err)
	} else if got2.ID != "post-rotate" {
		t.Errorf("expected post-rotate, got %s", got2.ID)
	}
}

// ===================== FileStorage Close Edge Cases =====================

func TestFileStorage_Close_NilWriter(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStorage(&FileStorageConfig{BaseDir: dir})
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}

	// Close once
	err = fs.Close()
	if err != nil {
		t.Fatalf("First Close failed: %v", err)
	}

	// Manually set writer to nil and call close again
	fs.mu.Lock()
	fs.writer = nil
	fs.currentFile = nil
	fs.mu.Unlock()

	err = fs.Close()
	if err != nil {
		t.Errorf("Close with nil writer should not error: %v", err)
	}
}

// ===================== FileReader Multi-File Tests =====================

func TestFileReader_MultipleFiles(t *testing.T) {
	dir := t.TempDir()

	// Create two log files
	for _, date := range []string{"2025-06-14", "2025-06-15"} {
		filename := filepath.Join(dir, fmt.Sprintf("audit-%s.log", date))
		f, err := os.Create(filename)
		if err != nil {
			t.Fatalf("failed to create file: %v", err)
		}
		for i := 0; i < 2; i++ {
			ev := makeMinimalEvent(fmt.Sprintf("%s-ev%d", date, i))
			data, _ := json.Marshal(ev)
			f.Write(data)
			f.WriteString("\n")
		}
		f.Close()
	}

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
	if count != 4 {
		t.Errorf("expected 4 events across 2 files, got %d", count)
	}
}

func TestFileReader_Close_NoFiles(t *testing.T) {
	dir := t.TempDir()
	reader, err := NewFileReader(dir)
	if err != nil {
		t.Fatalf("NewFileReader failed: %v", err)
	}
	err = reader.Close()
	if err != nil {
		t.Errorf("Close should not error: %v", err)
	}
}

// ===================== MemoryStorage Additional Coverage =====================

func TestMemoryStorage_Query_CombinedFilters(t *testing.T) {
	s := NewMemoryStorage()
	ctx := context.Background()
	ts := time.Now().UTC()

	ev1 := makeEvent("cf1", ts, audit.EventTypeAuthentication, audit.SeverityWarning, "login", audit.OutcomeSuccess)
	ev1.Actor = &audit.Actor{ID: "alice"}
	ev1.Resource = &audit.Resource{ID: "r1", Type: "session"}

	ev2 := makeEvent("cf2", ts, audit.EventTypeAuthentication, audit.SeverityInfo, "login", audit.OutcomeFailure)
	ev2.Actor = &audit.Actor{ID: "bob"}
	ev2.Resource = &audit.Resource{ID: "r2", Type: "session"}

	ev3 := makeEvent("cf3", ts, audit.EventTypeSystem, audit.SeverityWarning, "restart", audit.OutcomeSuccess)
	ev3.Actor = &audit.Actor{ID: "alice"}
	ev3.Resource = &audit.Resource{ID: "r3", Type: "config"}

	_ = s.Store(ctx, ev1)
	_ = s.Store(ctx, ev2)
	_ = s.Store(ctx, ev3)

	// Combine type + severity + actor + action + outcome + resource type
	events, err := s.Query(ctx, &audit.AuditQuery{
		Types:         []audit.EventType{audit.EventTypeAuthentication},
		Severities:    []audit.Severity{audit.SeverityWarning},
		ActorIDs:      []string{"alice"},
		Actions:       []string{"login"},
		Outcomes:      []audit.Outcome{audit.OutcomeSuccess},
		ResourceTypes: []string{"session"},
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event matching all filters, got %d", len(events))
	}
	if len(events) == 1 && events[0].ID != "cf1" {
		t.Errorf("expected cf1, got %s", events[0].ID)
	}
}

func TestMemoryStorage_Query_SortBySeverity_Ascending(t *testing.T) {
	s := NewMemoryStorage()
	ctx := context.Background()
	ts := time.Now().UTC()

	_ = s.Store(ctx, makeEvent("sev1", ts, audit.EventTypeSystem, audit.SeverityCritical, "test", audit.OutcomeSuccess))
	_ = s.Store(ctx, makeEvent("sev2", ts.Add(time.Millisecond), audit.EventTypeSystem, audit.SeverityInfo, "test", audit.OutcomeSuccess))
	_ = s.Store(ctx, makeEvent("sev3", ts.Add(2*time.Millisecond), audit.EventTypeSystem, audit.SeverityError, "test", audit.OutcomeSuccess))

	events, _ := s.Query(ctx, &audit.AuditQuery{OrderBy: "severity", Descending: false})
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if events[0].Severity != audit.SeverityInfo {
		t.Errorf("expected info first in ascending, got %s", events[0].Severity)
	}
	if events[2].Severity != audit.SeverityCritical {
		t.Errorf("expected critical last in ascending, got %s", events[2].Severity)
	}
}

func TestMemoryStorage_Query_NilActorResourceFilters(t *testing.T) {
	s := NewMemoryStorage()
	ctx := context.Background()

	// Event with nil actor and nil resource
	ev := &audit.AuditEvent{
		ID:        "nil-ar",
		Timestamp: time.Now().UTC(),
		Type:      audit.EventTypeSystem,
		Severity:  audit.SeverityInfo,
		Action:    "test",
		Outcome:   audit.OutcomeSuccess,
		Hash:      "hash-nil-ar",
	}
	// Note: Actor is nil, Resource is nil
	_ = s.Store(ctx, ev)

	// Query with ActorIDs filter - should not crash on nil Actor
	events, err := s.Query(ctx, &audit.AuditQuery{
		ActorIDs: []string{"someone"},
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	// Nil actor means the filter check is skipped (event passes the actor filter)
	// Looking at the code: if len(query.ActorIDs) > 0 && event.Actor != nil
	// Since Actor is nil, this condition is false, so event matches
	if len(events) != 1 {
		t.Errorf("expected 1 event (nil actor passes filter), got %d", len(events))
	}

	// Query with ResourceTypes filter - should not crash on nil Resource
	events, err = s.Query(ctx, &audit.AuditQuery{
		ResourceTypes: []string{"document"},
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	// Nil resource means the filter check is skipped
	if len(events) != 1 {
		t.Errorf("expected 1 event (nil resource passes filter), got %d", len(events))
	}
}

// ===================== RetentionCleaner Additional Coverage =====================

func TestRetentionCleaner_CreateAndStop(t *testing.T) {
	s := NewMemoryStorage()
	cleaner := NewRetentionCleaner(s, 7, 50*time.Millisecond)
	if cleaner == nil {
		t.Fatal("expected non-nil cleaner")
	}
	if cleaner.retentionDays != 7 {
		t.Errorf("expected retention 7 days, got %d", cleaner.retentionDays)
	}
	if cleaner.interval != 50*time.Millisecond {
		t.Errorf("expected interval 50ms, got %v", cleaner.interval)
	}

	// Start and stop immediately
	cleaner.Start()
	cleaner.Stop()
}

// ===================== FileStorage Store/Query With Nil Fields =====================

func TestFileStorage_StoreAndQuery_NilFields(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStorage(&FileStorageConfig{BaseDir: dir})
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}
	defer fs.Close()

	ctx := context.Background()

	// Event with nil Actor, Resource, Source
	ev := &audit.AuditEvent{
		ID:        "nilfields",
		Timestamp: time.Now().UTC(),
		Type:      audit.EventTypeSystem,
		Severity:  audit.SeverityInfo,
		Action:    "test",
		Outcome:   audit.OutcomeSuccess,
		Hash:      "hash-nilfields",
	}

	if err := fs.Store(ctx, ev); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	got, err := fs.Get(ctx, "nilfields")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.ID != "nilfields" {
		t.Errorf("expected nilfields, got %s", got.ID)
	}
}

// ===================== matchesQuery (file.go package-level func) Additional =====================

func TestMatchesQuery_NilActor(t *testing.T) {
	ev := &audit.AuditEvent{
		ID:        "no-actor",
		Timestamp: time.Now().UTC(),
		Type:      audit.EventTypeSystem,
		Severity:  audit.SeverityInfo,
		Action:    "test",
		Outcome:   audit.OutcomeSuccess,
	}

	// With ActorIDs filter and nil Actor - should pass (filter is skipped when Actor is nil)
	q := &audit.AuditQuery{ActorIDs: []string{"someone"}}
	if !matchesQuery(ev, q) {
		t.Error("nil actor should pass actor filter in package-level matchesQuery")
	}
}

// ===================== NewFileStorageFromConfig Partial Config =====================

func TestNewFileStorageFromConfig_PartialConfig(t *testing.T) {
	dir := t.TempDir()

	// Only set base_dir, others should get defaults
	cfg := map[string]interface{}{
		"base_dir": dir,
	}

	s, err := NewFileStorageFromConfig(cfg)
	if err != nil {
		t.Fatalf("NewFileStorageFromConfig failed: %v", err)
	}
	defer s.Close()
}

func TestNewFileStorageFromConfig_AllFields(t *testing.T) {
	dir := t.TempDir()

	cfg := map[string]interface{}{
		"base_dir":       dir,
		"max_file_size":  float64(1024),
		"rotate_daily":   true,
		"retention_days": float64(7),
	}

	s, err := NewFileStorageFromConfig(cfg)
	if err != nil {
		t.Fatalf("NewFileStorageFromConfig failed: %v", err)
	}
	defer s.Close()

	// Cast to verify config was applied
	fss := s.(*FileStorage)
	if fss.maxFileSize != 1024 {
		t.Errorf("expected maxFileSize 1024, got %d", fss.maxFileSize)
	}
	if !fss.rotateDaily {
		t.Error("expected rotateDaily true")
	}
	if fss.retentionDays != 7 {
		t.Errorf("expected retentionDays 7, got %d", fss.retentionDays)
	}
}

// ===================== FileStorage queryFile with malformed data =====================

func TestFileStorage_Query_MalformedLines(t *testing.T) {
	dir := t.TempDir()

	// Create a file with a mix of valid and invalid lines
	filename := filepath.Join(dir, "audit-2025-06-15.log")
	f, err := os.Create(filename)
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	f.WriteString("bad json line\n")
	ev := makeMinimalEvent("good1")
	data, _ := json.Marshal(ev)
	f.Write(data)
	f.WriteString("\n")
	f.WriteString("{incomplete\n")
	f.Close()

	fs, err := NewFileStorage(&FileStorageConfig{BaseDir: dir})
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}
	defer fs.Close()

	ctx := context.Background()
	events, err := fs.Query(ctx, &audit.AuditQuery{})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	// Should only return the valid event
	if len(events) != 1 {
		t.Errorf("expected 1 valid event, got %d", len(events))
	}
}

// ===================== FileStorage findEventInFile edge cases =====================

func TestFileStorage_Get_MalformedFile(t *testing.T) {
	dir := t.TempDir()

	// Create a file with malformed data then a valid event
	filename := filepath.Join(dir, "audit-2025-06-15.log")
	f, err := os.Create(filename)
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	f.WriteString("not json at all\n")
	ev := makeMinimalEvent("findme")
	data, _ := json.Marshal(ev)
	f.Write(data)
	f.WriteString("\n")
	f.Close()

	fs, err := NewFileStorage(&FileStorageConfig{BaseDir: dir})
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}
	defer fs.Close()

	got, err := fs.Get(context.Background(), "findme")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.ID != "findme" {
		t.Errorf("expected findme, got %s", got.ID)
	}
}
