// SPDX-License-Identifier: GPL-3.0-or-later
package database

import (
	"context"
	"database/sql"
	"time"

	"github.com/lib/pq"
)

// KanbanTask represents a kanban board task stored in The Well.
type KanbanTask struct {
	ID          int       `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	Priority    string    `json:"priority"`
	Tags        []string  `json:"tags"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// KanbanStore provides PostgreSQL-backed kanban task persistence.
type KanbanStore struct {
	db *sql.DB
}

// NewKanbanStore creates a new KanbanStore backed by the given database connection.
func NewKanbanStore(db *sql.DB) *KanbanStore {
	return &KanbanStore{db: db}
}

// ListByStatus returns all tasks with the given status, ordered by priority then creation time.
func (s *KanbanStore) ListByStatus(ctx context.Context, status string) ([]KanbanTask, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, title, description, status, priority, tags, created_at, updated_at
		 FROM kanban_tasks WHERE status = $1 ORDER BY priority, created_at`, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []KanbanTask
	for rows.Next() {
		var t KanbanTask
		if err := rows.Scan(&t.ID, &t.Title, &t.Description, &t.Status, &t.Priority,
			pq.Array(&t.Tags), &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// UpdateStatus changes the status of a task by ID.
func (s *KanbanStore) UpdateStatus(ctx context.Context, id int, newStatus string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE kanban_tasks SET status = $1, updated_at = NOW() WHERE id = $2`,
		newStatus, id)
	return err
}

// Create inserts a new kanban task and returns its ID.
func (s *KanbanStore) Create(ctx context.Context, t KanbanTask) (int, error) {
	var id int
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO kanban_tasks (title, description, status, priority, tags)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		t.Title, t.Description, t.Status, t.Priority, pq.Array(t.Tags)).Scan(&id)
	return id, err
}
