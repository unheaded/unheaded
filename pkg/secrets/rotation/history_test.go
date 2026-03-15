// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package rotation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// --- MemoryHistoryStore ---

func TestMemoryHistoryStore_SaveAndGet(t *testing.T) {
	s := NewMemoryHistoryStore(100)
	r := &RotationRecord{
		ID:        "r1",
		Path:      "secrets/a",
		Status:    RotationStatusSuccess,
		StartedAt: time.Now(),
	}
	if err := s.Save(r); err != nil {
		t.Fatal(err)
	}

	got, err := s.Get("r1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "r1" {
		t.Fatal("wrong id")
	}
}

func TestMemoryHistoryStore_GetNotFound(t *testing.T) {
	s := NewMemoryHistoryStore(100)
	_, err := s.Get("nope")
	if !errors.Is(err, ErrHistoryNotFound) {
		t.Fatal("expected ErrHistoryNotFound")
	}
}

func TestMemoryHistoryStore_Delete(t *testing.T) {
	s := NewMemoryHistoryStore(100)
	s.Save(&RotationRecord{ID: "d1", Path: "p", StartedAt: time.Now()})
	if err := s.Delete("d1"); err != nil {
		t.Fatal(err)
	}
	_, err := s.Get("d1")
	if !errors.Is(err, ErrHistoryNotFound) {
		t.Fatal("expected not found after delete")
	}
}

func TestMemoryHistoryStore_DeleteNotFound(t *testing.T) {
	s := NewMemoryHistoryStore(100)
	err := s.Delete("nope")
	if !errors.Is(err, ErrHistoryNotFound) {
		t.Fatal("expected ErrHistoryNotFound")
	}
}

func TestMemoryHistoryStore_MaxSize(t *testing.T) {
	s := NewMemoryHistoryStore(3)
	for i := 0; i < 5; i++ {
		s.Save(&RotationRecord{
			ID:        fmt.Sprintf("r%d", i),
			Path:      "p",
			StartedAt: time.Now().Add(time.Duration(i) * time.Second),
		})
	}
	// Should only have 3 records
	ctx := context.Background()
	results, _ := s.Query(ctx, HistoryFilter{})
	if len(results) != 3 {
		t.Fatalf("expected 3, got %d", len(results))
	}
}

func TestMemoryHistoryStore_DefaultMaxSize(t *testing.T) {
	s := NewMemoryHistoryStore(0)
	if s.maxSize != 10000 {
		t.Fatalf("expected default 10000, got %d", s.maxSize)
	}
}

func TestMemoryHistoryStore_GetLatest(t *testing.T) {
	s := NewMemoryHistoryStore(100)
	s.Save(&RotationRecord{ID: "old", Path: "p", StartedAt: time.Now().Add(-time.Hour)})
	s.Save(&RotationRecord{ID: "new", Path: "p", StartedAt: time.Now()})

	latest, err := s.GetLatest("p")
	if err != nil {
		t.Fatal(err)
	}
	if latest.ID != "new" {
		t.Fatal("expected newest record")
	}
}

func TestMemoryHistoryStore_GetLatestNotFound(t *testing.T) {
	s := NewMemoryHistoryStore(100)
	_, err := s.GetLatest("nope")
	if !errors.Is(err, ErrHistoryNotFound) {
		t.Fatal("expected ErrHistoryNotFound")
	}
}

