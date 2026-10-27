// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package sword

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

	"unheaded/pkg/database"
	"unheaded/pkg/deploy/cuirassdriver"
	"unheaded/pkg/deploy/strategy"
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

// newWellHistory gives each test a host of its own: the Well keeps every
// row ever written, and Load reads one host's records.
func newWellHistory(t *testing.T, db *sql.DB) History {
	t.Helper()
	h, err := NewWellHistory(context.Background(), db, fmt.Sprintf("t%d", time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// TestHistoryContract runs the same cases against both stores.
func TestHistoryContract(t *testing.T) {
	impls := map[string]func(t *testing.T) (History, func() History){
		"file": func(t *testing.T) (History, func() History) {
			dir := t.TempDir()
			return NewFileHistory(dir), func() History { return NewFileHistory(dir) }
		},
		"well": func(t *testing.T) (History, func() History) {
			db := wellTestDB(t)
			h := newWellHistory(t, db)
			host := h.(*wellHistory).host
			return h, func() History {
				h2, err := NewWellHistory(context.Background(), db, host)
				if err != nil {
					t.Fatal(err)
				}
				return h2
			}
		},
	}
	for name, mk := range impls {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			h, reopen := mk(t)
			if h.Mode() != name {
				t.Fatalf("mode %q", h.Mode())
			}
			if err := h.Ready(ctx); err != nil {
				t.Fatal(err)
			}
			got, err := h.Load(ctx)
			if err != nil || len(got) != 0 {
				t.Fatalf("empty load: %v %v", got, err)
			}
			u := time.Now().UnixNano()
			t0 := time.Now().UTC().Truncate(time.Millisecond)
			mkDep := func(n int, ver string, at time.Time) *Deployment {
				return &Deployment{ID: fmt.Sprintf("d%d-%d", u, n), Service: "steel-echo", Version: ver, Replicas: 3,
					Strategy: StrategySpec{Type: strategy.TypeRolling, MaxSurge: 1},
					Health:   &cuirassdriver.HealthSpec{Port: 8080, Path: "/health"},
					Labels:   map[string]string{"chaos": "allowed"},
					Status:   StatusPending, CreatedAt: at}
			}
			d1, d2 := mkDep(1, "v1", t0), mkDep(2, "v2", t0.Add(time.Second))
			var all []*Deployment
			all = append(all, d1)
			if err := h.Record(ctx, d1, all); err != nil {
				t.Fatal(err)
			}
			started := t0.Add(10 * time.Millisecond)
			d1.Status, d1.StartedAt = StatusInProgress, &started
			if err := h.Update(ctx, d1, StatusPending, all); err != nil {
				t.Fatal(err)
			}
			d1.PreviousVersion = "v0" // a change that is not a status change
			if err := h.Update(ctx, d1, StatusInProgress, all); err != nil {
				t.Fatal(err)
			}
			done := t0.Add(time.Second)
			d1.Status, d1.CompletedAt, d1.Message = StatusSucceeded, &done, "3/3 healthy"
			if err := h.Update(ctx, d1, StatusInProgress, all); err != nil {
				t.Fatal(err)
			}
			all = append(all, d2)
			if err := h.Record(ctx, d2, all); err != nil {
				t.Fatal(err)
			}

			got, err = reopen().Load(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 2 || got[0].ID != d1.ID || got[1].ID != d2.ID {
				t.Fatalf("load order: %+v", got)
			}
			want, _ := json.Marshal(d1)
			have, _ := json.Marshal(got[0])
			if string(want) != string(have) {
				t.Errorf("record did not round-trip:\n got %s\nwant %s", have, want)
			}
			if got[1].Status != StatusPending {
				t.Errorf("second record: %s", got[1].Status)
			}
		})
	}
}

// A rolling deploy against the fake Cuirass leaves one deploy_history row
// and every change after it as a transition.
func TestDeployRollingRecordsHistoryInWell(t *testing.T) {
	db := wellTestDB(t)
	h := newWellHistory(t, db)
	fx := newFixture(t, func(c *Config) { c.DataDir = ""; c.History = h })
	if fx.s.HistoryMode() != "well" {
		t.Fatalf("history mode %q", fx.s.HistoryMode())
	}
	d := fx.deploy(t, "v1", "rolling")
	got := fx.wait(t, d.ID)
	if got.Status != StatusSucceeded {
		t.Fatalf("deploy: %+v", got)
	}
	var rows, trans int
	if err := db.QueryRow(`SELECT count(*) FROM deploy_history WHERE deployment_id = $1`, d.ID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM deploy_history_transitions WHERE deployment_id = $1`, d.ID).Scan(&trans); err != nil {
		t.Fatal(err)
	}
	if rows != 1 || trans < 3 {
		t.Fatalf("deploy_history %d rows, transitions %d; want 1 and >= 3", rows, trans)
	}
	rec, err := database.NewDeployStore(db).Get(context.Background(), d.ID)
	if err != nil || rec.Status != string(StatusSucceeded) || rec.Strategy != "rolling" {
		t.Fatalf("stored: %+v %v", rec, err)
	}

	// A Sword that starts on the same Well sees it, and rolls back to it.
	s2, err := New(Config{History: newWellHistoryOn(t, db, h), AuthEnabled: true, Driver: fx.s.cfg.Driver, PollInterval: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if back, ok := s2.Get(d.ID); !ok || back.Status != StatusSucceeded {
		t.Fatalf("after restart: %+v %v", back, ok)
	}
}

func newWellHistoryOn(t *testing.T, db *sql.DB, h History) History {
	t.Helper()
	h2, err := NewWellHistory(context.Background(), db, h.(*wellHistory).host)
	if err != nil {
		t.Fatal(err)
	}
	return h2
}

// Switching an existing Sword to the Well brings its file history along
// once, so the previous success is still there to roll back to.
func TestWellHistoryImportsTheFileOnce(t *testing.T) {
	db := wellTestDB(t)
	dir := t.TempDir()
	u := time.Now().UnixNano()
	st := stateFile{Version: 1, Deployments: []*Deployment{
		{ID: fmt.Sprintf("d%d-1", u), Service: "steel-echo", Version: "v1", Replicas: 1,
			Strategy: StrategySpec{Type: strategy.TypeRecreate}, Status: StatusSucceeded, CreatedAt: time.Now().UTC()},
	}}
	b, _ := json.Marshal(st)
	if err := os.WriteFile(filepath.Join(dir, stateFileName), b, 0o600); err != nil {
		t.Fatal(err)
	}
	h := newWellHistory(t, db)
	s, err := New(Config{DataDir: dir, History: h, AuthEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	if _, ok := s.Get(st.Deployments[0].ID); !ok {
		t.Fatal("file record not imported")
	}
	// A second start finds the Well non-empty and imports nothing again.
	s2, err := New(Config{DataDir: dir, History: newWellHistoryOn(t, db, h), AuthEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	s2.Close()
	if n := len(s2.List()); n != 1 {
		t.Fatalf("after a second start: %d records", n)
	}
}
