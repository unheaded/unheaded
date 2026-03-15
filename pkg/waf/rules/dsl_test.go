// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package rules

import (
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// DSL tokenizer + parser
// ---------------------------------------------------------------------------

func TestDSLParser_BasicRule(t *testing.T) {
	input := `
	RULE sqli-001 "SQL Injection" {
		MATCH /select.*from/ ON QUERY AND BODY
		ACTION block
		SEVERITY critical
		SCORE 25
		MESSAGE "SQL injection attempt"
		TAGS sqli, owasp
	}
	`
	parser := NewDSLParser(true)
	rules, err := parser.ParseString(input)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	r := rules[0]
	if r.ID != "sqli-001" {
		t.Errorf("ID = %q", r.ID)
	}
	if r.Name != "SQL Injection" {
		t.Errorf("Name = %q", r.Name)
	}
	if r.Action != ActionBlock {
		t.Errorf("Action = %v", r.Action)
	}
	if r.Severity != SeverityCritical {
		t.Errorf("Severity = %v", r.Severity)
	}
	if r.Score != 25 {
		t.Errorf("Score = %d", r.Score)
	}
	if r.Message != "SQL injection attempt" {
		t.Errorf("Message = %q", r.Message)
	}
	if len(r.Tags) != 2 || r.Tags[0] != "sqli" || r.Tags[1] != "owasp" {
		t.Errorf("Tags = %v", r.Tags)
	}
	if len(r.Targets) != 2 {
		t.Errorf("expected 2 targets, got %d", len(r.Targets))
	}
}

func TestDSLParser_AllActions(t *testing.T) {
	actions := []struct {
		keyword string
		want    Action
	}{
		{"block", ActionBlock},
		{"allow", ActionAllow},
		{"log", ActionLog},
		{"challenge", ActionChallenge},
	}
	for _, tt := range actions {
		input := `RULE test "test" { MATCH "abc" ACTION ` + tt.keyword + ` }`
		parser := NewDSLParser(true)
		rules, err := parser.ParseString(input)
		if err != nil {
			t.Fatalf("action %q: %v", tt.keyword, err)
		}
		if rules[0].Action != tt.want {
			t.Errorf("action %q: got %v, want %v", tt.keyword, rules[0].Action, tt.want)
		}
	}
}

func TestDSLParser_RedirectAction(t *testing.T) {
	input := `RULE redir "Redirect" { MATCH "test" ACTION redirect "/blocked" }`
	parser := NewDSLParser(true)
	rules, err := parser.ParseString(input)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	if rules[0].Action != ActionRedirect {
		t.Errorf("Action = %v, want redirect", rules[0].Action)
	}
	if rules[0].RedirectURL != "/blocked" {
		t.Errorf("RedirectURL = %q", rules[0].RedirectURL)
	}
}

func TestDSLParser_AllSeverities(t *testing.T) {
	sevs := []struct {
		keyword  string
		want     Severity
		wantScore int
	}{
		{"critical", SeverityCritical, 20},
		{"high", SeverityHigh, 10},
		{"medium", SeverityMedium, 5},
		{"low", SeverityLow, 2},
		{"info", SeverityInfo, 1},
	}
	for _, tt := range sevs {
		input := `RULE test "test" { MATCH "abc" SEVERITY ` + tt.keyword + ` }`
		parser := NewDSLParser(true)
		rules, err := parser.ParseString(input)
		if err != nil {
			t.Fatalf("severity %q: %v", tt.keyword, err)
		}
		if rules[0].Severity != tt.want {
			t.Errorf("severity %q: got %v", tt.keyword, rules[0].Severity)
		}
		if rules[0].Score != tt.wantScore {
			t.Errorf("severity %q: score = %d, want %d", tt.keyword, rules[0].Score, tt.wantScore)
		}
	}
}

func TestDSLParser_AllTargets(t *testing.T) {
	targets := []struct {
		keyword string
		want    Target
	}{
		{"URL", TargetURL},
		{"PATH", TargetPath},
		{"QUERY", TargetQuery},
		{"BODY", TargetBody},
		{"HEADERS", TargetHeaders},
		{"COOKIES", TargetCookies},
		{"METHOD", TargetMethod},
		{"IP", TargetIP},
		{"USERAGENT", TargetUserAgent},
		{"ALL", TargetAll},
	}
	for _, tt := range targets {
		input := `RULE test "test" { MATCH "abc" ON ` + tt.keyword + ` }`
		parser := NewDSLParser(true)
		rules, err := parser.ParseString(input)
		if err != nil {
			t.Fatalf("target %q: %v", tt.keyword, err)
		}
		if len(rules[0].Targets) != 1 || rules[0].Targets[0] != tt.want {
			t.Errorf("target %q: got %v", tt.keyword, rules[0].Targets)
		}
	}
}

func TestDSLParser_EnabledDisabled(t *testing.T) {
	input := `RULE test1 "enabled" { MATCH "abc" ENABLED }
	          RULE test2 "disabled" { MATCH "abc" DISABLED }`
	parser := NewDSLParser(true)
	rules, err := parser.ParseString(input)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
	if !rules[0].Enabled {
		t.Error("first rule should be enabled")
	}
	if rules[1].Enabled {
		t.Error("second rule should be disabled")
	}
}

func TestDSLParser_VersionAndPriority(t *testing.T) {
	input := `RULE test "test" { MATCH "abc" VERSION 3 PRIORITY 10 }`
	parser := NewDSLParser(true)
	rules, err := parser.ParseString(input)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	if rules[0].Version != 3 {
		t.Errorf("Version = %d, want 3", rules[0].Version)
	}
	if rules[0].Priority != 10 {
		t.Errorf("Priority = %d, want 10", rules[0].Priority)
	}
}

func TestDSLParser_Chain(t *testing.T) {
	input := `
	RULE parent "Parent" {
		MATCH "attack" ON PATH
		CHAIN AND {
			RULE child1 "Child1" {
				MATCH "sqli" ON QUERY
			}
		}
	}`
	parser := NewDSLParser(true)
	rules, err := parser.ParseString(input)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule")
	}
	if rules[0].Operator != ChainAND {
		t.Errorf("Operator = %v, want AND", rules[0].Operator)
	}
	if len(rules[0].Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(rules[0].Children))
	}
	if rules[0].Children[0].ID != "child1" {
		t.Errorf("child ID = %q", rules[0].Children[0].ID)
	}
}

