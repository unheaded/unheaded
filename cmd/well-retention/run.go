// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ERDA 5.4's partition trigger (WELL-ERDA-AUDIT-2026-10-13.md §5.4).
const (
	partitionRows     = 1_000_000
	partitionBytes    = 1 << 30
	partitionLeadDays = 90
)

const day = 24 * time.Hour

// verdict is one partition check, stored as retention_runs.detail.
type verdict struct {
	Rows        int64      `json:"rows"`
	Bytes       int64      `json:"bytes"`
	Oldest      *time.Time `json:"oldest,omitempty"`
	PartitionBy *time.Time `json:"partition_by,omitempty"`
	Due         bool       `json:"due"`
	Reason      string     `json:"reason,omitempty"`
}

// result is one class in one run: a retention_runs row.
type result struct {
	Class, Table, Action string
	Started, Finished    time.Time
	Cutoff               *time.Time
	Eligible             int64
	Deleted              int64
	Archived             int64
	Held                 int64
	Redactions           int
	Verdict              *verdict
	Err                  string
}

func cutoff(now time.Time, days int) time.Time { return now.Add(-time.Duration(days) * day) }

// partitionVerdict applies the trigger: due at 1 M rows, at 1 GB, or once
// the oldest row is within 90 days of the online age (partition_by).
func partitionVerdict(now time.Time, rows, bytes int64, oldest *time.Time, onlineDays int) verdict {
	v := verdict{Rows: rows, Bytes: bytes, Oldest: oldest}
	var why []string
	if rows >= partitionRows {
		why = append(why, fmt.Sprintf("%d rows >= %d", rows, partitionRows))
	}
	if bytes >= partitionBytes {
		why = append(why, fmt.Sprintf("%d bytes >= 1 GiB", bytes))
	}
	if oldest != nil && onlineDays > 0 {
		by := oldest.Add(time.Duration(onlineDays-partitionLeadDays) * day)
		v.PartitionBy = &by
		if !now.Before(by) {
			why = append(why, fmt.Sprintf("oldest row %s is within %d days of the %d-day online age",
				oldest.UTC().Format("2006-01-02"), partitionLeadDays, onlineDays))
		}
	}
	v.Due = len(why) > 0
	v.Reason = strings.Join(why, "; ")
	return v
}

// runner executes classes against the two databases and records each.
type runner struct {
	app, ops    *sql.DB
	dryRun      bool
	batch       int
	stmtTimeout time.Duration
	runID, host string
	now         func() time.Time
}

func (r *runner) db(name string) *sql.DB {
	if name == dbApp {
		return r.app
	}
	return r.ops
}

// runClass runs one class and records it in retention_runs; a failure to
// record is a failure of the class.
func (r *runner) runClass(ctx context.Context, c Class) result {
	res := result{Class: c.Name, Table: c.Table, Action: c.Action, Started: r.now()}
	err := r.do(ctx, c, &res)
	res.Finished = r.now()
	if err != nil {
		res.Err = clip(err.Error(), 512)
	}
	if rerr := r.record(ctx, res); rerr != nil {
		msg := "record retention_runs: " + rerr.Error()
		if res.Err != "" {
			msg = res.Err + "; " + msg
		}
		res.Err = clip(msg, 512)
	}
	return res
}

func (r *runner) do(ctx context.Context, c Class, res *result) error {
	tg := targets[c.Table]
	db := r.db(tg.db)
	if db == nil {
		return fmt.Errorf("no connection to %s", tg.db)
	}
	if c.Action == actionPartitionCheck {
		return r.partitionCheck(ctx, db, c, tg, res)
	}
	cut := cutoff(res.Started, c.OnlineDays)
	res.Cutoff = &cut
	var err error
	if c.Action == actionPurgeFunction {
		err = db.QueryRowContext(ctx, tg.count).Scan(&res.Eligible)
	} else {
		err = db.QueryRowContext(ctx, tg.count, cut).Scan(&res.Eligible)
	}
	if err != nil {
		return fmt.Errorf("count: %w", err)
	}
	if tg.held != "" {
		if err := db.QueryRowContext(ctx, tg.held, cut).Scan(&res.Held); err != nil {
			return fmt.Errorf("count held: %w", err)
		}
	}
	if c.Hold {
		res.Action = actionHeld
		res.Held += res.Eligible
		return nil
	}
	if r.dryRun {
		return nil
	}
	switch c.Action {
	case actionPurge:
		return r.purge(ctx, db, tg, cut, res)
	case actionPurgeFunction:
		return r.inTx(ctx, db, func(tx *sql.Tx) error {
			return tx.QueryRowContext(ctx, tg.purgeFn).Scan(&res.Deleted)
		})
	case actionArchivePurge:
		return r.archivePurge(ctx, db, c, tg, cut, res)
	}
	return fmt.Errorf("action %q has no executor", c.Action)
}

