// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"unheaded/pkg/database"
)

// A Well stand-in: answers the ledger read with canned filenames and the
// kanban_tasks shape check (LIMIT 0) with no rows. Anything else fails.
type fakeWell struct {
	ledger    []string
	ledgerErr error
	closed    bool
}

var (
	fakeWellOnce sync.Once
	fakeWells    sync.Map
)

func openFakeWell(t *testing.T, w *fakeWell) *sql.DB {
	t.Helper()
	fakeWellOnce.Do(func() { sql.Register("kanbanfakewell", fakeWellDriver{}) })
	fakeWells.Store(t.Name(), w)
	db, err := sql.Open("kanbanfakewell", t.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(); fakeWells.Delete(t.Name()) })
	return db
}

type fakeWellDriver struct{}

func (fakeWellDriver) Open(dsn string) (driver.Conn, error) {
	v, ok := fakeWells.Load(dsn)
	if !ok {
		return nil, errors.New("no fake well " + dsn)
	}
	return fakeWellConn{w: v.(*fakeWell)}, nil
}

type fakeWellConn struct{ w *fakeWell }

func (c fakeWellConn) Prepare(q string) (driver.Stmt, error) { return fakeWellStmt{w: c.w, q: q}, nil }
func (c fakeWellConn) Close() error                          { return nil }
func (c fakeWellConn) Begin() (driver.Tx, error)             { return nil, errors.New("no tx") }

type fakeWellStmt struct {
	w *fakeWell
	q string
}

func (s fakeWellStmt) Close() error  { return nil }
func (s fakeWellStmt) NumInput() int { return -1 }
func (s fakeWellStmt) Exec([]driver.Value) (driver.Result, error) {
	return nil, errors.New("startup never writes")
}
func (s fakeWellStmt) Query([]driver.Value) (driver.Rows, error) {
	switch {
	case strings.Contains(s.q, "well_schema_migrations"):
		if s.w.ledgerErr != nil {
			return nil, s.w.ledgerErr
		}
		return &fakeWellRows{cols: []string{"filename"}, vals: append([]string(nil), s.w.ledger...)}, nil
	case strings.Contains(s.q, "FROM kanban_tasks LIMIT 0"):
		return &fakeWellRows{cols: []string{"id"}}, nil
	}
	return nil, errors.New("unexpected query: " + s.q)
}

type fakeWellRows struct {
	cols []string
	vals []string
}

func (r *fakeWellRows) Columns() []string { return r.cols }
func (r *fakeWellRows) Close() error      { return nil }
func (r *fakeWellRows) Next(dest []driver.Value) error {
	if len(r.vals) == 0 {
		return io.EOF
	}
	dest[0] = r.vals[0]
	r.vals = r.vals[1:]
	return nil
}

func TestWellStoreRequiresKanbanMigrations(t *testing.T) {
	t.Run("ledger has kanban's migrations: Postgres store", func(t *testing.T) {
		db := openFakeWell(t, &fakeWell{ledger: []string{"003_app_schema.sql", "007_grants.sql", "034_app_kanban_genesis.sql"}})
		st, err := wellStore(db, nil)
		if err != nil {
			t.Fatalf("want the Well store, got %v", err)
		}
		if _, ok := st.(*PgStore); !ok {
			t.Fatalf("want *PgStore, got %T", st)
		}
	})

	t.Run("ledger lacks a migration: refuse, no fallback", func(t *testing.T) {
		db := openFakeWell(t, &fakeWell{ledger: []string{"003_app_schema.sql"}})
		st, err := wellStore(db, nil)
		var missing *database.ErrMigrationMissing
		if !errors.As(err, &missing) || st != nil {
			t.Fatalf("want ErrMigrationMissing and no store, got %T %v", st, err)
		}
		if len(missing.Files) != 1 || missing.Files[0] != "007_grants.sql" {
			t.Fatalf("missing: %q", missing.Files)
		}
	})

	t.Run("ledger unreadable: refuse", func(t *testing.T) {
		db := openFakeWell(t, &fakeWell{ledgerErr: errors.New("permission denied for table well_schema_migrations")})
		if st, err := wellStore(db, nil); err == nil || st != nil {
			t.Fatalf("want an error, got %T %v", st, err)
		}
	})

	t.Run("Well not configured or unreachable: no store, no error (SQLite fallback)", func(t *testing.T) {
		st, err := wellStore(nil, database.ErrMissingCredentials)
		if err != nil || st != nil {
			t.Fatalf("want (nil, nil), got %T %v", st, err)
		}
	})
}
