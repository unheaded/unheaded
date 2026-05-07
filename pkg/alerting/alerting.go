// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package alerting provides the Kingdom's alarm system for detecting and notifying on issues.
package alerting

import (
	"context"
	"errors"
	"time"
)

// Common errors for the alerting package.
var (
	ErrRuleNotFound      = errors.New("alert rule not found")
	ErrAlertNotFound     = errors.New("alert not found")
	ErrSilenceNotFound   = errors.New("silence not found")
	ErrInvalidRule       = errors.New("invalid alert rule")
	ErrInvalidSilence    = errors.New("invalid silence configuration")
	ErrDuplicateRule     = errors.New("duplicate rule ID")
	ErrEvaluationFailed  = errors.New("rule evaluation failed")
	ErrNotificationError = errors.New("notification delivery failed")
)

// AlertState represents the current state of an alert.
type AlertState string

const (
	AlertStatePending  AlertState = "pending"
	AlertStateFiring   AlertState = "firing"
	AlertStateResolved AlertState = "resolved"
)

// AlertSeverity represents the severity level of an alert.
type AlertSeverity string

const (
	SeverityCritical AlertSeverity = "critical"
	SeverityWarning  AlertSeverity = "warning"
	SeverityInfo     AlertSeverity = "info"
)

// RuleType defines the type of alert rule.
type RuleType string

const (
	RuleTypeThreshold RuleType = "threshold"
	RuleTypeRate      RuleType = "rate"
	RuleTypeAbsence   RuleType = "absence"
)

// ComparisonOperator defines comparison operations for thresholds.
type ComparisonOperator string

const (
	OpGreaterThan      ComparisonOperator = ">"
	OpGreaterThanEqual ComparisonOperator = ">="
	OpLessThan         ComparisonOperator = "<"
	OpLessThanEqual    ComparisonOperator = "<="
	OpEqual            ComparisonOperator = "=="
	OpNotEqual         ComparisonOperator = "!="
)

// Alert represents an active or historical alert instance.
type Alert struct {
	ID           string            `json:"id"`
	RuleID       string            `json:"rule_id"`
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	State        AlertState        `json:"state"`
	Severity     AlertSeverity     `json:"severity"`
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	Value        float64           `json:"value"`
	Threshold    float64           `json:"threshold"`
	StartsAt     time.Time         `json:"starts_at"`
	EndsAt       time.Time         `json:"ends_at,omitempty"`
	FiredAt      time.Time         `json:"fired_at,omitempty"`
	ResolvedAt   time.Time         `json:"resolved_at,omitempty"`
	Acknowledged bool              `json:"acknowledged"`
	AckedBy      string            `json:"acked_by,omitempty"`
	AckedAt      time.Time         `json:"acked_at,omitempty"`
	Fingerprint  string            `json:"fingerprint"`
	GeneratorURL string            `json:"generator_url,omitempty"`
}

// AlertRule defines a condition that triggers alerts.
type AlertRule struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Type        RuleType          `json:"type"`
	Enabled     bool              `json:"enabled"`
	Expression  string            `json:"expression"`
	Severity    AlertSeverity     `json:"severity"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`

	// Threshold rule configuration
	Threshold  float64            `json:"threshold,omitempty"`
	Operator   ComparisonOperator `json:"operator,omitempty"`
	ForPeriod  time.Duration      `json:"for_period,omitempty"`
	MetricName string             `json:"metric_name,omitempty"`

	// Rate rule configuration
	RateWindow   time.Duration `json:"rate_window,omitempty"`
	RateIncrease float64       `json:"rate_increase,omitempty"`

	// Absence rule configuration
	AbsenceWindow time.Duration `json:"absence_window,omitempty"`

	// Evaluation settings
	EvaluationInterval time.Duration `json:"evaluation_interval"`
	NotifyChannels     []string      `json:"notify_channels"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AlertFilter provides filtering criteria for querying alerts.
type AlertFilter struct {
	RuleIDs    []string          `json:"rule_ids,omitempty"`
	States     []AlertState      `json:"states,omitempty"`
	Severities []AlertSeverity   `json:"severities,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
	StartTime  time.Time         `json:"start_time,omitempty"`
	EndTime    time.Time         `json:"end_time,omitempty"`
	Limit      int               `json:"limit,omitempty"`
	Offset     int               `json:"offset,omitempty"`
}