func TestDSLParser_ChainOperators(t *testing.T) {
	ops := []struct {
		keyword string
		want    ChainOperator
	}{
		{"AND", ChainAND},
		{"OR", ChainOR},
		{"NOT", ChainNOT},
		{"XOR", ChainXOR},
	}
	for _, tt := range ops {
		input := `RULE test "test" { MATCH "abc" CHAIN ` + tt.keyword + ` { RULE c "c" { MATCH "def" } } }`
		parser := NewDSLParser(true)
		rules, err := parser.ParseString(input)
		if err != nil {
			t.Fatalf("chain op %q: %v", tt.keyword, err)
		}
		if rules[0].Operator != tt.want {
			t.Errorf("chain op %q: got %v", tt.keyword, rules[0].Operator)
		}
	}
}

func TestDSLParser_RateLimit(t *testing.T) {
	input := `RULE test "test" { MATCH "abc" RATELIMIT 100 PER minute }`
	parser := NewDSLParser(true)
	rules, err := parser.ParseString(input)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	rl := rules[0].RateLimit
	if rl == nil {
		t.Fatal("RateLimit is nil")
	}
	if rl.MaxRequests != 100 {
		t.Errorf("MaxRequests = %d", rl.MaxRequests)
	}
	if rl.Window != time.Minute {
		t.Errorf("Window = %v", rl.Window)
	}
}

func TestDSLParser_Exception(t *testing.T) {
	input := `
	RULE test "test" {
		MATCH "abc" ON PATH
		EXCEPT {
			IP "192.168.1.1"
			PATH "/api/health"
			METHOD GET, POST
		}
	}`
	parser := NewDSLParser(true)
	rules, err := parser.ParseString(input)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	if len(rules[0].Exceptions) != 1 {
		t.Fatalf("expected 1 exception, got %d", len(rules[0].Exceptions))
	}
	exc := rules[0].Exceptions[0]
	if len(exc.SourceIPs) != 1 || exc.SourceIPs[0] != "192.168.1.1" {
		t.Errorf("SourceIPs = %v", exc.SourceIPs)
	}
	if len(exc.Paths) != 1 {
		t.Errorf("Paths = %v", exc.Paths)
	}
	if len(exc.Methods) != 2 {
		t.Errorf("Methods = %v", exc.Methods)
	}
}

func TestDSLParser_Comments(t *testing.T) {
	input := `
	# This is a comment
	// This is also a comment
	/* Multi-line
	   comment */
	RULE test "test" {
		MATCH "abc"
	}`
	parser := NewDSLParser(true)
	rules, err := parser.ParseString(input)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
}

