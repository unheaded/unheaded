// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package controls

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// EvidenceStore - basic CRUD
// ---------------------------------------------------------------------------

func TestNewEvidenceStore(t *testing.T) {
	s := NewEvidenceStore()
	if s == nil {
		t.Fatal("expected non-nil store")
	}
}

func TestStoreAndGet(t *testing.T) {
	s := NewEvidenceStore()
	ctx := context.Background()

	ev := &Evidence{
		ID:          "EV-001",
		ControlID:   "AC-1",
		Type:        EvidenceTypeDocument,
		Title:       "Test evidence",
		Content:     []byte("hello world"),
		CollectedAt: time.Now(),
		ValidFrom:   time.Now(),
	}

	if err := s.Store(ctx, ev); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	got, err := s.Get(ctx, "EV-001")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.ID != "EV-001" {
		t.Errorf("expected ID %q, got %q", "EV-001", got.ID)
	}
	if got.Title != "Test evidence" {
		t.Error("title mismatch")
	}
}

func TestStoreAutoGeneratesID(t *testing.T) {
	s := NewEvidenceStore()
	ctx := context.Background()

	ev := &Evidence{
		ControlID:   "AC-1",
		Type:        EvidenceTypeLog,
		Title:       "Auto-ID",
		CollectedAt: time.Now(),
		ValidFrom:   time.Now(),
	}

	if err := s.Store(ctx, ev); err != nil {
		t.Fatalf("Store failed: %v", err)
	}
	if ev.ID == "" {
		t.Error("expected auto-generated ID")
	}
	if !strings.HasPrefix(ev.ID, "EV-") {
		t.Errorf("expected ID starting with EV-, got %q", ev.ID)
	}
}

func TestStoreComputesHash(t *testing.T) {
	s := NewEvidenceStore()
	ctx := context.Background()

	ev := &Evidence{
		ID:          "EV-HASH",
		ControlID:   "AC-1",
		Content:     []byte("some content"),
		CollectedAt: time.Now(),
		ValidFrom:   time.Now(),
	}

	if err := s.Store(ctx, ev); err != nil {
		t.Fatalf("Store failed: %v", err)
	}
	if ev.ContentHash == "" {
		t.Error("expected content hash to be computed")
	}
}

func TestStorePreservesExistingHash(t *testing.T) {
	s := NewEvidenceStore()
	ctx := context.Background()

	ev := &Evidence{
		ID:          "EV-KEEP",
		ControlID:   "AC-1",
		Content:     []byte("data"),
		ContentHash: "pre-existing-hash",
		CollectedAt: time.Now(),
		ValidFrom:   time.Now(),
	}

	if err := s.Store(ctx, ev); err != nil {
		t.Fatalf("Store failed: %v", err)
	}
	if ev.ContentHash != "pre-existing-hash" {
		t.Error("should not overwrite existing content hash")
	}
}

func TestStoreEmptyContentNoHash(t *testing.T) {
	s := NewEvidenceStore()
	ctx := context.Background()

	ev := &Evidence{
		ID:          "EV-EMPTY",
		ControlID:   "AC-1",
		Content:     nil,
		CollectedAt: time.Now(),
		ValidFrom:   time.Now(),
	}

	if err := s.Store(ctx, ev); err != nil {
		t.Fatalf("Store failed: %v", err)
	}
	if ev.ContentHash != "" {
		t.Error("expected empty hash for nil content")
	}
}

func TestGetNotFound(t *testing.T) {
	s := NewEvidenceStore()
	_, err := s.Get(context.Background(), "NONEXISTENT")
	if err == nil {
		t.Error("expected error for nonexistent ID")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got %q", err.Error())
	}
}

// ---------------------------------------------------------------------------
// GetByControl
// ---------------------------------------------------------------------------

func TestGetByControl(t *testing.T) {
	s := NewEvidenceStore()
	ctx := context.Background()

	for _, ev := range []*Evidence{
		{ID: "EV-1", ControlID: "AC-1", CollectedAt: time.Now(), ValidFrom: time.Now()},
		{ID: "EV-2", ControlID: "AC-1", CollectedAt: time.Now(), ValidFrom: time.Now()},
		{ID: "EV-3", ControlID: "AC-2", CollectedAt: time.Now(), ValidFrom: time.Now()},
	} {
		if err := s.Store(ctx, ev); err != nil {
			t.Fatalf("Store failed: %v", err)
		}
	}

	evidence, err := s.GetByControl(ctx, "AC-1")
	if err != nil {
		t.Fatalf("GetByControl failed: %v", err)
	}
	if len(evidence) != 2 {
		t.Errorf("expected 2 evidence items for AC-1, got %d", len(evidence))
	}

	evidence, err = s.GetByControl(ctx, "AC-2")
	if err != nil {
		t.Fatalf("GetByControl failed: %v", err)
	}
	if len(evidence) != 1 {
		t.Errorf("expected 1 evidence item for AC-2, got %d", len(evidence))
	}
}

