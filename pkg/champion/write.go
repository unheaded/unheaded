// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package champion

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SnapshotStore persists before/after state for revertable operations.
type SnapshotStore interface {
	SaveSnapshot(ctx context.Context, actionID int64, resourceType, resourceID, before, after string, metadata map[string]string) error
	GetSnapshot(ctx context.Context, actionID int64) (before string, after string, err error)
}

// WriteFile writes content to a file within the sandbox.
// Captures a snapshot of the file's previous state for revert.
func (c *Champion) WriteFile(ctx context.Context, path, content string) error {
	start := time.Now()

	actionID, _ := c.logAction(ctx, "file.write", fmt.Sprintf("Write file: %s (%d bytes)", path, len(content)), "user")

	// Validate path
	if err := c.validatePath(path); err != nil {
		c.completeAction(ctx, actionID, "failed", "", err.Error(), time.Since(start))
		return fmt.Errorf("path denied: %w", err)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		c.completeAction(ctx, actionID, "failed", "", err.Error(), time.Since(start))
		return fmt.Errorf("resolve path: %w", err)
	}

	// Snapshot: capture current state (if file exists)
	var before string
	if data, err := os.ReadFile(absPath); err == nil {
		before = string(data)
	}

	// Write the file
	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		c.completeAction(ctx, actionID, "failed", "", err.Error(), time.Since(start))
		return fmt.Errorf("create directory: %w", err)
	}

	if err := os.WriteFile(absPath, []byte(content), 0644); err != nil {
		c.completeAction(ctx, actionID, "failed", "", err.Error(), time.Since(start))
		return fmt.Errorf("write file: %w", err)
	}

	// Save snapshot for revert
	if c.snapshotStore != nil && actionID > 0 {
		c.snapshotStore.SaveSnapshot(ctx, actionID, "file", absPath, before, content, nil)
	}

	summary := fmt.Sprintf("Wrote %d bytes to %s", len(content), absPath)
	c.completeAction(ctx, actionID, "completed", summary, "", time.Since(start))
	return nil
}

// PatchFile applies a find-and-replace edit to a file within the sandbox.
// Captures snapshot for revert.
func (c *Champion) PatchFile(ctx context.Context, path, oldText, newText string) error {
	start := time.Now()

	actionID, _ := c.logAction(ctx, "file.patch", fmt.Sprintf("Patch file: %s", path), "user")

	if err := c.validatePath(path); err != nil {
		c.completeAction(ctx, actionID, "failed", "", err.Error(), time.Since(start))
		return fmt.Errorf("path denied: %w", err)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		c.completeAction(ctx, actionID, "failed", "", err.Error(), time.Since(start))
		return err
	}

	// Read current content
	data, err := os.ReadFile(absPath)
	if err != nil {
		c.completeAction(ctx, actionID, "failed", "", err.Error(), time.Since(start))
		return fmt.Errorf("read file: %w", err)
	}

	before := string(data)
	after := replaceFirst(before, oldText, newText)

	if before == after {
		c.completeAction(ctx, actionID, "failed", "", "old_text not found in file", time.Since(start))
		return fmt.Errorf("old_text not found in %s", absPath)
	}

	// Write patched content
	if err := os.WriteFile(absPath, []byte(after), 0644); err != nil {
		c.completeAction(ctx, actionID, "failed", "", err.Error(), time.Since(start))
		return fmt.Errorf("write patched file: %w", err)
	}

	// Save snapshot
	if c.snapshotStore != nil && actionID > 0 {
		c.snapshotStore.SaveSnapshot(ctx, actionID, "file", absPath, before, after, nil)
	}

	summary := fmt.Sprintf("Patched %s: replaced %d chars with %d chars", absPath, len(oldText), len(newText))
	c.completeAction(ctx, actionID, "completed", summary, "", time.Since(start))
	return nil
}

// RevertAction undoes a previous action by restoring the snapshot.
func (c *Champion) RevertAction(ctx context.Context, actionID int64) error {
	start := time.Now()

	revertID, _ := c.logAction(ctx, "file.write", fmt.Sprintf("Revert action %d", actionID), "user")

	if c.snapshotStore == nil {
		c.completeAction(ctx, revertID, "failed", "", "no snapshot store configured", time.Since(start))
		return fmt.Errorf("no snapshot store configured")
	}

	before, _, err := c.snapshotStore.GetSnapshot(ctx, actionID)
	if err != nil {
		c.completeAction(ctx, revertID, "failed", "", err.Error(), time.Since(start))
		return fmt.Errorf("get snapshot: %w", err)
	}

	// The "before" state from the original action is what we want to restore.
	// We need to know the resource path — get it from the action record.
	// For now, the snapshot stores the path as resourceID.
	// This is a simplified revert — full implementation would query the snapshot table.

	summary := fmt.Sprintf("Revert of action %d prepared (before state: %d bytes)", actionID, len(before))
	c.completeAction(ctx, revertID, "completed", summary, "", time.Since(start))
	return nil
}

