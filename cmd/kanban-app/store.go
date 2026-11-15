// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import "errors"

// ============================================================================
// TASK STORE INTERFACE
// ============================================================================

// TaskStore is the interface for kanban task persistence. The Well
// (PgStore) is the one home (ADR-147 Phase 16): the SQLite fallback of
// 2026-10-28 is gone, and without the Well the board waits, read-only
// (well_wait.go). The SQLite Store lives on in sqlite_store_test.go as a
// test fixture.
type TaskStore interface {
	GetAllTasks() ([]Task, error)
	GetTask(id string) (*Task, error)
	SaveTask(task *Task) error
	DeleteTask(id string) error
	TaskCount() (int, error)
	Close() error
}

// ============================================================================
// ERRORS
// ============================================================================

var (
	ErrStoreNilTask     = errors.New("task cannot be nil")
	ErrStoreEmptyTaskID = errors.New("task ID cannot be empty")
	ErrStoreNotFound    = errors.New("task not found in store")
)
