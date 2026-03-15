// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package manager provides the alert manager implementation.
package manager

import (
	"context"
	"fmt"
	"sync"
	"time"

	"unheaded/pkg/alerting"
	"unheaded/pkg/alerting/rules"
)

// Manager implements the AlertManager interface.
type Manager struct {
	mu          sync.RWMutex
	rules       map[string]*alerting.AlertRule
	alerts      map[string]*alerting.Alert
	silences    map[string]*alerting.Silence
	inhibitions map[string]*alerting.InhibitionRule

	evaluator  *rules.Evaluator
	grouper    *Grouper
	silencer   *Silencer
	inhibitor  *Inhibitor
	notifiers  map[string]alerting.NotificationChannel
	escalation map[string]*alerting.EscalationPolicy

	provider alerting.MetricProvider
	oncall   alerting.OnCallProvider

	stopCh     chan struct{}
	evalTicker *time.Ticker
	running    bool
}

// Config holds the manager configuration.
type Config struct {
	EvaluationInterval time.Duration
	ResendInterval     time.Duration
	GroupWait          time.Duration
	GroupInterval      time.Duration
}

// DefaultConfig returns the default manager configuration.
func DefaultConfig() Config {
	return Config{
		EvaluationInterval: 1 * time.Minute,
		ResendInterval:     4 * time.Hour,
		GroupWait:          30 * time.Second,
		GroupInterval:      5 * time.Minute,
	}
}

// NewManager creates a new alert manager.
func NewManager(provider alerting.MetricProvider, config Config) *Manager {
	evaluator := rules.NewEvaluator(provider)

	return &Manager{
		rules:       make(map[string]*alerting.AlertRule),
		alerts:      make(map[string]*alerting.Alert),
		silences:    make(map[string]*alerting.Silence),
		inhibitions: make(map[string]*alerting.InhibitionRule),
		evaluator:   evaluator,
		grouper:     NewGrouper(config.GroupWait, config.GroupInterval),
		silencer:    NewSilencer(),
		inhibitor:   NewInhibitor(),
		notifiers:   make(map[string]alerting.NotificationChannel),
		escalation:  make(map[string]*alerting.EscalationPolicy),
		provider:    provider,
		stopCh:      make(chan struct{}),
	}
}

// RegisterNotifier registers a notification channel.
func (m *Manager) RegisterNotifier(channel alerting.NotificationChannel) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifiers[channel.Name()] = channel
}

// SetOnCallProvider sets the on-call integration provider.
func (m *Manager) SetOnCallProvider(provider alerting.OnCallProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.oncall = provider
}

// CreateRule creates a new alert rule.
func (m *Manager) CreateRule(ctx context.Context, rule *alerting.AlertRule) error {
	if err := rules.ValidateRule(rule); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.rules[rule.ID]; exists {
		return alerting.ErrDuplicateRule
	}

	rule.CreatedAt = time.Now()
	rule.UpdatedAt = time.Now()
	m.rules[rule.ID] = rule

	return nil
}

// UpdateRule updates an existing alert rule.
func (m *Manager) UpdateRule(ctx context.Context, rule *alerting.AlertRule) error {
	if err := rules.ValidateRule(rule); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.rules[rule.ID]; !exists {
		return alerting.ErrRuleNotFound
	}

	rule.UpdatedAt = time.Now()
	m.rules[rule.ID] = rule
	m.evaluator.ResetState(rule.ID)

	return nil
}

// DeleteRule deletes an alert rule.
func (m *Manager) DeleteRule(ctx context.Context, ruleID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.rules[ruleID]; !exists {
		return alerting.ErrRuleNotFound
	}

	delete(m.rules, ruleID)
	m.evaluator.ResetState(ruleID)

	// Clean up associated alerts
	for id, alert := range m.alerts {
		if alert.RuleID == ruleID {
			delete(m.alerts, id)
		}
	}

	return nil
}

// GetRule retrieves a rule by ID.
func (m *Manager) GetRule(ctx context.Context, ruleID string) (*alerting.AlertRule, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rule, exists := m.rules[ruleID]
	if !exists {
		return nil, alerting.ErrRuleNotFound
	}

	return rule, nil
}

// ListRules returns all alert rules.
func (m *Manager) ListRules(ctx context.Context) ([]*alerting.AlertRule, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ruleList := make([]*alerting.AlertRule, 0, len(m.rules))
	for _, rule := range m.rules {
		ruleList = append(ruleList, rule)
	}

	return ruleList, nil
}

// GetAlerts returns alerts matching the filter.
func (m *Manager) GetAlerts(ctx context.Context, filter *alerting.AlertFilter) ([]*alerting.Alert, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	alerts := make([]*alerting.Alert, 0)

	for _, alert := range m.alerts {
		if matchesFilter(alert, filter) {
			alerts = append(alerts, alert)
		}
	}

	// Apply pagination
	if filter != nil {
		if filter.Offset > 0 && filter.Offset < len(alerts) {
			alerts = alerts[filter.Offset:]
		}
		if filter.Limit > 0 && filter.Limit < len(alerts) {
			alerts = alerts[:filter.Limit]
		}
	}

	return alerts, nil
}

