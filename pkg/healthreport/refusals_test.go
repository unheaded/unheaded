// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package healthreport

import (
	"fmt"
	"testing"
	"time"

	"unheaded/pkg/metrics"
)

// A flood of refusals is counted in full and logged once an hour per reason.
func TestRefusals_CountsEveryOneAndLogsOnceAnHour(t *testing.T) {
	lines := 0
	r, err := NewRefusals("test_refused_total", "test", metrics.NewRegistry(), func(string, string) { lines++ })
	if err != nil {
		t.Fatal(err)
	}
	at := time.Unix(1000, 0)
	r.now = func() time.Time { return at }
	for range 100 {
		r.Refuse("unverified", "")
	}
	r.Refuse("bad_node", "kanban-app")
	if got := r.WithLabelValues("unverified").Value(); got != 100 {
		t.Fatalf("counted %v, want 100", got)
	}
	if lines != 2 {
		t.Fatalf("%d log lines, want one per reason", lines)
	}
	at = at.Add(refusedQuiet)
	r.Refuse("unverified", "")
	if lines != 3 {
		t.Fatalf("%d log lines, want a third after the quiet hour", lines)
	}
}

func TestReason_NamesEachRefusalFromAFixedSet(t *testing.T) {
	for err, want := range map[error]string{
		ErrUnverified:                   "unverified",
		ErrNotOwnService:                "not_own_service",
		ErrBadNode:                      "bad_node",
		ErrBadReport:                    "malformed",
		fmt.Errorf("%w: x", ErrBadNode): "bad_node",
	} {
		if got := Reason(err); got != want {
			t.Errorf("Reason(%v) = %q, want %q", err, got, want)
		}
	}
}
