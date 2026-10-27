// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package database

import (
	"context"
	"database/sql"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSLOStoreIntervalText(t *testing.T) {
	for d, want := range map[time.Duration]string{
		28 * 24 * time.Hour: "28 days",
		7 * 24 * time.Hour:  "7 days",
		6 * time.Hour:       "6 hours",
		30 * time.Minute:    "30 minutes",
		90 * time.Second:    "90 seconds",
	} {
		got, err := intervalText(d)
		if err != nil || got != want {
			t.Errorf("intervalText(%v) = %q, %v; want %q", d, got, err, want)
		}
	}
	for _, d := range []time.Duration{0, -time.Hour, 1500 * time.Millisecond} {
		if got, err := intervalText(d); err == nil {
			t.Errorf("intervalText(%v) = %q, want an error", d, got)
		}
	}
}

func validSLO(name string) SLODefinition {
	return SLODefinition{
		Name: name, Service: "svc-" + name, Description: "desc " + name,
		Objective: 0.995, Window: 28 * 24 * time.Hour, GoodExpr: `avg(up{job="` + name + `"})`,
		Urgency: "page", RunbookURL: "https://example.invalid/runbooks/observe/x.yaml",
		SourceSHA256: strings.Repeat("ab", 32),
	}
}

func TestSLOStoreValidate(t *testing.T) {
	if err := validSLO("wotan").validate(); err != nil {
		t.Fatalf("valid definition refused: %v", err)
	}
	bad := map[string]func(*SLODefinition){
		"name":          func(d *SLODefinition) { d.Name = "Bad Name" },
		"objective 1":   func(d *SLODefinition) { d.Objective = 1 },
		"objective 0":   func(d *SLODefinition) { d.Objective = 0 },
		"five places":   func(d *SLODefinition) { d.Objective = 0.99995 },
		"window":        func(d *SLODefinition) { d.Window = 0 },
		"urgency":       func(d *SLODefinition) { d.Urgency = "sometimes" },
		"good expr":     func(d *SLODefinition) { d.GoodExpr = " " },
		"runbook":       func(d *SLODefinition) { d.RunbookURL = "" },
		"sha256":        func(d *SLODefinition) { d.SourceSHA256 = "abc" },
		"service":       func(d *SLODefinition) { d.Service = "" },
		"description":   func(d *SLODefinition) { d.Description = "" },
		"long good":     func(d *SLODefinition) { d.GoodExpr = strings.Repeat("x", 4097) },
		"long runbook":  func(d *SLODefinition) { d.RunbookURL = strings.Repeat("x", 513) },
		"upper sha256":  func(d *SLODefinition) { d.SourceSHA256 = strings.Repeat("AB", 32) },
		"bad total sli": func(d *SLODefinition) { d.TotalExpr = strings.Repeat("x", 4097) },
	}
	for name, edit := range bad {
		d := validSLO("wotan")
		edit(&d)
		if err := d.validate(); err == nil {
			t.Errorf("%s: accepted %+v", name, d)
		}
	}
}

func TestSLOStoreSyncRefusesEmptyAndDuplicates(t *testing.T) {
	s := NewSLOStore(nil) // never reaches the database
	if _, err := s.Sync(context.Background(), nil, "x"); err == nil {
		t.Error("Sync with no SLOs would delete every row: want an error")
	}
	if _, err := s.Sync(context.Background(), []SLODefinition{validSLO("a"), validSLO("a")}, "x"); err == nil {
		t.Error("duplicate names accepted")
	}
	bad := validSLO("b")
	bad.Objective = 2
	if _, err := s.Sync(context.Background(), []SLODefinition{validSLO("a"), bad}, "x"); err == nil {
		t.Error("an invalid definition accepted")
	}
}

// TestSLOStoreWell runs against a throwaway Well (scripts/well/test-slo-definitions.sh
// sets WELL_TEST_DSN to ops_writer on its own server). Never point it at the real Well.
func TestSLOStoreWell(t *testing.T) {
	dsn := os.Getenv("WELL_TEST_DSN")
	if dsn == "" {
		t.Skip("WELL_TEST_DSN unset (run scripts/well/test-slo-definitions.sh)")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	s := NewSLOStore(db)

	a, b, c := validSLO("store-a"), validSLO("store-b"), validSLO("store-c")
	c.TotalExpr = "sum(rate(x[5m]))"
	c.Urgency = "ticket"
	c.Window = 6 * time.Hour
	res, err := s.Sync(ctx, []SLODefinition{a, b, c}, "first sync")
	if err != nil {
		t.Fatal(err)
	}
	// Whatever rows an earlier test left are deleted: the set is the YAML's.
	if !reflect.DeepEqual(res.Inserted, []string{"store-a", "store-b", "store-c"}) {
		t.Fatalf("first sync inserted %v", res.Inserted)
	}
	got, err := s.All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("All = %d rows, want 3: %+v", len(got), got)
	}
	for i, want := range []SLODefinition{a, b, c} {
		g := got[i]
		if g.UpdatedAt.IsZero() {
			t.Errorf("%s: updated_at not read", g.Name)
		}
		g.UpdatedAt = time.Time{}
		if !reflect.DeepEqual(g, want) {
			t.Errorf("row %d:\n got %+v\nwant %+v", i, g, want)
		}
	}

	// Same definitions under a new policy sha256: nothing changes.
	a2, b2, c2 := a, b, c
	for _, d := range []*SLODefinition{&a2, &b2, &c2} {
		d.SourceSHA256 = strings.Repeat("cd", 32)
	}
	res, err = s.Sync(ctx, []SLODefinition{a2, b2, c2}, "no-op")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Unchanged) != 3 || len(res.Inserted)+len(res.Updated)+len(res.Deleted) != 0 {
		t.Fatalf("no-op sync: %+v", res)
	}

	// One objective changes, one SLO leaves the YAML.
	a2.Objective = 0.999
	res, err = s.Sync(ctx, []SLODefinition{a2, c2}, "tighten store-a")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res, SLOSyncResult{Updated: []string{"store-a"}, Deleted: []string{"store-b"}, Unchanged: []string{"store-c"}}) {
		t.Fatalf("second sync: %+v", res)
	}
	got, err = s.All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Objective != 0.999 || got[0].SourceSHA256 != a2.SourceSHA256 || got[1].SourceSHA256 != c.SourceSHA256 {
		t.Fatalf("after second sync: %+v", got)
	}
	var reason string
	if err := db.QueryRowContext(ctx, `SELECT change_reason FROM slo_definitions_history
		WHERE name = 'store-a' AND op = 'UPDATE' ORDER BY id DESC LIMIT 1`).Scan(&reason); err != nil || reason != "tighten store-a" {
		t.Fatalf("history reason = %q, %v", reason, err)
	}
}
