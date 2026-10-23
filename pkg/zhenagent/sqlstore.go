// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package zhenagent

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

// SQLStore is Store over the Well's unheaded_app as app_zhen (migration
// 032). Every statement is parameterised; every call has the caller's
// deadline.
type SQLStore struct{ DB *sql.DB }

var _ Store = SQLStore{}

// eligibleTypes as a SQL array, in a stable order.
var eligibleTypeList = []string{"bug", "docs", "task", "tech-debt"}

// Candidates implements Store.
func (s SQLStore) Candidates(ctx context.Context, limit int) ([]Card, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT t.id, t.title, t.description, t.status, t.type, t.owner
		  FROM kanban_tasks t
		 WHERE t.owner = $1 AND t.status = 'todo' AND t.type = ANY($2)
		   AND t.deleted_at IS NULL AND t.archived_at IS NULL
		   AND NOT EXISTS (SELECT 1 FROM zhen_agent_runs r WHERE r.card_id = t.id AND r.status IN ('claimed', 'running'))
		   AND (SELECT count(*) FROM zhen_agent_runs r WHERE r.card_id = t.id AND r.status <> 'cancelled') < $3
		 ORDER BY t.created_at, t.id
		 LIMIT $4`, Owner, pq.Array(eligibleTypeList), MaxAttempts, limit)
	if err != nil {
		return nil, fmt.Errorf("zhenagent: candidates: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Card
	for rows.Next() {
		var c Card
		if err := rows.Scan(&c.ID, &c.Title, &c.Description, &c.Status, &c.Type, &c.Owner); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Attempts implements Store.
func (s SQLStore) Attempts(ctx context.Context, cardID string) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM zhen_agent_runs WHERE card_id = $1 AND status <> 'cancelled'`, cardID).Scan(&n)
	return n, err
}

// Claim implements Store: lock the card (SKIP LOCKED: another worker
// holding it means a conflict, not a wait), check it is still an eligible
// todo card, insert the run, move the card. The one-active index is the
// last word on a race.
func (s SQLStore) Claim(ctx context.Context, r Run) error {
	if !ValidRunID(r.ID) {
		return fmt.Errorf("zhenagent: bad run id %q", r.ID)
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var id string
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM kanban_tasks
		 WHERE id = $1 AND owner = $2 AND status = 'todo' AND type = ANY($3)
		   AND deleted_at IS NULL AND archived_at IS NULL
		 FOR UPDATE SKIP LOCKED`, r.CardID, Owner, pq.Array(eligibleTypeList)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrClaimConflict
	}
	if err != nil {
		return fmt.Errorf("zhenagent: lock card: %w", err)
	}
	lease := r.LeaseUntil
	if lease.IsZero() {
		lease = time.Now().Add(DefaultLease)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO zhen_agent_runs (id, card_id, lease_until, mode, family, branch, base_commit)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`, r.ID, r.CardID, lease, r.Mode, r.Family, r.Branch, r.BaseCommit)
	if err != nil {
		var pe *pq.Error
		if errors.As(err, &pe) && pe.Code == "23505" {
			return ErrClaimConflict
		}
		return fmt.Errorf("zhenagent: insert run: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE kanban_tasks SET status = 'in-progress', updated_at = now() WHERE id = $1`, r.CardID); err != nil {
		return fmt.Errorf("zhenagent: move card: %w", err)
	}
	return tx.Commit()
}

func (s SQLStore) exec1(ctx context.Context, q string, args ...any) error {
	res, err := s.DB.ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrNotInState
	}
	return nil
}

// Start implements Store.
func (s SQLStore) Start(ctx context.Context, id string) error {
	return s.exec1(ctx, `UPDATE zhen_agent_runs SET status = 'running' WHERE id = $1 AND status = 'claimed'`, id)
}

// Renew implements Store.
func (s SQLStore) Renew(ctx context.Context, id string, until time.Time) error {
	return s.exec1(ctx, `UPDATE zhen_agent_runs SET lease_until = $2 WHERE id = $1 AND status IN ('claimed', 'running')`, id, until)
}

// Finish implements Store.
func (s SQLStore) Finish(ctx context.Context, id string, o Outcome) error {
	sum, err := json.Marshal(o.Summary)
	if err != nil {
		return err
	}
	var head any
	if o.HeadCommit != "" {
		head = o.HeadCommit
	}
	err = s.exec1(ctx, `
		UPDATE zhen_agent_runs SET status = $2, head_commit = $3, route_summary = $4, claude_nano_usd = $5, error = $6
		 WHERE id = $1 AND status IN ('claimed', 'running')`,
		id, o.Status, head, string(sum), o.ClaudeNanoUSD, truncate(o.Error, MaxErrorBytes))
	if err != nil && !errors.Is(err, ErrNotInState) {
		return fmt.Errorf("zhenagent: finish %s: %w", id, err)
	}
	return err
}

