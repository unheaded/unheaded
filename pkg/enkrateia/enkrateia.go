// SPDX-License-Identifier: GPL-3.0-or-later
// Package enkrateia is the v1 alerts-only drift event aggregator for
// Mímir's Law / Gleipnir Phase 0 PoC.
//
// HARD CONDITION (ADR-043 #1): NO AUTO-RESTORE in v1.
// This package MUST NOT call any file write, unlink, rename, or restore
// syscall. Drift events are observed and emitted as alerts ONLY.
// Auto-restore is deferred to v2 contingent on LICH-012 clearance.
package enkrateia

import (
	"sync"
	"time"
)

// DriftEvent is a baseline drift observation from Heimdall.
type DriftEvent struct {
	NodeID       string
	Path         string
	HashActual   []byte
	HashExpected []byte
	Severity     string // "info" | "warn" | "alert"
	DetectedAt   time.Time
}

// Alert is what Enkrateia emits when drift is detected.
// Note: NEVER contains restore actions in v1.
type Alert struct {
	Event    DriftEvent
	Message  string
	EmittedAt time.Time
}

// Aggregator collects drift events and emits alerts.
// Stateless except for an in-memory event channel.
type Aggregator struct {
	mu     sync.Mutex
	alerts chan Alert
}

// NewAggregator creates a v1 alerts-only aggregator.
func NewAggregator(bufferSize int) *Aggregator {
	return &Aggregator{
		alerts: make(chan Alert, bufferSize),
	}
}

// HandleDrift converts a drift event into an alert.
// VERIFIED: zero file system mutations. ADR-043 hard condition #1.
func (a *Aggregator) HandleDrift(event DriftEvent) {
	a.mu.Lock()
	defer a.mu.Unlock()

	alert := Alert{
		Event:     event,
		Message:   "drift detected on " + event.NodeID + " path=" + event.Path,
		EmittedAt: time.Now(),
	}

	select {
	case a.alerts <- alert:
	default:
		// Buffer full — drop alert (v1 PoC, no persistence)
	}
}

// Alerts returns the read-only alert channel.
func (a *Aggregator) Alerts() <-chan Alert {
	return a.alerts
}
