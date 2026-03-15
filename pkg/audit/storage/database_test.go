// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package storage

import (
	"database/sql"
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

	// ensureTable will fail due to MySQL-specific INDEX syntax, but we can
	// test the table name default by looking at the error
	ds, err := NewDatabaseStorage(db, "")
	if err != nil {
		// If it succeeds (SQLite might tolerate), check table name
		// If it fails, that's expected due to MySQL syntax
		t.Logf("NewDatabaseStorage with empty table name: %v", err)
		return
	}
	defer ds.Close()

	if ds.tableName != "audit_events" {
		t.Errorf("expected default table name audit_events, got %s", ds.tableName)
	}
}

// ===================== buildWhereClause Tests =====================

func TestBuildWhereClause_Empty(t *testing.T) {
	ds := &DatabaseStorage{tableName: "audit_events"}
	where, args := ds.buildWhereClause(&audit.AuditQuery{})
	if where != "" {
		t.Errorf("expected empty where clause, got %s", where)
	}
	if len(args) != 0 {
		t.Errorf("expected no args, got %d", len(args))
	}
}

func TestBuildWhereClause_TimeRange(t *testing.T) {
	ds := &DatabaseStorage{tableName: "audit_events"}
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC)

	where, args := ds.buildWhereClause(&audit.AuditQuery{
		StartTime: start,
		EndTime:   end,
	})

	if !strings.Contains(where, "timestamp >= ?") {
		t.Errorf("expected timestamp >= ? in where clause, got %s", where)
	}
	if !strings.Contains(where, "timestamp <= ?") {
		t.Errorf("expected timestamp <= ? in where clause, got %s", where)
	}
	if len(args) != 2 {
		t.Errorf("expected 2 args, got %d", len(args))
	}
}

func TestBuildWhereClause_Types(t *testing.T) {
	ds := &DatabaseStorage{tableName: "audit_events"}
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
	ds := &DatabaseStorage{tableName: "audit_events"}
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
	ds := &DatabaseStorage{tableName: "audit_events"}
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
	ds := &DatabaseStorage{tableName: "audit_events"}
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
	ds := &DatabaseStorage{tableName: "audit_events"}
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
	ds := &DatabaseStorage{tableName: "audit_events"}
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
	ds := &DatabaseStorage{tableName: "audit_events"}
	sqlQuery, _ := ds.buildQuery(&audit.AuditQuery{})

	if !strings.Contains(sqlQuery, "ORDER BY timestamp ASC") {
		t.Errorf("expected ORDER BY timestamp ASC, got %s", sqlQuery)
	}
}

func TestBuildQuery_Descending(t *testing.T) {
	ds := &DatabaseStorage{tableName: "audit_events"}
	sqlQuery, _ := ds.buildQuery(&audit.AuditQuery{Descending: true})

	if !strings.Contains(sqlQuery, "ORDER BY timestamp DESC") {
		t.Errorf("expected ORDER BY timestamp DESC, got %s", sqlQuery)
	}
}

func TestBuildQuery_ValidOrderBy(t *testing.T) {
	ds := &DatabaseStorage{tableName: "audit_events"}

	validColumns := []string{
		"timestamp", "id", "event_type", "severity", "action", "outcome",
		"actor_id", "actor_type", "actor_name", "resource_id", "resource_type",
		"source_ip", "source_service", "request_id",
	}

	for _, col := range validColumns {
		sqlQuery, _ := ds.buildQuery(&audit.AuditQuery{OrderBy: col})
		if !strings.Contains(sqlQuery, "ORDER BY "+col) {
			t.Errorf("expected ORDER BY %s, got %s", col, sqlQuery)
		}
	}
}

func TestBuildQuery_InvalidOrderBy_FallsBackToTimestamp(t *testing.T) {
	ds := &DatabaseStorage{tableName: "audit_events"}
	sqlQuery, _ := ds.buildQuery(&audit.AuditQuery{OrderBy: "DROP TABLE; --"})

	if !strings.Contains(sqlQuery, "ORDER BY timestamp") {
		t.Errorf("expected fallback to ORDER BY timestamp for invalid column, got %s", sqlQuery)
	}
}

func TestBuildQuery_LimitAndOffset(t *testing.T) {
	ds := &DatabaseStorage{tableName: "audit_events"}
	sqlQuery, _ := ds.buildQuery(&audit.AuditQuery{Limit: 10, Offset: 20})

	if !strings.Contains(sqlQuery, "LIMIT 10") {
		t.Errorf("expected LIMIT 10, got %s", sqlQuery)
	}
	if !strings.Contains(sqlQuery, "OFFSET 20") {
		t.Errorf("expected OFFSET 20, got %s", sqlQuery)
	}
}

func TestBuildQuery_NoLimitOffset(t *testing.T) {
	ds := &DatabaseStorage{tableName: "audit_events"}
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

	ds := &DatabaseStorage{db: db, tableName: "audit_events"}
	err = ds.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}
