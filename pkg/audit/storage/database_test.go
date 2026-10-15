// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package storage

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/audit"

	_ "modernc.org/sqlite"
)

// ===================== DatabaseStorage Constructor Tests =====================

func TestNewDatabaseStorage_NilDB(t *testing.T) {
	_, err := NewDatabaseStorage(nil, "")
	if err == nil {
		t.Fatal("expected error for nil db")
	}
	if !strings.Contains(err.Error(), "database connection is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNewDatabaseStorage_DefaultTableName(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	defer db.Close()
	// No table: refused with ErrSchema, never created (the migrations own it).
	if _, err := NewDatabaseStorage(db, ""); !errors.Is(err, ErrSchema) {
		t.Fatalf("missing table: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE audit_events (id TEXT, occurred_at TEXT, event_type TEXT, severity TEXT,
		action TEXT, outcome TEXT, actor_id TEXT, actor_type TEXT, actor_name TEXT, resource_id TEXT,
		resource_type TEXT, source_ip TEXT, source_service TEXT, request_id TEXT, details TEXT,
		previous_hash TEXT, hash TEXT, signature TEXT)`); err != nil {
		t.Fatal(err)
	}
	ds, err := NewDatabaseStorage(db, "")
	if err != nil {
		t.Fatalf("complete table refused: %v", err)
	}
	if _, err := NewDatabaseStorage(db, "other_table"); err == nil {
		t.Error("a table other than audit_events accepted")
	}
	_ = ds
}

func TestStoreRefusesSubMicrosecondTimestamps(t *testing.T) {
	ds := &DatabaseStorage{}
	ev := &audit.AuditEvent{ID: "x", Timestamp: time.Date(2026, 10, 15, 1, 2, 3, 123456789, time.UTC)}
	if err := ds.Store(context.Background(), ev); !errors.Is(err, ErrTimestampPrecision) {
		t.Fatalf("got %v", err)
	}
}

// ===================== buildWhereClause Tests =====================

func TestBuildWhereClause_Empty(t *testing.T) {
	ds := &DatabaseStorage{}
	where, args := ds.buildWhereClause(&audit.AuditQuery{})
	if where != "" {
		t.Errorf("expected empty where clause, got %s", where)
	}
	if len(args) != 0 {
		t.Errorf("expected no args, got %d", len(args))
	}
}

func TestBuildWhereClause_TimeRange(t *testing.T) {
	ds := &DatabaseStorage{}
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC)

	where, args := ds.buildWhereClause(&audit.AuditQuery{
		StartTime: start,
		EndTime:   end,
	})

	if where != "WHERE occurred_at >= $1 AND occurred_at <= $2" {
		t.Errorf("where clause %q", where)
	}
	if len(args) != 2 {
		t.Errorf("expected 2 args, got %d", len(args))
	}
}

func TestBuildWhereClause_Types(t *testing.T) {
	ds := &DatabaseStorage{}
	where, args := ds.buildWhereClause(&audit.AuditQuery{
		Types: []audit.EventType{audit.EventTypeAuthentication, audit.EventTypeSystem},
	})

	if !strings.Contains(where, "event_type IN") {
		t.Errorf("expected event_type IN clause, got %s", where)
	}
	if len(args) != 2 {
		t.Errorf("expected 2 args, got %d", len(args))
	}
}

func TestBuildWhereClause_Severities(t *testing.T) {
	ds := &DatabaseStorage{}
	where, args := ds.buildWhereClause(&audit.AuditQuery{
		Severities: []audit.Severity{audit.SeverityInfo, audit.SeverityCritical},
	})

	if !strings.Contains(where, "severity IN") {
		t.Errorf("expected severity IN clause, got %s", where)
	}
	if len(args) != 2 {
		t.Errorf("expected 2 args, got %d", len(args))
	}
}

func TestBuildWhereClause_ActorIDs(t *testing.T) {
	ds := &DatabaseStorage{}
	where, args := ds.buildWhereClause(&audit.AuditQuery{
		ActorIDs: []string{"alice", "bob"},
	})

	if !strings.Contains(where, "actor_id IN") {
		t.Errorf("expected actor_id IN clause, got %s", where)
	}
	if len(args) != 2 {
		t.Errorf("expected 2 args, got %d", len(args))
	}
}

func TestBuildWhereClause_Actions(t *testing.T) {
	ds := &DatabaseStorage{}
	where, args := ds.buildWhereClause(&audit.AuditQuery{
		Actions: []string{"login", "logout"},
	})

	if !strings.Contains(where, "action IN") {
		t.Errorf("expected action IN clause, got %s", where)
	}
	if len(args) != 2 {
		t.Errorf("expected 2 args, got %d", len(args))
	}
}

func TestBuildWhereClause_Outcomes(t *testing.T) {
	ds := &DatabaseStorage{}
	where, args := ds.buildWhereClause(&audit.AuditQuery{
		Outcomes: []audit.Outcome{audit.OutcomeSuccess, audit.OutcomeFailure},
	})

	if !strings.Contains(where, "outcome IN") {
		t.Errorf("expected outcome IN clause, got %s", where)
	}
	if len(args) != 2 {
		t.Errorf("expected 2 args, got %d", len(args))
	}
}

func TestBuildWhereClause_AllFilters(t *testing.T) {
	ds := &DatabaseStorage{}
	where, args := ds.buildWhereClause(&audit.AuditQuery{
		StartTime:  time.Now().Add(-time.Hour),
		EndTime:    time.Now(),
		Types:      []audit.EventType{audit.EventTypeAuthentication},
		Severities: []audit.Severity{audit.SeverityInfo},
		ActorIDs:   []string{"alice"},
		Actions:    []string{"login"},
		Outcomes:   []audit.Outcome{audit.OutcomeSuccess},
	})

	if !strings.HasPrefix(where, "WHERE ") {
		t.Errorf("expected WHERE prefix, got %s", where)
	}
	// 7 conditions: startTime, endTime, types, severities, actorIDs, actions, outcomes
	andCount := strings.Count(where, " AND ")
	if andCount != 6 {
		t.Errorf("expected 6 AND connectors, got %d", andCount)
	}
	if len(args) != 8 { // 2 time + 1 type + 1 severity + 1 actor + 1 action + 1 outcome = 7... wait, let me count
		// startTime(1) + endTime(1) + types(1) + severities(1) + actorIDs(1) + actions(1) + outcomes(1) = 7
		if len(args) != 7 {
			t.Errorf("expected 7 args, got %d", len(args))
		}
	}
}

// ===================== buildQuery Tests =====================

func TestBuildQuery_DefaultOrder(t *testing.T) {
	ds := &DatabaseStorage{}
	sqlQuery, _ := ds.buildQuery(&audit.AuditQuery{})

	if !strings.Contains(sqlQuery, "ORDER BY occurred_at ASC") {
		t.Errorf("expected ORDER BY occurred_at ASC, got %s", sqlQuery)
	}
}

func TestBuildQuery_Descending(t *testing.T) {
	ds := &DatabaseStorage{}
	sqlQuery, _ := ds.buildQuery(&audit.AuditQuery{Descending: true})

	if !strings.Contains(sqlQuery, "ORDER BY occurred_at DESC") {
		t.Errorf("expected ORDER BY occurred_at DESC, got %s", sqlQuery)
	}
}

func TestBuildQuery_ValidOrderBy(t *testing.T) {
	ds := &DatabaseStorage{}

	validColumns := []string{
		"timestamp", "id", "event_type", "severity", "action", "outcome",
		"actor_id", "actor_type", "actor_name", "resource_id", "resource_type",
		"source_ip", "source_service", "request_id",
	}

	for _, col := range validColumns {
		want := col
		if col == "timestamp" { // the API name for occurred_at
			want = "occurred_at"
		}
		sqlQuery, _ := ds.buildQuery(&audit.AuditQuery{OrderBy: col})
		if !strings.Contains(sqlQuery, "ORDER BY "+want+" ") {
			t.Errorf("expected ORDER BY %s, got %s", want, sqlQuery)
		}
	}
}

func TestBuildQuery_InvalidOrderBy_FallsBackToTimestamp(t *testing.T) {
	ds := &DatabaseStorage{}
	sqlQuery, _ := ds.buildQuery(&audit.AuditQuery{OrderBy: "DROP TABLE; --"})

	if !strings.Contains(sqlQuery, "ORDER BY occurred_at") || strings.Contains(sqlQuery, "DROP") {
		t.Errorf("expected fallback to ORDER BY occurred_at for invalid column, got %s", sqlQuery)
	}
}

func TestBuildQuery_LimitAndOffset(t *testing.T) {
	ds := &DatabaseStorage{}
	sqlQuery, args := ds.buildQuery(&audit.AuditQuery{Limit: 10, Offset: 20, ActorIDs: []string{"a"}})

	// Parameters, numbered after the WHERE clause's.
	if !strings.HasSuffix(sqlQuery, "LIMIT $2 OFFSET $3") || len(args) != 3 || args[1] != 10 || args[2] != 20 {
		t.Errorf("limit/offset: %s %v", sqlQuery, args)
	}
}

func TestBuildQuery_NoLimitOffset(t *testing.T) {
	ds := &DatabaseStorage{}
	sqlQuery, _ := ds.buildQuery(&audit.AuditQuery{})

	if strings.Contains(sqlQuery, "LIMIT") {
		t.Errorf("expected no LIMIT clause, got %s", sqlQuery)
	}
	if strings.Contains(sqlQuery, "OFFSET") {
		t.Errorf("expected no OFFSET clause, got %s", sqlQuery)
	}
}

// ===================== DatabaseStorage Close Tests =====================

func TestDatabaseStorage_Close(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	ds := &DatabaseStorage{db: db}
	err = ds.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}
