// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package audit

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// AuditTrail — Append, Get, Count, LastSequence
// ---------------------------------------------------------------------------

func makeEvent(id, action string) *AuditEvent {
	return &AuditEvent{
		ID:     id,
		Type:   EventControlChecked,
		Action: action,
		Actor:  "tester",
	}
}

func TestAuditTrail_Append(t *testing.T) {
	trail := NewAuditTrail()
	ctx := context.Background()

	entry, err := trail.Append(ctx, makeEvent("E1", "check"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if entry.Sequence != 1 {
		t.Errorf("expected sequence 1, got %d", entry.Sequence)
	}
	if entry.PreviousHash != "genesis" {
		t.Errorf("expected previous_hash genesis, got %s", entry.PreviousHash)
	}
	if entry.Hash == "" {
		t.Error("expected hash to be computed")
	}
	if trail.Count() != 1 {
		t.Errorf("expected count 1, got %d", trail.Count())
	}
	if trail.LastSequence() != 1 {
		t.Errorf("expected last sequence 1, got %d", trail.LastSequence())
	}
}

func TestAuditTrail_Append_ChainedHashes(t *testing.T) {
	trail := NewAuditTrail()
	ctx := context.Background()

	e1, _ := trail.Append(ctx, makeEvent("E1", "a"))
	e2, _ := trail.Append(ctx, makeEvent("E2", "b"))
	e3, _ := trail.Append(ctx, makeEvent("E3", "c"))

	if e2.PreviousHash != e1.Hash {
		t.Error("e2.PreviousHash should equal e1.Hash")
	}
	if e3.PreviousHash != e2.Hash {
		t.Error("e3.PreviousHash should equal e2.Hash")
	}
	if trail.Count() != 3 {
		t.Errorf("expected count 3, got %d", trail.Count())
	}
	if trail.LastSequence() != 3 {
		t.Errorf("expected last sequence 3, got %d", trail.LastSequence())
	}
}

func TestAuditTrail_Append_ConcurrentSafety(t *testing.T) {
	trail := NewAuditTrail()
	ctx := context.Background()
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, _ = trail.Append(ctx, makeEvent("E", "action"))
		}(i)
	}
	wg.Wait()

	if trail.Count() != 50 {
		t.Errorf("expected 50 entries, got %d", trail.Count())
	}
}

// ---------------------------------------------------------------------------
// AuditTrail — Get
// ---------------------------------------------------------------------------

func TestAuditTrail_Get_Found(t *testing.T) {
	trail := NewAuditTrail()
	ctx := context.Background()

	trail.Append(ctx, makeEvent("E1", "a"))
	trail.Append(ctx, makeEvent("E2", "b"))

	entry, err := trail.Get(2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry.Event.ID != "E2" {
		t.Errorf("expected event ID E2, got %s", entry.Event.ID)
	}
}

func TestAuditTrail_Get_NotFound(t *testing.T) {
	trail := NewAuditTrail()
	_, err := trail.Get(999)
	if err == nil {
		t.Error("expected error for non-existent sequence")
	}
}

// ---------------------------------------------------------------------------
// AuditTrail — GetRange
// ---------------------------------------------------------------------------

func TestAuditTrail_GetRange(t *testing.T) {
	trail := NewAuditTrail()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		trail.Append(ctx, makeEvent("E", "action"))
	}

	entries, err := trail.GetRange(2, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("expected 3 entries in range [2,4], got %d", len(entries))
	}
}

