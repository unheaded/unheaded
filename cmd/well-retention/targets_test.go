// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// Zhen's tables are in unheaded_zhen since ADR-149, and the board's purge
// no longer names a Zhen table: the cards a run names come in as a
// parameter, read from Zhen's database first.
func TestTargets_ZhenTablesInZhenDB(t *testing.T) {
	if dbZhen != "unheaded_zhen" {
		t.Fatalf("dbZhen = %q", dbZhen)
	}
	for _, name := range []string{"zhen_conversations", "zhen_captures", "zhen_agent_runs"} {
		if tg, ok := targets[name]; !ok || tg.db != dbZhen {
			t.Errorf("%s: db %q, want %s", name, tg.db, dbZhen)
		}
	}
	k := targets["kanban_tasks"]
	if k.db != dbApp || !k.heldCards {
		t.Fatalf("kanban_tasks: %+v", k)
	}
	for what, q := range map[string]string{"count": k.count, "held": k.held, "archive": k.archive} {
		if strings.Contains(q, "zhen_agent_runs") {
			t.Errorf("kanban_tasks %s names zhen_agent_runs: %s", what, q)
		}
		// count and held: $1 cutoff, $2 the held cards; archive: $1
		// cutoff, $2 batch, $3 the held cards (no unused placeholder).
		want := "$2::text[]"
		if what == "archive" {
			want = "$3::text[]"
		}
		if !strings.Contains(q, want) {
			t.Errorf("kanban_tasks %s does not take the held cards as %s: %s", what, want, q)
		}
	}
	// Every other statement still matches its grant's database.
	for name, tg := range targets {
		if strings.HasPrefix(name, "zhen_") && tg.db != dbZhen {
			t.Errorf("%s is a Zhen table outside %s", name, dbZhen)
		}
	}
}

// No Zhen database (the module was never installed, or not yet moved): no
// card is held by a run, and that is not an error.
func TestHeldCards_NoZhenDatabase(t *testing.T) {
	var logged []string
	logf := func(format string, args ...any) { logged = append(logged, format) }
	ids, err := heldCards(context.Background(), func(context.Context) ([]string, error) {
		return nil, errors.New(`pq: database "unheaded_zhen" does not exist`)
	}, logf)
	if err != nil || len(ids) != 0 {
		t.Fatalf("ids %v, err %v", ids, err)
	}
	if len(logged) != 1 {
		t.Fatalf("logged %d lines, want 1", len(logged))
	}
	// A missing table is the same answer.
	logged = nil
	if ids, err := heldCards(context.Background(), func(context.Context) ([]string, error) {
		return nil, errors.New(`pq: relation "zhen_agent_runs" does not exist`)
	}, logf); err != nil || len(ids) != 0 || len(logged) != 1 {
		t.Fatalf("missing table: %v %v %d", ids, err, len(logged))
	}
	// Any other failure is an error: holding nothing on a failed read
	// would let the purge take a card a run names.
	if _, err := heldCards(context.Background(), func(context.Context) ([]string, error) {
		return nil, errors.New("connection refused")
	}, logf); err == nil {
		t.Fatal("a failed read held nothing and carried on")
	}
	got, err := heldCards(context.Background(), func(context.Context) ([]string, error) {
		return []string{"a", "b"}, nil
	}, logf)
	if err != nil || strings.Join(got, ",") != "a,b" {
		t.Fatalf("got %v %v", got, err)
	}
}
