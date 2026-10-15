// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	_ "modernc.org/sqlite"
)

// CheckSchema only reads, and names every column the store writes: a
// table missing one is refused at startup, and the store never creates or
// alters it (the migrations own zhen_actions, ERDA 3.3).
func TestCheckSchema(t *testing.T) {
	ctx := context.Background()
	open := func(t *testing.T, ddl string) *Store {
		db, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		if ddl != "" {
			if _, err := db.ExecContext(ctx, ddl); err != nil {
				t.Fatal(err)
			}
		}
		return New(db)
	}
	const cols = `id INTEGER PRIMARY KEY, session_id TEXT, action_type TEXT, status TEXT, intent TEXT,
		parameters TEXT, result TEXT, result_summary TEXT, error TEXT, triggered_by TEXT,
		planned_at TEXT, completed_at TEXT`
	if err := open(t, "CREATE TABLE zhen_actions ("+cols+", elapsed_ms INTEGER)").CheckSchema(ctx); err != nil {
		t.Fatalf("complete table refused: %v", err)
	}
	if err := open(t, "CREATE TABLE zhen_actions ("+cols+")").CheckSchema(ctx); !errors.Is(err, ErrSchema) {
		t.Errorf("table without elapsed_ms: %v", err)
	}
	if err := open(t, "").CheckSchema(ctx); !errors.Is(err, ErrSchema) {
		t.Errorf("no table: %v", err)
	}
}

func TestNullableString(t *testing.T) {
	cases := []struct {
		in   string
		want any
	}{
		{"", nil},
		{"hello", "hello"},
		{" ", " "}, // whitespace is content
	}
	for _, c := range cases {
		got := nullableString(c.in)
		if got != c.want {
			t.Errorf("nullableString(%q) = %v; want %v", c.in, got, c.want)
		}
	}
}

func TestNew_NotNil(t *testing.T) {
	s := New(nil) // permissive constructor; nil DB is the caller's problem
	if s == nil {
		t.Fatal("New returned nil")
	}
}
