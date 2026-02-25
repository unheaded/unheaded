// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ============================================================================
// HELPERS
// ============================================================================

func createTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test_kanban.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func createSampleTask(id string) *Task {
	now := time.Now()
	return &Task{
		ID:          id,
		Title:       "Test Task " + id,
		Description: "Description for " + id,
		Status:      "todo",
		Type:        "task",
		Owner:       "Architect",
		Progress:    42,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// ============================================================================
// CONSTRUCTOR TESTS
// ============================================================================

func TestNewStore_HappyPath(t *testing.T) {
	store := createTestStore(t)
	if store == nil {
		t.Fatal("expected non-nil store")
	}
}

func TestNewStore_EmptyPath_ReturnsError(t *testing.T) {
	_, err := NewStore("")
	if err != ErrStoreEmptyPath {
		t.Errorf("expected ErrStoreEmptyPath, got: %v", err)
	}
}

func TestNewStore_CreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "nested", "deep", "kanban.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore with nested path failed: %v", err)
	}
	defer store.Close()

	// Verify the directory was created
	if _, err := os.Stat(filepath.Dir(dbPath)); os.IsNotExist(err) {
		t.Error("expected parent directory to be created")
	}
}

// ============================================================================
// SEED TESTS
// ============================================================================

func TestSeedIfEmpty_SeedsOnEmptyDB(t *testing.T) {
	store := createTestStore(t)

	count, err := store.SeedIfEmpty()
	if err != nil {
		t.Fatalf("SeedIfEmpty failed: %v", err)
	}

	if count == 0 {
		t.Error("expected seed to insert tasks, got 0")
	}

	// Verify tasks actually in DB
	dbCount, err := store.TaskCount()
	if err != nil {
		t.Fatalf("TaskCount failed: %v", err)
	}
	if dbCount != count {
		t.Errorf("TaskCount = %d, SeedIfEmpty returned %d", dbCount, count)
	}
}

func TestSeedIfEmpty_SkipsOnNonEmptyDB(t *testing.T) {
	store := createTestStore(t)

	// Seed once
	firstCount, err := store.SeedIfEmpty()
	if err != nil {
		t.Fatalf("first SeedIfEmpty failed: %v", err)
	}

	// Seed again — should be a no-op
	secondCount, err := store.SeedIfEmpty()
	if err != nil {
		t.Fatalf("second SeedIfEmpty failed: %v", err)
	}

	if secondCount != firstCount {
		t.Errorf("second seed should return same count: got %d, want %d", secondCount, firstCount)
	}

	// Verify no duplicates
	dbCount, err := store.TaskCount()
	if err != nil {
		t.Fatalf("TaskCount failed: %v", err)
	}
	if dbCount != firstCount {
		t.Errorf("expected %d tasks after double-seed, got %d", firstCount, dbCount)
	}
}

// ============================================================================
// SAVE TASK TESTS
// ============================================================================

func TestSaveTask_HappyPath(t *testing.T) {
	store := createTestStore(t)
	task := createSampleTask("test-save-1")

	if err := store.SaveTask(task); err != nil {
		t.Fatalf("SaveTask failed: %v", err)
	}

	// Verify retrieval
	retrieved, err := store.GetTask("test-save-1")
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}

	if retrieved.Title != task.Title {
		t.Errorf("title = %q, want %q", retrieved.Title, task.Title)
	}
	if retrieved.Progress != 42 {
		t.Errorf("progress = %d, want 42", retrieved.Progress)
	}
	if retrieved.Owner != "Architect" {
		t.Errorf("owner = %q, want Architect", retrieved.Owner)
	}
}

func TestSaveTask_NilTask_ReturnsError(t *testing.T) {
	store := createTestStore(t)
	if err := store.SaveTask(nil); err != ErrStoreNilTask {
		t.Errorf("expected ErrStoreNilTask, got: %v", err)
	}
}

func TestSaveTask_EmptyID_ReturnsError(t *testing.T) {
	store := createTestStore(t)
	task := &Task{Title: "No ID"}
	if err := store.SaveTask(task); err != ErrStoreEmptyTaskID {
		t.Errorf("expected ErrStoreEmptyTaskID, got: %v", err)
	}
}