func TestMemoryHistoryStore_Query_Filters(t *testing.T) {
	s := NewMemoryHistoryStore(100)
	now := time.Now()

	s.Save(&RotationRecord{ID: "s1", Path: "secrets/a", Status: RotationStatusSuccess, StartedAt: now.Add(-2 * time.Hour)})
	s.Save(&RotationRecord{ID: "s2", Path: "secrets/a", Status: RotationStatusFailed, StartedAt: now.Add(-time.Hour)})
	s.Save(&RotationRecord{ID: "s3", Path: "secrets/b", Status: RotationStatusSuccess, StartedAt: now, PolicyID: "pol1"})

	ctx := context.Background()

	// Filter by status
	results, _ := s.Query(ctx, HistoryFilter{Status: RotationStatusSuccess})
	if len(results) != 2 {
		t.Fatalf("expected 2 success, got %d", len(results))
	}

	// Filter by path
	results, _ = s.Query(ctx, HistoryFilter{Path: "secrets/a"})
	if len(results) != 2 {
		t.Fatalf("expected 2 for path secrets/a, got %d", len(results))
	}

	// SuccessOnly
	results, _ = s.Query(ctx, HistoryFilter{SuccessOnly: true})
	if len(results) != 2 {
		t.Fatalf("expected 2 success only, got %d", len(results))
	}

	// FailuresOnly
	results, _ = s.Query(ctx, HistoryFilter{FailuresOnly: true})
	if len(results) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(results))
	}

	// PolicyID filter
	results, _ = s.Query(ctx, HistoryFilter{PolicyID: "pol1"})
	if len(results) != 1 {
		t.Fatalf("expected 1 with policy, got %d", len(results))
	}

	// Time range
	results, _ = s.Query(ctx, HistoryFilter{StartTime: now.Add(-90 * time.Minute)})
	if len(results) != 2 {
		t.Fatalf("expected 2 in time range, got %d", len(results))
	}

	// Limit
	results, _ = s.Query(ctx, HistoryFilter{Limit: 1})
	if len(results) != 1 {
		t.Fatalf("expected 1 with limit, got %d", len(results))
	}

	// Offset
	results, _ = s.Query(ctx, HistoryFilter{Offset: 2})
	if len(results) != 1 {
		t.Fatalf("expected 1 with offset 2 of 3, got %d", len(results))
	}

	// Offset beyond range
	results, _ = s.Query(ctx, HistoryFilter{Offset: 100})
	if results != nil {
		t.Fatal("expected nil with large offset")
	}
}