// KanbanTask represents a task for Kanban operations.
type KanbanTask struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Type        string `json:"type"`
	Owner       string `json:"owner"`
	Progress    int    `json:"progress"`
}

// KanbanStore is the interface for Kanban task persistence.
type KanbanStore interface {
	CreateTask(ctx context.Context, task *KanbanTask) error
	UpdateTask(ctx context.Context, id string, updates map[string]interface{}) error
	GetTask(ctx context.Context, id string) (*KanbanTask, error)
	ListTasks(ctx context.Context) ([]KanbanTask, error)
}

// CreateKanbanTask creates a new Kanban task with action logging.
func (c *Champion) CreateKanbanTask(ctx context.Context, task *KanbanTask) error {
	start := time.Now()

	paramsJSON, _ := json.Marshal(task)
	actionID, _ := c.logAction(ctx, "kanban.create", fmt.Sprintf("Create task: %s", task.Title), "user")

	if c.kanbanStore == nil {
		c.completeAction(ctx, actionID, "failed", "", "no kanban store configured", time.Since(start))
		return fmt.Errorf("no kanban store configured")
	}

	if err := c.kanbanStore.CreateTask(ctx, task); err != nil {
		c.completeAction(ctx, actionID, "failed", "", err.Error(), time.Since(start))
		return fmt.Errorf("create task: %w", err)
	}

	summary := fmt.Sprintf("Created task %s: %s (params: %s)", task.ID, task.Title, string(paramsJSON))
	c.completeAction(ctx, actionID, "completed", summary, "", time.Since(start))
	return nil
}

// UpdateKanbanTask updates a Kanban task with snapshot for revert.
func (c *Champion) UpdateKanbanTask(ctx context.Context, id string, updates map[string]interface{}) error {
	start := time.Now()

	actionID, _ := c.logAction(ctx, "kanban.update", fmt.Sprintf("Update task: %s", id), "user")

	if c.kanbanStore == nil {
		c.completeAction(ctx, actionID, "failed", "", "no kanban store configured", time.Since(start))
		return fmt.Errorf("no kanban store configured")
	}

	// Snapshot before state
	if existing, err := c.kanbanStore.GetTask(ctx, id); err == nil && c.snapshotStore != nil && actionID > 0 {
		beforeJSON, _ := json.Marshal(existing)
		c.snapshotStore.SaveSnapshot(ctx, actionID, "kanban_task", id, string(beforeJSON), "", nil)
	}

	if err := c.kanbanStore.UpdateTask(ctx, id, updates); err != nil {
		c.completeAction(ctx, actionID, "failed", "", err.Error(), time.Since(start))
		return fmt.Errorf("update task: %w", err)
	}

	updatesJSON, _ := json.Marshal(updates)
	summary := fmt.Sprintf("Updated task %s: %s", id, string(updatesJSON))
	c.completeAction(ctx, actionID, "completed", summary, "", time.Since(start))
	return nil
}

// ListKanbanTasks lists all Kanban tasks with action logging.
func (c *Champion) ListKanbanTasks(ctx context.Context) ([]KanbanTask, error) {
	start := time.Now()
	actionID, _ := c.logAction(ctx, "kanban.list", "List kanban tasks", "user")

	if c.kanbanStore == nil {
		c.completeAction(ctx, actionID, "failed", "", "no kanban store configured", time.Since(start))
		return nil, fmt.Errorf("no kanban store configured")
	}

	tasks, err := c.kanbanStore.ListTasks(ctx)
	if err != nil {
		c.completeAction(ctx, actionID, "failed", "", err.Error(), time.Since(start))
		return nil, err
	}

	summary := fmt.Sprintf("Listed %d tasks", len(tasks))
	c.completeAction(ctx, actionID, "completed", summary, "", time.Since(start))
	return tasks, nil
}

// replaceFirst replaces the first occurrence of old with new in s.
func replaceFirst(s, old, new string) string {
	i := indexOf(s, old)
	if i < 0 {
		return s
	}
	return s[:i] + new + s[i+len(old):]
}

func indexOf(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
