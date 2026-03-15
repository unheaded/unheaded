// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package micromanager

import (
	"fmt"
	"sync"
	"testing"
)

// TestNewStore creates a store successfully
func TestNewStore(t *testing.T) {
	store := NewStore()
	if store == nil {
		t.Fatal("NewStore returned nil")
	}
	if store.Count() != 0 {
		t.Errorf("Count = %d, want 0", store.Count())
	}
}

// TestCreate_HappyPath adds a valid task
func TestCreate_HappyPath(t *testing.T) {
	store := NewStore()
	task := NewTask("task-1", "Test task", "owner")

	err := store.Create(task)
	if err != nil {
		t.Fatalf("Create() = %v, want nil", err)
	}
	if store.Count() != 1 {
		t.Errorf("Count = %d, want 1", store.Count())
	}

	retrieved, err := store.Get("task-1")
	if err != nil {
		t.Fatalf("Get() = %v, want nil", err)
	}
	if retrieved.ID != "task-1" {
		t.Errorf("Retrieved task ID = %s, want task-1", retrieved.ID)
	}
}

// TestCreate_NilTask rejects nil task
func TestCreate_NilTask(t *testing.T) {
	store := NewStore()
	err := store.Create(nil)
	if err != ErrNilTask {
		t.Errorf("Create(nil) = %v, want %v", err, ErrNilTask)
	}
}

// TestCreate_InvalidTask rejects invalid task
func TestCreate_InvalidTask(t *testing.T) {
	store := NewStore()
	task := &Task{
		ID:       "task-1",
		Title:    "",  // Invalid: empty title
		Owner:    "owner",
		Priority: 3,
	}
	err := store.Create(task)
	if err != ErrEmptyTitle {
		t.Errorf("Create(invalid) = %v, want %v", err, ErrEmptyTitle)
	}
}

// TestCreate_DuplicateID rejects duplicate task ID
func TestCreate_DuplicateID(t *testing.T) {
	store := NewStore()
	task1 := NewTask("task-1", "First", "owner")
	task2 := NewTask("task-1", "Second", "owner")

	err := store.Create(task1)
	if err != nil {
		t.Fatalf("Create(task1) = %v, want nil", err)
	}

	err = store.Create(task2)
	if err == nil {
		t.Error("Create(duplicate) should return error")
	}
}

// TestGet_HappyPath retrieves a task
func TestGet_HappyPath(t *testing.T) {
	store := NewStore()
	original := NewTask("task-1", "Test", "owner")
	store.Create(original)

	task, err := store.Get("task-1")
	if err != nil {
		t.Fatalf("Get() = %v, want nil", err)
	}
	if task.ID != "task-1" {
		t.Errorf("ID = %s, want task-1", task.ID)
	}
}

// TestGet_EmptyID rejects empty ID
func TestGet_EmptyID(t *testing.T) {
	store := NewStore()
	_, err := store.Get("")
	if err != ErrEmptyID {
		t.Errorf("Get(\"\") = %v, want %v", err, ErrEmptyID)
	}
}

// TestGet_NotFound returns error for missing task
func TestGet_NotFound(t *testing.T) {
	store := NewStore()
	_, err := store.Get("nonexistent")
	if err != ErrTaskNotFound {
		t.Errorf("Get(nonexistent) = %v, want %v", err, ErrTaskNotFound)
	}
}

// TestUpdate_HappyPath modifies a task
func TestUpdate_HappyPath(t *testing.T) {
	store := NewStore()
	task := NewTask("task-1", "Original", "owner")
	store.Create(task)

	task.Title = "Updated"
	err := store.Update(task)
	if err != nil {
		t.Fatalf("Update() = %v, want nil", err)
	}

	retrieved, _ := store.Get("task-1")
	if retrieved.Title != "Updated" {
		t.Errorf("Title = %s, want Updated", retrieved.Title)
	}
}

// TestUpdate_NilTask rejects nil task
func TestUpdate_NilTask(t *testing.T) {
	store := NewStore()
	err := store.Update(nil)
	if err != ErrNilTask {
		t.Errorf("Update(nil) = %v, want %v", err, ErrNilTask)
	}
}

// TestUpdate_InvalidTask rejects invalid task
func TestUpdate_InvalidTask(t *testing.T) {
	store := NewStore()
	task := NewTask("task-1", "Test", "owner")
	store.Create(task)

	task.Title = "" // Invalid
	err := store.Update(task)
	if err != ErrEmptyTitle {
		t.Errorf("Update(invalid) = %v, want %v", err, ErrEmptyTitle)
	}
}

// TestUpdate_NotFound returns error for missing task
func TestUpdate_NotFound(t *testing.T) {
	store := NewStore()
	task := NewTask("task-1", "Test", "owner")
	err := store.Update(task)
	if err != ErrTaskNotFound {
		t.Errorf("Update(notfound) = %v, want %v", err, ErrTaskNotFound)
	}
}

