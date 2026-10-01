// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package well

import (
	"context"
	"database/sql"
	"os"
	"slices"
	"testing"
	"time"
)

func counts(t *testing.T, ctx context.Context, cfg, ops *sql.DB) map[string]int {
	t.Helper()
	out := map[string]int{}
	for _, q := range []struct {
		db    *sql.DB
		table string
	}{{cfg, "compliance_fixes"}, {cfg, "compliance_controls"}, {cfg, "compliance_frameworks"}, {cfg, "compliance_profiles"},
		{cfg, "compliance_host_bindings"}, {cfg, "compliance_host_bindings_history"}, {ops, "compliance_findings"}} {
		var n int
		if err := q.db.QueryRowContext(ctx, "SELECT count(*) FROM "+q.table).Scan(&n); err != nil { // #nosec G202 -- fixed table names in this test
			t.Fatal(err)
		}
		out[q.table] = n
	}
	return out
}

// Integration against the throwaway databases (both DSNs from
// scripts/well/test-compliance-schema.sh --go). It may run after other
// tests have left rows behind, so it asserts on differences only.
func TestIntegrationSync(t *testing.T) {
	cdsn, odsn := os.Getenv("WELL_TEST_CONFIG_DSN"), os.Getenv("WELL_TEST_OPS_DSN")
	if cdsn == "" || odsn == "" {
		t.Skip("WELL_TEST_CONFIG_DSN / WELL_TEST_OPS_DSN not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cfg, err := sql.Open("postgres", cdsn)
	if err != nil {
		t.Fatal(err)
	}
	defer cfg.Close()
	ops, err := sql.Open("postgres", odsn)
	if err != nil {
		t.Fatal(err)
	}
	defer ops.Close()

	before := counts(t, ctx, cfg, ops)
	dry, err := Sync(ctx, repo, cfg, ops, "test", true)
	if err != nil {
		t.Fatal(err)
	}
	if after := counts(t, ctx, cfg, ops); !mapsEqual(before, after) {
		t.Fatalf("dry run wrote: %v -> %v", before, after)
	}
	if _, err := Sync(ctx, repo, cfg, ops, "test", false); err != nil {
		t.Fatal(err)
	}
	if got := counts(t, ctx, cfg, ops); got["compliance_fixes"] < 10 || got["compliance_controls"] < 42 || got["compliance_frameworks"] < 11 {
		t.Errorf("after sync: %v (dry run planned %v)", got, dry.Created)
	}
	again, err := Sync(ctx, repo, cfg, ops, "test", false)
	if err != nil {
		t.Fatal(err)
	}
	if again.Changed() {
		t.Errorf("second sync changed things: created %v updated %v", again.Created, again.Updated)
	}

	// The Well owns state: a switched binding is reported, never reverted.
	m, err := LoadModel(repo)
	if err != nil {
		t.Fatal(err)
	}
	s := NewPGStore(cfg, m)
	if err := s.SetBinding(ctx, Change{Host: "west", Profile: "none", Mode: "audit", Actor: "test", Reason: "diverge on purpose"}); err != nil {
		t.Fatal(err)
	}
	// The repo authors definitions: a mirror row that drifted is restored.
	if _, err := cfg.ExecContext(ctx, `UPDATE compliance_fixes SET sha256 = repeat('0', 64) WHERE id = 'kmod-cis'`); err != nil {
		t.Fatal(err)
	}
	third, err := Sync(ctx, repo, cfg, ops, "test", false)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(third.Diverged, "binding:west") {
		t.Errorf("diverged %v, want binding:west", third.Diverged)
	}
	var profile string
	if err := cfg.QueryRowContext(ctx, `SELECT profile FROM compliance_host_bindings WHERE host = 'west'`).Scan(&profile); err != nil || profile != "none" {
		t.Errorf("sync overwrote The Well's binding: %q %v", profile, err)
	}
	if third.Updated["compliance_fixes"] != 1 {
		t.Errorf("drifted fix mirror: updated %v, want 1", third.Updated)
	}
	if len(third.Orphaned) != 0 {
		t.Errorf("orphaned %v", third.Orphaned)
	}
}

func mapsEqual(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
