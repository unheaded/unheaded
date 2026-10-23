// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package claudebridge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
)

// SQLConfirmQueue is the Well's unheaded_app.zhen_pending_confirmations
// (migration 031), written as app_zhen. The table's triggers enforce the
// one-way states, the immutable call, the expiry and the cap; this type
// checks the same things first so errors read well.
type SQLConfirmQueue struct{ DB *sql.DB }

// ErrNotPending is a decision on a row that is no longer pending (already
// decided, or unknown).
var ErrNotPending = errors.New("confirmation is not pending")

var queueIDRE = regexp.MustCompile(`^[0-9a-f]{16}$`)

// ValidQueueID reports whether id has the queue's shape (16 hex).
func ValidQueueID(id string) bool { return queueIDRE.MatchString(id) }

const queueColumns = `id, created_at, expires_at, source_service, emitted_by, project_root, tool, call,
	call_sha256, summary, coalesce(card_id, ''), status, decided_at, coalesce(decided_by, '')`

// Enqueue implements ConfirmQueue.
func (q SQLConfirmQueue) Enqueue(ctx context.Context, c QueuedCall) error {
	if q.DB == nil {
		return errors.New("claudebridge: SQLConfirmQueue has no database")
	}
	if !ValidQueueID(c.ID) {
		return fmt.Errorf("queue id %q is not 16 hex", c.ID)
	}
	var card sql.NullString
	if c.CardID != "" {
		card = sql.NullString{String: c.CardID, Valid: true}
	}
	const stmt = `INSERT INTO zhen_pending_confirmations
		(id, created_at, expires_at, source_service, emitted_by, project_root, tool, call, call_sha256, summary, card_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9, $10, $11)`
	_, err := q.DB.ExecContext(ctx, stmt, c.ID, c.CreatedAt, c.ExpiresAt, c.SourceService, c.EmittedBy, c.ProjectRoot,
		c.Tool, string(c.Call), c.CallSHA256, c.Summary, card)
	return err
}

// CountPending implements ConfirmQueue: unexpired pending rows of source.
func (q SQLConfirmQueue) CountPending(ctx context.Context, source string) (int, error) {
	var n int
	err := q.DB.QueryRowContext(ctx, `SELECT count(*) FROM zhen_pending_confirmations
		WHERE status = 'pending' AND source_service = $1 AND expires_at > now()`, source).Scan(&n)
	return n, err
}

// List returns pending rows, oldest first.
func (q SQLConfirmQueue) List(ctx context.Context, limit int) ([]QueuedCall, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := q.DB.QueryContext(ctx, `SELECT `+queueColumns+` FROM zhen_pending_confirmations
		WHERE status = 'pending' ORDER BY created_at LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []QueuedCall
	for rows.Next() {
		c, err := scanQueued(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Get returns one row by id.
func (q SQLConfirmQueue) Get(ctx context.Context, id string) (QueuedCall, error) {
	if !ValidQueueID(id) {
		return QueuedCall{}, fmt.Errorf("queue id %q is not 16 hex", id)
	}
	return scanQueued(q.DB.QueryRowContext(ctx, `SELECT `+queueColumns+` FROM zhen_pending_confirmations WHERE id = $1`, id))
}

// Decide moves a pending row to approved, denied or expired. It changes
// exactly one row or returns ErrNotPending.
func (q SQLConfirmQueue) Decide(ctx context.Context, id, status, by string) error {
	switch status {
	case "approved", "denied", "expired":
	default:
		return fmt.Errorf("cannot decide %q", status)
	}
	res, err := q.DB.ExecContext(ctx, `UPDATE zhen_pending_confirmations SET status = $2, decided_at = now(), decided_by = $3
		WHERE id = $1 AND status = 'pending'`, id, status, by)
	if err != nil {
		return err
	}
	return oneRow(res)
}

// Finish records the outcome of an approved call: executed or failed.
func (q SQLConfirmQueue) Finish(ctx context.Context, id, status string) error {
	if status != "executed" && status != "failed" {
		return fmt.Errorf("cannot finish as %q", status)
	}
	res, err := q.DB.ExecContext(ctx, `UPDATE zhen_pending_confirmations SET status = $2
		WHERE id = $1 AND status = 'approved'`, id, status)
	if err != nil {
		return err
	}
	return oneRow(res)
}

func oneRow(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrNotPending
	}
	return nil
}

type rowScanner interface{ Scan(dest ...any) error }

func scanQueued(r rowScanner) (QueuedCall, error) {
	var c QueuedCall
	var call []byte
	var decided sql.NullTime
	if err := r.Scan(&c.ID, &c.CreatedAt, &c.ExpiresAt, &c.SourceService, &c.EmittedBy, &c.ProjectRoot, &c.Tool, &call,
		&c.CallSHA256, &c.Summary, &c.CardID, &c.Status, &decided, &c.DecidedBy); err != nil {
		return QueuedCall{}, err
	}
	c.Call = json.RawMessage(call)
	if decided.Valid {
		t := decided.Time
		c.DecidedAt = &t
	}
	c.CreatedAt, c.ExpiresAt = c.CreatedAt.UTC(), c.ExpiresAt.UTC()
	return c, nil
}
