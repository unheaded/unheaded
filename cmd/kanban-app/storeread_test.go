// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

// downStore is a TaskStore whose database is unreachable.
type downStore struct{ stubStore }

func (*downStore) GetAllTasks() ([]Task, error) { return nil, errDiskFull }
func (*downStore) GetTask(string) (*Task, error) {
	return nil, errDiskFull
}

// missingStore answers every lookup "not found".
type missingStore struct{ stubStore }

func (*missingStore) GetTask(string) (*Task, error) { return nil, ErrStoreNotFound }

// A store that cannot be read is a 500, not a missing task and not an empty
// board. Standalone mode (no Wotan) used to answer 404 for every task and
// 200 with zero tasks while the database was down; and a create, taking the
// failed lookup for "absent", upserted over whatever the store held.
func TestStandaloneStoreReadErrors(t *testing.T) {
	newTask := []byte(`{"id":"t1","title":"T","status":"todo"}`)
	for _, tc := range []struct {
		name, method, path string
		body               []byte
		down, missing      int
	}{
		{"list", http.MethodGet, "/api/v1/tasks", nil, 500, 200},
		{"get", http.MethodGet, "/api/v1/tasks/t1", nil, 500, 404},
		{"patch", http.MethodPut, "/api/v1/tasks/t1", []byte(`{"title":"x"}`), 500, 404},
		{"update", http.MethodPut, "/api/v1/tasks", newTask, 500, 404},
		{"create", http.MethodPost, "/api/v1/tasks", newTask, 500, 201},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for store, want := range map[TaskStore]int{&downStore{}: tc.down, &missingStore{}: tc.missing} {
				s := &Server{store: store, sseClients: make(map[chan []byte]bool)}
				w := httptest.NewRecorder()
				r := httptest.NewRequest(tc.method, tc.path, bytes.NewReader(tc.body))
				if tc.path == "/api/v1/tasks" {
					s.handleTasks(w, r)
				} else {
					s.handleTaskByID(w, r)
				}
				if w.Code != want {
					t.Errorf("%T: status %d, want %d", store, w.Code, want)
				}
			}
		})
	}
}
