// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

// Package pgstore implements champion.ActionStore against PostgreSQL
// (The Well). Every Champion gate decision and every underlying tool
// op (file read/write/patch, kanban CRUD) is logged to the
// zhen_actions table for full auditability.
//
// Replaces the stderrActionStore stub that ships with the daemons —
// production deployments wire this in with a DSN to The Well's
// unheaded_app database.
//
// Schema (in schema.sql, embedded; applied via Migrate):
//
//	zhen_actions (id, session_id, action_type, status, intent,
//	              parameters, result, result_summary, error,
//	              triggered_by, planned_at, completed_at, elapsed_ms)
//
// Indexes on session_id (audit queries), planned_at DESC (time-range),
// and (action_type, status) (forensic).
package pgstore

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"time"

	"unheaded/pkg/champion"
)

//go:embed schema.sql
var schemaSQL string

// Schema returns the embedded schema.sql so callers can apply it
// out-of-band (e.g., a separate migration tool).
func Schema() string { return schemaSQL }

// Store implements champion.ActionStore against PostgreSQL.
type Store struct {
	db *sql.DB
}

// New wraps an open *sql.DB. The DB must already be connected and
// pinged. Migrate(ctx) should be called once on daemon startup to
// ensure the schema exists.
func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// Migrate applies schema.sql (idempotent — uses CREATE TABLE / CREATE
// INDEX with IF NOT EXISTS). Safe to call on every daemon startup.
func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}
	return nil
}

// LogAction records a new action with status=planned. The returned id
// must be passed to UpdateAction to mark completion.
func (s *Store) LogAction(ctx context.Context, action *champion.Action) (int64, error) {
	if action.PlannedAt.IsZero() {
		action.PlannedAt = time.Now().UTC()
	}
	var id int64
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO zhen_actions
		   (session_id, action_type, status, intent, parameters,
		    triggered_by, planned_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id`,
		action.SessionID, action.ActionType, action.Status,
		nullableString(action.Intent), nullableString(action.Parameters),
		nullableString(action.TriggeredBy), action.PlannedAt,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert action: %w", err)
	}
	action.ID = id
	return id, nil
}

// UpdateAction marks an in-flight action complete. The completed_at
// column is set by NOW() at the database side so the timestamp is
// authoritative across daemons sharing the same store.
func (s *Store) UpdateAction(ctx context.Context, id int64, status, result, errMsg string, elapsedMs int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE zhen_actions
		    SET status         = $1,
		        result_summary = $2,
		        error          = $3,
		        elapsed_ms     = $4,
		        completed_at   = NOW()
		  WHERE id = $5`,
		status, nullableString(result), nullableString(errMsg), elapsedMs, id,
	)
	if err != nil {
		return fmt.Errorf("update action %d: %w", id, err)
	}
	return nil
}

// GetActions returns the most-recent `limit` actions for a session,
// newest-first.
func (s *Store) GetActions(ctx context.Context, sessionID string, limit int) ([]champion.Action, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, action_type, status, intent, parameters,
		        result, result_summary, error, triggered_by,
		        planned_at, completed_at, elapsed_ms
		   FROM zhen_actions
		  WHERE session_id = $1
		  ORDER BY id DESC
		  LIMIT $2`,
		sessionID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query actions: %w", err)
	}
	defer rows.Close()

	var actions []champion.Action
	for rows.Next() {
		var (
			a           champion.Action
			intent      sql.NullString
			parameters  sql.NullString
			result      sql.NullString
			summary     sql.NullString
			errMsg      sql.NullString
			triggeredBy sql.NullString
			completedAt sql.NullTime
			elapsedMs   sql.NullInt64
		)
		if err := rows.Scan(
			&a.ID, &a.SessionID, &a.ActionType, &a.Status,
			&intent, &parameters, &result, &summary, &errMsg, &triggeredBy,
			&a.PlannedAt, &completedAt, &elapsedMs,
		); err != nil {
			return nil, fmt.Errorf("scan action: %w", err)
		}
		a.Intent = intent.String
		a.Parameters = parameters.String
		a.Result = result.String
		a.ResultSummary = summary.String
		a.Error = errMsg.String
		a.TriggeredBy = triggeredBy.String
		if completedAt.Valid {
			a.CompletedAt = completedAt.Time
		}
		if elapsedMs.Valid {
			a.ElapsedMs = int(elapsedMs.Int64)
		}
		actions = append(actions, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate actions: %w", err)
	}
	return actions, nil
}

// nullableString converts an empty string to a SQL NULL via
// sql.NullString. Keeps the DB clean of empty-string placeholders.
func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
