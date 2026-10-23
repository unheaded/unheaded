// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"fmt"
	"testing"

	"unheaded/pkg/gpulease"
	"unheaded/pkg/zhenagent"
)

// S-TWO-BRAINS Step 233: the worker shares the GPU with forge training.
// Finding the lease held exclusively, it defers the card (no claim, no
// Claude escalation because the GPU is busy) and retries next poll.

func TestWorker_DefersWhenGPUBusy(t *testing.T) {
	m := zhenagent.NewMemStore()
	m.AddCard(zhenagent.Card{ID: "good", Title: "x", Description: goodDesc, Status: "todo", Type: "task", Owner: "zhen"})
	r := &fakeRunner{write: true, out: passOutcome()}
	w, _, _, _ := newTestWorker(t, m, r)
	busy := true
	w.cfg.GPUBusy = func() bool { return busy }
	res, err := w.runOnce(context.Background())
	if err != nil || res.Outcome != outcomeDeferred || len(r.tasks) != 0 {
		t.Fatalf("res %+v err %v tasks %d", res, err, len(r.tasks))
	}
	if n, _ := m.Attempts(context.Background(), "good"); n != 0 {
		t.Errorf("a deferral claimed the card (%d runs)", n)
	}
	busy = false
	if res, _ := w.runOnce(context.Background()); res.Outcome != outcomeSucceeded {
		t.Errorf("after the lease is free: %+v", res)
	}
}

func TestWorker_GPUBusyMidCardDefers(t *testing.T) {
	m := zhenagent.NewMemStore()
	m.AddCard(zhenagent.Card{ID: "good", Title: "x", Description: goodDesc, Status: "todo", Type: "task", Owner: "zhen"})
	r := &fakeRunner{write: true, out: passOutcome(), err: fmt.Errorf("route: local LLM: %w", gpulease.ErrTimeout)}
	w, _, _, _ := newTestWorker(t, m, r)
	res, _ := w.runOnce(context.Background())
	if res.Outcome != outcomeDeferred || res.Infra {
		t.Fatalf("res %+v", res)
	}
	run, _ := m.Get(context.Background(), res.Run)
	if run.Status != zhenagent.StatusCancelled {
		t.Errorf("run = %s", run.Status)
	}
	if n, _ := m.Attempts(context.Background(), "good"); n != 0 {
		t.Errorf("a GPU deferral used an attempt (%d)", n)
	}
	if c, _ := m.CardState("good"); c.Status != "todo" {
		t.Errorf("card = %s", c.Status)
	}
	// A cancelled run committed nothing: its worktree and branch go, so
	// the next poll starts the card again on the same branch name.
	r.err = nil
	res2, _ := w.runOnce(context.Background())
	if res2.Outcome != outcomeSucceeded || res2.Branch != "zhen/good" {
		t.Errorf("retry after the deferral: %+v", res2)
	}
}
