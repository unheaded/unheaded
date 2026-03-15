// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package rules

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/alerting"
)

// ---------------------------------------------------------------------------
// Mock MetricProvider
// ---------------------------------------------------------------------------

type mockMetricProvider struct {
	mu          sync.Mutex
	queryFunc   func(ctx context.Context, expr string, ts time.Time) (float64, error)
	rangeFunc   func(ctx context.Context, expr string, start, end time.Time, step time.Duration) ([]float64, error)
	queryCalls  int
	rangeCalls  int
}

func (m *mockMetricProvider) Query(ctx context.Context, expression string, timestamp time.Time) (float64, error) {
	m.mu.Lock()
	m.queryCalls++
	m.mu.Unlock()
	if m.queryFunc != nil {
		return m.queryFunc(ctx, expression, timestamp)
	}
	return 0, nil
}

func (m *mockMetricProvider) QueryRange(ctx context.Context, expression string, start, end time.Time, step time.Duration) ([]float64, error) {
	m.mu.Lock()
	m.rangeCalls++
	m.mu.Unlock()
	if m.rangeFunc != nil {
		return m.rangeFunc(ctx, expression, start, end, step)
	}
	return nil, nil
}

// ---------------------------------------------------------------------------
// 1. Rule Creation and Validation
// ---------------------------------------------------------------------------

func TestRuleBuilder_BasicCreation(t *testing.T) {
	rule, err := NewRuleBuilder("cpu-high", "High CPU Usage").
		WithType(alerting.RuleTypeThreshold).
		WithSeverity(alerting.SeverityCritical).
		WithExpression("avg(cpu_usage)").
		WithThreshold(90, alerting.OpGreaterThan).
		WithDescription("CPU usage exceeded 90%").
		WithLabel("team", "infra").
		WithAnnotation("runbook", "https://wiki/cpu-high").
		WithNotifyChannels("slack", "pagerduty").
		Build()

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if rule.ID != "cpu-high" {
		t.Errorf("expected ID 'cpu-high', got %q", rule.ID)
	}
	if rule.Name != "High CPU Usage" {
		t.Errorf("expected Name 'High CPU Usage', got %q", rule.Name)
	}
	if rule.Severity != alerting.SeverityCritical {
		t.Errorf("expected severity critical, got %q", rule.Severity)
	}
	if rule.Threshold != 90 {
		t.Errorf("expected threshold 90, got %f", rule.Threshold)
	}
	if rule.Operator != alerting.OpGreaterThan {
		t.Errorf("expected operator '>', got %q", rule.Operator)
	}
	if rule.Description != "CPU usage exceeded 90%" {
		t.Errorf("expected description set, got %q", rule.Description)
	}
	if !rule.Enabled {
		t.Error("expected rule to be enabled by default")
	}
	if rule.Labels["team"] != "infra" {
		t.Errorf("expected label team=infra, got %q", rule.Labels["team"])
	}
	if rule.Annotations["runbook"] != "https://wiki/cpu-high" {
		t.Errorf("expected annotation runbook set, got %q", rule.Annotations["runbook"])
	}
	if len(rule.NotifyChannels) != 2 {
		t.Errorf("expected 2 notify channels, got %d", len(rule.NotifyChannels))
	}
	if rule.EvaluationInterval != DefaultEvaluationInterval {
		t.Errorf("expected default evaluation interval, got %v", rule.EvaluationInterval)
	}
}

func TestRuleBuilder_Disabled(t *testing.T) {
	rule, err := NewRuleBuilder("r1", "Test").
		WithType(alerting.RuleTypeThreshold).
		WithExpression("up").
		WithThreshold(0, alerting.OpEqual).
		Disabled().
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rule.Enabled {
		t.Error("expected rule to be disabled")
	}
}

func TestRuleBuilder_CustomEvaluationInterval(t *testing.T) {
	rule, err := NewRuleBuilder("r1", "Test").
		WithType(alerting.RuleTypeThreshold).
		WithExpression("up").
		WithThreshold(0, alerting.OpEqual).
		WithEvaluationInterval(30 * time.Second).
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rule.EvaluationInterval != 30*time.Second {
		t.Errorf("expected 30s interval, got %v", rule.EvaluationInterval)
	}
}

func TestRuleBuilder_WithForPeriod(t *testing.T) {
	rule, err := NewRuleBuilder("r1", "Test").
		WithType(alerting.RuleTypeThreshold).
		WithExpression("up").
		WithThreshold(0, alerting.OpEqual).
		WithForPeriod(5 * time.Minute).
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rule.ForPeriod != 5*time.Minute {
		t.Errorf("expected 5m for period, got %v", rule.ForPeriod)
	}
}

func TestRuleBuilder_RateRule(t *testing.T) {
	rule, err := NewRuleBuilder("rate-1", "Error Rate").
		WithExpression("http_errors_total").
		WithRateWindow(5*time.Minute, 0.5).
		WithSeverity(alerting.SeverityWarning).
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rule.Type != alerting.RuleTypeRate {
		t.Errorf("expected type rate, got %q", rule.Type)
	}
	if rule.RateWindow != 5*time.Minute {
		t.Errorf("expected rate window 5m, got %v", rule.RateWindow)
	}
	if rule.RateIncrease != 0.5 {
		t.Errorf("expected rate increase 0.5, got %f", rule.RateIncrease)
	}
}

func TestRuleBuilder_AbsenceRule(t *testing.T) {
	rule, err := NewRuleBuilder("absence-1", "Metric Missing").
		WithExpression("heartbeat").
		WithAbsenceWindow(10 * time.Minute).
		WithSeverity(alerting.SeverityCritical).
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rule.Type != alerting.RuleTypeAbsence {
		t.Errorf("expected type absence, got %q", rule.Type)
	}
	if rule.AbsenceWindow != 10*time.Minute {
		t.Errorf("expected absence window 10m, got %v", rule.AbsenceWindow)
	}
}

func TestRuleBuilder_WithMetricName(t *testing.T) {
	rule, err := NewRuleBuilder("r1", "Test").
		WithType(alerting.RuleTypeThreshold).
		WithMetricName("cpu_usage").
		WithThreshold(90, alerting.OpGreaterThan).
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rule.MetricName != "cpu_usage" {
		t.Errorf("expected metric name 'cpu_usage', got %q", rule.MetricName)
	}
}

// ---------------------------------------------------------------------------
// Validation Tests
// ---------------------------------------------------------------------------

