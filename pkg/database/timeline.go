// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package database

// TimelineStore is timeguru's home in the Well: unheaded_app's
// timeline_milestones (migration 050, ADR-147), one row per milestone of
// references/timeline.md, written as app_timeguru. The file is the source;
// a row changed through timeguru's API after the file's modification time
// wins over the file at start (NewerThan). It replaces a store for 003's
// sprint/title shape that nothing called.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/lib/pq"
)

// TimelineMigration is the migration timeguru needs (RequireMigrations).
const TimelineMigration = "050_app_timeline_milestones.sql"

// ErrMilestoneNotFound is returned when no row has the key.
var ErrMilestoneNotFound = errors.New("milestone not found")

// AppTimeguruConfig returns the database config for timeguru: the scoped
// role app_timeguru on unheaded_app's timeline_milestones. There is no
// default password.
func AppTimeguruConfig() Config {
	return Config{
		Host:        getEnv("WELL_HOST", "localhost"),
		Port:        getEnv("WELL_PORT", "5432"),
		User:        getEnv("WELL_USER", "app_timeguru"),
		Password:    os.Getenv("WELL_PASSWORD"),
		DBName:      getEnv("WELL_DB", "unheaded_app"),
		SSLMode:     getEnv("WELL_SSLMODE", "require"),
		SSLRootCert: os.Getenv("WELL_SSLROOTCERT"),
	}
}

// MilestoneRow is one timeline_milestones row.
type MilestoneRow struct {
	Key         string
	Name        string
	Status      string
	ProgressPct int
	ETA         *time.Time
	Owner       string
	Risk        string
	Description string
	UpdatedAt   time.Time
}

// TimelineStore reads and writes timeline_milestones.
type TimelineStore struct {
	db *sql.DB
}

// NewTimelineStore wraps a connection as app_timeguru.
func NewTimelineStore(db *sql.DB) *TimelineStore {
	return &TimelineStore{db: db}
}

// maxMilestones bounds one ReplaceMilestones: the file holds under a hundred.
const maxMilestones = 10000

// ReplaceMilestones makes the table hold exactly rows, in one transaction:
// every row is upserted (updated_at moves only when a value changed) and
// every key not in rows is deleted.
func (s *TimelineStore) ReplaceMilestones(ctx context.Context, rows []MilestoneRow) error {
	if len(rows) > maxMilestones {
		return fmt.Errorf("replace milestones: %d rows, more than %d", len(rows), maxMilestones)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	keys := make([]string, 0, len(rows))
	for _, r := range rows {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO timeline_milestones AS t
			    (milestone_key, name, status, progress_pct, eta, owner, risk, description)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			 ON CONFLICT (milestone_key) DO UPDATE SET
			    name = EXCLUDED.name, status = EXCLUDED.status, progress_pct = EXCLUDED.progress_pct,
			    eta = EXCLUDED.eta, owner = EXCLUDED.owner, risk = EXCLUDED.risk,
			    description = EXCLUDED.description, updated_at = now()
			 WHERE (t.name, t.status, t.progress_pct, t.eta, t.owner, t.risk, t.description)
			       IS DISTINCT FROM
			       (EXCLUDED.name, EXCLUDED.status, EXCLUDED.progress_pct, EXCLUDED.eta,
			        EXCLUDED.owner, EXCLUDED.risk, EXCLUDED.description)`,
			r.Key, r.Name, r.Status, r.ProgressPct, r.ETA, r.Owner, r.Risk, r.Description); err != nil {
			return fmt.Errorf("upsert milestone %q: %w", r.Key, err)
		}
		keys = append(keys, r.Key)
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM timeline_milestones WHERE NOT (milestone_key = ANY($1::text[]))`, pq.Array(keys)); err != nil {
		return fmt.Errorf("delete milestones not in the file: %w", err)
	}
	return tx.Commit()
}

// UpdateMilestone sets one row's progress and status.
func (s *TimelineStore) UpdateMilestone(ctx context.Context, key string, progress int, status string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE timeline_milestones SET progress_pct = $2, status = $3, updated_at = now()
		 WHERE milestone_key = $1`, key, progress, status)
	if err != nil {
		return fmt.Errorf("update milestone %q: %w", key, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("%w: %s", ErrMilestoneNotFound, key)
	}
	return nil
}

// NewerThan returns the rows updated after t, by key.
func (s *TimelineStore) NewerThan(ctx context.Context, t time.Time) (map[string]MilestoneRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT milestone_key, name, status, progress_pct, eta, owner, risk, description, updated_at
		 FROM timeline_milestones WHERE updated_at > $1`, t)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]MilestoneRow{}
	for rows.Next() {
		var r MilestoneRow
		var eta sql.NullTime
		if err := rows.Scan(&r.Key, &r.Name, &r.Status, &r.ProgressPct, &eta, &r.Owner, &r.Risk, &r.Description, &r.UpdatedAt); err != nil {
			return nil, err
		}
		if eta.Valid {
			e := eta.Time
			r.ETA = &e
		}
		out[r.Key] = r
	}
	return out, rows.Err()
}
