// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

//go:build wellint

package sessions

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq" // PostgreSQL driver
)

// The store contract against a real Well (migrations 055 and 056), each
// front logged in as its own role and the Admin as app_auth:
// scripts/well/test-bff-sessions.sh --with-go bootstraps a throwaway server
// and sets the three DSNs. Never the live Well: every case starts by
// deleting the two fronts' sessions.
const (
	wellFrontA = "dashboard-backend"
	wellFrontB = "kanban-app"
)

func wellTestDB(t *testing.T, env string) *sql.DB {
	t.Helper()
	dsn := os.Getenv(env)
	if dsn == "" {
		t.Skipf("%s not set (scripts/well/test-bff-sessions.sh --with-go)", env)
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestWell_Suite(t *testing.T) {
	ctx := context.Background()
	dbA := wellTestDB(t, "WELL_TEST_SESSION_DSN_A")
	dbB := wellTestDB(t, "WELL_TEST_SESSION_DSN_B")
	dbAdmin := wellTestDB(t, "WELL_TEST_SESSION_ADMIN_DSN")
	a, err := NewWellStore(ctx, dbA, wellFrontA)
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewWellStore(ctx, dbB, wellFrontB)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := NewWellAdmin(ctx, dbAdmin)
	if err != nil {
		t.Fatal(err)
	}
	storeSuite(t, func(t *testing.T) world {
		t.Helper()
		for _, db := range []*sql.DB{dbA, dbB} {
			if _, err := db.ExecContext(ctx, `DELETE FROM `+ownView); err != nil {
				t.Fatalf("empty the table: %v", err)
			}
		}
		return world{a: a, b: b, admin: admin, frontA: wellFrontA, frontB: wellFrontB}
	})
}

// A front's connection is held to its own rows by the Well, not only by
// this package: the same statements with the front named wrongly.
func TestWell_TheViewHoldsAFrontToItsOwnRows(t *testing.T) {
	ctx := context.Background()
	dbA := wellTestDB(t, "WELL_TEST_SESSION_DSN_A")
	dbB := wellTestDB(t, "WELL_TEST_SESSION_DSN_B")
	a, err := NewWellStore(ctx, dbA, wellFrontA)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dbA.ExecContext(ctx, `DELETE FROM `+ownView); err != nil {
		t.Fatal(err)
	}
	s := sess(wellFrontA, 1, t0)
	mustCreate(t, a, s)

	// B, lying about which front it is, still reads nothing of A's.
	lying := &wellStore{db: dbB, front: wellFrontA}
	if _, ok, err := lying.Get(ctx, s.KeyHash); ok || err != nil {
		t.Fatalf("front B's login read front A's session: %v %v", ok, err)
	}
	// And cannot write a session in A's name: the view's check option.
	forged := sess(wellFrontA, 2, t0)
	if err := lying.Create(ctx, forged, 100); err == nil {
		t.Fatal("front B's login created a session of front A")
	}
	if _, ok, _ := a.Get(ctx, forged.KeyHash); ok {
		t.Fatal("the forged session exists")
	}
	if err := lying.Revoke(ctx, s.KeyHash, t0); err != nil {
		t.Fatal(err)
	}
	if got, _, _ := a.Get(ctx, s.KeyHash); got.RevokedAt != nil {
		t.Fatal("front B's login revoked front A's session")
	}
	if n, _ := lying.Sweep(ctx, t0.AddDate(1, 0, 0), 100); n != 0 {
		t.Fatalf("front B's login deleted %d of front A's sessions", n)
	}
	// No front role may touch the table itself.
	if _, err := dbB.ExecContext(ctx, `SELECT 1 FROM `+sessionTable+` LIMIT 1`); err == nil {
		t.Fatal("a front role read bff_sessions directly")
	}
}

// The admin login lists and revokes; it cannot make a session.
func TestWell_AdminCannotCreate(t *testing.T) {
	ctx := context.Background()
	dbAdmin := wellTestDB(t, "WELL_TEST_SESSION_ADMIN_DSN")
	s := sess(wellFrontA, 3, t0)
	_, err := dbAdmin.ExecContext(ctx,
		`INSERT INTO `+sessionTable+` (id_hash, front, subject, roles, created_at, last_used_at, expires_at, absolute_expires_at)
		 VALUES ($1, $2, $3, 'admin', $4, $4, $5, $6)`,
		s.KeyHash, s.Front, s.Subject, s.CreatedAt, s.ExpiresAt, s.AbsoluteExpiresAt)
	if err == nil {
		t.Fatal("app_auth inserted a session")
	}
}