func TestGetByControlNotFound(t *testing.T) {
	s := NewEvidenceStore()
	evidence, err := s.GetByControl(context.Background(), "UNKNOWN")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(evidence) != 0 {
		t.Errorf("expected 0 evidence items, got %d", len(evidence))
	}
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

func TestDelete(t *testing.T) {
	s := NewEvidenceStore()
	ctx := context.Background()

	ev := &Evidence{ID: "EV-DEL", ControlID: "AC-1", CollectedAt: time.Now(), ValidFrom: time.Now()}
	if err := s.Store(ctx, ev); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	if err := s.Delete(ctx, "EV-DEL"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err := s.Get(ctx, "EV-DEL")
	if err == nil {
		t.Error("expected error after deletion")
	}
}

func TestDeleteNotFound(t *testing.T) {
	s := NewEvidenceStore()
	err := s.Delete(context.Background(), "NONEXISTENT")
	if err == nil {
		t.Error("expected error deleting nonexistent evidence")
	}
}

func TestDeleteRemovesFromControlIndex(t *testing.T) {
	s := NewEvidenceStore()
	ctx := context.Background()

	ev1 := &Evidence{ID: "EV-A", ControlID: "AC-1", CollectedAt: time.Now(), ValidFrom: time.Now()}
	ev2 := &Evidence{ID: "EV-B", ControlID: "AC-1", CollectedAt: time.Now(), ValidFrom: time.Now()}
	s.Store(ctx, ev1)
	s.Store(ctx, ev2)

	s.Delete(ctx, "EV-A")

	evidence, _ := s.GetByControl(ctx, "AC-1")
	if len(evidence) != 1 {
		t.Errorf("expected 1 remaining evidence, got %d", len(evidence))
	}
	if evidence[0].ID != "EV-B" {
		t.Errorf("expected remaining evidence EV-B, got %s", evidence[0].ID)
	}
}

// ---------------------------------------------------------------------------
// List and filters
// ---------------------------------------------------------------------------

func TestListAll(t *testing.T) {
	s := NewEvidenceStore()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		s.Store(ctx, &Evidence{
			ID:          fmt.Sprintf("EV-LIST-%d", i),
			ControlID:   "AC-1",
			Type:        EvidenceTypeDocument,
			CollectedAt: time.Now(),
			ValidFrom:   time.Now(),
		})
	}

	all, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(all) != 5 {
		t.Errorf("expected 5 items, got %d", len(all))
	}
}

func TestFilterByType(t *testing.T) {
	s := NewEvidenceStore()
	ctx := context.Background()

	s.Store(ctx, &Evidence{ID: "EV-D1", ControlID: "AC-1", Type: EvidenceTypeDocument, CollectedAt: time.Now(), ValidFrom: time.Now()})
	s.Store(ctx, &Evidence{ID: "EV-L1", ControlID: "AC-1", Type: EvidenceTypeLog, CollectedAt: time.Now(), ValidFrom: time.Now()})
	s.Store(ctx, &Evidence{ID: "EV-D2", ControlID: "AC-1", Type: EvidenceTypeDocument, CollectedAt: time.Now(), ValidFrom: time.Now()})

	docs, err := s.List(ctx, FilterByType(EvidenceTypeDocument))
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(docs) != 2 {
		t.Errorf("expected 2 documents, got %d", len(docs))
	}

	logs, _ := s.List(ctx, FilterByType(EvidenceTypeLog))
	if len(logs) != 1 {
		t.Errorf("expected 1 log, got %d", len(logs))
	}
}

func TestFilterByControl(t *testing.T) {
	s := NewEvidenceStore()
	ctx := context.Background()

	s.Store(ctx, &Evidence{ID: "E1", ControlID: "AC-1", CollectedAt: time.Now(), ValidFrom: time.Now()})
	s.Store(ctx, &Evidence{ID: "E2", ControlID: "AC-2", CollectedAt: time.Now(), ValidFrom: time.Now()})
	s.Store(ctx, &Evidence{ID: "E3", ControlID: "AC-1", CollectedAt: time.Now(), ValidFrom: time.Now()})

	filtered, _ := s.List(ctx, FilterByControl("AC-1"))
	if len(filtered) != 2 {
		t.Errorf("expected 2 items for AC-1, got %d", len(filtered))
	}
}

func TestFilterByDateRange(t *testing.T) {
	s := NewEvidenceStore()
	ctx := context.Background()

	base := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)

	s.Store(ctx, &Evidence{ID: "E1", ControlID: "C1", CollectedAt: base.Add(-48 * time.Hour), ValidFrom: base})
	s.Store(ctx, &Evidence{ID: "E2", ControlID: "C1", CollectedAt: base, ValidFrom: base})
	s.Store(ctx, &Evidence{ID: "E3", ControlID: "C1", CollectedAt: base.Add(48 * time.Hour), ValidFrom: base})

	start := base.Add(-24 * time.Hour)
	end := base.Add(24 * time.Hour)
	filtered, _ := s.List(ctx, FilterByDateRange(start, end))
	if len(filtered) != 1 {
		t.Errorf("expected 1 item in date range, got %d", len(filtered))
	}
}

