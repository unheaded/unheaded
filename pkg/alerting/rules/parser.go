// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package rules provides rule parsing functionality.
package rules

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"unheaded/pkg/alerting"
	"gopkg.in/yaml.v3"
)

// RuleConfig represents the YAML/JSON configuration for rules.
type RuleConfig struct {
	Groups []RuleGroupConfig `yaml:"groups" json:"groups"`
}

// RuleGroupConfig represents a group of rules in configuration.
type RuleGroupConfig struct {
	Name     string            `yaml:"name" json:"name"`
	Interval string            `yaml:"interval" json:"interval"`
	Rules    []RuleDefinition  `yaml:"rules" json:"rules"`
	Labels   map[string]string `yaml:"labels" json:"labels"`
}

// RuleDefinition represents a single rule in configuration.
type RuleDefinition struct {
	ID          string            `yaml:"id" json:"id"`
	Alert       string            `yaml:"alert" json:"alert"`
	Expr        string            `yaml:"expr" json:"expr"`
	For         string            `yaml:"for" json:"for"`
	Type        string            `yaml:"type" json:"type"`
	Severity    string            `yaml:"severity" json:"severity"`
	Labels      map[string]string `yaml:"labels" json:"labels"`
	Annotations map[string]string `yaml:"annotations" json:"annotations"`
	Threshold   *float64          `yaml:"threshold" json:"threshold"`
	Operator    string            `yaml:"operator" json:"operator"`
	Channels    []string          `yaml:"channels" json:"channels"`
}

// Parser handles parsing rule configurations.
type Parser struct {
	defaultInterval time.Duration
	defaultSeverity alerting.AlertSeverity
}

// NewParser creates a new rule parser.
func NewParser() *Parser {
	return &Parser{
		defaultInterval: DefaultEvaluationInterval,
		defaultSeverity: alerting.SeverityWarning,
	}
}

// WithDefaultInterval sets the default evaluation interval.
func (p *Parser) WithDefaultInterval(interval time.Duration) *Parser {
	p.defaultInterval = interval
	return p
}

// WithDefaultSeverity sets the default severity.
func (p *Parser) WithDefaultSeverity(severity alerting.AlertSeverity) *Parser {
	p.defaultSeverity = severity
	return p
}

// ParseYAML parses YAML rule configuration.
func (p *Parser) ParseYAML(data []byte) ([]*RuleSet, error) {
	var config RuleConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}
	return p.processConfig(&config)
}

// ParseJSON parses JSON rule configuration.
func (p *Parser) ParseJSON(data []byte) ([]*RuleSet, error) {
	var config RuleConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}
	return p.processConfig(&config)
}

// processConfig processes the parsed configuration.
func (p *Parser) processConfig(config *RuleConfig) ([]*RuleSet, error) {
	ruleSets := make([]*RuleSet, 0, len(config.Groups))

	for _, group := range config.Groups {
		rs, err := p.processGroup(&group)
		if err != nil {
			return nil, fmt.Errorf("error processing group %s: %w", group.Name, err)
		}
		ruleSets = append(ruleSets, rs)
	}

	return ruleSets, nil
}

// processGroup processes a rule group configuration.
func (p *Parser) processGroup(group *RuleGroupConfig) (*RuleSet, error) {
	rs := NewRuleSet(group.Name)
	rs.Labels = group.Labels

	interval := p.defaultInterval
	if group.Interval != "" {
		parsed, err := ParseDuration(group.Interval)
		if err != nil {
			return nil, fmt.Errorf("invalid interval %q: %w", group.Interval, err)
		}
		interval = parsed
	}

	for i, ruleDef := range group.Rules {
		rule, err := p.processRuleDefinition(&ruleDef, interval)
		if err != nil {
			return nil, fmt.Errorf("error in rule %d: %w", i, err)
		}
		if err := rs.AddRule(rule); err != nil {
			return nil, err
		}
	}

	return rs, nil
}