func (r *runner) partitionCheck(ctx context.Context, db *sql.DB, c Class, tg target, res *result) error {
	var rows, bytes int64
	if err := db.QueryRowContext(ctx, sizeQuery, c.Table).Scan(&rows, &bytes); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("table %s does not exist in %s (migration not applied?)", c.Table, tg.db)
		}
		return fmt.Errorf("size: %w", err)
	}
	var oldest sql.NullTime
	if err := db.QueryRowContext(ctx, tg.oldest).Scan(&oldest); err != nil {
		return fmt.Errorf("oldest row: %w", err)
	}
	var o *time.Time
	if oldest.Valid {
		t := oldest.Time.UTC()
		o = &t
	}
	v := partitionVerdict(res.Started, rows, bytes, o, c.OnlineDays)
	res.Verdict = &v
	return nil
}

// inTx runs fn in a transaction with the run's statement timeout.
func (r *runner) inTx(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := r.setTimeout(ctx, tx); err != nil {
		tx.Rollback() // #nosec G104 -- the timeout error is the one returned
		return err
	}
	if err := fn(tx); err != nil {
		tx.Rollback() // #nosec G104 -- fn's error is the one returned
		return err
	}
	return tx.Commit()
}

func (r *runner) setTimeout(ctx context.Context, tx *sql.Tx) error {
	ms := fmt.Sprintf("%d", r.stmtTimeout.Milliseconds())
	_, err := tx.ExecContext(ctx, `SELECT set_config('statement_timeout', $1, true)`, ms)
	return err
}

// purge deletes in batches, one transaction each, until a batch is short.
func (r *runner) purge(ctx context.Context, db *sql.DB, tg target, cut time.Time, res *result) error {
	for {
		var n int64
		err := r.inTx(ctx, db, func(tx *sql.Tx) error {
			out, err := tx.ExecContext(ctx, tg.purge, cut, r.batch)
			if err != nil {
				return err
			}
			n, err = out.RowsAffected()
			return err
		})
		if err != nil {
			return fmt.Errorf("purge after %d rows: %w", res.Deleted, err)
		}
		res.Deleted += n
		if n < int64(r.batch) {
			return nil
		}
	}
}

type archived struct {
	key string
	row []byte
	n   int
}

// archivePurge deletes a batch, copies it (redacted) into
// retention_archive and commits the copy, and only then commits the
// delete. A failed copy rolls the delete back; a failed delete commit
// leaves extra copies of rows that are still live, never a lost row.
func (r *runner) archivePurge(ctx context.Context, db *sql.DB, c Class, tg target, cut time.Time, res *result) error {
	for {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		batch, err := r.deleteBatch(ctx, tx, tg, cut)
		if err != nil {
			tx.Rollback() // #nosec G104 -- the batch error is the one returned
			return fmt.Errorf("archive batch after %d rows: %w", res.Deleted, err)
		}
		if len(batch) == 0 {
			return tx.Rollback()
		}
		if err := r.insertArchive(ctx, c, tg, batch); err != nil {
			tx.Rollback() // #nosec G104 -- the archive error is the one returned; nothing was deleted
			return fmt.Errorf("archive copy (nothing deleted): %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("delete commit failed after the archive copy: %d rows of run %s are in retention_archive and still live: %w",
				len(batch), r.runID, err)
		}
		for _, a := range batch {
			res.Redactions += a.n
		}
		res.Archived += int64(len(batch))
		res.Deleted += int64(len(batch))
		if len(batch) < r.batch {
			return nil
		}
	}
}

func (r *runner) deleteBatch(ctx context.Context, tx *sql.Tx, tg target, cut time.Time) ([]archived, error) {
	if err := r.setTimeout(ctx, tx); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, tg.archive, cut, r.batch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []archived
	for rows.Next() {
		var key string
		var raw []byte
		if err := rows.Scan(&key, &raw); err != nil {
			return nil, err
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, fmt.Errorf("row %s: %w", key, err)
		}
		red, n := redactRow(m)
		b, err := json.Marshal(red)
		if err != nil {
			return nil, err
		}
		out = append(out, archived{key: key, row: b, n: n})
	}
	return out, rows.Err()
}

func (r *runner) insertArchive(ctx context.Context, c Class, tg target, batch []archived) error {
	return r.inTx(ctx, r.ops, func(tx *sql.Tx) error {
		for _, a := range batch {
			if _, err := tx.ExecContext(ctx, `INSERT INTO retention_archive
				(run_id, class, source_db, source_table, source_key, row_data, redactions)
				VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7)`,
				r.runID, c.Name, tg.db, c.Table, a.key, string(a.row), a.n); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *runner) record(ctx context.Context, res result) error {
	detail := map[string]any{}
	if res.Verdict != nil {
		detail["partition"] = res.Verdict
	}
	if res.Redactions > 0 {
		detail["redactions"] = res.Redactions
	}
	d, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	var cut any
	if res.Cutoff != nil {
		cut = *res.Cutoff
	}
	_, err = r.ops.ExecContext(ctx, `INSERT INTO retention_runs
		(run_id, class, action, dry_run, host, started_at, finished_at, cutoff, eligible, deleted, archived, held, detail, error)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13::jsonb, $14)`,
		r.runID, res.Class, res.Action, r.dryRun, r.host, res.Started, res.Finished, cut,
		res.Eligible, res.Deleted, res.Archived, res.Held, string(d), res.Err)
	return err
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8Start(s[n]) {
		n--
	}
	return s[:n]
}

func utf8Start(b byte) bool { return b&0xC0 != 0x80 }
