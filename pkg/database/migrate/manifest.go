// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

// Package migrate applies The Well's migrations in the order and to the
// databases db/migrations/MANIFEST names, and records each one in a
// per-database ledger (well_schema_migrations) with its sha256 (ADR-101).
// db/init.sh reads the same manifest on an empty volume, so a fresh Well
// and a migrated one end with the same ledger.
package migrate

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"
)

// ManifestName and LedgerName live in the migrations directory.
const (
	ManifestName = "MANIFEST"
	LedgerName   = "LEDGER.sql"
)

var (
	dbNameRe   = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,62}$`)
	fileNameRe = regexp.MustCompile(`^[0-9]{3}_[a-z0-9_]+\.sql$`)
	// A psql meta-command (\connect, \c, \set, \echo, ...) at the start of
	// a line. The runner sends SQL to the server; psql never sees it.
	metaRe = regexp.MustCompile(`(?m)^[ \t]*\\[a-zA-Z!]`)
)

// Entry is one manifest line: apply File to database DB. NoTx entries run
// statement by statement outside a transaction (CREATE DATABASE cannot run
// in one). Every entry runs as the database's owner role when that owner is
// not a superuser (ERDA 2.2: objects a migration creates belong to the
// owner); Superuser entries keep the runner's superuser (the ones that
// hand objects over, or need cluster privileges).
type Entry struct {
	DB        string
	File      string
	NoTx      bool
	Superuser bool
	Line      int
}

// Manifest is the ordered entries plus the files deliberately not applied.
type Manifest struct {
	Entries []Entry
	Skip    []string
}

// ParseManifest reads lines of `<database> <file> [notx] [superuser]` and
// `skip <file>`; blank lines and # comments are ignored.
func ParseManifest(r io.Reader) (Manifest, error) {
	var m Manifest
	seen := map[[2]string]bool{}
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		if f[0] == "skip" {
			if len(f) != 2 || !fileNameRe.MatchString(f[1]) {
				return m, fmt.Errorf("manifest line %d: want `skip <NNN_name.sql>`", n)
			}
			m.Skip = append(m.Skip, f[1])
			continue
		}
		if len(f) < 2 || len(f) > 4 {
			return m, fmt.Errorf("manifest line %d: want `<database> <file> [notx] [superuser]`", n)
		}
		e := Entry{DB: f[0], File: f[1], Line: n}
		if !dbNameRe.MatchString(e.DB) {
			return m, fmt.Errorf("manifest line %d: bad database name %q", n, e.DB)
		}
		if !fileNameRe.MatchString(e.File) {
			return m, fmt.Errorf("manifest line %d: bad file name %q (want NNN_name.sql)", n, e.File)
		}
		for _, flag := range f[2:] {
			switch {
			case flag == "notx" && !e.NoTx:
				e.NoTx = true
			case flag == "superuser" && !e.Superuser:
				e.Superuser = true
			default:
				return m, fmt.Errorf("manifest line %d: unknown or repeated flag %q", n, flag)
			}
		}
		key := [2]string{e.DB, e.File}
		if seen[key] {
			return m, fmt.Errorf("manifest line %d: %s already applied to %s", n, e.File, e.DB)
		}
		seen[key] = true
		m.Entries = append(m.Entries, e)
	}
	if err := sc.Err(); err != nil {
		return m, err
	}
	if len(m.Entries) == 0 {
		return m, errors.New("manifest has no entries")
	}
	for _, s := range m.Skip {
		for _, e := range m.Entries {
			if e.File == s {
				return m, fmt.Errorf("manifest: %s is both applied (line %d) and skipped", s, e.Line)
			}
		}
	}
	return m, nil
}

// noTxControl refuses a top-level statement that begins or ends a
// transaction. The runner wraps each transactional file and its ledger row
// in one transaction: a COMMIT in the file would make what precedes it
// permanent while a later failure leaves the file unrecorded, and a BEGIN
// in a notx file would leave the session's ledger row uncommitted. BEGIN
// and END inside a DO block or function body are PL/pgSQL, inside one
// statement, and ROLLBACK TO a savepoint stays in the transaction.
func noTxControl(sql string) error {
	stmts, err := Split(sql)
	if err != nil {
		return err
	}
	for i, s := range stmts {
		w := strings.Fields(strings.ToUpper(stripLeadingComments(s)))
		if len(w) == 0 {
			continue
		}
		switch w[0] {
		case "BEGIN", "START", "COMMIT", "END", "ABORT":
		case "PREPARE": // PREPARE TRANSACTION; PREPARE name AS ... is a statement
			if len(w) < 2 || w[1] != "TRANSACTION" {
				continue
			}
		case "ROLLBACK":
			if len(w) > 1 && strings.HasPrefix(w[1], "TO") {
				continue
			}
		default:
			continue
		}
		return fmt.Errorf("statement %d is transaction control (%s); the runner owns the transaction", i+1, strings.TrimSuffix(w[0], ";"))
	}
	return nil
}

// stripLeadingComments drops the -- and /* */ comments before a statement's
// first word (Split keeps them with the statement that follows).
func stripLeadingComments(s string) string {
	for {
		s = strings.TrimSpace(s)
		switch {
		case strings.HasPrefix(s, "--"):
			i := strings.IndexByte(s, '\n')
			if i < 0 {
				return ""
			}
			s = s[i+1:]
		case strings.HasPrefix(s, "/*"):
			depth, i := 1, 2
			for ; i < len(s) && depth > 0; i++ {
				if strings.HasPrefix(s[i:], "/*") {
					depth++
					i++
				} else if strings.HasPrefix(s[i:], "*/") {
					depth--
					i++
				}
			}
			s = s[i:]
		default:
			return s
		}
	}
}

// File is a manifest entry with its contents and checksum.
type File struct {
	Entry
	SQL    string
	SHA256 string
}

// Set is everything the runner needs from the migrations directory.
type Set struct {
	Files  []File
	Ledger string // LEDGER.sql: the ledger's own DDL, applied before any record
}

// Databases returns the databases the manifest names, in first-use order.
func (s Set) Databases() []string {
	var out []string
	for _, f := range s.Files {
		if !slices.Contains(out, f.DB) {
			out = append(out, f.DB)
		}
	}
	return out
}

// Load reads the manifest and every file it names from dir. It refuses a
// numbered .sql file the manifest neither applies nor skips (a migration
// nobody routed is the gap W-09 describes), a missing file, and psql
// meta-commands, which only psql understands.
func Load(dir string) (Set, error) {
	var set Set
	// Every read goes through os.Root: nothing outside dir can be opened,
	// whatever a manifest line says.
	root, err := os.OpenRoot(dir)
	if err != nil {
		return set, err
	}
	defer func() { _ = root.Close() }()
	mf, err := root.Open(ManifestName)
	if err != nil {
		return set, err
	}
	defer func() { _ = mf.Close() }()
	m, err := ParseManifest(mf)
	if err != nil {
		return set, err
	}
	ledger, err := root.ReadFile(LedgerName)
	if err != nil {
		return set, err
	}
	set.Ledger = string(ledger)

	routed := map[string]bool{}
	for _, s := range m.Skip {
		routed[s] = true
	}
	for _, e := range m.Entries {
		routed[e.File] = true
		b, err := root.ReadFile(e.File)
		if err != nil {
			return set, fmt.Errorf("manifest line %d: %w", e.Line, err)
		}
		if loc := metaRe.FindIndex(b); loc != nil {
			line := 1 + strings.Count(string(b[:loc[0]]), "\n")
			return set, fmt.Errorf("%s:%d: psql meta-command; the manifest routes files, the runner sends plain SQL", e.File, line)
		}
		if err := noTxControl(string(b)); err != nil {
			return set, fmt.Errorf("%s: %w", e.File, err)
		}
		sum := sha256.Sum256(b)
		set.Files = append(set.Files, File{Entry: e, SQL: string(b), SHA256: hex.EncodeToString(sum[:])})
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return set, err
	}
	for _, de := range ents {
		if fileNameRe.MatchString(de.Name()) && !routed[de.Name()] {
			return set, fmt.Errorf("%s is in %s but not in the manifest (add it, or `skip` it)", de.Name(), dir)
		}
	}
	return set, nil
}
