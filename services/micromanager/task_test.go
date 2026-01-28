package micromanager

import (
	"testing"
	"time"
)

// TestNewTask verifies task creation with defaults
func TestNewTask(t *testing.T) {
	task := NewTask("task-1", "Build micromanager", "muck")

	if task == nil {
		t.Fatal("NewTask returned nil")
	}
	if task.ID != "task-1" {
		t.Errorf("ID = %s, want task-1", task.ID)
	}
	if task.Title != "Build micromanager" {
		t.Errorf("Title = %s, want Build micromanager", task.Title)
	}
	if task.Owner != "muck" {
		t.Errorf("Owner = %s, want muck", task.Owner)
	}
	if task.Status != StatusPending {
		t.Errorf("Status = %s, want %s", task.Status, StatusPending)
	}
	if task.Priority != 3 {
		t.Errorf("Priority = %d, want 3", task.Priority)
	}
	if task.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero")
	}
	if len(task.Tags) != 0 {
		t.Errorf("Tags length = %d, want 0", len(task.Tags))
	}
}

// TestValidate_HappyPath verifies valid task passes validation
func TestValidate_HappyPath(t *testing.T) {
	task := NewTask("task-1", "Test task", "owner")
	err := task.Validate()
	if err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

// TestValidate_NilTask rejects nil task
func TestValidate_NilTask(t *testing.T) {
	var task *Task
	err := task.Validate()
	if err != ErrNilTask {
		t.Errorf("Validate(nil) = %v, want %v", err, ErrNilTask)
	}
}

// TestValidate_EmptyID rejects empty ID
func TestValidate_EmptyID(t *testing.T) {
	task := &Task{
		ID:    "",
		Title: "Test",
		Owner: "owner",
	}
	err := task.Validate()
	if err != ErrEmptyID {
		t.Errorf("Validate(empty ID) = %v, want %v", err, ErrEmptyID)
	}
}

// TestValidate_EmptyTitle rejects empty title
func TestValidate_EmptyTitle(t *testing.T) {
	task := &Task{
		ID:    "task-1",
		Title: "",
		Owner: "owner",
	}
	err := task.Validate()
	if err != ErrEmptyTitle {
		t.Errorf("Validate(empty Title) = %v, want %v", err, ErrEmptyTitle)
	}
}

// TestValidate_EmptyOwner rejects empty owner
func TestValidate_EmptyOwner(t *testing.T) {
	task := &Task{
		ID:    "task-1",
		Title: "Test",
		Owner: "",
	}
	err := task.Validate()
	if err != ErrEmptyOwner {
		t.Errorf("Validate(empty Owner) = %v, want %v", err, ErrEmptyOwner)
	}
}

// TestValidate_InvalidPriority_TooLow rejects priority < 1
func TestValidate_InvalidPriority_TooLow(t *testing.T) {
	task := NewTask("task-1", "Test", "owner")
	task.Priority = 0
	err := task.Validate()
	if err != ErrInvalidPriority {
		t.Errorf("Validate(priority=0) = %v, want %v", err, ErrInvalidPriority)
	}
}

// TestValidate_InvalidPriority_TooHigh rejects priority > 5
func TestValidate_InvalidPriority_TooHigh(t *testing.T) {
	task := NewTask("task-1", "Test", "owner")
	task.Priority = 6
	err := task.Validate()
	if err != ErrInvalidPriority {
		t.Errorf("Validate(priority=6) = %v, want %v", err, ErrInvalidPriority)
	}
}

// TestValidate_InvalidStatus rejects invalid status
func TestValidate_InvalidStatus(t *testing.T) {
	task := NewTask("task-1", "Test", "owner")
	task.Status = TaskStatus("invalid")
	err := task.Validate()
	if err != ErrInvalidStatus {
		t.Errorf("Validate(invalid status) = %v, want %v", err, ErrInvalidStatus)
	}
}

// TestValidate_ValidStatuses tests all valid statuses
func TestValidate_ValidStatuses(t *testing.T) {
	statuses := []TaskStatus{StatusPending, StatusInProgress, StatusCompleted, StatusBlocked}
	for _, status := range statuses {
		task := NewTask("task-1", "Test", "owner")
		task.Status = status
		err := task.Validate()
		if err != nil {
			t.Errorf("Validate(status=%s) = %v, want nil", status, err)
		}
	}
}

// TestValidate_ValidPriorities tests boundary priority values
func TestValidate_ValidPriorities(t *testing.T) {
	tests := []struct {
		priority int
		wantErr  bool
	}{
		{1, false},
		{2, false},
		{3, false},
		{4, false},
		{5, false},
		{0, true},
		{-1, true},
		{6, true},
		{100, true},
	}
	for _, tt := range tests {
		task := NewTask("task-1", "Test", "owner")
		task.Priority = tt.priority
		err := task.Validate()
		if (err != nil) != tt.wantErr {
			t.Errorf("Validate(priority=%d) error = %v, wantErr %v", tt.priority, err, tt.wantErr)
		}
	}
}

// TestUpdateStatus_HappyPath changes status successfully
func TestUpdateStatus_HappyPath(t *testing.T) {
	task := NewTask("task-1", "Test", "owner")
	originalUpdated := task.UpdatedAt

	err := task.UpdateStatus(StatusInProgress)
	if err != nil {
		t.Fatalf("UpdateStatus() = %v, want nil", err)
	}
	if task.Status != StatusInProgress {
		t.Errorf("Status = %s, want %s", task.Status, StatusInProgress)
	}
	if task.UpdatedAt.Before(originalUpdated) {
		t.Error("UpdatedAt should be updated")
	}
}

// TestUpdateStatus_ToCompleted sets CompletedAt
func TestUpdateStatus_ToCompleted(t *testing.T) {
	task := NewTask("task-1", "Test", "owner")
	err := task.UpdateStatus(StatusCompleted)
	if err != nil {
		t.Fatalf("UpdateStatus() = %v, want nil", err)
	}
	if task.Status != StatusCompleted {
		t.Errorf("Status = %s, want %s", task.Status, StatusCompleted)
	}
	if task.CompletedAt == nil {
		t.Fatal("CompletedAt should be set")
	}
	if task.CompletedAt.IsZero() {
		t.Error("CompletedAt should not be zero")
	}
}

// TestUpdateStatus_NilTask rejects nil receiver
func TestUpdateStatus_NilTask(t *testing.T) {
	var task *Task
	err := task.UpdateStatus(StatusInProgress)
	if err != ErrNilTask {
		t.Errorf("UpdateStatus(nil) = %v, want %v", err, ErrNilTask)
	}
}

// TestUpdateStatus_InvalidStatus rejects invalid status
func TestUpdateStatus_InvalidStatus(t *testing.T) {
	task := NewTask("task-1", "Test", "owner")
	err := task.UpdateStatus(TaskStatus("invalid"))
	if err != ErrInvalidStatus {
		t.Errorf("UpdateStatus(invalid) = %v, want %v", err, ErrInvalidStatus)
	}
}

// TestUpdateTitle_HappyPath updates title successfully
func TestUpdateTitle_HappyPath(t *testing.T) {
	task := NewTask("task-1", "Old title", "owner")
	originalUpdated := task.UpdatedAt
	time.Sleep(1 * time.Millisecond) // Ensure time difference

	err := task.UpdateTitle("New title")
	if err != nil {
		t.Fatalf("UpdateTitle() = %v, want nil", err)
	}
	if task.Title != "New title" {
		t.Errorf("Title = %s, want New title", task.Title)
	}
	if !task.UpdatedAt.After(originalUpdated) {
		t.Error("UpdatedAt should be updated")
	}
}

// TestUpdateTitle_EmptyTitle rejects empty title
func TestUpdateTitle_EmptyTitle(t *testing.T) {
	task := NewTask("task-1", "Title", "owner")
	err := task.UpdateTitle("")
	if err != ErrEmptyTitle {
		t.Errorf("UpdateTitle(\"\") = %v, want %v", err, ErrEmptyTitle)
	}
	if task.Title != "Title" {
		t.Error("Title should not be changed on error")
	}
}

// TestUpdateTitle_NilTask rejects nil receiver
func TestUpdateTitle_NilTask(t *testing.T) {
	var task *Task
	err := task.UpdateTitle("New title")
	if err != ErrNilTask {
		t.Errorf("UpdateTitle(nil) = %v, want %v", err, ErrNilTask)
	}
}

// TestUpdatePriority_HappyPath updates priority successfully
func TestUpdatePriority_HappyPath(t *testing.T) {
	task := NewTask("task-1", "Test", "owner")
	originalUpdated := task.UpdatedAt
	time.Sleep(1 * time.Millisecond)

	err := task.UpdatePriority(5)
	if err != nil {
		t.Fatalf("UpdatePriority() = %v, want nil", err)
	}
	if task.Priority != 5 {
		t.Errorf("Priority = %d, want 5", task.Priority)
	}
	if !task.UpdatedAt.After(originalUpdated) {
		t.Error("UpdatedAt should be updated")
	}
}

// TestUpdatePriority_InvalidLow rejects priority < 1
func TestUpdatePriority_InvalidLow(t *testing.T) {
	task := NewTask("task-1", "Test", "owner")
	original := task.Priority
	err := task.UpdatePriority(0)
	if err != ErrInvalidPriority {
		t.Errorf("UpdatePriority(0) = %v, want %v", err, ErrInvalidPriority)
	}
	if task.Priority != original {
		t.Error("Priority should not be changed on error")
	}
}

// TestUpdatePriority_InvalidHigh rejects priority > 5
func TestUpdatePriority_InvalidHigh(t *testing.T) {
	task := NewTask("task-1", "Test", "owner")
	original := task.Priority
	err := task.UpdatePriority(6)
	if err != ErrInvalidPriority {
		t.Errorf("UpdatePriority(6) = %v, want %v", err, ErrInvalidPriority)
	}
	if task.Priority != original {
		t.Error("Priority should not be changed on error")
	}
}

// TestUpdatePriority_NilTask rejects nil receiver
func TestUpdatePriority_NilTask(t *testing.T) {
	var task *Task
	err := task.UpdatePriority(3)
	if err != ErrNilTask {
		t.Errorf("UpdatePriority(nil) = %v, want %v", err, ErrNilTask)
	}
}