// TestDelete_HappyPath removes a task
func TestDelete_HappyPath(t *testing.T) {
	store := NewStore()
	task := NewTask("task-1", "Test", "owner")
	store.Create(task)

	err := store.Delete("task-1")
	if err != nil {
		t.Fatalf("Delete() = %v, want nil", err)
	}
	if store.Count() != 0 {
		t.Errorf("Count = %d, want 0", store.Count())
	}

	_, err = store.Get("task-1")
	if err != ErrTaskNotFound {
		t.Errorf("Get after delete = %v, want %v", err, ErrTaskNotFound)
	}
}

// TestDelete_EmptyID rejects empty ID
func TestDelete_EmptyID(t *testing.T) {
	store := NewStore()
	err := store.Delete("")
	if err != ErrEmptyID {
		t.Errorf("Delete(\"\") = %v, want %v", err, ErrEmptyID)
	}
}

// TestDelete_NotFound returns error for missing task
func TestDelete_NotFound(t *testing.T) {
	store := NewStore()
	err := store.Delete("nonexistent")
	if err != ErrTaskNotFound {
		t.Errorf("Delete(nonexistent) = %v, want %v", err, ErrTaskNotFound)
	}
}

// TestListAll returns all tasks
func TestListAll(t *testing.T) {
	store := NewStore()
	task1 := NewTask("task-1", "First", "owner1")
	task2 := NewTask("task-2", "Second", "owner2")
	store.Create(task1)
	store.Create(task2)

	tasks := store.ListAll()
	if len(tasks) != 2 {
		t.Errorf("ListAll length = %d, want 2", len(tasks))
	}
}

// TestListAll_Empty returns empty slice when no tasks
func TestListAll_Empty(t *testing.T) {
	store := NewStore()
	tasks := store.ListAll()
	if len(tasks) != 0 {
		t.Errorf("ListAll length = %d, want 0", len(tasks))
	}
	if tasks == nil {
		t.Error("ListAll should return non-nil slice, not nil")
	}
}

// TestListByStatus filters tasks by status
func TestListByStatus(t *testing.T) {
	store := NewStore()
	task1 := NewTask("task-1", "Pending", "owner")
	task2 := NewTask("task-2", "InProgress", "owner")
	task2.Status = StatusInProgress
	task3 := NewTask("task-3", "Completed", "owner")
	task3.Status = StatusCompleted

	store.Create(task1)
	store.Create(task2)
	store.Create(task3)

	pending, _ := store.ListByStatus(StatusPending)
	if len(pending) != 1 {
		t.Errorf("ListByStatus(pending) length = %d, want 1", len(pending))
	}

	inProgress, _ := store.ListByStatus(StatusInProgress)
	if len(inProgress) != 1 {
		t.Errorf("ListByStatus(in_progress) length = %d, want 1", len(inProgress))
	}

	completed, _ := store.ListByStatus(StatusCompleted)
	if len(completed) != 1 {
		t.Errorf("ListByStatus(completed) length = %d, want 1", len(completed))
	}
}

// TestListByStatus_InvalidStatus rejects invalid status
func TestListByStatus_InvalidStatus(t *testing.T) {
	store := NewStore()
	_, err := store.ListByStatus(TaskStatus("invalid"))
	if err != ErrInvalidStatus {
		t.Errorf("ListByStatus(invalid) = %v, want %v", err, ErrInvalidStatus)
	}
}

// TestListByOwner filters tasks by owner
func TestListByOwner(t *testing.T) {
	store := NewStore()
	task1 := NewTask("task-1", "Owner1 task", "owner1")
	task2 := NewTask("task-2", "Owner2 task", "owner2")
	task3 := NewTask("task-3", "Owner1 task 2", "owner1")

	store.Create(task1)
	store.Create(task2)
	store.Create(task3)

	owner1Tasks, _ := store.ListByOwner("owner1")
	if len(owner1Tasks) != 2 {
		t.Errorf("ListByOwner(owner1) length = %d, want 2", len(owner1Tasks))
	}

	owner2Tasks, _ := store.ListByOwner("owner2")
	if len(owner2Tasks) != 1 {
		t.Errorf("ListByOwner(owner2) length = %d, want 1", len(owner2Tasks))
	}
}

// TestListByOwner_EmptyOwner rejects empty owner
func TestListByOwner_EmptyOwner(t *testing.T) {
	store := NewStore()
	_, err := store.ListByOwner("")
	if err != ErrEmptyOwner {
		t.Errorf("ListByOwner(\"\") = %v, want %v", err, ErrEmptyOwner)
	}
}

// TestListByOwner_NotFound returns empty slice when no matches
func TestListByOwner_NotFound(t *testing.T) {
	store := NewStore()
	task := NewTask("task-1", "Test", "owner1")
	store.Create(task)

	tasks, _ := store.ListByOwner("nonexistent")
	if len(tasks) != 0 {
		t.Errorf("ListByOwner(nonexistent) length = %d, want 0", len(tasks))
	}
}