// processRuleDefinition converts a rule definition to an AlertRule.
func (p *Parser) processRuleDefinition(def *RuleDefinition, interval time.Duration) (*alerting.AlertRule, error) {
	rule := &alerting.AlertRule{
		ID:                 def.ID,
		Name:               def.Alert,
		Expression:         def.Expr,
		Labels:             def.Labels,
		Annotations:        def.Annotations,
		EvaluationInterval: interval,
		NotifyChannels:     def.Channels,
		Enabled:            true,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	if rule.ID == "" {
		rule.ID = GenerateRuleID(def.Alert)
	}

	if rule.Labels == nil {
		rule.Labels = make(map[string]string)
	}
	if rule.Annotations == nil {
		rule.Annotations = make(map[string]string)
	}

	// Parse type
	ruleType, err := ParseRuleType(def.Type)
	if err != nil {
		ruleType = alerting.RuleTypeThreshold
	}
	rule.Type = ruleType

	// Parse severity
	severity, err := ParseSeverity(def.Severity)
	if err != nil {
		severity = p.defaultSeverity
	}
	rule.Severity = severity

	// Parse for duration
	if def.For != "" {
		forDuration, err := ParseDuration(def.For)
		if err != nil {
			return nil, fmt.Errorf("invalid for duration %q: %w", def.For, err)
		}
		rule.ForPeriod = forDuration
	}

	// Parse threshold and operator
	if def.Threshold != nil {
		rule.Threshold = *def.Threshold
	}
	if def.Operator != "" {
		op, err := ParseOperator(def.Operator)
		if err != nil {
			return nil, fmt.Errorf("invalid operator %q: %w", def.Operator, err)
		}
		rule.Operator = op
	}

	return rule, nil
}

// ParseDuration parses a duration string (supports Prometheus-style durations).
func ParseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}

	// Try standard Go duration first
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}

	// Parse Prometheus-style duration (e.g., "5m", "1h30m", "1d")
	re := regexp.MustCompile(`^(\d+)([smhdwy])$`)
	matches := re.FindStringSubmatch(s)
	if len(matches) != 3 {
		return 0, fmt.Errorf("invalid duration format: %s", s)
	}

	value, _ := strconv.Atoi(matches[1])
	unit := matches[2]

	switch unit {
	case "s":
		return time.Duration(value) * time.Second, nil
	case "m":
		return time.Duration(value) * time.Minute, nil
	case "h":
		return time.Duration(value) * time.Hour, nil
	case "d":
		return time.Duration(value) * 24 * time.Hour, nil
	case "w":
		return time.Duration(value) * 7 * 24 * time.Hour, nil
	case "y":
		return time.Duration(value) * 365 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unknown duration unit: %s", unit)
	}
}

// ParseRuleType parses a rule type string.
func ParseRuleType(s string) (alerting.RuleType, error) {
	switch strings.ToLower(s) {
	case "threshold", "":
		return alerting.RuleTypeThreshold, nil
	case "rate":
		return alerting.RuleTypeRate, nil
	case "absence":
		return alerting.RuleTypeAbsence, nil
	default:
		return "", fmt.Errorf("unknown rule type: %s", s)
	}
}

// ParseSeverity parses a severity string.
func ParseSeverity(s string) (alerting.AlertSeverity, error) {
	switch strings.ToLower(s) {
	case "critical", "crit":
		return alerting.SeverityCritical, nil
	case "warning", "warn":
		return alerting.SeverityWarning, nil
	case "info", "informational":
		return alerting.SeverityInfo, nil
	default:
		return "", fmt.Errorf("unknown severity: %s", s)
	}
}

// ParseOperator parses a comparison operator string.
func ParseOperator(s string) (alerting.ComparisonOperator, error) {
	switch s {
	case ">", "gt":
		return alerting.OpGreaterThan, nil
	case ">=", "gte", "ge":
		return alerting.OpGreaterThanEqual, nil
	case "<", "lt":
		return alerting.OpLessThan, nil
	case "<=", "lte", "le":
		return alerting.OpLessThanEqual, nil
	case "==", "eq":
		return alerting.OpEqual, nil
	case "!=", "ne", "neq":
		return alerting.OpNotEqual, nil
	default:
		return "", fmt.Errorf("unknown operator: %s", s)
	}
}

// GenerateRuleID generates a rule ID from the alert name.
func GenerateRuleID(name string) string {
	// Convert to lowercase and replace spaces/special chars with underscores
	re := regexp.MustCompile(`[^a-zA-Z0-9]+`)
	id := re.ReplaceAllString(strings.ToLower(name), "_")
	id = strings.Trim(id, "_")
	return id
}