func TestFilterValid(t *testing.T) {
	s := NewEvidenceStore()
	ctx := context.Background()
	now := time.Now()

	past := now.Add(-24 * time.Hour)
	future := now.Add(24 * time.Hour)
	farPast := now.Add(-48 * time.Hour)

	// Valid: ValidFrom in past, ValidUntil in future
	s.Store(ctx, &Evidence{ID: "VALID", ControlID: "C1", CollectedAt: now, ValidFrom: past, ValidUntil: &future})
	// Expired: ValidUntil in past
	s.Store(ctx, &Evidence{ID: "EXPIRED", ControlID: "C1", CollectedAt: now, ValidFrom: farPast, ValidUntil: &past})
	// Not yet valid: ValidFrom in future
	s.Store(ctx, &Evidence{ID: "FUTURE", ControlID: "C1", CollectedAt: now, ValidFrom: future})
	// Valid: no expiry
	s.Store(ctx, &Evidence{ID: "NOEXPIRY", ControlID: "C1", CollectedAt: now, ValidFrom: past})

	valid, _ := s.List(ctx, FilterValid())
	if len(valid) != 2 {
		t.Errorf("expected 2 valid items, got %d", len(valid))
	}

	ids := map[string]bool{}
	for _, v := range valid {
		ids[v.ID] = true
	}
	if !ids["VALID"] || !ids["NOEXPIRY"] {
		t.Error("expected VALID and NOEXPIRY in results")
	}
}

func TestFilterCombined(t *testing.T) {
	s := NewEvidenceStore()
	ctx := context.Background()

	s.Store(ctx, &Evidence{ID: "E1", ControlID: "AC-1", Type: EvidenceTypeDocument, CollectedAt: time.Now(), ValidFrom: time.Now()})
	s.Store(ctx, &Evidence{ID: "E2", ControlID: "AC-1", Type: EvidenceTypeLog, CollectedAt: time.Now(), ValidFrom: time.Now()})
	s.Store(ctx, &Evidence{ID: "E3", ControlID: "AC-2", Type: EvidenceTypeDocument, CollectedAt: time.Now(), ValidFrom: time.Now()})

	filtered, _ := s.List(ctx, FilterByControl("AC-1"), FilterByType(EvidenceTypeDocument))
	if len(filtered) != 1 {
		t.Errorf("expected 1 item matching both filters, got %d", len(filtered))
	}
	if filtered[0].ID != "E1" {
		t.Errorf("expected E1, got %s", filtered[0].ID)
	}
}

// ---------------------------------------------------------------------------
// EvidenceType constants
// ---------------------------------------------------------------------------

func TestEvidenceTypeConstants(t *testing.T) {
	types := []EvidenceType{
		EvidenceTypeDocument,
		EvidenceTypeScreenshot,
		EvidenceTypeLog,
		EvidenceTypeConfig,
		EvidenceTypeReport,
		EvidenceTypeArtifact,
		EvidenceTypeAttestation,
	}
	seen := make(map[EvidenceType]bool)
	for _, et := range types {
		if seen[et] {
			t.Errorf("duplicate evidence type: %s", et)
		}
		seen[et] = true
	}
	if len(types) != 7 {
		t.Errorf("expected 7 evidence types, got %d", len(types))
	}
}

// ---------------------------------------------------------------------------
// EvidenceCollectorRegistry
// ---------------------------------------------------------------------------

