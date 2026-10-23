// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite" // Pure Go SQLite driver — no CGO
)

// ============================================================================
// TASK STORE INTERFACE — abstracts SQLite vs Postgres backends
// ============================================================================

// TaskStore is the backend-agnostic interface for kanban task persistence.
// Both the SQLite Store and the Postgres KanbanStore satisfy this interface.
type TaskStore interface {
	GetAllTasks() ([]Task, error)
	GetTask(id string) (*Task, error)
	SaveTask(task *Task) error
	DeleteTask(id string) error
	TaskCount() (int, error)
	Close() error
}

// ============================================================================
// ERRORS
// ============================================================================

var (
	ErrStoreEmptyPath   = errors.New("database path cannot be empty")
	ErrStoreNilTask     = errors.New("task cannot be nil")
	ErrStoreEmptyTaskID = errors.New("task ID cannot be empty")
	ErrStoreNotFound    = errors.New("task not found in store")
	ErrStoreClosed      = errors.New("store is closed")
)

// ============================================================================
// STORE — L1 Cache / Local Persistence (SQLite)
// ============================================================================

// Store provides task persistence using SQLite.
// Acts as the "local memory" in the L1 (SQLite) / L2 (Wotan) hybrid model.
// Mirrors the pattern from services/timeguru/internal/storage/storage.go.
type Store struct {
	db     *sql.DB
	mu     sync.RWMutex
	closed bool
}

// checkOpen returns ErrStoreClosed if the store has been closed.
func (s *Store) checkOpen() error {
	if s.db == nil || s.closed {
		return ErrStoreClosed
	}
	return nil
}

// NewStore creates a new Store with defensive validation.
func NewStore(dbPath string) (*Store, error) {
	if dbPath == "" {
		return nil, ErrStoreEmptyPath
	}

	// Ensure parent directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	// Open database
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		db.Close() // #nosec G104 -- cleanup on an error path; the function is already returning the failure that matters
		return nil, fmt.Errorf("ping database: %w", err)
	}

	// SQLite pragmas for performance + safety
	pragmas := []string{
		"PRAGMA journal_mode=WAL",   // Write-ahead log for concurrent reads
		"PRAGMA synchronous=NORMAL", // Good durability without FULL overhead
		"PRAGMA foreign_keys=ON",    // Enforce FK constraints
		"PRAGMA busy_timeout=5000",  // 5s retry on lock contention
		"PRAGMA cache_size=-8000",   // 8MB page cache
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close() // #nosec G104 -- cleanup on an error path; the function is already returning the failure that matters
			return nil, fmt.Errorf("set pragma %q: %w", p, err)
		}
	}

	store := &Store{db: db}

	// Initialize schema
	if err := store.initSchema(); err != nil {
		db.Close() // #nosec G104 -- cleanup on an error path; the function is already returning the failure that matters
		return nil, fmt.Errorf("initialize schema: %w", err)
	}

	return store, nil
}

