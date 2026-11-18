// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/lib/pq"
)

// REV-3 (independent review 2026-11-15): only SQLSTATE 3D000 from the real
// connect means "the Zhen module is not installed here". Any other connect
// failure (a transient refusal, a bad password, a timeout) must fail every
// class that needs unheaded_zhen, so no card is purged unheld.
func TestOpenZhen_ClassifiesTheConnectError(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		wantHeld  bool // heldCards returns an empty hold without error
		wantInErr string
	}{
		{"missing database", fmt.Errorf("database ping: %w", &pq.Error{Code: "3D000"}), true, ""},
		{"refused", fmt.Errorf("database ping: %w", errors.New("dial tcp 127.0.0.1:5432: connect: connection refused")), false, "connection refused"},
		{"bad password", fmt.Errorf("database ping: %w", &pq.Error{Code: "28P01"}), false, "28P01"},
		{"too many connections", fmt.Errorf("database ping: %w", &pq.Error{Code: "53300"}), false, "53300"},
		{"text that only reads like 3D000", errors.New(`database ping: pq: database "unheaded_zhen" does not exist`), false, "does not exist"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &runner{}
			r.openZhen(context.Background(), func(context.Context) (*sql.DB, error) { return nil, tc.err })
			if r.zhen != nil {
				t.Fatal("a failed connect left a handle")
			}
			ids, err := heldCards(context.Background(), r.readHeldCards, r.logf)
			if tc.wantHeld {
				if err != nil || len(ids) != 0 {
					t.Fatalf("ids %v err %v, want an empty hold", ids, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("held %v and carried on after %v", ids, tc.err)
			}
			if !strings.Contains(err.Error(), tc.wantInErr) && !strings.Contains(fmt.Sprintf("%v", errors.Unwrap(err)), tc.wantInErr) {
				t.Fatalf("error %q does not carry the connect failure %q", err, tc.wantInErr)
			}
			// A class living in unheaded_zhen fails with the same cause.
			derr := r.do(context.Background(), Class{Name: "c", Table: "zhen_conversations", Action: actionPurge}, &result{})
			if derr == nil || !strings.Contains(derr.Error(), tc.wantInErr) {
				t.Fatalf("zhen class error %v, want it to carry %q", derr, tc.wantInErr)
			}
		})
	}
}

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
// card is held by a run, and that is not an error. Only the connect's
// SQLSTATE 3D000 says so (errZhenNotInstalled); REV-3 (independent review
// 2026-11-15).
func TestHeldCards_NoZhenDatabase(t *testing.T) {
	var logged []string
	logf := func(format string, args ...any) { logged = append(logged, format) }
	ids, err := heldCards(context.Background(), func(context.Context) ([]string, error) {
		return nil, fmt.Errorf("%s: %w", dbZhen, errZhenNotInstalled)
	}, logf)
	if err != nil || len(ids) != 0 {
		t.Fatalf("ids %v, err %v", ids, err)
	}
	if len(logged) != 1 {
		t.Fatalf("logged %d lines, want 1", len(logged))
	}
	// Any other failure is an error: holding nothing on a failed read
	// would let the purge take a card a run names. That includes a message
	// that only reads like a missing database, and a missing table in a
	// database that exists.
	for _, e := range []error{
		errors.New("connection refused"),
		errors.New(`pq: database "unheaded_zhen" does not exist`),
		&pq.Error{Code: "42P01", Message: `relation "zhen_agent_runs" does not exist`},
		&pq.Error{Code: "3D000"}, // from a query, not the connect
	} {
		logged = nil
		if ids, err := heldCards(context.Background(), func(context.Context) ([]string, error) {
			return nil, e
		}, logf); err == nil {
			t.Errorf("%v: held %v and carried on", e, ids)
		}
	}
	got, err := heldCards(context.Background(), func(context.Context) ([]string, error) {
		return []string{"a", "b"}, nil
	}, logf)
	if err != nil || strings.Join(got, ",") != "a,b" {
		t.Fatalf("got %v %v", got, err)
	}
}