// TestCount returns correct count
func TestCount(t *testing.T) {
	store := NewStore()
	if store.Count() != 0 {
		t.Errorf("Count = %d, want 0", store.Count())
	}

	store.Create(NewTask("task-1", "Test", "owner"))
	if store.Count() != 1 {
		t.Errorf("Count = %d, want 1", store.Count())
	}

	store.Create(NewTask("task-2", "Test", "owner"))
	if store.Count() != 2 {
		t.Errorf("Count = %d, want 2", store.Count())
	}

	store.Delete("task-1")
	if store.Count() != 1 {
		t.Errorf("Count = %d, want 1", store.Count())
	}
}

// TestStoreError_Error verifies storeError message
func TestStoreError_Error(t *testing.T) {
	err := TaskIDAlreadyExists("task-42")
	msg := err.Error()
	if msg == "" {
		t.Fatal("Error() returned empty string")
	}
	if msg != "task id already exists: task-42" {
		t.Errorf("Error() = %s, want 'task id already exists: task-42'", msg)
	}
}

// TestCreate_InvalidStatus rejects task with invalid status
func TestCreate_InvalidStatus(t *testing.T) {
	store := NewStore()
	task := NewTask("task-1", "Test", "owner")
	task.Status = TaskStatus("bogus")
	err := store.Create(task)
	if err != ErrInvalidStatus {
		t.Errorf("Create(invalid status) = %v, want %v", err, ErrInvalidStatus)
	}
}

// TestUpdate_InvalidPriority rejects task with invalid priority
func TestUpdate_InvalidPriority(t *testing.T) {
	store := NewStore()
	task := NewTask("task-1", "Test", "owner")
	store.Create(task)

	task.Priority = 99
	err := store.Update(task)
	if err != ErrInvalidPriority {
		t.Errorf("Update(invalid priority) = %v, want %v", err, ErrInvalidPriority)
	}
}

// TestListByStatus_Empty returns empty slice when no tasks match
func TestListByStatus_Empty(t *testing.T) {
	store := NewStore()
	task := NewTask("task-1", "Test", "owner")
	store.Create(task)

	blocked, err := store.ListByStatus(StatusBlocked)
	if err != nil {
		t.Fatalf("ListByStatus(blocked) error = %v", err)
	}
	if len(blocked) != 0 {
		t.Errorf("ListByStatus(blocked) length = %d, want 0", len(blocked))
	}
}

// TestListByStatus_Blocked returns blocked tasks
func TestListByStatus_Blocked(t *testing.T) {
	store := NewStore()
	task := NewTask("task-1", "Test", "owner")
	task.Status = StatusBlocked
	store.Create(task)

	blocked, err := store.ListByStatus(StatusBlocked)
	if err != nil {
		t.Fatalf("ListByStatus(blocked) error = %v", err)
	}
	if len(blocked) != 1 {
		t.Errorf("ListByStatus(blocked) length = %d, want 1", len(blocked))
	}
}

// TestConcurrentReadWrite tests concurrent reads and writes
func TestConcurrentReadWrite(t *testing.T) {
	store := NewStore()
	// Seed a task
	task := NewTask("seed-1", "Seed", "owner")
	store.Create(task)

	var wg sync.WaitGroup
	// Concurrent reads
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = store.ListAll()
			_ = store.Count()
			_, _ = store.ListByStatus(StatusPending)
			_, _ = store.ListByOwner("owner")
		}()
	}
	// Concurrent writes
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			t := NewTask(fmt.Sprintf("rw-%d", n), "Task", "owner")
			_ = store.Create(t)
		}(i)
	}
	wg.Wait()
}

// TestConcurrentAccess tests store under concurrent load
func TestConcurrentAccess(t *testing.T) {
	store := NewStore()
	var wg sync.WaitGroup
	numGoroutines := 100

	// Concurrent creates
	for i := 1; i <= numGoroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			task := NewTask("task-"+string(rune(n)), "Test", "owner")
			_ = store.Create(task)
		}(i)
	}

	wg.Wait()

	// Verify all tasks were created
	if store.Count() != numGoroutines {
		t.Errorf("Count = %d, want %d", store.Count(), numGoroutines)
	}

	// Concurrent reads
	for i := 1; i <= 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, _ = store.Get("task-" + string(rune(n)))
		}(i)
	}

	wg.Wait()

	// Concurrent deletes
	for i := 1; i <= 25; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = store.Delete("task-" + string(rune(n)))
		}(i)
	}

	wg.Wait()

	// Should have deleted 25
	if store.Count() != numGoroutines-25 {
		t.Errorf("Count after deletes = %d, want %d", store.Count(), numGoroutines-25)
	}
}
