// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Chaos journal kinds: Yaldabaoth's experiments, and the `chaos-controller
// packet-chaos` lab subcommand's own journal. Each reverts only its own.
const (
	ChaosKindExperiment = "experiment"
	ChaosKindPacket     = "packet"
)

// ChaosEntry is one fault in unheaded_ops.chaos_journal (migration 039),
// written before the fault lands. It is open until a 'reverted' row in
// chaos_journal_transitions names it.
type ChaosEntry struct {
	ID        string
	Host      string // the controller's host: west and east share one Well
	Kind      string // ChaosKindExperiment or ChaosKindPacket
	Type      string // experiment type (network_delay, packet_chaos, ...)
	Target    json.RawMessage
	Recipe    json.RawMessage // how to undo it
	CreatedAt time.Time
	ExpiresAt time.Time
}

func (e *ChaosEntry) validate() error {
	switch {
	case e.ID == "":
		return errors.New("chaos entry: id is required")
	case e.Host == "":
		return errors.New("chaos entry: host is required")
	case e.Kind != ChaosKindExperiment && e.Kind != ChaosKindPacket:
		return fmt.Errorf("chaos entry: kind %q", e.Kind)
	case e.Type == "":
		return errors.New("chaos entry: type is required")
	case !isObject(e.Target) || !isObject(e.Recipe):
		return errors.New("chaos entry: target and recipe must be JSON objects")
	case e.CreatedAt.IsZero() || e.ExpiresAt.Before(e.CreatedAt):
		return errors.New("chaos entry: created_at is required and expires_at may not precede it")
	}
	return nil
}

// ChaosStore appends to and reads the chaos revert journal as ops_writer.
type ChaosStore struct {
	db *sql.DB
}

// NewChaosStore wraps a connection to unheaded_ops.
func NewChaosStore(db *sql.DB) *ChaosStore { return &ChaosStore{db: db} }

// CheckSchema confirms migration 039's chaos tables answer for this role.
func (s *ChaosStore) CheckSchema(ctx context.Context) error {
	for _, q := range []string{
		`SELECT entry_id, host, kind, fault_type, target, recipe, created_at, expires_at FROM chaos_journal LIMIT 0`,
		`SELECT entry_id, to_status, note, occurred_at FROM chaos_journal_transitions LIMIT 0`,
	} {
		rows, err := s.db.QueryContext(ctx, q)
		if err != nil {
			return fmt.Errorf("chaos journal schema missing (migration 039_ops_deploy_chaos.sql, well-migrate): %w", err)
		}
		if err := rows.Close(); err != nil {
			return err
		}
	}
	return nil
}

// Append journals a fault. A second Append of the same id is ErrDuplicate.
func (s *ChaosStore) Append(ctx context.Context, e ChaosEntry) error {
	if err := e.validate(); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO chaos_journal (entry_id, host, kind, fault_type, target, recipe, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		e.ID, e.Host, e.Kind, e.Type, []byte(e.Target), []byte(e.Recipe), e.CreatedAt.UTC(), e.ExpiresAt.UTC()); err != nil {
		return fmt.Errorf("chaos journal %s: %w", e.ID, classify(err, e.ID))
	}
	return nil
}

// MarkReverted closes entry id with a 'reverted' row; the journal row is
// never changed. Marking an entry twice is a no-op (two revert passes may
// race); an unknown id is ErrNotFound.
func (s *ChaosStore) MarkReverted(ctx context.Context, id, why string) error {
	if len(why) > 512 {
		why = why[:512]
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO chaos_journal_transitions (entry_id, to_status, note)
		VALUES ($1, 'reverted', $2)
		ON CONFLICT (entry_id, to_status) DO NOTHING`, id, why); err != nil {
		return fmt.Errorf("chaos journal %s: %w", id, classify(err, id))
	}
	return nil
}

// Open returns host's open entries of kind, oldest first.
func (s *ChaosStore) Open(ctx context.Context, host, kind string) ([]ChaosEntry, error) {
	return s.open(ctx, host, kind, 0)
}

// OpenOlderThan returns host's open entries of kind created more than age
// ago, oldest first.
func (s *ChaosStore) OpenOlderThan(ctx context.Context, host, kind string, age time.Duration) ([]ChaosEntry, error) {
	if age <= 0 {
		return nil, errors.New("chaos journal: age must be positive")
	}
	return s.open(ctx, host, kind, age)
}

func (s *ChaosStore) open(ctx context.Context, host, kind string, age time.Duration) ([]ChaosEntry, error) {
	var cutoff any // nil: no age filter
	if age > 0 {
		cutoff = time.Now().UTC().Add(-age)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT j.entry_id, j.host, j.kind, j.fault_type, j.target, j.recipe, j.created_at, j.expires_at
		FROM chaos_journal j
		WHERE j.host = $1 AND j.kind = $2 AND ($3::timestamptz IS NULL OR j.created_at <= $3)
		  AND NOT EXISTS (SELECT 1 FROM chaos_journal_transitions t
		                  WHERE t.entry_id = j.entry_id AND t.to_status = 'reverted')
		ORDER BY j.created_at, j.id`, host, kind, cutoff)
	if err != nil {
		return nil, fmt.Errorf("chaos journal: %w", err)
	}
	defer rows.Close()
	var out []ChaosEntry
	for rows.Next() {
		var e ChaosEntry
		var target, recipe []byte
		if err := rows.Scan(&e.ID, &e.Host, &e.Kind, &e.Type, &target, &recipe, &e.CreatedAt, &e.ExpiresAt); err != nil {
			return nil, fmt.Errorf("chaos journal: %w", err)
		}
		e.Target, e.Recipe = target, recipe
		e.CreatedAt, e.ExpiresAt = e.CreatedAt.UTC(), e.ExpiresAt.UTC()
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("chaos journal: %w", err)
	}
	return out, nil
}