// Silence suppresses notifications for matching alerts.
type Silence struct {
	ID        string         `json:"id"`
	Matchers  []LabelMatcher `json:"matchers"`
	StartsAt  time.Time      `json:"starts_at"`
	EndsAt    time.Time      `json:"ends_at"`
	CreatedBy string         `json:"created_by"`
	Comment   string         `json:"comment"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	Status    SilenceStatus  `json:"status"`
}

// SilenceStatus represents the current status of a silence.
type SilenceStatus string

const (
	SilenceStatusActive  SilenceStatus = "active"
	SilenceStatusPending SilenceStatus = "pending"
	SilenceStatusExpired SilenceStatus = "expired"
)

// LabelMatcher defines a label matching criteria.
type LabelMatcher struct {
	Name  string    `json:"name"`
	Value string    `json:"value"`
	Type  MatchType `json:"type"`
}

// MatchType defines how labels are matched.
type MatchType string

const (
	MatchTypeEqual    MatchType = "="
	MatchTypeNotEqual MatchType = "!="
	MatchTypeRegex    MatchType = "=~"
	MatchTypeNotRegex MatchType = "!~"
)

// InhibitionRule prevents alerts from firing when other alerts are active.
type InhibitionRule struct {
	ID          string         `json:"id"`
	SourceMatch []LabelMatcher `json:"source_match"`
	TargetMatch []LabelMatcher `json:"target_match"`
	EqualLabels []string       `json:"equal_labels"`
	Enabled     bool           `json:"enabled"`
}

// AlertGroup represents a group of related alerts.
type AlertGroup struct {
	Key       string            `json:"key"`
	Labels    map[string]string `json:"labels"`
	Alerts    []*Alert          `json:"alerts"`
	Receiver  string            `json:"receiver"`
	CreatedAt time.Time         `json:"created_at"`
}

// EscalationPolicy defines how alerts are escalated over time.
type EscalationPolicy struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Rules       []EscalationStep `json:"rules"`
	RepeatAfter time.Duration    `json:"repeat_after"`
	Enabled     bool             `json:"enabled"`
}

// EscalationStep defines a single step in an escalation policy.
type EscalationStep struct {
	DelayMinutes int      `json:"delay_minutes"`
	Channels     []string `json:"channels"`
	NotifyUsers  []string `json:"notify_users"`
}

// AlertManager is the main interface for managing alerts.
type AlertManager interface {
	// Rule management
	CreateRule(ctx context.Context, rule *AlertRule) error
	UpdateRule(ctx context.Context, rule *AlertRule) error
	DeleteRule(ctx context.Context, ruleID string) error
	GetRule(ctx context.Context, ruleID string) (*AlertRule, error)
	ListRules(ctx context.Context) ([]*AlertRule, error)

	// Alert operations
	GetAlerts(ctx context.Context, filter *AlertFilter) ([]*Alert, error)
	GetAlert(ctx context.Context, alertID string) (*Alert, error)
	Acknowledge(ctx context.Context, alertID string, user string) error
	Resolve(ctx context.Context, alertID string) error

	// Silence management
	Silence(ctx context.Context, silence *Silence) error
	DeleteSilence(ctx context.Context, silenceID string) error
	GetSilences(ctx context.Context) ([]*Silence, error)

	// Inhibition management
	AddInhibition(ctx context.Context, rule *InhibitionRule) error
	RemoveInhibition(ctx context.Context, ruleID string) error

	// Lifecycle
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// MetricProvider is the interface for fetching metric values.
type MetricProvider interface {
	Query(ctx context.Context, expression string, timestamp time.Time) (float64, error)
	QueryRange(ctx context.Context, expression string, start, end time.Time, step time.Duration) ([]float64, error)
}

// NotificationChannel is the interface for sending alert notifications.
type NotificationChannel interface {
	Name() string
	Send(ctx context.Context, alerts []*Alert) error
	Test(ctx context.Context) error
}

// OnCallProvider is the interface for on-call schedule integration.
type OnCallProvider interface {
	GetCurrentOnCall(ctx context.Context, scheduleID string) ([]string, error)
	GetEscalationContacts(ctx context.Context, policyID string, level int) ([]string, error)
}
