// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

//go:build wellint

package claudebridge

// Runs only under scripts/well/test-zhen-confirm.sh, against a throwaway
// server, as app_zhen (WELL_TEST_APP_DSN). Never the real Well.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/lib/pq"

	"unheaded/pkg/champion"
)

func TestWellConfirmQueue_RoundTrip(t *testing.T) {
	dsn := os.Getenv("WELL_TEST_APP_DSN")
	if dsn == "" {
		t.Skip("WELL_TEST_APP_DSN unset (run scripts/well/test-zhen-confirm.sh)")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	q := SQLConfirmQueue{DB: db}

	root := t.TempDir()
	champ, err := champion.New(champion.Config{ProjectRoot: root}, nil)
	if err != nil {
		t.Fatal(err)
	}
	cp, _ := champion.NewToolProvider(champ, "test")
	pub := &recPub{}
	qc := &QueueConfirmer{Champion: champ, Queue: q, Pub: pub, Source: "zhen-claude"}
	// Numbers, nesting and unicode: the hash must survive the JSONB round trip.
	args := map[string]any{"path": filepath.Join(root, "ü.txt"), "content": "line 1\nline 2 é \"quoted\" 3.50 1e3\n"}
	out := qc.Confirm(ctx, ConfirmRequest{Tool: "write_file", Args: args, EmittedBy: "claude-api/m/c", Provider: cp})
	if out.Decision != "queued" || !ValidQueueID(out.QueueID) {
		t.Fatalf("outcome = %+v", out)
	}
	rows, err := q.List(ctx, 10)
	if err != nil || len(rows) == 0 {
		t.Fatalf("list: %v %d", err, len(rows))
	}
	r, err := q.Get(ctx, out.QueueID)
	if err != nil {
		t.Fatal(err)
	}
	var stored map[string]any
	if err := json.Unmarshal(r.Call, &stored); err != nil {
		t.Fatal(err)
	}
	_, sum, err := champion.CanonicalCall(r.Tool, stored, r.ProjectRoot)
	if err != nil || sum != r.CallSHA256 {
		t.Fatalf("hash after the JSONB round trip = %s, stored %s (%v)", sum, r.CallSHA256, err)
	}
	if err := q.Decide(ctx, r.ID, "approved", "stevie@test"); err != nil {
		t.Fatal(err)
	}
	// Confirm as `zhen-claude confirm approve` does: the queue principal is the
	// only one that may confirm a call with no recorded requester (RT-03).
	if _, err := champ.ConfirmCall(ctx, champion.PendingCall{Tool: r.Tool, Args: stored, ProjectRoot: r.ProjectRoot, ExpiresAt: r.ExpiresAt}, r.CallSHA256, champion.QueuePrincipal); err != nil {
		t.Fatal(err)
	}
	if err := q.Finish(ctx, r.ID, "executed"); err != nil {
		t.Fatal(err)
	}
	if err := q.Decide(ctx, r.ID, "approved", "again"); !errors.Is(err, ErrNotPending) {
		t.Errorf("second decision: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "ü.txt")); string(got) != args["content"] {
		t.Errorf("file = %q", got)
	}
	if n, err := q.CountPending(ctx, "zhen-claude"); err != nil || n < 0 {
		t.Errorf("count: %d %v", n, err)
	}
}
