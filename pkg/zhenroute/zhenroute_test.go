// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package zhenroute

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"unheaded/pkg/claudebridge"
	"unheaded/pkg/claudebridge/privacy"
)

func llama(t *testing.T, answer string) string {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		b, _ := json.Marshal(map[string]any{"model": "qwen.gguf", "choices": []any{map[string]any{"finish_reason": "stop",
			"message": map[string]any{"role": "assistant", "content": answer}}}, "usage": map[string]any{"prompt_tokens": 5, "completion_tokens": 2}})
		_, _ = w.Write(b)
	}))
	t.Cleanup(s.Close)
	return s.URL
}

func routing() *claudebridge.RoutingConfig {
	return &claudebridge.RoutingConfig{Version: 1, Families: map[string]claudebridge.FamilyRoute{
		"docs": {Route: claudebridge.RouteClaudeDirect, MaxClaudeUSD: 0.25},
	}, Default: claudebridge.FamilyRoute{Route: claudebridge.RouteClaudeDirect, MaxClaudeUSD: 0.25}}
}

func stubFilter(t *testing.T) *privacy.Filter {
	t.Helper()
	p := filepath.Join(t.TempDir(), "scanner")
	if err := os.WriteFile(p, []byte("#!/bin/sh\ncat >/dev/null\necho '[]'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	r, err := privacy.NewRedactor(privacy.RedactorConfig{Scanner: p})
	if err != nil {
		t.Fatal(err)
	}
	return &privacy.Filter{Redactor: r, Mode: privacy.ModeStandard}
}

func TestRun_CardBudgetOnlyLowers(t *testing.T) {
	var got claudebridge.NanoUSD
	leg := func(_ context.Context, in ClaudeInput) (claudebridge.ClaudeResult, error) {
		got = in.Budget
		return claudebridge.ClaudeResult{Text: "ok"}, nil
	}
	f := stubFilter(t)
	for _, c := range []struct {
		card float64
		want claudebridge.NanoUSD
	}{{0, 250_000_000}, {0.1, 100_000_000}, {3, 250_000_000}} {
		out, err := Run(context.Background(), Config{Routing: routing(), Claude: leg, Privacy: f, LocalURL: llama(t, "x")},
			Task{Family: "docs", Prompt: "p", MaxClaudeUSD: c.card})
		if err != nil || out.RouteReason != "claude_direct" {
			t.Fatalf("card %v: %+v %v", c.card, out, err)
		}
		if got != c.want {
			t.Errorf("card %v: budget %d, want %d", c.card, got, c.want)
		}
	}
}

func TestRun_NoPrivacyNoClaude(t *testing.T) {
	called := false
	leg := func(context.Context, ClaudeInput) (claudebridge.ClaudeResult, error) {
		called = true
		return claudebridge.ClaudeResult{}, nil
	}
	_, err := Run(context.Background(), Config{Routing: routing(), Claude: leg, PrivacyErr: errors.New("no scanner"), LocalURL: llama(t, "x")},
		Task{Family: "docs", Prompt: "p"})
	if err == nil || called {
		t.Errorf("err %v, called %v", err, called)
	}
}

func TestRun_LocalOnlyNeverCallsClaude(t *testing.T) {
	called := false
	leg := func(context.Context, ClaudeInput) (claudebridge.ClaudeResult, error) {
		called = true
		return claudebridge.ClaudeResult{}, nil
	}
	out, err := Run(context.Background(), Config{Routing: routing(), Claude: leg, LocalURL: llama(t, "local answer")},
		Task{Family: "docs", Prompt: "p", LocalOnly: true})
	if err != nil || called || out.Route != claudebridge.RouteLocalOnly || out.Text != "local answer" {
		t.Errorf("out %+v err %v called %v", out, err, called)
	}
}

func TestRun_NotAnAgentWorktree(t *testing.T) {
	_, err := Run(context.Background(), Config{Routing: routing(), LocalURL: llama(t, "x")},
		Task{Family: "docs", Prompt: "p", Worktree: t.TempDir(), AgentRoot: t.TempDir(), LocalOnly: true})
	if !errors.Is(err, ErrNotAgentWorktree) {
		t.Errorf("err = %v", err)
	}
}

func TestRun_CheckNeedsWorktree(t *testing.T) {
	if _, err := Run(context.Background(), Config{Routing: routing()}, Task{Family: "docs", Prompt: "p", Check: "go-test"}); err == nil {
		t.Error("a check without a worktree ran")
	}
}

func TestFakeClaude(t *testing.T) {
	r, err := FakeClaude()(context.Background(), ClaudeInput{Reason: "claude_direct"})
	if err != nil || r.Cost != 0 || !strings.Contains(r.Text, "fake") {
		t.Errorf("%+v %v", r, err)
	}
}

func TestApplyDraft_RefusesProtected(t *testing.T) {
	for _, p := range []string{"check.sh", "sub/check.sh", "../x", "/etc/passwd", "TASK.md"} {
		if err := ApplyDraft(context.Background(), nil, "/tmp", "```file:"+p+"\nx\n```", nil); err == nil {
			t.Errorf("%s accepted", p)
		}
	}
	if err := ApplyDraft(context.Background(), nil, "/tmp", "```file:a_test.go\nx\n```", map[string]bool{"a_test.go": true}); err == nil {
		t.Error("a protected test accepted")
	}
}

// A per-task cap below one request's worst case at max_tokens 4096 must
// shrink max_tokens, not stop every Claude leg at escalation_budget
// before anything is sent (Step 219a).
func TestFitMaxTokens(t *testing.T) {
	const outRate = 25_000 // nano-USD per output token, the table's highest
	cases := []struct {
		capNano claudebridge.NanoUSD
		want    int64
	}{
		{250_000_000, 4096}, // $0.25: the configured maximum fits
		{100_000_000, 2800}, // $0.10: 70 % of the cap at the highest rate
		{50_000_000, 1400},  // $0.05: the repo-task family cap
		{30_000_000, 0},     // under the 1024-token floor: no request
	}
	for _, c := range cases {
		if got := fitMaxTokens(c.capNano, outRate, 4096); got != c.want {
			t.Errorf("cap %d: %d, want %d", c.capNano, got, c.want)
		}
	}
}