func TestValidateRule(t *testing.T) {
	tests := []struct {
		name    string
		rule    *alerting.AlertRule
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid threshold rule with expression",
			rule: &alerting.AlertRule{
				ID:                 "r1",
				Name:               "Test",
				Type:               alerting.RuleTypeThreshold,
				Operator:           alerting.OpGreaterThan,
				Expression:         "cpu_usage",
				EvaluationInterval: time.Minute,
			},
			wantErr: false,
		},
		{
			name: "valid threshold rule with metric name",
			rule: &alerting.AlertRule{
				ID:                 "r2",
				Name:               "Test",
				Type:               alerting.RuleTypeThreshold,
				Operator:           alerting.OpLessThan,
				MetricName:         "disk_free",
				EvaluationInterval: time.Minute,
			},
			wantErr: false,
		},
		{
			name: "valid rate rule",
			rule: &alerting.AlertRule{
				ID:                 "r3",
				Name:               "Rate",
				Type:               alerting.RuleTypeRate,
				RateWindow:         5 * time.Minute,
				EvaluationInterval: time.Minute,
			},
			wantErr: false,
		},
		{
			name: "valid absence rule",
			rule: &alerting.AlertRule{
				ID:                 "r4",
				Name:               "Absence",
				Type:               alerting.RuleTypeAbsence,
				AbsenceWindow:      10 * time.Minute,
				EvaluationInterval: time.Minute,
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			rule: &alerting.AlertRule{
				Name:               "Test",
				Type:               alerting.RuleTypeThreshold,
				Operator:           alerting.OpGreaterThan,
				Expression:         "cpu",
				EvaluationInterval: time.Minute,
			},
			wantErr: true,
			errMsg:  "rule ID is required",
		},
		{
			name: "missing name",
			rule: &alerting.AlertRule{
				ID:                 "r1",
				Type:               alerting.RuleTypeThreshold,
				Operator:           alerting.OpGreaterThan,
				Expression:         "cpu",
				EvaluationInterval: time.Minute,
			},
			wantErr: true,
			errMsg:  "rule name is required",
		},
		{
			name: "missing type",
			rule: &alerting.AlertRule{
				ID:                 "r1",
				Name:               "Test",
				Expression:         "cpu",
				EvaluationInterval: time.Minute,
			},
			wantErr: true,
			errMsg:  "rule type is required",
		},
		{
			name: "threshold rule missing operator",
			rule: &alerting.AlertRule{
				ID:                 "r1",
				Name:               "Test",
				Type:               alerting.RuleTypeThreshold,
				Expression:         "cpu",
				EvaluationInterval: time.Minute,
			},
			wantErr: true,
			errMsg:  "threshold rule requires operator",
		},
		{
			name: "threshold rule missing expression and metric name",
			rule: &alerting.AlertRule{
				ID:                 "r1",
				Name:               "Test",
				Type:               alerting.RuleTypeThreshold,
				Operator:           alerting.OpGreaterThan,
				EvaluationInterval: time.Minute,
			},
			wantErr: true,
			errMsg:  "threshold rule requires expression or metric name",
		},
		{
			name: "rate rule with zero window",
			rule: &alerting.AlertRule{
				ID:                 "r1",
				Name:               "Test",
				Type:               alerting.RuleTypeRate,
				EvaluationInterval: time.Minute,
			},
			wantErr: true,
			errMsg:  "rate rule requires positive rate window",
		},
		{
			name: "absence rule with zero window",
			rule: &alerting.AlertRule{
				ID:                 "r1",
				Name:               "Test",
				Type:               alerting.RuleTypeAbsence,
				EvaluationInterval: time.Minute,
			},
			wantErr: true,
			errMsg:  "absence rule requires positive absence window",
		},
		{
			name: "unknown rule type",
			rule: &alerting.AlertRule{
				ID:                 "r1",
				Name:               "Test",
				Type:               alerting.RuleType("custom"),
				EvaluationInterval: time.Minute,
			},
			wantErr: true,
			errMsg:  "unknown rule type: custom",
		},
		{
			name: "zero evaluation interval",
			rule: &alerting.AlertRule{
				ID:       "r1",
				Name:     "Test",
				Type:     alerting.RuleTypeRate,
				RateWindow: 5 * time.Minute,
			},
			wantErr: true,
			errMsg:  "evaluation interval must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRule(tt.rule)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !errors.Is(err, alerting.ErrInvalidRule) {
					t.Errorf("expected ErrInvalidRule, got %v", err)
				}
				if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
			}
		})
	}
}

func TestBuildValidationErrors(t *testing.T) {
	// Builder should propagate validation errors from Build()
	_, err := NewRuleBuilder("", "Test").Build()
	if err == nil {
		t.Fatal("expected error for empty ID")
	}

	_, err = NewRuleBuilder("r1", "").Build()
	if err == nil {
		t.Fatal("expected error for empty name")
	}

	// Missing type
	_, err = NewRuleBuilder("r1", "Test").
		WithExpression("cpu").
		Build()
	if err == nil {
		t.Fatal("expected error for missing type")
	}
}

