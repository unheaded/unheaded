// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package manager provides alert inhibition functionality.
package manager

import (
	"regexp"
	"sync"

	"unheaded/pkg/alerting"
)

// Inhibitor manages inhibition rules for alerts.
type Inhibitor struct {
	mu       sync.RWMutex
	rules    map[string]*alerting.InhibitionRule
	compiled map[string]*compiledInhibition
}

// compiledInhibition holds compiled matchers for an inhibition rule.
type compiledInhibition struct {
	sourceMatchers []*compiledMatcher
	targetMatchers []*compiledMatcher
	equalLabels    []string
}

// NewInhibitor creates a new alert inhibitor.
func NewInhibitor() *Inhibitor {
	return &Inhibitor{
		rules:    make(map[string]*alerting.InhibitionRule),
		compiled: make(map[string]*compiledInhibition),
	}
}

// AddRule adds an inhibition rule.
func (i *Inhibitor) AddRule(rule *alerting.InhibitionRule) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	compiled := &compiledInhibition{
		equalLabels: rule.EqualLabels,
	}

	// Compile source matchers
	for _, m := range rule.SourceMatch {
		cm, err := i.compileMatcher(m)
		if err != nil {
			return err
		}
		compiled.sourceMatchers = append(compiled.sourceMatchers, cm)
	}

	// Compile target matchers
	for _, m := range rule.TargetMatch {
		cm, err := i.compileMatcher(m)
		if err != nil {
			return err
		}
		compiled.targetMatchers = append(compiled.targetMatchers, cm)
	}

	i.rules[rule.ID] = rule
	i.compiled[rule.ID] = compiled

	return nil
}

// compileMatcher compiles a label matcher.
func (i *Inhibitor) compileMatcher(m alerting.LabelMatcher) (*compiledMatcher, error) {
	cm := &compiledMatcher{
		name:      m.Name,
		value:     m.Value,
		matchType: m.Type,
	}

	if m.Type == alerting.MatchTypeRegex || m.Type == alerting.MatchTypeNotRegex {
		re, err := regexp.Compile(m.Value)
		if err != nil {
			return nil, err
		}
		cm.regex = re
	}

	return cm, nil
}

// RemoveRule removes an inhibition rule.
func (i *Inhibitor) RemoveRule(ruleID string) {
	i.mu.Lock()
	defer i.mu.Unlock()

	delete(i.rules, ruleID)
	delete(i.compiled, ruleID)
}

// IsInhibited checks if an alert is inhibited by any active alerts.
func (i *Inhibitor) IsInhibited(alert *alerting.Alert, activeAlerts []*alerting.Alert) bool {
	i.mu.RLock()
	defer i.mu.RUnlock()

	for ruleID, rule := range i.rules {
		if !rule.Enabled {
			continue
		}

		compiled := i.compiled[ruleID]

		// Check if the alert matches the target
		if !i.matchesAll(alert, compiled.targetMatchers) {
			continue
		}

		// Check if any active alert matches the source and has equal labels
		for _, source := range activeAlerts {
			if source.Fingerprint == alert.Fingerprint {
				continue // Don't inhibit self
			}

			if source.State != alerting.AlertStateFiring {
				continue
			}

			if i.matchesAll(source, compiled.sourceMatchers) {
				if i.equalLabelsMatch(source, alert, compiled.equalLabels) {
					return true
				}
			}
		}
	}

	return false
}

// matchesAll checks if an alert matches all matchers.
func (i *Inhibitor) matchesAll(alert *alerting.Alert, matchers []*compiledMatcher) bool {
	for _, m := range matchers {
		if !i.matches(alert, m) {
			return false
		}
	}
	return true
}

// matches checks if a single matcher matches the alert.
func (i *Inhibitor) matches(alert *alerting.Alert, m *compiledMatcher) bool {
	value := i.getLabelValue(alert, m.name)

	switch m.matchType {
	case alerting.MatchTypeEqual:
		return value == m.value
	case alerting.MatchTypeNotEqual:
		return value != m.value
	case alerting.MatchTypeRegex:
		if m.regex != nil {
			return m.regex.MatchString(value)
		}
		return false
	case alerting.MatchTypeNotRegex:
		if m.regex != nil {
			return !m.regex.MatchString(value)
		}
		return true
	default:
		return false
	}
}

