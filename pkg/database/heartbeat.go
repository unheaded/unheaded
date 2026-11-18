// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package database

import (
	"sync"
	"time"

	"unheaded/pkg/metrics/auto"
	"unheaded/pkg/metrics/prom"
)

// Every health report used to be a commit: 0.68 a second, 73% of the
// Well's WAL flushes (ADR-147, Erda F3). The gate lets a report through
// when its status changed (a transition, committed synchronously) or as a
// heartbeat heartbeatInterval after the row's last write (committed without
// waiting for the WAL flush); every other report is skipped. last_check is
// therefore up to 5 minutes old for a service whose status has not changed.
const (
	heartbeatInterval = 5 * time.Minute
	// heartbeatMaxRows bounds the gate: one entry per (service, reporter).
	heartbeatMaxRows = 4096
)

var (
	healthReportsSkipped = auto.NewCounter(prom.CounterOpts{
		Name: "unheaded_well_health_reports_skipped_total",
		Help: "Health reports not written to the Well: same status as the row's last write, under 5 minutes ago",
	})
	heartbeatRowsFull = auto.NewCounter(prom.CounterOpts{
		Name: "unheaded_well_health_heartbeat_rows_full_total",
		Help: "Health reports written because the heartbeat gate was full (4096 rows) and could not remember their row",
	})
)

// heartbeatDecision is what the gate says about one report.
type heartbeatDecision struct {
	write bool
	// heartbeatOnly: the status is the row's last written one; the write
	// only refreshes last_check and may commit asynchronously.
	heartbeatOnly bool
}

type heartbeatKey struct{ service, reporter string }

type heartbeatEntry struct {
	status string
	at     time.Time
}

// heartbeatGate remembers each row's last written status and time.
type heartbeatGate struct {
	mu   sync.Mutex
	now  func() time.Time
	last map[heartbeatKey]heartbeatEntry
}

func newHeartbeatGate(now func() time.Time) *heartbeatGate {
	if now == nil {
		now = time.Now
	}
	return &heartbeatGate{now: now, last: make(map[heartbeatKey]heartbeatEntry)}
}

// decide says whether a report with this status is written.
func (g *heartbeatGate) decide(service, reporter, status string) heartbeatDecision {
	g.mu.Lock()
	defer g.mu.Unlock()
	e, ok := g.last[heartbeatKey{service, reporter}]
	switch {
	case !ok:
		if len(g.last) >= heartbeatMaxRows {
			heartbeatRowsFull.Inc()
		}
		return heartbeatDecision{write: true}
	case e.status != status:
		return heartbeatDecision{write: true}
	case g.now().Sub(e.at) >= heartbeatInterval:
		return heartbeatDecision{write: true, heartbeatOnly: true}
	default:
		healthReportsSkipped.Inc()
		return heartbeatDecision{}
	}
}

// record notes a committed write. A row the full gate cannot hold is not
// remembered (so it writes every time and is counted).
func (g *heartbeatGate) record(service, reporter, status string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	k := heartbeatKey{service, reporter}
	if _, ok := g.last[k]; !ok && len(g.last) >= heartbeatMaxRows {
		return
	}
	g.last[k] = heartbeatEntry{status: status, at: g.now()}
}

// rows is how many rows the gate remembers.
func (g *heartbeatGate) rows() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.last)
}

// writePrelude is what a write's transaction runs first: a heartbeat-only
// write commits without waiting for the WAL flush (a crash may lose the
// last few hundred milliseconds of last_check, never a transition).
func writePrelude(d heartbeatDecision) []string {
	if d.heartbeatOnly {
		return []string{"SET LOCAL synchronous_commit = off"}
	}
	return nil
}
