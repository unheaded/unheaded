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

	"github.com/lib/pq"
)

// ErrNotFound means no row has that id.
var ErrNotFound = errors.New("not found")

// ErrDuplicate means a row with that id already exists. The deploy and chaos
// tables are append-only, so a second write of the same id is refused,
// never merged.
var ErrDuplicate = errors.New("already recorded")

// classify maps the PostgreSQL errors the append-only stores expect to
// their typed errors.
func classify(err error, id string) error {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		switch pqErr.Code {
		case "23505": // unique_violation
			return fmt.Errorf("%w: %s", ErrDuplicate, id)
		case "23503": // foreign_key_violation: a transition for an unknown id
			return fmt.Errorf("%w: %s", ErrNotFound, id)
		}
	}
	return err
}

func isObject(b json.RawMessage) bool {
	var m map[string]json.RawMessage
	return len(b) > 0 && json.Unmarshal(b, &m) == nil && m != nil
}

// DeployRecord is one deployment in unheaded_ops (migration 039):
// deploy_history holds what was asked for and never changes; every change
// appends a deploy_history_transitions row, and the newest one gives
// Status and State.
type DeployRecord struct {
	ID        string
	Service   string
	Version   string
	Strategy  string
	Replicas  int
	Spec      json.RawMessage // the request: strategy, health, proxy, labels
	Host      string          // where the writer runs
	CreatedAt time.Time
	Status    string          // newest transition's to_status
	State     json.RawMessage // newest transition's full record
	UpdatedAt time.Time       // newest transition's time
}

func (r *DeployRecord) validate() error {
	switch {
	case r.ID == "" || r.Service == "" || r.Version == "" || r.Strategy == "":
		return errors.New("deploy record: id, service, version and strategy are required")
	case r.Host == "":
		return errors.New("deploy record: host is required")
	case !isObject(r.Spec):
		return errors.New("deploy record: spec must be a JSON object")
	case !isObject(r.State):
		return errors.New("deploy record: state must be a JSON object")
	case r.Status == "":
		return errors.New("deploy record: status is required")
	case r.CreatedAt.IsZero():
		return errors.New("deploy record: created_at is required")
	}
	return nil
}

// DeployStore reads and appends deploy history as ops_writer.
type DeployStore struct {
	db *sql.DB
}

// NewDeployStore wraps a connection to unheaded_ops. The tables come from
// migration 039 (well-migrate), never from the service.
func NewDeployStore(db *sql.DB) *DeployStore { return &DeployStore{db: db} }

// CheckSchema confirms migration 039's deploy tables answer for this role.
// It reads nothing (LIMIT 0), so it needs only SELECT.
func (s *DeployStore) CheckSchema(ctx context.Context) error {
	for _, q := range []string{
		`SELECT deployment_id, service, version, strategy, replicas, spec, host, created_at FROM deploy_history LIMIT 0`,
		`SELECT deployment_id, from_status, to_status, note, state, occurred_at FROM deploy_history_transitions LIMIT 0`,
	} {
		rows, err := s.db.QueryContext(ctx, q)
		if err != nil {
			return fmt.Errorf("deploy history schema missing (migration 039_ops_deploy_chaos.sql, well-migrate): %w", err)
		}
		if err := rows.Close(); err != nil {
			return err
		}
	}
	return nil
}

// Ping reports whether the Well answers.
func (s *DeployStore) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

// Record writes a new deployment and its first transition (” → Status)
// in one transaction. A second Record of the same id is ErrDuplicate.
func (s *DeployStore) Record(ctx context.Context, r DeployRecord) error {
	if err := r.validate(); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("deploy record: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO deploy_history (deployment_id, service, version, strategy, replicas, spec, host, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		r.ID, r.Service, r.Version, r.Strategy, r.Replicas, []byte(r.Spec), r.Host, r.CreatedAt.UTC()); err != nil {
		return fmt.Errorf("deploy record %s: %w", r.ID, classify(err, r.ID))
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO deploy_history_transitions (deployment_id, from_status, to_status, note, state)
		VALUES ($1, '', $2, '', $3)`, r.ID, r.Status, []byte(r.State)); err != nil {
		return fmt.Errorf("deploy record %s: %w", r.ID, classify(err, r.ID))
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("deploy record %s: %w", r.ID, err)
	}
	return nil
}

// Transition appends a change to deployment id: the status it had, the
// status it has now (the same for a change that is not a status change),
// a short note, and the whole record after the change.
func (s *DeployStore) Transition(ctx context.Context, id, from, to, note string, state json.RawMessage) error {
	if !isObject(state) {
		return errors.New("deploy transition: state must be a JSON object")
	}
	if len(note) > 512 {
		note = note[:512]
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO deploy_history_transitions (deployment_id, from_status, to_status, note, state)
		VALUES ($1, $2, $3, $4, $5)`, id, from, to, note, []byte(state)); err != nil {
		return fmt.Errorf("deploy transition %s: %w", id, classify(err, id))
	}
	return nil
}

const deploySelect = `
	SELECT h.deployment_id, h.service, h.version, h.strategy, h.replicas, h.spec, h.host, h.created_at,
	       t.to_status, t.state, t.occurred_at
	FROM deploy_history h
	JOIN LATERAL (
		SELECT to_status, state, occurred_at FROM deploy_history_transitions x
		WHERE x.deployment_id = h.deployment_id ORDER BY x.id DESC LIMIT 1
	) t ON true`

func scanDeploy(sc interface{ Scan(...any) error }) (*DeployRecord, error) {
	var r DeployRecord
	var spec, state []byte
	if err := sc.Scan(&r.ID, &r.Service, &r.Version, &r.Strategy, &r.Replicas, &spec, &r.Host, &r.CreatedAt,
		&r.Status, &state, &r.UpdatedAt); err != nil {
		return nil, err
	}
	r.Spec, r.State = spec, state
	r.CreatedAt, r.UpdatedAt = r.CreatedAt.UTC(), r.UpdatedAt.UTC()
	return &r, nil
}

// Get returns deployment id as it stands now, or ErrNotFound.
func (s *DeployStore) Get(ctx context.Context, id string) (*DeployRecord, error) {
	r, err := scanDeploy(s.db.QueryRowContext(ctx, deploySelect+` WHERE h.deployment_id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("deployment %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("deployment %s: %w", id, err)
	}
	return r, nil
}

// List returns up to limit deployments recorded on host of service, newest
// first; "" for host or service matches all. Sword reads its own host's:
// a Sword deploys through that host's Cuirass, so only its records are
// rollback targets.
func (s *DeployStore) List(ctx context.Context, host, service string, limit int) ([]*DeployRecord, error) {
	if limit <= 0 {
		return nil, errors.New("deploy list: limit must be positive")
	}
	rows, err := s.db.QueryContext(ctx, deploySelect+`
		WHERE ($1 = '' OR h.host = $1) AND ($2 = '' OR h.service = $2)
		ORDER BY h.created_at DESC, h.id DESC
		LIMIT $3`, host, service, limit)
	if err != nil {
		return nil, fmt.Errorf("deploy list: %w", err)
	}
	defer rows.Close()
	var out []*DeployRecord
	for rows.Next() {
		r, err := scanDeploy(rows)
		if err != nil {
			return nil, fmt.Errorf("deploy list: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("deploy list: %w", err)
	}
	return out, nil
}
