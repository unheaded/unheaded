// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

//go:build integration
// +build integration

// Integration tests for Champion against real PostgreSQL (The Well).
// Run with: go test ./pkg/champion/... -tags=integration -count=1
// Requires: Docker PostgreSQL running on localhost:5432

package champion

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// pgActionStore implements ActionStore against real PostgreSQL.
type pgActionStore struct {
	db *sql.DB
}

func (s *pgActionStore) LogAction(ctx context.Context, action *Action) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO zhen_actions (session_id, action_type, status, intent, triggered_by, planned_at)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		action.SessionID, action.ActionType, action.Status, action.Intent, action.TriggeredBy, time.Now(),
	).Scan(&id)
	return id, err
}

func (s *pgActionStore) UpdateAction(ctx context.Context, id int64, status, result, errMsg string, elapsedMs int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE zhen_actions SET status=$1, result_summary=$2, error=$3, elapsed_ms=$4, completed_at=NOW() WHERE id=$5`,
		status, result, errMsg, elapsedMs, id,
	)
	return err
}

func (s *pgActionStore) GetActions(ctx context.Context, sessionID string, limit int) ([]Action, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, action_type, status, intent FROM zhen_actions WHERE session_id=$1 ORDER BY id DESC LIMIT $2`,
		sessionID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var actions []Action
	for rows.Next() {
		var a Action
		rows.Scan(&a.ID, &a.SessionID, &a.ActionType, &a.Status, &a.Intent)
		actions = append(actions, a)
	}
	return actions, nil
}

func connectPG(t *testing.T) *sql.DB {
	host := os.Getenv("WELL_HOST")
	if host == "" {
		host = "localhost"
	}
	password := os.Getenv("WELL_PASSWORD")
	if password == "" {
		password = "unheaded_dev"
	}
	dsn := fmt.Sprintf("host=%s port=5432 user=unheaded password=%s dbname=unheaded_app sslmode=disable", host, password)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Skipf("Cannot connect to PostgreSQL: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("PostgreSQL not reachable: %v", err)
	}
	return db
}

func TestIntegration_ReadFile_WithRealDB(t *testing.T) {
	db := connectPG(t)
	defer db.Close()

	store := &pgActionStore{db: db}
	dir := t.TempDir()

	// Write a test file
	testFile := dir + "/test.txt"
	os.WriteFile(testFile, []byte("integration test content"), 0644)

	c, err := New(Config{
		ProjectRoot:  dir,
		AllowedPaths: []string{dir},
		SessionID:    "integration-test",
	}, store)
	if err != nil {
		t.Fatal(err)
	}

	// Read file
	content, err := c.ReadFile(context.Background(), testFile)
	if err != nil {
		t.Fatal(err)
	}
	if content != "integration test content" {
		t.Errorf("unexpected content: %q", content)
	}

	// Verify action was logged to The Well
	actions, err := store.GetActions(context.Background(), "integration-test", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) == 0 {
		t.Fatal("No actions logged to The Well")
	}

	found := false
	for _, a := range actions {
		if a.ActionType == "file.read" && a.Status == "completed" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected file.read action with status=completed in The Well")
	}

	// Cleanup
	db.Exec("DELETE FROM zhen_actions WHERE session_id = 'integration-test'")
}

func TestIntegration_DeniedPath_WithRealDB(t *testing.T) {
	db := connectPG(t)
	defer db.Close()

	store := &pgActionStore{db: db}
	c, err := New(Config{
		ProjectRoot:  "/tmp/test",
		AllowedPaths: []string{"/tmp/test"},
		SessionID:    "integration-test-denied",
	}, store)
	if err != nil {
		t.Fatal(err)
	}

	// Try to read /etc/shadow — should be denied AND logged
	_, err = c.ReadFile(context.Background(), "/etc/shadow")
	if err == nil {
		t.Fatal("Expected error for /etc/shadow")
	}

	// Verify failed action was logged
	actions, err := store.GetActions(context.Background(), "integration-test-denied", 10)
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, a := range actions {
		if a.ActionType == "file.read" && a.Status == "failed" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected file.read action with status=failed in The Well")
	}

	// Cleanup
	db.Exec("DELETE FROM zhen_actions WHERE session_id = 'integration-test-denied'")
}
