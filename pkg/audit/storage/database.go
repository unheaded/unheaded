// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"unheaded/pkg/audit"
)

// validTableName matches conservative SQL-identifier rules: starts with a
// letter or underscore, then [A-Za-z0-9_] up to 64 chars. Rejects anything
// that would let a caller smuggle SQL fragments through tableName.
var validTableName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)

// DatabaseStorage implements database-backed audit storage.
type DatabaseStorage struct {
	db        *sql.DB
	tableName string
}

// DatabaseConfig holds configuration for database storage.
type DatabaseConfig struct {
	Driver    string `json:"driver"`
	DSN       string `json:"dsn"`
	TableName string `json:"table_name"`
}

// NewDatabaseStorage creates a new database storage backend.
func NewDatabaseStorage(db *sql.DB, tableName string) (*DatabaseStorage, error) {
	if db == nil {
		return nil, fmt.Errorf("database connection is required")
	}
	if tableName == "" {
		tableName = "audit_events"
	}
	if !validTableName.MatchString(tableName) {
		return nil, fmt.Errorf("audit storage: tableName %q does not match safe identifier pattern", tableName)
	}

	ds := &DatabaseStorage{
		db:        db,
		tableName: tableName,
	}

	// Ensure table exists
	if err := ds.ensureTable(); err != nil {
		return nil, fmt.Errorf("failed to ensure table: %w", err)
	}

	return ds, nil
}

// ensureTable creates the audit table if it doesn't exist.
func (ds *DatabaseStorage) ensureTable() error {
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id VARCHAR(255) PRIMARY KEY,
			timestamp TIMESTAMP NOT NULL,
			event_type VARCHAR(50) NOT NULL,
			severity VARCHAR(20) NOT NULL,
			action VARCHAR(255) NOT NULL,
			outcome VARCHAR(20) NOT NULL,
			actor_id VARCHAR(255),
			actor_type VARCHAR(50),
			actor_name VARCHAR(255),
			resource_id VARCHAR(255),
			resource_type VARCHAR(50),
			source_ip VARCHAR(45),
			source_service VARCHAR(100),
			request_id VARCHAR(255),
			details JSON,
			previous_hash VARCHAR(64),
			hash VARCHAR(64) NOT NULL,
			signature TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_timestamp (timestamp),
			INDEX idx_event_type (event_type),
			INDEX idx_actor_id (actor_id),
			INDEX idx_severity (severity)
		)
	`, ds.tableName)

	_, err := ds.db.Exec(query)
	return err
}

// Store persists an audit event.
func (ds *DatabaseStorage) Store(ctx context.Context, event *audit.AuditEvent) error {
	details, err := json.Marshal(event.Details)
	if err != nil {
		return fmt.Errorf("failed to marshal details: %w", err)
	}

	var actorID, actorType, actorName sql.NullString
	if event.Actor != nil {
		actorID = sql.NullString{String: event.Actor.ID, Valid: event.Actor.ID != ""}
		actorType = sql.NullString{String: event.Actor.Type, Valid: event.Actor.Type != ""}
		actorName = sql.NullString{String: event.Actor.Name, Valid: event.Actor.Name != ""}
	}

	var resourceID, resourceType sql.NullString
	if event.Resource != nil {
		resourceID = sql.NullString{String: event.Resource.ID, Valid: event.Resource.ID != ""}
		resourceType = sql.NullString{String: event.Resource.Type, Valid: event.Resource.Type != ""}
	}

	var sourceIP, sourceService, requestID sql.NullString
	if event.Source != nil {
		sourceIP = sql.NullString{String: event.Source.IP, Valid: event.Source.IP != ""}
		sourceService = sql.NullString{String: event.Source.Service, Valid: event.Source.Service != ""}
		requestID = sql.NullString{String: event.Source.RequestID, Valid: event.Source.RequestID != ""}
	}

	query := fmt.Sprintf(`
		INSERT INTO %s (
			id, timestamp, event_type, severity, action, outcome,
			actor_id, actor_type, actor_name,
			resource_id, resource_type,
			source_ip, source_service, request_id,
			details, previous_hash, hash, signature
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, ds.tableName)

	_, err = ds.db.ExecContext(ctx, query,
		event.ID, event.Timestamp, event.Type, event.Severity, event.Action, event.Outcome,
		actorID, actorType, actorName,
		resourceID, resourceType,
		sourceIP, sourceService, requestID,
		string(details), event.PreviousHash, event.Hash, event.Signature,
	)

	return err
}

