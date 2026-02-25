// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package rules provides alert rule definitions and management.
package rules

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"unheaded/pkg/alerting"
)

// DefaultEvaluationInterval is the default interval for rule evaluation.
const DefaultEvaluationInterval = 1 * time.Minute

// RuleBuilder provides a fluent interface for building alert rules.
type RuleBuilder struct {
	rule *alerting.AlertRule
}

// NewRuleBuilder creates a new rule builder.
func NewRuleBuilder(id, name string) *RuleBuilder {
	return &RuleBuilder{
		rule: &alerting.AlertRule{
			ID:                 id,
			Name:               name,
			Enabled:            true,
			Labels:             make(map[string]string),
			Annotations:        make(map[string]string),
			EvaluationInterval: DefaultEvaluationInterval,
			NotifyChannels:     []string{},
			CreatedAt:          time.Now(),
			UpdatedAt:          time.Now(),
		},
	}
}

// WithDescription sets the rule description.
func (rb *RuleBuilder) WithDescription(desc string) *RuleBuilder {
	rb.rule.Description = desc
	return rb
}

// WithType sets the rule type.
func (rb *RuleBuilder) WithType(ruleType alerting.RuleType) *RuleBuilder {
	rb.rule.Type = ruleType
	return rb
}

// WithSeverity sets the alert severity.
func (rb *RuleBuilder) WithSeverity(severity alerting.AlertSeverity) *RuleBuilder {
	rb.rule.Severity = severity
	return rb
}

// WithExpression sets the rule expression.
func (rb *RuleBuilder) WithExpression(expr string) *RuleBuilder {
	rb.rule.Expression = expr
	return rb
}

// WithThreshold sets the threshold configuration.
func (rb *RuleBuilder) WithThreshold(threshold float64, op alerting.ComparisonOperator) *RuleBuilder {
	rb.rule.Type = alerting.RuleTypeThreshold
	rb.rule.Threshold = threshold
	rb.rule.Operator = op
	return rb
}

// WithForPeriod sets the duration the condition must be true.
func (rb *RuleBuilder) WithForPeriod(duration time.Duration) *RuleBuilder {
	rb.rule.ForPeriod = duration
	return rb
}

// WithMetricName sets the metric name to query.
func (rb *RuleBuilder) WithMetricName(name string) *RuleBuilder {
	rb.rule.MetricName = name
	return rb
}

// WithRateWindow sets the rate calculation window.
func (rb *RuleBuilder) WithRateWindow(window time.Duration, increase float64) *RuleBuilder {
	rb.rule.Type = alerting.RuleTypeRate
	rb.rule.RateWindow = window
	rb.rule.RateIncrease = increase
	return rb
}

// WithAbsenceWindow sets the absence detection window.
func (rb *RuleBuilder) WithAbsenceWindow(window time.Duration) *RuleBuilder {
	rb.rule.Type = alerting.RuleTypeAbsence
	rb.rule.AbsenceWindow = window
	return rb
}

// WithLabel adds a label to the rule.
func (rb *RuleBuilder) WithLabel(key, value string) *RuleBuilder {
	rb.rule.Labels[key] = value
	return rb
}

// WithAnnotation adds an annotation to the rule.
func (rb *RuleBuilder) WithAnnotation(key, value string) *RuleBuilder {
	rb.rule.Annotations[key] = value
	return rb
}

// WithEvaluationInterval sets the evaluation interval.
func (rb *RuleBuilder) WithEvaluationInterval(interval time.Duration) *RuleBuilder {
	rb.rule.EvaluationInterval = interval
	return rb
}

// WithNotifyChannels sets the notification channels.
func (rb *RuleBuilder) WithNotifyChannels(channels ...string) *RuleBuilder {
	rb.rule.NotifyChannels = channels
	return rb
}

// Disabled sets the rule as disabled.
func (rb *RuleBuilder) Disabled() *RuleBuilder {
	rb.rule.Enabled = false
	return rb
}

// Build finalizes and returns the rule.
func (rb *RuleBuilder) Build() (*alerting.AlertRule, error) {
	if err := ValidateRule(rb.rule); err != nil {
		return nil, err
	}
	return rb.rule, nil
}

