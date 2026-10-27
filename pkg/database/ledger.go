// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package database

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
)

// Querier is what RequireMigrations needs from a connection: *sql.DB,
// *sql.Conn and *sql.Tx all satisfy it.
type Querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// ErrMigrationMissing reports that the database's well_schema_migrations
// ledger (ADR-101) has no row for one or more migration files a service
// depends on: the schema it would use was never applied by well-migrate or
// db/init.sh. A service refuses to start on it instead of creating tables
// itself or falling back to another store.
type ErrMigrationMissing struct {
	Files []string
}

func (e *ErrMigrationMissing) Error() string {
	return fmt.Sprintf("migration(s) %s not in well_schema_migrations: run well-migrate (ADR-101)",
		strings.Join(e.Files, ", "))
}

// ledgerFile is the ledger's own CHECK on filename (db/migrations/LEDGER.sql).
var ledgerFile = regexp.MustCompile(`^[0-9]{3}_[a-z0-9_]+\.sql$`)

// RequireMigrations returns nil when every named file has a row in the
// connected database's well_schema_migrations ledger, *ErrMigrationMissing
// naming the absent ones otherwise, and a wrapped error when the ledger
// cannot be read (a role without SELECT on it gets permission denied:
// db/migrations/038 grants it to app_kanban). An empty list is nil without
// a query. The ledger is tens of rows, so it is read whole and diffed here.
func RequireMigrations(ctx context.Context, q Querier, files ...string) error {
	if len(files) == 0 {
		return nil
	}
	for _, f := range files {
		if !ledgerFile.MatchString(f) {
			return fmt.Errorf("require migrations: %q is not a migration file name", f)
		}
	}
	rows, err := q.QueryContext(ctx, `SELECT filename FROM well_schema_migrations`)
	if err != nil {
		return fmt.Errorf("read well_schema_migrations: %w", err)
	}
	defer rows.Close()
	applied := make(map[string]bool)
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			return fmt.Errorf("read well_schema_migrations: %w", err)
		}
		applied[f] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read well_schema_migrations: %w", err)
	}
	var missing []string
	for _, f := range files {
		if !applied[f] {
			missing = append(missing, f)
		}
	}
	if len(missing) > 0 {
		return &ErrMigrationMissing{Files: missing}
	}
	return nil
}
