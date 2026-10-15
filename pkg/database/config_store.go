// SPDX-License-Identifier: GPL-3.0-or-later
package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// ConfigEntry represents a key-value configuration entry in The Well (kingdom_config).
type ConfigEntry struct {
	Key         string          `json:"key"`
	Value       json.RawMessage `json:"value"`
	ValueType   string          `json:"value_type"`
	Description string          `json:"description"`
	Category    string          `json:"category"`
	IsSensitive bool            `json:"is_sensitive"`
	UpdatedAt   time.Time       `json:"updated_at"`
	UpdatedBy   string          `json:"updated_by"`
}

// ConfigStore provides PostgreSQL-backed kingdom configuration persistence.
type ConfigStore struct {
	db *sql.DB
}

// NewConfigStore creates a new ConfigStore backed by the given database connection.
func NewConfigStore(db *sql.DB) *ConfigStore {
	return &ConfigStore{db: db}
}

// Get returns a single configuration entry by key, or nil if not found.
func (s *ConfigStore) Get(ctx context.Context, key string) (*ConfigEntry, error) {
	var e ConfigEntry
	var value []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT key, value, value_type, description, category, is_sensitive, updated_at, updated_by
		 FROM kingdom_config
		 WHERE key = $1`, key).
		Scan(&e.Key, &value, &e.ValueType, &e.Description, &e.Category,
			&e.IsSensitive, &e.UpdatedAt, &e.UpdatedBy)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	e.Value = json.RawMessage(value)
	return &e, nil
}

// ErrNoReason is returned by Set without a reason: every change to
// kingdom_config is recorded in its history with why (db/migrations/028).
var ErrNoReason = errors.New("kingdom_config change needs a reason")

// Set creates or updates a configuration entry (UPSERT). reason goes to
// kingdom_config_history through well.change_reason, set for this
// transaction only; The Well refuses a change without one.
func (s *ConfigStore) Set(ctx context.Context, key string, value json.RawMessage, updatedBy, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return ErrNoReason
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT set_config('well.change_reason', $1, true)`, reason); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO kingdom_config (key, value, updated_by)
		 VALUES ($1, $2::jsonb, $3)
		 ON CONFLICT (key) DO UPDATE SET
		    value = EXCLUDED.value,
		    updated_by = EXCLUDED.updated_by`,
		key, string(value), updatedBy); err != nil {
		return err
	}
	return tx.Commit()
}

// List returns all configuration entries in a given category.
func (s *ConfigStore) List(ctx context.Context, category string) ([]ConfigEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT key, value, value_type, description, category, is_sensitive, updated_at, updated_by
		 FROM kingdom_config
		 WHERE category = $1
		 ORDER BY key`, category)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []ConfigEntry
	for rows.Next() {
		var e ConfigEntry
		var value []byte
		if err := rows.Scan(&e.Key, &value, &e.ValueType, &e.Description, &e.Category,
			&e.IsSensitive, &e.UpdatedAt, &e.UpdatedBy); err != nil {
			return nil, err
		}
		e.Value = json.RawMessage(value)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// ListAll returns all configuration entries across all categories.
func (s *ConfigStore) ListAll(ctx context.Context) ([]ConfigEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT key, value, value_type, description, category, is_sensitive, updated_at, updated_by
		 FROM kingdom_config
		 ORDER BY category, key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []ConfigEntry
	for rows.Next() {
		var e ConfigEntry
		var value []byte
		if err := rows.Scan(&e.Key, &value, &e.ValueType, &e.Description, &e.Category,
			&e.IsSensitive, &e.UpdatedAt, &e.UpdatedBy); err != nil {
			return nil, err
		}
		e.Value = json.RawMessage(value)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
