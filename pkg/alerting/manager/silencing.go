// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package manager provides alert silencing functionality.
package manager

import (
	"regexp"
	"sync"
	"time"

	"unheaded/pkg/alerting"
)

// Silencer manages silence rules for alerts.
type Silencer struct {
	mu       sync.RWMutex
	silences map[string]*alerting.Silence
	matchers map[string][]*compiledMatcher
}

// compiledMatcher holds a pre-compiled regex matcher.
type compiledMatcher struct {
	name    string
	value   string
	regex   *regexp.Regexp
	matchType alerting.MatchType
}

// NewSilencer creates a new alert silencer.
func NewSilencer() *Silencer {
	return &Silencer{
		silences: make(map[string]*alerting.Silence),
		matchers: make(map[string][]*compiledMatcher),
	}
}

// AddSilence adds a silence rule.
func (s *Silencer) AddSilence(silence *alerting.Silence) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Compile matchers
	compiled := make([]*compiledMatcher, 0, len(silence.Matchers))
	for _, m := range silence.Matchers {
		cm := &compiledMatcher{
			name:      m.Name,
			value:     m.Value,
			matchType: m.Type,
		}

		if m.Type == alerting.MatchTypeRegex || m.Type == alerting.MatchTypeNotRegex {
			re, err := regexp.Compile(m.Value)
			if err != nil {
				return err
			}
			cm.regex = re
		}

		compiled = append(compiled, cm)
	}

	s.silences[silence.ID] = silence
	s.matchers[silence.ID] = compiled

	return nil
}

// RemoveSilence removes a silence rule.
func (s *Silencer) RemoveSilence(silenceID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.silences, silenceID)
	delete(s.matchers, silenceID)
}

// IsSilenced checks if an alert is silenced.
func (s *Silencer) IsSilenced(alert *alerting.Alert) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()

	for silenceID, silence := range s.silences {
		// Check if silence is active
		if silence.Status != alerting.SilenceStatusActive {
			continue
		}
		if now.Before(silence.StartsAt) || now.After(silence.EndsAt) {
			continue
		}

		// Check if all matchers match
		matchers := s.matchers[silenceID]
		if s.matchesAll(alert, matchers) {
			return true
		}
	}

	return false
}

// matchesAll checks if all matchers match the alert.
func (s *Silencer) matchesAll(alert *alerting.Alert, matchers []*compiledMatcher) bool {
	for _, m := range matchers {
		if !s.matches(alert, m) {
			return false
		}
	}
	return true
}

// matches checks if a single matcher matches the alert.
func (s *Silencer) matches(alert *alerting.Alert, m *compiledMatcher) bool {
	value := s.getLabelValue(alert, m.name)

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
func (s *Silencer) getLabelValue(alert *alerting.Alert, name string) string {
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

// GetMatchingSilences returns all silences that match an alert.
func (s *Silencer) GetMatchingSilences(alert *alerting.Alert) []*alerting.Silence {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	matching := make([]*alerting.Silence, 0)

	for silenceID, silence := range s.silences {
		if silence.Status != alerting.SilenceStatusActive {
			continue
		}
		if now.Before(silence.StartsAt) || now.After(silence.EndsAt) {
			continue
		}

		matchers := s.matchers[silenceID]
		if s.matchesAll(alert, matchers) {
			matching = append(matching, silence)
		}
	}

	return matching
}

// GetActiveSilences returns all currently active silences.
func (s *Silencer) GetActiveSilences() []*alerting.Silence {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	active := make([]*alerting.Silence, 0)

	for _, silence := range s.silences {
		if silence.Status == alerting.SilenceStatusActive &&
			now.After(silence.StartsAt) && now.Before(silence.EndsAt) {
			active = append(active, silence)
		}
	}

	return active
}

// UpdateSilenceStatus updates the status of silences based on current time.
func (s *Silencer) UpdateSilenceStatus() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	for _, silence := range s.silences {
		if now.After(silence.EndsAt) {
			silence.Status = alerting.SilenceStatusExpired
		} else if now.After(silence.StartsAt) && silence.Status == alerting.SilenceStatusPending {
			silence.Status = alerting.SilenceStatusActive
		}
	}
}

// SilenceBuilder provides a fluent interface for building silences.
type SilenceBuilder struct {
	silence *alerting.Silence
}

// NewSilenceBuilder creates a new silence builder.
func NewSilenceBuilder(id string) *SilenceBuilder {
	return &SilenceBuilder{
		silence: &alerting.Silence{
			ID:       id,
			Matchers: make([]alerting.LabelMatcher, 0),
			StartsAt: time.Now(),
			EndsAt:   time.Now().Add(2 * time.Hour),
		},
	}
}

// WithMatcher adds a matcher to the silence.
func (sb *SilenceBuilder) WithMatcher(name, value string, matchType alerting.MatchType) *SilenceBuilder {
	sb.silence.Matchers = append(sb.silence.Matchers, alerting.LabelMatcher{
		Name:  name,
		Value: value,
		Type:  matchType,
	})
	return sb
}

// WithEqualMatcher adds an equality matcher.
func (sb *SilenceBuilder) WithEqualMatcher(name, value string) *SilenceBuilder {
	return sb.WithMatcher(name, value, alerting.MatchTypeEqual)
}

// WithRegexMatcher adds a regex matcher.
func (sb *SilenceBuilder) WithRegexMatcher(name, pattern string) *SilenceBuilder {
	return sb.WithMatcher(name, pattern, alerting.MatchTypeRegex)
}

// WithDuration sets the silence duration from now.
func (sb *SilenceBuilder) WithDuration(d time.Duration) *SilenceBuilder {
	sb.silence.StartsAt = time.Now()
	sb.silence.EndsAt = time.Now().Add(d)
	return sb
}

// WithTimeRange sets the exact start and end times.
func (sb *SilenceBuilder) WithTimeRange(start, end time.Time) *SilenceBuilder {
	sb.silence.StartsAt = start
	sb.silence.EndsAt = end
	return sb
}

// WithCreator sets the silence creator.
func (sb *SilenceBuilder) WithCreator(creator string) *SilenceBuilder {
	sb.silence.CreatedBy = creator
	return sb
}

// WithComment sets the silence comment.
func (sb *SilenceBuilder) WithComment(comment string) *SilenceBuilder {
	sb.silence.Comment = comment
	return sb
}

// Build finalizes and returns the silence.
func (sb *SilenceBuilder) Build() *alerting.Silence {
	sb.silence.CreatedAt = time.Now()
	sb.silence.UpdatedAt = time.Now()

	if time.Now().Before(sb.silence.StartsAt) {
		sb.silence.Status = alerting.SilenceStatusPending
	} else {
		sb.silence.Status = alerting.SilenceStatusActive
	}

	return sb.silence
}
