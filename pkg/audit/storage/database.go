// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"unheaded/pkg/audit"
)

// Table is the one table this storage uses, defined by
// db/migrations/022. It is a constant so every statement is a constant.
const Table = "audit_events"

// ErrSchema reports that the table lacks the shape this storage uses.
var ErrSchema = errors.New("audit table schema missing or incomplete: run well-migrate (db/migrations/022)")

// ErrTimestampPrecision rejects an event whose timestamp has a finer
// precision than PostgreSQL keeps (microseconds): it would be stored
// rounded, and its hash, computed over the nanosecond timestamp, would no
// longer verify when read back.
var ErrTimestampPrecision = errors.New("audit event timestamp has sub-microsecond precision")

// DatabaseStorage stores audit events in The Well's unheaded_ops.audit_events
// (db/migrations/022). The table is append-only there: this storage only
// inserts and reads. It never creates or alters the table.
type DatabaseStorage struct {
	db *sql.DB
}

// DatabaseConfig holds configuration for database storage.
type DatabaseConfig struct {
	Driver    string `json:"driver"`
	DSN       string `json:"dsn"`
	TableName string `json:"table_name"`
}

// columns is the table's shape, in the order every query uses.
const columns = `id, occurred_at, event_type, severity, action, outcome,
			actor_id, actor_type, actor_name,
			resource_id, resource_type,
			source_ip, source_service, request_id,
			details, previous_hash, hash, signature`

// NewDatabaseStorage returns storage over The Well's audit table, after
// checking it has every column used (a SELECT that reads no rows).
// tableName must be "" or Table: the migrations define one table, and a
// constant name keeps every statement constant.
func NewDatabaseStorage(db *sql.DB, tableName string) (*DatabaseStorage, error) {
	if db == nil {
		return nil, fmt.Errorf("database connection is required")
	}
	if tableName != "" && tableName != Table {
		return nil, fmt.Errorf("audit storage: table %q is not %s (db/migrations/022)", tableName, Table)
	}
	ds := &DatabaseStorage{db: db}
	if err := ds.checkSchema(context.Background()); err != nil {
		return nil, err
	}
	return ds, nil
}

func (ds *DatabaseStorage) checkSchema(ctx context.Context) error {
	rows, err := ds.db.QueryContext(ctx, "SELECT "+columns+" FROM "+Table+" LIMIT 0")
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSchema, err)
	}
	return rows.Close()
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Store persists an audit event.
func (ds *DatabaseStorage) Store(ctx context.Context, event *audit.AuditEvent) error {
	return ds.insert(ctx, ds.db, event)
}

