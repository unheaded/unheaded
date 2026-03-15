// SPDX-License-Identifier: GPL-3.0-or-later
package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// HealthCurrent represents the latest health state of a service.
type HealthCurrent struct {
	ServiceName    string          `json:"service_name"`
	Host           string          `json:"host"`
	Status         string          `json:"status"`
	Port           int             `json:"port"`
	ResponseTimeMs int             `json:"response_time_ms"`
	UptimeSeconds  int64           `json:"uptime_seconds"`
	Details        json.RawMessage `json:"details"`
	LastCheck      time.Time       `json:"last_check"`
}

// HealthStore provides PostgreSQL-backed health monitoring persistence.
// It complements WotanBridge (which writes health data) with read-only queries.
type HealthStore struct {
	db *sql.DB
}

// NewHealthStore creates a new HealthStore backed by the given database connection.
func NewHealthStore(db *sql.DB) *HealthStore {
	return &HealthStore{db: db}
}

// GetAllCurrent returns the latest health state for all services.
func (s *HealthStore) GetAllCurrent(ctx context.Context) ([]HealthCurrent, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT service_name, host, status, port, response_time_ms,
		        uptime_seconds, details, last_check
		 FROM service_health_current
		 ORDER BY service_name, host`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []HealthCurrent
	for rows.Next() {
		var h HealthCurrent
		var details []byte
		if err := rows.Scan(&h.ServiceName, &h.Host, &h.Status, &h.Port,
			&h.ResponseTimeMs, &h.UptimeSeconds, &details, &h.LastCheck); err != nil {
			return nil, err
		}
		h.Details = json.RawMessage(details)
		results = append(results, h)
	}
	return results, rows.Err()
}

// GetTransitions returns recent status transitions for a service, ordered newest first.
// Uses the HealthTransition type defined in bridge.go.
func (s *HealthStore) GetTransitions(ctx context.Context, service string, limit int) ([]HealthTransition, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT service_name, host, old_status, new_status,
		        response_time_ms, transition_at
		 FROM service_health_transitions
		 WHERE service_name = $1
		 ORDER BY transition_at DESC
		 LIMIT $2`, service, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []HealthTransition
	for rows.Next() {
		var t HealthTransition
		if err := rows.Scan(&t.ServiceName, &t.Host, &t.OldStatus, &t.NewStatus,
			&t.ResponseTimeMs, &t.TransitionAt); err != nil {
			return nil, err
		}
		results = append(results, t)
	}
	return results, rows.Err()
}

// GetHourly returns hourly health statistics for a service over the given number of hours.
// Uses the HourlyStat type defined in bridge.go.
func (s *HealthStore) GetHourly(ctx context.Context, service string, hours int) ([]HourlyStat, error) {
	if hours <= 0 {
		hours = 24
	}
	since := time.Now().UTC().Add(-time.Duration(hours) * time.Hour).Truncate(time.Hour)

	rows, err := s.db.QueryContext(ctx,
		`SELECT service_name, host, hour, checks_total, checks_healthy,
		        checks_degraded, checks_unhealthy, avg_response_ms,
		        max_response_ms, min_response_ms
		 FROM service_health_hourly
		 WHERE service_name = $1 AND hour >= $2
		 ORDER BY hour DESC`, service, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []HourlyStat
	for rows.Next() {
		var h HourlyStat
		if err := rows.Scan(&h.ServiceName, &h.Host, &h.Hour, &h.ChecksTotal,
			&h.ChecksHealthy, &h.ChecksDegraded, &h.ChecksUnhealthy,
			&h.AvgResponseMs, &h.MaxResponseMs, &h.MinResponseMs); err != nil {
			return nil, err
		}
		results = append(results, h)
	}
	return results, rows.Err()
}
