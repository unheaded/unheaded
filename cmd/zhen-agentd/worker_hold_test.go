// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"strings"
	"testing"

	"unheaded/pkg/zhenagent"
)

// ADR-147 D2 = B (S-WASTE-OUT Step 143, plan 3's handoff): while a critical
// alert fires, an unattended run does not start. The card is deferred (no
// claim, no task), and the next poll after the alert clears runs it.
func TestWorker_DefersWhileCriticalAlertFires(t *testing.T) {
	m := zhenagent.NewMemStore()
	m.AddCard(optedIn(zhenagent.Card{ID: "good", Title: "x", Description: goodDesc, Status: "todo", Type: "task", Owner: "zhen"}))
	r := &fakeRunner{write: true, out: passOutcome()}
	w, _, _, _ := newTestWorker(t, m, r)
	firing := true
	w.cfg.Hold = func() (bool, string) {
		if firing {
			return true, "critical alert firing: WellDown"
		}
		return false, ""
	}
	res, err := w.runOnce(context.Background())
	if err != nil || res.Outcome != outcomeDeferred || len(r.tasks) != 0 {
		t.Fatalf("res %+v err %v tasks %d", res, err, len(r.tasks))
	}
	if !strings.Contains(res.Reason, "critical alert firing: WellDown") {
		t.Errorf("reason %q does not name the alert", res.Reason)
	}
	if n, _ := m.Attempts(context.Background(), "good"); n != 0 {
		t.Errorf("a held run claimed the card (%d runs)", n)
	}
	firing = false
	if res, _ := w.runOnce(context.Background()); res.Outcome != outcomeSucceeded {
		t.Errorf("after the alert clears: %+v", res)
	}
}

// A nil hook (no Wotan client, so the alert state is unknown) never holds:
// unknown allows, as for Sword and chaos-controller.
func TestWorker_NoHoldHookRuns(t *testing.T) {
	m := zhenagent.NewMemStore()
	m.AddCard(optedIn(zhenagent.Card{ID: "good", Title: "x", Description: goodDesc, Status: "todo", Type: "task", Owner: "zhen"}))
	r := &fakeRunner{write: true, out: passOutcome()}
	w, _, _, _ := newTestWorker(t, m, r)
	w.cfg.Hold = nil
	if res, err := w.runOnce(context.Background()); err != nil || res.Outcome != outcomeSucceeded {
		t.Fatalf("res %+v err %v", res, err)
	}
}
