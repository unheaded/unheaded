// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package yaldabaoth

import (
	"context"
	"testing"
	"time"

	wotanClient "unheaded/pkg/wotan-client"
)

// Step 284 (BlackMage) findings, fixed with tests.

// A request cannot make the controller fan out unbounded probe traffic.
func TestReview_ProbeCountIsBounded(t *testing.T) {
	fx := newFixture(t)
	e := delayExp("steel-echo-v2-0", time.Second)
	e.SteadyState = &SteadyState{}
	for i := 0; i <= maxProbes; i++ {
		e.SteadyState.Probes = append(e.SteadyState.Probes, Probe{Name: "p", Port: 8080, Path: "/health"})
	}
	if _, err := fx.s.StartExperiment(context.Background(), e); !isRefusal(err, RefuseInvalid) {
		t.Fatalf("%d probes: %v", len(e.SteadyState.Probes), err)
	}
}

// Finished experiments do not accumulate without bound.
func TestReview_HistoryIsBounded(t *testing.T) {
	fx := newFixture(t, func(c *Config, _ *Deps) { c.DryRunMode = true })
	for i := 0; i < maxHistory+5; i++ {
		e := fx.start(t, delayExp("steel-echo-v2-0", time.Second))
		fx.wait(t, e.ID)
	}
	if n := len(fx.s.List()); n > maxHistory {
		t.Fatalf("history holds %d experiments, cap %d", n, maxHistory)
	}
}

// Wotan's HTTP polling starts from seq 0, so every resubscribe replays the
// topic's history: an abort already handled, or one from long before this
// controller started, must not abort the experiments running now.
func TestReview_WotanAbortReplayIgnored(t *testing.T) {
	fx := newFixture(t)
	old := &wotanClient.Message{MessageID: "m-old", Topic: TopicAbort, CreatedAt: time.Now().Add(-time.Hour), Payload: `{"reason":"last week"}`}
	fx.s.HandleAbortMessage(old) // at subscribe time: nothing runs
	exp := fx.start(t, delayExp("steel-echo-v2-0", 300*time.Millisecond))
	waitFor(t, "injection", func() bool { return fx.inj.activeCount() == 1 })
	fx.s.HandleAbortMessage(old) // replayed by a resubscribe
	fx.s.HandleAbortMessage(&wotanClient.Message{MessageID: "m-older", Topic: TopicAbort, CreatedAt: time.Now().Add(-time.Hour)})
	if e := fx.wait(t, exp.ID); e.Status != StatusCompleted {
		t.Fatalf("a replayed abort stopped the run: %s %+v", e.Status, e.Results)
	}
	// A fresh one still aborts, and so does one without an ID or a time.
	for _, m := range []*wotanClient.Message{
		{MessageID: "m-new", Topic: TopicAbort, CreatedAt: time.Now()},
		{Topic: TopicAbort},
	} {
		exp := fx.start(t, delayExp("steel-echo-v2-0", 10*time.Second))
		waitFor(t, "injection", func() bool { return fx.inj.activeCount() == 1 })
		fx.s.HandleAbortMessage(m)
		if e := fx.wait(t, exp.ID); e.Status != StatusAborted {
			t.Fatalf("fresh abort %+v ignored: %s", m, e.Status)
		}
	}
}

// revert-all needs only the journal and the injectors: a controller whose
// allowlist or guard is gone must still be able to take its faults back.
func TestReview_RevertNeedsNoAllowlist(t *testing.T) {
	cfg := DefaultConfig()
	cfg.StateDir = t.TempDir()
	inj := newFakeInjector()
	s, err := NewService(cfg, Deps{})
	if err != nil {
		t.Fatal(err)
	}
	s.RegisterInjector(ExpTypeNetworkDelay, inj)
	if err := s.journal.add(&JournalEntry{ID: "x", Type: ExpTypeNetworkDelay, Recipe: Recipe{"fault": "f"}, Expires: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	inj.active["f"] = true
	if left, err := s.RevertAll(context.Background()); left != 0 || err != nil || inj.activeCount() != 0 {
		t.Fatalf("revert without an allowlist: left %d err %v active %d", left, err, inj.activeCount())
	}
}