func TestAuditTrail_GetRange_Empty(t *testing.T) {
	trail := NewAuditTrail()
	entries, err := trail.GetRange(1, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

// ---------------------------------------------------------------------------
// AuditTrail — GetByTimeRange
// ---------------------------------------------------------------------------

func TestAuditTrail_GetByTimeRange(t *testing.T) {
	trail := NewAuditTrail()
	ctx := context.Background()

	// Add entries with small delays to ensure ordering
	trail.Append(ctx, makeEvent("E1", "a"))
	trail.Append(ctx, makeEvent("E2", "b"))

	// Query everything from a minute ago to a minute from now
	start := time.Now().Add(-1 * time.Minute)
	end := time.Now().Add(1 * time.Minute)

	entries, err := trail.GetByTimeRange(start, end)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries in time range, got %d", len(entries))
	}
}

func TestAuditTrail_GetByTimeRange_NoMatch(t *testing.T) {
	trail := NewAuditTrail()
	ctx := context.Background()
	trail.Append(ctx, makeEvent("E1", "a"))

	// Query a range in the past
	start := time.Now().Add(-2 * time.Hour)
	end := time.Now().Add(-1 * time.Hour)

	entries, err := trail.GetByTimeRange(start, end)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

// ---------------------------------------------------------------------------
// AuditTrail — Verify
// ---------------------------------------------------------------------------

func TestAuditTrail_Verify_Valid(t *testing.T) {
	trail := NewAuditTrail()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		trail.Append(ctx, makeEvent("E", "action"))
	}

	result, err := trail.Verify()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsValid {
		t.Error("expected trail to be valid")
	}
	if result.TotalEntries != 5 {
		t.Errorf("expected 5 total entries, got %d", result.TotalEntries)
	}
	if result.ValidEntries != 5 {
		t.Errorf("expected 5 valid entries, got %d", result.ValidEntries)
	}
	if len(result.InvalidEntries) != 0 {
		t.Errorf("expected 0 invalid entries, got %d", len(result.InvalidEntries))
	}
	if result.VerifiedAt.IsZero() {
		t.Error("expected VerifiedAt to be set")
	}
}

func TestAuditTrail_Verify_EmptyTrail(t *testing.T) {
	trail := NewAuditTrail()
	result, err := trail.Verify()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsValid {
		t.Error("expected empty trail to be valid")
	}
	if result.TotalEntries != 0 {
		t.Errorf("expected 0 total entries, got %d", result.TotalEntries)
	}
}

func TestAuditTrail_Verify_TamperedHash(t *testing.T) {
	trail := NewAuditTrail()
	ctx := context.Background()

	trail.Append(ctx, makeEvent("E1", "a"))
	trail.Append(ctx, makeEvent("E2", "b"))
	trail.Append(ctx, makeEvent("E3", "c"))

	// Tamper with the second entry's hash
	trail.mu.Lock()
	trail.entries[1].Hash = "tampered-hash"
	trail.mu.Unlock()

	result, err := trail.Verify()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsValid {
		t.Error("expected trail to be invalid after tampering")
	}
	if len(result.InvalidEntries) == 0 {
		t.Error("expected at least one invalid entry")
	}
}

func TestAuditTrail_Verify_TamperedPreviousHash(t *testing.T) {
	trail := NewAuditTrail()
	ctx := context.Background()

	trail.Append(ctx, makeEvent("E1", "a"))
	trail.Append(ctx, makeEvent("E2", "b"))

	// Tamper with the second entry's previous hash
	trail.mu.Lock()
	trail.entries[1].PreviousHash = "wrong-prev"
	trail.mu.Unlock()

	result, err := trail.Verify()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsValid {
		t.Error("expected trail to be invalid after tampering previous hash")
	}

	found := false
	for _, inv := range result.InvalidEntries {
		if inv.Reason == "previous_hash_mismatch" {
			found = true
		}
	}
	if !found {
		t.Error("expected a previous_hash_mismatch invalid entry")
	}
}

// ---------------------------------------------------------------------------
// AuditTrail — Export
// ---------------------------------------------------------------------------

func TestAuditTrail_Export(t *testing.T) {
	trail := NewAuditTrail()
	ctx := context.Background()

	trail.Append(ctx, makeEvent("E1", "a"))
	trail.Append(ctx, makeEvent("E2", "b"))

	data, err := trail.Export(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var entries []*TrailEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries in export, got %d", len(entries))
	}
}

// ---------------------------------------------------------------------------
// InMemoryArchiveStore
// ---------------------------------------------------------------------------

func TestInMemoryArchiveStore_StoreAndRetrieve(t *testing.T) {
	store := NewInMemoryArchiveStore()
	ctx := context.Background()

	entries := []*TrailEntry{
		{Sequence: 1, Hash: "h1"},
		{Sequence: 2, Hash: "h2"},
	}
	meta := ArchiveMetadata{
		StartSequence: 1,
		EndSequence:   2,
		EntryCount:    2,
	}

	archiveID, err := store.Store(ctx, entries, meta)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if archiveID == "" {
		t.Error("expected non-empty archive ID")
	}

	retrieved, err := store.Retrieve(ctx, archiveID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(retrieved) != 2 {
		t.Errorf("expected 2 entries, got %d", len(retrieved))
	}
}

func TestInMemoryArchiveStore_Retrieve_NotFound(t *testing.T) {
	store := NewInMemoryArchiveStore()
	_, err := store.Retrieve(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for non-existent archive")
	}
}

func TestInMemoryArchiveStore_List(t *testing.T) {
	store := NewInMemoryArchiveStore()
	ctx := context.Background()

	store.Store(ctx, []*TrailEntry{{Sequence: 1}}, ArchiveMetadata{EntryCount: 1})
	store.Store(ctx, []*TrailEntry{{Sequence: 2}}, ArchiveMetadata{EntryCount: 1})

	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 archives, got %d", len(list))
	}
}

func TestInMemoryArchiveStore_ListEmpty(t *testing.T) {
	store := NewInMemoryArchiveStore()
	list, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected 0 archives, got %d", len(list))
	}
}

// ---------------------------------------------------------------------------
// NewTrailArchiver
// ---------------------------------------------------------------------------

