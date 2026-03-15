// SPDX-License-Identifier: GPL-3.0-or-later
package database

// WotanBridge listens on Wotan topics and persists events to PostgreSQL.
// This is the canonical pattern for service → DB persistence:
// Service publishes to Wotan topic → Bridge subscribes → Bridge writes to DB
//
// Usage:
//   bridge := database.NewWotanBridge(db, wotanClient)
//   bridge.Subscribe("kanban.task.updated", bridge.HandleKanbanUpdate)
//   bridge.Subscribe("timeline.milestone.changed", bridge.HandleTimelineUpdate)
//   bridge.Subscribe("system.health.report", bridge.HandleHealthReport)
//   bridge.Start(ctx)

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
)

type WotanBridge struct {
	db       *sql.DB
	handlers map[string]BridgeHandler
}

type BridgeHandler func(ctx context.Context, payload json.RawMessage) error

func NewWotanBridge(db *sql.DB) *WotanBridge {
	return &WotanBridge{
		db:       db,
		handlers: make(map[string]BridgeHandler),
	}
}

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

// HandleHealthReport persists service health snapshots.
func (b *WotanBridge) HandleHealthReport(ctx context.Context, payload json.RawMessage) error {
	var report struct {
		Service      string `json:"service"`
		Host         string `json:"host"`
		Status       string `json:"status"`
		Port         int    `json:"port"`
		ResponseTime int    `json:"response_time_ms"`
	}
	if err := json.Unmarshal(payload, &report); err != nil {
		return err
	}
	_, err := b.db.ExecContext(ctx,
		`INSERT INTO service_health (service_name, host, status, port, response_time_ms, last_check)
		 VALUES ($1, $2, $3, $4, $5, NOW())
		 ON CONFLICT (service_name, host) DO UPDATE SET
		    status = EXCLUDED.status,
		    response_time_ms = EXCLUDED.response_time_ms,
		    last_check = NOW()`,
		report.Service, report.Host, report.Status, report.Port, report.ResponseTime)
	return err
}
