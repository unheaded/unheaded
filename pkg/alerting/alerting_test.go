// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package alerting_test

import (
	"context"
	"testing"
	"time"

	"unheaded/pkg/alerting"
	"unheaded/pkg/alerting/manager"
	"unheaded/pkg/alerting/notify"
	"unheaded/pkg/alerting/rules"
)

// MockMetricProvider is a mock implementation of MetricProvider for testing.
type MockMetricProvider struct {
	Values      map[string]float64
	RangeValues map[string][]float64
	Error       error
}

func NewMockMetricProvider() *MockMetricProvider {
	return &MockMetricProvider{
		Values:      make(map[string]float64),
		RangeValues: make(map[string][]float64),
	}
}

func (m *MockMetricProvider) Query(ctx context.Context, expression string, timestamp time.Time) (float64, error) {
	if m.Error != nil {
		return 0, m.Error
	}
	if val, ok := m.Values[expression]; ok {
		return val, nil
	}
	return 0, nil
}

func (m *MockMetricProvider) QueryRange(ctx context.Context, expression string, start, end time.Time, step time.Duration) ([]float64, error) {
	if m.Error != nil {
		return nil, m.Error
	}
	if vals, ok := m.RangeValues[expression]; ok {
		return vals, nil
	}
	return []float64{}, nil
}

