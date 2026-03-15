// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite" // SQLite driver

	"unheaded/services/timeguru/internal/timeline"
)

// ============================================================================
// ERRORS
// ============================================================================

var (
	ErrEmptyPath        = errors.New("database path cannot be empty")
	ErrNilTimeline      = errors.New("timeline cannot be nil")
	ErrNotFound         = errors.New("timeline not found")
	ErrMilestoneNotFound = errors.New("milestone not found")
	ErrEmptyMilestoneID = errors.New("milestone ID cannot be empty")
	ErrInvalidProgress  = errors.New("progress must be between 0 and 100")
	ErrInvalidStatus    = errors.New("invalid status")
)

// ============================================================================
// STORE
// ============================================================================

// Store provides timeline persistence using SQLite
type Store struct {
	db *sql.DB
	mu sync.RWMutex
}

// NewStore creates a new Store with defensive validation
func NewStore(dbPath string) (*Store, error) {
	if dbPath == "" {
		return nil, ErrEmptyPath
	}

	// Ensure parent directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	// Open database
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	store := &Store{db: db}

	// Initialize schema
	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize schema: %w", err)
	}

	return store, nil
}

// initSchema creates the database schema
func (s *Store) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS timeline (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		version TEXT NOT NULL,
		last_updated TEXT NOT NULL,
		status TEXT NOT NULL,
		vision TEXT,
		data TEXT NOT NULL,
		created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS milestone_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		milestone_id TEXT NOT NULL,
		progress INTEGER NOT NULL,
		status TEXT NOT NULL,
		changed_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_milestone_history_id
		ON milestone_history(milestone_id);
	CREATE INDEX IF NOT EXISTS idx_milestone_history_changed
		ON milestone_history(changed_at DESC);
	`

	_, err := s.db.Exec(schema)
	return err
}

// SaveTimeline saves a timeline with defensive validation
func (s *Store) SaveTimeline(ctx context.Context, tl *timeline.Timeline) error {
	if tl == nil {
		return ErrNilTimeline
	}

	// Defensive: validate before saving
	if err := tl.Validate(); err != nil {
		return fmt.Errorf("validate timeline: %w", err)
	}

	// Handle nil context
	if ctx == nil {
		ctx = context.Background()
	}

	// Serialize to JSON
	data, err := json.Marshal(tl)
	if err != nil {
		return fmt.Errorf("marshal timeline: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Use REPLACE to upsert (always id=1 for singleton)
	query := `
		INSERT INTO timeline (id, version, last_updated, status, vision, data, updated_at)
		VALUES (1, ?, datetime('now'), ?, ?, ?, datetime('now'))
		ON CONFLICT(id) DO UPDATE SET
			version = excluded.version,
			last_updated = excluded.last_updated,
			status = excluded.status,
			vision = excluded.vision,
			data = excluded.data,
			updated_at = datetime('now')
	`

	_, err = s.db.ExecContext(ctx, query,
		tl.Version,
		tl.Status,
		tl.Vision,
		string(data),
	)

	if err != nil {
		return fmt.Errorf("save timeline: %w", err)
	}

	return nil
}

// GetTimeline retrieves the current timeline
func (s *Store) GetTimeline(ctx context.Context) (*timeline.Timeline, error) {
	// Handle nil context
	if ctx == nil {
		ctx = context.Background()
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `SELECT data FROM timeline WHERE id = 1`

	var dataJSON string
	err := s.db.QueryRowContext(ctx, query).Scan(&dataJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("query timeline: %w", err)
	}

	var tl timeline.Timeline
	if err := json.Unmarshal([]byte(dataJSON), &tl); err != nil {
		return nil, fmt.Errorf("unmarshal timeline: %w", err)
	}

	return &tl, nil
}

// UpdateMilestone updates a milestone's progress and status with defensive validation
func (s *Store) UpdateMilestone(ctx context.Context, milestoneID string, progress int, status string) error {
	// Defensive validation
	if milestoneID == "" {
		return ErrEmptyMilestoneID
	}

	if progress < 0 || progress > 100 {
		return fmt.Errorf("%w: %d", ErrInvalidProgress, progress)
	}

	// Validate status
	switch status {
	case "pending", "in_progress", "completed", "blocked":
		// valid
	default:
		return fmt.Errorf("%w: %q", ErrInvalidStatus, status)
	}

	// Handle nil context
	if ctx == nil {
		ctx = context.Background()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Get current timeline
	var dataJSON string
	query := `SELECT data FROM timeline WHERE id = 1`
	err := s.db.QueryRowContext(ctx, query).Scan(&dataJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return ErrNotFound
		}
		return fmt.Errorf("query timeline: %w", err)
	}

	var tl timeline.Timeline
	if err := json.Unmarshal([]byte(dataJSON), &tl); err != nil {
		return fmt.Errorf("unmarshal timeline: %w", err)
	}

	// Find and update milestone
	milestone, found := tl.GetMilestoneByID(milestoneID)
	if !found {
		return fmt.Errorf("%w: %s", ErrMilestoneNotFound, milestoneID)
	}

	milestone.Progress = progress
	milestone.Status = status

	// Validate after update
	if err := milestone.Validate(); err != nil {
		return fmt.Errorf("validate updated milestone: %w", err)
	}

	// Save updated timeline
	data, err := json.Marshal(&tl)
	if err != nil {
		return fmt.Errorf("marshal timeline: %w", err)
	}

	updateQuery := `
		UPDATE timeline
		SET data = ?, updated_at = datetime('now')
		WHERE id = 1
	`
	_, err = s.db.ExecContext(ctx, updateQuery, string(data))
	if err != nil {
		return fmt.Errorf("update timeline: %w", err)
	}

	// Record history
	historyQuery := `
		INSERT INTO milestone_history (milestone_id, progress, status)
		VALUES (?, ?, ?)
	`
	_, err = s.db.ExecContext(ctx, historyQuery, milestoneID, progress, status)
	if err != nil {
		// Non-fatal: log but don't fail
		// In production, use structured logging here
		_ = err
	}

	return nil
}

// Close closes the database connection (idempotent)
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db == nil {
		return nil
	}

	err := s.db.Close()
	s.db = nil
	return err
}