// MoveCard implements Store.
func (s SQLStore) MoveCard(ctx context.Context, cardID, status string, progress int, entry *CommitEntry) error {
	if progress < KeepProgress || progress > 100 {
		return fmt.Errorf("zhenagent: progress %d", progress)
	}
	if entry == nil {
		return s.exec1(ctx, `UPDATE kanban_tasks SET status = $2, progress = CASE WHEN $3 < 0 THEN progress ELSE $3 END, updated_at = now() WHERE id = $1`, cardID, status, progress)
	}
	b, err := json.Marshal([]CommitEntry{*entry})
	if err != nil {
		return err
	}
	return s.exec1(ctx, `
		UPDATE kanban_tasks SET status = $2, progress = CASE WHEN $3 < 0 THEN progress ELSE $3 END, updated_at = now(),
		       commits = (CASE WHEN jsonb_typeof(commits) = 'array' THEN commits ELSE '[]'::jsonb END) || $4::jsonb
		 WHERE id = $1`, cardID, status, progress, string(b))
}

// ExpireLeases implements Store.
func (s SQLStore) ExpireLeases(ctx context.Context, now time.Time) ([]Run, error) {
	rows, err := s.DB.QueryContext(ctx, `
		UPDATE zhen_agent_runs SET status = 'abandoned', error = 'lease expired'
		 WHERE status IN ('claimed', 'running') AND lease_until <= $1
		RETURNING `+runCols, now)
	if err != nil {
		return nil, fmt.Errorf("zhenagent: expire leases: %w", err)
	}
	return scanRuns(rows)
}

const runCols = `id, card_id, claimed_at, lease_until, status, mode, family, branch, base_commit,
	COALESCE(head_commit, ''), route_summary::text, claude_nano_usd, COALESCE(review_reason, ''), reviewed_at, error`

func scanRuns(rows *sql.Rows) ([]Run, error) {
	defer func() { _ = rows.Close() }()
	var out []Run
	for rows.Next() {
		var r Run
		var sum string
		var reviewed sql.NullTime
		if err := rows.Scan(&r.ID, &r.CardID, &r.ClaimedAt, &r.LeaseUntil, &r.Status, &r.Mode, &r.Family, &r.Branch,
			&r.BaseCommit, &r.HeadCommit, &sum, &r.ClaudeNanoUSD, &r.ReviewReason, &reviewed, &r.Error); err != nil {
			return nil, err
		}
		r.BaseCommit, r.HeadCommit = strings.TrimSpace(r.BaseCommit), strings.TrimSpace(r.HeadCommit)
		if err := json.Unmarshal([]byte(sum), &r.Summary); err != nil {
			return nil, fmt.Errorf("zhenagent: route_summary of %s: %w", r.ID, err)
		}
		if reviewed.Valid {
			t := reviewed.Time
			r.ReviewedAt = &t
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// List implements Store.
func (s SQLStore) List(ctx context.Context, limit int) ([]Run, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+runCols+` FROM zhen_agent_runs ORDER BY claimed_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("zhenagent: list: %w", err)
	}
	return scanRuns(rows)
}

// Get implements Store.
func (s SQLStore) Get(ctx context.Context, id string) (Run, error) {
	if !ValidRunID(id) {
		return Run{}, ErrNoRun
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+runCols+` FROM zhen_agent_runs WHERE id = $1`, id)
	if err != nil {
		return Run{}, fmt.Errorf("zhenagent: get: %w", err)
	}
	rs, err := scanRuns(rows)
	if err != nil {
		return Run{}, err
	}
	if len(rs) == 0 {
		return Run{}, ErrNoRun
	}
	return rs[0], nil
}

// Review implements Store.
func (s SQLStore) Review(ctx context.Context, id, status, reason string) error {
	if status != StatusAccepted && status != StatusRejected {
		return fmt.Errorf("zhenagent: review status %q", status)
	}
	var r any
	if reason != "" {
		r = truncate(reason, 1000)
	}
	return s.exec1(ctx, `UPDATE zhen_agent_runs SET status = $2, review_reason = $3, reviewed_at = now() WHERE id = $1 AND status = 'succeeded'`, id, status, r)
}
