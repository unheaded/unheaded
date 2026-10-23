// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

//go:build wellint

package claudebridge

// Runs only under scripts/well/test-llm-requests.sh --go, against a
// throwaway server bootstrapped from db/migrations, as ops_writer
// (WELL_TEST_DSN). Never point it at the real Well.

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/lib/pq"

	"unheaded/pkg/audit"
	"unheaded/pkg/audit/storage"
)

func TestWellIntegration_OneTurnTwoToolCalls(t *testing.T) {
	dsn := os.Getenv("WELL_TEST_DSN")
	if dsn == "" {
		t.Skip("WELL_TEST_DSN unset (run scripts/well/test-llm-requests.sh --go)")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	events, err := storage.NewDatabaseStorage(db, "")
	if err != nil {
		t.Fatal(err)
	}
	wa, err := NewWellAuditor(WellAuditorConfig{Source: "zhen-claude", Backend: BackendClaude,
		Requests: SQLRequestSink{DB: db}, Events: events})
	if err != nil {
		t.Fatal(err)
	}

	h := newHarness(t, nil, nil, func(c *Config) { c.Auditor = wa })
	target := filepath.Join(h.root, "notes.txt")
	if err := os.WriteFile(target, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	rin, _ := json.Marshal(map[string]string{"path": target})
	win, _ := json.Marshal(map[string]string{"path": filepath.Join(h.root, "out.txt"), "content": secretMarker})
	h.api.responses = []cannedMsg{
		{StopReason: "tool_use", Blocks: []cannedBlock{
			{ToolID: "toolu_r", ToolName: "read_file", ToolInput: string(rin)},
			{ToolID: "toolu_w", ToolName: "write_file", ToolInput: string(win)},
		}},
		endTurn("done"),
	}
	conv := h.bridge.NewConversation()
	conv.SetRoute("repo-task", "claude_direct")
	if _, err := conv.Send(ctx, "read and write "+secretMarker); err != nil {
		t.Fatal(err)
	}
	if err := wa.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if wa.Errors() != 0 || wa.Dropped() != 0 {
		t.Fatalf("sink errors %d, dropped %d", wa.Errors(), wa.Dropped())
	}

	var rows int
	var family, reason, backend string
	if err := db.QueryRowContext(ctx, `SELECT count(*), min(task_family), min(route_reason), min(backend)
		FROM llm_requests WHERE conversation_id = $1`, conv.ID()).Scan(&rows, &family, &reason, &backend); err != nil {
		t.Fatal(err)
	}
	if rows != 2 || family != "repo-task" || reason != "claude_direct" || backend != "claude" {
		t.Errorf("llm_requests: %d rows (want 2: tool_use then end_turn), family %q reason %q backend %q", rows, family, reason, backend)
	}

	evs, err := events.Query(ctx, &audit.AuditQuery{})
	if err != nil {
		t.Fatal(err)
	}
	outcomes := map[audit.Outcome]int{}
	for _, e := range evs {
		if e.Source == nil || e.Source.RequestID != conv.ID() {
			continue
		}
		outcomes[e.Outcome]++
		if !e.VerifyHash() {
			t.Errorf("event %s: stored hash does not verify", e.ID)
		}
	}
	if outcomes[audit.OutcomeSuccess] != 1 || outcomes[audit.OutcomePending] != 1 {
		t.Errorf("audit_events outcomes = %v, want 1 success + 1 pending", outcomes)
	}
	// The chain: every event's previous_hash is the hash of the one before.
	rs, err := db.QueryContext(ctx, `SELECT hash, previous_hash FROM audit_events ORDER BY occurred_at, id`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rs.Close() }()
	prev := ""
	for rs.Next() {
		var hash, ph string
		if err := rs.Scan(&hash, &ph); err != nil {
			t.Fatal(err)
		}
		if ph != prev {
			t.Errorf("chain broken at %s: previous_hash %q, want %q", hash, ph, prev)
		}
		prev = hash
	}
}