// getLabelValue gets a label value from an alert.
func (i *Inhibitor) getLabelValue(alert *alerting.Alert, name string) string {
	switch name {
	case "alertname":
		return alert.Name
	case "severity":
		return string(alert.Severity)
	case "state":
		return string(alert.State)
	default:
		return alert.Labels[name]
	}
}

// equalLabelsMatch checks if the source and target have equal values for specified labels.
func (i *Inhibitor) equalLabelsMatch(source, target *alerting.Alert, labels []string) bool {
	for _, label := range labels {
		sourceVal := i.getLabelValue(source, label)
		targetVal := i.getLabelValue(target, label)
		if sourceVal != targetVal {
			return false
		}
	}
	return true
}

// GetInhibitingSources returns all alerts that inhibit the given alert.
func (i *Inhibitor) GetInhibitingSources(alert *alerting.Alert, activeAlerts []*alerting.Alert) []*alerting.Alert {
	i.mu.RLock()
	defer i.mu.RUnlock()

	sources := make([]*alerting.Alert, 0)

	for ruleID, rule := range i.rules {
		if !rule.Enabled {
			continue
		}

		compiled := i.compiled[ruleID]

		if !i.matchesAll(alert, compiled.targetMatchers) {
			continue
		}

		for _, source := range activeAlerts {
			if source.Fingerprint == alert.Fingerprint {
				continue
			}

			if source.State != alerting.AlertStateFiring {
				continue
			}

			if i.matchesAll(source, compiled.sourceMatchers) {
				if i.equalLabelsMatch(source, alert, compiled.equalLabels) {
					sources = append(sources, source)
				}
			}
		}
	}

	return sources
}

// GetRules returns all inhibition rules.
func (i *Inhibitor) GetRules() []*alerting.InhibitionRule {
	i.mu.RLock()
	defer i.mu.RUnlock()

	rules := make([]*alerting.InhibitionRule, 0, len(i.rules))
	for _, rule := range i.rules {
		rules = append(rules, rule)
	}

	return rules
}

// InhibitionBuilder provides a fluent interface for building inhibition rules.
type InhibitionBuilder struct {
	rule *alerting.InhibitionRule
}

// NewInhibitionBuilder creates a new inhibition rule builder.
func NewInhibitionBuilder(id string) *InhibitionBuilder {
	return &InhibitionBuilder{
		rule: &alerting.InhibitionRule{
			ID:          id,
			SourceMatch: make([]alerting.LabelMatcher, 0),
			TargetMatch: make([]alerting.LabelMatcher, 0),
			EqualLabels: make([]string, 0),
			Enabled:     true,
		},
	}
}

// WithSourceMatch adds a source matcher.
func (ib *InhibitionBuilder) WithSourceMatch(name, value string, matchType alerting.MatchType) *InhibitionBuilder {
	ib.rule.SourceMatch = append(ib.rule.SourceMatch, alerting.LabelMatcher{
		Name:  name,
		Value: value,
		Type:  matchType,
	})
	return ib
}

// WithSourceEqual adds an equality source matcher.
func (ib *InhibitionBuilder) WithSourceEqual(name, value string) *InhibitionBuilder {
	return ib.WithSourceMatch(name, value, alerting.MatchTypeEqual)
}

// WithTargetMatch adds a target matcher.
func (ib *InhibitionBuilder) WithTargetMatch(name, value string, matchType alerting.MatchType) *InhibitionBuilder {
	ib.rule.TargetMatch = append(ib.rule.TargetMatch, alerting.LabelMatcher{
		Name:  name,
		Value: value,
		Type:  matchType,
	})
	return ib
}

// WithTargetEqual adds an equality target matcher.
func (ib *InhibitionBuilder) WithTargetEqual(name, value string) *InhibitionBuilder {
	return ib.WithTargetMatch(name, value, alerting.MatchTypeEqual)
}

// WithEqualLabels sets the labels that must be equal.
func (ib *InhibitionBuilder) WithEqualLabels(labels ...string) *InhibitionBuilder {
	ib.rule.EqualLabels = labels
	return ib
}

// Disabled sets the rule as disabled.
func (ib *InhibitionBuilder) Disabled() *InhibitionBuilder {
	ib.rule.Enabled = false
	return ib
}

// Build finalizes and returns the inhibition rule.
func (ib *InhibitionBuilder) Build() *alerting.InhibitionRule {
	return ib.rule
}