// StoreBatch persists multiple events.
func (ds *DatabaseStorage) StoreBatch(ctx context.Context, events []*audit.AuditEvent) error {
	tx, err := ds.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, event := range events {
		if err := ds.storeInTx(ctx, tx, event); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (ds *DatabaseStorage) storeInTx(ctx context.Context, tx *sql.Tx, event *audit.AuditEvent) error {
	details, err := json.Marshal(event.Details)
	if err != nil {
		return fmt.Errorf("failed to marshal details: %w", err)
	}

	var actorID, actorType, actorName sql.NullString
	if event.Actor != nil {
		actorID = sql.NullString{String: event.Actor.ID, Valid: event.Actor.ID != ""}
		actorType = sql.NullString{String: event.Actor.Type, Valid: event.Actor.Type != ""}
		actorName = sql.NullString{String: event.Actor.Name, Valid: event.Actor.Name != ""}
	}

	var resourceID, resourceType sql.NullString
	if event.Resource != nil {
		resourceID = sql.NullString{String: event.Resource.ID, Valid: event.Resource.ID != ""}
		resourceType = sql.NullString{String: event.Resource.Type, Valid: event.Resource.Type != ""}
	}

	var sourceIP, sourceService, requestID sql.NullString
	if event.Source != nil {
		sourceIP = sql.NullString{String: event.Source.IP, Valid: event.Source.IP != ""}
		sourceService = sql.NullString{String: event.Source.Service, Valid: event.Source.Service != ""}
		requestID = sql.NullString{String: event.Source.RequestID, Valid: event.Source.RequestID != ""}
	}

	query := fmt.Sprintf(`
		INSERT INTO %s (
			id, timestamp, event_type, severity, action, outcome,
			actor_id, actor_type, actor_name,
			resource_id, resource_type,
			source_ip, source_service, request_id,
			details, previous_hash, hash, signature
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, ds.tableName)

	_, err = tx.ExecContext(ctx, query,
		event.ID, event.Timestamp, event.Type, event.Severity, event.Action, event.Outcome,
		actorID, actorType, actorName,
		resourceID, resourceType,
		sourceIP, sourceService, requestID,
		string(details), event.PreviousHash, event.Hash, event.Signature,
	)

	return err
}

// Get retrieves a single event by ID.
func (ds *DatabaseStorage) Get(ctx context.Context, id string) (*audit.AuditEvent, error) {
	query := fmt.Sprintf(`
		SELECT id, timestamp, event_type, severity, action, outcome,
			actor_id, actor_type, actor_name,
			resource_id, resource_type,
			source_ip, source_service, request_id,
			details, previous_hash, hash, signature
		FROM %s WHERE id = ?
	`, ds.tableName)

	row := ds.db.QueryRowContext(ctx, query, id)
	return ds.scanEvent(row)
}

// Query retrieves events matching the query.
func (ds *DatabaseStorage) Query(ctx context.Context, query *audit.AuditQuery) ([]*audit.AuditEvent, error) {
	sqlQuery, args := ds.buildQuery(query)
	rows, err := ds.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*audit.AuditEvent
	for rows.Next() {
		event, err := ds.scanEventRows(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}

	return events, rows.Err()
}

// GetLastHash returns the hash of the most recent event.
func (ds *DatabaseStorage) GetLastHash(ctx context.Context) (string, error) {
	query := fmt.Sprintf(`
		SELECT hash FROM %s ORDER BY timestamp DESC, id DESC LIMIT 1
	`, ds.tableName)

	var hash string
	err := ds.db.QueryRowContext(ctx, query).Scan(&hash)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return hash, err
}

// Count returns the number of events matching the query.
func (ds *DatabaseStorage) Count(ctx context.Context, query *audit.AuditQuery) (int64, error) {
	where, args := ds.buildWhereClause(query)
	sqlQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s %s", ds.tableName, where)

	var count int64
	err := ds.db.QueryRowContext(ctx, sqlQuery, args...).Scan(&count)
	return count, err
}

// Close releases resources.
func (ds *DatabaseStorage) Close() error {
	return ds.db.Close()
}

// buildQuery builds the SQL query string.
func (ds *DatabaseStorage) buildQuery(query *audit.AuditQuery) (string, []interface{}) {
	where, args := ds.buildWhereClause(query)

	// SECURITY: Whitelist OrderBy values to prevent SQL injection.
	// The OrderBy field comes from query parameters and must not be interpolated
	// directly into SQL. Only known column names are allowed.
	orderBy := "timestamp"
	if query.OrderBy != "" {
		switch query.OrderBy {
		case "timestamp", "id", "event_type", "severity", "action", "outcome",
			"actor_id", "actor_type", "actor_name", "resource_id", "resource_type",
			"source_ip", "source_service", "request_id":
			orderBy = query.OrderBy
		default:
			// Reject unknown column names to prevent SQL injection
			orderBy = "timestamp"
		}
	}
	order := "ASC"
	if query.Descending {
		order = "DESC"
	}

	sqlQuery := fmt.Sprintf(`
		SELECT id, timestamp, event_type, severity, action, outcome,
			actor_id, actor_type, actor_name,
			resource_id, resource_type,
			source_ip, source_service, request_id,
			details, previous_hash, hash, signature
		FROM %s %s ORDER BY %s %s
	`, ds.tableName, where, orderBy, order)

	if query.Limit > 0 {
		sqlQuery += fmt.Sprintf(" LIMIT %d", query.Limit)
	}
	if query.Offset > 0 {
		sqlQuery += fmt.Sprintf(" OFFSET %d", query.Offset)
	}

	return sqlQuery, args
}

// buildWhereClause builds the WHERE clause.
func (ds *DatabaseStorage) buildWhereClause(query *audit.AuditQuery) (string, []interface{}) {
	var conditions []string
	var args []interface{}

	if !query.StartTime.IsZero() {
		conditions = append(conditions, "timestamp >= ?")
		args = append(args, query.StartTime)
	}
	if !query.EndTime.IsZero() {
		conditions = append(conditions, "timestamp <= ?")
		args = append(args, query.EndTime)
	}

	if len(query.Types) > 0 {
		placeholders := make([]string, len(query.Types))
		for i, t := range query.Types {
			placeholders[i] = "?"
			args = append(args, string(t))
		}
		conditions = append(conditions, fmt.Sprintf("event_type IN (%s)", strings.Join(placeholders, ",")))
	}

	if len(query.Severities) > 0 {
		placeholders := make([]string, len(query.Severities))
		for i, s := range query.Severities {
			placeholders[i] = "?"
			args = append(args, string(s))
		}
		conditions = append(conditions, fmt.Sprintf("severity IN (%s)", strings.Join(placeholders, ",")))
	}

	if len(query.ActorIDs) > 0 {
		placeholders := make([]string, len(query.ActorIDs))
		for i, id := range query.ActorIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		conditions = append(conditions, fmt.Sprintf("actor_id IN (%s)", strings.Join(placeholders, ",")))
	}

	if len(query.Actions) > 0 {
		placeholders := make([]string, len(query.Actions))
		for i, a := range query.Actions {
			placeholders[i] = "?"
			args = append(args, a)
		}
		conditions = append(conditions, fmt.Sprintf("action IN (%s)", strings.Join(placeholders, ",")))
	}

	if len(query.Outcomes) > 0 {
		placeholders := make([]string, len(query.Outcomes))
		for i, o := range query.Outcomes {
			placeholders[i] = "?"
			args = append(args, string(o))
		}
		conditions = append(conditions, fmt.Sprintf("outcome IN (%s)", strings.Join(placeholders, ",")))
	}

	if len(conditions) == 0 {
		return "", nil
	}

	return "WHERE " + strings.Join(conditions, " AND "), args
}

// scanEvent scans a single row into an AuditEvent.
func (ds *DatabaseStorage) scanEvent(row *sql.Row) (*audit.AuditEvent, error) {
	var event audit.AuditEvent
	var actorID, actorType, actorName sql.NullString
	var resourceID, resourceType sql.NullString
	var sourceIP, sourceService, requestID sql.NullString
	var details string
	var timestamp time.Time

	err := row.Scan(
		&event.ID, &timestamp, &event.Type, &event.Severity, &event.Action, &event.Outcome,
		&actorID, &actorType, &actorName,
		&resourceID, &resourceType,
		&sourceIP, &sourceService, &requestID,
		&details, &event.PreviousHash, &event.Hash, &event.Signature,
	)
	if err != nil {
		return nil, err
	}

	event.Timestamp = timestamp

	if actorID.Valid {
		event.Actor = &audit.Actor{
			ID:   actorID.String,
			Type: actorType.String,
			Name: actorName.String,
		}
	}

	if resourceID.Valid {
		event.Resource = &audit.Resource{
			ID:   resourceID.String,
			Type: resourceType.String,
		}
	}

	if sourceIP.Valid || sourceService.Valid || requestID.Valid {
		event.Source = &audit.Source{
			IP:        sourceIP.String,
			Service:   sourceService.String,
			RequestID: requestID.String,
		}
	}

	if details != "" {
		_ = json.Unmarshal([]byte(details), &event.Details)
	}

	return &event, nil
}

// scanEventRows scans rows into an AuditEvent.
func (ds *DatabaseStorage) scanEventRows(rows *sql.Rows) (*audit.AuditEvent, error) {
	var event audit.AuditEvent
	var actorID, actorType, actorName sql.NullString
	var resourceID, resourceType sql.NullString
	var sourceIP, sourceService, requestID sql.NullString
	var details string
	var timestamp time.Time

	err := rows.Scan(
		&event.ID, &timestamp, &event.Type, &event.Severity, &event.Action, &event.Outcome,
		&actorID, &actorType, &actorName,
		&resourceID, &resourceType,
		&sourceIP, &sourceService, &requestID,
		&details, &event.PreviousHash, &event.Hash, &event.Signature,
	)
	if err != nil {
		return nil, err
	}

	event.Timestamp = timestamp

	if actorID.Valid {
		event.Actor = &audit.Actor{
			ID:   actorID.String,
			Type: actorType.String,
			Name: actorName.String,
		}
	}

	if resourceID.Valid {
		event.Resource = &audit.Resource{
			ID:   resourceID.String,
			Type: resourceType.String,
		}
	}

	if sourceIP.Valid || sourceService.Valid || requestID.Valid {
		event.Source = &audit.Source{
			IP:        sourceIP.String,
			Service:   sourceService.String,
			RequestID: requestID.String,
		}
	}

	if details != "" {
		_ = json.Unmarshal([]byte(details), &event.Details)
	}

	return &event, nil
}
