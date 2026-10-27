// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
)

// fakeLedger is an in-memory database/sql driver that answers the one
// query RequireMigrations makes with canned ledger rows (or an error), so
// the test needs no Postgres.
type fakeLedger struct {
	mu      sync.Mutex
	rows    []string
	err     error
	queries []string
}

var (
	fakeLedgerOnce sync.Once
	fakeLedgers    sync.Map // dsn -> *fakeLedger
)

func openFakeLedger(t *testing.T, rows []string, err error) (*sql.DB, *fakeLedger) {
	t.Helper()
	fakeLedgerOnce.Do(func() { sql.Register("fakeledger", fakeLedgerDriver{}) })
	fl := &fakeLedger{rows: rows, err: err}
	fakeLedgers.Store(t.Name(), fl)
	db, openErr := sql.Open("fakeledger", t.Name())
	if openErr != nil {
		t.Fatal(openErr)
	}
	t.Cleanup(func() { _ = db.Close(); fakeLedgers.Delete(t.Name()) })
	return db, fl
}

type fakeLedgerDriver struct{}

func (fakeLedgerDriver) Open(dsn string) (driver.Conn, error) {
	v, ok := fakeLedgers.Load(dsn)
	if !ok {
		return nil, errors.New("no fake ledger for " + dsn)
	}
	return &fakeLedgerConn{l: v.(*fakeLedger)}, nil
}

type fakeLedgerConn struct{ l *fakeLedger }

func (c *fakeLedgerConn) Prepare(q string) (driver.Stmt, error) {
	return &fakeLedgerStmt{c: c, q: q}, nil
}
func (c *fakeLedgerConn) Close() error              { return nil }
func (c *fakeLedgerConn) Begin() (driver.Tx, error) { return nil, errors.New("no transactions") }

type fakeLedgerStmt struct {
	c *fakeLedgerConn
	q string
}

func (s *fakeLedgerStmt) Close() error  { return nil }
func (s *fakeLedgerStmt) NumInput() int { return -1 }
func (s *fakeLedgerStmt) Exec([]driver.Value) (driver.Result, error) {
	return nil, errors.New("the ledger check never writes")
}
func (s *fakeLedgerStmt) Query([]driver.Value) (driver.Rows, error) {
	l := s.c.l
	l.mu.Lock()
	defer l.mu.Unlock()
	l.queries = append(l.queries, s.q)
	if l.err != nil {
		return nil, l.err
	}
	return &fakeLedgerRows{rows: append([]string(nil), l.rows...)}, nil
}

type fakeLedgerRows struct{ rows []string }

func (r *fakeLedgerRows) Columns() []string { return []string{"filename"} }
func (r *fakeLedgerRows) Close() error      { return nil }
func (r *fakeLedgerRows) Next(dest []driver.Value) error {
	if len(r.rows) == 0 {
		return io.EOF
	}
	dest[0] = r.rows[0]
	r.rows = r.rows[1:]
	return nil
}

func TestRequireMigrations(t *testing.T) {
	ctx := context.Background()
	ledger := []string{"003_app_schema.sql", "007_grants.sql", "034_app_kanban_genesis.sql"}

	t.Run("all present", func(t *testing.T) {
		db, fl := openFakeLedger(t, ledger, nil)
		if err := RequireMigrations(ctx, db, "003_app_schema.sql", "007_grants.sql"); err != nil {
			t.Fatalf("want nil, got %v", err)
		}
		if len(fl.queries) != 1 || !strings.Contains(fl.queries[0], "well_schema_migrations") {
			t.Fatalf("want one ledger query, got %q", fl.queries)
		}
	})

	t.Run("one missing names the file", func(t *testing.T) {
		db, _ := openFakeLedger(t, ledger, nil)
		err := RequireMigrations(ctx, db, "003_app_schema.sql", "038_app_ledger_read.sql")
		var missing *ErrMigrationMissing
		if !errors.As(err, &missing) {
			t.Fatalf("want *ErrMigrationMissing, got %T %v", err, err)
		}
		if len(missing.Files) != 1 || missing.Files[0] != "038_app_ledger_read.sql" {
			t.Fatalf("missing files: %q", missing.Files)
		}
		if !strings.Contains(err.Error(), "038_app_ledger_read.sql") || !strings.Contains(err.Error(), "well-migrate") {
			t.Fatalf("error must name the file and the fix: %v", err)
		}
	})

	t.Run("empty list is nil without a query", func(t *testing.T) {
		db, fl := openFakeLedger(t, nil, errors.New("must not be called"))
		if err := RequireMigrations(ctx, db); err != nil {
			t.Fatalf("want nil, got %v", err)
		}
		if len(fl.queries) != 0 {
			t.Fatalf("empty list queried the ledger: %q", fl.queries)
		}
	})

	t.Run("query error is wrapped", func(t *testing.T) {
		boom := errors.New("permission denied for table well_schema_migrations")
		db, _ := openFakeLedger(t, nil, boom)
		err := RequireMigrations(ctx, db, "003_app_schema.sql")
		if !errors.Is(err, boom) {
			t.Fatalf("want wrapped query error, got %v", err)
		}
		var missing *ErrMigrationMissing
		if errors.As(err, &missing) {
			t.Fatalf("a query error is not a missing migration: %v", err)
		}
	})

	t.Run("a bad file name is refused before querying", func(t *testing.T) {
		db, fl := openFakeLedger(t, ledger, nil)
		if err := RequireMigrations(ctx, db, "003_app_schema.sql; DROP"); err == nil {
			t.Fatal("want an error for a name the ledger cannot hold")
		}
		if len(fl.queries) != 0 {
			t.Fatalf("queried with a bad name: %q", fl.queries)
		}
	})
}
