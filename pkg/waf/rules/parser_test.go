// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package rules

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Parser (JSON format)
// ---------------------------------------------------------------------------

func TestParser_ParseJSON(t *testing.T) {
	jsonInput := `{
		"name": "test-set",
		"description": "Test rules",
		"version": "1.0",
		"rules": [
			{
				"id": "r1",
				"name": "Rule 1",
				"pattern": "select.*from",
				"targets": ["query", "body"],
				"action": "block",
				"severity": "high",
				"enabled": true,
				"tags": ["sqli"]
			}
		]
	}`

	p := NewParser(true)
	rs, err := p.ParseJSON(strings.NewReader(jsonInput))
	if err != nil {
		t.Fatalf("ParseJSON: %v", err)
	}
	if rs.Name != "test-set" {
		t.Errorf("Name = %q", rs.Name)
	}
	rules := rs.ListRules()
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	r := rules[0]
	if r.ID != "r1" {
		t.Errorf("ID = %q", r.ID)
	}
	if r.Action != ActionBlock {
		t.Errorf("Action = %v", r.Action)
	}
	if r.Severity != SeverityHigh {
		t.Errorf("Severity = %v", r.Severity)
	}
	if len(r.Targets) != 2 {
		t.Errorf("Targets = %v", r.Targets)
	}
	if len(r.Tags) != 1 || r.Tags[0] != "sqli" {
		t.Errorf("Tags = %v", r.Tags)
	}
}

func TestParser_ParseJSONString(t *testing.T) {
	jsonInput := `{"name":"s","description":"d","version":"1","rules":[{"id":"r1","name":"n","pattern":"abc","targets":[],"action":"log","severity":"low","enabled":true}]}`
	p := NewParser(true)
	rs, err := p.ParseJSONString(jsonInput)
	if err != nil {
		t.Fatalf("ParseJSONString: %v", err)
	}
	if len(rs.ListRules()) != 1 {
		t.Errorf("expected 1 rule")
	}
}