// initSchema creates the tasks table if it doesn't exist.
func (s *Store) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS tasks (
		id          TEXT PRIMARY KEY,
		title       TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		status      TEXT NOT NULL DEFAULT 'todo',
		type        TEXT NOT NULL DEFAULT 'task',
		owner       TEXT NOT NULL DEFAULT '',
		progress    INTEGER NOT NULL DEFAULT 0 CHECK (progress >= 0 AND progress <= 100),
		created_at  TEXT NOT NULL,
		updated_at  TEXT NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
	CREATE INDEX IF NOT EXISTS idx_tasks_type ON tasks(type);
	CREATE INDEX IF NOT EXISTS idx_tasks_owner ON tasks(owner);
	`

	_, err := s.db.Exec(schema)
	return err
}

// ============================================================================
// GENESIS — SQLite dev runs only
// ============================================================================

// GenesisFileEnv names a Genesis JSON (db/seed/kanban-genesis.json) that an
// empty SQLite store is seeded from. Unset (the default everywhere: compose,
// debs, units) a new SQLite board starts empty. The Well gets its Genesis
// cards from migration 034, never from kanban-app.
const GenesisFileEnv = "KANBAN_GENESIS_FILE"

// openSQLiteStore opens the SQLite store and, when KANBAN_GENESIS_FILE is
// set, seeds it from that file if it is empty. A seed that fails closes the
// store: no half-seeded board.
func openSQLiteStore(dbPath string) (*Store, error) {
	store, err := NewStore(dbPath)
	if err != nil {
		return nil, err
	}
	if path := os.Getenv(GenesisFileEnv); path != "" {
		n, err := store.SeedFromFile(path)
		if err != nil {
			_ = store.Close()
			return nil, fmt.Errorf("%s=%s: %w", GenesisFileEnv, path, err)
		}
		log.Info().Int("count", n).Str("file", path).Msg("store: Genesis file applied (cards now in the store)")
	}
	return store, nil
}

// genesisCard is one card of the Genesis JSON.
type genesisCard struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	Type        string    `json:"type"`
	Owner       string    `json:"owner"`
	Progress    int       `json:"progress"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// maxGenesisBytes bounds the Genesis file (65 cards are ~40 KiB).
const maxGenesisBytes = 4 << 20

// readGenesisFile parses a Genesis JSON strictly: unknown fields, a
// missing id or title, a duplicate id, or a bad progress is an error.
func readGenesisFile(path string) ([]Task, error) {
	// #nosec G304 G703 -- the operator's own KANBAN_GENESIS_FILE (a dev setting), read only
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close() // #nosec G307 -- read-only file
	dec := json.NewDecoder(io.LimitReader(f, maxGenesisBytes))
	dec.DisallowUnknownFields()
	var cards []genesisCard
	if err := dec.Decode(&cards); err != nil {
		return nil, fmt.Errorf("parse genesis file: %w", err)
	}
	seen := map[string]bool{}
	tasks := make([]Task, 0, len(cards))
	for i, c := range cards {
		switch {
		case c.ID == "" || c.Title == "":
			return nil, fmt.Errorf("genesis card %d: id and title are required", i)
		case seen[c.ID]:
			return nil, fmt.Errorf("genesis card %s: duplicate id", c.ID)
		case c.Progress < 0 || c.Progress > 100:
			return nil, fmt.Errorf("genesis card %s: progress %d", c.ID, c.Progress)
		case c.CreatedAt.IsZero() || c.UpdatedAt.IsZero():
			return nil, fmt.Errorf("genesis card %s: created_at and updated_at are required", c.ID)
		}
		seen[c.ID] = true
		tasks = append(tasks, Task{ID: c.ID, Title: c.Title, Description: c.Description, Status: c.Status, Type: c.Type,
			Owner: c.Owner, Progress: c.Progress, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt})
	}
	return tasks, nil
}

// SeedFromFile inserts the cards of a Genesis JSON when the store is
// empty, in one transaction. It returns the cards inserted, or the
// existing count when the store already has tasks (nothing is merged).
func (s *Store) SeedFromFile(path string) (int, error) {
	tasks, err := readGenesisFile(path)
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkOpen(); err != nil {
		return 0, err
	}
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM tasks").Scan(&count); err != nil {
		return 0, fmt.Errorf("count existing tasks: %w", err)
	}
	if count > 0 {
		log.Info().Int("count", count).Msg("store: not empty — Genesis file not applied")
		return count, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin seed transaction: %w", err)
	}
	stmt, err := tx.Prepare(`
		INSERT INTO tasks (id, title, description, status, type, owner, progress, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		_ = tx.Rollback()
		return 0, fmt.Errorf("prepare seed statement: %w", err)
	}
	defer stmt.Close() // #nosec G307 -- statement close after commit or rollback
	for _, t := range tasks {
		if _, err := stmt.Exec(t.ID, t.Title, t.Description, t.Status, t.Type, t.Owner, t.Progress,
			t.CreatedAt.UTC().Format(time.RFC3339), t.UpdatedAt.UTC().Format(time.RFC3339)); err != nil {
			_ = tx.Rollback()
			return 0, fmt.Errorf("seed task %s: %w", t.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit seed transaction: %w", err)
	}
	return len(tasks), nil
}

// ============================================================================
// CRUD — Task Operations
// ============================================================================

// GetAllTasks returns all tasks from the store.
func (s *Store) GetAllTasks() ([]Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := s.checkOpen(); err != nil {
		return nil, err
	}

	rows, err := s.db.Query(`
		SELECT id, title, description, status, type, owner, progress, created_at, updated_at
		FROM tasks ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("query tasks: %w", err)
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		tasks = append(tasks, t)
	}

	return tasks, rows.Err()
}

