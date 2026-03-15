// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package micromanager

import (
	"errors"
	"fmt"
	"time"
)

// TaskStatus represents the state of a task
type TaskStatus string

const (
	StatusPending  TaskStatus = "pending"
	StatusInProgress TaskStatus = "in_progress"
	StatusCompleted TaskStatus = "completed"
	StatusBlocked   TaskStatus = "blocked"
)

// Task represents an execution task in the micromanager service
type Task struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      TaskStatus `json:"status"`
	Priority    int       `json:"priority"` // 1-5, higher = more urgent
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Assignee    string    `json:"assignee,omitempty"`
	DueDate     *time.Time `json:"due_date,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	Owner       string    `json:"owner"` // Sprint/milestone owner
}

var (
	ErrNilTask           = errors.New("task cannot be nil")
	ErrEmptyID           = errors.New("task id cannot be empty")
	ErrEmptyTitle        = errors.New("task title cannot be empty")
	ErrInvalidPriority   = errors.New("priority must be between 1 and 5")
	ErrInvalidStatus     = errors.New("invalid task status")
	ErrTaskNotFound      = errors.New("task not found")
	ErrEmptyOwner        = errors.New("task owner cannot be empty")
)

// Validate ensures the task has valid data
func (t *Task) Validate() error {
	if t == nil {
		return ErrNilTask
	}
	if t.ID == "" {
		return ErrEmptyID
	}
	if t.Title == "" {
		return ErrEmptyTitle
	}
	if t.Owner == "" {
		return ErrEmptyOwner
	}
	if t.Priority < 1 || t.Priority > 5 {
		return ErrInvalidPriority
	}
	if !isValidStatus(t.Status) {
		return ErrInvalidStatus
	}
	return nil
}

func isValidStatus(status TaskStatus) bool {
	switch status {
	case StatusPending, StatusInProgress, StatusCompleted, StatusBlocked:
		return true
	default:
		return false
	}
}

// NewTask creates a new task with defaults
func NewTask(id, title, owner string) *Task {
	now := time.Now()
	return &Task{
		ID:        id,
		Title:     title,
		Owner:     owner,
		Status:    StatusPending,
		Priority:  3,
		CreatedAt: now,
		UpdatedAt: now,
		Tags:      []string{},
	}
}

// UpdateStatus changes the task status and updates timestamps
func (t *Task) UpdateStatus(newStatus TaskStatus) error {
	if t == nil {
		return ErrNilTask
	}
	if !isValidStatus(newStatus) {
		return ErrInvalidStatus
	}

	t.Status = newStatus
	t.UpdatedAt = time.Now()

	if newStatus == StatusCompleted {
		now := time.Now()
		t.CompletedAt = &now
	}

	return nil
}

// UpdateTitle updates the task title
func (t *Task) UpdateTitle(newTitle string) error {
	if t == nil {
		return ErrNilTask
	}
	if newTitle == "" {
		return ErrEmptyTitle
	}
	t.Title = newTitle
	t.UpdatedAt = time.Now()
	return nil
}

// UpdatePriority updates the task priority
func (t *Task) UpdatePriority(newPriority int) error {
	if t == nil {
		return ErrNilTask
	}
	if newPriority < 1 || newPriority > 5 {
		return ErrInvalidPriority
	}
	t.Priority = newPriority
	t.UpdatedAt = time.Now()
	return nil
}

// String representation for logging
func (t *Task) String() string {
	if t == nil {
		return "<nil>"
	}
	return fmt.Sprintf("Task{ID=%s, Title=%s, Status=%s, Owner=%s}", t.ID, t.Title, t.Status, t.Owner)
}
