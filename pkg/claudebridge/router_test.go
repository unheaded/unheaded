// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package claudebridge

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func testRouting() *RoutingConfig {
	return &RoutingConfig{Version: 1, Families: map[string]FamilyRoute{
		"textbook-local": {Route: RouteLocalOnly, LocalAttempts: 1},
		"repo-task":      {Route: RouteLocalFirst, LocalAttempts: 2, EscalateOn: []string{"check_fail", "parse_fail"}, MaxClaudeUSD: 0.25},
		"parse-only":     {Route: RouteLocalFirst, LocalAttempts: 1, EscalateOn: []string{"parse_fail"}, MaxClaudeUSD: 0.10},
		"hard":           {Route: RouteClaudeDirect, MaxClaudeUSD: 0.05},
	}, Default: FamilyRoute{Route: RouteClaudeDirect, MaxClaudeUSD: 0.25}}
}

// routeFakes: a scripted local model, applier, check and Claude.
type routeFakes struct {
	mu          sync.Mutex
	local       []string // answers, in order
	localErr    error
	applyErr    error
	checks      []bool // check results, in order
	claudeText  string
	claudeCost  NanoUSD
	claudeErr   error
	claudeCalls []string
	reasons     []string
	budgets     []NanoUSD
	localCalls  []string
	audit       recAuditor
}

func (f *routeFakes) router(cfg *RoutingConfig, withCheck bool) *ThresholdRouter {
	r := &ThresholdRouter{
		Config: cfg,
		Local: func(_ context.Context, prompt string) (LocalAnswer, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.localCalls = append(f.localCalls, prompt)
			if f.localErr != nil {
				return LocalAnswer{}, f.localErr
			}
			a := f.local[0]
			if len(f.local) > 1 {
				f.local = f.local[1:]
			}
			return LocalAnswer{Text: a, Model: "qwen-7b", InputTokens: 10, OutputTokens: 5}, nil
		},
		Claude: func(_ context.Context, prompt, reason, family string, budget NanoUSD) (ClaudeResult, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.claudeCalls = append(f.claudeCalls, prompt)
			f.reasons = append(f.reasons, reason)
			f.budgets = append(f.budgets, budget)
			return ClaudeResult{Text: f.claudeText, Cost: f.claudeCost}, f.claudeErr
		},
		Redact: func(_ context.Context, s string) (string, error) {
			return strings.ReplaceAll(s, "10.10.10.10", "[REDACTED:ipv4#1]"), nil
		},
		Auditor: &f.audit,
	}
	if withCheck {
		r.Apply = func(_ context.Context, answer string) error {
			if f.applyErr != nil {
				return f.applyErr
			}
			if !strings.Contains(answer, "```file:") {
				return ErrNoFileBlocks
			}
			return nil
		}
		r.Check = func(context.Context) (CheckOutcome, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			ok := f.checks[0]
			if len(f.checks) > 1 {
				f.checks = f.checks[1:]
			}
			out := "ok"
			if !ok {
				out = "--- FAIL: TestX (peer 10.10.10.10 refused)"
			}
			return CheckOutcome{Pass: ok, Output: out}, nil
		}
	}
	return r
}

const draft = "```file:lib.go\npackage x\n```"

func TestRouter_LocalOnlyNeverCallsClaude(t *testing.T) {
	f := &routeFakes{local: []string{"an answer"}}
	out, err := f.router(testRouting(), false).Run(context.Background(), "textbook-local", "q")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.claudeCalls) != 0 || out.Escalated || out.Route != RouteLocalOnly || out.Text != "an answer" || out.RouteReason != "local_only" {
		t.Errorf("out = %+v, claude calls %d", out, len(f.claudeCalls))
	}
	// A failing local-only task stops; it never escalates.
	f2 := &routeFakes{local: []string{draft}, checks: []bool{false}}
	out, _ = f2.router(testRouting(), true).Run(context.Background(), "textbook-local", "q")
	if len(f2.claudeCalls) != 0 || out.Check != "fail" || out.Stop != "local_only" {
		t.Errorf("failing local_only: %+v", out)
	}
}

func TestRouter_LocalFirstPassingCheckNoClaude(t *testing.T) {
	f := &routeFakes{local: []string{draft}, checks: []bool{true}}
	out, err := f.router(testRouting(), true).Run(context.Background(), "repo-task", "fix it")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.claudeCalls) != 0 || out.Check != "pass" || out.LocalAttempts != 1 || out.RouteReason != "local_first" {
		t.Errorf("out = %+v", out)
	}
}

