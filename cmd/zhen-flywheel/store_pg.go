// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
)

// errMissingCapture: a label names a capture that is not in the Well.
var errMissingCapture = errors.New("label names a capture that is not stored")

// queryTimeout bounds every statement.
const queryTimeout = 30 * time.Second

// pgStore is the Well: unheaded_app as app_zhen, and (optionally)
// unheaded_ops for llm_requests.
type pgStore struct {
	app *sql.DB
	ops *sql.DB // nil: Escalations is ErrUnavailable
}

func openPG(appDSN, opsDSN string) (*pgStore, error) {
	app, err := sql.Open("postgres", appDSN)
	if err != nil {
		return nil, err
	}
	app.SetMaxOpenConns(4)
	s := &pgStore{app: app}
	if opsDSN != "" {
		if s.ops, err = sql.Open("postgres", opsDSN); err != nil {
			_ = app.Close()
			return nil, err
		}
		s.ops.SetMaxOpenConns(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := app.PingContext(ctx); err != nil {
		s.Close()
		return nil, fmt.Errorf("unheaded_app: %w", err)
	}
	return s, nil
}

func (s *pgStore) Close() {
	_ = s.app.Close()
	if s.ops != nil {
		_ = s.ops.Close()
	}
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (s *pgStore) InsertCapture(ctx context.Context, c *Capture) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	// purge_after is the insert trigger's (captured_at + 180 days).
	res, err := s.app.ExecContext(ctx, `INSERT INTO zhen_captures
		(id, captured_at, source, backend, model, author_kind, task_family, conversation_id, run_id, record_id,
		 schema, redaction_version, prompt, response, tool_trace, truncated)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		ON CONFLICT (id) DO NOTHING`,
		c.ID, c.CapturedAt, c.Source, c.Backend, c.Model, c.AuthorKind, c.TaskFamily, c.ConversationID, nullable(c.RunID),
		c.RecordID, c.Schema, c.RedactionVersion, c.Prompt, c.Response, string(c.ToolTrace), c.Truncated)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

func (s *pgStore) InsertLabel(ctx context.Context, l Label) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	val := l.Value
	if len(val) == 0 {
		val = json.RawMessage("{}")
	}
	res, err := s.app.ExecContext(ctx, `INSERT INTO zhen_capture_labels (capture_id, kind, value, source_ref)
		VALUES ($1, $2, $3, $4) ON CONFLICT ON CONSTRAINT zhen_capture_labels_once DO NOTHING`,
		l.CaptureID, l.Kind, string(val), l.SourceRef)
	if err != nil {
		var pe *pq.Error
		if errors.As(err, &pe) && pe.Code == "23503" {
			return false, errMissingCapture
		}
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

func (s *pgStore) Captures(ctx context.Context) ([]Capture, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	rows, err := s.app.QueryContext(ctx, `SELECT id, captured_at, source, backend, model, author_kind, task_family,
		conversation_id, coalesce(run_id, ''), record_id, schema, redaction_version, prompt, response, tool_trace::text,
		truncated, promoted, quarantined FROM zhen_captures ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Capture
	for rows.Next() {
		var c Capture
		var trace string
		if err := rows.Scan(&c.ID, &c.CapturedAt, &c.Source, &c.Backend, &c.Model, &c.AuthorKind, &c.TaskFamily,
			&c.ConversationID, &c.RunID, &c.RecordID, &c.Schema, &c.RedactionVersion, &c.Prompt, &c.Response, &trace,
			&c.Truncated, &c.Promoted, &c.Quarantined); err != nil {
			return nil, err
		}
		c.ID = trimChar(c.ID)
		c.ToolTrace = json.RawMessage(trace)
		out = append(out, c)
	}
	return out, rows.Err()
}

// trimChar strips CHAR(n) padding (ids are exactly 64 hex, so none).
func trimChar(s string) string {
	for len(s) > 0 && s[len(s)-1] == ' ' {
		s = s[:len(s)-1]
	}
	return s
}

func (s *pgStore) Labels(ctx context.Context) ([]Label, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	rows, err := s.app.QueryContext(ctx, `SELECT capture_id, kind, value::text, source_ref FROM zhen_capture_labels ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Label
	for rows.Next() {
		var l Label
		var v string
		if err := rows.Scan(&l.CaptureID, &l.Kind, &v, &l.SourceRef); err != nil {
			return nil, err
		}
		l.CaptureID = trimChar(l.CaptureID)
		l.Value = json.RawMessage(v)
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *pgStore) CheckActions(ctx context.Context) ([]CheckAction, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	rows, err := s.app.QueryContext(ctx, `SELECT id, session_id, coalesce(planned_at, 'epoch'), coalesce(completed_at, 'epoch'),
		status, coalesce(result_summary, '') FROM zhen_actions WHERE action_type = 'run_check' AND status = 'completed' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []CheckAction
	for rows.Next() {
		var a CheckAction
		if err := rows.Scan(&a.ID, &a.SessionID, &a.PlannedAt, &a.CompletedAt, &a.Status, &a.Summary); err != nil {
			return nil, err
		}
		if a.CompletedAt.Unix() == 0 {
			a.CompletedAt = time.Time{}
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *pgStore) Escalations(ctx context.Context) ([]Escalation, error) {
	if s.ops == nil {
		return nil, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	rows, err := s.ops.QueryContext(ctx, `SELECT min(id), conversation_id, route_reason FROM llm_requests
		WHERE route_reason LIKE 'escalated\_%' GROUP BY conversation_id, route_reason ORDER BY 1`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Escalation
	for rows.Next() {
		var e Escalation
		if err := rows.Scan(&e.FirstID, &e.ConversationID, &e.RouteReason); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *pgStore) AgentRuns(ctx context.Context) ([]AgentRun, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	rows, err := s.app.QueryContext(ctx, `SELECT id, status, coalesce(review_reason, ''), base_commit, coalesce(head_commit, '')
		FROM zhen_agent_runs WHERE status IN ('accepted', 'rejected') ORDER BY id`)
	if err != nil {
		var pe *pq.Error
		if errors.As(err, &pe) && (pe.Code == "42P01" || pe.Code == "42703") { // 032 not applied
			return nil, ErrUnavailable
		}
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []AgentRun
	for rows.Next() {
		var r AgentRun
		if err := rows.Scan(&r.ID, &r.Status, &r.ReviewReason, &r.BaseCommit, &r.HeadCommit); err != nil {
			return nil, err
		}
		r.BaseCommit, r.HeadCommit = trimChar(r.BaseCommit), trimChar(r.HeadCommit)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *pgStore) Quarantine(ctx context.Context, id string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	res, err := s.app.ExecContext(ctx, `UPDATE zhen_captures SET quarantined = true WHERE id = $1`, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

func (s *pgStore) Promote(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	_, err := s.app.ExecContext(ctx, `UPDATE zhen_captures SET promoted = true WHERE id = ANY($1) AND NOT promoted`, pq.Array(ids))
	return err
}