// ---------------------------------------------------------------------------
// 2. Rule Expression Parsing
// ---------------------------------------------------------------------------

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input   string
		want    time.Duration
		wantErr bool
	}{
		// Standard Go durations
		{"1s", time.Second, false},
		{"30s", 30 * time.Second, false},
		{"5m0s", 5 * time.Minute, false},
		{"1h30m0s", 90 * time.Minute, false},

		// Prometheus-style durations
		{"5m", 5 * time.Minute, false},
		{"10s", 10 * time.Second, false},
		{"2h", 2 * time.Hour, false},
		{"1d", 24 * time.Hour, false},
		{"1w", 7 * 24 * time.Hour, false},
		{"1y", 365 * 24 * time.Hour, false},

		// Edge cases
		{"", 0, false},
		{"0s", 0, false},

		// Invalid
		{"abc", 0, true},
		{"5x", 0, true},

		// Negative durations are valid Go durations
		{"-1m", -1 * time.Minute, false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseDuration(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("ParseDuration(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseRuleType(t *testing.T) {
	tests := []struct {
		input   string
		want    alerting.RuleType
		wantErr bool
	}{
		{"threshold", alerting.RuleTypeThreshold, false},
		{"Threshold", alerting.RuleTypeThreshold, false},
		{"THRESHOLD", alerting.RuleTypeThreshold, false},
		{"", alerting.RuleTypeThreshold, false}, // default
		{"rate", alerting.RuleTypeRate, false},
		{"Rate", alerting.RuleTypeRate, false},
		{"absence", alerting.RuleTypeAbsence, false},
		{"Absence", alerting.RuleTypeAbsence, false},
		{"unknown", "", true},
		{"custom", "", true},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("input=%q", tt.input), func(t *testing.T) {
			got, err := ParseRuleType(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("ParseRuleType(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseSeverity(t *testing.T) {
	tests := []struct {
		input   string
		want    alerting.AlertSeverity
		wantErr bool
	}{
		{"critical", alerting.SeverityCritical, false},
		{"crit", alerting.SeverityCritical, false},
		{"Critical", alerting.SeverityCritical, false},
		{"warning", alerting.SeverityWarning, false},
		{"warn", alerting.SeverityWarning, false},
		{"Warning", alerting.SeverityWarning, false},
		{"info", alerting.SeverityInfo, false},
		{"informational", alerting.SeverityInfo, false},
		{"Info", alerting.SeverityInfo, false},
		{"unknown", "", true},
		{"", "", true},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("input=%q", tt.input), func(t *testing.T) {
			got, err := ParseSeverity(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("ParseSeverity(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseOperator(t *testing.T) {
	tests := []struct {
		input   string
		want    alerting.ComparisonOperator
		wantErr bool
	}{
		{">", alerting.OpGreaterThan, false},
		{"gt", alerting.OpGreaterThan, false},
		{">=", alerting.OpGreaterThanEqual, false},
		{"gte", alerting.OpGreaterThanEqual, false},
		{"ge", alerting.OpGreaterThanEqual, false},
		{"<", alerting.OpLessThan, false},
		{"lt", alerting.OpLessThan, false},
		{"<=", alerting.OpLessThanEqual, false},
		{"lte", alerting.OpLessThanEqual, false},
		{"le", alerting.OpLessThanEqual, false},
		{"==", alerting.OpEqual, false},
		{"eq", alerting.OpEqual, false},
		{"!=", alerting.OpNotEqual, false},
		{"ne", alerting.OpNotEqual, false},
		{"neq", alerting.OpNotEqual, false},
		{"unknown", "", true},
		{"", "", true},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("input=%q", tt.input), func(t *testing.T) {
			got, err := ParseOperator(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("ParseOperator(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGenerateRuleID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"High CPU Usage", "high_cpu_usage"},
		{"error-rate-spike", "error_rate_spike"},
		{"Simple", "simple"},
		{"  leading trailing  ", "leading_trailing"},
		{"Multi   Space", "multi_space"},
		{"Special!@#Characters", "special_characters"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := GenerateRuleID(tt.input)
			if got != tt.want {
				t.Errorf("GenerateRuleID(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// YAML and JSON Parsing
// ---------------------------------------------------------------------------

func TestParserParseYAML(t *testing.T) {
	yamlData := []byte(`
groups:
  - name: cpu-alerts
    interval: 30s
    labels:
      env: production
    rules:
      - id: cpu-high
        alert: High CPU
        expr: "avg(cpu_usage)"
        for: "5m"
        type: threshold
        severity: critical
        threshold: 90.0
        operator: ">"
        labels:
          team: infra
        annotations:
          summary: "CPU is high"
        channels:
          - slack
  - name: health-checks
    rules:
      - id: service-down
        alert: Service Down
        expr: "up"
        type: threshold
        severity: warning
        threshold: 0
        operator: "=="
`)

	parser := NewParser()
	ruleSets, err := parser.ParseYAML(yamlData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ruleSets) != 2 {
		t.Fatalf("expected 2 rule sets, got %d", len(ruleSets))
	}

	// Check first group
	rs := ruleSets[0]
	if rs.Name != "cpu-alerts" {
		t.Errorf("expected name 'cpu-alerts', got %q", rs.Name)
	}
	if len(rs.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rs.Rules))
	}

	rule := rs.Rules[0]
	if rule.ID != "cpu-high" {
		t.Errorf("expected ID 'cpu-high', got %q", rule.ID)
	}
	if rule.Name != "High CPU" {
		t.Errorf("expected name 'High CPU', got %q", rule.Name)
	}
	if rule.Type != alerting.RuleTypeThreshold {
		t.Errorf("expected type threshold, got %q", rule.Type)
	}
	if rule.Severity != alerting.SeverityCritical {
		t.Errorf("expected severity critical, got %q", rule.Severity)
	}
	if rule.Threshold != 90.0 {
		t.Errorf("expected threshold 90.0, got %f", rule.Threshold)
	}
	if rule.Operator != alerting.OpGreaterThan {
		t.Errorf("expected operator '>', got %q", rule.Operator)
	}
	if rule.ForPeriod != 5*time.Minute {
		t.Errorf("expected for period 5m, got %v", rule.ForPeriod)
	}
	if rule.EvaluationInterval != 30*time.Second {
		t.Errorf("expected 30s interval, got %v", rule.EvaluationInterval)
	}
	if rule.Annotations["summary"] != "CPU is high" {
		t.Errorf("expected annotation summary set, got %q", rule.Annotations["summary"])
	}
	// Rule-set-level labels should merge into rule labels
	if rule.Labels["env"] != "production" {
		t.Errorf("expected rule-set label env=production to propagate, got %q", rule.Labels["env"])
	}
	if rule.Labels["team"] != "infra" {
		t.Errorf("expected rule label team=infra, got %q", rule.Labels["team"])
	}
}

func TestParserParseJSON(t *testing.T) {
	jsonData := []byte(`{
		"groups": [
			{
				"name": "memory-alerts",
				"rules": [
					{
						"id": "mem-high",
						"alert": "High Memory",
						"expr": "mem_used_percent",
						"type": "threshold",
						"severity": "warning",
						"threshold": 85,
						"operator": ">="
					}
				]
			}
		]
	}`)

	parser := NewParser()
	ruleSets, err := parser.ParseJSON(jsonData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ruleSets) != 1 {
		t.Fatalf("expected 1 rule set, got %d", len(ruleSets))
	}

	rule := ruleSets[0].Rules[0]
	if rule.ID != "mem-high" {
		t.Errorf("expected ID 'mem-high', got %q", rule.ID)
	}
	if rule.Operator != alerting.OpGreaterThanEqual {
		t.Errorf("expected operator '>=', got %q", rule.Operator)
	}
}

func TestParserAutoGenerateID(t *testing.T) {
	yamlData := []byte(`
groups:
  - name: test
    rules:
      - alert: High CPU Usage
        expr: "cpu"
        type: threshold
        severity: warning
        threshold: 90
        operator: ">"
`)

	parser := NewParser()
	ruleSets, err := parser.ParseYAML(yamlData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rule := ruleSets[0].Rules[0]
	if rule.ID == "" {
		t.Fatal("expected auto-generated ID, got empty")
	}
	expected := GenerateRuleID("High CPU Usage")
	if rule.ID != expected {
		t.Errorf("expected auto-generated ID %q, got %q", expected, rule.ID)
	}
}

func TestParserDefaultSeverity(t *testing.T) {
	yamlData := []byte(`
groups:
  - name: test
    rules:
      - id: r1
        alert: Test Rule
        expr: "up"
        type: threshold
        threshold: 1
        operator: "=="
`)

	// Use custom default severity
	parser := NewParser().WithDefaultSeverity(alerting.SeverityInfo)
	ruleSets, err := parser.ParseYAML(yamlData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rule := ruleSets[0].Rules[0]
	if rule.Severity != alerting.SeverityInfo {
		t.Errorf("expected default severity info, got %q", rule.Severity)
	}
}

func TestParserDefaultInterval(t *testing.T) {
	yamlData := []byte(`
groups:
  - name: test
    rules:
      - id: r1
        alert: Test Rule
        expr: "up"
        type: threshold
        threshold: 1
        operator: "=="
`)

	parser := NewParser().WithDefaultInterval(15 * time.Second)
	ruleSets, err := parser.ParseYAML(yamlData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rule := ruleSets[0].Rules[0]
	if rule.EvaluationInterval != 15*time.Second {
		t.Errorf("expected 15s interval, got %v", rule.EvaluationInterval)
	}
}

func TestParserInvalidYAML(t *testing.T) {
	parser := NewParser()
	_, err := parser.ParseYAML([]byte("{{invalid yaml"))
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestParserInvalidJSON(t *testing.T) {
	parser := NewParser()
	_, err := parser.ParseJSON([]byte("{invalid json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParserInvalidForDuration(t *testing.T) {
	yamlData := []byte(`
groups:
  - name: test
    rules:
      - id: r1
        alert: Test
        expr: "up"
        type: threshold
        threshold: 1
        operator: ">"
        for: "notaduration"
`)

	parser := NewParser()
	_, err := parser.ParseYAML(yamlData)
	if err == nil {
		t.Fatal("expected error for invalid for duration")
	}
}

func TestParserInvalidOperator(t *testing.T) {
	yamlData := []byte(`
groups:
  - name: test
    rules:
      - id: r1
        alert: Test
        expr: "up"
        type: threshold
        threshold: 1
        operator: "~="
`)

	parser := NewParser()
	_, err := parser.ParseYAML(yamlData)
	if err == nil {
		t.Fatal("expected error for invalid operator")
	}
}

func TestParserInvalidGroupInterval(t *testing.T) {
	yamlData := []byte(`
groups:
  - name: test
    interval: "badinterval"
    rules:
      - id: r1
        alert: Test
        expr: "up"
        type: threshold
        threshold: 1
        operator: ">"
`)

	parser := NewParser()
	_, err := parser.ParseYAML(yamlData)
	if err == nil {
		t.Fatal("expected error for invalid group interval")
	}
}

func TestParserEmptyGroups(t *testing.T) {
	yamlData := []byte(`
groups: []
`)

	parser := NewParser()
	ruleSets, err := parser.ParseYAML(yamlData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ruleSets) != 0 {
		t.Errorf("expected 0 rule sets, got %d", len(ruleSets))
	}
}

// ---------------------------------------------------------------------------
// 3. Rule Evaluation Against Metric Values
// ---------------------------------------------------------------------------

func TestEvaluateThresholdRule(t *testing.T) {
	tests := []struct {
		name      string
		threshold float64
		operator  alerting.ComparisonOperator
		value     float64
		wantFire  bool
	}{
		{"gt firing", 90, alerting.OpGreaterThan, 95, true},
		{"gt not firing", 90, alerting.OpGreaterThan, 85, false},
		{"gt boundary", 90, alerting.OpGreaterThan, 90, false},
		{"gte firing", 90, alerting.OpGreaterThanEqual, 90, true},
		{"gte not firing", 90, alerting.OpGreaterThanEqual, 89, false},
		{"lt firing", 10, alerting.OpLessThan, 5, true},
		{"lt not firing", 10, alerting.OpLessThan, 15, false},
		{"lt boundary", 10, alerting.OpLessThan, 10, false},
		{"lte firing", 10, alerting.OpLessThanEqual, 10, true},
		{"lte not firing", 10, alerting.OpLessThanEqual, 11, false},
		{"eq firing", 42, alerting.OpEqual, 42, true},
		{"eq not firing", 42, alerting.OpEqual, 43, false},
		{"ne firing", 0, alerting.OpNotEqual, 1, true},
		{"ne not firing", 0, alerting.OpNotEqual, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &mockMetricProvider{
				queryFunc: func(_ context.Context, _ string, _ time.Time) (float64, error) {
					return tt.value, nil
				},
			}

			eval := NewEvaluator(provider)
			rule := &alerting.AlertRule{
				ID:                 "test-rule",
				Name:               "Test",
				Type:               alerting.RuleTypeThreshold,
				Expression:         "test_metric",
				Threshold:          tt.threshold,
				Operator:           tt.operator,
				EvaluationInterval: time.Minute,
				Labels:             map[string]string{},
			}

			result, err := eval.Evaluate(context.Background(), rule)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.Firing != tt.wantFire {
				t.Errorf("expected firing=%v, got %v (value=%.2f, threshold=%.2f, op=%s)",
					tt.wantFire, result.Firing, tt.value, tt.threshold, tt.operator)
			}
			if result.Value != tt.value {
				t.Errorf("expected value %.2f, got %.2f", tt.value, result.Value)
			}
			if result.RuleID != "test-rule" {
				t.Errorf("expected rule ID 'test-rule', got %q", result.RuleID)
			}
		})
	}
}

func TestEvaluateRateRule(t *testing.T) {
	tests := []struct {
		name         string
		values       []float64
		rateIncrease float64
		rateWindow   time.Duration
		wantFire     bool
	}{
		{
			name:         "rate exceeds threshold",
			values:       []float64{100, 110, 120, 130, 200},
			rateIncrease: 0.1,
			rateWindow:   5 * time.Minute,
			wantFire:     true,
		},
		{
			name:         "rate below threshold",
			values:       []float64{100, 100, 100, 100, 101},
			rateIncrease: 10.0,
			rateWindow:   5 * time.Minute,
			wantFire:     false,
		},
		{
			name:         "insufficient data points",
			values:       []float64{100},
			rateIncrease: 0.1,
			rateWindow:   5 * time.Minute,
			wantFire:     false,
		},
		{
			name:         "no data",
			values:       []float64{},
			rateIncrease: 0.1,
			rateWindow:   5 * time.Minute,
			wantFire:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &mockMetricProvider{
				rangeFunc: func(_ context.Context, _ string, _, _ time.Time, _ time.Duration) ([]float64, error) {
					return tt.values, nil
				},
			}

			eval := NewEvaluator(provider)
			rule := &alerting.AlertRule{
				ID:                 "rate-rule",
				Name:               "Rate Test",
				Type:               alerting.RuleTypeRate,
				Expression:         "http_requests_total",
				RateWindow:         tt.rateWindow,
				RateIncrease:       tt.rateIncrease,
				EvaluationInterval: time.Minute,
				Labels:             map[string]string{},
			}

			result, err := eval.Evaluate(context.Background(), rule)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.Firing != tt.wantFire {
				t.Errorf("expected firing=%v, got %v (rate value=%.4f, threshold=%.4f)",
					tt.wantFire, result.Firing, result.Value, tt.rateIncrease)
			}
		})
	}
}

func TestEvaluateAbsenceRule(t *testing.T) {
	tests := []struct {
		name     string
		values   []float64
		wantFire bool
	}{
		{
			name:     "data absent (no values)",
			values:   []float64{},
			wantFire: true,
		},
		{
			name:     "data absent (all zeros)",
			values:   []float64{0, 0, 0, 0},
			wantFire: true,
		},
		{
			name:     "data present",
			values:   []float64{1, 2, 3},
			wantFire: false,
		},
		{
			name:     "data present (one non-zero)",
			values:   []float64{0, 0, 1, 0},
			wantFire: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &mockMetricProvider{
				rangeFunc: func(_ context.Context, _ string, _, _ time.Time, _ time.Duration) ([]float64, error) {
					return tt.values, nil
				},
			}

			eval := NewEvaluator(provider)
			rule := &alerting.AlertRule{
				ID:                 "absence-rule",
				Name:               "Absence Test",
				Type:               alerting.RuleTypeAbsence,
				Expression:         "heartbeat",
				AbsenceWindow:      10 * time.Minute,
				EvaluationInterval: time.Minute,
				Labels:             map[string]string{},
			}

			result, err := eval.Evaluate(context.Background(), rule)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.Firing != tt.wantFire {
				t.Errorf("expected firing=%v, got %v", tt.wantFire, result.Firing)
			}
		})
	}
}

func TestEvaluateUnsupportedRuleType(t *testing.T) {
	provider := &mockMetricProvider{}
	eval := NewEvaluator(provider)
	rule := &alerting.AlertRule{
		ID:     "bad-type",
		Name:   "Bad",
		Type:   alerting.RuleType("custom"),
		Labels: map[string]string{},
	}

	_, err := eval.Evaluate(context.Background(), rule)
	if err == nil {
		t.Fatal("expected error for unsupported rule type")
	}
	if !errors.Is(err, alerting.ErrEvaluationFailed) {
		t.Errorf("expected ErrEvaluationFailed, got %v", err)
	}
}

func TestEvaluateLabelsInResult(t *testing.T) {
	provider := &mockMetricProvider{
		queryFunc: func(_ context.Context, _ string, _ time.Time) (float64, error) {
			return 95, nil
		},
	}

	eval := NewEvaluator(provider)
	rule := &alerting.AlertRule{
		ID:                 "r1",
		Name:               "Test",
		Type:               alerting.RuleTypeThreshold,
		Expression:         "cpu",
		Threshold:          90,
		Operator:           alerting.OpGreaterThan,
		EvaluationInterval: time.Minute,
		Labels:             map[string]string{"team": "infra", "env": "prod"},
	}

	result, err := eval.Evaluate(context.Background(), rule)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Labels["team"] != "infra" {
		t.Errorf("expected label team=infra, got %q", result.Labels["team"])
	}
	if result.Labels["env"] != "prod" {
		t.Errorf("expected label env=prod, got %q", result.Labels["env"])
	}
}

// ---------------------------------------------------------------------------
// 4. Evaluation with Time Windows (ForPeriod)
// ---------------------------------------------------------------------------

func TestCheckForPeriod_ImmediateNotFiring(t *testing.T) {
	// With a ForPeriod, the first evaluation should NOT fire even if the
	// condition is met -- it should only start the pending timer.
	provider := &mockMetricProvider{
		queryFunc: func(_ context.Context, _ string, _ time.Time) (float64, error) {
			return 95, nil
		},
	}

	eval := NewEvaluator(provider)
	rule := &alerting.AlertRule{
		ID:                 "for-rule",
		Name:               "For Test",
		Type:               alerting.RuleTypeThreshold,
		Expression:         "cpu",
		Threshold:          90,
		Operator:           alerting.OpGreaterThan,
		ForPeriod:          5 * time.Minute,
		EvaluationInterval: time.Minute,
		Labels:             map[string]string{},
	}

	result, err := eval.Evaluate(context.Background(), rule)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Firing {
		t.Error("expected not firing on first evaluation with ForPeriod")
	}

	// The state should now be tracking pending
	state, exists := eval.GetState("for-rule")
	if !exists {
		t.Fatal("expected evaluation state to exist")
	}
	if state.pendingSince.IsZero() {
		t.Error("expected pendingSince to be set")
	}
	if state.consecutiveHits != 1 {
		t.Errorf("expected 1 consecutive hit, got %d", state.consecutiveHits)
	}
}

func TestCheckForPeriod_ResetsOnRecovery(t *testing.T) {
	callCount := 0
	provider := &mockMetricProvider{
		queryFunc: func(_ context.Context, _ string, _ time.Time) (float64, error) {
			callCount++
			if callCount <= 2 {
				return 95, nil // firing
			}
			return 80, nil // recovered
		},
	}

	eval := NewEvaluator(provider)
	rule := &alerting.AlertRule{
		ID:                 "reset-rule",
		Name:               "Reset Test",
		Type:               alerting.RuleTypeThreshold,
		Expression:         "cpu",
		Threshold:          90,
		Operator:           alerting.OpGreaterThan,
		ForPeriod:          5 * time.Minute,
		EvaluationInterval: time.Minute,
		Labels:             map[string]string{},
	}

	ctx := context.Background()

	// First eval: condition met, start pending
	eval.Evaluate(ctx, rule)
	state, _ := eval.GetState("reset-rule")
	if state.pendingSince.IsZero() {
		t.Error("expected pending state after first eval")
	}

	// Second eval: condition still met
	eval.Evaluate(ctx, rule)
	state, _ = eval.GetState("reset-rule")
	if state.consecutiveHits != 2 {
		t.Errorf("expected 2 consecutive hits, got %d", state.consecutiveHits)
	}

	// Third eval: condition NOT met, should reset
	eval.Evaluate(ctx, rule)
	state, _ = eval.GetState("reset-rule")
	if !state.pendingSince.IsZero() {
		t.Error("expected pendingSince to be reset after recovery")
	}
	if state.consecutiveHits != 0 {
		t.Errorf("expected 0 consecutive hits after reset, got %d", state.consecutiveHits)
	}
}

func TestCheckForPeriod_NoForPeriodFiresImmediately(t *testing.T) {
	provider := &mockMetricProvider{
		queryFunc: func(_ context.Context, _ string, _ time.Time) (float64, error) {
			return 95, nil
		},
	}

	eval := NewEvaluator(provider)
	rule := &alerting.AlertRule{
		ID:                 "no-for",
		Name:               "No For",
		Type:               alerting.RuleTypeThreshold,
		Expression:         "cpu",
		Threshold:          90,
		Operator:           alerting.OpGreaterThan,
		EvaluationInterval: time.Minute,
		Labels:             map[string]string{},
	}

	result, err := eval.Evaluate(context.Background(), rule)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Firing {
		t.Error("expected immediate firing without ForPeriod")
	}
}

func TestResetState(t *testing.T) {
	provider := &mockMetricProvider{
		queryFunc: func(_ context.Context, _ string, _ time.Time) (float64, error) {
			return 95, nil
		},
	}

	eval := NewEvaluator(provider)
	rule := &alerting.AlertRule{
		ID:                 "state-rule",
		Name:               "State",
		Type:               alerting.RuleTypeThreshold,
		Expression:         "cpu",
		Threshold:          90,
		Operator:           alerting.OpGreaterThan,
		ForPeriod:          5 * time.Minute,
		EvaluationInterval: time.Minute,
		Labels:             map[string]string{},
	}

	eval.Evaluate(context.Background(), rule)

	_, exists := eval.GetState("state-rule")
	if !exists {
		t.Fatal("expected state to exist")
	}

	eval.ResetState("state-rule")
	_, exists = eval.GetState("state-rule")
	if exists {
		t.Error("expected state to be cleared after ResetState")
	}
}

// ---------------------------------------------------------------------------
// 5. Alert State Transitions (inactive -> pending -> firing)
// ---------------------------------------------------------------------------

func TestAlertStateTransitions(t *testing.T) {
	// We simulate the inactive -> pending -> firing transition by
	// pre-seeding the evaluator's cache with an old pendingSince time.
	provider := &mockMetricProvider{
		queryFunc: func(_ context.Context, _ string, _ time.Time) (float64, error) {
			return 95, nil
		},
	}

	eval := NewEvaluator(provider)
	rule := &alerting.AlertRule{
		ID:                 "transition-rule",
		Name:               "Transition",
		Type:               alerting.RuleTypeThreshold,
		Expression:         "cpu",
		Threshold:          90,
		Operator:           alerting.OpGreaterThan,
		ForPeriod:          5 * time.Minute,
		EvaluationInterval: time.Minute,
		Labels:             map[string]string{},
	}

	ctx := context.Background()

	// Step 1: First evaluation -- goes from inactive to pending
	result, _ := eval.Evaluate(ctx, rule)
	if result.Firing {
		t.Error("step 1: should not be firing yet (pending)")
	}

	// Step 2: Manually backdate pendingSince to simulate time passing
	eval.mu.Lock()
	state := eval.cache["transition-rule"]
	state.pendingSince = time.Now().Add(-6 * time.Minute) // 6min ago, exceeds 5min ForPeriod
	eval.mu.Unlock()

	// Step 3: Next evaluation should now fire
	result, _ = eval.Evaluate(ctx, rule)
	if !result.Firing {
		t.Error("step 3: should be firing after ForPeriod elapsed")
	}

	// Verify state shows firing
	eval.mu.RLock()
	state = eval.cache["transition-rule"]
	if !state.firingState {
		t.Error("expected firingState to be true")
	}
	eval.mu.RUnlock()
}

func TestAlertCreateFromRule(t *testing.T) {
	rule := &alerting.AlertRule{
		ID:          "cpu-high",
		Name:        "High CPU",
		Description: "CPU too high",
		Severity:    alerting.SeverityCritical,
		Threshold:   90,
		Labels:      map[string]string{"team": "infra"},
		Annotations: map[string]string{"runbook": "https://wiki"},
	}

	alert := CreateAlertFromRule(rule, 95.5, map[string]string{"host": "server-1"})

	if alert.RuleID != "cpu-high" {
		t.Errorf("expected rule ID 'cpu-high', got %q", alert.RuleID)
	}
	if alert.Name != "High CPU" {
		t.Errorf("expected name 'High CPU', got %q", alert.Name)
	}
	if alert.State != alerting.AlertStatePending {
		t.Errorf("expected state pending, got %q", alert.State)
	}
	if alert.Severity != alerting.SeverityCritical {
		t.Errorf("expected severity critical, got %q", alert.Severity)
	}
	if alert.Value != 95.5 {
		t.Errorf("expected value 95.5, got %f", alert.Value)
	}
	if alert.Threshold != 90 {
		t.Errorf("expected threshold 90, got %f", alert.Threshold)
	}
	// Merged labels
	if alert.Labels["team"] != "infra" {
		t.Errorf("expected label team=infra, got %q", alert.Labels["team"])
	}
	if alert.Labels["host"] != "server-1" {
		t.Errorf("expected label host=server-1, got %q", alert.Labels["host"])
	}
	if alert.Annotations["runbook"] != "https://wiki" {
		t.Errorf("expected annotation runbook set")
	}
	if alert.Fingerprint == "" {
		t.Error("expected fingerprint to be generated")
	}
	if alert.StartsAt.IsZero() {
		t.Error("expected StartsAt to be set")
	}
	if alert.ID == "" {
		t.Error("expected alert ID to be generated")
	}
}

func TestCreateAlertFromRule_LabelOverride(t *testing.T) {
	// Evaluation labels should override rule labels
	rule := &alerting.AlertRule{
		ID:          "r1",
		Name:        "Test",
		Labels:      map[string]string{"env": "staging", "team": "infra"},
		Annotations: map[string]string{},
	}

	alert := CreateAlertFromRule(rule, 100, map[string]string{"env": "production"})

	if alert.Labels["env"] != "production" {
		t.Errorf("expected evaluation labels to override rule labels, got env=%q", alert.Labels["env"])
	}
	if alert.Labels["team"] != "infra" {
		t.Errorf("expected non-conflicting rule labels preserved, got team=%q", alert.Labels["team"])
	}
}

// ---------------------------------------------------------------------------
// 6. Multiple Rules with Priority Ordering (RuleSet)
// ---------------------------------------------------------------------------

func TestRuleSet_AddGetRemove(t *testing.T) {
	rs := NewRuleSet("test-set")
	rs.Description = "Test rule set"

	rule1 := &alerting.AlertRule{
		ID:                 "r1",
		Name:               "Rule 1",
		Type:               alerting.RuleTypeThreshold,
		Operator:           alerting.OpGreaterThan,
		Expression:         "cpu",
		EvaluationInterval: time.Minute,
		Labels:             map[string]string{},
	}
	rule2 := &alerting.AlertRule{
		ID:                 "r2",
		Name:               "Rule 2",
		Type:               alerting.RuleTypeRate,
		RateWindow:         5 * time.Minute,
		EvaluationInterval: time.Minute,
		Labels:             map[string]string{},
	}

	if err := rs.AddRule(rule1); err != nil {
		t.Fatalf("unexpected error adding rule1: %v", err)
	}
	if err := rs.AddRule(rule2); err != nil {
		t.Fatalf("unexpected error adding rule2: %v", err)
	}

	if len(rs.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rs.Rules))
	}

	// Get
	found, ok := rs.GetRule("r1")
	if !ok {
		t.Fatal("expected to find rule r1")
	}
	if found.Name != "Rule 1" {
		t.Errorf("expected 'Rule 1', got %q", found.Name)
	}

	_, ok = rs.GetRule("nonexistent")
	if ok {
		t.Error("expected not found for nonexistent rule")
	}

	// Remove
	removed := rs.RemoveRule("r1")
	if !removed {
		t.Error("expected successful removal")
	}
	if len(rs.Rules) != 1 {
		t.Errorf("expected 1 rule after removal, got %d", len(rs.Rules))
	}

	removed = rs.RemoveRule("nonexistent")
	if removed {
		t.Error("expected false for removing nonexistent rule")
	}
}

func TestRuleSet_RejectsInvalidRule(t *testing.T) {
	rs := NewRuleSet("test")
	badRule := &alerting.AlertRule{
		Name: "No ID",
		Type: alerting.RuleTypeThreshold,
	}

	err := rs.AddRule(badRule)
	if err == nil {
		t.Fatal("expected error for invalid rule")
	}
}

func TestRuleSet_InheritsLabels(t *testing.T) {
	rs := NewRuleSet("prod-alerts")
	rs.Labels["env"] = "production"
	rs.Labels["team"] = "platform"

	rule := &alerting.AlertRule{
		ID:                 "r1",
		Name:               "Test",
		Type:               alerting.RuleTypeThreshold,
		Operator:           alerting.OpGreaterThan,
		Expression:         "cpu",
		EvaluationInterval: time.Minute,
		Labels:             map[string]string{"team": "custom"},
	}

	if err := rs.AddRule(rule); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// RuleSet label 'env' should be inherited
	if rule.Labels["env"] != "production" {
		t.Errorf("expected inherited label env=production, got %q", rule.Labels["env"])
	}
	// Rule's own label 'team' should not be overridden
	if rule.Labels["team"] != "custom" {
		t.Errorf("expected rule label team=custom preserved, got %q", rule.Labels["team"])
	}
}

func TestEvaluateAll_SkipsDisabledRules(t *testing.T) {
	provider := &mockMetricProvider{
		queryFunc: func(_ context.Context, _ string, _ time.Time) (float64, error) {
			return 95, nil
		},
	}

	eval := NewEvaluator(provider)

	rules := []*alerting.AlertRule{
		{
			ID:                 "r1",
			Name:               "Enabled",
			Type:               alerting.RuleTypeThreshold,
			Expression:         "cpu",
			Threshold:          90,
			Operator:           alerting.OpGreaterThan,
			Enabled:            true,
			EvaluationInterval: time.Minute,
			Labels:             map[string]string{},
		},
		{
			ID:                 "r2",
			Name:               "Disabled",
			Type:               alerting.RuleTypeThreshold,
			Expression:         "mem",
			Threshold:          80,
			Operator:           alerting.OpGreaterThan,
			Enabled:            false,
			EvaluationInterval: time.Minute,
			Labels:             map[string]string{},
		},
		{
			ID:                 "r3",
			Name:               "Also Enabled",
			Type:               alerting.RuleTypeThreshold,
			Expression:         "disk",
			Threshold:          85,
			Operator:           alerting.OpGreaterThan,
			Enabled:            true,
			EvaluationInterval: time.Minute,
			Labels:             map[string]string{},
		},
	}

	results := eval.EvaluateAll(context.Background(), rules)

	if len(results) != 2 {
		t.Fatalf("expected 2 results (disabled skipped), got %d", len(results))
	}

	resultIDs := make(map[string]bool)
	for _, r := range results {
		resultIDs[r.RuleID] = true
	}
	if resultIDs["r2"] {
		t.Error("disabled rule r2 should not have been evaluated")
	}
}

func TestEvaluateAll_EmptyRules(t *testing.T) {
	provider := &mockMetricProvider{}
	eval := NewEvaluator(provider)

	results := eval.EvaluateAll(context.Background(), nil)
	if len(results) != 0 {
		t.Errorf("expected 0 results for nil rules, got %d", len(results))
	}

	results = eval.EvaluateAll(context.Background(), []*alerting.AlertRule{})
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty rules, got %d", len(results))
	}
}

// ---------------------------------------------------------------------------
// 7. Label Matching and Templating
// ---------------------------------------------------------------------------

func TestGenerateAlertFingerprint(t *testing.T) {
	// Same rule + labels = same fingerprint
	fp1 := GenerateAlertFingerprint("rule-1", map[string]string{"host": "a", "env": "prod"})
	fp2 := GenerateAlertFingerprint("rule-1", map[string]string{"env": "prod", "host": "a"})
	if fp1 != fp2 {
		t.Error("expected same fingerprint for same labels in different order")
	}

	// Different labels = different fingerprint
	fp3 := GenerateAlertFingerprint("rule-1", map[string]string{"host": "b", "env": "prod"})
	if fp1 == fp3 {
		t.Error("expected different fingerprint for different labels")
	}

	// Different rule ID = different fingerprint
	fp4 := GenerateAlertFingerprint("rule-2", map[string]string{"host": "a", "env": "prod"})
	if fp1 == fp4 {
		t.Error("expected different fingerprint for different rule IDs")
	}

	// Empty labels
	fp5 := GenerateAlertFingerprint("rule-1", map[string]string{})
	fp6 := GenerateAlertFingerprint("rule-1", nil)
	if fp5 != fp6 {
		t.Error("expected same fingerprint for empty and nil labels")
	}
}

func TestGenerateAlertFingerprint_Deterministic(t *testing.T) {
	labels := map[string]string{"host": "server-1", "env": "production", "team": "sre"}

	fp1 := GenerateAlertFingerprint("rule-x", labels)
	fp2 := GenerateAlertFingerprint("rule-x", labels)
	if fp1 != fp2 {
		t.Error("expected deterministic fingerprint")
	}

	if len(fp1) != 16 { // hex encoded 8 bytes = 16 chars
		t.Errorf("expected 16 char fingerprint, got %d chars: %q", len(fp1), fp1)
	}
}

func TestAlertFromRule_MergesLabels(t *testing.T) {
	rule := &alerting.AlertRule{
		ID:          "r1",
		Name:        "Test",
		Labels:      map[string]string{"severity": "critical", "team": "sre"},
		Annotations: map[string]string{},
	}

	evalLabels := map[string]string{"instance": "server-1", "job": "prometheus"}
	alert := CreateAlertFromRule(rule, 42, evalLabels)

	// All labels should be present
	expected := map[string]string{
		"severity": "critical",
		"team":     "sre",
		"instance": "server-1",
		"job":      "prometheus",
	}

	for k, v := range expected {
		if alert.Labels[k] != v {
			t.Errorf("expected label %s=%s, got %q", k, v, alert.Labels[k])
		}
	}
}

// ---------------------------------------------------------------------------
// 8. Error Handling
// ---------------------------------------------------------------------------

func TestEvaluateThreshold_QueryError(t *testing.T) {
	queryErr := fmt.Errorf("connection refused")
	provider := &mockMetricProvider{
		queryFunc: func(_ context.Context, _ string, _ time.Time) (float64, error) {
			return 0, queryErr
		},
	}

	eval := NewEvaluator(provider)
	rule := &alerting.AlertRule{
		ID:                 "err-rule",
		Name:               "Error",
		Type:               alerting.RuleTypeThreshold,
		Expression:         "cpu",
		Threshold:          90,
		Operator:           alerting.OpGreaterThan,
		EvaluationInterval: time.Minute,
		Labels:             map[string]string{},
	}

	result, err := eval.Evaluate(context.Background(), rule)
	if err == nil {
		t.Fatal("expected error from query failure")
	}
	if !errors.Is(err, alerting.ErrEvaluationFailed) {
		t.Errorf("expected ErrEvaluationFailed, got %v", err)
	}
	if result.Error == nil {
		t.Error("expected result.Error to be set")
	}
}

func TestEvaluateRate_QueryRangeError(t *testing.T) {
	provider := &mockMetricProvider{
		rangeFunc: func(_ context.Context, _ string, _, _ time.Time, _ time.Duration) ([]float64, error) {
			return nil, fmt.Errorf("timeout")
		},
	}

	eval := NewEvaluator(provider)
	rule := &alerting.AlertRule{
		ID:                 "rate-err",
		Name:               "Rate Error",
		Type:               alerting.RuleTypeRate,
		Expression:         "requests",
		RateWindow:         5 * time.Minute,
		EvaluationInterval: time.Minute,
		Labels:             map[string]string{},
	}

	_, err := eval.Evaluate(context.Background(), rule)
	if err == nil {
		t.Fatal("expected error from range query failure")
	}
	if !errors.Is(err, alerting.ErrEvaluationFailed) {
		t.Errorf("expected ErrEvaluationFailed, got %v", err)
	}
}

func TestEvaluateAbsence_QueryRangeError(t *testing.T) {
	provider := &mockMetricProvider{
		rangeFunc: func(_ context.Context, _ string, _, _ time.Time, _ time.Duration) ([]float64, error) {
			return nil, fmt.Errorf("backend error")
		},
	}

	eval := NewEvaluator(provider)
	rule := &alerting.AlertRule{
		ID:                 "absence-err",
		Name:               "Absence Error",
		Type:               alerting.RuleTypeAbsence,
		Expression:         "heartbeat",
		AbsenceWindow:      10 * time.Minute,
		EvaluationInterval: time.Minute,
		Labels:             map[string]string{},
	}

	_, err := eval.Evaluate(context.Background(), rule)
	if err == nil {
		t.Fatal("expected error from range query failure")
	}
	if !errors.Is(err, alerting.ErrEvaluationFailed) {
		t.Errorf("expected ErrEvaluationFailed, got %v", err)
	}
}

func TestCompareThreshold_UnknownOperator(t *testing.T) {
	provider := &mockMetricProvider{
		queryFunc: func(_ context.Context, _ string, _ time.Time) (float64, error) {
			return 95, nil
		},
	}

	eval := NewEvaluator(provider)
	rule := &alerting.AlertRule{
		ID:                 "bad-op",
		Name:               "Bad Op",
		Type:               alerting.RuleTypeThreshold,
		Expression:         "cpu",
		Threshold:          90,
		Operator:           alerting.ComparisonOperator("~="),
		EvaluationInterval: time.Minute,
		Labels:             map[string]string{},
	}

	result, err := eval.Evaluate(context.Background(), rule)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Unknown operator should result in not firing
	if result.Firing {
		t.Error("expected not firing for unknown operator")
	}
}

func TestCheckFiringCondition_UnknownRuleType(t *testing.T) {
	provider := &mockMetricProvider{}
	eval := NewEvaluator(provider)
	rule := &alerting.AlertRule{
		Type: alerting.RuleType("mystery"),
	}

	firing := eval.checkFiringCondition(rule, 100)
	if firing {
		t.Error("expected not firing for unknown rule type")
	}
}

// ---------------------------------------------------------------------------
// 9. Concurrent Rule Evaluation (BatchEvaluator)
// ---------------------------------------------------------------------------

func TestBatchEvaluator_ConcurrentEvaluation(t *testing.T) {
	var mu sync.Mutex
	evaluated := make(map[string]bool)

	provider := &mockMetricProvider{
		queryFunc: func(_ context.Context, expr string, _ time.Time) (float64, error) {
			mu.Lock()
			evaluated[expr] = true
			mu.Unlock()
			return 95, nil
		},
	}

	eval := NewEvaluator(provider)
	batchEval := NewBatchEvaluator(eval, 4)

	rules := make([]*alerting.AlertRule, 20)
	for i := 0; i < 20; i++ {
		rules[i] = &alerting.AlertRule{
			ID:                 fmt.Sprintf("rule-%d", i),
			Name:               fmt.Sprintf("Rule %d", i),
			Type:               alerting.RuleTypeThreshold,
			Expression:         fmt.Sprintf("metric_%d", i),
			Threshold:          90,
			Operator:           alerting.OpGreaterThan,
			Enabled:            true,
			EvaluationInterval: time.Minute,
			Labels:             map[string]string{},
		}
	}

	results := batchEval.EvaluateBatch(context.Background(), rules)

	if len(results) != 20 {
		t.Fatalf("expected 20 results, got %d", len(results))
	}

	// Verify all rules were evaluated
	mu.Lock()
	if len(evaluated) != 20 {
		t.Errorf("expected 20 unique evaluations, got %d", len(evaluated))
	}
	mu.Unlock()

	// All should be firing
	for _, r := range results {
		if !r.Firing {
			t.Errorf("expected rule %s to be firing", r.RuleID)
		}
	}
}

func TestBatchEvaluator_SkipsDisabled(t *testing.T) {
	provider := &mockMetricProvider{
		queryFunc: func(_ context.Context, _ string, _ time.Time) (float64, error) {
			return 95, nil
		},
	}

	eval := NewEvaluator(provider)
	batchEval := NewBatchEvaluator(eval, 2)

	rules := []*alerting.AlertRule{
		{
			ID: "enabled", Name: "E", Type: alerting.RuleTypeThreshold,
			Expression: "cpu", Threshold: 90, Operator: alerting.OpGreaterThan,
			Enabled: true, EvaluationInterval: time.Minute, Labels: map[string]string{},
		},
		{
			ID: "disabled", Name: "D", Type: alerting.RuleTypeThreshold,
			Expression: "mem", Threshold: 80, Operator: alerting.OpGreaterThan,
			Enabled: false, EvaluationInterval: time.Minute, Labels: map[string]string{},
		},
	}

	results := batchEval.EvaluateBatch(context.Background(), rules)

	if len(results) != 1 {
		t.Fatalf("expected 1 result (disabled skipped), got %d", len(results))
	}
	if results[0].RuleID != "enabled" {
		t.Errorf("expected enabled rule result, got %q", results[0].RuleID)
	}
}

func TestBatchEvaluator_EmptyRules(t *testing.T) {
	provider := &mockMetricProvider{}
	eval := NewEvaluator(provider)
	batchEval := NewBatchEvaluator(eval, 4)

	results := batchEval.EvaluateBatch(context.Background(), nil)
	if results != nil {
		t.Errorf("expected nil for empty rules, got %v", results)
	}

	results = batchEval.EvaluateBatch(context.Background(), []*alerting.AlertRule{})
	if results != nil {
		t.Errorf("expected nil for empty slice, got %v", results)
	}
}

func TestBatchEvaluator_DefaultConcurrency(t *testing.T) {
	provider := &mockMetricProvider{}
	eval := NewEvaluator(provider)

	// Zero concurrency should default to 4
	batchEval := NewBatchEvaluator(eval, 0)
	if batchEval.concurrency != 4 {
		t.Errorf("expected default concurrency 4, got %d", batchEval.concurrency)
	}

	// Negative concurrency should default to 4
	batchEval = NewBatchEvaluator(eval, -1)
	if batchEval.concurrency != 4 {
		t.Errorf("expected default concurrency 4, got %d", batchEval.concurrency)
	}
}

func TestBatchEvaluator_HandlesErrors(t *testing.T) {
	callCount := 0
	var mu sync.Mutex
	provider := &mockMetricProvider{
		queryFunc: func(_ context.Context, expr string, _ time.Time) (float64, error) {
			mu.Lock()
			callCount++
			mu.Unlock()
			if expr == "failing_metric" {
				return 0, fmt.Errorf("query error")
			}
			return 95, nil
		},
	}

	eval := NewEvaluator(provider)
	batchEval := NewBatchEvaluator(eval, 2)

	rules := []*alerting.AlertRule{
		{
			ID: "ok", Name: "OK", Type: alerting.RuleTypeThreshold,
			Expression: "good_metric", Threshold: 90, Operator: alerting.OpGreaterThan,
			Enabled: true, EvaluationInterval: time.Minute, Labels: map[string]string{},
		},
		{
			ID: "fail", Name: "Fail", Type: alerting.RuleTypeThreshold,
			Expression: "failing_metric", Threshold: 90, Operator: alerting.OpGreaterThan,
			Enabled: true, EvaluationInterval: time.Minute, Labels: map[string]string{},
		},
	}

	results := batchEval.EvaluateBatch(context.Background(), rules)

	// Both rules should produce results (one with error)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	errCount := 0
	for _, r := range results {
		if r.Error != nil {
			errCount++
		}
	}
	if errCount != 1 {
		t.Errorf("expected 1 error result, got %d", errCount)
	}
}

func TestBatchEvaluator_ConcurrencyStress(t *testing.T) {
	// Stress test with many rules and limited concurrency
	var queryCount int64
	var mu sync.Mutex
	provider := &mockMetricProvider{
		queryFunc: func(_ context.Context, _ string, _ time.Time) (float64, error) {
			mu.Lock()
			queryCount++
			mu.Unlock()
			// Small delay to exercise concurrency
			time.Sleep(time.Millisecond)
			return 50, nil
		},
	}

	eval := NewEvaluator(provider)
	batchEval := NewBatchEvaluator(eval, 3)

	numRules := 50
	rules := make([]*alerting.AlertRule, numRules)
	for i := 0; i < numRules; i++ {
		rules[i] = &alerting.AlertRule{
			ID: fmt.Sprintf("stress-%d", i), Name: fmt.Sprintf("Stress %d", i),
			Type: alerting.RuleTypeThreshold, Expression: fmt.Sprintf("m%d", i),
			Threshold: 90, Operator: alerting.OpGreaterThan,
			Enabled: true, EvaluationInterval: time.Minute, Labels: map[string]string{},
		}
	}

	results := batchEval.EvaluateBatch(context.Background(), rules)

	if len(results) != numRules {
		t.Fatalf("expected %d results, got %d", numRules, len(results))
	}

	mu.Lock()
	if queryCount != int64(numRules) {
		t.Errorf("expected %d queries, got %d", numRules, queryCount)
	}
	mu.Unlock()
}

// ---------------------------------------------------------------------------
// Integration-style tests combining parsing + evaluation
// ---------------------------------------------------------------------------

func TestParseAndEvaluate_Integration(t *testing.T) {
	yamlData := []byte(`
groups:
  - name: system-alerts
    interval: 30s
    rules:
      - id: cpu-high
        alert: High CPU Usage
        expr: "avg(cpu_usage)"
        type: threshold
        severity: critical
        threshold: 90
        operator: ">"
      - id: mem-high
        alert: High Memory
        expr: "avg(mem_usage)"
        type: threshold
        severity: warning
        threshold: 85
        operator: ">="
`)

	parser := NewParser()
	ruleSets, err := parser.ParseYAML(yamlData)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	provider := &mockMetricProvider{
		queryFunc: func(_ context.Context, expr string, _ time.Time) (float64, error) {
			switch expr {
			case "avg(cpu_usage)":
				return 95, nil // Above threshold
			case "avg(mem_usage)":
				return 80, nil // Below threshold
			default:
				return 0, fmt.Errorf("unknown expression: %s", expr)
			}
		},
	}

	eval := NewEvaluator(provider)
	results := eval.EvaluateAll(context.Background(), ruleSets[0].Rules)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// Find results by rule ID
	resultMap := make(map[string]*EvaluationResult)
	for _, r := range results {
		resultMap[r.RuleID] = r
	}

	cpuResult := resultMap["cpu-high"]
	if cpuResult == nil {
		t.Fatal("missing cpu-high result")
	}
	if !cpuResult.Firing {
		t.Error("expected cpu-high to be firing (95 > 90)")
	}

	memResult := resultMap["mem-high"]
	if memResult == nil {
		t.Fatal("missing mem-high result")
	}
	if memResult.Firing {
		t.Error("expected mem-high to NOT be firing (80 < 85)")
	}
}

func TestEvaluatedAtTimestamp(t *testing.T) {
	provider := &mockMetricProvider{
		queryFunc: func(_ context.Context, _ string, _ time.Time) (float64, error) {
			return 50, nil
		},
	}

	eval := NewEvaluator(provider)
	before := time.Now()

	rule := &alerting.AlertRule{
		ID:                 "ts-rule",
		Name:               "Timestamp",
		Type:               alerting.RuleTypeThreshold,
		Expression:         "cpu",
		Threshold:          90,
		Operator:           alerting.OpGreaterThan,
		EvaluationInterval: time.Minute,
		Labels:             map[string]string{},
	}

	result, err := eval.Evaluate(context.Background(), rule)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	after := time.Now()
	if result.EvaluatedAt.Before(before) || result.EvaluatedAt.After(after) {
		t.Error("expected EvaluatedAt to be between before and after timestamps")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
