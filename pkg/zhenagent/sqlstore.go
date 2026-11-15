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

// SQLStore is Store over two pools of the Well (ADR-149, S-ZHEN-SWITCH D2
// (a)): Runs is unheaded_zhen as app_zhen (zhen_agent_runs, migration 052),
// Board is unheaded_app as app_zhen (kanban_tasks and kanban_zhen_optin, the
// board's own tables). Every statement names one database's tables only, and
// is parameterised; every call has the caller's deadline.
//
// A tool that only reads or reviews runs (zhen-claude review) may leave Board
// nil: a method that needs the board then returns ErrNoBoard.
type SQLStore struct{ Board, Runs *sql.DB }

// ErrNoBoard is returned by a method that moves or reads cards on a store
// built without the board's pool.
var ErrNoBoard = errors.New("zhenagent: this store has no board pool (WELL_APP_ZHEN_DSN)")

// NewSQLStore is the worker's store: both pools are required.
func NewSQLStore(board, runs *sql.DB) (SQLStore, error) {
	if board == nil || runs == nil {
		return SQLStore{}, errors.New("zhenagent: NewSQLStore needs the board pool (unheaded_app) and the runs pool (unheaded_zhen)")
	}
	return SQLStore{Board: board, Runs: runs}, nil
}

var (
	_ Store          = SQLStore{}
	_ ScheduledStore = SQLStore{}
)

// eligibleTypes as a SQL array, in a stable order.
var eligibleTypeList = []string{"bug", "docs", "task", "tech-debt"}

// candidateScan bounds the board read before the runs filter.
const candidateScan = 200

