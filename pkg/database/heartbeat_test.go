// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package database

import (
	"fmt"
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time      { return c.t }
func (c *fakeClock) add(d time.Duration) { c.t = c.t.Add(d) }
func newTestGate() (*heartbeatGate, *fakeClock) {
	c := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	return newHeartbeatGate(c.now), c
}

// decideAndRecord is what HandleHealthReport does on a successful write.
func decideAndRecord(g *heartbeatGate, svc, rep, status string) heartbeatDecision {
	d := g.decide(svc, rep, status)
	if d.write {
		g.record(svc, rep, status)
	}
	return d
}

// ADR-147, Erda F3: every health report was a commit (0.68 a second, 73%
// of the WAL flushes). A report writes when its status changed, or as a
// heartbeat 5 minutes after the last write; otherwise it is skipped.
func TestHeartbeatWritesOnChange(t *testing.T) {
	g, c := newTestGate()
	if d := decideAndRecord(g, "wotan", "akira@west", "healthy"); !d.write || d.heartbeatOnly {
		t.Fatalf("first report: %+v, want a synchronous write", d)
	}
	c.add(10 * time.Second)
	if d := decideAndRecord(g, "wotan", "akira@west", "healthy"); d.write {
		t.Fatalf("same status 10 s later: %+v, want skip", d)
	}
	c.add(10 * time.Second)
	if d := decideAndRecord(g, "wotan", "akira@west", "unhealthy"); !d.write || d.heartbeatOnly {
		t.Fatalf("status changed: %+v, want a synchronous write", d)
	}
	c.add(5*time.Minute - time.Second)
	if d := decideAndRecord(g, "wotan", "akira@west", "unhealthy"); d.write {
		t.Fatalf("same status 4 m 59 s after the last write: %+v, want skip", d)
	}
	c.add(2 * time.Second)
	if d := decideAndRecord(g, "wotan", "akira@west", "unhealthy"); !d.write || !d.heartbeatOnly {
		t.Fatalf("same status 5 m 1 s after the last write: %+v, want a heartbeat write", d)
	}
	// A failed write is not recorded: the next report tries again.
	c.add(time.Second)
	if d := g.decide("wotan", "akira@west", "healthy"); !d.write {
		t.Fatal("change not written")
	}
	if d := g.decide("wotan", "akira@west", "healthy"); !d.write {
		t.Fatal("a change whose write was not recorded was skipped")
	}
}

// Each (service, reporter) is its own row with its own clock.
func TestHeartbeatPerRow(t *testing.T) {
	g, c := newTestGate()
	decideAndRecord(g, "wotan", "akira@west", "healthy")
	c.add(3 * time.Minute)
	if d := decideAndRecord(g, "wotan", "wotan@west", "healthy"); !d.write {
		t.Fatal("second reporter's first report skipped")
	}
	c.add(2*time.Minute + time.Second)
	if d := decideAndRecord(g, "wotan", "akira@west", "healthy"); !d.write || !d.heartbeatOnly {
		t.Errorf("first row at 5 m 1 s: %+v, want heartbeat", d)
	}
	if d := decideAndRecord(g, "wotan", "wotan@west", "healthy"); d.write {
		t.Errorf("second row at 2 m 1 s: %+v, want skip", d)
	}
}

// The gate is bounded: past its cap a new row is not remembered and always
// writes, and the counter says so.
func TestHeartbeatBounded(t *testing.T) {
	g, _ := newTestGate()
	for i := 0; i < heartbeatMaxRows; i++ {
		decideAndRecord(g, fmt.Sprintf("svc-%d", i), "akira@west", "healthy")
	}
	before := heartbeatRowsFull.Value()
	for i := 0; i < 3; i++ {
		if d := decideAndRecord(g, "one-too-many", "akira@west", "healthy"); !d.write {
			t.Fatalf("report %d past the cap skipped", i)
		}
	}
	if got := heartbeatRowsFull.Value() - before; got != 3 {
		t.Errorf("rows-full counter rose %v, want 3", got)
	}
	if n := g.rows(); n != heartbeatMaxRows {
		t.Errorf("gate holds %d rows, want the cap %d", n, heartbeatMaxRows)
	}
}

// A heartbeat-only write commits without waiting for the WAL flush; a
// transition (or a first write) stays synchronous.
func TestHeartbeatOnlyIsAsync(t *testing.T) {
	hb := writePrelude(heartbeatDecision{write: true, heartbeatOnly: true})
	if len(hb) == 0 || hb[0] != "SET LOCAL synchronous_commit = off" {
		t.Errorf("heartbeat prelude = %q, want SET LOCAL synchronous_commit = off first", hb)
	}
	if tr := writePrelude(heartbeatDecision{write: true}); len(tr) != 0 {
		t.Errorf("transition prelude = %q, want none", tr)
	}
}
