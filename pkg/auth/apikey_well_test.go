// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

//go:build wellint

package auth

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
)

// The shared store suite against a real Well (migrations 035/036), logged
// in as app_auth: scripts/well/test-api-keys.sh --with-go bootstraps a
// throwaway server and sets WELL_TEST_APP_DSN. Never the live Well: the
// table refuses DELETE, so the rows this leaves stay.
func wellTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("WELL_TEST_APP_DSN")
	if dsn == "" {
		t.Skip("WELL_TEST_APP_DSN not set (scripts/well/test-api-keys.sh --with-go)")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestWellStore_Suite(t *testing.T) {
	db := wellTestDB(t)
	apiKeyStoreSuite(t, func(t *testing.T, now func() time.Time) APIKeyStore {
		s, err := NewWellAPIKeyStore(context.Background(), db, WithAPIKeyClock(now))
		if err != nil {
			t.Fatal(err)
		}
		return s
	})
}

// last_used_at is written on use, at most once a minute.
func TestWellStore_LastUsed(t *testing.T) {
	db := wellTestDB(t)
	ctx := context.Background()
	clk := newTestClock()
	s, err := NewWellAPIKeyStore(ctx, db, WithAPIKeyClock(clk.Now))
	if err != nil {
		t.Fatal(err)
	}
	key, rec, err := s.Create(ctx, "lastused", RoleService, clk.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	clk.Advance(time.Second)
	if _, err := s.Authenticate(ctx, key); err != nil {
		t.Fatal(err)
	}
	first := clk.Now()
	clk.Advance(10 * time.Second)
	if _, err := s.Authenticate(ctx, key); err != nil {
		t.Fatal(err)
	}
	recs, err := s.Revoke(ctx, rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recs.LastUsedAt == nil || !recs.LastUsedAt.Equal(first) {
		t.Errorf("last_used_at = %v, want %v (the second use within a minute is not written)", recs.LastUsedAt, first)
	}
}

func TestWellStore_RefusesWithoutTable(t *testing.T) {
	db := wellTestDB(t)
	// One connection, so the search_path set on it applies to the probe.
	db.SetMaxOpenConns(1)
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(context.Background(), `SET search_path = pg_catalog`); err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	if _, err := NewWellAPIKeyStore(context.Background(), db); err == nil {
		t.Error("a database without api_keys opened")
	}
}
