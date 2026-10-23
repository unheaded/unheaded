// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"unheaded/pkg/champion"
	"unheaded/pkg/claudebridge"
	"unheaded/pkg/lichtools"
)

// The routing weight chooses which MODEL runs a task (Haiku vs Opus) and
// nothing else. It is not an input to the Champion gate, the dollar cap, or the
// confirmation flow, so a jailbroken driver cannot relabel a task's weight to
// dodge any of them. These tests pin that orthogonality (Phase 4, Steps 119-121).

func TestBoundary_WeightDoesNotChangeGateClassification(t *testing.T) {
	// The gate keys off the tool name, never a routing weight. A driver calling
	// AutoRouter with any hint cannot move file_finding out of "mutating".
	ff := &champion.ToolCall{Name: lichtools.ToolFileFinding}
	if !ff.IsMutating() {
		t.Fatal("file_finding must always be mutating regardless of routing")
	}
	ro := &champion.ToolCall{Name: lichtools.ToolLocalLLM}
	if ro.IsMutating() {
		t.Fatal("lich_local_llm must always be read-only regardless of routing")
	}

	// Even when the router is forced to MINOR, the gate still refuses
	// file_finding pending confirmation — routing changed nothing.
	r := claudebridge.AutoRouter{}
	w, _ := r.Route(context.Background(), claudebridge.Task{
		Prompt: "file this finding",
		Hints:  map[string]string{"weight": "minor"},
	})
	if w != claudebridge.WeightMinor {
		t.Fatalf("forced hint = %q, want minor", w)
	}
	g, _, prov := newGateForTest(t)
	_, err := g.Execute(context.Background(), fileFindingCall(`{"severity":"high","summary":"x"}`), prov)
	var ref *claudebridge.RefusedError
	if !errors.As(err, &ref) || !ref.PendingConfirmation {
		t.Fatalf("minor-weighted file_finding still went through the gate: err=%v", err)
	}
	if prov.filed != 0 {
		t.Fatal("a minor-weighted file_finding ran without confirmation")
	}
}

func TestBoundary_HeavyForcedToClaudeStillCapped(t *testing.T) {
	// Forcing HEAVY selects Opus, but the spend cap is a property of the Ledger
	// the Bridge holds, not of the weight. A zero-cap ledger refuses every
	// request regardless of the model the weight picked.
	r := claudebridge.AutoRouter{}
	w, _ := r.Route(context.Background(), claudebridge.Task{
		Prompt: "generate seeds", // reads minor
		Hints:  map[string]string{"weight": "heavy"},
	})
	if w != claudebridge.WeightHeavy {
		t.Fatalf("forced hint = %q, want heavy", w)
	}
	model := claudebridge.ModelForWeight(w, "claude-haiku-5-5", "claude-opus-5-5")
	if model != "claude-opus-5-5" {
		t.Fatalf("heavy weight model = %q, want opus", model)
	}

	led, err := claudebridge.NewLedger("", 0) // zero cap
	if err != nil {
		t.Fatalf("ledger: %v", err)
	}
	if _, rerr := led.Reserve(1); rerr == nil {
		t.Fatal("a zero-cap ledger must refuse any reserve, whatever the weight")
	}
}

func TestBoundary_LocalPathIsFree(t *testing.T) {
	// The local path never touches a Ledger: the provider holds none, so a
	// driver cannot burn dollars by routing MINOR work locally.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model":   "local",
			"choices": []map[string]any{{"finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": "ok"}}},
		})
	}))
	defer srv.Close()

	root := t.TempDir()
	cfg := &lichtools.Config{ProjectRoot: root}
	cfg.Scope.SandboxPrefix = lichtools.DefaultSandboxPrefix
	cfg.Scope.HarnessDir = root // NewProvider only needs a valid scope; local_llm is scope-free
	p, err := lichtools.NewProvider(cfg)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	p.WithLocalLLM(srv.URL, "local", 0)
	if _, err := p.Call(context.Background(), lichtools.ToolLocalLLM, json.RawMessage(`{"prompt":"x"}`)); err != nil {
		t.Fatalf("local call: %v", err)
	}
	used, _ := p.LocalGenBudget()
	if used != 1 {
		t.Fatalf("local gen budget = %d, want 1 (compute tick, no dollars)", used)
	}
}
