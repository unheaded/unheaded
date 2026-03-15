// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package database

// WotanBridge listens on Wotan topics and persists events to PostgreSQL.
// This is the canonical pattern for service -> DB persistence:
// Service publishes to Wotan topic -> Bridge subscribes -> Bridge writes to DB
//
// Usage:
//
//	bridge := database.NewWotanBridge(db)
//	bridge.Subscribe("kanban.task.updated", bridge.HandleKanbanUpdate)
//	bridge.Subscribe("timeline.milestone.changed", bridge.HandleTimelineUpdate)
//	bridge.Subscribe("system.health.report", bridge.HandleHealthReport)
//	bridge.Start(ctx)

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"time"
)

// WotanBridge routes Wotan messages to database handlers.
type WotanBridge struct {
	db       *sql.DB
	handlers map[string]BridgeHandler
}

// BridgeHandler processes a single Wotan message payload.
type BridgeHandler func(ctx context.Context, payload json.RawMessage) error

// NewWotanBridge creates a bridge with the given database connection.
func NewWotanBridge(db *sql.DB) *WotanBridge {
	return &WotanBridge{
		db:       db,
		handlers: make(map[string]BridgeHandler),
	}
}

// Subscribe registers a handler for a Wotan topic.
func (b *WotanBridge) Subscribe(topic string, handler BridgeHandler) {
	b.handlers[topic] = handler
	log.Printf("[well] subscribed to topic: %s", topic)
}