func TestNewEvidenceCollectorRegistry(t *testing.T) {
	r := NewEvidenceCollectorRegistry()
	if r == nil {
		t.Fatal("expected non-nil registry")
	}
}

func TestEvidenceCollectorRegistryRegisterAndGet(t *testing.T) {
	r := NewEvidenceCollectorRegistry()
	configs := map[string][]byte{"app.json": []byte(`{"key":"val"}`)}
	collector := NewConfigurationCollector(configs)
	r.Register("config", collector)

	ctrl := makeControl("AC-1", CategoryAccessControl)
	collectors := r.GetCollectors(ctrl)
	if len(collectors) != 1 {
		t.Fatalf("expected 1 collector, got %d", len(collectors))
	}
	if collectors[0].Name() != "configuration_collector" {
		t.Errorf("unexpected collector name: %s", collectors[0].Name())
	}
}

func TestEvidenceCollectorRegistryNoMatch(t *testing.T) {
	r := NewEvidenceCollectorRegistry()
	logReader := strings.NewReader("log data")
	r.Register("log", NewLogCollector(logReader, "test"))

	// LogCollector only supports audit_logging
	ctrl := makeControl("AC-1", CategoryAccessControl)
	collectors := r.GetCollectors(ctrl)
	if len(collectors) != 0 {
		t.Errorf("expected 0 collectors, got %d", len(collectors))
	}
}

func TestCollectAll(t *testing.T) {
	r := NewEvidenceCollectorRegistry()
	configs := map[string][]byte{
		"a.json": []byte(`{"a":1}`),
		"b.json": []byte(`{"b":2}`),
	}
	r.Register("config", NewConfigurationCollector(configs))

	ctrl := makeControl("AC-1", CategoryAccessControl)
	evidence, err := r.CollectAll(context.Background(), ctrl)
	if err != nil {
		t.Fatalf("CollectAll failed: %v", err)
	}
	if len(evidence) != 2 {
		t.Errorf("expected 2 evidence items, got %d", len(evidence))
	}
}

func TestCollectAllNoCollectors(t *testing.T) {
	r := NewEvidenceCollectorRegistry()
	ctrl := makeControl("X-1", CategoryPrivacy)
	evidence, err := r.CollectAll(context.Background(), ctrl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(evidence) != 0 {
		t.Errorf("expected 0 evidence, got %d", len(evidence))
	}
}

// ---------------------------------------------------------------------------
// ConfigurationCollector
// ---------------------------------------------------------------------------

func TestConfigurationCollectorName(t *testing.T) {
	c := NewConfigurationCollector(nil)
	if c.Name() != "configuration_collector" {
		t.Errorf("expected name %q, got %q", "configuration_collector", c.Name())
	}
}

func TestConfigurationCollectorSupports(t *testing.T) {
	c := NewConfigurationCollector(nil)
	// Should support all controls
	if !c.Supports(makeControl("AC-1", CategoryAccessControl)) {
		t.Error("configuration collector should support any control")
	}
	if !c.Supports(makeControl("AU-1", CategoryAuditLogging)) {
		t.Error("configuration collector should support any control")
	}
}

func TestConfigurationCollectorCollect(t *testing.T) {
	configs := map[string][]byte{
		"db.json":  []byte(`{"host":"localhost"}`),
		"app.json": []byte(`{"port":8080}`),
	}
	c := NewConfigurationCollector(configs)

	evidence, err := c.Collect(context.Background(), "AC-1")
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}
	if len(evidence) != 2 {
		t.Fatalf("expected 2 evidence items, got %d", len(evidence))
	}

	for _, ev := range evidence {
		if ev.Type != EvidenceTypeConfig {
			t.Errorf("expected type %q, got %q", EvidenceTypeConfig, ev.Type)
		}
		if ev.ControlID != "AC-1" {
			t.Errorf("expected controlID AC-1, got %s", ev.ControlID)
		}
		if ev.ContentHash == "" {
			t.Error("expected content hash")
		}
		if ev.ContentType != "application/json" {
			t.Errorf("expected content type application/json, got %s", ev.ContentType)
		}
		if ev.CollectedBy != "automated" {
			t.Errorf("expected collectedBy automated, got %s", ev.CollectedBy)
		}
		if ev.ID == "" {
			t.Error("expected auto-generated ID")
		}
	}
}

