// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package migrate

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseManifest(t *testing.T) {
	m, err := ParseManifest(strings.NewReader("# c\n\npostgres 002_a.sql notx\nunheaded_app 003_b.sql\nunheaded_ops 003_b.sql superuser\npostgres 004_c.sql superuser notx\nskip 001_old.sql\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := []Entry{{DB: "postgres", File: "002_a.sql", NoTx: true, Line: 3}, {DB: "unheaded_app", File: "003_b.sql", Line: 4}, {DB: "unheaded_ops", File: "003_b.sql", Superuser: true, Line: 5}, {DB: "postgres", File: "004_c.sql", NoTx: true, Superuser: true, Line: 6}}
	if !reflect.DeepEqual(m.Entries, want) || !reflect.DeepEqual(m.Skip, []string{"001_old.sql"}) {
		t.Fatalf("got %+v", m)
	}
	for name, in := range map[string]string{
		"empty":             "# nothing\n",
		"bad flag":          "postgres 002_a.sql tx\n",
		"bad db":            "Postgres; 002_a.sql\n",
		"traversal":         "postgres ../002_a.sql\n",
		"unnumbered":        "postgres a.sql\n",
		"duplicate":         "postgres 002_a.sql\npostgres 002_a.sql\n",
		"applied and skip":  "postgres 002_a.sql\nskip 002_a.sql\n",
		"unknown flag":      "postgres 002_a.sql notx extra\n",
		"repeated flag":     "postgres 002_a.sql notx notx\n",
		"too many fields":   "postgres 002_a.sql notx superuser extra\n",
		"skip without file": "skip\n",
	} {
		if _, err := ParseManifest(strings.NewReader(in)); err == nil {
			t.Errorf("%s: accepted %q", name, in)
		}
	}
}

func writeDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for n, c := range files {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(c), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestLoad(t *testing.T) {
	base := map[string]string{
		ManifestName:  "postgres 002_a.sql notx\nunheaded_app 003_b.sql\nskip 001_old.sql\n",
		LedgerName:    "CREATE TABLE IF NOT EXISTS well_schema_migrations ();",
		"001_old.sql": "\\connect x\n",
		"002_a.sql":   "CREATE DATABASE a;\n",
		"003_b.sql":   "CREATE TABLE b ();\n",
		"README.md":   "not a migration",
	}
	set, err := Load(writeDir(t, base))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(base["003_b.sql"]))
	if len(set.Files) != 2 || set.Files[1].SHA256 != hex.EncodeToString(sum[:]) || !set.Files[0].NoTx {
		t.Fatalf("files %+v", set.Files)
	}
	if got := set.Databases(); !reflect.DeepEqual(got, []string{"postgres", "unheaded_app"}) {
		t.Errorf("databases %v", got)
	}
	for name, change := range map[string]map[string]string{
		"unrouted file":  {"004_new.sql": "SELECT 1;"},
		"missing file":   {ManifestName: "postgres 009_gone.sql\nskip 001_old.sql\n"},
		"meta-command":   {"003_b.sql": "SELECT 1;\n  \\c unheaded_app\n"},
		"echo":           {"003_b.sql": "\\echo done\n"},
		"no ledger file": {LedgerName: ""},
		"no manifest":    {ManifestName: ""},
	} {
		files := map[string]string{}
		for k, v := range base {
			files[k] = v
		}
		dir := writeDir(t, files)
		for k, v := range change {
			p := filepath.Join(dir, k)
			if v == "" {
				os.Remove(p)
				continue
			}
			if err := os.WriteFile(p, []byte(v), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := Load(dir); err == nil {
			t.Errorf("%s: loaded", name)
		}
	}
}

// The real directory loads: every numbered file is routed or skipped and
// none uses a psql meta-command.
func TestLoadRepoMigrations(t *testing.T) {
	set, err := Load("../../../db/migrations")
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Files) == 0 || set.Files[0].File != "002_create_databases.sql" || !set.Files[0].NoTx {
		t.Fatalf("first entry %+v", set.Files[0].Entry)
	}
	for _, f := range set.Files {
		if f.NoTx {
			if _, err := Split(f.SQL); err != nil {
				t.Errorf("%s: %v", f.File, err)
			}
		}
	}
}

func TestSplit(t *testing.T) {
	in := `-- header; with a semicolon
CREATE DATABASE a;
CREATE USER "we;ird";
SELECT 'it''s; fine', E'back\'slash;';
/* block; /* nested; */ still; */
CREATE FUNCTION f() RETURNS int AS $$ SELECT 1; $$ LANGUAGE sql;
CREATE FUNCTION g() RETURNS int AS $body$ SELECT 2; $body$ LANGUAGE sql;
SELECT $1;
-- trailing comment only`
	got, err := Split(in)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"-- header; with a semicolon\nCREATE DATABASE a",
		`CREATE USER "we;ird"`,
		`SELECT 'it''s; fine', E'back\'slash;'`,
		"/* block; /* nested; */ still; */\nCREATE FUNCTION f() RETURNS int AS $$ SELECT 1; $$ LANGUAGE sql",
		"CREATE FUNCTION g() RETURNS int AS $body$ SELECT 2; $body$ LANGUAGE sql",
		"SELECT $1",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q", got)
	}
	for _, bad := range []string{"SELECT 'open;", `CREATE USER "open;`, "SELECT $$ open;", "/* open; "} {
		if _, err := Split(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

func TestCompute(t *testing.T) {
	f := func(db, file, sum string) File {
		return File{Entry: Entry{DB: db, File: file}, SHA256: sum}
	}
	set := Set{Files: []File{
		f("postgres", "002_a.sql", "a"),
		f("app", "003_b.sql", "b"),
		f("app", "004_c.sql", "c"),
		f("ops", "004_c.sql", "c"),
	}}
	rec := func(db, file, sum string) Record { return Record{DB: db, File: file, SHA256: sum, Mode: "initdb"} }

	p := Compute(set, Ledger{"postgres": {"002_a.sql": rec("postgres", "002_a.sql", "a")}, "app": {"003_b.sql": rec("app", "003_b.sql", "b")}})
	if err := p.Drift(); err != nil {
		t.Fatal(err)
	}
	if got := p.Pending(); len(got) != 2 || got[0].DB != "app" || got[1].DB != "ops" {
		t.Fatalf("pending %+v", got)
	}

	p = Compute(set, Ledger{"app": {"003_b.sql": rec("app", "003_b.sql", "edited")}})
	if err := p.Drift(); err == nil || !strings.Contains(err.Error(), "changed after it was applied") {
		t.Errorf("mismatch: %v", err)
	}
	p = Compute(set, Ledger{"app": {"009_x.sql": rec("app", "009_x.sql", "x")}})
	if err := p.Drift(); err == nil || !strings.Contains(err.Error(), "does not name") {
		t.Errorf("orphan: %v", err)
	}
	// 004 applied on app but 003 not: a gap. On ops, 004 alone is fine.
	p = Compute(set, Ledger{"app": {"004_c.sql": rec("app", "004_c.sql", "c")}, "ops": {"004_c.sql": rec("ops", "004_c.sql", "c")}})
	if err := p.Drift(); err == nil || !strings.Contains(err.Error(), "003_b.sql on app is pending") {
		t.Errorf("gap: %v", err)
	}
}

func TestThrough(t *testing.T) {
	f := func(db, file string) File { return File{Entry: Entry{DB: db, File: file}} }
	p := Compute(Set{Files: []File{f("postgres", "002_a.sql"), f("app", "007_g.sql"), f("ops", "007_g.sql"), f("app", "013_c.sql")}}, Ledger{})
	q, err := p.Through("007_g.sql")
	if err != nil {
		t.Fatal(err)
	}
	if got := q.Pending(); len(got) != 3 || got[2].DB != "ops" {
		t.Fatalf("through 007 (every database): %+v", got)
	}
	if _, err := p.Through("099_nope.sql"); err == nil {
		t.Error("unknown file accepted")
	}
}

func TestForDB(t *testing.T) {
	f := func(db, file string) File { return File{Entry: Entry{DB: db, File: file}} }
	p := Compute(Set{Files: []File{f("postgres", "002_a.sql"), f("app", "007_g.sql"), f("ops", "007_g.sql")}}, Ledger{})
	q, err := p.ForDB("ops")
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Steps) != 1 || q.Steps[0].DB != "ops" {
		t.Fatalf("ops: %+v", q.Steps)
	}
	if _, err := p.ForDB("nope"); err == nil {
		t.Error("unknown database accepted")
	}
}