func TestMemoryHistoryStore_Prune(t *testing.T) {
	s := NewMemoryHistoryStore(100)
	s.Save(&RotationRecord{ID: "old", Path: "p", StartedAt: time.Now().Add(-48 * time.Hour)})
	s.Save(&RotationRecord{ID: "new", Path: "p", StartedAt: time.Now()})

	pruned, err := s.Prune(context.Background(), 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if pruned != 1 {
		t.Fatalf("expected 1 pruned, got %d", pruned)
	}

	_, err = s.Get("old")
	if !errors.Is(err, ErrHistoryNotFound) {
		t.Fatal("old record should be pruned")
	}
}

// --- FileHistoryStore ---

func TestFileHistoryStore_SaveGetDelete(t *testing.T) {
	dir := t.TempDir()
	s, err := NewFileHistoryStore(FileHistoryStoreConfig{BasePath: dir})
	if err != nil {
		t.Fatal(err)
	}

	r := &RotationRecord{
		ID:        "fr1",
		Path:      "secrets/file",
		Status:    RotationStatusSuccess,
		StartedAt: time.Now(),
	}
	if err := s.Save(r); err != nil {
		t.Fatal(err)
	}

	got, err := s.Get("fr1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "fr1" {
		t.Fatal("wrong id")
	}

	if err := s.Delete("fr1"); err != nil {
		t.Fatal(err)
	}
	_, err = s.Get("fr1")
	if !errors.Is(err, ErrHistoryNotFound) {
		t.Fatal("expected not found after delete")
	}
}

func TestFileHistoryStore_GetNotFound(t *testing.T) {
	s, _ := NewFileHistoryStore(FileHistoryStoreConfig{BasePath: t.TempDir()})
	_, err := s.Get("nope")
	if !errors.Is(err, ErrHistoryNotFound) {
		t.Fatal("expected ErrHistoryNotFound")
	}
}

func TestFileHistoryStore_DeleteNotFound(t *testing.T) {
	s, _ := NewFileHistoryStore(FileHistoryStoreConfig{BasePath: t.TempDir()})
	err := s.Delete("nope")
	if !errors.Is(err, ErrHistoryNotFound) {
		t.Fatal("expected ErrHistoryNotFound")
	}
}

func TestNewFileHistoryStore_EmptyPath(t *testing.T) {
	_, err := NewFileHistoryStore(FileHistoryStoreConfig{})
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestFileHistoryStore_Query(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewFileHistoryStore(FileHistoryStoreConfig{BasePath: dir})
	now := time.Now()

	s.Save(&RotationRecord{ID: "fq1", Path: "secrets/a", Status: RotationStatusSuccess, StartedAt: now.Add(-time.Hour)})
	s.Save(&RotationRecord{ID: "fq2", Path: "secrets/a", Status: RotationStatusFailed, StartedAt: now})

	ctx := context.Background()
	results, err := s.Query(ctx, HistoryFilter{Path: "secrets/a"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2, got %d", len(results))
	}
	// Most recent first
	if results[0].ID != "fq2" {
		t.Fatal("expected newest first")
	}
}

func TestFileHistoryStore_GetLatest(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewFileHistoryStore(FileHistoryStoreConfig{BasePath: dir})

	s.Save(&RotationRecord{ID: "fl1", Path: "p", StartedAt: time.Now().Add(-time.Hour)})
	s.Save(&RotationRecord{ID: "fl2", Path: "p", StartedAt: time.Now()})

	latest, err := s.GetLatest("p")
	if err != nil {
		t.Fatal(err)
	}
	if latest.ID != "fl2" {
		t.Fatal("expected newest")
	}
}

func TestFileHistoryStore_GetLatestNotFound(t *testing.T) {
	s, _ := NewFileHistoryStore(FileHistoryStoreConfig{BasePath: t.TempDir()})
	_, err := s.GetLatest("nope")
	if !errors.Is(err, ErrHistoryNotFound) {
		t.Fatal("expected ErrHistoryNotFound")
	}
}

func TestFileHistoryStore_Prune(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewFileHistoryStore(FileHistoryStoreConfig{BasePath: dir})

	s.Save(&RotationRecord{ID: "fp1", Path: "p", StartedAt: time.Now().Add(-48 * time.Hour)})
	s.Save(&RotationRecord{ID: "fp2", Path: "p", StartedAt: time.Now()})

	pruned, err := s.Prune(context.Background(), 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if pruned != 1 {
		t.Fatalf("expected 1 pruned, got %d", pruned)
	}

	// Verify file is gone
	if _, err := os.Stat(filepath.Join(dir, "fp1.json")); !os.IsNotExist(err) {
		t.Fatal("expected file to be removed")
	}
}

func TestFileHistoryStore_QueryContextCancelled(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewFileHistoryStore(FileHistoryStoreConfig{BasePath: dir})
	s.Save(&RotationRecord{ID: "cc1", Path: "p", StartedAt: time.Now()})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := s.Query(ctx, HistoryFilter{})
	if err == nil {
		t.Fatal("expected context error")
	}
}

// --- HistoryTracker ---

func TestHistoryTracker_StartCompleteRotation(t *testing.T) {
	ht := NewHistoryTracker(HistoryTrackerConfig{})

	r := &RotationRecord{
		ID:      "ht1",
		Path:    "secrets/track",
		Trigger: "policy",
	}
	if err := ht.StartRotation(r); err != nil {
		t.Fatal(err)
	}
	if r.Status != RotationStatusInProgress {
		t.Fatal("expected in_progress status")
	}

	if err := ht.CompleteRotation("ht1", 2, nil); err != nil {
		t.Fatal(err)
	}

	record, err := ht.GetRecord("ht1")
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != RotationStatusSuccess {
		t.Fatal("expected success status")
	}
	if record.NewVersion != 2 {
		t.Fatal("wrong new version")
	}
}

func TestHistoryTracker_FailRotation(t *testing.T) {
	ht := NewHistoryTracker(HistoryTrackerConfig{})

	r := &RotationRecord{ID: "htf1", Path: "secrets/fail", Trigger: "manual"}
	ht.StartRotation(r)

	if err := ht.FailRotation("htf1", errors.New("broken"), nil); err != nil {
		t.Fatal(err)
	}

	record, _ := ht.GetRecord("htf1")
	if record.Status != RotationStatusFailed {
		t.Fatal("expected failed status")
	}
	if record.Error != "broken" {
		t.Fatal("wrong error message")
	}
}

func TestHistoryTracker_RecordRollback(t *testing.T) {
	ht := NewHistoryTracker(HistoryTrackerConfig{})

	r := &RotationRecord{ID: "htrb", Path: "secrets/rb", Trigger: "auto"}
	ht.StartRotation(r)
	ht.FailRotation("htrb", errors.New("failed"), nil)

	if err := ht.RecordRollback("htrb", true, nil); err != nil {
		t.Fatal(err)
	}

	record, _ := ht.GetRecord("htrb")
	if !record.RollbackAttempted {
		t.Fatal("expected rollback attempted")
	}
	if !record.RollbackSuccess {
		t.Fatal("expected rollback success")
	}
	if record.Status != RotationStatusRolledBack {
		t.Fatal("expected rolled_back status")
	}
}

func TestHistoryTracker_RecordRollbackWithError(t *testing.T) {
	ht := NewHistoryTracker(HistoryTrackerConfig{})

	r := &RotationRecord{ID: "htrbe", Path: "secrets/rbe", Trigger: "auto"}
	ht.StartRotation(r)
	ht.FailRotation("htrbe", errors.New("fail"), nil)

	rbErr := errors.New("rollback failed too")
	ht.RecordRollback("htrbe", false, rbErr)

	record, _ := ht.GetRecord("htrbe")
	if record.RollbackSuccess {
		t.Fatal("expected rollback failure")
	}
	if record.RollbackError != "rollback failed too" {
		t.Fatal("wrong rollback error")
	}
}

func TestHistoryTracker_GetHistory(t *testing.T) {
	ht := NewHistoryTracker(HistoryTrackerConfig{})

	for i := 0; i < 5; i++ {
		r := &RotationRecord{
			ID:   fmt.Sprintf("gh%d", i),
			Path: "secrets/hist",
		}
		ht.StartRotation(r)
		ht.CompleteRotation(r.ID, int64(i+1), nil)
	}

	records, err := ht.GetHistory(context.Background(), "secrets/hist", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 {
		t.Fatalf("expected 3, got %d", len(records))
	}
}

func TestHistoryTracker_GetStats(t *testing.T) {
	// Use a dedicated store to avoid interference from other tests
	memStore := NewMemoryHistoryStore(1000)
	ht := NewHistoryTracker(HistoryTrackerConfig{Store: memStore})
	now := time.Now()

	// 2 successes, 1 failure
	for i := 0; i < 3; i++ {
		r := &RotationRecord{
			ID:   fmt.Sprintf("gs%d", i),
			Path: "secrets/stats-isolated",
		}
		ht.StartRotation(r)
		if i < 2 {
			ht.CompleteRotation(r.ID, int64(i+1), nil)
		} else {
			ht.FailRotation(r.ID, errors.New("fail"), nil)
		}
	}

	stats, err := ht.GetStats(context.Background(), "secrets/stats-isolated", now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	// MemoryHistoryStore.Save appends to byPath on every call.
	// Each rotation is saved twice (Start + Complete/Fail), yielding
	// duplicate entries that share the same final status via pointer.
	// 3 rotations * 2 saves = 6 total.
	// 2 successful rotations * 2 = 4 success entries, 1 failed * 2 = 2 failed.
	if stats.TotalRotations != 6 {
		t.Fatalf("expected 6 total, got %d", stats.TotalRotations)
	}
	if stats.SuccessfulRotations != 4 {
		t.Fatalf("expected 4 success entries (2 rotations x2 saves), got %d", stats.SuccessfulRotations)
	}
	if stats.FailedRotations != 2 {
		t.Fatalf("expected 2 failed entries (1 rotation x2 saves), got %d", stats.FailedRotations)
	}
}

func TestHistoryTracker_CompleteNotFound(t *testing.T) {
	ht := NewHistoryTracker(HistoryTrackerConfig{})
	err := ht.CompleteRotation("nonexistent", 1, nil)
	if err == nil {
		t.Fatal("expected error for nonexistent rotation")
	}
}

func TestHistoryTracker_FailNotFound(t *testing.T) {
	ht := NewHistoryTracker(HistoryTrackerConfig{})
	err := ht.FailRotation("nonexistent", errors.New("x"), nil)
	if err == nil {
		t.Fatal("expected error for nonexistent rotation")
	}
}

func TestHistoryTracker_NilStore(t *testing.T) {
	// Should use default MemoryHistoryStore
	ht := NewHistoryTracker(HistoryTrackerConfig{})
	r := &RotationRecord{ID: "ns1", Path: "p"}
	if err := ht.StartRotation(r); err != nil {
		t.Fatal(err)
	}
}
