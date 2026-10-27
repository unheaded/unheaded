// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"database/sql"
	"errors"
	"time"

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

// kanbanMigrations are the ledger rows (ADR-101) the Postgres store needs
// in unheaded_app: 003 defines kanban_tasks, 007 grants app_kanban its
// rows. Reading the ledger as app_kanban needs 038's SELECT grant.
var kanbanMigrations = []string{"003_app_schema.sql", "007_grants.sql"}

// startupCheckTimeout bounds the ledger and shape checks at startup.
const startupCheckTimeout = 10 * time.Second

// NewPgStore creates a Postgres-backed TaskStore. It refuses to start when
// The Well's ledger lacks kanban's migrations or the kanban_tasks shape is
// incomplete (migrations define the table, never this process: M-073).
func NewPgStore(db *sql.DB) (*PgStore, error) {
	ctx, cancel := context.WithTimeout(context.Background(), startupCheckTimeout)
	defer cancel()
	if err := database.RequireMigrations(ctx, db, kanbanMigrations...); err != nil {
		return nil, err
	}
	ks := database.NewKanbanStore(db)
	if err := ks.CheckSchema(ctx); err != nil {
		return nil, err
	}
	return &PgStore{inner: ks, db: db}, nil
}

// wellStore decides the store from database.Connect's result. A Well that
// is not configured or does not answer (connErr) gives (nil, nil): the
// caller falls back to SQLite, which is for dev runs without a Well. A Well
// that answers but whose ledger or shape is not kanban's gives an error and
// the caller refuses to start: falling back there would split the board
// between the Well and a local file (cards come from the Well).
func wellStore(db *sql.DB, connErr error) (TaskStore, error) {
	if connErr != nil || db == nil {
		return nil, nil
	}
	st, err := NewPgStore(db)
	if err != nil {
		db.Close() // #nosec G104 -- cleanup on an error path; the caller reports err
		return nil, err
	}
	return st, nil
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
		GUID:        pt.GUID,
		Title:       pt.Title,
		Description: pt.Description,
		Status:      pt.Status,
		Type:        pt.Type,
		Owner:       pt.Owner,
		Progress:    pt.Progress,
		CreatedAt:   pt.CreatedAt,
		UpdatedAt:   pt.UpdatedAt,
		DeletedAt:   pt.DeletedAt,
		ArchivedAt:  pt.ArchivedAt,
		Commits:     pt.Commits,
	}
}

func toPgTask(t Task) database.KanbanTask {
	return database.KanbanTask{
		ID:          t.ID,
		GUID:        t.GUID,
		Title:       t.Title,
		Description: t.Description,
		Status:      t.Status,
		Type:        t.Type,
		Owner:       t.Owner,
		Progress:    t.Progress,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
		DeletedAt:   t.DeletedAt,
		ArchivedAt:  t.ArchivedAt,
		Commits:     t.Commits,
	}
}