func TestParser_ParseJSON_InvalidJSON(t *testing.T) {
	p := NewParser(true)
	_, err := p.ParseJSON(strings.NewReader(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestParser_ParseJSON_MissingID(t *testing.T) {
	jsonInput := `{"name":"s","description":"d","version":"1","rules":[{"id":"","name":"n","pattern":"abc","targets":[],"action":"block","severity":"high","enabled":true}]}`
	p := NewParser(true)
	_, err := p.ParseJSONString(jsonInput)
	if err == nil {
		t.Error("expected error for missing rule ID in strict mode")
	}
}

func TestParser_ParseJSON_MissingPattern(t *testing.T) {
	jsonInput := `{"name":"s","description":"d","version":"1","rules":[{"id":"r1","name":"n","pattern":"","targets":[],"action":"block","severity":"high","enabled":true}]}`
	p := NewParser(true)
	_, err := p.ParseJSONString(jsonInput)
	if err == nil {
		t.Error("expected error for missing pattern in strict mode")
	}
}

func TestParser_ParseJSON_InvalidPattern(t *testing.T) {
	jsonInput := `{"name":"s","description":"d","version":"1","rules":[{"id":"r1","name":"n","pattern":"[invalid","targets":[],"action":"block","severity":"high","enabled":true}]}`
	p := NewParser(true)
	_, err := p.ParseJSONString(jsonInput)
	if err == nil {
		t.Error("expected error for invalid regex pattern in strict mode")
	}
}

func TestParser_ParseJSON_NonStrict_SkipsBadRules(t *testing.T) {
	jsonInput := `{"name":"s","description":"d","version":"1","rules":[
		{"id":"","name":"bad","pattern":"abc","targets":[],"action":"block","severity":"high","enabled":true},
		{"id":"r1","name":"good","pattern":"abc","targets":[],"action":"block","severity":"high","enabled":true}
	]}`
	p := NewParser(false)
	rs, err := p.ParseJSONString(jsonInput)
	if err != nil {
		t.Fatalf("non-strict should not error: %v", err)
	}
	if len(rs.ListRules()) != 1 {
		t.Errorf("expected 1 rule (skipping bad), got %d", len(rs.ListRules()))
	}
}

func TestParser_ParseJSON_EmptyTargets(t *testing.T) {
	jsonInput := `{"name":"s","description":"d","version":"1","rules":[{"id":"r1","name":"n","pattern":"abc","targets":[],"action":"block","severity":"high","enabled":true}]}`
	p := NewParser(true)
	rs, err := p.ParseJSONString(jsonInput)
	if err != nil {
		t.Fatalf("ParseJSONString: %v", err)
	}
	r := rs.ListRules()[0]
	// Empty targets should default to TargetAll
	if len(r.Targets) != 1 || r.Targets[0] != TargetAll {
		t.Errorf("expected default TargetAll, got %v", r.Targets)
	}
}

func TestParser_ParseJSON_CustomScore(t *testing.T) {
	jsonInput := `{"name":"s","description":"d","version":"1","rules":[{"id":"r1","name":"n","pattern":"abc","targets":[],"action":"block","severity":"high","enabled":true,"score":99}]}`
	p := NewParser(true)
	rs, err := p.ParseJSONString(jsonInput)
	if err != nil {
		t.Fatalf("ParseJSONString: %v", err)
	}
	if rs.ListRules()[0].Score != 99 {
		t.Errorf("Score = %d, want 99", rs.ListRules()[0].Score)
	}
}

func TestParser_ParseJSON_AllActions(t *testing.T) {
	actions := []struct {
		str  string
		want Action
	}{
		{"allow", ActionAllow},
		{"block", ActionBlock},
		{"log", ActionLog},
		{"redirect", ActionRedirect},
		{"challenge", ActionChallenge},
	}
	for _, tt := range actions {
		got, err := parseAction(tt.str)
		if err != nil {
			t.Fatalf("parseAction(%q): %v", tt.str, err)
		}
		if got != tt.want {
			t.Errorf("parseAction(%q) = %v, want %v", tt.str, got, tt.want)
		}
	}
	// Invalid
	_, err := parseAction("invalid")
	if err == nil {
		t.Error("expected error for invalid action")
	}
}

func TestParser_ParseJSON_AllSeverities(t *testing.T) {
	sevs := []struct {
		str  string
		want Severity
	}{
		{"info", SeverityInfo},
		{"low", SeverityLow},
		{"medium", SeverityMedium},
		{"med", SeverityMedium},
		{"high", SeverityHigh},
		{"critical", SeverityCritical},
		{"crit", SeverityCritical},
	}
	for _, tt := range sevs {
		got, err := parseSeverity(tt.str)
		if err != nil {
			t.Fatalf("parseSeverity(%q): %v", tt.str, err)
		}
		if got != tt.want {
			t.Errorf("parseSeverity(%q) = %v, want %v", tt.str, got, tt.want)
		}
	}
	_, err := parseSeverity("invalid")
	if err == nil {
		t.Error("expected error for invalid severity")
	}
}

func TestParser_ParseJSON_AllTargets(t *testing.T) {
	targets := []struct {
		str  string
		want Target
	}{
		{"url", TargetURL},
		{"path", TargetPath},
		{"query", TargetQuery},
		{"body", TargetBody},
		{"headers", TargetHeaders},
		{"cookies", TargetCookies},
		{"method", TargetMethod},
		{"ip", TargetIP},
		{"useragent", TargetUserAgent},
		{"user_agent", TargetUserAgent},
		{"user-agent", TargetUserAgent},
		{"all", TargetAll},
		{"*", TargetAll},
	}
	for _, tt := range targets {
		got, err := parseTarget(tt.str)
		if err != nil {
			t.Fatalf("parseTarget(%q): %v", tt.str, err)
		}
		if got != tt.want {
			t.Errorf("parseTarget(%q) = %v, want %v", tt.str, got, tt.want)
		}
	}
	_, err := parseTarget("invalid")
	if err == nil {
		t.Error("expected error for invalid target")
	}
}

// ---------------------------------------------------------------------------
// Parser (Simple format)
// ---------------------------------------------------------------------------

func TestParser_ParseSimple(t *testing.T) {
	input := `# Comment
r1|Rule 1|select.*from|query|block|high
r2|Rule 2|<script>|body,headers|log|medium

# Another comment
r3|Rule 3|../|path|block|critical
`
	p := NewParser(true)
	rs, err := p.ParseSimple(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseSimple: %v", err)
	}
	rules := rs.ListRules()
	if len(rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(rules))
	}
	if rules[0].ID != "r1" {
		t.Errorf("first rule ID = %q", rules[0].ID)
	}
	if rules[0].Severity != SeverityHigh {
		t.Errorf("first rule severity = %v", rules[0].Severity)
	}
}

func TestParser_ParseSimple_DefaultActionSeverity(t *testing.T) {
	// Only 4 fields: defaults to block/medium
	input := `r1|Rule 1|abc|path`
	p := NewParser(true)
	rs, err := p.ParseSimple(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseSimple: %v", err)
	}
	r := rs.ListRules()[0]
	if r.Action != ActionBlock {
		t.Errorf("Action = %v, want block", r.Action)
	}
	if r.Severity != SeverityMedium {
		t.Errorf("Severity = %v, want medium", r.Severity)
	}
}

func TestParser_ParseSimple_InsufficientFields_Strict(t *testing.T) {
	input := `r1|Rule 1|pattern`
	p := NewParser(true)
	_, err := p.ParseSimple(strings.NewReader(input))
	if err == nil {
		t.Error("expected error for insufficient fields in strict mode")
	}
}

func TestParser_ParseSimple_InsufficientFields_NonStrict(t *testing.T) {
	input := `r1|Rule 1|pattern
r2|Rule 2|abc|path`
	p := NewParser(false)
	rs, err := p.ParseSimple(strings.NewReader(input))
	if err != nil {
		t.Fatalf("non-strict should not error: %v", err)
	}
	if len(rs.ListRules()) != 1 {
		t.Errorf("expected 1 valid rule, got %d", len(rs.ListRules()))
	}
}

func TestParser_ParseSimple_InvalidPattern_Strict(t *testing.T) {
	input := `r1|Rule 1|[invalid|path|block|high`
	p := NewParser(true)
	_, err := p.ParseSimple(strings.NewReader(input))
	if err == nil {
		t.Error("expected error for invalid pattern")
	}
}

// ---------------------------------------------------------------------------
// ExportJSON
// ---------------------------------------------------------------------------

func TestExportJSON(t *testing.T) {
	rs := NewRuleSet("export-test", "Export test set")
	r1, _ := NewRule("r1", "Rule 1", "abc", []Target{TargetPath, TargetQuery}, ActionBlock, SeverityHigh)
	r1.Description = "test desc"
	r1.Tags = []string{"tag1", "tag2"}
	r1.Message = "blocked"
	r1.RedirectURL = "/blocked"
	rs.AddRule(r1)

	var buf bytes.Buffer
	err := ExportJSON(rs, &buf)
	if err != nil {
		t.Fatalf("ExportJSON: %v", err)
	}

	// Parse back
	var config RuleSetConfig
	if err := json.Unmarshal(buf.Bytes(), &config); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if config.Name != "export-test" {
		t.Errorf("Name = %q", config.Name)
	}
	if config.Version != "1.0" {
		t.Errorf("Version = %q", config.Version)
	}
	if len(config.Rules) != 1 {
		t.Fatalf("expected 1 rule")
	}
	rc := config.Rules[0]
	if rc.ID != "r1" {
		t.Errorf("rule ID = %q", rc.ID)
	}
	if rc.Action != "block" {
		t.Errorf("rule action = %q", rc.Action)
	}
	if rc.Severity != "high" {
		t.Errorf("rule severity = %q", rc.Severity)
	}
	if len(rc.Targets) != 2 {
		t.Errorf("rule targets = %v", rc.Targets)
	}
}

// ---------------------------------------------------------------------------
// targetToString
// ---------------------------------------------------------------------------

func TestTargetToString(t *testing.T) {
	tests := []struct {
		target Target
		want   string
	}{
		{TargetURL, "url"},
		{TargetPath, "path"},
		{TargetQuery, "query"},
		{TargetBody, "body"},
		{TargetHeaders, "headers"},
		{TargetCookies, "cookies"},
		{TargetMethod, "method"},
		{TargetIP, "ip"},
		{TargetUserAgent, "useragent"},
		{TargetAll, "all"},
		{Target(99), "unknown"},
	}
	for _, tt := range tests {
		if got := targetToString(tt.target); got != tt.want {
			t.Errorf("targetToString(%d) = %q, want %q", tt.target, got, tt.want)
		}
	}
}