func TestSaveTask_Upsert_UpdatesExisting(t *testing.T) {
	store := createTestStore(t)
	task := createSampleTask("upsert-test")

	// First save
	if err := store.SaveTask(task); err != nil {
		t.Fatalf("first SaveTask failed: %v", err)
	}

	// Modify and save again (upsert)
	task.Title = "Updated Title"
	task.Progress = 99
	if err := store.SaveTask(task); err != nil {
		t.Fatalf("second SaveTask failed: %v", err)
	}

	// Verify update
	retrieved, err := store.GetTask("upsert-test")
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}

	if retrieved.Title != "Updated Title" {
		t.Errorf("title = %q, want 'Updated Title'", retrieved.Title)
	}
	if retrieved.Progress != 99 {
		t.Errorf("progress = %d, want 99", retrieved.Progress)
	}

	// Verify no duplicates
	count, err := store.TaskCount()
	if err != nil {
		t.Fatalf("TaskCount failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 task after upsert, got %d", count)
	}
}

// ============================================================================
// GET TASK TESTS
// ============================================================================

func TestGetTask_NotFound_ReturnsError(t *testing.T) {
	store := createTestStore(t)
	_, err := store.GetTask("nonexistent")
	if err != ErrStoreNotFound {
		t.Errorf("expected ErrStoreNotFound, got: %v", err)
	}
}

func TestGetTask_EmptyID_ReturnsError(t *testing.T) {
	store := createTestStore(t)
	_, err := store.GetTask("")
	if err != ErrStoreEmptyTaskID {
		t.Errorf("expected ErrStoreEmptyTaskID, got: %v", err)
	}
}

// ============================================================================
// GET ALL TASKS TESTS
// ============================================================================

func TestGetAllTasks_EmptyDB_ReturnsNil(t *testing.T) {
	store := createTestStore(t)
	tasks, err := store.GetAllTasks()
	if err != nil {
		t.Fatalf("GetAllTasks failed: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks on empty DB, got %d", len(tasks))
	}
}

func TestGetAllTasks_ReturnsSavedTasks(t *testing.T) {
	store := createTestStore(t)

	// Save 3 tasks
	for i := 0; i < 3; i++ {
		task := createSampleTask(fmt.Sprintf("list-test-%d", i))
		if err := store.SaveTask(task); err != nil {
			t.Fatalf("SaveTask %d failed: %v", i, err)
		}
	}

	tasks, err := store.GetAllTasks()
	if err != nil {
		t.Fatalf("GetAllTasks failed: %v", err)
	}
	if len(tasks) != 3 {
		t.Errorf("expected 3 tasks, got %d", len(tasks))
	}
}

// ============================================================================
// DELETE TASK TESTS
// ============================================================================

func TestDeleteTask_HappyPath(t *testing.T) {
	store := createTestStore(t)
	task := createSampleTask("delete-me")
	store.SaveTask(task)

	if err := store.DeleteTask("delete-me"); err != nil {
		t.Fatalf("DeleteTask failed: %v", err)
	}

	// Verify gone
	_, err := store.GetTask("delete-me")
	if err != ErrStoreNotFound {
		t.Errorf("expected ErrStoreNotFound after delete, got: %v", err)
	}
}

func TestDeleteTask_NotFound_ReturnsError(t *testing.T) {
	store := createTestStore(t)
	err := store.DeleteTask("ghost")
	if err != ErrStoreNotFound {
		t.Errorf("expected ErrStoreNotFound, got: %v", err)
	}
}

func TestDeleteTask_EmptyID_ReturnsError(t *testing.T) {
	store := createTestStore(t)
	err := store.DeleteTask("")
	if err != ErrStoreEmptyTaskID {
		t.Errorf("expected ErrStoreEmptyTaskID, got: %v", err)
	}
}

// ============================================================================
// PERSISTENCE TEST — THE WHOLE POINT
// ============================================================================

func TestStore_PersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "persist_test.db")

	// Open, seed, save a custom task, close
	store1, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("first NewStore failed: %v", err)
	}
	store1.SeedIfEmpty()
	custom := createSampleTask("custom-persist")
	custom.Title = "I Survived a Restart"
	store1.SaveTask(custom)
	store1.Close()

	// Reopen — should find our task
	store2, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("second NewStore failed: %v", err)
	}
	defer store2.Close()

	retrieved, err := store2.GetTask("custom-persist")
	if err != nil {
		t.Fatalf("GetTask after reopen failed: %v", err)
	}
	if retrieved.Title != "I Survived a Restart" {
		t.Errorf("title = %q, want 'I Survived a Restart'", retrieved.Title)
	}

	// Seed should NOT re-run (DB is non-empty)
	count, _ := store2.TaskCount()
	seedCount, _ := store2.SeedIfEmpty()
	afterSeedCount, _ := store2.TaskCount()

	if afterSeedCount != count {
		t.Errorf("seed re-ran on non-empty DB: before=%d, after=%d", count, afterSeedCount)
	}
	if seedCount != count {
		t.Errorf("SeedIfEmpty returned %d, expected existing count %d", seedCount, count)
	}
}