func TestRouter_FailingCheckEscalatesOnceWithContext(t *testing.T) {
	f := &routeFakes{local: []string{draft, draft + " v2"}, checks: []bool{false, false, true}, claudeText: "fixed", claudeCost: 30_000_000}
	out, err := f.router(testRouting(), true).Run(context.Background(), "repo-task", "TASK: fix lib.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.localCalls) != 2 || !strings.Contains(f.localCalls[1], "peer [REDACTED:ipv4#1] refused") {
		t.Errorf("second local attempt did not see the (redacted) check output: %q", f.localCalls)
	}
	if len(f.claudeCalls) != 1 {
		t.Fatalf("claude calls = %d", len(f.claudeCalls))
	}
	p := f.claudeCalls[0]
	for _, want := range []string{"TASK: fix lib.go", "v2", "[REDACTED:ipv4#1]"} {
		if !strings.Contains(p, want) {
			t.Errorf("escalation prompt lacks %q:\n%s", want, p)
		}
	}
	if strings.Contains(p, "10.10.10.10") {
		t.Error("the escalation prompt carries unredacted check output")
	}
	if f.reasons[0] != "escalated_check_fail" || f.budgets[0] != 250_000_000 {
		t.Errorf("reason %q budget %d", f.reasons[0], f.budgets[0])
	}
	if !out.Escalated || out.Check != "pass" || out.RouteReason != "escalated_check_fail" || out.Cost != 30_000_000 || out.LocalAttempts != 2 {
		t.Errorf("out = %+v", out)
	}
}

func TestRouter_BudgetStops(t *testing.T) {
	f := &routeFakes{local: []string{draft}, checks: []bool{false}, claudeErr: fmt.Errorf("claudebridge: %w", ErrSpendCap)}
	out, err := f.router(testRouting(), true).Run(context.Background(), "repo-task", "x")
	if err != nil {
		t.Fatal(err)
	}
	if out.Stop != "escalation_budget" || out.Check != "fail" {
		t.Errorf("out = %+v", out)
	}
}

// The cap can stop a Claude leg after it already changed the worktree
// (S-TWO-BRAINS Step 219: patch_file and run_check in the first request,
// the closing request refused). The check still judges what was bought.
func TestRouter_BudgetStopStillChecks(t *testing.T) {
	f := &routeFakes{local: []string{draft}, checks: []bool{false, false, true}, claudeCost: 16_716_000,
		claudeErr: fmt.Errorf("claudebridge: %w", ErrSpendCap)}
	out, err := f.router(testRouting(), true).Run(context.Background(), "repo-task", "x")
	if err != nil {
		t.Fatal(err)
	}
	if out.Stop != "escalation_budget" || out.Check != "pass" || out.Cost != 16_716_000 {
		t.Errorf("out = %+v", out)
	}
	// A cap hit before anything was sent: nothing changed, the check fails.
	f2 := &routeFakes{local: []string{draft}, checks: []bool{false, false}, claudeErr: fmt.Errorf("claudebridge: %w", ErrSpendCap)}
	if out, _ := f2.router(testRouting(), true).Run(context.Background(), "repo-task", "x"); out.Check != "fail" || out.Stop != "escalation_budget" {
		t.Errorf("out = %+v", out)
	}
}

func TestRouter_ParseFailureEscalatesWhenConfigured(t *testing.T) {
	f := &routeFakes{local: []string{"no blocks here"}, checks: []bool{true}, claudeText: "ok"}
	out, _ := f.router(testRouting(), true).Run(context.Background(), "parse-only", "x")
	if len(f.claudeCalls) != 1 || f.reasons[0] != "escalated_parse_fail" || !out.Escalated {
		t.Errorf("out = %+v reasons %v", out, f.reasons)
	}
	// check_fail is not in parse-only's escalate_on: a failing check stops.
	f2 := &routeFakes{local: []string{draft}, checks: []bool{false}}
	out, _ = f2.router(testRouting(), true).Run(context.Background(), "parse-only", "x")
	if len(f2.claudeCalls) != 0 || out.Stop != "not_escalated" {
		t.Errorf("out = %+v", out)
	}
}

func TestRouter_LocalDownIsParseFail(t *testing.T) {
	f := &routeFakes{localErr: errors.New("local LLM unavailable"), checks: []bool{true}, claudeText: "ok"}
	out, _ := f.router(testRouting(), true).Run(context.Background(), "repo-task", "x")
	if len(f.claudeCalls) != 1 || f.reasons[0] != "escalated_parse_fail" || out.Check != "pass" {
		t.Errorf("out = %+v reasons %v", out, f.reasons)
	}
}

func TestRouter_ClaudeDirectSkipsLocal(t *testing.T) {
	f := &routeFakes{claudeText: "answer", claudeCost: 1}
	out, _ := f.router(testRouting(), false).Run(context.Background(), "hard", "q")
	if len(f.localCalls) != 0 || len(f.claudeCalls) != 1 || f.reasons[0] != "claude_direct" || f.budgets[0] != 50_000_000 || out.Text != "answer" {
		t.Errorf("out = %+v", out)
	}
	// Unknown family -> default (claude_direct).
	f2 := &routeFakes{claudeText: "a"}
	out, _ = f2.router(testRouting(), false).Run(context.Background(), "made-up", "q")
	if out.Route != RouteClaudeDirect || len(f2.claudeCalls) != 1 {
		t.Errorf("default: %+v", out)
	}
}

func TestRouter_EveryRequestHasARouteReason(t *testing.T) {
	f := &routeFakes{local: []string{draft}, checks: []bool{false, true}, claudeText: "ok"}
	r := f.router(testRouting(), true)
	r.Config.Families["repo-task"] = FamilyRoute{Route: RouteLocalFirst, LocalAttempts: 1, EscalateOn: []string{"check_fail"}, MaxClaudeUSD: 0.25}
	_, _ = r.Run(context.Background(), "repo-task", "x")
	if len(f.audit.reqs) != 1 || f.audit.reqs[0].RouteReason != "local_first" || f.audit.reqs[0].Backend != BackendLocal || f.audit.reqs[0].TaskFamily != "repo-task" {
		t.Errorf("local audit = %+v", f.audit.reqs)
	}
	if f.reasons[0] == "" {
		t.Error("the Claude leg has no route reason")
	}
}

func TestLoadRoutingConfig(t *testing.T) {
	p := filepath.Join(t.TempDir(), "r.yaml")
	doc := "version: 1\nfamilies:\n  repo-task: {route: local_first, local_model: qwen-7b, local_attempts: 2, escalate_on: [check_fail], max_claude_usd: 0.25, tune: {n: 13}}\ndefault: {route: claude_direct, max_claude_usd: 0.25}\n"
	if err := os.WriteFile(p, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := LoadRoutingConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if fr := c.For("repo-task"); fr.Route != RouteLocalFirst || fr.LocalAttempts != 2 || fr.MaxClaudeUSD != 0.25 {
		t.Errorf("repo-task = %+v", fr)
	}
	if c.For("nope").Route != RouteClaudeDirect {
		t.Error("default not used")
	}
	for _, bad := range []string{
		"version: 2\nfamilies: {}\ndefault: {route: claude_direct}",
		"version: 1\nfamilies: {a: {route: yolo}}\ndefault: {route: claude_direct}",
		"version: 1\nfamilies: {a: {route: local_first, escalate_on: [whenever]}}\ndefault: {route: claude_direct}",
		"version: 1\nfamilies: {a: {route: local_first, max_claude_usd: 50}}\ndefault: {route: claude_direct}",
		"version: 1\nfamilies: {a: {route: local_first, local_attempts: 9}}\ndefault: {route: claude_direct}",
	} {
		_ = os.WriteFile(p, []byte(bad), 0o600)
		if _, err := LoadRoutingConfig(p); err == nil {
			t.Errorf("accepted:\n%s", bad)
		}
	}
}

func TestParseFileBlocks(t *testing.T) {
	bs, err := ParseFileBlocks("x\n```file:a/b.go\npackage b\n```\ny\n```file: c.py \nprint(1)\n```")
	if err != nil || len(bs) != 2 || bs[0].Path != "a/b.go" || bs[1].Path != "c.py" || bs[1].Content != "print(1)\n" {
		t.Errorf("%+v %v", bs, err)
	}
	if _, err := ParseFileBlocks("none"); !errors.Is(err, ErrNoFileBlocks) {
		t.Errorf("err = %v", err)
	}
}

func TestRouter_Metrics(t *testing.T) {
	before := routeTotal.WithLabelValues("repo-task", RouteLocalFirst, "pass").Value()
	esc := escalationsTotal.WithLabelValues("repo-task", "escalated_check_fail").Value()
	other := routeTotal.WithLabelValues("default", RouteClaudeDirect, "answered").Value()
	f := &routeFakes{local: []string{draft}, checks: []bool{false, false, true}, claudeText: "ok"}
	if _, err := f.router(testRouting(), true).Run(context.Background(), "repo-task", "x"); err != nil {
		t.Fatal(err)
	}
	if routeTotal.WithLabelValues("repo-task", RouteLocalFirst, "pass").Value() != before+1 {
		t.Error("zhen_route_total not counted")
	}
	if escalationsTotal.WithLabelValues("repo-task", "escalated_check_fail").Value() != esc+1 {
		t.Error("zhen_escalations_total not counted")
	}
	f2 := &routeFakes{claudeText: "a"}
	_, _ = f2.router(testRouting(), false).Run(context.Background(), "attacker-chosen-label", "q")
	if routeTotal.WithLabelValues("default", RouteClaudeDirect, "answered").Value() != other+1 {
		t.Error("an unknown family was not folded into default")
	}
}