// Candidates implements Store: the board's eligible cards (one database),
// then the ones with no active run and fewer than MaxAttempts counted runs
// (the other).
func (s SQLStore) Candidates(ctx context.Context, limit int) ([]Card, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if s.Board == nil {
		return nil, ErrNoBoard
	}
	rows, err := s.Board.QueryContext(ctx, `
		SELECT t.id, t.title, t.description, t.status, t.type, t.owner, o.card_sha256
		  FROM kanban_tasks t
		  JOIN kanban_zhen_optin o ON o.card_id = t.id
		 WHERE t.owner = $1 AND t.status = 'todo' AND t.type = ANY($2)
		   AND t.deleted_at IS NULL AND t.archived_at IS NULL
		 ORDER BY t.created_at, t.id
		 LIMIT $3`, Owner, pq.Array(eligibleTypeList), candidateScan)
	if err != nil {
		return nil, fmt.Errorf("zhenagent: candidates: %w", err)
	}
	var cards []Card
	for rows.Next() {
		var c Card
		if err := rows.Scan(&c.ID, &c.Title, &c.Description, &c.Status, &c.Type, &c.Owner, &c.OptInSHA256); err != nil {
			_ = rows.Close()
			return nil, err
		}
		cards = append(cards, c)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()
	if len(cards) == 0 {
		return nil, nil
	}
	ids := make([]string, len(cards))
	for i, c := range cards {
		ids[i] = c.ID
	}
	rrows, err := s.Runs.QueryContext(ctx, `
		SELECT card_id, bool_or(status IN ('claimed', 'running')), count(*) FILTER (WHERE status <> 'cancelled')
		  FROM zhen_agent_runs WHERE card_id = ANY($1) GROUP BY card_id`, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("zhenagent: candidates' runs: %w", err)
	}
	defer func() { _ = rrows.Close() }()
	skip := map[string]bool{}
	for rrows.Next() {
		var id string
		var active bool
		var n int
		if err := rrows.Scan(&id, &active, &n); err != nil {
			return nil, err
		}
		if active || n >= MaxAttempts {
			skip[id] = true
		}
	}
	if err := rrows.Err(); err != nil {
		return nil, err
	}
	var out []Card
	for _, c := range cards {
		if skip[c.ID] {
			continue
		}
		out = append(out, c)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

// Attempts implements Store.
func (s SQLStore) Attempts(ctx context.Context, cardID string) (int, error) {
	var n int
	err := s.Runs.QueryRowContext(ctx, `SELECT count(*) FROM zhen_agent_runs WHERE card_id = $1 AND status <> 'cancelled'`, cardID).Scan(&n)
	return n, err
}

// Claim implements Store in two steps, because the run (unheaded_zhen) and
// the card (unheaded_app) are in two databases (ADR-149, D2 (a)):
//
//  1. insert the run, born claimed: the one-active index on
//     zhen_agent_runs is the last word on a race (23505: ErrClaimConflict);
//  2. on the board, one transaction: lock the card (SKIP LOCKED: another
//     worker holding it is a conflict, not a wait), check it is still an
//     eligible todo card whose title and description are the ones an
//     operator opted in, move it to in-progress.
//
// If step 2 finds no card or fails, the run is moved to cancelled, which
// Attempts does not count, so a refused claim costs the card no attempt.
// The run is the record; a card the second step could not move is put right
// by scripts/well-reconcile-kanban.sh.
func (s SQLStore) Claim(ctx context.Context, r Run) error {
	if !ValidRunID(r.ID) {
		return fmt.Errorf("zhenagent: bad run id %q", r.ID)
	}
	if s.Board == nil {
		return ErrNoBoard
	}
	lease := r.LeaseUntil
	if lease.IsZero() {
		lease = time.Now().Add(DefaultLease)
	}
	_, err := s.Runs.ExecContext(ctx, `
		INSERT INTO zhen_agent_runs (id, card_id, lease_until, mode, family, branch, base_commit)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`, r.ID, r.CardID, lease, r.Mode, r.Family, r.Branch, r.BaseCommit)
	if err != nil {
		var pe *pq.Error
		if errors.As(err, &pe) && pe.Code == "23505" {
			return ErrClaimConflict
		}
		return fmt.Errorf("zhenagent: insert run: %w", err)
	}
	moved, err := s.claimCard(ctx, r.CardID)
	if err == nil && moved {
		return nil
	}
	// Undo step 1 on a fresh context: the caller's may be what failed.
	cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if _, cerr := s.Runs.ExecContext(cctx, `UPDATE zhen_agent_runs SET status = 'cancelled' WHERE id = $1 AND status = 'claimed'`, r.ID); cerr != nil {
		if err == nil {
			err = ErrClaimConflict
		}
		return fmt.Errorf("zhenagent: claim %s: card step: %w; and the run could not be cancelled: %v", r.ID, err, cerr)
	}
	if err != nil {
		return fmt.Errorf("zhenagent: claim %s: %w", r.ID, err)
	}
	return ErrClaimConflict
}

// claimCard is Claim's board step: true when the card was eligible and moved.
func (s SQLStore) claimCard(ctx context.Context, cardID string) (bool, error) {
	tx, err := s.Board.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	var id string
	err = tx.QueryRowContext(ctx, `
		SELECT t.id FROM kanban_tasks t
		  JOIN kanban_zhen_optin o ON o.card_id = t.id
		 WHERE t.id = $1 AND t.owner = $2 AND t.status = 'todo' AND t.type = ANY($3)
		   AND t.deleted_at IS NULL AND t.archived_at IS NULL
		   AND o.card_sha256 = encode(sha256(convert_to(t.title, 'UTF8') || '\x00'::bytea || convert_to(t.description, 'UTF8')), 'hex')
		 FOR UPDATE OF t SKIP LOCKED`, cardID, Owner, pq.Array(eligibleTypeList)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("lock card: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE kanban_tasks SET status = 'in-progress', updated_at = now() WHERE id = $1`, cardID); err != nil {
		return false, fmt.Errorf("move card: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("move card: %w", err)
	}
	return true, nil
}

// exec1 runs one statement on db and wants exactly one row changed.
func exec1(ctx context.Context, db *sql.DB, q string, args ...any) error {
	if db == nil {
		return ErrNoBoard
	}
	res, err := db.ExecContext(ctx, q, args...)
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
	return exec1(ctx, s.Runs, `UPDATE zhen_agent_runs SET status = 'running' WHERE id = $1 AND status = 'claimed'`, id)
}

// Renew implements Store.
func (s SQLStore) Renew(ctx context.Context, id string, until time.Time) error {
	return exec1(ctx, s.Runs, `UPDATE zhen_agent_runs SET lease_until = $2 WHERE id = $1 AND status IN ('claimed', 'running')`, id, until)
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
	err = exec1(ctx, s.Runs, `
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
		return exec1(ctx, s.Board, `UPDATE kanban_tasks SET status = $2, progress = CASE WHEN $3 < 0 THEN progress ELSE $3 END, updated_at = now() WHERE id = $1`, cardID, status, progress)
	}
	b, err := json.Marshal([]CommitEntry{*entry})
	if err != nil {
		return err
	}
	return exec1(ctx, s.Board, `
		UPDATE kanban_tasks SET status = $2, progress = CASE WHEN $3 < 0 THEN progress ELSE $3 END, updated_at = now(),
		       commits = (CASE WHEN jsonb_typeof(commits) = 'array' THEN commits ELSE '[]'::jsonb END) || $4::jsonb
		 WHERE id = $1`, cardID, status, progress, string(b))
}

// ExpireLeases implements Store.
func (s SQLStore) ExpireLeases(ctx context.Context, now time.Time) ([]Run, error) {
	rows, err := s.Runs.QueryContext(ctx, `
		UPDATE zhen_agent_runs SET status = 'abandoned', error = 'lease expired'
		 WHERE status IN ('claimed', 'running') AND lease_until <= $1
		RETURNING `+runCols, now)
	if err != nil {
		return nil, fmt.Errorf("zhenagent: expire leases: %w", err)
	}
	return scanRuns(rows)
}

const runCols = `id, card_id, claimed_at, lease_until, status, mode, family, COALESCE(branch, ''), COALESCE(base_commit, ''),
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
	rows, err := s.Runs.QueryContext(ctx, `SELECT `+runCols+` FROM zhen_agent_runs ORDER BY claimed_at DESC LIMIT $1`, limit)
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
	rows, err := s.Runs.QueryContext(ctx, `SELECT `+runCols+` FROM zhen_agent_runs WHERE id = $1`, id)
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
	return exec1(ctx, s.Runs, `UPDATE zhen_agent_runs SET status = $2, review_reason = $3, reviewed_at = now() WHERE id = $1 AND status = 'succeeded' AND mode <> 'scheduled'`, id, status, r)
}

// BeginScheduled implements ScheduledStore: one transaction inserts the
// run (born claimed, 052's trigger) and moves it to running. The scheduler
// creates the card first; unheaded_zhen holds no foreign key to the board
// (ADR-149), so the card's existence is the scheduler's order, not a check.
func (s SQLStore) BeginScheduled(ctx context.Context, r Run) error {
	if err := checkScheduled(r, time.Now()); err != nil {
		return err
	}
	tx, err := s.Runs.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO zhen_agent_runs (id, card_id, lease_until, mode, family)
		VALUES ($1, $2, $3, 'scheduled', $4)`, r.ID, r.CardID, r.LeaseUntil, r.Family); err != nil {
		return fmt.Errorf("zhenagent: insert scheduled run: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE zhen_agent_runs SET status = 'running' WHERE id = $1 AND status = 'claimed'`, r.ID); err != nil {
		return fmt.Errorf("zhenagent: start scheduled run: %w", err)
	}
	return tx.Commit()
}