func TestAlertRule_Validation(t *testing.T) {
	tests := []struct {
		name    string
		rule    *alerting.AlertRule
		wantErr bool
	}{
		{
			name: "valid threshold rule",
			rule: &alerting.AlertRule{
				ID:                 "test-rule-1",
				Name:               "Test Rule",
				Type:               alerting.RuleTypeThreshold,
				Expression:         "cpu_usage",
				Operator:           alerting.OpGreaterThan,
				Threshold:          80,
				EvaluationInterval: time.Minute,
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			rule: &alerting.AlertRule{
				Name:               "Test Rule",
				Type:               alerting.RuleTypeThreshold,
				Expression:         "cpu_usage",
				Operator:           alerting.OpGreaterThan,
				EvaluationInterval: time.Minute,
			},
			wantErr: true,
		},
		{
			name: "missing name",
			rule: &alerting.AlertRule{
				ID:                 "test-rule",
				Type:               alerting.RuleTypeThreshold,
				Expression:         "cpu_usage",
				Operator:           alerting.OpGreaterThan,
				EvaluationInterval: time.Minute,
			},
			wantErr: true,
		},
		{
			name: "threshold rule missing operator",
			rule: &alerting.AlertRule{
				ID:                 "test-rule",
				Name:               "Test Rule",
				Type:               alerting.RuleTypeThreshold,
				Expression:         "cpu_usage",
				EvaluationInterval: time.Minute,
			},
			wantErr: true,
		},
		{
			name: "valid rate rule",
			rule: &alerting.AlertRule{
				ID:                 "rate-rule",
				Name:               "Rate Rule",
				Type:               alerting.RuleTypeRate,
				Expression:         "request_count",
				RateWindow:         5 * time.Minute,
				RateIncrease:       100,
				EvaluationInterval: time.Minute,
			},
			wantErr: false,
		},
		{
			name: "rate rule missing window",
			rule: &alerting.AlertRule{
				ID:                 "rate-rule",
				Name:               "Rate Rule",
				Type:               alerting.RuleTypeRate,
				Expression:         "request_count",
				EvaluationInterval: time.Minute,
			},
			wantErr: true,
		},
		{
			name: "valid absence rule",
			rule: &alerting.AlertRule{
				ID:                 "absence-rule",
				Name:               "Absence Rule",
				Type:               alerting.RuleTypeAbsence,
				Expression:         "heartbeat",
				AbsenceWindow:      5 * time.Minute,
				EvaluationInterval: time.Minute,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rules.ValidateRule(tt.rule)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRule() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRuleBuilder(t *testing.T) {
	rule, err := rules.NewRuleBuilder("cpu-high", "High CPU Usage").
		WithDescription("CPU usage is above threshold").
		WithThreshold(80, alerting.OpGreaterThan).
		WithExpression("avg(cpu_usage)").
		WithSeverity(alerting.SeverityCritical).
		WithForPeriod(5*time.Minute).
		WithLabel("team", "platform").
		WithAnnotation("runbook", "https://wiki.example.com/cpu-high").
		WithNotifyChannels("slack", "pagerduty").
		Build()

	if err != nil {
		t.Fatalf("Failed to build rule: %v", err)
	}

	if rule.ID != "cpu-high" {
		t.Errorf("Expected ID 'cpu-high', got '%s'", rule.ID)
	}
	if rule.Name != "High CPU Usage" {
		t.Errorf("Expected Name 'High CPU Usage', got '%s'", rule.Name)
	}
	if rule.Threshold != 80 {
		t.Errorf("Expected Threshold 80, got %f", rule.Threshold)
	}
	if rule.Operator != alerting.OpGreaterThan {
		t.Errorf("Expected Operator '>', got '%s'", rule.Operator)
	}
	if rule.Severity != alerting.SeverityCritical {
		t.Errorf("Expected Severity 'critical', got '%s'", rule.Severity)
	}
	if len(rule.NotifyChannels) != 2 {
		t.Errorf("Expected 2 notify channels, got %d", len(rule.NotifyChannels))
	}
}

func TestEvaluator_ThresholdRule(t *testing.T) {
	provider := NewMockMetricProvider()
	provider.Values["cpu_usage"] = 85

	evaluator := rules.NewEvaluator(provider)

	rule := &alerting.AlertRule{
		ID:                 "cpu-high",
		Name:               "High CPU",
		Type:               alerting.RuleTypeThreshold,
		Expression:         "cpu_usage",
		Threshold:          80,
		Operator:           alerting.OpGreaterThan,
		EvaluationInterval: time.Minute,
		Enabled:            true,
	}

	ctx := context.Background()
	result, err := evaluator.Evaluate(ctx, rule)
	if err != nil {
		t.Fatalf("Evaluation failed: %v", err)
	}

	if !result.Firing {
		t.Error("Expected rule to be firing")
	}
	if result.Value != 85 {
		t.Errorf("Expected value 85, got %f", result.Value)
	}
}

func TestEvaluator_ThresholdNotFiring(t *testing.T) {
	provider := NewMockMetricProvider()
	provider.Values["cpu_usage"] = 50

	evaluator := rules.NewEvaluator(provider)

	rule := &alerting.AlertRule{
		ID:                 "cpu-high",
		Name:               "High CPU",
		Type:               alerting.RuleTypeThreshold,
		Expression:         "cpu_usage",
		Threshold:          80,
		Operator:           alerting.OpGreaterThan,
		EvaluationInterval: time.Minute,
		Enabled:            true,
	}

	ctx := context.Background()
	result, err := evaluator.Evaluate(ctx, rule)
	if err != nil {
		t.Fatalf("Evaluation failed: %v", err)
	}

	if result.Firing {
		t.Error("Expected rule to not be firing")
	}
}

func TestManager_RuleLifecycle(t *testing.T) {
	provider := NewMockMetricProvider()
	mgr := manager.NewManager(provider, manager.DefaultConfig())

	ctx := context.Background()

	// Create rule
	rule := &alerting.AlertRule{
		ID:                 "test-rule",
		Name:               "Test Rule",
		Type:               alerting.RuleTypeThreshold,
		Expression:         "test_metric",
		Threshold:          50,
		Operator:           alerting.OpGreaterThan,
		EvaluationInterval: time.Minute,
		Enabled:            true,
		Labels:             make(map[string]string),
		Annotations:        make(map[string]string),
	}

	if err := mgr.CreateRule(ctx, rule); err != nil {
		t.Fatalf("Failed to create rule: %v", err)
	}

	// Get rule
	retrieved, err := mgr.GetRule(ctx, "test-rule")
	if err != nil {
		t.Fatalf("Failed to get rule: %v", err)
	}
	if retrieved.Name != "Test Rule" {
		t.Errorf("Expected name 'Test Rule', got '%s'", retrieved.Name)
	}

	// List rules
	ruleList, err := mgr.ListRules(ctx)
	if err != nil {
		t.Fatalf("Failed to list rules: %v", err)
	}
	if len(ruleList) != 1 {
		t.Errorf("Expected 1 rule, got %d", len(ruleList))
	}

	// Update rule
	rule.Threshold = 60
	if err := mgr.UpdateRule(ctx, rule); err != nil {
		t.Fatalf("Failed to update rule: %v", err)
	}

	retrieved, _ = mgr.GetRule(ctx, "test-rule")
	if retrieved.Threshold != 60 {
		t.Errorf("Expected threshold 60, got %f", retrieved.Threshold)
	}

	// Delete rule
	if err := mgr.DeleteRule(ctx, "test-rule"); err != nil {
		t.Fatalf("Failed to delete rule: %v", err)
	}

	_, err = mgr.GetRule(ctx, "test-rule")
	if err != alerting.ErrRuleNotFound {
		t.Errorf("Expected ErrRuleNotFound, got %v", err)
	}
}

func TestManager_SilenceLifecycle(t *testing.T) {
	provider := NewMockMetricProvider()
	mgr := manager.NewManager(provider, manager.DefaultConfig())

	ctx := context.Background()

	silence := manager.NewSilenceBuilder("silence-1").
		WithEqualMatcher("alertname", "TestAlert").
		WithDuration(2 * time.Hour).
		WithCreator("admin").
		WithComment("Planned maintenance").
		Build()

	if err := mgr.Silence(ctx, silence); err != nil {
		t.Fatalf("Failed to create silence: %v", err)
	}

	silences, err := mgr.GetSilences(ctx)
	if err != nil {
		t.Fatalf("Failed to get silences: %v", err)
	}
	if len(silences) != 1 {
		t.Errorf("Expected 1 silence, got %d", len(silences))
	}

	if err := mgr.DeleteSilence(ctx, "silence-1"); err != nil {
		t.Fatalf("Failed to delete silence: %v", err)
	}

	silences, _ = mgr.GetSilences(ctx)
	if len(silences) != 0 {
		t.Errorf("Expected 0 silences, got %d", len(silences))
	}
}

func TestSilencer_IsSilenced(t *testing.T) {
	silencer := manager.NewSilencer()

	silence := manager.NewSilenceBuilder("test-silence").
		WithEqualMatcher("alertname", "HighCPU").
		WithEqualMatcher("severity", "warning").
		WithDuration(1 * time.Hour).
		Build()

	if err := silencer.AddSilence(silence); err != nil {
		t.Fatalf("Failed to add silence: %v", err)
	}

	// Alert that should be silenced
	alert1 := &alerting.Alert{
		Name:     "HighCPU",
		Severity: alerting.SeverityWarning,
		Labels:   map[string]string{"alertname": "HighCPU"},
	}

	if !silencer.IsSilenced(alert1) {
		t.Error("Expected alert to be silenced")
	}

	// Alert that should not be silenced
	alert2 := &alerting.Alert{
		Name:     "HighMemory",
		Severity: alerting.SeverityWarning,
		Labels:   map[string]string{"alertname": "HighMemory"},
	}

	if silencer.IsSilenced(alert2) {
		t.Error("Expected alert to not be silenced")
	}
}

func TestInhibitor_IsInhibited(t *testing.T) {
	inhibitor := manager.NewInhibitor()

	rule := manager.NewInhibitionBuilder("inhibit-warning").
		WithSourceEqual("severity", "critical").
		WithTargetEqual("severity", "warning").
		WithEqualLabels("alertname").
		Build()

	if err := inhibitor.AddRule(rule); err != nil {
		t.Fatalf("Failed to add inhibition rule: %v", err)
	}

	// Critical alert (source)
	criticalAlert := &alerting.Alert{
		ID:          "alert-1",
		Name:        "HighCPU",
		Severity:    alerting.SeverityCritical,
		State:       alerting.AlertStateFiring,
		Labels:      map[string]string{"alertname": "HighCPU"},
		Fingerprint: "fp1",
	}

	// Warning alert (target) - should be inhibited
	warningAlert := &alerting.Alert{
		ID:          "alert-2",
		Name:        "HighCPU",
		Severity:    alerting.SeverityWarning,
		State:       alerting.AlertStateFiring,
		Labels:      map[string]string{"alertname": "HighCPU"},
		Fingerprint: "fp2",
	}

	activeAlerts := []*alerting.Alert{criticalAlert}

	if !inhibitor.IsInhibited(warningAlert, activeAlerts) {
		t.Error("Expected warning alert to be inhibited by critical alert")
	}

	// Different alertname - should not be inhibited
	differentAlert := &alerting.Alert{
		ID:          "alert-3",
		Name:        "HighMemory",
		Severity:    alerting.SeverityWarning,
		State:       alerting.AlertStateFiring,
		Labels:      map[string]string{"alertname": "HighMemory"},
		Fingerprint: "fp3",
	}

	if inhibitor.IsInhibited(differentAlert, activeAlerts) {
		t.Error("Expected different alert to not be inhibited")
	}
}

func TestGrouper_Group(t *testing.T) {
	grouper := manager.NewGrouper(30*time.Second, 5*time.Minute)
	grouper.SetGroupBy([]string{"alertname", "severity"})

	alerts := []*alerting.Alert{
		{
			ID:       "alert-1",
			Name:     "HighCPU",
			Severity: alerting.SeverityCritical,
			Labels:   map[string]string{"host": "server1"},
		},
		{
			ID:       "alert-2",
			Name:     "HighCPU",
			Severity: alerting.SeverityCritical,
			Labels:   map[string]string{"host": "server2"},
		},
		{
			ID:       "alert-3",
			Name:     "HighMemory",
			Severity: alerting.SeverityWarning,
			Labels:   map[string]string{"host": "server1"},
		},
	}

	groups := grouper.Group(alerts)

	if len(groups) != 2 {
		t.Errorf("Expected 2 groups, got %d", len(groups))
	}

	// Find the HighCPU group
	var cpuGroup *alerting.AlertGroup
	for _, g := range groups {
		if g.Labels["alertname"] == "HighCPU" {
			cpuGroup = g
			break
		}
	}

	if cpuGroup == nil {
		t.Fatal("Expected to find HighCPU group")
	}

	if len(cpuGroup.Alerts) != 2 {
		t.Errorf("Expected 2 alerts in HighCPU group, got %d", len(cpuGroup.Alerts))
	}
}

func TestRuleParser_ParseYAML(t *testing.T) {
	yamlConfig := `
groups:
  - name: test-group
    interval: 1m
    labels:
      team: platform
    rules:
      - id: cpu-high
        alert: HighCPU
        expr: avg(cpu_usage) > 80
        for: 5m
        severity: critical
        threshold: 80
        operator: ">"
        labels:
          component: compute
        annotations:
          summary: CPU usage is high
        channels:
          - slack
          - pagerduty
`

	parser := rules.NewParser()
	ruleSets, err := parser.ParseYAML([]byte(yamlConfig))
	if err != nil {
		t.Fatalf("Failed to parse YAML: %v", err)
	}

	if len(ruleSets) != 1 {
		t.Fatalf("Expected 1 rule set, got %d", len(ruleSets))
	}

	rs := ruleSets[0]
	if rs.Name != "test-group" {
		t.Errorf("Expected name 'test-group', got '%s'", rs.Name)
	}

	if len(rs.Rules) != 1 {
		t.Fatalf("Expected 1 rule, got %d", len(rs.Rules))
	}

	rule := rs.Rules[0]
	if rule.ID != "cpu-high" {
		t.Errorf("Expected ID 'cpu-high', got '%s'", rule.ID)
	}
	if rule.Severity != alerting.SeverityCritical {
		t.Errorf("Expected severity 'critical', got '%s'", rule.Severity)
	}
	if rule.Threshold != 80 {
		t.Errorf("Expected threshold 80, got %f", rule.Threshold)
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
		wantErr  bool
	}{
		{"5m", 5 * time.Minute, false},
		{"1h", time.Hour, false},
		{"30s", 30 * time.Second, false},
		{"1d", 24 * time.Hour, false},
		{"1w", 7 * 24 * time.Hour, false},
		{"1h30m", 90 * time.Minute, false},
		{"invalid", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := rules.ParseDuration(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseDuration(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && result != tt.expected {
				t.Errorf("ParseDuration(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGenerateAlertFingerprint(t *testing.T) {
	labels1 := map[string]string{"a": "1", "b": "2"}
	labels2 := map[string]string{"b": "2", "a": "1"}
	labels3 := map[string]string{"a": "1", "b": "3"}

	fp1 := rules.GenerateAlertFingerprint("rule1", labels1)
	fp2 := rules.GenerateAlertFingerprint("rule1", labels2)
	fp3 := rules.GenerateAlertFingerprint("rule1", labels3)
	fp4 := rules.GenerateAlertFingerprint("rule2", labels1)

	// Same labels in different order should produce same fingerprint
	if fp1 != fp2 {
		t.Error("Expected same fingerprint for labels in different order")
	}

	// Different labels should produce different fingerprint
	if fp1 == fp3 {
		t.Error("Expected different fingerprint for different labels")
	}

	// Different rule ID should produce different fingerprint
	if fp1 == fp4 {
		t.Error("Expected different fingerprint for different rule ID")
	}
}

func TestNotificationRouter(t *testing.T) {
	router := notify.NewRouter("default")

	// Create mock channels
	mockPublisher := notify.NewMockWotanPublisher()
	wotanChannel := notify.NewWotanChannel(notify.WotanConfig{
		ChannelConfig: notify.ChannelConfig{Name: "wotan"},
		Topic:         "alerts",
	}, mockPublisher)
	defer wotanChannel.Close()

	router.RegisterChannel(wotanChannel)

	router.AddRoute(notify.Route{
		Matchers: []alerting.LabelMatcher{
			{Name: "severity", Value: "critical", Type: alerting.MatchTypeEqual},
		},
		Channel: "wotan",
	})

	ctx := context.Background()
	alerts := []*alerting.Alert{
		{
			ID:       "alert-1",
			Name:     "CriticalAlert",
			Severity: alerting.SeverityCritical,
			State:    alerting.AlertStateFiring,
			Labels:   map[string]string{},
		},
	}

	statuses := router.Route(ctx, alerts)

	if len(statuses) != 1 {
		t.Errorf("Expected 1 status, got %d", len(statuses))
	}

	if !statuses[0].Success {
		t.Errorf("Expected successful notification, got error: %s", statuses[0].Error)
	}
}

func TestWotanChannel(t *testing.T) {
	mockPublisher := notify.NewMockWotanPublisher()
	channel := notify.NewWotanChannel(notify.WotanConfig{
		ChannelConfig: notify.ChannelConfig{Name: "wotan-test"},
		Topic:         "test.alerts",
		FlushInterval: 10 * time.Millisecond, // Short interval for testing
	}, mockPublisher)
	defer channel.Close()

	ctx := context.Background()
	alerts := []*alerting.Alert{
		{
			ID:          "alert-1",
			Name:        "TestAlert",
			Description: "Test description",
			Severity:    alerting.SeverityCritical,
			State:       alerting.AlertStateFiring,
			Labels:      map[string]string{"env": "prod"},
			Annotations: map[string]string{"summary": "Test alert"},
			Value:       95.5,
			Threshold:   80,
			StartsAt:    time.Now(),
			Fingerprint: "test-fp",
		},
	}

	if err := channel.Send(ctx, alerts); err != nil {
		t.Fatalf("Failed to send alert: %v", err)
	}

	// Wait a bit for async processing
	time.Sleep(100 * time.Millisecond)

	if mockPublisher.EventCount() == 0 {
		t.Error("Expected at least one event to be published")
	}
}
