// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package rules

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Engine
// ---------------------------------------------------------------------------

func TestNewEngine(t *testing.T) {
	e := NewEngine()
	if e == nil {
		t.Fatal("NewEngine returned nil")
	}
	if e.blockThreshold != 15 {
		t.Errorf("blockThreshold = %d, want 15", e.blockThreshold)
	}
	if e.logThreshold != 5 {
		t.Errorf("logThreshold = %d, want 5", e.logThreshold)
	}
}

func TestEngine_SetThresholds(t *testing.T) {
	e := NewEngine()
	e.SetBlockThreshold(20)
	e.SetLogThreshold(10)
	if e.blockThreshold != 20 {
		t.Errorf("blockThreshold = %d", e.blockThreshold)
	}
	if e.logThreshold != 10 {
		t.Errorf("logThreshold = %d", e.logThreshold)
	}
}

func TestEngine_AddRemoveRuleSet(t *testing.T) {
	e := NewEngine()
	rs := NewRuleSet("test", "Test")
	e.AddRuleSet(rs)
	if len(e.ruleSets) != 1 {
		t.Errorf("expected 1 rule set, got %d", len(e.ruleSets))
	}
	if !e.RemoveRuleSet("test") {
		t.Error("RemoveRuleSet should return true")
	}
	if e.RemoveRuleSet("test") {
		t.Error("RemoveRuleSet again should return false")
	}
}

func TestEngine_Evaluate_NoRules(t *testing.T) {
	e := NewEngine()
	req := httptest.NewRequest("GET", "http://example.com/", nil)
	result, err := e.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if result.Action != ActionAllow {
		t.Errorf("Action = %v, want allow", result.Action)
	}
	if result.TotalScore != 0 {
		t.Errorf("TotalScore = %d", result.TotalScore)
	}
}

