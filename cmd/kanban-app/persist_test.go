// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"unheaded/pkg/wotan-client/mock"
)

// failingStore is a TaskStore whose writes fail: a database that is there at
// startup and then is not (disk full, The Well restarting, a lock timeout).
type failingStore struct{ stubStore }

var errDiskFull = errors.New("disk full")

func (*failingStore) SaveTask(*Task) error    { return errDiskFull }
func (*failingStore) DeleteTask(string) error { return errDiskFull }

func storeBackedServer(t *testing.T, store TaskStore) *Server {
	t.Helper()
	tm, err := NewTaskManager(mock.NewMockClient(mock.WithAutoApprove()), func(string, interface{}) {}, store)
	if err != nil {
		t.Fatalf("create task manager: %v", err)
	}
	tm.addTask(createTestTask("existing"))
	return &Server{taskManager: tm, store: store, sseClients: make(map[chan []byte]bool)}
}

// A write the store refused is not a write. TaskManager used to log the
// store error and answer 201/200: the board showed the change until the next
// restart, then it was gone.
func TestTaskManagerWrites_StoreFailureIsNotSuccess(t *testing.T) {
	newTask, _ := json.Marshal(createTestTask("new-card"))
	changed := createTestTask("existing")
	changed.Title = "changed"
	update, _ := json.Marshal(changed)

	for _, tc := range []struct {
		name, method, path string
		body               []byte
		check              func(*TaskManager) bool // true: memory unchanged
	}{
		{"create", http.MethodPost, "/api/v1/tasks", newTask,
			func(tm *TaskManager) bool { return !tm.taskExists("new-card") }},
		{"update", http.MethodPut, "/api/v1/tasks", update,
			func(tm *TaskManager) bool { return tm.getTask("existing").Title != "changed" }},
		{"delete", http.MethodDelete, "/api/v1/tasks?id=existing", nil,
			func(tm *TaskManager) bool { return tm.taskExists("existing") }},
		{"patch by id", http.MethodPut, "/api/v1/tasks/existing", []byte(`{"title":"changed"}`),
			func(tm *TaskManager) bool { return tm.getTask("existing").Title != "changed" }},
		{"delete by id", http.MethodDelete, "/api/v1/tasks/existing", nil,
			func(tm *TaskManager) bool { return tm.taskExists("existing") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := storeBackedServer(t, &failingStore{})
			w := httptest.NewRecorder()
			r := httptest.NewRequest(tc.method, tc.path, bytes.NewReader(tc.body))
			if tc.path == "/api/v1/tasks" || tc.path[len("/api/v1/tasks")] == '?' {
				s.handleTasks(w, r)
			} else {
				s.handleTaskByID(w, r)
			}
			if w.Code != http.StatusInternalServerError {
				t.Errorf("status = %d, want 500: the store refused the write", w.Code)
			}
			if !tc.check(s.taskManager) {
				t.Error("the board shows a change the store does not hold")
			}
		})
	}
}

// The same writes against a working store still succeed.
func TestTaskManagerWrites_StoreSuccess(t *testing.T) {
	s := storeBackedServer(t, &stubStore{})
	newTask, _ := json.Marshal(createTestTask("new-card"))
	w := httptest.NewRecorder()
	s.handleTasks(w, httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewReader(newTask)))
	if w.Code != http.StatusCreated || !s.taskManager.taskExists("new-card") {
		t.Errorf("create: status %d, exists %v; want 201 and the card", w.Code, s.taskManager.taskExists("new-card"))
	}
	if err := s.taskManager.DeleteTask(context.Background(), "existing"); err != nil {
		t.Errorf("delete: %v", err)
	}
}

// unreadableStore opens and seeds but cannot be read.
type unreadableStore struct{ stubStore }

func (*unreadableStore) GetAllTasks() ([]Task, error) { return nil, errDiskFull }

// When the store cannot be read at startup the board is empty and
// read-only. TaskManager drops the store, but main hands the server the same
// store, and writable() once trusted the server's copy, so writes reached
// memory only and answered 200. (kanban-app no longer seeds anything: the
// Genesis cards are migration 034's.)
func TestTaskManagerInitialize_BrokenStoreIsReadOnly(t *testing.T) {
	for _, tc := range []struct {
		name  string
		store TaskStore
		want  int
	}{
		{"unreadable", &unreadableStore{}, http.StatusServiceUnavailable},
		{"working", &stubStore{tasks: []Task{{ID: "ms-phase0", Title: "x", Status: "todo"}}}, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tm, err := NewTaskManager(mock.NewMockClient(mock.WithAutoApprove()), func(string, interface{}) {}, tc.store)
			if err != nil {
				t.Fatal(err)
			}
			if err := tm.Initialize(context.Background()); err != nil {
				t.Fatal(err)
			}
			defer tm.Close()
			// As main builds it: the server holds the same store.
			s := NewServerWithTaskManager(Config{}, tm, tc.store)
			body, _ := json.Marshal(Task{ID: "ms-phase0", Title: "edited", Status: "done"})
			w := httptest.NewRecorder()
			s.handleTasks(w, httptest.NewRequest(http.MethodPut, "/api/v1/tasks", bytes.NewReader(body)))
			if w.Code != tc.want {
				t.Errorf("PUT: status %d, want %d", w.Code, tc.want)
			}
		})
	}
}

// An update keeps the task's created_at, whatever the PUT body says.
func TestTaskManagerUpdate_KeepsCreatedAt(t *testing.T) {
	s := storeBackedServer(t, &stubStore{})
	created := s.taskManager.getTask("existing").CreatedAt
	for _, body := range []string{
		`{"id":"existing","title":"t","status":"todo"}`,
		`{"id":"existing","title":"t","status":"todo","created_at":"2001-01-01T00:00:00Z"}`,
	} {
		w := httptest.NewRecorder()
		s.handleTasks(w, httptest.NewRequest(http.MethodPut, "/api/v1/tasks", bytes.NewReader([]byte(body))))
		if w.Code != http.StatusOK {
			t.Fatalf("%s: status %d", body, w.Code)
		}
		var resp struct{ Task Task }
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		for where, got := range map[string]time.Time{
			"response": resp.Task.CreatedAt, "board": s.taskManager.getTask("existing").CreatedAt,
		} {
			if !got.Equal(created) {
				t.Errorf("%s: %s created_at = %v, want %v", body, where, got, created)
			}
		}
	}
}

// A task TaskManager rejects is the client's error. TaskManager wraps
// ErrInvalidTask with the reason; the handlers compared with ==, so a bad
// status or progress answered 500 Internal server error.
func TestTaskManagerWrites_InvalidTaskIs400(t *testing.T) {
	bad := createTestTask("new-card")
	bad.Status = "someday"
	create, _ := json.Marshal(bad)
	bad.ID = "existing"
	update, _ := json.Marshal(bad)

	for _, tc := range []struct {
		name, method, path string
		body               []byte
	}{
		{"create", http.MethodPost, "/api/v1/tasks", create},
		{"update", http.MethodPut, "/api/v1/tasks", update},
		{"patch by id", http.MethodPut, "/api/v1/tasks/existing", []byte(`{"progress":250}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := storeBackedServer(t, &stubStore{})
			w := httptest.NewRecorder()
			r := httptest.NewRequest(tc.method, tc.path, bytes.NewReader(tc.body))
			if tc.path == "/api/v1/tasks" {
				s.handleTasks(w, r)
			} else {
				s.handleTaskByID(w, r)
			}
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", w.Code)
			}
		})
	}
}