func TestConfigurationCollectorEmpty(t *testing.T) {
	c := NewConfigurationCollector(map[string][]byte{})
	evidence, err := c.Collect(context.Background(), "AC-1")
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}
	if len(evidence) != 0 {
		t.Errorf("expected 0 evidence from empty configs, got %d", len(evidence))
	}
}

// ---------------------------------------------------------------------------
// LogCollector
// ---------------------------------------------------------------------------

func TestLogCollectorName(t *testing.T) {
	c := NewLogCollector(strings.NewReader(""), "src")
	if c.Name() != "log_collector" {
		t.Errorf("expected name %q, got %q", "log_collector", c.Name())
	}
}

func TestLogCollectorSupports(t *testing.T) {
	c := NewLogCollector(strings.NewReader(""), "src")
	if !c.Supports(makeControl("AU-1", CategoryAuditLogging)) {
		t.Error("should support audit logging controls")
	}
	if c.Supports(makeControl("AC-1", CategoryAccessControl)) {
		t.Error("should not support non-audit-logging controls")
	}
}

func TestLogCollectorCollect(t *testing.T) {
	logData := "2025-01-01 INFO login event\n2025-01-01 WARN failed auth"
	c := NewLogCollector(strings.NewReader(logData), "auth-service")

	evidence, err := c.Collect(context.Background(), "AU-2")
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}
	if len(evidence) != 1 {
		t.Fatalf("expected 1 evidence item, got %d", len(evidence))
	}

	ev := evidence[0]
	if ev.Type != EvidenceTypeLog {
		t.Errorf("expected type %q, got %q", EvidenceTypeLog, ev.Type)
	}
	if ev.ControlID != "AU-2" {
		t.Errorf("expected controlID AU-2, got %s", ev.ControlID)
	}
	if ev.Source != "auth-service" {
		t.Errorf("expected source auth-service, got %s", ev.Source)
	}
	if !bytes.Equal(ev.Content, []byte(logData)) {
		t.Error("content mismatch")
	}
	if ev.ContentType != "text/plain" {
		t.Errorf("expected content type text/plain, got %s", ev.ContentType)
	}
	if ev.Size != int64(len(logData)) {
		t.Errorf("expected size %d, got %d", len(logData), ev.Size)
	}
	if ev.Metadata["log_source"] != "auth-service" {
		t.Error("metadata log_source mismatch")
	}
}

func TestLogCollectorReadError(t *testing.T) {
	reader := &errorReader{err: bytes.ErrTooLarge}
	c := NewLogCollector(reader, "bad-source")

	_, err := c.Collect(context.Background(), "AU-1")
	if err == nil {
		t.Fatal("expected error from failed reader")
	}
	if !strings.Contains(err.Error(), "failed to read logs") {
		t.Errorf("expected 'failed to read logs' error, got %q", err.Error())
	}
}

// errorReader is an io.Reader that always returns an error.
type errorReader struct {
	err error
}

func (r *errorReader) Read(p []byte) (int, error) {
	return 0, r.err
}

// ---------------------------------------------------------------------------
// computeHash
// ---------------------------------------------------------------------------

func TestComputeHash(t *testing.T) {
	h1 := computeHash([]byte("hello"))
	h2 := computeHash([]byte("hello"))
	h3 := computeHash([]byte("world"))

	if h1 != h2 {
		t.Error("same data should produce same hash")
	}
	if h1 == h3 {
		t.Error("different data should produce different hashes")
	}
	if len(h1) != 64 {
		t.Errorf("expected SHA-256 hex length 64, got %d", len(h1))
	}
}

func TestComputeHashEmpty(t *testing.T) {
	h := computeHash([]byte{})
	if h == "" {
		t.Error("hash of empty data should not be empty string")
	}
}

// ---------------------------------------------------------------------------
// generateEvidenceID
// ---------------------------------------------------------------------------

func TestGenerateEvidenceID(t *testing.T) {
	id := generateEvidenceID()
	if !strings.HasPrefix(id, "EV-") {
		t.Errorf("expected ID prefix EV-, got %q", id)
	}
}

func TestGenerateEvidenceIDUnique(t *testing.T) {
	// generateEvidenceID uses UnixNano, so duplicates within a tight loop
	// are expected on fast machines. We simply verify the format here.
	id1 := generateEvidenceID()
	if !strings.HasPrefix(id1, "EV-") {
		t.Errorf("expected prefix EV-, got %q", id1)
	}
	// Small sleep to guarantee different nanosecond
	time.Sleep(time.Microsecond)
	id2 := generateEvidenceID()
	if id1 == id2 {
		t.Error("two IDs with sleep should differ")
	}
}
