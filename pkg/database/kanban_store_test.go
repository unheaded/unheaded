// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package database

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	_ "modernc.org/sqlite"
)

// CheckSchema only reads, so it works for a role that cannot alter the
// table, and it names every column the store uses: a table missing one
// (here `commits`, added after 003's first version) is refused at startup
// rather than failing on the first write.
func TestCheckSchema(t *testing.T) {
	ctx := context.Background()
	open := func(t *testing.T, ddl string) *KanbanStore {
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
		return NewKanbanStore(db)
	}
	const cols = `id TEXT PRIMARY KEY, guid TEXT, title TEXT, description TEXT, status TEXT, type TEXT,
		owner TEXT, progress INTEGER, created_at TEXT, updated_at TEXT, deleted_at TEXT, archived_at TEXT`
	if err := open(t, "CREATE TABLE kanban_tasks ("+cols+", commits TEXT)").CheckSchema(ctx); err != nil {
		t.Fatalf("complete table refused: %v", err)
	}
	if err := open(t, "CREATE TABLE kanban_tasks ("+cols+")").CheckSchema(ctx); !errors.Is(err, ErrSchema) {
		t.Errorf("table without commits: %v", err)
	}
	if err := open(t, "").CheckSchema(ctx); !errors.Is(err, ErrSchema) {
		t.Errorf("no table: %v", err)
	}
}
