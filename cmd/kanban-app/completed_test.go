// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func completedCount(t *testing.T, s *Server) (int, string) {
	t.Helper()
	w := httptest.NewRecorder()
	s.handleTaskByID(w, httptest.NewRequest(http.MethodGet, "/api/v1/tasks/completed", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body struct {
		Tasks []Task `json:"tasks"`
		Count int    `json:"count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Count != len(body.Tasks) {
		t.Errorf("count %d but %d tasks", body.Count, len(body.Tasks))
	}
	ids := ""
	for _, task := range body.Tasks {
		ids += task.ID + " "
	}
	return body.Count, ids
}

// /api/v1/tasks/completed read Postgres only and answered an empty list for
// every other store. East runs SQLite: it said 0 completed while the board
// had 9 done cards.
func TestCompletedTasks_SQLiteStore(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "kanban.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, task := range []Task{
		{ID: "a", Title: "A", Status: "done"},
		{ID: "b", Title: "B", Status: "todo"},
		{ID: "c", Title: "C", Status: "done"},
	} {
		if err := store.SaveTask(&task); err != nil {
			t.Fatal(err)
		}
	}
	s := &Server{store: store, sseClients: make(map[chan []byte]bool)}
	if n, ids := completedCount(t, s); n != 2 || (ids != "a c " && ids != "c a ") {
		t.Errorf("completed = %d (%s), want 2 (a c)", n, ids)
	}
}

func TestCompletedTasks_NoStore(t *testing.T) {
	s := &Server{tasks: []Task{{ID: "a", Status: "done"}, {ID: "b", Status: "review"}}}
	if n, _ := completedCount(t, s); n != 1 {
		t.Errorf("completed = %d, want 1", n)
	}
	// None done is an empty array, not null.
	w := httptest.NewRecorder()
	(&Server{}).handleTaskByID(w, httptest.NewRequest(http.MethodGet, "/api/v1/tasks/completed", nil))
	if got := w.Body.String(); got != "{\"count\":0,\"tasks\":[]}\n" {
		t.Errorf("empty board body = %q", got)
	}
}
