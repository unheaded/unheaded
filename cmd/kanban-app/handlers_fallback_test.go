// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"unheaded/pkg/wotan-client/mock"
)

// stubStore is a TaskStore holding a fixed set of tasks.
type stubStore struct{ tasks []Task }

func (s *stubStore) GetAllTasks() ([]Task, error)  { return s.tasks, nil }
func (s *stubStore) GetTask(string) (*Task, error) { return nil, nil }
func (s *stubStore) SaveTask(*Task) error          { return nil }
func (s *stubStore) DeleteTask(string) error       { return nil }
func (s *stubStore) TaskCount() (int, error)       { return len(s.tasks), nil }
func (s *stubStore) SeedIfEmpty() (int, error)     { return 0, nil }
func (s *stubStore) Close() error                  { return nil }

// emptyTaskManager builds a real TaskManager holding no tasks — the state the
// service is in from startup until Wotan's first delivery, and again whenever
// a wotan restart drops its subscriptions.
func emptyTaskManager(t *testing.T) *TaskManager {
	t.Helper()
	tm, err := NewTaskManager(mock.NewMockClient(mock.WithAutoApprove()), func(string, interface{}) {}, nil)
	if err != nil {
		t.Fatalf("create task manager: %v", err)
	}
	return tm
}

func getTasksCount(t *testing.T, s *Server) (int, []map[string]any) {
	t.Helper()
	w := httptest.NewRecorder()
	s.handleGetTasks(w, httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body struct {
		Tasks []map[string]any `json:"tasks"`
		Count int              `json:"count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return body.Count, body.Tasks
}

// An empty-but-present TaskManager must not shadow the persisted tasks.
//
// handleGetTasks assigns the manager's slice into an interface{} and then
// tests `tasks == nil` to decide whether to fall through. GetAllTasks always
// returns a non-nil slice (make([]*Task, 0, n)), so the interface is never
// nil and every fallback below is skipped — the board renders zero tasks
// while the store still holds them. The manager is non-nil and empty for the
// whole window between startup and the first Wotan delivery, and again after
// a wotan restart drops subscriptions.
func TestHandleGetTasks_EmptyManagerFallsBackToStore(t *testing.T) {
	srv := &Server{
		taskManager: emptyTaskManager(t), // present, no tasks
		store:       &stubStore{tasks: []Task{{ID: "a", Title: "persisted"}, {ID: "b", Title: "also persisted"}}},
	}

	count, tasks := getTasksCount(t, srv)
	if count != 2 {
		t.Fatalf("count = %d, want 2 — an empty manager shadowed the store", count)
	}
	if len(tasks) != 2 {
		t.Errorf("returned %d tasks, want 2", len(tasks))
	}
}

// With no store either, the in-memory list is the last resort.
func TestHandleGetTasks_EmptyManagerAndNoStoreFallsBackToMemory(t *testing.T) {
	srv := &Server{
		taskManager: emptyTaskManager(t),
		tasks:       []Task{{ID: "mem", Title: "in memory"}},
	}

	count, _ := getTasksCount(t, srv)
	if count != 1 {
		t.Fatalf("count = %d, want 1 — an empty manager shadowed the in-memory list", count)
	}
}

// A manager that does have tasks still wins: the fallback must not override
// live data with a stale store copy.
func TestHandleGetTasks_PopulatedManagerWins(t *testing.T) {
	tm := emptyTaskManager(t)
	tm.tasks = map[string]*Task{"live": {ID: "live", Title: "from wotan"}}

	srv := &Server{
		taskManager: tm,
		store:       &stubStore{tasks: []Task{{ID: "a"}, {ID: "b"}, {ID: "c"}}},
	}

	count, tasks := getTasksCount(t, srv)
	if count != 1 {
		t.Fatalf("count = %d, want 1 — the store overrode live manager data", count)
	}
	if len(tasks) == 1 && tasks[0]["id"] != "live" {
		t.Errorf("returned %v, want the manager's task", tasks[0]["id"])
	}
}

// All sources empty is a legitimate empty board, not an error.
func TestHandleGetTasks_AllSourcesEmpty(t *testing.T) {
	srv := &Server{
		taskManager: emptyTaskManager(t),
		store:       &stubStore{},
	}

	count, _ := getTasksCount(t, srv)
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
}

// With every source empty the response must still carry a JSON array.
// Encoding a nil slice yields "tasks": null, which the board cannot iterate.
func TestHandleGetTasks_EmptyBoardEncodesArrayNotNull(t *testing.T) {
	srv := &Server{taskManager: emptyTaskManager(t), store: &stubStore{}}

	w := httptest.NewRecorder()
	srv.handleGetTasks(w, httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil))

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := string(raw["tasks"]); got != "[]" {
		t.Errorf(`tasks = %s, want [] — "null" is not iterable on the board`, got)
	}
}
