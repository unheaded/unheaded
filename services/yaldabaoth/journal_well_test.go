// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package yaldabaoth

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// wellTestDB opens WELL_TEST_DSN (a throwaway unheaded_ops with migration
// 039, as ops_writer: scripts/well/test-deploy-journal.sh) or skips.
func wellTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("WELL_TEST_DSN")
	if dsn == "" {
		t.Skip("WELL_TEST_DSN not set (scripts/well/test-deploy-journal.sh sets it)")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func count(t *testing.T, db *sql.DB, q string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// The journal in The Well: an experiment's entry is a row before the fault
// lands, its revert a second row, and a controller that died with a fault
// in place has it reverted by the next one on the same host, never by
// another host's.
func TestWellJournal(t *testing.T) {
	db := wellTestDB(t)
	ctx := context.Background()
	host := fmt.Sprintf("t%d", time.Now().UnixNano())
	newStore := func(h string) JournalStore {
		j, err := NewWellJournal(ctx, db, h)
		if err != nil {
			t.Fatal(err)
		}
		return j
	}

	t.Run("an experiment journals and reverts in the Well", func(t *testing.T) {
		fx := newFixture(t, func(c *Config, _ *Deps) { c.StateDir = ""; c.Journal = newStore(host) })
		fx.inj.svc = fx.s
		exp := fx.start(t, delayExp("steel-echo-v2-0", 50*time.Millisecond))
		if e := fx.wait(t, exp.ID); e.Status != StatusCompleted {
			t.Fatalf("experiment: %s %+v", e.Status, e)
		}
		if seen := fx.inj.journalSeen; len(seen) != 1 || seen[0] != 1 {
			t.Fatalf("journal size at injection = %v, want [1]", seen)
		}
		if n := count(t, db, `SELECT count(*) FROM chaos_journal WHERE entry_id = $1 AND host = $2 AND kind = 'experiment'`, exp.ID, host); n != 1 {
			t.Fatalf("chaos_journal rows for %s: %d", exp.ID, n)
		}
		if n := count(t, db, `SELECT count(*) FROM chaos_journal_transitions WHERE entry_id = $1 AND to_status = 'reverted'`, exp.ID); n != 1 {
			t.Fatalf("reverted rows: %d", n)
		}
	})

	t.Run("a crashed controller's fault is reverted by the next one", func(t *testing.T) {
		st := newStore(host)
		e := &JournalEntry{ID: fmt.Sprintf("chaos-crashed-%d", time.Now().UnixNano()), Type: ExpTypeNetworkDelay,
			Target:  ResolvedTarget{Kind: KindLXD, ID: "steel-echo-v2-0", IP: "10.10.20.31"},
			Recipe:  Recipe{"fault": "lxd/steel-echo-v2-0"},
			Created: time.Now().UTC().Add(-time.Minute), Expires: time.Now().UTC().Add(time.Hour)}
		if err := st.Add(ctx, e, nil); err != nil {
			t.Fatal(err)
		}
		// Another host's controller does not see it.
		other, err := NewService(&Config{Journal: newStore(host + "-other")}, Deps{})
		if err != nil {
			t.Fatal(err)
		}
		if n := len(other.JournalEntries()); n != 0 {
			t.Fatalf("another host loaded %d entries", n)
		}
		inj := newFakeInjector()
		inj.active["lxd/steel-echo-v2-0"] = true
		s, err := NewService(&Config{Journal: newStore(host)}, Deps{})
		if err != nil {
			t.Fatal(err)
		}
		s.RegisterInjector(ExpTypeNetworkDelay, inj)
		got := s.JournalEntries()
		if len(got) != 1 || got[0].ID != e.ID || got[0].Recipe["fault"] != "lxd/steel-echo-v2-0" || got[0].Target.IP != "10.10.20.31" {
			t.Fatalf("loaded: %+v", got)
		}
		if left, err := s.RevertAll(ctx); left != 0 || err != nil || inj.activeCount() != 0 {
			t.Fatalf("revert-all: left %d err %v active %d", left, err, inj.activeCount())
		}
		if n := count(t, db, `SELECT count(*) FROM chaos_journal_transitions WHERE entry_id = $1 AND note = 'revert-all'`, e.ID); n != 1 {
			t.Fatalf("reverted rows: %d", n)
		}
		if n := count(t, db, `SELECT count(*) FROM chaos_journal WHERE entry_id = $1`, e.ID); n != 1 {
			t.Fatalf("the journal row changed: %d", n)
		}
	})

	t.Run("a fault the injector cannot verify stays open", func(t *testing.T) {
		st := newStore(host)
		e := &JournalEntry{ID: fmt.Sprintf("chaos-stuck-%d", time.Now().UnixNano()), Type: ExpTypeNetworkDelay,
			Recipe: Recipe{"fault": "stuck"}, Created: time.Now().UTC(), Expires: time.Now().UTC().Add(time.Hour)}
		if err := st.Add(ctx, e, nil); err != nil {
			t.Fatal(err)
		}
		inj := newFakeInjector()
		inj.active["stuck"] = true
		inj.stuck = true
		s, err := NewService(&Config{Journal: newStore(host)}, Deps{})
		if err != nil {
			t.Fatal(err)
		}
		s.RegisterInjector(ExpTypeNetworkDelay, inj)
		if left, _ := s.RevertAll(ctx); left != 1 {
			t.Fatalf("left %d, want 1", left)
		}
		s2, err := NewService(&Config{Journal: newStore(host)}, Deps{})
		if err != nil {
			t.Fatal(err)
		}
		if n := len(s2.JournalEntries()); n != 1 {
			t.Fatalf("an unverified revert closed the entry: %d open", n)
		}
		inj.stuck = false
		s2.RegisterInjector(ExpTypeNetworkDelay, inj)
		if left, err := s2.RevertAll(ctx); left != 0 || err != nil {
			t.Fatalf("second pass: %d %v", left, err)
		}
	})

	t.Run("switching from the file moves its entries into the Well", func(t *testing.T) {
		dir := t.TempDir()
		id := fmt.Sprintf("chaos-filed-%d", time.Now().UnixNano())
		jf := journalFile{Version: 1, Entries: []*JournalEntry{{ID: id, Type: ExpTypeNetworkDelay,
			Recipe: Recipe{"fault": "filed"}, Created: time.Now().UTC(), Expires: time.Now().UTC().Add(time.Hour)}}}
		b, _ := json.Marshal(jf)
		if err := os.WriteFile(filepath.Join(dir, journalFileName), b, 0o600); err != nil {
			t.Fatal(err)
		}
		h := host + "-switch"
		s, err := NewService(&Config{StateDir: dir, Journal: newStore(h)}, Deps{})
		if err != nil {
			t.Fatal(err)
		}
		if got := s.JournalEntries(); len(got) != 1 || got[0].ID != id {
			t.Fatalf("after the switch: %+v", got)
		}
		left, err := NewFileJournal(dir).Load(ctx)
		if err != nil || len(left) != 0 {
			t.Fatalf("file still holds %d entries (%v)", len(left), err)
		}
		// A second start (the file is empty now) changes nothing.
		s2, err := NewService(&Config{StateDir: dir, Journal: newStore(h)}, Deps{})
		if err != nil || len(s2.JournalEntries()) != 1 {
			t.Fatalf("second start: %v %v", s2, err)
		}
	})
}