// HandleKanbanUpdate persists kanban task changes from Wotan.
func (b *WotanBridge) HandleKanbanUpdate(ctx context.Context, payload json.RawMessage) error {
	var task struct {
		ID     int    `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(payload, &task); err != nil {
		return err
	}
	_, err := b.db.ExecContext(ctx,
		`UPDATE kanban_tasks SET status = $1, updated_at = NOW() WHERE id = $2`,
		task.Status, task.ID)
	return err
}

// HandleHealthReport performs smart health aggregation:
//  1. UPSERT into service_health_current (latest state)
//  2. Detect status transitions and log to service_health_transitions
//  3. Accumulate hourly statistics in service_health_hourly
func (b *WotanBridge) HandleHealthReport(ctx context.Context, payload json.RawMessage) error {
	var report struct {
		Service       string          `json:"service"`
		Host          string          `json:"host"`
		Status        string          `json:"status"`
		Port          int             `json:"port"`
		ResponseTime  int             `json:"response_time_ms"`
		UptimeSeconds int64           `json:"uptime_seconds"`
		Details       json.RawMessage `json:"details"`
	}
	if err := json.Unmarshal(payload, &report); err != nil {
		return err
	}
	if report.Host == "" {
		report.Host = "west"
	}
	detailsStr := "{}"
	if len(report.Details) > 0 {
		detailsStr = string(report.Details)
	}

	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// 1. Read current status for transition detection
	var oldStatus string
	err = tx.QueryRowContext(ctx,
		`SELECT status FROM service_health_current WHERE service_name = $1 AND host = $2`,
		report.Service, report.Host).Scan(&oldStatus)
	isNew := err == sql.ErrNoRows
	if err != nil && !isNew {
		return err
	}

	// 2. UPSERT current state
	_, err = tx.ExecContext(ctx,
		`INSERT INTO service_health_current (service_name, host, status, port, response_time_ms, uptime_seconds, details, last_check)
		 VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, NOW())
		 ON CONFLICT (service_name, host) DO UPDATE SET
		    status = EXCLUDED.status,
		    port = EXCLUDED.port,
		    response_time_ms = EXCLUDED.response_time_ms,
		    uptime_seconds = EXCLUDED.uptime_seconds,
		    details = EXCLUDED.details,
		    last_check = NOW()`,
		report.Service, report.Host, report.Status, report.Port,
		report.ResponseTime, report.UptimeSeconds, detailsStr)
	if err != nil {
		return err
	}

	// 3. Detect transitions — log when status changes
	if !isNew && oldStatus != report.Status {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO service_health_transitions (service_name, host, old_status, new_status, response_time_ms, details, transition_at)
			 VALUES ($1, $2, $3, $4, $5, $6::jsonb, NOW())`,
			report.Service, report.Host, oldStatus, report.Status,
			report.ResponseTime, detailsStr)
		if err != nil {
			return err
		}
	}

	// 4. Accumulate hourly stats
	hour := time.Now().UTC().Truncate(time.Hour)
	healthyInc, degradedInc, unhealthyInc := 0, 0, 0
	switch report.Status {
	case "healthy":
		healthyInc = 1
	case "degraded":
		degradedInc = 1
	case "unhealthy":
		unhealthyInc = 1
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO service_health_hourly
		    (service_name, host, hour, checks_total, checks_healthy, checks_degraded, checks_unhealthy,
		     avg_response_ms, max_response_ms, min_response_ms)
		 VALUES ($1, $2, $3, 1, $4, $5, $6, $7, $7, $7)
		 ON CONFLICT (service_name, host, hour) DO UPDATE SET
		    checks_total     = service_health_hourly.checks_total + 1,
		    checks_healthy   = service_health_hourly.checks_healthy + $4,
		    checks_degraded  = service_health_hourly.checks_degraded + $5,
		    checks_unhealthy = service_health_hourly.checks_unhealthy + $6,
		    avg_response_ms  = (service_health_hourly.avg_response_ms * service_health_hourly.checks_total + $7)
		                       / (service_health_hourly.checks_total + 1),
		    max_response_ms  = GREATEST(service_health_hourly.max_response_ms, $7),
		    min_response_ms  = LEAST(service_health_hourly.min_response_ms, $7)`,
		report.Service, report.Host, hour,
		healthyInc, degradedInc, unhealthyInc,
		report.ResponseTime)
	if err != nil {
		return err
	}

	err = tx.Commit()
	return err
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
		`SELECT service_name, host, old_status, new_status, response_time_ms, transition_at
		 FROM service_health_transitions
		 WHERE service_name = $1
		 ORDER BY transition_at DESC
		 LIMIT $2`,
		service, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

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

// HourlyStat represents one row from service_health_hourly.
type HourlyStat struct {
	ServiceName      string    `json:"service_name"`
	Host             string    `json:"host"`
	Hour             time.Time `json:"hour"`
	ChecksTotal      int       `json:"checks_total"`
	ChecksHealthy    int       `json:"checks_healthy"`
	ChecksDegraded   int       `json:"checks_degraded"`
	ChecksUnhealthy  int       `json:"checks_unhealthy"`
	AvgResponseMs    float64   `json:"avg_response_ms"`
	MaxResponseMs    int       `json:"max_response_ms"`
	MinResponseMs    int       `json:"min_response_ms"`
}

// QueryHealthTrends reads hourly health statistics for a service over the
// requested number of hours (capped at 168 = 1 week).
func (b *WotanBridge) QueryHealthTrends(ctx context.Context, service string, hours int) ([]HourlyStat, error) {
	if hours <= 0 || hours > 168 {
		hours = 24
	}
	since := time.Now().UTC().Add(-time.Duration(hours) * time.Hour).Truncate(time.Hour)

	rows, err := b.db.QueryContext(ctx,
		`SELECT service_name, host, hour,
		        checks_total, checks_healthy, checks_degraded, checks_unhealthy,
		        avg_response_ms, max_response_ms, min_response_ms
		 FROM service_health_hourly
		 WHERE service_name = $1 AND hour >= $2
		 ORDER BY hour ASC`,
		service, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []HourlyStat
	for rows.Next() {
		var s HourlyStat
		if err := rows.Scan(
			&s.ServiceName, &s.Host, &s.Hour,
			&s.ChecksTotal, &s.ChecksHealthy, &s.ChecksDegraded, &s.ChecksUnhealthy,
			&s.AvgResponseMs, &s.MaxResponseMs, &s.MinResponseMs,
		); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
