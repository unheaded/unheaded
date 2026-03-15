// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package rules

import (
	"testing"
)

// ---------------------------------------------------------------------------
// Action.String
// ---------------------------------------------------------------------------

func TestAction_String(t *testing.T) {
	tests := []struct {
		action Action
		want   string
	}{
		{ActionAllow, "allow"},
		{ActionBlock, "block"},
		{ActionLog, "log"},
		{ActionRedirect, "redirect"},
		{ActionChallenge, "challenge"},
		{Action(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.action.String(); got != tt.want {
			t.Errorf("Action(%d).String() = %q, want %q", tt.action, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Severity.String
// ---------------------------------------------------------------------------

func TestSeverity_String(t *testing.T) {
	tests := []struct {
		severity Severity
		want     string
	}{
		{SeverityInfo, "info"},
		{SeverityLow, "low"},
		{SeverityMedium, "medium"},
		{SeverityHigh, "high"},
		{SeverityCritical, "critical"},
		{Severity(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.severity.String(); got != tt.want {
			t.Errorf("Severity(%d).String() = %q, want %q", tt.severity, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// severityToScore
// ---------------------------------------------------------------------------

func TestSeverityToScore(t *testing.T) {
	tests := []struct {
		severity Severity
		want     int
	}{
		{SeverityInfo, 1},
		{SeverityLow, 2},
		{SeverityMedium, 5},
		{SeverityHigh, 10},
		{SeverityCritical, 20},
		{Severity(99), 0},
	}
	for _, tt := range tests {
		if got := severityToScore(tt.severity); got != tt.want {
			t.Errorf("severityToScore(%d) = %d, want %d", tt.severity, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// NewRule
// ---------------------------------------------------------------------------

func TestNewRule(t *testing.T) {
	r, err := NewRule("id1", "name1", `foo\d+`, []Target{TargetPath}, ActionBlock, SeverityHigh)
	if err != nil {
		t.Fatalf("NewRule: %v", err)
	}
	if r.ID != "id1" {
		t.Errorf("ID = %q, want id1", r.ID)
	}
	if r.Name != "name1" {
		t.Errorf("Name = %q, want name1", r.Name)
	}
	if r.RawPattern != `foo\d+` {
		t.Errorf("RawPattern = %q", r.RawPattern)
	}
	if r.Pattern == nil {
		t.Fatal("Pattern is nil")
	}
	if r.Action != ActionBlock {
		t.Errorf("Action = %v", r.Action)
	}
	if r.Severity != SeverityHigh {
		t.Errorf("Severity = %v", r.Severity)
	}
	if r.Score != 10 {
		t.Errorf("Score = %d, want 10", r.Score)
	}
	if !r.Enabled {
		t.Error("Enabled should be true")
	}
	if len(r.Targets) != 1 || r.Targets[0] != TargetPath {
		t.Errorf("Targets = %v", r.Targets)
	}
}

func TestNewRule_InvalidPattern(t *testing.T) {
	_, err := NewRule("id1", "name1", `[invalid`, []Target{TargetAll}, ActionBlock, SeverityHigh)
	if err == nil {
		t.Error("expected error for invalid pattern")
	}
}

// ---------------------------------------------------------------------------
// Rule.Enable / Disable / IsEnabled
// ---------------------------------------------------------------------------

func TestRule_EnableDisable(t *testing.T) {
	r, _ := NewRule("r1", "test", "abc", []Target{TargetAll}, ActionBlock, SeverityLow)
	if !r.IsEnabled() {
		t.Fatal("should start enabled")
	}
	r.Disable()
	if r.IsEnabled() {
		t.Fatal("should be disabled")
	}
	r.Enable()
	if !r.IsEnabled() {
		t.Fatal("should be re-enabled")
	}
}

// ---------------------------------------------------------------------------
// Rule.Match
// ---------------------------------------------------------------------------

func TestRule_Match(t *testing.T) {
	r, _ := NewRule("r1", "test", "select.*from", []Target{TargetAll}, ActionBlock, SeverityHigh)

	if !r.Match("SELECT foo FROM bar") {
		t.Error("expected match (case insensitive)")
	}
	if r.Match("nothing here") {
		t.Error("unexpected match")
	}

	// Disabled rule should not match
	r.Disable()
	if r.Match("SELECT foo FROM bar") {
		t.Error("disabled rule should not match")
	}
}

func TestRule_Match_NilPattern(t *testing.T) {
	r := &Rule{Enabled: true, Pattern: nil}
	if r.Match("anything") {
		t.Error("nil pattern should not match")
	}
}

// ---------------------------------------------------------------------------
// Rule.FindMatches
// ---------------------------------------------------------------------------

func TestRule_FindMatches(t *testing.T) {
	r, _ := NewRule("r1", "test", `\d+`, []Target{TargetAll}, ActionBlock, SeverityLow)

	matches := r.FindMatches("abc 123 def 456")
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
	if matches[0] != "123" || matches[1] != "456" {
		t.Errorf("matches = %v", matches)
	}

	// Disabled
	r.Disable()
	if got := r.FindMatches("abc 123"); got != nil {
		t.Errorf("disabled rule FindMatches should return nil, got %v", got)
	}
}

func TestRule_FindMatches_NilPattern(t *testing.T) {
	r := &Rule{Enabled: true, Pattern: nil}
	if got := r.FindMatches("anything"); got != nil {
		t.Errorf("nil pattern FindMatches should return nil, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// Rule.Clone
// ---------------------------------------------------------------------------

func TestRule_Clone(t *testing.T) {
	r, _ := NewRule("r1", "test", "abc", []Target{TargetPath, TargetQuery}, ActionBlock, SeverityHigh)
	r.Description = "desc"
	r.Tags = []string{"sqli", "owasp"}
	r.Message = "blocked"
	r.RedirectURL = "/blocked"

	c := r.Clone()

	// Verify fields copied
	if c.ID != r.ID || c.Name != r.Name || c.Description != r.Description {
		t.Error("basic fields not cloned")
	}
	if c.RawPattern != r.RawPattern {
		t.Error("RawPattern not cloned")
	}
	if c.Action != r.Action || c.Severity != r.Severity || c.Score != r.Score {
		t.Error("action/severity/score not cloned")
	}
	if c.Enabled != r.Enabled {
		t.Error("Enabled not cloned")
	}
	if c.Message != r.Message || c.RedirectURL != r.RedirectURL {
		t.Error("Message/RedirectURL not cloned")
	}

	// Verify slices are independent copies
	if len(c.Tags) != 2 || c.Tags[0] != "sqli" {
		t.Error("Tags not cloned")
	}
	c.Tags[0] = "modified"
	if r.Tags[0] == "modified" {
		t.Error("Tags slice not independent")
	}

	if len(c.Targets) != 2 {
		t.Error("Targets not cloned")
	}
	c.Targets[0] = TargetBody
	if r.Targets[0] == TargetBody {
		t.Error("Targets slice not independent")
	}
}

// ---------------------------------------------------------------------------
// RuleSet
// ---------------------------------------------------------------------------

func TestRuleSet_AddGetRemoveList(t *testing.T) {
	rs := NewRuleSet("test-set", "A test set")
	if rs.Name != "test-set" || rs.Description != "A test set" {
		t.Error("NewRuleSet fields incorrect")
	}

	r1, _ := NewRule("r1", "rule1", "abc", []Target{TargetAll}, ActionBlock, SeverityLow)
	r2, _ := NewRule("r2", "rule2", "def", []Target{TargetAll}, ActionLog, SeverityMedium)

	rs.AddRule(r1)
	rs.AddRule(r2)

	// GetRule
	if got := rs.GetRule("r1"); got != r1 {
		t.Error("GetRule r1 failed")
	}
	if got := rs.GetRule("r2"); got != r2 {
		t.Error("GetRule r2 failed")
	}
	if got := rs.GetRule("nonexistent"); got != nil {
		t.Error("GetRule nonexistent should return nil")
	}

	// ListRules
	list := rs.ListRules()
	if len(list) != 2 {
		t.Fatalf("ListRules: expected 2, got %d", len(list))
	}

	// RemoveRule
	if !rs.RemoveRule("r1") {
		t.Error("RemoveRule r1 should return true")
	}
	if rs.RemoveRule("r1") {
		t.Error("RemoveRule r1 again should return false")
	}
	if rs.GetRule("r1") != nil {
		t.Error("r1 should be removed")
	}
	if len(rs.ListRules()) != 1 {
		t.Error("should have 1 rule remaining")
	}
}