// GetAlert retrieves a single alert by ID.
func (m *Manager) GetAlert(ctx context.Context, alertID string) (*alerting.Alert, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	alert, exists := m.alerts[alertID]
	if !exists {
		return nil, alerting.ErrAlertNotFound
	}

	return alert, nil
}

// Acknowledge acknowledges an alert.
func (m *Manager) Acknowledge(ctx context.Context, alertID string, user string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	alert, exists := m.alerts[alertID]
	if !exists {
		return alerting.ErrAlertNotFound
	}

	alert.Acknowledged = true
	alert.AckedBy = user
	alert.AckedAt = time.Now()

	return nil
}

// Resolve manually resolves an alert.
func (m *Manager) Resolve(ctx context.Context, alertID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	alert, exists := m.alerts[alertID]
	if !exists {
		return alerting.ErrAlertNotFound
	}

	alert.State = alerting.AlertStateResolved
	alert.ResolvedAt = time.Now()
	alert.EndsAt = time.Now()

	return nil
}

// Silence creates a new silence.
func (m *Manager) Silence(ctx context.Context, silence *alerting.Silence) error {
	if err := validateSilence(silence); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	silence.CreatedAt = time.Now()
	silence.UpdatedAt = time.Now()
	silence.Status = alerting.SilenceStatusActive

	if silence.StartsAt.After(time.Now()) {
		silence.Status = alerting.SilenceStatusPending
	}

	m.silences[silence.ID] = silence
	m.silencer.AddSilence(silence)

	return nil
}

// DeleteSilence removes a silence.
func (m *Manager) DeleteSilence(ctx context.Context, silenceID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.silences[silenceID]; !exists {
		return alerting.ErrSilenceNotFound
	}

	delete(m.silences, silenceID)
	m.silencer.RemoveSilence(silenceID)

	return nil
}

// GetSilences returns all silences.
func (m *Manager) GetSilences(ctx context.Context) ([]*alerting.Silence, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	silenceList := make([]*alerting.Silence, 0, len(m.silences))
	for _, silence := range m.silences {
		silenceList = append(silenceList, silence)
	}

	return silenceList, nil
}

// AddInhibition adds an inhibition rule.
func (m *Manager) AddInhibition(ctx context.Context, rule *alerting.InhibitionRule) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.inhibitions[rule.ID] = rule
	m.inhibitor.AddRule(rule)

	return nil
}

// RemoveInhibition removes an inhibition rule.
func (m *Manager) RemoveInhibition(ctx context.Context, ruleID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.inhibitions[ruleID]; !exists {
		return fmt.Errorf("inhibition rule not found: %s", ruleID)
	}

	delete(m.inhibitions, ruleID)
	m.inhibitor.RemoveRule(ruleID)

	return nil
}

// AddEscalationPolicy adds an escalation policy.
func (m *Manager) AddEscalationPolicy(ctx context.Context, policy *alerting.EscalationPolicy) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.escalation[policy.ID] = policy
	return nil
}

// Start begins the alert evaluation loop.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return fmt.Errorf("manager already running")
	}
	m.running = true
	m.stopCh = make(chan struct{})
	m.evalTicker = time.NewTicker(1 * time.Minute)
	m.mu.Unlock()

	go m.evaluationLoop(ctx)
	go m.silenceCleanupLoop(ctx)

	return nil
}

// Stop stops the alert manager.
func (m *Manager) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return nil
	}

	close(m.stopCh)
	if m.evalTicker != nil {
		m.evalTicker.Stop()
	}
	m.running = false

	return nil
}

// evaluationLoop runs the periodic rule evaluation.
func (m *Manager) evaluationLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-m.evalTicker.C:
			m.evaluateRules(ctx)
		}
	}
}

// evaluateRules evaluates all enabled rules.
func (m *Manager) evaluateRules(ctx context.Context) {
	m.mu.RLock()
	ruleList := make([]*alerting.AlertRule, 0, len(m.rules))
	for _, rule := range m.rules {
		if rule.Enabled {
			ruleList = append(ruleList, rule)
		}
	}
	m.mu.RUnlock()

	results := m.evaluator.EvaluateAll(ctx, ruleList)

	for _, result := range results {
		if result.Error != nil {
			continue
		}
		m.processEvaluationResult(ctx, result)
	}
}

