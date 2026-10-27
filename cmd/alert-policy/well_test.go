// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/database"
)

var repoRoot = filepath.Join("..", "..")

func TestWellPromDuration(t *testing.T) {
	for in, want := range map[string]time.Duration{
		"28d": 28 * 24 * time.Hour, "1w": 7 * 24 * time.Hour, "6h": 6 * time.Hour, "5m": 5 * time.Minute, "30s": 30 * time.Second,
	} {
		if got, err := promDuration(in); err != nil || got != want {
			t.Errorf("promDuration(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "28", "d", "0d", "1y", "-1d", "1.5d", "99999999999999d"} {
		if got, err := promDuration(in); err == nil {
			t.Errorf("promDuration(%q) = %v, want an error", in, got)
		}
	}
}

// The rows -sync-well writes are the policy's SLOs as the burn alerts use them.
func TestWellRows(t *testing.T) {
	p, err := loadPolicy(filepath.Join(repoRoot, policyPath))
	if err != nil {
		t.Fatal(err)
	}
	sha := strings.Repeat("0f", 32)
	rows, err := sloRows(p, sha)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != len(p.SLOs) || len(rows) < 7 {
		t.Fatalf("%d rows for %d SLOs", len(rows), len(p.SLOs))
	}
	by := map[string]database.SLODefinition{}
	for _, r := range rows {
		by[r.Name] = r
		if r.SourceSHA256 != sha || r.Window != 28*24*time.Hour || r.TotalExpr != "" || strings.Contains(r.GoodExpr, "\n") {
			t.Errorf("%s: %+v", r.Name, r)
		}
	}
	w := by["wotan"]
	if w.Objective != 0.995 || w.Urgency != "page" || w.Service != "wotan" || w.RunbookURL != runbookURL || w.GoodExpr != `avg(up{job="wotan"})` {
		t.Errorf("wotan row: %+v", w)
	}
	if b := by["well-backups"]; b.Urgency != "ticket" || b.Objective != 0.99 {
		t.Errorf("well-backups row: %+v", b)
	}
	if ci := by["ci-develop"]; ci.RunbookURL != repoBlob+"runbooks/observe/ci-red.yaml" {
		t.Errorf("ci-develop runbook: %q", ci.RunbookURL)
	}
}

func TestWellDrift(t *testing.T) {
	p, err := loadPolicy(filepath.Join(repoRoot, policyPath))
	if err != nil {
		t.Fatal(err)
	}
	yaml, err := sloRows(p, strings.Repeat("0f", 32))
	if err != nil {
		t.Fatal(err)
	}
	clone := func() []database.SLODefinition {
		out := append([]database.SLODefinition(nil), yaml...)
		for i := range out {
			out[i].SourceSHA256 = strings.Repeat("ee", 32) // provenance, not drift
			out[i].UpdatedAt = time.Now()
		}
		return out
	}
	if errs := driftErrors(yaml, clone()); len(errs) > 0 {
		t.Fatalf("equal sets drift: %v", errs)
	}
	idx := func(rows []database.SLODefinition, name string) int {
		for i, r := range rows {
			if r.Name == name {
				return i
			}
		}
		t.Fatalf("no %s", name)
		return -1
	}
	cases := map[string]struct {
		edit func([]database.SLODefinition) []database.SLODefinition
		want string
	}{
		"objective": {func(w []database.SLODefinition) []database.SLODefinition {
			w[idx(w, "wotan")].Objective = 0.99
			return w
		}, "drift: slo wotan objective well=0.99 yaml=0.995"},
		"window": {func(w []database.SLODefinition) []database.SLODefinition {
			w[idx(w, "wotan")].Window = 30 * 24 * time.Hour
			return w
		}, "drift: slo wotan window well=720h0m0s yaml=672h0m0s"},
		"urgency": {func(w []database.SLODefinition) []database.SLODefinition {
			w[idx(w, "dashboard")].Urgency = "none"
			return w
		}, `drift: slo dashboard urgency well="none" yaml="page"`},
		"good expr": {func(w []database.SLODefinition) []database.SLODefinition {
			w[idx(w, "wotan")].GoodExpr = "vector(1)"
			return w
		}, `drift: slo wotan sli_good_expr well="vector(1)" yaml="avg(up{job=\"wotan\"})"`},
		"total expr": {func(w []database.SLODefinition) []database.SLODefinition {
			w[idx(w, "wotan")].TotalExpr = "vector(1)"
			return w
		}, `drift: slo wotan sli_total_expr well="vector(1)" yaml=""`},
		"runbook": {func(w []database.SLODefinition) []database.SLODefinition {
			w[idx(w, "wotan")].RunbookURL = "https://example.invalid/x"
			return w
		}, `drift: slo wotan runbook_url well="https://example.invalid/x"`},
		"service": {func(w []database.SLODefinition) []database.SLODefinition {
			w[idx(w, "wotan")].Service = "other"
			return w
		}, `drift: slo wotan service well="other" yaml="wotan"`},
		"description": {func(w []database.SLODefinition) []database.SLODefinition {
			w[idx(w, "wotan")].Description = "other"
			return w
		}, `drift: slo wotan description well="other"`},
		"missing row": {func(w []database.SLODefinition) []database.SLODefinition {
			return append(w[:idx(w, "wotan")], w[idx(w, "wotan")+1:]...)
		}, "drift: slo wotan is in " + policyPath + ", not in the Well (run go run ./cmd/alert-policy -sync-well)"},
		"extra row": {func(w []database.SLODefinition) []database.SLODefinition {
			x := w[0]
			x.Name = "ghost"
			return append(w, x)
		}, "drift: slo ghost is in the Well, not in " + policyPath},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			errs := driftErrors(yaml, c.edit(clone()))
			if len(errs) != 1 || !strings.HasPrefix(errs[0].Error(), c.want) {
				t.Fatalf("want one error starting %q, got %v", c.want, errs)
			}
		})
	}
}