// StoreBatch persists multiple events in one transaction.
func (ds *DatabaseStorage) StoreBatch(ctx context.Context, events []*audit.AuditEvent) error {
	tx, err := ds.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, event := range events {
		if err := ds.insert(ctx, tx, event); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (ds *DatabaseStorage) insert(ctx context.Context, x execer, event *audit.AuditEvent) error {
	if !event.Timestamp.Equal(event.Timestamp.Truncate(time.Microsecond)) {
		return fmt.Errorf("%w: %s", ErrTimestampPrecision, event.ID)
	}
	details := []byte("{}")
	if event.Details != nil {
		var err error
		if details, err = json.Marshal(event.Details); err != nil {
			return fmt.Errorf("failed to marshal details: %w", err)
		}
	}
	var actorID, actorType, actorName sql.NullString
	if event.Actor != nil {
		actorID = nullString(event.Actor.ID)
		actorType = nullString(event.Actor.Type)
		actorName = nullString(event.Actor.Name)
	}
	var resourceID, resourceType sql.NullString
	if event.Resource != nil {
		resourceID = nullString(event.Resource.ID)
		resourceType = nullString(event.Resource.Type)
	}
	var sourceIP, sourceService, requestID sql.NullString
	if event.Source != nil {
		sourceIP = nullString(event.Source.IP)
		sourceService = nullString(event.Source.Service)
		requestID = nullString(event.Source.RequestID)
	}
	const query = "INSERT INTO " + Table + " (" + columns + `)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15::jsonb, $16, $17, $18)`
	_, err := x.ExecContext(ctx, query,
		event.ID, event.Timestamp, string(event.Type), string(event.Severity), event.Action, string(event.Outcome),
		actorID, actorType, actorName,
		resourceID, resourceType,
		sourceIP, sourceService, requestID,
		string(details), event.PreviousHash, event.Hash, event.Signature,
	)
	return err
}

func nullString(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }

// Get retrieves a single event by ID.
func (ds *DatabaseStorage) Get(ctx context.Context, id string) (*audit.AuditEvent, error) {
	const query = "SELECT " + selectColumns + " FROM " + Table + " WHERE id = $1"
	return scanEvent(ds.db.QueryRowContext(ctx, query, id))
}

// Query retrieves events matching the query.
func (ds *DatabaseStorage) Query(ctx context.Context, query *audit.AuditQuery) ([]*audit.AuditEvent, error) {
	sqlQuery, args := ds.buildQuery(query)
	rows, err := ds.db.QueryContext(ctx, sqlQuery, args...) // #nosec G701 -- built only from constant fragments, the orderColumns allowlist and $n placeholders; every value is in args
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var events []*audit.AuditEvent
	for rows.Next() {
		event, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

// GetLastHash returns the hash of the most recent event.
func (ds *DatabaseStorage) GetLastHash(ctx context.Context) (string, error) {
	const query = "SELECT hash FROM " + Table + " ORDER BY occurred_at DESC, id DESC LIMIT 1"
	var hash string
	err := ds.db.QueryRowContext(ctx, query).Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return hash, err
}

// Count returns the number of events matching the query.
func (ds *DatabaseStorage) Count(ctx context.Context, query *audit.AuditQuery) (int64, error) {
	where, args := ds.buildWhereClause(query)
	sqlQuery := "SELECT COUNT(*) FROM " + Table + " " + where
	var count int64
	err := ds.db.QueryRowContext(ctx, sqlQuery, args...).Scan(&count) // #nosec G701 -- WHERE from buildWhereClause: constant fragments and $n placeholders; every value is in args
	return count, err
}

// Close releases resources.
func (ds *DatabaseStorage) Close() error {
	return ds.db.Close()
}

// selectColumns is columns with details read back as text for the JSON
// decode. A literal, like columns: nothing in a query is computed.
const selectColumns = `id, occurred_at, event_type, severity, action, outcome,
			actor_id, actor_type, actor_name,
			resource_id, resource_type,
			source_ip, source_service, request_id,
			details::text, previous_hash, hash, signature`

// orderColumns maps the query API's field names to columns. Only these can
// reach ORDER BY: the field comes from callers and is never interpolated.
var orderColumns = map[string]string{
	"timestamp": "occurred_at", "id": "id", "event_type": "event_type", "severity": "severity",
	"action": "action", "outcome": "outcome", "actor_id": "actor_id", "actor_type": "actor_type",
	"actor_name": "actor_name", "resource_id": "resource_id", "resource_type": "resource_type",
	"source_ip": "source_ip", "source_service": "source_service", "request_id": "request_id",
}

// buildQuery builds the SQL query string.
func (ds *DatabaseStorage) buildQuery(query *audit.AuditQuery) (string, []any) {
	where, args := ds.buildWhereClause(query)
	orderBy, ok := orderColumns[query.OrderBy]
	if !ok {
		orderBy = "occurred_at"
	}
	order := "ASC"
	if query.Descending {
		order = "DESC"
	}
	sqlQuery := "SELECT " + selectColumns + " FROM " + Table + " " + where + " ORDER BY " + orderBy + " " + order
	if query.Limit > 0 {
		args = append(args, query.Limit)
		sqlQuery += " LIMIT $" + strconv.Itoa(len(args))
	}
	if query.Offset > 0 {
		args = append(args, query.Offset)
		sqlQuery += " OFFSET $" + strconv.Itoa(len(args))
	}
	return sqlQuery, args
}

// buildWhereClause builds the WHERE clause with numbered parameters.
func (ds *DatabaseStorage) buildWhereClause(query *audit.AuditQuery) (string, []any) {
	var conditions []string
	var args []any
	param := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}
	in := func(column string, values []string) {
		if len(values) == 0 {
			return
		}
		ps := make([]string, len(values))
		for i, v := range values {
			ps[i] = param(v)
		}
		conditions = append(conditions, fmt.Sprintf("%s IN (%s)", column, strings.Join(ps, ",")))
	}
	if !query.StartTime.IsZero() {
		conditions = append(conditions, "occurred_at >= "+param(query.StartTime))
	}
	if !query.EndTime.IsZero() {
		conditions = append(conditions, "occurred_at <= "+param(query.EndTime))
	}
	in("event_type", stringsOf(query.Types))
	in("severity", stringsOf(query.Severities))
	in("actor_id", query.ActorIDs)
	in("action", query.Actions)
	in("outcome", stringsOf(query.Outcomes))
	if len(conditions) == 0 {
		return "", nil
	}
	return "WHERE " + strings.Join(conditions, " AND "), args
}

func stringsOf[T ~string](vs []T) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = string(v)
	}
	return out
}

type scanner interface {
	Scan(dest ...any) error
}

func scanEvent(row scanner) (*audit.AuditEvent, error) {
	var event audit.AuditEvent
	var eventType, severity, outcome string
	var actorID, actorType, actorName sql.NullString
	var resourceID, resourceType sql.NullString
	var sourceIP, sourceService, requestID sql.NullString
	var details string
	err := row.Scan(
		&event.ID, &event.Timestamp, &eventType, &severity, &event.Action, &outcome,
		&actorID, &actorType, &actorName,
		&resourceID, &resourceType,
		&sourceIP, &sourceService, &requestID,
		&details, &event.PreviousHash, &event.Hash, &event.Signature,
	)
	if err != nil {
		return nil, err
	}
	event.Timestamp = event.Timestamp.UTC()
	event.Type, event.Severity, event.Outcome = audit.EventType(eventType), audit.Severity(severity), audit.Outcome(outcome)
	if actorID.Valid {
		event.Actor = &audit.Actor{ID: actorID.String, Type: actorType.String, Name: actorName.String}
	}
	if resourceID.Valid {
		event.Resource = &audit.Resource{ID: resourceID.String, Type: resourceType.String}
	}
	if sourceIP.Valid || sourceService.Valid || requestID.Valid {
		event.Source = &audit.Source{IP: sourceIP.String, Service: sourceService.String, RequestID: requestID.String}
	}
	if details != "" && details != "{}" {
		if err := json.Unmarshal([]byte(details), &event.Details); err != nil {
			return nil, fmt.Errorf("event %s: details: %w", event.ID, err)
		}
	}
	return &event, nil
}
