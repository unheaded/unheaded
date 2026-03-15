// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package rules provides rule parsing functionality
package rules

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// RuleConfig represents a rule configuration for parsing
type RuleConfig struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Pattern     string   `json:"pattern"`
	Targets     []string `json:"targets"`
	Action      string   `json:"action"`
	Severity    string   `json:"severity"`
	Score       int      `json:"score,omitempty"`
	Enabled     bool     `json:"enabled"`
	Tags        []string `json:"tags,omitempty"`
	Message     string   `json:"message,omitempty"`
	RedirectURL string   `json:"redirect_url,omitempty"`
}

// RuleSetConfig represents a rule set configuration
type RuleSetConfig struct {
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Version     string       `json:"version"`
	Rules       []RuleConfig `json:"rules"`
}

// Parser parses rule configurations
type Parser struct {
	strict bool
}

// NewParser creates a new rule parser
func NewParser(strict bool) *Parser {
	return &Parser{strict: strict}
}

// ParseJSON parses rules from JSON
func (p *Parser) ParseJSON(r io.Reader) (*RuleSet, error) {
	var config RuleSetConfig
	decoder := json.NewDecoder(r)
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return p.parseConfig(&config)
}

// ParseJSONString parses rules from a JSON string
func (p *Parser) ParseJSONString(s string) (*RuleSet, error) {
	return p.ParseJSON(strings.NewReader(s))
}

// parseConfig converts a RuleSetConfig to a RuleSet
func (p *Parser) parseConfig(config *RuleSetConfig) (*RuleSet, error) {
	rs := NewRuleSet(config.Name, config.Description)

	for i, rc := range config.Rules {
		rule, err := p.parseRuleConfig(&rc)
		if err != nil {
			if p.strict {
				return nil, fmt.Errorf("rule %d (%s): %w", i, rc.ID, err)
			}
			continue
		}
		rs.AddRule(rule)
	}

	return rs, nil
}

// parseRuleConfig converts a RuleConfig to a Rule
func (p *Parser) parseRuleConfig(rc *RuleConfig) (*Rule, error) {
	if rc.ID == "" {
		return nil, errors.New("rule ID is required")
	}
	if rc.Pattern == "" {
		return nil, errors.New("rule pattern is required")
	}

	targets, err := parseTargets(rc.Targets)
	if err != nil {
		return nil, err
	}

	action, err := parseAction(rc.Action)
	if err != nil {
		return nil, err
	}

	severity, err := parseSeverity(rc.Severity)
	if err != nil {
		return nil, err
	}

	rule, err := NewRule(rc.ID, rc.Name, rc.Pattern, targets, action, severity)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern: %w", err)
	}

	rule.Description = rc.Description
	rule.Enabled = rc.Enabled
	rule.Tags = rc.Tags
	rule.Message = rc.Message
	rule.RedirectURL = rc.RedirectURL

	if rc.Score > 0 {
		rule.Score = rc.Score
	}

	return rule, nil
}

// parseTargets parses target strings to Target values
func parseTargets(targets []string) ([]Target, error) {
	if len(targets) == 0 {
		return []Target{TargetAll}, nil
	}

	result := make([]Target, 0, len(targets))
	for _, t := range targets {
		target, err := parseTarget(t)
		if err != nil {
			return nil, err
		}
		result = append(result, target)
	}
	return result, nil
}

// parseTarget parses a single target string
func parseTarget(s string) (Target, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "url":
		return TargetURL, nil
	case "path":
		return TargetPath, nil
	case "query":
		return TargetQuery, nil
	case "body":
		return TargetBody, nil
	case "headers":
		return TargetHeaders, nil
	case "cookies":
		return TargetCookies, nil
	case "method":
		return TargetMethod, nil
	case "ip":
		return TargetIP, nil
	case "useragent", "user_agent", "user-agent":
		return TargetUserAgent, nil
	case "all", "*":
		return TargetAll, nil
	default:
		return TargetAll, fmt.Errorf("unknown target: %s", s)
	}
}

