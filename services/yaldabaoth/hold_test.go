// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package yaldabaoth

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeHold is a critical-alert state (pkg/critalert's Tracker in the binary).
type fakeHold struct {
	mu   sync.Mutex
	held bool
	why  string
}

func (f *fakeHold) Hold() (bool, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.held, f.why
}

func (f *fakeHold) set(held bool, why string) {
	f.mu.Lock()
	f.held, f.why = held, why
	f.mu.Unlock()
}

// TestExperimentRefusedWhileCritical (ADR-147 Phase 8, D2 = B): no
// experiment starts while a critical alert fires; one already running is
// not stopped; an unknown state (held false) allows.
func TestExperimentRefusedWhileCritical(t *testing.T) {
	h := &fakeHold{}
	fx := newFixture(t, func(_ *Config, d *Deps) { d.Hold = h })

	running := fx.start(t, delayExp("steel-echo-v2-0", 300*time.Millisecond))
	h.set(true, "critical alert firing: WotanDown")
	before := fx.s.refusals.WithLabelValues(string(RefuseCriticalAlert)).Value()
	_, err := fx.s.StartExperiment(context.Background(), delayExp("steel-echo-v2-0", time.Second))
	if !isRefusal(err, RefuseCriticalAlert) || !strings.Contains(err.Error(), "WotanDown") {
		t.Fatalf("while critical: %v, want a critical_alert refusal naming the alert", err)
	}
	if got := fx.s.refusals.WithLabelValues(string(RefuseCriticalAlert)).Value() - before; got != 1 {
		t.Errorf(`chaos_refusals_total{reason="critical_alert"} rose %v, want 1`, got)
	}
	if e := fx.wait(t, running.ID); e.Status != StatusCompleted {
		t.Errorf("the running experiment ended %s; a hold does not stop it", e.Status)
	}

	h.set(false, "") // ok, or unknown: allowed
	exp := fx.start(t, delayExp("steel-echo-v2-0", 50*time.Millisecond))
	fx.wait(t, exp.ID)
}