// GetTask returns a single task by ID.
func (s *Store) GetTask(id string) (*Task, error) {
	if id == "" {
		return nil, ErrStoreEmptyTaskID
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := s.checkOpen(); err != nil {
		return nil, err
	}

	row := s.db.QueryRow(`
		SELECT id, title, description, status, type, owner, progress, created_at, updated_at
		FROM tasks WHERE id = ?
	`, id)

	t, err := scanTaskRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrStoreNotFound
		}
		return nil, fmt.Errorf("get task %s: %w", id, err)
	}

	return &t, nil
}

// SaveTask inserts or updates a task (upsert).
func (s *Store) SaveTask(task *Task) error {
	if task == nil {
		return ErrStoreNilTask
	}
	if task.ID == "" {
		return ErrStoreEmptyTaskID
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkOpen(); err != nil {
		return err
	}

	now := time.Now().Format(time.RFC3339)
	createdAt := task.CreatedAt.Format(time.RFC3339)
	if task.CreatedAt.IsZero() {
		createdAt = now
	}

	_, err := s.db.Exec(`
		INSERT INTO tasks (id, title, description, status, type, owner, progress, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			title = excluded.title,
			description = excluded.description,
			status = excluded.status,
			type = excluded.type,
			owner = excluded.owner,
			progress = excluded.progress,
			updated_at = excluded.updated_at
	`, task.ID, task.Title, task.Description, task.Status, task.Type,
		task.Owner, task.Progress, createdAt, now)

	if err != nil {
		return fmt.Errorf("save task %s: %w", task.ID, err)
	}
	return nil
}

// DeleteTask removes a task by ID.
func (s *Store) DeleteTask(id string) error {
	if id == "" {
		return ErrStoreEmptyTaskID
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkOpen(); err != nil {
		return err
	}

	result, err := s.db.Exec("DELETE FROM tasks WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete task %s: %w", id, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check delete result: %w", err)
	}
	if rows == 0 {
		return ErrStoreNotFound
	}

	return nil
}

// TaskCount returns the number of tasks in the store.
func (s *Store) TaskCount() (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := s.checkOpen(); err != nil {
		return 0, err
	}

	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM tasks").Scan(&count)
	return count, err
}

// Close closes the database connection and marks the store as closed.
// Subsequent calls to any Store method will return ErrStoreClosed.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed || s.db == nil {
		return nil
	}
	s.closed = true
	return s.db.Close()
}

// ============================================================================
// SCAN HELPERS
// ============================================================================

// scanTask scans a task from sql.Rows
func scanTask(rows *sql.Rows) (Task, error) {
	var t Task
	var createdAt, updatedAt string

	err := rows.Scan(
		&t.ID, &t.Title, &t.Description, &t.Status,
		&t.Type, &t.Owner, &t.Progress,
		&createdAt, &updatedAt,
	)
	if err != nil {
		return Task{}, err
	}

	t.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	t.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return t, nil
}

// scanTaskRow scans a task from sql.Row
func scanTaskRow(row *sql.Row) (Task, error) {
	var t Task
	var createdAt, updatedAt string

	err := row.Scan(
		&t.ID, &t.Title, &t.Description, &t.Status,
		&t.Type, &t.Owner, &t.Progress,
		&createdAt, &updatedAt,
	)
	if err != nil {
		return Task{}, err
	}

	t.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	t.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return t, nil
}
