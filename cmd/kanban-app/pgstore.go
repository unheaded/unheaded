// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"database/sql"
	"errors"

	"unheaded/pkg/database"
)

// PgStore adapts database.KanbanStore to the kanban-app TaskStore interface.
// It translates between the app-local Task type and database.KanbanTask,
// and injects a background context for each call (the SQLite Store methods
// do not accept contexts — the interface is kept simple for backwards compat).
type PgStore struct {
	inner *database.KanbanStore
	db    *sql.DB // retained for Close()
}

// NewPgStore creates a Postgres-backed TaskStore. It ensures the schema
// exists before returning.
func NewPgStore(db *sql.DB) (*PgStore, error) {
	ks := database.NewKanbanStore(db)
	if err := ks.EnsureSchema(context.Background()); err != nil {
		return nil, err
	}
	return &PgStore{inner: ks, db: db}, nil
}

func (p *PgStore) GetAllTasks() ([]Task, error) {
	pgTasks, err := p.inner.GetAllTasks(context.Background())
	if err != nil {
		return nil, err
	}
	out := make([]Task, len(pgTasks))
	for i, t := range pgTasks {
		out[i] = fromPgTask(t)
	}
	return out, nil
}

func (p *PgStore) GetTask(id string) (*Task, error) {
	pt, err := p.inner.GetTask(context.Background(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrStoreNotFound
		}
		return nil, err
	}
	t := fromPgTask(*pt)
	return &t, nil
}

func (p *PgStore) SaveTask(task *Task) error {
	if task == nil {
		return ErrStoreNilTask
	}
	if task.ID == "" {
		return ErrStoreEmptyTaskID
	}
	pt := toPgTask(*task)
	return p.inner.SaveTask(context.Background(), &pt)
}

func (p *PgStore) DeleteTask(id string) error {
	if id == "" {
		return ErrStoreEmptyTaskID
	}
	err := p.inner.DeleteTask(context.Background(), id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrStoreNotFound
	}
	return err
}

func (p *PgStore) TaskCount() (int, error) {
	return p.inner.TaskCount(context.Background())
}

func (p *PgStore) SeedIfEmpty() (int, error) {
	initial := getInitialTasks()
	pgTasks := make([]database.KanbanTask, len(initial))
	for i, t := range initial {
		pgTasks[i] = toPgTask(t)
	}
	return p.inner.SeedIfEmpty(context.Background(), pgTasks)
}

func (p *PgStore) Close() error {
	// Close the underlying Postgres connection.
	return p.db.Close()
}

// ============================================================================
// Conversion helpers
// ============================================================================

func fromPgTask(pt database.KanbanTask) Task {
	return Task{
		ID:          pt.ID,
		Title:       pt.Title,
		Description: pt.Description,
		Status:      pt.Status,
		Type:        pt.Type,
		Owner:       pt.Owner,
		Progress:    pt.Progress,
		CreatedAt:   pt.CreatedAt,
		UpdatedAt:   pt.UpdatedAt,
	}
}

func toPgTask(t Task) database.KanbanTask {
	return database.KanbanTask{
		ID:          t.ID,
		Title:       t.Title,
		Description: t.Description,
		Status:      t.Status,
		Type:        t.Type,
		Owner:       t.Owner,
		Progress:    t.Progress,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
	}
}