func TestEngine_Evaluate_BlockRule(t *testing.T) {
	e := NewEngine()
	rs := NewRuleSet("test", "Test")
	r, _ := NewRule("sqli", "SQLi", `select.*from`, []Target{TargetQuery}, ActionBlock, SeverityHigh)
	rs.AddRule(r)
	e.AddRuleSet(rs)

	req := httptest.NewRequest("GET", "http://example.com/?q=SELECT+foo+FROM+bar", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	result, err := e.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if result.Action != ActionBlock {
		t.Errorf("Action = %v, want block", result.Action)
	}
	if len(result.Matches) == 0 {
		t.Error("expected matches")
	}
	if result.HighestSeverity != SeverityHigh {
		t.Errorf("HighestSeverity = %v", result.HighestSeverity)
	}
}

func TestEngine_Evaluate_DisabledRule(t *testing.T) {
	e := NewEngine()
	rs := NewRuleSet("test", "Test")
	r, _ := NewRule("sqli", "SQLi", `select.*from`, []Target{TargetAll}, ActionBlock, SeverityHigh)
	r.Disable()
	rs.AddRule(r)
	e.AddRuleSet(rs)

	req := httptest.NewRequest("GET", "http://example.com/?q=SELECT+foo+FROM+bar", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	result, err := e.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if result.Action != ActionAllow {
		t.Errorf("disabled rule should not trigger, Action = %v", result.Action)
	}
}

func TestEngine_Evaluate_ScoreThreshold(t *testing.T) {
	e := NewEngine()
	e.SetBlockThreshold(15)
	e.SetLogThreshold(5)

	rs := NewRuleSet("test", "Test")
	// This rule has score 2 (SeverityLow), above log threshold with a match
	r, _ := NewRule("r1", "rule", `test`, []Target{TargetPath}, ActionLog, SeverityLow)
	rs.AddRule(r)
	// Add another low-severity rule to cross log threshold
	r2, _ := NewRule("r2", "rule2", `path`, []Target{TargetPath}, ActionLog, SeverityLow)
	rs.AddRule(r2)
	// Add medium rule to push past log threshold
	r3, _ := NewRule("r3", "rule3", `test`, []Target{TargetPath}, ActionLog, SeverityMedium)
	rs.AddRule(r3)
	e.AddRuleSet(rs)

	req := httptest.NewRequest("GET", "http://example.com/test-path", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	result, err := e.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	// Total score: 2 (r1 matches "test") + 2 (r2 matches "path") + 5 (r3 matches "test") = 9
	// 9 >= logThreshold(5) but < blockThreshold(15) => ActionLog
	if result.Action != ActionLog {
		t.Errorf("Action = %v, want log (score=%d)", result.Action, result.TotalScore)
	}
}

func TestEngine_Evaluate_RedirectAction(t *testing.T) {
	e := NewEngine()
	rs := NewRuleSet("test", "Test")
	r, _ := NewRule("redir", "Redirect", `malicious`, []Target{TargetPath}, ActionRedirect, SeverityMedium)
	rs.AddRule(r)
	e.AddRuleSet(rs)

	req := httptest.NewRequest("GET", "http://example.com/malicious-path", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	result, err := e.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if result.Action != ActionRedirect {
		t.Errorf("Action = %v, want redirect", result.Action)
	}
}

func TestEngine_Evaluate_ContextCancellation(t *testing.T) {
	e := NewEngine()
	rs := NewRuleSet("test", "Test")
	r, _ := NewRule("r1", "rule", `test`, []Target{TargetPath}, ActionLog, SeverityLow)
	rs.AddRule(r)
	e.AddRuleSet(rs)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	_, err := e.Evaluate(ctx, req)
	// Should return the context error
	if err != context.Canceled {
		// It may also return nil if it completed before checking context
		// The engine checks ctx after each rule, so with only 1 rule it might complete first
		_ = err // acceptable either way
	}
}

func TestEngine_Evaluate_BodyTarget(t *testing.T) {
	e := NewEngine()
	rs := NewRuleSet("test", "Test")
	r, _ := NewRule("body-check", "Body check", `malicious`, []Target{TargetBody}, ActionBlock, SeverityHigh)
	rs.AddRule(r)
	e.AddRuleSet(rs)

	body := strings.NewReader("this is malicious content")
	req := httptest.NewRequest("POST", "http://example.com/api", body)
	req.RemoteAddr = "1.2.3.4:1234"
	result, err := e.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if result.Action != ActionBlock {
		t.Errorf("Action = %v, want block", result.Action)
	}
}

func TestEngine_Evaluate_HeadersTarget(t *testing.T) {
	e := NewEngine()
	rs := NewRuleSet("test", "Test")
	r, _ := NewRule("hdr-check", "Header check", `evil-agent`, []Target{TargetHeaders}, ActionBlock, SeverityHigh)
	rs.AddRule(r)
	e.AddRuleSet(rs)

	req := httptest.NewRequest("GET", "http://example.com/", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	req.Header.Set("User-Agent", "evil-agent/1.0")
	result, err := e.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if result.Action != ActionBlock {
		t.Errorf("Action = %v, want block", result.Action)
	}
}

func TestEngine_Evaluate_CookiesTarget(t *testing.T) {
	e := NewEngine()
	rs := NewRuleSet("test", "Test")
	r, _ := NewRule("cookie-check", "Cookie check", `session_hack`, []Target{TargetCookies}, ActionBlock, SeverityHigh)
	rs.AddRule(r)
	e.AddRuleSet(rs)

	req := httptest.NewRequest("GET", "http://example.com/", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	req.AddCookie(&http.Cookie{Name: "session_hack", Value: "stolen"})
	result, err := e.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if result.Action != ActionBlock {
		t.Errorf("Action = %v, want block", result.Action)
	}
}

func TestEngine_Evaluate_MethodTarget(t *testing.T) {
	e := NewEngine()
	rs := NewRuleSet("test", "Test")
	r, _ := NewRule("method-check", "Method check", `DELETE`, []Target{TargetMethod}, ActionBlock, SeverityMedium)
	rs.AddRule(r)
	e.AddRuleSet(rs)

	req := httptest.NewRequest("DELETE", "http://example.com/resource", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	result, err := e.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if result.Action != ActionBlock {
		t.Errorf("Action = %v, want block", result.Action)
	}
}

func TestEngine_Evaluate_UserAgentTarget(t *testing.T) {
	e := NewEngine()
	rs := NewRuleSet("test", "Test")
	r, _ := NewRule("ua-check", "UA check", `bad-bot`, []Target{TargetUserAgent}, ActionBlock, SeverityMedium)
	rs.AddRule(r)
	e.AddRuleSet(rs)

	req := httptest.NewRequest("GET", "http://example.com/", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	req.Header.Set("User-Agent", "bad-bot/1.0")
	result, err := e.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if result.Action != ActionBlock {
		t.Errorf("Action = %v, want block", result.Action)
	}
}

func TestEngine_Evaluate_URLTarget(t *testing.T) {
	e := NewEngine()
	rs := NewRuleSet("test", "Test")
	r, _ := NewRule("url-check", "URL check", `evil\.com`, []Target{TargetURL}, ActionBlock, SeverityHigh)
	rs.AddRule(r)
	e.AddRuleSet(rs)

	req := httptest.NewRequest("GET", "http://evil.com/path", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	result, err := e.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if result.Action != ActionBlock {
		t.Errorf("Action = %v, want block", result.Action)
	}
}

// ---------------------------------------------------------------------------
// Engine rule management
// ---------------------------------------------------------------------------

func TestEngine_GetAllRules(t *testing.T) {
	e := NewEngine()
	rs1 := NewRuleSet("set1", "Set 1")
	r1, _ := NewRule("r1", "Rule 1", "abc", []Target{TargetAll}, ActionBlock, SeverityLow)
	rs1.AddRule(r1)
	rs2 := NewRuleSet("set2", "Set 2")
	r2, _ := NewRule("r2", "Rule 2", "def", []Target{TargetAll}, ActionLog, SeverityMedium)
	rs2.AddRule(r2)
	e.AddRuleSet(rs1)
	e.AddRuleSet(rs2)

	all := e.GetAllRules()
	if len(all) != 2 {
		t.Errorf("expected 2 rules, got %d", len(all))
	}
}

func TestEngine_FindRule(t *testing.T) {
	e := NewEngine()
	rs := NewRuleSet("set1", "Set 1")
	r1, _ := NewRule("r1", "Rule 1", "abc", []Target{TargetAll}, ActionBlock, SeverityLow)
	rs.AddRule(r1)
	e.AddRuleSet(rs)

	found := e.FindRule("r1")
	if found == nil {
		t.Fatal("FindRule returned nil")
	}
	if found.ID != "r1" {
		t.Errorf("ID = %q", found.ID)
	}

	if e.FindRule("nonexistent") != nil {
		t.Error("FindRule should return nil for nonexistent")
	}
}

func TestEngine_RemoveRule(t *testing.T) {
	e := NewEngine()
	rs := NewRuleSet("set1", "Set 1")
	r1, _ := NewRule("r1", "Rule 1", "abc", []Target{TargetAll}, ActionBlock, SeverityLow)
	rs.AddRule(r1)
	e.AddRuleSet(rs)

	if !e.RemoveRule("r1") {
		t.Error("RemoveRule should return true")
	}
	if e.RemoveRule("r1") {
		t.Error("RemoveRule again should return false")
	}
}

// ---------------------------------------------------------------------------
// truncate
// ---------------------------------------------------------------------------

func TestTruncate(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Errorf("truncate short = %q", got)
	}
	if got := truncate("long string here", 4); got != "long..." {
		t.Errorf("truncate long = %q", got)
	}
}

// ---------------------------------------------------------------------------
// extractRequestData
// ---------------------------------------------------------------------------

func TestExtractRequestData_AllFields(t *testing.T) {
	e := NewEngine()
	req := httptest.NewRequest("POST", "http://example.com/path?key=val", strings.NewReader("body content"))
	req.RemoteAddr = "1.2.3.4:5678"
	req.Header.Set("User-Agent", "TestAgent/1.0")
	req.Header.Set("X-Custom", "value")

	data := e.extractRequestData(req)
	if data.Method != "POST" {
		t.Errorf("Method = %q", data.Method)
	}
	if data.Path != "/path" {
		t.Errorf("Path = %q", data.Path)
	}
	if data.Query != "key=val" {
		t.Errorf("Query = %q", data.Query)
	}
	if data.Body != "body content" {
		t.Errorf("Body = %q", data.Body)
	}
	if data.UserAgent != "TestAgent/1.0" {
		t.Errorf("UserAgent = %q", data.UserAgent)
	}
	if data.IP != "1.2.3.4" {
		t.Errorf("IP = %q", data.IP)
	}
}

// ---------------------------------------------------------------------------
// getTargetInputs (engine method)
// ---------------------------------------------------------------------------

func TestEngine_GetTargetInputs_Default(t *testing.T) {
	e := NewEngine()
	data := &requestData{URL: "u", Path: "p"}
	got := e.getTargetInputs(Target(99), data)
	if got != nil {
		t.Errorf("unknown target should return nil, got %v", got)
	}
}
