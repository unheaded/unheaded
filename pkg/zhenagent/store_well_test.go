// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

//go:build wellint

package zhenagent

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"

	_ "github.com/lib/pq"
)

// The SQLStore against a throwaway Well as app_zhen
// (scripts/well/test-zhen-agent-runs.sh sets the DSNs; cards are created
// as app_kanban, the board's own writer).
func TestWellAgentRuns_Conformance(t *testing.T) {
	dsn, kdsn := os.Getenv("WELL_TEST_APP_DSN"), os.Getenv("WELL_TEST_KANBAN_DSN")
	if dsn == "" || kdsn == "" {
		t.Skip("WELL_TEST_APP_DSN / WELL_TEST_KANBAN_DSN not set (run scripts/well/test-zhen-agent-runs.sh)")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	kdb, err := sql.Open("postgres", kdsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = kdb.Close() }()
	add := func(c Card) {
		if _, err := kdb.ExecContext(context.Background(),
			`INSERT INTO kanban_tasks (id, title, description, status, type, owner) VALUES ($1, $2, $3, $4, $5, $6)`,
			c.ID, c.Title, c.Description, c.Status, c.Type, c.Owner); err != nil {
			t.Fatal(err)
		}
	}
	conformance(t, SQLStore{DB: db}, add, "w-")
	scheduledConformance(t, SQLStore{DB: db}, add, "w-")

	// The card moved and kept its commits as a JSON array.
	var status, commits string
	if err := kdb.QueryRow(`SELECT status, commits::text FROM kanban_tasks WHERE id = 'w-ok'`).Scan(&status, &commits); err != nil {
		t.Fatal(err)
	}
	var cs []CommitEntry
	if err := json.Unmarshal([]byte(commits), &cs); err != nil || status != "review" || len(cs) != 1 || cs[0].Branch != "zhen/w-ok" {
		t.Errorf("card = %s %s (%v)", status, commits, err)
	}
}
