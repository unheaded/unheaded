// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package champion

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// mockSnapshotStore implements SnapshotStore for testing.
type mockSnapshotStore struct {
	snapshots map[int64]struct{ before, after string }
}

func newMockSnapshotStore() *mockSnapshotStore {
	return &mockSnapshotStore{snapshots: make(map[int64]struct{ before, after string })}
}

func (m *mockSnapshotStore) SaveSnapshot(_ context.Context, actionID int64, _, _, before, after string, _ map[string]string) error {
	m.snapshots[actionID] = struct{ before, after string }{before, after}
	return nil
}

func (m *mockSnapshotStore) GetSnapshot(_ context.Context, actionID int64) (string, string, error) {
	s, ok := m.snapshots[actionID]
	if !ok {
		return "", "", nil
	}
	return s.before, s.after, nil
}

// mockKanbanStore implements KanbanStore for testing.
type mockKanbanStore struct {
	tasks map[string]*KanbanTask
}

func newMockKanbanStore() *mockKanbanStore {
	return &mockKanbanStore{tasks: make(map[string]*KanbanTask)}
}

func (m *mockKanbanStore) CreateTask(_ context.Context, task *KanbanTask) error {
	m.tasks[task.ID] = task
	return nil
}

func (m *mockKanbanStore) UpdateTask(_ context.Context, id string, updates map[string]interface{}) error {
	t, ok := m.tasks[id]
	if !ok {
		return nil
	}
	if v, ok := updates["status"].(string); ok {
		t.Status = v
	}
	if v, ok := updates["title"].(string); ok {
		t.Title = v
	}
	return nil
}

func (m *mockKanbanStore) GetTask(_ context.Context, id string) (*KanbanTask, error) {
	t, ok := m.tasks[id]
	if !ok {
		return nil, nil
	}
	return t, nil
}

func (m *mockKanbanStore) ListTasks(_ context.Context) ([]KanbanTask, error) {
	var tasks []KanbanTask
	for _, t := range m.tasks {
		tasks = append(tasks, *t)
	}
	return tasks, nil
}

func TestWriteFile_Success(t *testing.T) {
	dir := t.TempDir()
	store := &mockStore{}
	snap := newMockSnapshotStore()

	c, _ := New(Config{ProjectRoot: dir, AllowedPaths: []string{dir}}, store)
	c.WithSnapshotStore(snap)

	path := filepath.Join(dir, "test.txt")
	err := c.WriteFile(context.Background(), path, "hello kingdom")
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "hello kingdom" {
		t.Errorf("expected 'hello kingdom', got %q", string(data))
	}

	// Verify action logged
	if len(store.actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(store.actions))
	}
	if store.actions[0].ActionType != "file.write" {
		t.Errorf("expected file.write, got %s", store.actions[0].ActionType)
	}
}

func TestWriteFile_Denied(t *testing.T) {
	store := &mockStore{}
	c, _ := New(Config{ProjectRoot: "/tmp/test", AllowedPaths: []string{"/tmp/test"}}, store)

	err := c.WriteFile(context.Background(), "/etc/shadow", "hacked")
	if err == nil {
		t.Fatal("expected error writing to /etc/shadow")
	}
}

func TestWriteFile_CreatesDirectories(t *testing.T) {
	dir := t.TempDir()
	c, _ := New(Config{ProjectRoot: dir, AllowedPaths: []string{dir}}, nil)

	path := filepath.Join(dir, "sub", "deep", "file.txt")
	err := c.WriteFile(context.Background(), path, "deep write")
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "deep write" {
		t.Errorf("expected 'deep write', got %q", string(data))
	}
}

func TestWriteFile_SnapshotCaptured(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.txt")
	os.WriteFile(path, []byte("original"), 0644)

	store := &mockStore{}
	snap := newMockSnapshotStore()
	c, _ := New(Config{ProjectRoot: dir, AllowedPaths: []string{dir}}, store)
	c.WithSnapshotStore(snap)

	err := c.WriteFile(context.Background(), path, "updated")
	if err != nil {
		t.Fatal(err)
	}

	// Verify snapshot captured the original content
	if len(snap.snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snap.snapshots))
	}
	for _, s := range snap.snapshots {
		if s.before != "original" {
			t.Errorf("expected before='original', got %q", s.before)
		}
		if s.after != "updated" {
			t.Errorf("expected after='updated', got %q", s.after)
		}
	}
}

func TestPatchFile_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte("port: 8080\nhost: localhost"), 0644)

	store := &mockStore{}
	c, _ := New(Config{ProjectRoot: dir, AllowedPaths: []string{dir}}, store)

	err := c.PatchFile(context.Background(), path, "port: 8080", "port: 9090")
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "port: 9090\nhost: localhost" {
		t.Errorf("unexpected content: %q", string(data))
	}
}

func TestPatchFile_NotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte("port: 8080"), 0644)

	c, _ := New(Config{ProjectRoot: dir, AllowedPaths: []string{dir}}, &mockStore{})

	err := c.PatchFile(context.Background(), path, "nonexistent text", "replacement")
	if err == nil {
		t.Fatal("expected error for missing old_text")
	}
}

func TestCreateKanbanTask(t *testing.T) {
	store := &mockStore{}
	kanban := newMockKanbanStore()
	c, _ := New(Config{ProjectRoot: "/tmp"}, store)
	c.WithKanbanStore(kanban)

	task := &KanbanTask{ID: "task-1", Title: "Fix bug", Status: "backlog", Type: "bug"}
	err := c.CreateKanbanTask(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}

	if kanban.tasks["task-1"] == nil {
		t.Fatal("task not created in store")
	}
	if kanban.tasks["task-1"].Title != "Fix bug" {
		t.Errorf("wrong title: %s", kanban.tasks["task-1"].Title)
	}
}

func TestUpdateKanbanTask(t *testing.T) {
	store := &mockStore{}
	kanban := newMockKanbanStore()
	snap := newMockSnapshotStore()
	c, _ := New(Config{ProjectRoot: "/tmp"}, store)
	c.WithKanbanStore(kanban).WithSnapshotStore(snap)

	// Create initial task
	kanban.tasks["task-1"] = &KanbanTask{ID: "task-1", Title: "Fix bug", Status: "backlog"}

	// Update it
	err := c.UpdateKanbanTask(context.Background(), "task-1", map[string]interface{}{
		"status": "in_progress",
	})
	if err != nil {
		t.Fatal(err)
	}

	if kanban.tasks["task-1"].Status != "in_progress" {
		t.Errorf("expected in_progress, got %s", kanban.tasks["task-1"].Status)
	}

	// Verify snapshot was captured
	if len(snap.snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snap.snapshots))
	}
}

func TestListKanbanTasks(t *testing.T) {
	store := &mockStore{}
	kanban := newMockKanbanStore()
	c, _ := New(Config{ProjectRoot: "/tmp"}, store)
	c.WithKanbanStore(kanban)

	kanban.tasks["t1"] = &KanbanTask{ID: "t1", Title: "Task 1"}
	kanban.tasks["t2"] = &KanbanTask{ID: "t2", Title: "Task 2"}

	tasks, err := c.ListKanbanTasks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(tasks))
	}
}

func TestNoKanbanStore(t *testing.T) {
	c, _ := New(Config{ProjectRoot: "/tmp"}, nil)

	err := c.CreateKanbanTask(context.Background(), &KanbanTask{})
	if err == nil {
		t.Fatal("expected error with no kanban store")
	}
}
