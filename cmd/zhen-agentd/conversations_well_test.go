// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

//go:build wellint

package main

// Runs only under scripts/well/test-llm-requests.sh, against a throwaway
// server bootstrapped from db/migrations, as app_zhen on unheaded_app
// (WELL_TEST_APP_DSN). Never point it at the real Well.

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	_ "github.com/lib/pq"
)

func wellConversationStore(t *testing.T) (*pgMemoryStore, *sql.DB) {
	t.Helper()
	dsn := os.Getenv("WELL_TEST_APP_DSN")
	if dsn == "" {
		t.Skip("WELL_TEST_APP_DSN not set (run scripts/well/test-llm-requests.sh)")
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
	var role string
	if err := db.QueryRowContext(ctx, `SELECT current_user`).Scan(&role); err != nil {
		t.Fatal(err)
	}
	if role != "app_zhen" {
		t.Fatalf("connected as %q, want app_zhen: the daemon's role is the one under test", role)
	}
	// The daemon's own start-up path: the store, then the 045 probe.
	store, err := newPGMemoryStore(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.conversationsReady(ctx); err != nil {
		t.Fatalf("conversationsReady: %v", err)
	}
	return store, db
}

func wellSessionID(t *testing.T) string {
	t.Helper()
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return "wellint-" + hex.EncodeToString(b)
}

func sessionRows(t *testing.T, db *sql.DB, session string) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(context.Background(), `SELECT count(*) FROM zhen_conversations WHERE session_id = $1`, session).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// AppendTurns against a real PostgreSQL, as app_zhen: until now it had only
// run against the fake store.
func TestWellAppendTurns_WritesTheSession(t *testing.T) {
	store, db := wellConversationStore(t)
	ctx := context.Background()
	session := wellSessionID(t)
	turns := []conversationTurn{
		{Role: "user", Content: "what is a monad?"},
		{Role: "assistant", Content: "a 20-byte register", Sources: []string{"vor:monad", "wiki/Monad.md"}, ElapsedMs: 1234},
		{Role: "user", Content: "thanks"},
	}
	n, err := store.AppendTurns(ctx, "cli", session, "qwen-coder-14b", turns)
	if err != nil || n != len(turns) {
		t.Fatalf("AppendTurns = %d, %v; want %d, nil", n, err, len(turns))
	}

	rows, err := db.QueryContext(ctx, `SELECT role, content, sources::text, model, elapsed_ms, client
		FROM zhen_conversations WHERE session_id = $1 ORDER BY id`, session)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	i := 0
	for rows.Next() {
		var role, content, rawSources, model, client string
		var elapsed int
		if err := rows.Scan(&role, &content, &rawSources, &model, &elapsed, &client); err != nil {
			t.Fatal(err)
		}
		if i >= len(turns) {
			t.Fatalf("more rows than turns: row %d", i)
		}
		want := turns[i]
		var sources []string
		if err := json.Unmarshal([]byte(rawSources), &sources); err != nil {
			t.Fatalf("row %d: sources %q: %v", i, rawSources, err)
		}
		wantSources := want.Sources
		if wantSources == nil {
			wantSources = []string{} // stored as [], never null
		}
		if role != want.Role || content != want.Content || elapsed != want.ElapsedMs || model != "qwen-coder-14b" || client != "cli" {
			t.Errorf("row %d = (%q, %q, %d ms, model %q, client %q), want %+v with the model and client cli", i, role, content, elapsed, model, client, want)
		}
		if !reflect.DeepEqual(sources, wantSources) {
			t.Errorf("row %d: sources = %v, want %v", i, sources, wantSources)
		}
		i++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if i != len(turns) {
		t.Fatalf("read back %d rows, want %d", i, len(turns))
	}
}

// The comment on AppendTurns promises one transaction: a turn the schema
// refuses mid-batch leaves no row of the session behind.
func TestWellAppendTurns_BadTurnLeavesNothing(t *testing.T) {
	store, db := wellConversationStore(t)
	session := wellSessionID(t)
	turns := []conversationTurn{
		{Role: "user", Content: "first"},
		{Role: "assistant", Content: "second"},
		{Role: "narrator", Content: "refused by the role CHECK"}, // handleConversations' validate would stop this; the store is called direct
		{Role: "user", Content: "never reached"},
	}
	n, err := store.AppendTurns(context.Background(), "cli", session, "m", turns)
	if err == nil {
		t.Fatalf("AppendTurns took a role the schema refuses: stored %d", n)
	}
	if n != 0 {
		t.Errorf("AppendTurns = %d with an error, want 0", n)
	}
	if got := sessionRows(t, db, session); got != 0 {
		t.Fatalf("%d rows of the session were kept after a refused turn: not one transaction", got)
	}

	// An unknown client fails on the first row (045's CHECK): nothing either.
	if _, err := store.AppendTurns(context.Background(), "gui", session, "m", turns[:2]); err == nil {
		t.Fatal("AppendTurns took an unknown client")
	}
	if got := sessionRows(t, db, session); got != 0 {
		t.Fatalf("%d rows kept after an unknown client", got)
	}

	// Control: the same store still writes a good batch after the failures.
	if n, err := store.AppendTurns(context.Background(), "cli", session, "m", turns[:2]); err != nil || n != 2 {
		t.Fatalf("good batch after the failures = %d, %v; want 2, nil", n, err)
	}
	if got := sessionRows(t, db, session); got != 2 {
		t.Fatalf("%d rows after the good batch, want 2", got)
	}
}