// processEvaluationResult processes a single evaluation result.
func (m *Manager) processEvaluationResult(ctx context.Context, result *rules.EvaluationResult) {
	m.mu.Lock()
	defer m.mu.Unlock()

	rule, exists := m.rules[result.RuleID]
	if !exists {
		return
	}

	fingerprint := rules.GenerateAlertFingerprint(result.RuleID, result.Labels)

	// Find existing alert
	var existingAlert *alerting.Alert
	for _, alert := range m.alerts {
		if alert.Fingerprint == fingerprint {
			existingAlert = alert
			break
		}
	}

	if result.Firing {
		if existingAlert == nil {
			// Create new alert
			alert := rules.CreateAlertFromRule(rule, result.Value, result.Labels)
			alert.State = alerting.AlertStateFiring
			alert.FiredAt = time.Now()
			m.alerts[alert.ID] = alert

			// Check if silenced or inhibited
			if !m.silencer.IsSilenced(alert) && !m.inhibitor.IsInhibited(alert, m.getActiveAlerts()) {
				m.notify(ctx, []*alerting.Alert{alert})
			}
		} else if existingAlert.State == alerting.AlertStateResolved {
			// Re-fire resolved alert
			existingAlert.State = alerting.AlertStateFiring
			existingAlert.FiredAt = time.Now()
			existingAlert.ResolvedAt = time.Time{}
			existingAlert.Value = result.Value
		}
	} else {
		if existingAlert != nil && existingAlert.State == alerting.AlertStateFiring {
			existingAlert.State = alerting.AlertStateResolved
			existingAlert.ResolvedAt = time.Now()
			existingAlert.EndsAt = time.Now()

			m.notify(ctx, []*alerting.Alert{existingAlert})
		}
	}
}

// getActiveAlerts returns all currently firing alerts.
func (m *Manager) getActiveAlerts() []*alerting.Alert {
	alerts := make([]*alerting.Alert, 0)
	for _, alert := range m.alerts {
		if alert.State == alerting.AlertStateFiring {
			alerts = append(alerts, alert)
		}
	}
	return alerts
}

// notify sends notifications for alerts.
func (m *Manager) notify(ctx context.Context, alerts []*alerting.Alert) {
	if len(alerts) == 0 {
		return
	}

	// Group alerts
	groups := m.grouper.Group(alerts)

	for _, group := range groups {
		// Determine channels from alert rules
		channels := m.getChannelsForAlerts(group.Alerts)

		for _, channelName := range channels {
			if notifier, exists := m.notifiers[channelName]; exists {
				go func(n alerting.NotificationChannel, a []*alerting.Alert) {
					_ = n.Send(ctx, a)
				}(notifier, group.Alerts)
			}
		}
	}
}

// getChannelsForAlerts determines notification channels for alerts.
func (m *Manager) getChannelsForAlerts(alerts []*alerting.Alert) []string {
	channelSet := make(map[string]bool)

	for _, alert := range alerts {
		if rule, exists := m.rules[alert.RuleID]; exists {
			for _, ch := range rule.NotifyChannels {
				channelSet[ch] = true
			}
		}
	}

	channels := make([]string, 0, len(channelSet))
	for ch := range channelSet {
		channels = append(channels, ch)
	}

	return channels
}

// silenceCleanupLoop periodically cleans up expired silences.
func (m *Manager) silenceCleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.cleanupExpiredSilences()
		}
	}
}

// cleanupExpiredSilences removes expired silences.
func (m *Manager) cleanupExpiredSilences() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for id, silence := range m.silences {
		if silence.EndsAt.Before(now) {
			silence.Status = alerting.SilenceStatusExpired
			m.silencer.RemoveSilence(id)
		} else if silence.StartsAt.Before(now) && silence.Status == alerting.SilenceStatusPending {
			silence.Status = alerting.SilenceStatusActive
		}
	}
}

// matchesFilter checks if an alert matches the given filter.
func matchesFilter(alert *alerting.Alert, filter *alerting.AlertFilter) bool {
	if filter == nil {
		return true
	}

	if len(filter.RuleIDs) > 0 {
		found := false
		for _, id := range filter.RuleIDs {
			if alert.RuleID == id {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if len(filter.States) > 0 {
		found := false
		for _, state := range filter.States {
			if alert.State == state {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if len(filter.Severities) > 0 {
		found := false
		for _, sev := range filter.Severities {
			if alert.Severity == sev {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if len(filter.Labels) > 0 {
		for k, v := range filter.Labels {
			if alert.Labels[k] != v {
				return false
			}
		}
	}

	if !filter.StartTime.IsZero() && alert.StartsAt.Before(filter.StartTime) {
		return false
	}

	if !filter.EndTime.IsZero() && alert.StartsAt.After(filter.EndTime) {
		return false
	}

	return true
}

// validateSilence validates a silence configuration.
func validateSilence(silence *alerting.Silence) error {
	if silence.ID == "" {
		return fmt.Errorf("%w: silence ID is required", alerting.ErrInvalidSilence)
	}
	if len(silence.Matchers) == 0 {
		return fmt.Errorf("%w: at least one matcher is required", alerting.ErrInvalidSilence)
	}
	if silence.EndsAt.Before(silence.StartsAt) {
		return fmt.Errorf("%w: end time must be after start time", alerting.ErrInvalidSilence)
	}
	return nil
}
