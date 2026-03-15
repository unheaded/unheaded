// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// KanbanTask mirrors the kanban-app Task type for Postgres persistence.
// Field names and JSON tags match the kanban-app's canonical Task struct.
type KanbanTask struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Status      string    `json:"status"`
	Type        string    `json:"type,omitempty"`
	Owner       string    `json:"owner,omitempty"`
	Progress    int       `json:"progress,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// KanbanStore provides Postgres-backed task persistence.
// It implements the same operations as the SQLite Store in cmd/kanban-app/store.go,
// allowing the kanban-app to swap backends transparently.
type KanbanStore struct {
	db *sql.DB
}

// NewKanbanStore wraps an existing Postgres connection for kanban task CRUD.
// The caller must ensure the kanban_tasks table exists (created by migrations).
func NewKanbanStore(db *sql.DB) *KanbanStore {
	return &KanbanStore{db: db}
}

// EnsureSchema creates the kanban_tasks table if it does not exist.
// Safe to call on every startup (idempotent).
func (s *KanbanStore) EnsureSchema(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS kanban_tasks (
			id          TEXT PRIMARY KEY,
			title       TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			status      TEXT NOT NULL DEFAULT 'todo',
			type        TEXT NOT NULL DEFAULT 'task',
			owner       TEXT NOT NULL DEFAULT '',
			progress    INTEGER NOT NULL DEFAULT 0 CHECK (progress >= 0 AND progress <= 100),
			created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_kanban_tasks_status ON kanban_tasks(status);
	`)
	return err
}

// GetAllTasks returns all tasks ordered by creation time.
func (s *KanbanStore) GetAllTasks(ctx context.Context) ([]KanbanTask, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, title, description, status, type, owner, progress, created_at, updated_at
		FROM kanban_tasks ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("query kanban tasks: %w", err)
	}
	defer rows.Close()

	var tasks []KanbanTask
	for rows.Next() {
		var t KanbanTask
		if err := rows.Scan(&t.ID, &t.Title, &t.Description, &t.Status,
			&t.Type, &t.Owner, &t.Progress, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan kanban task: %w", err)
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// GetTask returns a single task by ID, or sql.ErrNoRows if not found.
func (s *KanbanStore) GetTask(ctx context.Context, id string) (*KanbanTask, error) {
	var t KanbanTask
	err := s.db.QueryRowContext(ctx, `
		SELECT id, title, description, status, type, owner, progress, created_at, updated_at
		FROM kanban_tasks WHERE id = $1
	`, id).Scan(&t.ID, &t.Title, &t.Description, &t.Status,
		&t.Type, &t.Owner, &t.Progress, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// SaveTask performs an upsert (insert or update on conflict).
func (s *KanbanStore) SaveTask(ctx context.Context, t *KanbanTask) error {
	if t == nil {
		return fmt.Errorf("task cannot be nil")
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO kanban_tasks (id, title, description, status, type, owner, progress, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
		ON CONFLICT(id) DO UPDATE SET
			title       = EXCLUDED.title,
			description = EXCLUDED.description,
			status      = EXCLUDED.status,
			type        = EXCLUDED.type,
			owner       = EXCLUDED.owner,
			progress    = EXCLUDED.progress,
			updated_at  = NOW()
	`, t.ID, t.Title, t.Description, t.Status, t.Type, t.Owner, t.Progress, t.CreatedAt)
	return err
}

// DeleteTask removes a task by ID.
func (s *KanbanStore) DeleteTask(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM kanban_tasks WHERE id = $1`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// TaskCount returns the number of rows in kanban_tasks.
func (s *KanbanStore) TaskCount(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM kanban_tasks`).Scan(&count)
	return count, err
}

// SeedIfEmpty inserts the provided tasks only when the table is empty.
// Returns the number of existing or newly-inserted rows.
func (s *KanbanStore) SeedIfEmpty(ctx context.Context, tasks []KanbanTask) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM kanban_tasks`).Scan(&count); err != nil {
		return 0, err
	}
	if count > 0 {
		return count, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	for _, t := range tasks {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO kanban_tasks (id, title, description, status, type, owner, progress, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT(id) DO NOTHING
		`, t.ID, t.Title, t.Description, t.Status, t.Type, t.Owner, t.Progress, t.CreatedAt, t.UpdatedAt)
		if err != nil {
			_ = tx.Rollback()
			return 0, fmt.Errorf("seed task %s: %w", t.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(tasks), nil
}

// Close is a no-op — the underlying *sql.DB is owned by the caller.
func (s *KanbanStore) Close() error {
	return nil
}
