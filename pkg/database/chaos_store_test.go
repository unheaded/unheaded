// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package database

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"testing"
	"time"
)

func TestChaosStore(t *testing.T) {
	db := wellTestDB(t)
	s := NewChaosStore(db)
	ctx := context.Background()
	if err := s.CheckSchema(ctx); err != nil {
		t.Fatal(err)
	}
	u := uniq()
	host := "h" + u // a host of its own: Open is per host
	now := time.Now().UTC().Truncate(time.Microsecond)
	entry := func(id, kind string, created time.Time) ChaosEntry {
		return ChaosEntry{
			ID: id, Host: host, Kind: kind, Type: "network_delay",
			Target:    json.RawMessage(`{"kind":"lxd","id":"steel-echo-1"}`),
			Recipe:    json.RawMessage(`{"iface":"veth1","netem":"delay 300ms"}`),
			CreatedAt: created, ExpiresAt: created.Add(3 * time.Minute),
		}
	}
	old, young, pkt := "chaos-old-"+u, "chaos-young-"+u, "packet-"+u
	for _, e := range []ChaosEntry{
		entry(old, ChaosKindExperiment, now.Add(-10*time.Minute)),
		entry(young, ChaosKindExperiment, now),
		entry(pkt, ChaosKindPacket, now),
	} {
		if err := s.Append(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	ids := func(es []ChaosEntry) []string {
		var out []string
		for _, e := range es {
			out = append(out, e.ID)
		}
		sort.Strings(out)
		return out
	}

	t.Run("append twice is an error", func(t *testing.T) {
		if err := s.Append(ctx, entry(old, ChaosKindExperiment, now)); !errors.Is(err, ErrDuplicate) {
			t.Fatalf("want ErrDuplicate, got %v", err)
		}
	})

	t.Run("open lists this host's kind, oldest first, recipe intact", func(t *testing.T) {
		got, err := s.Open(ctx, host, ChaosKindExperiment)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || got[0].ID != old || got[1].ID != young {
			t.Fatalf("open: %v", ids(got))
		}
		if !jsonEqual(t, got[0].Recipe, []byte(`{"iface":"veth1","netem":"delay 300ms"}`)) || got[0].Type != "network_delay" {
			t.Errorf("entry: %+v", got[0])
		}
		if !got[1].CreatedAt.Equal(now) || !got[1].ExpiresAt.Equal(now.Add(3*time.Minute)) {
			t.Errorf("times: %v %v", got[1].CreatedAt, got[1].ExpiresAt)
		}
		pk, err := s.Open(ctx, host, ChaosKindPacket)
		if err != nil || len(pk) != 1 || pk[0].ID != pkt {
			t.Fatalf("packet kind: %v %v", ids(pk), err)
		}
		other, err := s.Open(ctx, "other-"+u, ChaosKindExperiment)
		if err != nil || len(other) != 0 {
			t.Fatalf("another host sees %v (%v)", ids(other), err)
		}
	})

	t.Run("open older than", func(t *testing.T) {
		got, err := s.OpenOlderThan(ctx, host, ChaosKindExperiment, 5*time.Minute)
		if err != nil || len(got) != 1 || got[0].ID != old {
			t.Fatalf("older than 5m: %v %v", ids(got), err)
		}
	})

	t.Run("mark reverted adds a row and never edits the original", func(t *testing.T) {
		if err := s.MarkReverted(ctx, old, "revert-all"); err != nil {
			t.Fatal(err)
		}
		// Twice is fine: revert passes may race (TTL and end of run).
		if err := s.MarkReverted(ctx, old, "ttl"); err != nil {
			t.Fatal(err)
		}
		got, err := s.Open(ctx, host, ChaosKindExperiment)
		if err != nil || len(got) != 1 || got[0].ID != young {
			t.Fatalf("open after revert: %v %v", ids(got), err)
		}
		var journal, reverted int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM chaos_journal WHERE entry_id = $1`, old).Scan(&journal); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM chaos_journal_transitions WHERE entry_id = $1 AND to_status = 'reverted'`, old).Scan(&reverted); err != nil {
			t.Fatal(err)
		}
		if journal != 1 || reverted != 1 {
			t.Errorf("rows: journal %d reverted %d, want 1 and 1", journal, reverted)
		}
		if _, err := db.ExecContext(ctx, `UPDATE chaos_journal SET fault_type = 'x' WHERE entry_id = $1`, old); err == nil {
			t.Fatal("UPDATE on chaos_journal allowed")
		}
		if _, err := db.ExecContext(ctx, `DELETE FROM chaos_journal_transitions WHERE entry_id = $1`, old); err == nil {
			t.Fatal("DELETE on chaos_journal_transitions allowed")
		}
	})

	t.Run("mark reverted of an unknown id", func(t *testing.T) {
		if err := s.MarkReverted(ctx, "chaos-nope-"+u, "x"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("want ErrNotFound, got %v", err)
		}
	})
}

func TestChaosEntryValidate(t *testing.T) {
	now := time.Now()
	ok := ChaosEntry{ID: "chaos-1", Host: "west", Kind: ChaosKindExperiment, Type: "network_delay",
		Target: json.RawMessage(`{}`), Recipe: json.RawMessage(`{}`), CreatedAt: now, ExpiresAt: now}
	if err := ok.validate(); err != nil {
		t.Fatal(err)
	}
	for name, mut := range map[string]func(*ChaosEntry){
		"no id":          func(e *ChaosEntry) { e.ID = "" },
		"no host":        func(e *ChaosEntry) { e.Host = "" },
		"unknown kind":   func(e *ChaosEntry) { e.Kind = "boom" },
		"no type":        func(e *ChaosEntry) { e.Type = "" },
		"recipe not obj": func(e *ChaosEntry) { e.Recipe = json.RawMessage(`"x"`) },
		"expires before": func(e *ChaosEntry) { e.ExpiresAt = now.Add(-time.Second) },
	} {
		e := ok
		mut(&e)
		if err := e.validate(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