func TestDSLParser_MultipleRules(t *testing.T) {
	input := `
	RULE r1 "Rule1" { MATCH "abc" }
	RULE r2 "Rule2" { MATCH "def" }
	RULE r3 "Rule3" { MATCH "ghi" }`
	parser := NewDSLParser(true)
	rules, err := parser.ParseString(input)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	if len(rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(rules))
	}
}

func TestDSLParser_StrictError(t *testing.T) {
	// Missing opening brace
	input := `RULE test "test" MATCH "abc" }`
	parser := NewDSLParser(true)
	_, err := parser.ParseString(input)
	if err == nil {
		t.Error("expected error in strict mode")
	}
}

func TestDSLParser_NonStrictSkipsBadRules(t *testing.T) {
	input := `
	RULE bad_rule {
	RULE good "good" { MATCH "abc" }`
	parser := NewDSLParser(false)
	rules, err := parser.ParseString(input)
	if err != nil {
		t.Fatalf("non-strict should not error: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 valid rule, got %d", len(rules))
	}
}

func TestDSLParser_UnterminatedString(t *testing.T) {
	input := `RULE test "unterminated`
	parser := NewDSLParser(true)
	_, err := parser.ParseString(input)
	if err == nil {
		t.Error("expected error for unterminated string")
	}
}

func TestDSLParser_UnterminatedPattern(t *testing.T) {
	input := `RULE test "test" { MATCH /unterminated }`
	parser := NewDSLParser(true)
	_, err := parser.ParseString(input)
	if err == nil {
		t.Error("expected error for unterminated pattern")
	}
}

func TestDSLParser_ParseViaReader(t *testing.T) {
	input := `RULE test "test" { MATCH "abc" }`
	parser := NewDSLParser(true)
	rules, err := parser.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule")
	}
}

func TestDSLParser_StringMatchPattern(t *testing.T) {
	input := `RULE test "test" { MATCH "some-pattern" ON PATH }`
	parser := NewDSLParser(true)
	rules, err := parser.ParseString(input)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	if rules[0].RawPattern != "some-pattern" {
		t.Errorf("RawPattern = %q", rules[0].RawPattern)
	}
}

