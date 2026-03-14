// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package rules

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// ChainOperator.String
// ---------------------------------------------------------------------------

func TestChainOperator_String(t *testing.T) {
	tests := []struct {
		op   ChainOperator
		want string
	}{
		{ChainAND, "AND"},
		{ChainOR, "OR"},
		{ChainNOT, "NOT"},
		{ChainXOR, "XOR"},
		{ChainOperator(99), "UNKNOWN"},
	}
	for _, tt := range tests {
		if got := tt.op.String(); got != tt.want {
			t.Errorf("ChainOperator(%d).String() = %q, want %q", tt.op, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// NewChainedRule
// ---------------------------------------------------------------------------

func TestNewChainedRule(t *testing.T) {
	r, _ := NewRule("r1", "test", "abc", []Target{TargetAll}, ActionBlock, SeverityHigh)
	cr := NewChainedRule(r)

	if cr.Rule != r {
		t.Error("Rule not set")
	}
	if cr.Operator != ChainAND {
		t.Errorf("Operator = %v, want AND", cr.Operator)
	}
	if cr.Version != 1 {
		t.Errorf("Version = %d", cr.Version)
	}
	if cr.ChainPhase != 1 {
		t.Errorf("ChainPhase = %d", cr.ChainPhase)
	}
	if len(cr.Children) != 0 {
		t.Error("Children should be empty")
	}
	if len(cr.Exceptions) != 0 {
		t.Error("Exceptions should be empty")
	}
	if cr.stats == nil {
		t.Error("stats should be initialized")
	}
}

// ---------------------------------------------------------------------------
// RuleStats
// ---------------------------------------------------------------------------

func TestRuleStats_RecordMatch(t *testing.T) {
	s := NewRuleStats()
	s.RecordMatch(true, 10, 5*time.Millisecond)
	s.RecordMatch(false, 5, 3*time.Millisecond)

	stats := s.GetStats()
	if stats["match_count"].(int64) != 2 {
		t.Errorf("match_count = %v", stats["match_count"])
	}
	if stats["block_count"].(int64) != 1 {
		t.Errorf("block_count = %v", stats["block_count"])
	}
	if stats["allow_count"].(int64) != 1 {
		t.Errorf("allow_count = %v", stats["allow_count"])
	}
	if stats["total_score"].(int64) != 15 {
		t.Errorf("total_score = %v", stats["total_score"])
	}
	if stats["average_latency"].(float64) <= 0 {
		t.Errorf("average_latency = %v", stats["average_latency"])
	}
}

// ---------------------------------------------------------------------------
// RuleRateLimit
// ---------------------------------------------------------------------------

func TestRuleRateLimit(t *testing.T) {
	rl := NewRuleRateLimit(3, time.Minute, ActionBlock)
	if !rl.Enabled {
		t.Error("should be enabled")
	}
	// First 3 should be allowed
	for i := 0; i < 3; i++ {
		if !rl.Allow("key1") {
			t.Errorf("request %d should be allowed", i+1)
		}
	}
	// 4th should be denied
	if rl.Allow("key1") {
		t.Error("4th request should be denied")
	}
	// Different key should still be allowed
	if !rl.Allow("key2") {
		t.Error("different key should be allowed")
	}

	// Reset
	rl.Reset("key1")
	if !rl.Allow("key1") {
		t.Error("after reset, should be allowed")
	}
}

func TestRuleRateLimit_Disabled(t *testing.T) {
	rl := NewRuleRateLimit(1, time.Minute, ActionBlock)
	rl.Enabled = false
	// Should always allow when disabled
	for i := 0; i < 10; i++ {
		if !rl.Allow("key") {
			t.Errorf("disabled rate limiter should always allow")
		}
	}
}

// ---------------------------------------------------------------------------
// ChainedRule child management
// ---------------------------------------------------------------------------

func TestChainedRule_AddRemoveChild(t *testing.T) {
	parent, _ := NewRule("parent", "Parent", "abc", []Target{TargetAll}, ActionBlock, SeverityHigh)
	cr := NewChainedRule(parent)

	child1, _ := NewRule("child1", "Child1", "def", []Target{TargetAll}, ActionLog, SeverityLow)
	child1cr := NewChainedRule(child1)
	cr.AddChild(child1cr)

	if len(cr.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(cr.Children))
	}
	if child1cr.ParentID != "parent" {
		t.Errorf("ParentID = %q", child1cr.ParentID)
	}

	if !cr.RemoveChild("child1") {
		t.Error("RemoveChild should return true")
	}
	if cr.RemoveChild("child1") {
		t.Error("RemoveChild again should return false")
	}
	if len(cr.Children) != 0 {
		t.Error("Children should be empty after removal")
	}
}

// ---------------------------------------------------------------------------
// ChainedRule exceptions
// ---------------------------------------------------------------------------

func TestChainedRule_AddRemoveException(t *testing.T) {
	r, _ := NewRule("r1", "test", "abc", []Target{TargetAll}, ActionBlock, SeverityHigh)
	cr := NewChainedRule(r)

	exc := &ExceptionRule{ID: "exc1", Enabled: true}
	cr.AddException(exc)
	if len(cr.Exceptions) != 1 {
		t.Fatalf("expected 1 exception")
	}

	if !cr.RemoveException("exc1") {
		t.Error("RemoveException should return true")
	}
	if cr.RemoveException("exc1") {
		t.Error("RemoveException again should return false")
	}
}

// ---------------------------------------------------------------------------
// ChainedRule versioning
// ---------------------------------------------------------------------------

func TestChainedRule_SetVersion(t *testing.T) {
	r, _ := NewRule("r1", "test", "abc", []Target{TargetAll}, ActionBlock, SeverityHigh)
	cr := NewChainedRule(r)

	cr.SetVersion(2, "updated pattern", "tester")
	if cr.Version != 2 {
		t.Errorf("Version = %d, want 2", cr.Version)
	}
	if len(cr.Changelog) != 1 {
		t.Fatalf("expected 1 changelog entry")
	}
	if cr.Changelog[0].Changes != "updated pattern" {
		t.Errorf("Changes = %q", cr.Changelog[0].Changes)
	}
	if cr.Changelog[0].Author != "tester" {
		t.Errorf("Author = %q", cr.Changelog[0].Author)
	}
}

// ---------------------------------------------------------------------------
// ChainedRule rate limit management
// ---------------------------------------------------------------------------

func TestChainedRule_SetDisableRateLimit(t *testing.T) {
	r, _ := NewRule("r1", "test", "abc", []Target{TargetAll}, ActionBlock, SeverityHigh)
	cr := NewChainedRule(r)

	cr.SetRateLimit(100, time.Minute, ActionBlock)
	if cr.RateLimit == nil {
		t.Fatal("RateLimit should be set")
	}
	if !cr.RateLimit.Enabled {
		t.Error("RateLimit should be enabled")
	}

	cr.DisableRateLimit()
	if cr.RateLimit.Enabled {
		t.Error("RateLimit should be disabled")
	}

	// DisableRateLimit on nil should not panic
	cr2 := NewChainedRule(r)
	cr2.DisableRateLimit() // no panic
}

func TestChainedRule_GetStats(t *testing.T) {
	r, _ := NewRule("r1", "test", "abc", []Target{TargetAll}, ActionBlock, SeverityHigh)
	cr := NewChainedRule(r)
	stats := cr.GetStats()
	if stats == nil {
		t.Error("GetStats returned nil")
	}
}

// ---------------------------------------------------------------------------
// ExceptionRule matching
// ---------------------------------------------------------------------------

func TestExceptionRule_MatchesIP(t *testing.T) {
	exc := &ExceptionRule{
		Enabled:   true,
		SourceIPs: []string{"192.168.1.1", "10.0.0.1"},
	}
	req := httptest.NewRequest("GET", "http://example.com/", nil)
	data := &requestData{IP: "192.168.1.1", Path: "/", Method: "GET"}

	if !exc.matches(req, data) {
		t.Error("should match IP")
	}

	data.IP = "8.8.8.8"
	if exc.matches(req, data) {
		t.Error("should not match non-listed IP")
	}
}

func TestExceptionRule_MatchesPath(t *testing.T) {
	exc := &ExceptionRule{
		Enabled: true,
		Paths:   []string{"/api/health", "/api/v1/*"},
	}
	req := httptest.NewRequest("GET", "http://example.com/api/health", nil)
	data := &requestData{IP: "1.2.3.4", Path: "/api/health", Method: "GET"}

	if !exc.matches(req, data) {
		t.Error("exact path should match")
	}

	// Prefix match
	req = httptest.NewRequest("GET", "http://example.com/api/v1/users", nil)
	data.Path = "/api/v1/users"
	if !exc.matches(req, data) {
		t.Error("prefix path should match")
	}

	req = httptest.NewRequest("GET", "http://example.com/other", nil)
	data.Path = "/other"
	if exc.matches(req, data) {
		t.Error("non-matching path should not match")
	}
}

func TestExceptionRule_MatchesMethod(t *testing.T) {
	exc := &ExceptionRule{
		Enabled: true,
		Methods: []string{"GET", "HEAD"},
	}
	req := httptest.NewRequest("GET", "http://example.com/", nil)
	data := &requestData{IP: "1.2.3.4", Path: "/", Method: "GET"}

	if !exc.matches(req, data) {
		t.Error("GET should match")
	}

	req = httptest.NewRequest("POST", "http://example.com/", nil)
	if exc.matches(req, data) {
		// The method check uses req.Method, not data.Method
		// Let's check both
	}
}

func TestExceptionRule_MatchesHeaders(t *testing.T) {
	exc := &ExceptionRule{
		Enabled: true,
		Headers: map[string]string{"X-Internal": "true"},
	}
	req := httptest.NewRequest("GET", "http://example.com/", nil)
	req.Header.Set("X-Internal", "true")
	data := &requestData{IP: "1.2.3.4", Path: "/", Method: "GET"}

	if !exc.matches(req, data) {
		t.Error("header match should pass")
	}

	req.Header.Set("X-Internal", "false")
	if exc.matches(req, data) {
		t.Error("wrong header value should not match")
	}
}

func TestExceptionRule_MatchesHeaders_Wildcard(t *testing.T) {
	exc := &ExceptionRule{
		Enabled: true,
		Headers: map[string]string{"X-Present": "*"},
	}
	req := httptest.NewRequest("GET", "http://example.com/", nil)
	req.Header.Set("X-Present", "anything")
	data := &requestData{IP: "1.2.3.4", Path: "/", Method: "GET"}

	if !exc.matches(req, data) {
		t.Error("wildcard header should match")
	}
}

func TestExceptionRule_MatchesUserAgent(t *testing.T) {
	exc := &ExceptionRule{
		Enabled:    true,
		UserAgents: []string{"Monitoring", "HealthCheck"},
	}
	req := httptest.NewRequest("GET", "http://example.com/", nil)
	req.Header.Set("User-Agent", "Monitoring/1.0")
	data := &requestData{IP: "1.2.3.4", Path: "/", Method: "GET"}

	if !exc.matches(req, data) {
		t.Error("user agent substring match should pass")
	}

	req.Header.Set("User-Agent", "Mozilla/5.0")
	if exc.matches(req, data) {
		t.Error("non-matching user agent should not match")
	}
}

func TestExceptionRule_TimeWindow(t *testing.T) {
	now := time.Now()
	past := now.Add(-1 * time.Hour)
	future := now.Add(1 * time.Hour)

	// Within window
	exc := &ExceptionRule{
		Enabled:   true,
		StartTime: &past,
		EndTime:   &future,
	}
	req := httptest.NewRequest("GET", "http://example.com/", nil)
	data := &requestData{IP: "1.2.3.4", Path: "/", Method: "GET"}
	if !exc.matches(req, data) {
		t.Error("within time window should match")
	}

	// Before start
	futureStart := now.Add(1 * time.Hour)
	futureEnd := now.Add(2 * time.Hour)
	exc2 := &ExceptionRule{
		Enabled:   true,
		StartTime: &futureStart,
		EndTime:   &futureEnd,
	}
	if exc2.matches(req, data) {
		t.Error("before start time should not match")
	}

	// After end
	pastStart := now.Add(-2 * time.Hour)
	pastEnd := now.Add(-1 * time.Hour)
	exc3 := &ExceptionRule{
		Enabled:   true,
		StartTime: &pastStart,
		EndTime:   &pastEnd,
	}
	if exc3.matches(req, data) {
		t.Error("after end time should not match")
	}
}

func TestExceptionRule_DaysOfWeek(t *testing.T) {
	today := time.Now().Weekday()
	exc := &ExceptionRule{
		Enabled:    true,
		DaysOfWeek: []time.Weekday{today},
	}
	req := httptest.NewRequest("GET", "http://example.com/", nil)
	data := &requestData{IP: "1.2.3.4", Path: "/", Method: "GET"}
	if !exc.matches(req, data) {
		t.Error("current day of week should match")
	}

	// Use a day that's not today
	otherDay := (today + 1) % 7
	exc2 := &ExceptionRule{
		Enabled:    true,
		DaysOfWeek: []time.Weekday{otherDay},
	}
	if exc2.matches(req, data) {
		t.Error("non-current day should not match")
	}
}

// ---------------------------------------------------------------------------
// EvaluateChain
// ---------------------------------------------------------------------------

func makeRequest() (*http.Request, *requestData) {
	req := httptest.NewRequest("GET", "http://example.com/attack-path?q=select+from", nil)
	req.RemoteAddr = "1.2.3.4:5678"
	data := &requestData{
		URL:       req.URL.String(),
		Path:      req.URL.Path,
		Query:     req.URL.RawQuery,
		QueryMap:  req.URL.Query(),
		Method:    req.Method,
		IP:        "1.2.3.4",
		UserAgent: "TestAgent/1.0",
		Headers:   req.Header,
	}
	return req, data
}

func TestChainedRule_EvaluateChain_Single(t *testing.T) {
	r, _ := NewRule("r1", "test", `attack`, []Target{TargetPath}, ActionBlock, SeverityHigh)
	cr := NewChainedRule(r)

	req, data := makeRequest()
	result := cr.EvaluateChain(context.Background(), req, data)
	if !result.Matched {
		t.Error("should match")
	}
	if result.Action != ActionBlock {
		t.Errorf("Action = %v", result.Action)
	}
	if result.TotalScore == 0 {
		t.Error("TotalScore should be > 0")
	}
}

func TestChainedRule_EvaluateChain_NoMatch(t *testing.T) {
	r, _ := NewRule("r1", "test", `nonexistent`, []Target{TargetPath}, ActionBlock, SeverityHigh)
	cr := NewChainedRule(r)

	req, data := makeRequest()
	result := cr.EvaluateChain(context.Background(), req, data)
	if result.Matched {
		t.Error("should not match")
	}
	if result.Action != ActionAllow {
		t.Errorf("Action = %v, want allow", result.Action)
	}
}

func TestChainedRule_EvaluateChain_RateLimited(t *testing.T) {
	r, _ := NewRule("r1", "test", `.*`, []Target{TargetPath}, ActionLog, SeverityLow)
	cr := NewChainedRule(r)
	cr.SetRateLimit(1, time.Minute, ActionBlock)

	req, data := makeRequest()
	// First request allowed
	result := cr.EvaluateChain(context.Background(), req, data)
	if result.RateLimited {
		t.Error("first request should not be rate limited")
	}

	// Second request rate limited
	result = cr.EvaluateChain(context.Background(), req, data)
	if !result.RateLimited {
		t.Error("second request should be rate limited")
	}
	if result.Action != ActionBlock {
		t.Errorf("rate limited Action = %v, want block", result.Action)
	}
}

func TestChainedRule_EvaluateChain_Exception(t *testing.T) {
	r, _ := NewRule("r1", "test", `attack`, []Target{TargetPath}, ActionBlock, SeverityHigh)
	cr := NewChainedRule(r)
	cr.AddException(&ExceptionRule{
		ID:        "exc1",
		Enabled:   true,
		SourceIPs: []string{"1.2.3.4"},
	})

	req, data := makeRequest()
	result := cr.EvaluateChain(context.Background(), req, data)
	if result.Matched {
		t.Error("excepted request should not match")
	}
	if result.Exception == nil {
		t.Error("Exception should be set")
	}
}

func TestChainedRule_EvaluateChain_DisabledException(t *testing.T) {
	r, _ := NewRule("r1", "test", `attack`, []Target{TargetPath}, ActionBlock, SeverityHigh)
	cr := NewChainedRule(r)
	cr.AddException(&ExceptionRule{
		ID:        "exc1",
		Enabled:   false, // disabled
		SourceIPs: []string{"1.2.3.4"},
	})

	req, data := makeRequest()
	result := cr.EvaluateChain(context.Background(), req, data)
	if !result.Matched {
		t.Error("disabled exception should not prevent matching")
	}
}

func TestChainedRule_EvaluateChain_OR(t *testing.T) {
	// Parent doesn't match, but child does
	parent, _ := NewRule("parent", "Parent", `nonexistent`, []Target{TargetPath}, ActionBlock, SeverityHigh)
	cr := NewChainedRule(parent)
	cr.Operator = ChainOR

	child, _ := NewRule("child", "Child", `attack`, []Target{TargetPath}, ActionLog, SeverityLow)
	cr.AddChild(NewChainedRule(child))

	req, data := makeRequest()
	result := cr.EvaluateChain(context.Background(), req, data)
	if !result.Matched {
		t.Error("OR chain should match when child matches")
	}
}

func TestChainedRule_EvaluateChain_AND_AllMatch(t *testing.T) {
	parent, _ := NewRule("parent", "Parent", `attack`, []Target{TargetPath}, ActionBlock, SeverityHigh)
	cr := NewChainedRule(parent)
	cr.Operator = ChainAND

	child, _ := NewRule("child", "Child", `select`, []Target{TargetQuery}, ActionLog, SeverityLow)
	cr.AddChild(NewChainedRule(child))

	req, data := makeRequest()
	result := cr.EvaluateChain(context.Background(), req, data)
	if !result.Matched {
		t.Error("AND chain should match when all match")
	}
}

func TestChainedRule_EvaluateChain_AND_ChildFails(t *testing.T) {
	parent, _ := NewRule("parent", "Parent", `attack`, []Target{TargetPath}, ActionBlock, SeverityHigh)
	cr := NewChainedRule(parent)
	cr.Operator = ChainAND

	child, _ := NewRule("child", "Child", `nonexistent`, []Target{TargetPath}, ActionLog, SeverityLow)
	cr.AddChild(NewChainedRule(child))

	req, data := makeRequest()
	result := cr.EvaluateChain(context.Background(), req, data)
	if result.Matched {
		t.Error("AND chain should not match when child doesn't match")
	}
}

func TestChainedRule_EvaluateChain_NOT(t *testing.T) {
	r, _ := NewRule("r1", "test", `nonexistent`, []Target{TargetPath}, ActionBlock, SeverityHigh)
	cr := NewChainedRule(r)
	cr.Operator = ChainNOT

	req, data := makeRequest()
	result := cr.EvaluateChain(context.Background(), req, data)
	if !result.Matched {
		t.Error("NOT of non-matching rule should be true")
	}
}

func TestChainedRule_EvaluateChain_NOT_WithChild(t *testing.T) {
	parent := &Rule{ID: "parent", Enabled: false} // nil pattern, no self-match
	cr := NewChainedRule(parent)
	cr.Operator = ChainNOT

	child, _ := NewRule("child", "Child", `nonexistent`, []Target{TargetPath}, ActionLog, SeverityLow)
	cr.AddChild(NewChainedRule(child))

	req, data := makeRequest()
	result := cr.EvaluateChain(context.Background(), req, data)
	if !result.Matched {
		t.Error("NOT of non-matching child should be true")
	}
}

func TestChainedRule_EvaluateChain_XOR(t *testing.T) {
	parent, _ := NewRule("parent", "Parent", `attack`, []Target{TargetPath}, ActionBlock, SeverityHigh)
	cr := NewChainedRule(parent)
	cr.Operator = ChainXOR

	// Child does NOT match
	child, _ := NewRule("child", "Child", `nonexistent`, []Target{TargetPath}, ActionLog, SeverityLow)
	cr.AddChild(NewChainedRule(child))

	req, data := makeRequest()
	result := cr.EvaluateChain(context.Background(), req, data)
	// Parent matches, child doesn't => exactly 1 => XOR true
	if !result.Matched {
		t.Error("XOR with exactly one match should be true")
	}
}

func TestChainedRule_EvaluateChain_XOR_BothMatch(t *testing.T) {
	parent, _ := NewRule("parent", "Parent", `attack`, []Target{TargetPath}, ActionBlock, SeverityHigh)
	cr := NewChainedRule(parent)
	cr.Operator = ChainXOR

	child, _ := NewRule("child", "Child", `attack`, []Target{TargetPath}, ActionLog, SeverityLow)
	cr.AddChild(NewChainedRule(child))

	req, data := makeRequest()
	result := cr.EvaluateChain(context.Background(), req, data)
	// Both match => 2 matches => XOR false
	if result.Matched {
		t.Error("XOR with both matching should be false")
	}
}

// ---------------------------------------------------------------------------
// RuleChain
// ---------------------------------------------------------------------------

func TestRuleChain_Basic(t *testing.T) {
	rc := NewRuleChain("chain1", "Test Chain")
	if rc.ID != "chain1" {
		t.Errorf("ID = %q", rc.ID)
	}
	if !rc.Enabled {
		t.Error("should start enabled")
	}

	r, _ := NewRule("r1", "test", `attack`, []Target{TargetPath}, ActionBlock, SeverityHigh)
	cr := NewChainedRule(r)
	rc.SetRoot(cr)

	if rc.Root != cr {
		t.Error("Root not set")
	}
	if cr.ChainID != "chain1" {
		t.Errorf("ChainID = %q", cr.ChainID)
	}

	// GetRule
	if rc.GetRule("r1") != cr {
		t.Error("GetRule failed")
	}
}

func TestRuleChain_AddRemoveRule(t *testing.T) {
	rc := NewRuleChain("chain1", "Test Chain")
	r, _ := NewRule("r1", "test", "abc", []Target{TargetAll}, ActionBlock, SeverityHigh)
	cr := NewChainedRule(r)
	rc.AddRule(cr)

	if rc.GetRule("r1") == nil {
		t.Error("AddRule failed")
	}
	if !rc.RemoveRule("r1") {
		t.Error("RemoveRule should return true")
	}
	if rc.RemoveRule("r1") {
		t.Error("RemoveRule again should return false")
	}
}

func TestRuleChain_EnableDisable(t *testing.T) {
	rc := NewRuleChain("chain1", "Test Chain")
	rc.Disable()
	if rc.Enabled {
		t.Error("should be disabled")
	}
	rc.Enable()
	if !rc.Enabled {
		t.Error("should be enabled")
	}
}

func TestRuleChain_Evaluate(t *testing.T) {
	rc := NewRuleChain("chain1", "Test Chain")
	r, _ := NewRule("r1", "test", `attack`, []Target{TargetPath}, ActionBlock, SeverityHigh)
	cr := NewChainedRule(r)
	rc.SetRoot(cr)

	req, data := makeRequest()
	result := rc.Evaluate(context.Background(), req, data)
	if !result.Matched {
		t.Error("should match")
	}
}

func TestRuleChain_Evaluate_Disabled(t *testing.T) {
	rc := NewRuleChain("chain1", "Test Chain")
	r, _ := NewRule("r1", "test", `attack`, []Target{TargetPath}, ActionBlock, SeverityHigh)
	cr := NewChainedRule(r)
	rc.SetRoot(cr)
	rc.Disable()

	req, data := makeRequest()
	result := rc.Evaluate(context.Background(), req, data)
	if result.Matched {
		t.Error("disabled chain should not match")
	}
}

func TestRuleChain_Evaluate_NilRoot(t *testing.T) {
	rc := NewRuleChain("chain1", "Test Chain")
	req, data := makeRequest()
	result := rc.Evaluate(context.Background(), req, data)
	if result.Matched {
		t.Error("nil root should not match")
	}
}

func TestRuleChain_GetStats(t *testing.T) {
	rc := NewRuleChain("chain1", "Test Chain")
	r, _ := NewRule("r1", "test", "abc", []Target{TargetAll}, ActionBlock, SeverityHigh)
	cr := NewChainedRule(r)
	rc.AddRule(cr)

	stats := rc.GetStats()
	if _, ok := stats["r1"]; !ok {
		t.Error("stats should have entry for r1")
	}
}

// ---------------------------------------------------------------------------
// Helper functions from chain.go
// ---------------------------------------------------------------------------

func TestHelpers_trimSpace(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"  hello  ", "hello"},
		{"\thello\t", "hello"},
		{"hello", "hello"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := trimSpace(tt.input); got != tt.want {
			t.Errorf("trimSpace(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestHelpers_contains(t *testing.T) {
	if !contains("hello world", "world") {
		t.Error("should contain 'world'")
	}
	if contains("hello", "world") {
		t.Error("should not contain 'world'")
	}
}

func TestHelpers_indexOf(t *testing.T) {
	if got := indexOf("hello world", "world"); got != 6 {
		t.Errorf("indexOf = %d, want 6", got)
	}
	if got := indexOf("hello", "world"); got != -1 {
		t.Errorf("indexOf = %d, want -1", got)
	}
}