// ValidateRule validates an alert rule.
func ValidateRule(rule *alerting.AlertRule) error {
	if rule.ID == "" {
		return fmt.Errorf("%w: rule ID is required", alerting.ErrInvalidRule)
	}
	if rule.Name == "" {
		return fmt.Errorf("%w: rule name is required", alerting.ErrInvalidRule)
	}
	if rule.Type == "" {
		return fmt.Errorf("%w: rule type is required", alerting.ErrInvalidRule)
	}

	switch rule.Type {
	case alerting.RuleTypeThreshold:
		if rule.Operator == "" {
			return fmt.Errorf("%w: threshold rule requires operator", alerting.ErrInvalidRule)
		}
		if rule.Expression == "" && rule.MetricName == "" {
			return fmt.Errorf("%w: threshold rule requires expression or metric name", alerting.ErrInvalidRule)
		}
	case alerting.RuleTypeRate:
		if rule.RateWindow <= 0 {
			return fmt.Errorf("%w: rate rule requires positive rate window", alerting.ErrInvalidRule)
		}
	case alerting.RuleTypeAbsence:
		if rule.AbsenceWindow <= 0 {
			return fmt.Errorf("%w: absence rule requires positive absence window", alerting.ErrInvalidRule)
		}
	default:
		return fmt.Errorf("%w: unknown rule type: %s", alerting.ErrInvalidRule, rule.Type)
	}

	if rule.EvaluationInterval <= 0 {
		return fmt.Errorf("%w: evaluation interval must be positive", alerting.ErrInvalidRule)
	}

	return nil
}

// GenerateAlertFingerprint generates a unique fingerprint for an alert.
func GenerateAlertFingerprint(ruleID string, labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	parts = append(parts, ruleID)
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, labels[k]))
	}

	hash := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(hash[:8])
}

// CreateAlertFromRule creates a new alert instance from a rule evaluation.
func CreateAlertFromRule(rule *alerting.AlertRule, value float64, labels map[string]string) *alerting.Alert {
	now := time.Now()

	// Merge rule labels with evaluation labels
	mergedLabels := make(map[string]string)
	for k, v := range rule.Labels {
		mergedLabels[k] = v
	}
	for k, v := range labels {
		mergedLabels[k] = v
	}

	return &alerting.Alert{
		ID:          fmt.Sprintf("%s-%d", rule.ID, now.UnixNano()),
		RuleID:      rule.ID,
		Name:        rule.Name,
		Description: rule.Description,
		State:       alerting.AlertStatePending,
		Severity:    rule.Severity,
		Labels:      mergedLabels,
		Annotations: rule.Annotations,
		Value:       value,
		Threshold:   rule.Threshold,
		StartsAt:    now,
		Fingerprint: GenerateAlertFingerprint(rule.ID, mergedLabels),
	}
}

// RuleSet represents a collection of rules.
type RuleSet struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Rules       []*alerting.AlertRule  `json:"rules"`
	Labels      map[string]string      `json:"labels"`
}

// NewRuleSet creates a new rule set.
func NewRuleSet(name string) *RuleSet {
	return &RuleSet{
		Name:   name,
		Rules:  make([]*alerting.AlertRule, 0),
		Labels: make(map[string]string),
	}
}

// AddRule adds a rule to the set.
func (rs *RuleSet) AddRule(rule *alerting.AlertRule) error {
	if err := ValidateRule(rule); err != nil {
		return err
	}

	// Apply rule set labels
	for k, v := range rs.Labels {
		if _, exists := rule.Labels[k]; !exists {
			rule.Labels[k] = v
		}
	}

	rs.Rules = append(rs.Rules, rule)
	return nil
}

// GetRule retrieves a rule by ID.
func (rs *RuleSet) GetRule(id string) (*alerting.AlertRule, bool) {
	for _, rule := range rs.Rules {
		if rule.ID == id {
			return rule, true
		}
	}
	return nil, false
}

// RemoveRule removes a rule from the set.
func (rs *RuleSet) RemoveRule(id string) bool {
	for i, rule := range rs.Rules {
		if rule.ID == id {
			rs.Rules = append(rs.Rules[:i], rs.Rules[i+1:]...)
			return true
		}
	}
	return false
}