func TestDSLParser_TimeUnits(t *testing.T) {
	parser := NewDSLParser(true)
	tests := []struct {
		unit string
		want time.Duration
	}{
		{"SECOND", time.Second},
		{"SECONDS", time.Second},
		{"MINUTE", time.Minute},
		{"MINUTES", time.Minute},
		{"HOUR", time.Hour},
		{"HOURS", time.Hour},
		{"DAY", 24 * time.Hour},
		{"DAYS", 24 * time.Hour},
		{"unknown", time.Minute}, // default
	}
	for _, tt := range tests {
		got := parser.parseTimeUnit(tt.unit)
		if got != tt.want {
			t.Errorf("parseTimeUnit(%q) = %v, want %v", tt.unit, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// CompileDSL / CompileDSLToChains
// ---------------------------------------------------------------------------

func TestCompileDSL(t *testing.T) {
	src := `RULE test "test" { MATCH "abc" ON PATH ACTION block SEVERITY high }`
	rs, err := CompileDSL(src)
	if err != nil {
		t.Fatalf("CompileDSL: %v", err)
	}
	if rs.Name != "dsl-rules" {
		t.Errorf("RuleSet name = %q", rs.Name)
	}
	if len(rs.ListRules()) != 1 {
		t.Errorf("expected 1 rule, got %d", len(rs.ListRules()))
	}
}

func TestCompileDSL_Error(t *testing.T) {
	_, err := CompileDSL(`invalid DSL`)
	if err == nil {
		t.Error("expected error for invalid DSL")
	}
}

func TestCompileDSLToChains(t *testing.T) {
	src := `RULE test "test" { MATCH "abc" }`
	chains, err := CompileDSLToChains(src)
	if err != nil {
		t.Fatalf("CompileDSLToChains: %v", err)
	}
	if len(chains) != 1 {
		t.Errorf("expected 1 chain, got %d", len(chains))
	}
}

// ---------------------------------------------------------------------------
// Tokenizer edge cases
// ---------------------------------------------------------------------------

func TestDSLParser_SingleQuoteStrings(t *testing.T) {
	input := `RULE test 'test name' { MATCH 'pattern' }`
	parser := NewDSLParser(true)
	rules, err := parser.ParseString(input)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule")
	}
	if rules[0].Name != "test name" {
		t.Errorf("Name = %q", rules[0].Name)
	}
}

func TestDSLParser_EscapedStringChars(t *testing.T) {
	input := `RULE test "test \"name\"" { MATCH "abc" }`
	parser := NewDSLParser(true)
	// The tokenizer handles escaped chars by skipping them
	_, err := parser.ParseString(input)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
}

func TestDSLParser_UnexpectedCharacter(t *testing.T) {
	input := `RULE test "test" { @ }`
	parser := NewDSLParser(true)
	_, err := parser.ParseString(input)
	if err == nil {
		t.Error("expected error for unexpected character")
	}
}

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

func TestHelpers_toUpper(t *testing.T) {
	if got := toUpper("hello"); got != "HELLO" {
		t.Errorf("toUpper(hello) = %q", got)
	}
	if got := toUpper("Hello123"); got != "HELLO123" {
		t.Errorf("toUpper(Hello123) = %q", got)
	}
}

func TestHelpers_splitByChar(t *testing.T) {
	got := splitByChar("a-b-c", '-')
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("splitByChar = %v", got)
	}
	// No separator
	got = splitByChar("abc", '-')
	if len(got) != 1 || got[0] != "abc" {
		t.Errorf("splitByChar no sep = %v", got)
	}
}

func TestHelpers_isDigit(t *testing.T) {
	if !isDigit('0') || !isDigit('9') {
		t.Error("digits should return true")
	}
	if isDigit('a') || isDigit(' ') {
		t.Error("non-digits should return false")
	}
}

func TestHelpers_isAlpha(t *testing.T) {
	if !isAlpha('a') || !isAlpha('Z') {
		t.Error("alpha should return true")
	}
	if isAlpha('0') || isAlpha(' ') {
		t.Error("non-alpha should return false")
	}
}

func TestHelpers_isAlphaNumeric(t *testing.T) {
	if !isAlphaNumeric('a') || !isAlphaNumeric('0') {
		t.Error("alphanumeric should return true")
	}
	if isAlphaNumeric(' ') || isAlphaNumeric('-') {
		t.Error("non-alphanumeric should return false")
	}
}

func TestGetKeywordToken(t *testing.T) {
	parser := NewDSLParser(true)
	tests := []struct {
		input string
		want  TokenType
	}{
		{"RULE", TokenRule},
		{"rule", TokenRule},
		{"MATCH", TokenMatch},
		{"ON", TokenOn},
		{"ACTION", TokenAction},
		{"SEVERITY", TokenSeverity},
		{"SCORE", TokenScore},
		{"MESSAGE", TokenMessage},
		{"TAGS", TokenTags},
		{"CHAIN", TokenChain},
		{"RATELIMIT", TokenRateLimit},
		{"PER", TokenPer},
		{"EXCEPT", TokenExcept},
		{"EXCEPTION", TokenExcept},
		{"IP", TokenIP},
		{"PATH", TokenPath},
		{"METHOD", TokenMethod},
		{"HEADER", TokenHeader},
		{"TIME", TokenTime},
		{"COUNTRY", TokenCountry},
		{"USERAGENT", TokenUserAgent},
		{"UA", TokenUserAgent},
		{"AND", TokenAnd},
		{"OR", TokenOr},
		{"NOT", TokenNot},
		{"XOR", TokenXor},
		{"VERSION", TokenVersion},
		{"PRIORITY", TokenPriority},
		{"ENABLED", TokenEnabled},
		{"DISABLED", TokenDisabled},
		{"URL", TokenTarget},
		{"QUERY", TokenTarget},
		{"BODY", TokenTarget},
		{"HEADERS", TokenTarget},
		{"COOKIES", TokenTarget},
		{"ALL", TokenTarget},
		{"BLOCK", TokenActionValue},
		{"ALLOW", TokenActionValue},
		{"LOG", TokenActionValue},
		{"REDIRECT", TokenActionValue},
		{"CHALLENGE", TokenActionValue},
		{"CRITICAL", TokenSeverityValue},
		{"HIGH", TokenSeverityValue},
		{"MEDIUM", TokenSeverityValue},
		{"LOW", TokenSeverityValue},
		{"INFO", TokenSeverityValue},
		{"SECOND", TokenTimeUnit},
		{"MINUTES", TokenTimeUnit},
		{"HOURS", TokenTimeUnit},
		{"DAYS", TokenTimeUnit},
		{"random_ident", TokenIdent},
	}
	for _, tt := range tests {
		got := parser.getKeywordToken(tt.input)
		if got != tt.want {
			t.Errorf("getKeywordToken(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
