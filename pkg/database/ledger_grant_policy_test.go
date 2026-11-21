// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package database

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// ledgerReaders maps every non-test Go file that calls RequireMigrations to
// the Well role its connection runs as. LEDGER.sql revokes
// well_schema_migrations from PUBLIC, so a caller's role needs its own
// SELECT grant in a migration: without it the service exits at start with
// "permission denied for table well_schema_migrations" (timeguru on west,
// 2026-11-21: 050 granted its table and not the ledger).
var ledgerReaders = map[string]string{
	"cmd/kanban-app/pgstore.go":              "app_kanban",
	"services/timeguru/cmd/timeguru/main.go": "app_timeguru",
}

// TestLedgerReadersHaveGrant fails when a binary starts calling
// RequireMigrations without being listed above, and when a listed role has
// no migration granting it SELECT on the ledger.
func TestLedgerReadersHaveGrant(t *testing.T) {
	root := moduleRoot(t)

	call := regexp.MustCompile(`\bdatabase\.RequireMigrations\(`)
	found := map[string]bool{}
	for _, top := range []string{"cmd", "services", "pkg", "internal"} {
		dir := filepath.Join(root, top)
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "vendor" || d.Name() == "node_modules" || d.Name() == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			src, err := os.ReadFile(p) // #nosec G304 -- a path under the module root, walked by the test itself
			if err != nil {
				return err
			}
			if call.Match(src) {
				rel, err := filepath.Rel(root, p)
				if err != nil {
					return err
				}
				found[filepath.ToSlash(rel)] = true
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for f := range found {
		if _, ok := ledgerReaders[f]; !ok {
			t.Errorf("%s calls database.RequireMigrations and is not in ledgerReaders: name the Well role it connects as, and grant that role SELECT on well_schema_migrations in a migration", f)
		}
	}
	for f := range ledgerReaders {
		if !found[f] {
			t.Errorf("ledgerReaders names %s, which no longer calls database.RequireMigrations: remove the entry", f)
		}
	}

	files, err := filepath.Glob(filepath.Join(root, "db", "migrations", "[0-9][0-9][0-9]_*.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no migrations found under db/migrations (%v)", err)
	}
	granted := map[string]string{}
	grant := regexp.MustCompile(`(?im)^\s*GRANT\s+SELECT\s+ON\s+well_schema_migrations\s+TO\s+([^;]+);`)
	for _, f := range files {
		src, err := os.ReadFile(f) // #nosec G304 -- a migration file found by the glob above
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range grant.FindAllStringSubmatch(string(src), -1) {
			for _, role := range strings.Split(m[1], ",") {
				granted[strings.TrimSpace(role)] = filepath.Base(f)
			}
		}
	}
	for f, role := range ledgerReaders {
		if _, ok := granted[role]; !ok {
			t.Errorf("%s reads the migration ledger as %s, and no migration grants %s SELECT on well_schema_migrations: add a numbered migration (see 038)", f, role, role)
		}
	}
}

// moduleRoot walks up from the test's directory to the directory holding go.mod.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above the test directory")
		}
		dir = parent
	}
}