func TestNewTrailArchiver(t *testing.T) {
	trail := NewAuditTrail()
	store := NewInMemoryArchiveStore()
	archiver := NewTrailArchiver(trail, store, 24*time.Hour)

	if archiver == nil {
		t.Fatal("expected non-nil archiver")
	}
	if archiver.trail != trail {
		t.Error("trail not set correctly")
	}
	if archiver.archiveStore != store {
		t.Error("store not set correctly")
	}
	if archiver.retention != 24*time.Hour {
		t.Errorf("expected retention 24h, got %v", archiver.retention)
	}
}

// ---------------------------------------------------------------------------
// TrailQueryBuilder
// ---------------------------------------------------------------------------

func seedTrail(t *testing.T) *AuditTrail {
	t.Helper()
	trail := NewAuditTrail()
	ctx := context.Background()

	events := []*AuditEvent{
		{ID: "E1", Type: EventControlChecked, Actor: "alice", Action: "check"},
		{ID: "E2", Type: EventEvidenceCollected, Actor: "bob", Action: "collect"},
		{ID: "E3", Type: EventReportGenerated, Actor: "alice", Action: "generate"},
		{ID: "E4", Type: EventError, Actor: "system", Action: "error"},
		{ID: "E5", Type: EventControlChecked, Actor: "bob", Action: "check"},
	}

	for _, e := range events {
		trail.Append(ctx, e)
	}
	return trail
}

func TestTrailQuery_NoFilters(t *testing.T) {
	trail := seedTrail(t)
	entries, err := NewTrailQuery(trail).Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 5 {
		t.Errorf("expected 5 entries, got %d", len(entries))
	}
}

func TestTrailQuery_BySequenceRange(t *testing.T) {
	trail := seedTrail(t)
	entries, err := NewTrailQuery(trail).
		FromSequence(2).
		ToSequence(4).
		Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("expected 3 entries for sequence [2,4], got %d", len(entries))
	}
}

func TestTrailQuery_ByEventType(t *testing.T) {
	trail := seedTrail(t)
	entries, err := NewTrailQuery(trail).
		WithEventTypes(EventControlChecked).
		Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}
}

func TestTrailQuery_ByMultipleEventTypes(t *testing.T) {
	trail := seedTrail(t)
	entries, err := NewTrailQuery(trail).
		WithEventTypes(EventControlChecked, EventError).
		Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}
}

func TestTrailQuery_ByActor(t *testing.T) {
	trail := seedTrail(t)
	entries, err := NewTrailQuery(trail).
		WithActors("alice").
		Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries for alice, got %d", len(entries))
	}
}

func TestTrailQuery_ByMultipleActors(t *testing.T) {
	trail := seedTrail(t)
	entries, err := NewTrailQuery(trail).
		WithActors("alice", "system").
		Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}
}

func TestTrailQuery_WithLimit(t *testing.T) {
	trail := seedTrail(t)
	entries, err := NewTrailQuery(trail).
		WithLimit(2).
		Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries with limit, got %d", len(entries))
	}
}

func TestTrailQuery_ByTimeRange(t *testing.T) {
	trail := seedTrail(t)
	start := time.Now().Add(-1 * time.Minute)
	end := time.Now().Add(1 * time.Minute)

	entries, err := NewTrailQuery(trail).
		FromTime(start).
		ToTime(end).
		Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 5 {
		t.Errorf("expected 5 entries in time range, got %d", len(entries))
	}
}

func TestTrailQuery_CombinedFilters(t *testing.T) {
	trail := seedTrail(t)
	entries, err := NewTrailQuery(trail).
		WithEventTypes(EventControlChecked).
		WithActors("bob").
		Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 entry (bob + control_checked), got %d", len(entries))
	}
}

// ---------------------------------------------------------------------------
// VerificationResult JSON
// ---------------------------------------------------------------------------

func TestVerificationResult_JSONRoundTrip(t *testing.T) {
	result := &VerificationResult{
		IsValid:      false,
		TotalEntries: 10,
		ValidEntries: 8,
		InvalidEntries: []InvalidEntry{
			{Sequence: 3, Reason: "hash_mismatch", Expected: "abc", Actual: "xyz"},
			{Sequence: 7, Reason: "previous_hash_mismatch", Expected: "def", Actual: "uvw"},
		},
		VerifiedAt: time.Now(),
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded VerificationResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.IsValid != result.IsValid {
		t.Errorf("IsValid mismatch")
	}
	if decoded.TotalEntries != result.TotalEntries {
		t.Errorf("TotalEntries mismatch")
	}
	if decoded.ValidEntries != result.ValidEntries {
		t.Errorf("ValidEntries mismatch")
	}
	if len(decoded.InvalidEntries) != 2 {
		t.Errorf("expected 2 invalid entries, got %d", len(decoded.InvalidEntries))
	}
}
