// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package well

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"unheaded/pkg/compliance/remediation"
	"unheaded/pkg/database"
)

var repo = filepath.Join("..", "..", "..")

func TestYAMLStoreReadsRepoAndRefusesWrites(t *testing.T) {
	s, err := NewYAMLStore(repo)
	if err != nil {
		t.Fatal(err)
	}
	if s.Source() != SourceYAML {
		t.Errorf("source %q", s.Source())
	}
	ps, err := s.Profiles(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"none", "lab", "cis-l1-server", "all"} {
		if !ps.Has(name) {
			t.Errorf("profile %s missing", name)
		}
	}
	bs, err := s.Bindings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) != 2 {
		t.Fatalf("%d bindings, want west and east", len(bs))
	}
	for _, b := range bs {
		if b.Profile != "lab" || b.Mode != "audit" {
			t.Errorf("%s: %s/%s", b.Host, b.Profile, b.Mode)
		}
	}
	err = s.SetBinding(context.Background(), Change{Host: "east", Profile: "none", Mode: "audit", Actor: "t", Reason: "r"})
	if !errors.Is(err, ErrReadOnlyFallback) {
		t.Fatalf("write on fallback: %v, want ErrReadOnlyFallback", err)
	}
}

func TestOpenFallsBackWhenTheWellIsUnreachable(t *testing.T) {
	for name, cfg := range map[string]database.Config{
		"unreachable": {Host: "127.0.0.1", Port: "1", User: "config_reader", Password: "x", DBName: "unheaded_config", SSLMode: "disable"},
		"no password": {Host: "127.0.0.1", Port: "5432", User: "config_reader", DBName: "unheaded_config", SSLMode: "disable"},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			start := time.Now()
			s, reason, err := Open(ctx, repo, cfg)
			if err != nil {
				t.Fatal(err)
			}
			if s.Source() != SourceYAML || reason == "" {
				t.Errorf("source %q reason %q, want labelled fallback with a reason", s.Source(), reason)
			}
			if d := time.Since(start); d > 7*time.Second {
				t.Errorf("fallback took %s", d)
			}
		})
	}
}

func TestOpenRefusesInvalidRepoData(t *testing.T) {
	// Invalid repo data is an error, never a silent fallback: the fallback
	// is for The Well being down, not for a broken model.
	if _, _, err := Open(context.Background(), t.TempDir(), database.Config{}); err == nil {
		t.Fatal("opened without a fix model")
	}
}

func TestChangeValidation(t *testing.T) {
	s, err := NewYAMLStore(repo)
	if err != nil {
		t.Fatal(err)
	}
	ps, _ := s.Profiles(context.Background())
	ok := Change{Host: "east", Profile: "lab", Mode: "audit", Actor: "govan", Reason: "lab baseline"}
	if err := ok.Validate(ps); err != nil {
		t.Fatalf("valid change refused: %v", err)
	}
	for name, mut := range map[string]func(*Change){
		"unknown host":    func(c *Change) { c.Host = "north" },
		"unknown profile": func(c *Change) { c.Profile = "yolo" },
		"no actor":        func(c *Change) { c.Actor = " " },
		"no reason":       func(c *Change) { c.Reason = "" },
		"control chars":   func(c *Change) { c.Reason = "ok\x1b[2Jcleared" },
		"long reason":     func(c *Change) { c.Reason = strings.Repeat("r", 501) },
		"long actor":      func(c *Change) { c.Actor = strings.Repeat("a", 101) },
	} {
		t.Run(name, func(t *testing.T) {
			c := ok
			mut(&c)
			if err := c.Validate(ps); !errors.Is(err, ErrInvalidChange) {
				t.Fatalf("got %v, want ErrInvalidChange", err)
			}
		})
	}
	c := ok
	c.Mode = "enforcing"
	if err := c.Validate(ps); !errors.Is(err, remediation.ErrEnforcingGate) {
		t.Fatalf("enforcing: got %v, want ErrEnforcingGate", err)
	}
}

// Integration: runs against a real unheaded_config when WELL_TEST_CONFIG_DSN
// names one as config_admin (scripts/well/test-compliance-schema.sh --go
// provides a throwaway one). Never point it at The Well itself: it writes.
func TestIntegrationPGStore(t *testing.T) {
	dsn := os.Getenv("WELL_TEST_CONFIG_DSN")
	if dsn == "" {
		t.Skip("WELL_TEST_CONFIG_DSN not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	m, err := LoadModel(repo)
	if err != nil {
		t.Fatal(err)
	}
	// Seed as `well sync` will: fix mirrors first (profiles refer to them).
	for _, f := range m.Fixes {
		if _, err := db.ExecContext(ctx, `INSERT INTO compliance_fixes (id, title, class, risk, body, sha256)
			VALUES ($1, $2, $3, $4, '{}', $5) ON CONFLICT (id) DO NOTHING`,
			f.ID, f.Title, f.Class, f.Risk, strings.Repeat("0", 64)); err != nil {
			t.Fatal(err)
		}
	}
	y, err := NewYAMLStore(repo)
	if err != nil {
		t.Fatal(err)
	}
	yps, _ := y.Profiles(ctx)
	for _, p := range yps.Profiles {
		if _, err := db.ExecContext(ctx, `INSERT INTO compliance_profiles (name, description, all_fixes, fixes, updated_by)
			VALUES ($1, $2, $3, $4, 'test') ON CONFLICT (name) DO NOTHING`,
			p.Name, p.Description, p.All, pqArray(p.Fixes)); err != nil {
			t.Fatal(err)
		}
	}
	s := NewPGStore(db, m)
	if s.Source() != SourceWell {
		t.Errorf("source %q", s.Source())
	}
	ps, err := s.Profiles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := ps.Resolve("lab"); len(got) != 8 {
		t.Errorf("lab resolves to %v", got)
	}
	var before int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM compliance_host_bindings_history WHERE host = 'east'`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"lab", "none"} {
		if err := s.SetBinding(ctx, Change{Host: "east", Profile: p, Mode: "audit", Actor: "test", Reason: "switch to " + p}); err != nil {
			t.Fatal(err)
		}
	}
	bs, err := s.Bindings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) != 1 || bs[0].Host != "east" || bs[0].Profile != "none" {
		t.Errorf("bindings %+v", bs)
	}
	var after int
	var reason string
	if err := db.QueryRowContext(ctx, `SELECT count(*), max(change_reason) FILTER (WHERE new_profile = 'none')
		FROM compliance_host_bindings_history WHERE host = 'east'`).Scan(&after, &reason); err != nil {
		t.Fatal(err)
	}
	if after != before+2 || reason != "switch to none" {
		t.Errorf("history %d -> %d (%q), want two rows with who and why", before, after, reason)
	}
	if err := s.SetBinding(ctx, Change{Host: "east", Profile: "lab", Mode: "enforcing", Actor: "test", Reason: "r"}); !errors.Is(err, remediation.ErrEnforcingGate) {
		t.Errorf("enforcing reached the database: %v", err)
	}
}
