// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package well

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"unheaded/pkg/compliance/crosswalk"
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
	want := map[string]string{"west": "lab", "east": "lab-hardened"} // east since 2026-10-12
	for _, b := range bs {
		if b.Profile != want[b.Host] || b.Mode != "audit" {
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
	// The Well must resolve exactly as the YAML it was seeded from (a fixed
	// count went stale when the lab profile grew).
	if got, want := ps.Resolve("lab"), yps.Resolve("lab"); len(want) == 0 || !slices.Equal(got, want) {
		t.Errorf("lab resolves to %v, YAML to %v", got, want)
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

func journal(t *testing.T, n int) []remediation.JournalEntry {
	t.Helper()
	var out []remediation.JournalEntry
	prev := remediation.GenesisHash
	for i := 1; i <= n; i++ {
		e := remediation.JournalEntry{Host: "east", Seq: int64(i), FixID: "kmod-cis", FixSHA256: strings.Repeat("b", 64),
			Action: "apply", Outcome: "success", Operator: "test", At: time.Date(2026, 10, 1, 2, i, 0, 0, time.UTC)}
		if err := e.Seal(prev); err != nil {
			t.Fatal(err)
		}
		prev = e.Hash
		out = append(out, e)
	}
	return out
}

func TestIngestJournalRefusesBrokenInputWithoutTheWell(t *testing.T) {
	s := NewOpsStore(nil) // never reached: the chain is checked first
	j := journal(t, 3)
	j[1].Outcome = "failure"
	if _, err := s.IngestJournal(context.Background(), "east", j); !errors.Is(err, remediation.ErrChainBroken) {
		t.Fatalf("tampered: %v", err)
	}
	if _, err := s.IngestJournal(context.Background(), "west", journal(t, 2)); !errors.Is(err, remediation.ErrChainBroken) {
		t.Fatalf("wrong host: %v", err)
	}
	if _, err := s.IngestJournal(context.Background(), "east", journal(t, 3)[1:]); !errors.Is(err, remediation.ErrChainBroken) {
		t.Fatalf("not from seq 1: %v", err)
	}
}

// Integration against a throwaway unheaded_ops as ops_writer
// (scripts/well/test-compliance-schema.sh --go). Never The Well itself.
func TestIntegrationIngestJournal(t *testing.T) {
	dsn := os.Getenv("WELL_TEST_OPS_DSN")
	if dsn == "" {
		t.Skip("WELL_TEST_OPS_DSN not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewOpsStore(db)
	j := journal(t, 5)
	if n, err := s.IngestJournal(ctx, "east", j[:3]); err != nil || n != 3 {
		t.Fatalf("first ingest: %d %v", n, err)
	}
	if n, err := s.IngestJournal(ctx, "east", j); err != nil || n != 2 {
		t.Fatalf("second ingest: %d %v (want only the 2 new)", n, err)
	}
	if n, err := s.IngestJournal(ctx, "east", j); err != nil || n != 0 {
		t.Fatalf("re-ingest: %d %v (want idempotent)", n, err)
	}
	// Truncated: the host now shows fewer entries than The Well holds.
	if _, err := s.IngestJournal(ctx, "east", j[:4]); !errors.Is(err, remediation.ErrChainBroken) {
		t.Fatalf("truncated: %v", err)
	}
	// Rewritten: a new self-consistent chain whose early entries differ.
	re := journal(t, 6)
	re[0].Operator = "mallory"
	prev := remediation.GenesisHash
	for i := range re {
		if err := re[i].Seal(prev); err != nil {
			t.Fatal(err)
		}
		prev = re[i].Hash
	}
	if _, err := s.IngestJournal(ctx, "east", re); !errors.Is(err, remediation.ErrChainBroken) {
		t.Fatalf("rewritten: %v", err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM compliance_apply_journal WHERE host = 'east'`).Scan(&count); err != nil || count != 5 {
		t.Fatalf("held %d rows (%v), want 5", count, err)
	}
}

func TestDecodeEntryIsStrict(t *testing.T) {
	if _, err := decodeEntry([]byte(`{"id":"FND-001","severity":"low","shell":"rm -rf /"}`)); !errors.Is(err, ErrInvalidChange) {
		t.Fatalf("unknown field accepted: %v", err)
	}
	if err := checkWho("", "r"); !errors.Is(err, ErrInvalidChange) {
		t.Error("empty actor accepted")
	}
	if err := checkWho("a", "line\nbreak"); !errors.Is(err, ErrInvalidChange) {
		t.Error("control character accepted")
	}
}

// Integration: the register round-trips through The Well and gives the
// same findings report as the YAML, on the repo's own evidence.
func TestIntegrationFindingsParity(t *testing.T) {
	dsn := os.Getenv("WELL_TEST_OPS_DSN")
	if dsn == "" {
		t.Skip("WELL_TEST_OPS_DSN not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cat, err := crosswalk.LoadFS(os.DirFS(repo), "compliance/catalog/frameworks", "compliance/catalog/controls.yaml")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(repo, "compliance/findings/register.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	yreg, err := crosswalk.LoadRegister(raw, cat)
	if err != nil {
		t.Fatal(err)
	}
	s := NewOpsStore(db)
	for _, e := range yreg.Entries {
		if err := s.UpsertFinding(ctx, cat, e, "test", "seed from register.yaml"); err != nil {
			t.Fatal(err)
		}
	}
	wreg, err := s.Register(ctx, cat)
	if err != nil {
		t.Fatal(err)
	}
	var recs []crosswalk.Record
	if f, err := os.Open(filepath.Join(repo, "var/compliance/evidence.json")); err == nil {
		recs, err = crosswalk.ReadEvidence(f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := s.AppendEvidence(ctx, recs); err != nil {
		t.Fatal(err)
	}
	latest, err := s.LatestEvidence(ctx)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	a := crosswalk.Findings(cat, recs, yreg, now)
	b := crosswalk.Findings(cat, latest, wreg, now)
	if len(a.Open) != len(b.Open) || a.Accepted != b.Accepted || a.Overdue != b.Overdue ||
		len(a.Resolved) != len(b.Resolved) || len(a.Unobserved) != len(b.Unobserved) || len(a.Stale) != len(b.Stale) {
		t.Fatalf("YAML: %d open %d accepted %d resolved %d unobserved %d stale; Well: %d open %d accepted %d resolved %d unobserved %d stale",
			len(a.Open), a.Accepted, len(a.Resolved), len(a.Unobserved), len(a.Stale),
			len(b.Open), b.Accepted, len(b.Resolved), len(b.Unobserved), len(b.Stale))
	}
	for sev, n := range a.Counts {
		if b.Counts[sev] != n {
			t.Errorf("%s: YAML %d, Well %d", sev, n, b.Counts[sev])
		}
	}
	t.Logf("parity on %d records: %d open, %d accepted", len(recs), len(a.Open), a.Accepted)

	key := yreg.Entries[0].Key()
	if err := s.ResolveFinding(ctx, key, "test", "evidence passes"); err != nil {
		t.Fatal(err)
	}
	var op, by, why string
	if err := db.QueryRowContext(ctx, `SELECT op, changed_by, change_reason FROM compliance_findings_history
		WHERE key = $1 ORDER BY id DESC LIMIT 1`, key).Scan(&op, &by, &why); err != nil {
		t.Fatal(err)
	}
	if op != "DELETE" || by != "test" || why != "evidence passes" {
		t.Errorf("resolve history: %s by %s (%s)", op, by, why)
	}
}

func TestLazyRetriesAtMostOncePerInterval(t *testing.T) {
	l := NewLazy(database.Config{Host: "127.0.0.1", Port: "1", User: "u", Password: "p", DBName: "d", SSLMode: "disable"}, time.Minute)
	now := time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC)
	l.now = func() time.Time { return now }
	if _, err := l.DB(context.Background()); err == nil {
		t.Fatal("connected to port 1")
	}
	first := l.lastTry
	now = now.Add(30 * time.Second)
	if _, err := l.DB(context.Background()); err == nil || l.lastTry != first {
		t.Fatalf("dialled again inside the retry interval (last try %s)", l.lastTry)
	}
	now = now.Add(31 * time.Second)
	if _, err := l.DB(context.Background()); err == nil || l.lastTry.Equal(first) {
		t.Fatal("did not retry after the interval")
	}
	var nilLazy *Lazy
	if _, err := nilLazy.DB(context.Background()); err == nil {
		t.Error("nil Lazy returned a connection")
	}
}
