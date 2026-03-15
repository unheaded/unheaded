// SPDX-License-Identifier: GPL-3.0-or-later
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
