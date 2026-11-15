// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"unheaded/pkg/database"
)

// Zhen's runs live in unheaded_zhen (ADR-149). kanban-app opens a small pool
// there only while the Zhen module is on, with the board's own login and
// every other setting of the board's config, and keeps it.
func TestRunsPool_OnlyWithTheModule(t *testing.T) {
	zhenOff(t)
	t.Setenv("WELL_ZHEN_DB", "")
	was := agentRunWarned.Swap(false)
	t.Cleanup(func() { agentRunWarned.Store(was) })

	var calls int
	var got database.Config
	fake := &sql.DB{} // never dialled
	s := &Server{store: &PgStore{}, openRuns: func(_ context.Context, c database.Config) (*sql.DB, error) {
		calls++
		got = c
		return fake, nil
	}}
	ctx := context.Background()
	if db := s.zhenRuns(ctx); db != nil || calls != 0 {
		t.Fatalf("module off: db %v, %d opens", db, calls)
	}

	dirWithZhen(t)
	board := database.AppKanbanConfig()
	if db := s.zhenRuns(ctx); db != fake || calls != 1 {
		t.Fatalf("module on: db %v, %d opens", db, calls)
	}
	if got.DBName != "unheaded_zhen" {
		t.Errorf("DBName %q, want unheaded_zhen", got.DBName)
	}
	got.DBName = board.DBName
	if got != board {
		t.Errorf("config %+v, want the board's %+v with DBName unheaded_zhen", got, board)
	}
	if db := s.zhenRuns(ctx); db != fake || calls != 1 {
		t.Fatalf("second call: db %v, %d opens (want the pool kept)", db, calls)
	}

	t.Setenv("WELL_ZHEN_DB", "zhen_elsewhere")
	s2 := &Server{store: &PgStore{}, openRuns: func(_ context.Context, c database.Config) (*sql.DB, error) {
		got = c
		return fake, nil
	}}
	if s2.zhenRuns(ctx); got.DBName != "zhen_elsewhere" {
		t.Errorf("WELL_ZHEN_DB not honoured: %q", got.DBName)
	}
}

func TestRunsPool_OpenFailureWarnsOnce(t *testing.T) {
	dirWithZhen(t)
	was := agentRunWarned.Swap(false)
	t.Cleanup(func() { agentRunWarned.Store(was) })
	calls := 0
	s := &Server{store: &PgStore{}, openRuns: func(context.Context, database.Config) (*sql.DB, error) {
		calls++
		return nil, errors.New(`database "unheaded_zhen" does not exist`)
	}}
	if db := s.zhenRuns(context.Background()); db != nil {
		t.Fatal("a failed open returned a pool")
	}
	if !agentRunWarned.Load() {
		t.Fatal("no warning")
	}
	if db := s.zhenRuns(context.Background()); db != nil || calls != 2 {
		t.Fatalf("retry: db %v, %d opens", db, calls)
	}
}

// A board that is not the Well (SQLite, memory) has no runs to read.
func TestRunsPool_NotForANonWellBoard(t *testing.T) {
	dirWithZhen(t)
	calls := 0
	s := &Server{store: &stubStore{}, openRuns: func(context.Context, database.Config) (*sql.DB, error) {
		calls++
		return &sql.DB{}, nil
	}}
	if db := s.zhenRuns(context.Background()); db != nil || calls != 0 {
		t.Fatalf("db %v, %d opens", db, calls)
	}
}
