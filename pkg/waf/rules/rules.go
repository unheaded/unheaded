// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package rules provides rule definitions for the WAF
package rules

import (
	"regexp"
	"sync"
)

// Action represents what the WAF should do when a rule matches
type Action int

const (
	ActionAllow Action = iota
	ActionBlock
	ActionLog
	ActionRedirect
	ActionChallenge
)

// String returns the string representation of an action
func (a Action) String() string {
	switch a {
	case ActionAllow:
		return "allow"
	case ActionBlock:
		return "block"
	case ActionLog:
		return "log"
	case ActionRedirect:
		return "redirect"
	case ActionChallenge:
		return "challenge"
	default:
		return "unknown"
	}
}

// Severity represents the severity level of a rule
type Severity int

const (
	SeverityInfo Severity = iota
	SeverityLow
	SeverityMedium
	SeverityHigh
	SeverityCritical
)

// String returns the string representation of a severity
func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "info"
	case SeverityLow:
		return "low"
	case SeverityMedium:
		return "medium"
	case SeverityHigh:
		return "high"
	case SeverityCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// Target represents what part of the request to inspect
type Target int

const (
	TargetURL Target = iota
	TargetPath
	TargetQuery
	TargetBody
	TargetHeaders
	TargetCookies
	TargetMethod
	TargetIP
	TargetUserAgent
	TargetAll
)

// Rule represents a WAF rule
type Rule struct {
	ID          string
	Name        string
	Description string
	Pattern     *regexp.Regexp
	RawPattern  string
	Targets     []Target
	Action      Action
	Severity    Severity
	Score       int
	Enabled     bool
	Tags        []string
	Message     string
	RedirectURL string // For redirect actions
	mu          sync.RWMutex
}

// NewRule creates a new rule with the given parameters
func NewRule(id, name, pattern string, targets []Target, action Action, severity Severity) (*Rule, error) {
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return nil, err
	}

	return &Rule{
		ID:         id,
		Name:       name,
		Pattern:    re,
		RawPattern: pattern,
		Targets:    targets,
		Action:     action,
		Severity:   severity,
		Enabled:    true,
		Score:      severityToScore(severity),
	}, nil
}

// severityToScore converts severity to anomaly score
func severityToScore(s Severity) int {
	switch s {
	case SeverityInfo:
		return 1
	case SeverityLow:
		return 2
	case SeverityMedium:
		return 5
	case SeverityHigh:
		return 10
	case SeverityCritical:
		return 20
	default:
		return 0
	}
}

// Enable enables the rule
func (r *Rule) Enable() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Enabled = true
}

// Disable disables the rule
func (r *Rule) Disable() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Enabled = false
}

// IsEnabled returns whether the rule is enabled
func (r *Rule) IsEnabled() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Enabled
}

// Match checks if the input matches the rule pattern
func (r *Rule) Match(input string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if !r.Enabled || r.Pattern == nil {
		return false
	}
	return r.Pattern.MatchString(input)
}

// FindMatches returns all matches in the input
func (r *Rule) FindMatches(input string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if !r.Enabled || r.Pattern == nil {
		return nil
	}
	return r.Pattern.FindAllString(input, -1)
}

// Clone creates a copy of the rule
func (r *Rule) Clone() *Rule {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tags := make([]string, len(r.Tags))
	copy(tags, r.Tags)

	targets := make([]Target, len(r.Targets))
	copy(targets, r.Targets)

	return &Rule{
		ID:          r.ID,
		Name:        r.Name,
		Description: r.Description,
		Pattern:     r.Pattern,
		RawPattern:  r.RawPattern,
		Targets:     targets,
		Action:      r.Action,
		Severity:    r.Severity,
		Score:       r.Score,
		Enabled:     r.Enabled,
		Tags:        tags,
		Message:     r.Message,
		RedirectURL: r.RedirectURL,
	}
}

// RuleSet represents a collection of rules
type RuleSet struct {
	Name        string
	Description string
	Rules       []*Rule
	mu          sync.RWMutex
}

// NewRuleSet creates a new rule set
func NewRuleSet(name, description string) *RuleSet {
	return &RuleSet{
		Name:        name,
		Description: description,
		Rules:       make([]*Rule, 0),
	}
}

// AddRule adds a rule to the set
func (rs *RuleSet) AddRule(rule *Rule) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.Rules = append(rs.Rules, rule)
}

// RemoveRule removes a rule by ID
func (rs *RuleSet) RemoveRule(id string) bool {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	for i, r := range rs.Rules {
		if r.ID == id {
			rs.Rules = append(rs.Rules[:i], rs.Rules[i+1:]...)
			return true
		}
	}
	return false
}

// GetRule gets a rule by ID
func (rs *RuleSet) GetRule(id string) *Rule {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	for _, r := range rs.Rules {
		if r.ID == id {
			return r
		}
	}
	return nil
}

// ListRules returns all rules in the set
func (rs *RuleSet) ListRules() []*Rule {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	rules := make([]*Rule, len(rs.Rules))
	copy(rules, rs.Rules)
	return rules
}
