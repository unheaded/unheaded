// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package database

// WotanBridge persists Akira's health reports (Wotan topic
// system.health.reports, ADR-029) into The Well's unheaded_ops:
//
//	service_health_current      latest state per (service, reporter)
//	service_health_transitions  one row per status change (append-only)
//
// dashboard-backend runs it (cmd/dashboard-backend: persistHealthReports)
// as ops_writer. Time series of latency and up/down live in VictoriaMetrics;
// the hourly rollup table was dropped (db/migrations/024).

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// HealthReportTopic is the topic Akira publishes every check on.
const HealthReportTopic = "system.health.reports"

// WotanBridge writes health reports to The Well.
type WotanBridge struct {
	db *sql.DB
}

// NewWotanBridge creates a bridge with the given database connection.
func NewWotanBridge(db *sql.DB) *WotanBridge {
	return &WotanBridge{db: db}
}

// HealthReport is the payload Akira publishes (pkg/health.HealthReport, by
// JSON tag; TestHealthReportContract pins the two together).
type HealthReport struct {
	Service   string        `json:"service"`
	Reporter  string        `json:"reporter"`
	Healthy   bool          `json:"healthy"`
	Latency   time.Duration `json:"latency_ns"`
	Error     string        `json:"error,omitempty"`
	Timestamp time.Time     `json:"timestamp"`
}

// ErrBadReport is returned for a payload the bridge will not store.
var ErrBadReport = errors.New("health report rejected")

// HandleHealthReport stores one report. The host column is the reporter:
// each reporter's view of a service is its own row. A report no newer than
// the stored one changes nothing, so a replayed or out-of-order message
// (or a second dashboard instance) cannot move the state backwards or
// record a transition twice. A status change adds a transition row at the
// report's own time.
func (b *WotanBridge) HandleHealthReport(ctx context.Context, payload json.RawMessage) error {
	var r HealthReport
	if err := json.Unmarshal(payload, &r); err != nil {
		return fmt.Errorf("%w: %v", ErrBadReport, err)
	}
	if r.Service == "" || r.Reporter == "" || r.Timestamp.IsZero() || len(r.Service) > 50 || len(r.Reporter) > 50 {
		return fmt.Errorf("%w: service, reporter (each <= 50 chars) and timestamp are required", ErrBadReport)
	}
	status := "unhealthy"
	if r.Healthy {
		status = "healthy"
	}
	details := []byte("{}")
	if r.Error != "" {
		details, _ = json.Marshal(map[string]string{"error": r.Error})
	}
	ms := int(r.Latency / time.Millisecond)

	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var old string
	err = tx.QueryRowContext(ctx,
		`SELECT status FROM service_health_current WHERE service_name = $1 AND host = $2 FOR UPDATE`,
		r.Service, r.Reporter).Scan(&old)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO service_health_current (service_name, host, status, response_time_ms, details, last_check)
		 VALUES ($1, $2, $3, $4, $5::jsonb, $6)
		 ON CONFLICT (service_name, host) DO UPDATE SET
		    status = EXCLUDED.status,
		    response_time_ms = EXCLUDED.response_time_ms,
		    details = EXCLUDED.details,
		    last_check = EXCLUDED.last_check
		 WHERE service_health_current.last_check < EXCLUDED.last_check`,
		r.Service, r.Reporter, status, ms, string(details), r.Timestamp)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return tx.Commit() // stale: nothing changes
	}
	if old != "" && old != status {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO service_health_transitions
			    (service_name, host, old_status, new_status, response_time_ms, reason, details, transition_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8)`,
			r.Service, r.Reporter, old, status, ms, r.Error, string(details), r.Timestamp); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// HealthTransition represents a single status change event.
type HealthTransition struct {
	ServiceName    string    `json:"service_name"`
	Host           string    `json:"host"`
	OldStatus      string    `json:"old_status"`
	NewStatus      string    `json:"new_status"`
	ResponseTimeMs int       `json:"response_time_ms"`
	TransitionAt   time.Time `json:"transition_at"`
}

// QueryHealthHistory reads recent status transitions for a service.
func (b *WotanBridge) QueryHealthHistory(ctx context.Context, service string, limit int) ([]HealthTransition, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := b.db.QueryContext(ctx,
		`SELECT service_name, host, old_status, new_status, coalesce(response_time_ms, 0), transition_at
		 FROM service_health_transitions
		 WHERE service_name = $1
		 ORDER BY transition_at DESC
		 LIMIT $2`,
		service, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []HealthTransition
	for rows.Next() {
		var t HealthTransition
		if err := rows.Scan(&t.ServiceName, &t.Host, &t.OldStatus, &t.NewStatus, &t.ResponseTimeMs, &t.TransitionAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