// -check with WELL_DB set and no usable Well fails; without WELL_DB it is
// YAML only, as before.
func TestWellCheckFailsClosed(t *testing.T) {
	t.Setenv("WELL_DB", "")
	if errs := runWellCheck(context.Background(), repoRoot); errs != nil {
		t.Fatalf("WELL_DB unset: want the YAML-only check, got %v", errs)
	}
	t.Setenv("WELL_DB", "unheaded_ops")
	t.Setenv("WELL_PASSWORD", "")
	errs := runWellCheck(context.Background(), repoRoot)
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "WELL_DB is set") {
		t.Fatalf("WELL_DB set without credentials: want one fail-closed error, got %v", errs)
	}
	if _, err := syncWellEnv(context.Background(), repoRoot); err == nil {
		t.Fatal("-sync-well without credentials: want an error")
	}
	t.Setenv("WELL_DB", "")
	if _, err := syncWellEnv(context.Background(), repoRoot); err == nil || !strings.Contains(err.Error(), "WELL_DB") {
		t.Fatalf("-sync-well without WELL_DB: want an error naming WELL_DB, got %v", err)
	}
}

// TestWellSyncAndCheck runs against a throwaway Well
// (scripts/well/test-slo-definitions.sh sets WELL_TEST_DSN to ops_writer on
// its own server). Never point it at the real Well.
func TestWellSyncAndCheck(t *testing.T) {
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
	p, err := loadPolicy(filepath.Join(repoRoot, policyPath))
	if err != nil {
		t.Fatal(err)
	}

	res, err := syncWell(ctx, db, repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(res.Inserted) + len(res.Updated) + len(res.Unchanged); n != len(p.SLOs) {
		t.Fatalf("sync touched %d SLOs, policy has %d: %+v", n, len(p.SLOs), res)
	}
	if errs := checkWell(ctx, db, repoRoot); len(errs) > 0 {
		t.Fatalf("after sync: %v", errs)
	}
	res, err = syncWell(ctx, db, repoRoot)
	if err != nil || len(res.Unchanged) != len(p.SLOs) {
		t.Fatalf("second sync: %+v, %v", res, err)
	}

	// Someone changes a row behind the policy's back.
	if _, err := db.ExecContext(ctx, `UPDATE slo_definitions SET objective = 0.99 WHERE name = 'wotan'`); err != nil {
		t.Fatal(err)
	}
	errs := checkWell(ctx, db, repoRoot)
	if len(errs) != 1 || errs[0].Error() != "drift: slo wotan objective well=0.99 yaml=0.995" {
		t.Fatalf("want the wotan drift, got %v", errs)
	}
	res, err = syncWell(ctx, db, repoRoot)
	if err != nil || !reflect.DeepEqual(res.Updated, []string{"wotan"}) {
		t.Fatalf("re-sync: %+v, %v", res, err)
	}
	if errs := checkWell(ctx, db, repoRoot); len(errs) > 0 {
		t.Fatalf("after re-sync: %v", errs)
	}
	var reason string
	if err := db.QueryRowContext(ctx, `SELECT change_reason FROM slo_definitions_history
		WHERE name = 'wotan' AND op = 'UPDATE' ORDER BY id DESC LIMIT 1`).Scan(&reason); err != nil ||
		!strings.HasPrefix(reason, "alert-policy -sync-well") {
		t.Fatalf("history reason = %q, %v", reason, err)
	}
}
