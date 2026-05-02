// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

package main

import (
	"context"
	"database/sql"
	"fmt"

	"unheaded/pkg/champion"
	"unheaded/pkg/database"
)

// pgKanbanStore adapts a *pkg/database.KanbanStore to the
// pkg/champion.KanbanStore interface. Two concerns:
//
//  1. The two packages each declare their own KanbanTask struct. Field
//     shapes are compatible, so we translate at the boundary instead of
//     refactoring either package's public types (which has churn cost
//     across kanban-app + dashboard-backend + tooling).
//
//  2. pkg/database.KanbanStore is upsert-shaped (SaveTask handles both
//     create and update). pkg/champion.KanbanStore is verb-shaped
//     (CreateTask, UpdateTask). The adapter splits the two.
//
// This is the missing wire that closes hop 4 of the trust chain
// user → zhen-agentd → The Well → kanban-app → Champion. Without it,
// kanban_create / kanban_update tool calls passed through Champion's
// gate but failed at dispatch with "no kanban store configured".
type pgKanbanStore struct {
	inner *database.KanbanStore
}

// newPGKanbanStore wraps an existing *sql.DB. Schema is migrated
// idempotently on first call (EnsureSchema is safe to invoke on every
// startup).
func newPGKanbanStore(ctx context.Context, db *sql.DB) (*pgKanbanStore, error) {
	inner := database.NewKanbanStore(db)
	if err := inner.EnsureSchema(ctx); err != nil {
		return nil, fmt.Errorf("ensure kanban_tasks schema: %w", err)
	}
	return &pgKanbanStore{inner: inner}, nil
}

func (p *pgKanbanStore) CreateTask(ctx context.Context, task *champion.KanbanTask) error {
	dbTask := toDBTask(task)
	return p.inner.SaveTask(ctx, dbTask)
}

func (p *pgKanbanStore) UpdateTask(ctx context.Context, id string, updates map[string]any) error {
	existing, err := p.inner.GetTask(ctx, id)
	if err != nil {
		return fmt.Errorf("fetch task %q: %w", id, err)
	}
	if existing == nil {
		return fmt.Errorf("task %q not found", id)
	}
	if v, ok := updates["title"].(string); ok {
		existing.Title = v
	}
	if v, ok := updates["description"].(string); ok {
		existing.Description = v
	}
	if v, ok := updates["status"].(string); ok {
		existing.Status = v
	}
	if v, ok := updates["type"].(string); ok {
		existing.Type = v
	}
	if v, ok := updates["owner"].(string); ok {
		existing.Owner = v
	}
	// Numeric updates can arrive as either int (Go callers) or float64
	// (JSON callers). Both must apply.
	switch v := updates["progress"].(type) {
	case int:
		existing.Progress = v
	case float64:
		existing.Progress = int(v)
	}
	return p.inner.SaveTask(ctx, existing)
}

func (p *pgKanbanStore) GetTask(ctx context.Context, id string) (*champion.KanbanTask, error) {
	dbTask, err := p.inner.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}
	if dbTask == nil {
		return nil, nil
	}
	return toChampionTask(dbTask), nil
}

func (p *pgKanbanStore) ListTasks(ctx context.Context) ([]champion.KanbanTask, error) {
	dbTasks, err := p.inner.GetAllTasks(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]champion.KanbanTask, 0, len(dbTasks))
	for i := range dbTasks {
		out = append(out, *toChampionTask(&dbTasks[i]))
	}
	return out, nil
}

// toDBTask converts a champion.KanbanTask to a database.KanbanTask.
// Champion's KanbanTask is the lighter type (fewer timestamp / metadata
// fields); the missing fields default to zero / sensible empty values.
func toDBTask(t *champion.KanbanTask) *database.KanbanTask {
	return &database.KanbanTask{
		ID:          t.ID,
		Title:       t.Title,
		Description: t.Description,
		Status:      t.Status,
		Type:        t.Type,
		Owner:       t.Owner,
		Progress:    t.Progress,
	}
}

// toChampionTask converts the other direction. Database-only fields
// (CreatedAt, UpdatedAt, GUID, etc.) are dropped — Champion's gate +
// audit layer doesn't need them.
func toChampionTask(t *database.KanbanTask) *champion.KanbanTask {
	return &champion.KanbanTask{
		ID:          t.ID,
		Title:       t.Title,
		Description: t.Description,
		Status:      t.Status,
		Type:        t.Type,
		Owner:       t.Owner,
		Progress:    t.Progress,
	}
}