// parseAction parses an action string
func parseAction(s string) (Action, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "allow":
		return ActionAllow, nil
	case "block":
		return ActionBlock, nil
	case "log":
		return ActionLog, nil
	case "redirect":
		return ActionRedirect, nil
	case "challenge":
		return ActionChallenge, nil
	default:
		return ActionBlock, fmt.Errorf("unknown action: %s", s)
	}
}

// parseSeverity parses a severity string
func parseSeverity(s string) (Severity, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "info":
		return SeverityInfo, nil
	case "low":
		return SeverityLow, nil
	case "medium", "med":
		return SeverityMedium, nil
	case "high":
		return SeverityHigh, nil
	case "critical", "crit":
		return SeverityCritical, nil
	default:
		return SeverityMedium, fmt.Errorf("unknown severity: %s", s)
	}
}

// ParseSimple parses simple rule definitions (one per line)
// Format: ID|NAME|PATTERN|TARGETS|ACTION|SEVERITY
func (p *Parser) ParseSimple(r io.Reader) (*RuleSet, error) {
	rs := NewRuleSet("simple-rules", "Rules parsed from simple format")
	scanner := bufio.NewScanner(r)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) < 4 {
			if p.strict {
				return nil, fmt.Errorf("line %d: insufficient fields", lineNum)
			}
			continue
		}

		id := strings.TrimSpace(parts[0])
		name := strings.TrimSpace(parts[1])
		pattern := strings.TrimSpace(parts[2])
		targetStr := strings.TrimSpace(parts[3])

		actionStr := "block"
		if len(parts) > 4 {
			actionStr = strings.TrimSpace(parts[4])
		}

		severityStr := "medium"
		if len(parts) > 5 {
			severityStr = strings.TrimSpace(parts[5])
		}

		targets, err := parseTargets(strings.Split(targetStr, ","))
		if err != nil && p.strict {
			return nil, fmt.Errorf("line %d: %w", lineNum, err)
		}

		action, err := parseAction(actionStr)
		if err != nil && p.strict {
			return nil, fmt.Errorf("line %d: %w", lineNum, err)
		}

		severity, err := parseSeverity(severityStr)
		if err != nil && p.strict {
			return nil, fmt.Errorf("line %d: %w", lineNum, err)
		}

		rule, err := NewRule(id, name, pattern, targets, action, severity)
		if err != nil {
			if p.strict {
				return nil, fmt.Errorf("line %d: invalid pattern: %w", lineNum, err)
			}
			continue
		}

		rs.AddRule(rule)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading input: %w", err)
	}

	return rs, nil
}

// ExportJSON exports a rule set to JSON
func ExportJSON(rs *RuleSet, w io.Writer) error {
	config := RuleSetConfig{
		Name:        rs.Name,
		Description: rs.Description,
		Version:     "1.0",
		Rules:       make([]RuleConfig, 0, len(rs.Rules)),
	}

	for _, r := range rs.Rules {
		rc := RuleConfig{
			ID:          r.ID,
			Name:        r.Name,
			Description: r.Description,
			Pattern:     r.RawPattern,
			Targets:     targetsToStrings(r.Targets),
			Action:      r.Action.String(),
			Severity:    r.Severity.String(),
			Score:       r.Score,
			Enabled:     r.Enabled,
			Tags:        r.Tags,
			Message:     r.Message,
			RedirectURL: r.RedirectURL,
		}
		config.Rules = append(config.Rules, rc)
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(&config)
}

// targetsToStrings converts Target values to strings
func targetsToStrings(targets []Target) []string {
	result := make([]string, 0, len(targets))
	for _, t := range targets {
		result = append(result, targetToString(t))
	}
	return result
}

// targetToString converts a Target to string
func targetToString(t Target) string {
	switch t {
	case TargetURL:
		return "url"
	case TargetPath:
		return "path"
	case TargetQuery:
		return "query"
	case TargetBody:
		return "body"
	case TargetHeaders:
		return "headers"
	case TargetCookies:
		return "cookies"
	case TargetMethod:
		return "method"
	case TargetIP:
		return "ip"
	case TargetUserAgent:
		return "useragent"
	case TargetAll:
		return "all"
	default:
		return "unknown"
	}
}
