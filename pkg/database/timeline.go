// SPDX-License-Identifier: GPL-3.0-or-later
package database

import (
	"context"
	"database/sql"
	"time"

	"github.com/lib/pq"
)

// TimelineMilestone represents a sprint milestone stored in The Well.
type TimelineMilestone struct {
	ID            int64      `json:"id"`
	Sprint        string     `json:"sprint"`
	Phase         string     `json:"phase"`
	Title         string     `json:"title"`
	Description   string     `json:"description"`
	Status        string     `json:"status"`
	ProgressPct   int        `json:"progress_pct"`
	TargetDate    *time.Time `json:"target_date"`
	CompletedDate *time.Time `json:"completed_date"`
	Tags          []string   `json:"tags"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// TimelineStore provides PostgreSQL-backed timeline milestone persistence.
type TimelineStore struct {
	db *sql.DB
}

// NewTimelineStore creates a new TimelineStore backed by the given database connection.
func NewTimelineStore(db *sql.DB) *TimelineStore {
	return &TimelineStore{db: db}
}

// List returns all milestones, ordered by sprint then target date.
func (s *TimelineStore) List(ctx context.Context) ([]TimelineMilestone, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, sprint, phase, title, description, status, progress_pct,
		        target_date, completed_date, tags, created_at, updated_at
		 FROM timeline_milestones
		 ORDER BY sprint, target_date NULLS LAST`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var milestones []TimelineMilestone
	for rows.Next() {
		var m TimelineMilestone
		if err := rows.Scan(&m.ID, &m.Sprint, &m.Phase, &m.Title, &m.Description,
			&m.Status, &m.ProgressPct, &m.TargetDate, &m.CompletedDate,
			pq.Array(&m.Tags), &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		milestones = append(milestones, m)
	}
	return milestones, rows.Err()
}

// GetByID returns a single milestone by ID.
func (s *TimelineStore) GetByID(ctx context.Context, id int64) (*TimelineMilestone, error) {
	var m TimelineMilestone
	err := s.db.QueryRowContext(ctx,
		`SELECT id, sprint, phase, title, description, status, progress_pct,
		        target_date, completed_date, tags, created_at, updated_at
		 FROM timeline_milestones
		 WHERE id = $1`, id).
		Scan(&m.ID, &m.Sprint, &m.Phase, &m.Title, &m.Description,
			&m.Status, &m.ProgressPct, &m.TargetDate, &m.CompletedDate,
			pq.Array(&m.Tags), &m.CreatedAt, &m.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// Create inserts a new milestone and returns its ID.
func (s *TimelineStore) Create(ctx context.Context, m TimelineMilestone) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO timeline_milestones
		    (sprint, phase, title, description, status, progress_pct, target_date, completed_date, tags)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING id`,
		m.Sprint, m.Phase, m.Title, m.Description, m.Status, m.ProgressPct,
		m.TargetDate, m.CompletedDate, pq.Array(m.Tags)).Scan(&id)
	return id, err
}

// Update modifies an existing milestone by ID.
func (s *TimelineStore) Update(ctx context.Context, id int64, m TimelineMilestone) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE timeline_milestones
		 SET sprint = $1, phase = $2, title = $3, description = $4,
		     status = $5, progress_pct = $6, target_date = $7,
		     completed_date = $8, tags = $9
		 WHERE id = $10`,
		m.Sprint, m.Phase, m.Title, m.Description, m.Status, m.ProgressPct,
		m.TargetDate, m.CompletedDate, pq.Array(m.Tags), id)
	return err
}

// UpdateStatus changes the status and progress percentage of a milestone.
func (s *TimelineStore) UpdateStatus(ctx context.Context, id int64, status string, progressPct int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE timeline_milestones
		 SET status = $1, progress_pct = $2
		 WHERE id = $3`,
		status, progressPct, id)
	return err
}
