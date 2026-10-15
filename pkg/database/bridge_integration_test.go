// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// Integration: a throwaway unheaded_ops (004, 015, 024) as ops_writer,
// from scripts/well/test-compliance-schema.sh --go. Never The Well.
func TestIntegrationHealthBridge(t *testing.T) {
	dsn := os.Getenv("WELL_TEST_OPS_DSN")
	if dsn == "" {
		t.Skip("WELL_TEST_OPS_DSN not set")
	}
	ctx := context.Background()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	b := NewWotanBridge(db)
	t0 := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	send := func(healthy bool, at time.Time, errText string) error {
		p, _ := json.Marshal(HealthReport{Service: "bridge-test", Reporter: "west", Healthy: healthy,
			Latency: 42 * time.Millisecond, Error: errText, Timestamp: at})
		return b.HandleHealthReport(ctx, p)
	}
	state := func() (status string, last time.Time) {
		if err := db.QueryRowContext(ctx, `SELECT status, last_check FROM service_health_current WHERE service_name='bridge-test' AND host='west'`).Scan(&status, &last); err != nil {
			t.Fatal(err)
		}
		return status, last.UTC()
	}
	transitions := func() int {
		var n int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM service_health_transitions WHERE service_name='bridge-test'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	for _, step := range []struct {
		healthy bool
		at      time.Time
		err     string
		status  string
		trans   int
	}{
		{true, t0, "", "healthy", 0},                                           // first report: state, no transition
		{true, t0.Add(time.Minute), "", "healthy", 0},                          // same status: no transition
		{false, t0.Add(2 * time.Minute), "connection refused", "unhealthy", 1}, // change: one transition
		{true, t0.Add(90 * time.Second), "", "unhealthy", 1},                   // older than stored: ignored
		{false, t0.Add(2 * time.Minute), "connection refused", "unhealthy", 1}, // replay of the same: ignored
		{true, t0.Add(3 * time.Minute), "", "healthy", 2},                      // recovery: second transition
	} {
		if err := send(step.healthy, step.at, step.err); err != nil {
			t.Fatalf("at %v: %v", step.at, err)
		}
		if st, _ := state(); st != step.status || transitions() != step.trans {
			t.Fatalf("at %v: status %s, %d transitions; want %s, %d", step.at, st, transitions(), step.status, step.trans)
		}
	}
	if _, last := state(); !last.Equal(t0.Add(3 * time.Minute)) {
		t.Errorf("last_check %v is not the newest report's time", last)
	}
	h, err := b.QueryHealthHistory(ctx, "bridge-test", 10)
	if err != nil || len(h) != 2 || h[0].NewStatus != "healthy" || h[1].NewStatus != "unhealthy" || !h[1].TransitionAt.Equal(t0.Add(2*time.Minute)) {
		t.Errorf("history %+v %v", h, err)
	}
	for _, bad := range []string{`{}`, `not json`, `{"service":"x","reporter":"west"}`} {
		if err := b.HandleHealthReport(ctx, json.RawMessage(bad)); !errors.Is(err, ErrBadReport) {
			t.Errorf("%s: %v", bad, err)
		}
	}
	var hourly bool
	if err := db.QueryRowContext(ctx, `SELECT to_regclass('service_health_hourly') IS NULL`).Scan(&hourly); err != nil || !hourly {
		t.Errorf("service_health_hourly still exists (024 not applied?)")
	}
}
