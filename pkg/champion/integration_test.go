// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

//go:build integration
// +build integration

// Integration tests for Champion against real PostgreSQL (The Well).
// Run with: WELL_TEST_APP_DSN='host=... user=app_zhen password=... dbname=unheaded_app sslmode=require' \
//   go test ./pkg/champion/... -tags=integration -count=1
// against a Well bootstrapped from db/ (zhen_actions comes from the
// migrations, never from the test). Skipped when WELL_TEST_APP_DSN is unset;
// there is no default credential.

package champion_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"

	"unheaded/pkg/champion"
	"unheaded/pkg/champion/pgstore"
)

// connectPG opens WELL_TEST_APP_DSN and checks the zhen_actions shape.
func connectPG(t *testing.T) *sql.DB {
	dsn := os.Getenv("WELL_TEST_APP_DSN")
	if dsn == "" {
		t.Skip("WELL_TEST_APP_DSN not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("PostgreSQL not reachable: %v", err)
	}
	if err := pgstore.New(db).CheckSchema(context.Background()); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestIntegration_ReadFile_WithRealDB(t *testing.T) {
	db := connectPG(t)
	defer db.Close()

	store := pgstore.New(db)
	dir := t.TempDir()

	// Write a test file
	testFile := dir + "/test.txt"
	os.WriteFile(testFile, []byte("integration test content"), 0644)

	c, err := champion.New(champion.Config{
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

	store := pgstore.New(db)
	c, err := champion.New(champion.Config{
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

// TestIntegration_ToolCallAudit_WithRealDB confirms that a Champion
// tool call (via Dispatch, the agent path) gets the gate decision
// logged with the right status string. Specifically, an external-
// trust justification on a write_file should produce a
// "denied_untrusted_justification" entry.
func TestIntegration_ToolCallAudit_WithRealDB(t *testing.T) {
	db := connectPG(t)
	defer db.Close()

	store := pgstore.New(db)
	dir := t.TempDir()
	c, err := champion.New(champion.Config{
		ProjectRoot:  dir,
		AllowedPaths: []string{dir},
		SessionID:    "integration-test-toolcall",
	}, store)
	if err != nil {
		t.Fatal(err)
	}

	tc := champion.ToolCall{
		Name: "write_file",
		Args: map[string]any{
			"path":    dir + "/x.txt",
			"content": "hi",
		},
		Justification: []champion.Reference{{
			Topic:       "evil",
			Category:    ".",
			SourceKind:  "user-source",
			SourceTrust: "external",
			SourceLabel: "lbl",
		}},
		EmittedBy: "test",
	}
	_, err = c.Dispatch(context.Background(), tc)
	if err == nil {
		t.Fatal("expected gate refusal")
	}

	actions, err := store.GetActions(context.Background(), "integration-test-toolcall", 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range actions {
		if a.ActionType == "tool_call_attempt" && a.Status == "denied_untrusted_justification" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected tool_call_attempt + denied_untrusted_justification in actions, got %d entries", len(actions))
	}

	db.Exec("DELETE FROM zhen_actions WHERE session_id = 'integration-test-toolcall'")
}
