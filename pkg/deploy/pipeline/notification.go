// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package pipeline provides the deployment pipeline for orchestrating deployment stages.
package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"text/template"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// Notification errors.
var (
	ErrNotificationFailed      = errors.New("notification failed")
	ErrChannelNotConfigured    = errors.New("notification channel not configured")
	ErrInvalidNotificationType = errors.New("invalid notification type")
	ErrTemplateError           = errors.New("template error")
)

// NotificationType represents the type of notification.
type NotificationType string

const (
	NotifyDeploymentStarted   NotificationType = "deployment_started"
	NotifyDeploymentCompleted NotificationType = "deployment_completed"
	NotifyDeploymentFailed    NotificationType = "deployment_failed"
	NotifyDeploymentProgress  NotificationType = "deployment_progress"
	NotifyRollbackStarted     NotificationType = "rollback_started"
	NotifyRollbackCompleted   NotificationType = "rollback_completed"
	NotifyRollbackFailed      NotificationType = "rollback_failed"
	NotifyCanaryAnalysis      NotificationType = "canary_analysis"
	NotifyHealthCheckFailed   NotificationType = "health_check_failed"
	NotifyApprovalRequired    NotificationType = "approval_required"
	NotifyHookFailed          NotificationType = "hook_failed"
	NotifyAlert               NotificationType = "alert"
)

// Severity represents notification severity.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
	SeveritySuccess  Severity = "success"
)

// Notification represents a notification to be sent.
type Notification struct {
	// ID is the unique notification identifier
	ID string `json:"id"`

	// Type is the notification type
	Type NotificationType `json:"type"`

	// Severity is the notification severity
	Severity Severity `json:"severity"`

	// Title is the notification title
	Title string `json:"title"`

	// Message is the notification message
	Message string `json:"message"`

	// Service is the service name
	Service string `json:"service,omitempty"`

	// Version is the deployment version
	Version string `json:"version,omitempty"`

	// Environment is the deployment environment
	Environment string `json:"environment,omitempty"`

	// DeploymentID is the deployment identifier
	DeploymentID string `json:"deployment_id,omitempty"`

	// PipelineID is the pipeline identifier
	PipelineID string `json:"pipeline_id,omitempty"`

	// Progress for progress notifications
	Progress int `json:"progress,omitempty"`

	// Duration for completion notifications
	Duration time.Duration `json:"duration,omitempty"`

	// Error for failure notifications
	Error string `json:"error,omitempty"`

	// Metadata contains additional data
	Metadata map[string]interface{} `json:"metadata,omitempty"`

	// Timestamp of the notification
	Timestamp time.Time `json:"timestamp"`

	// Links contains relevant links
	Links []NotificationLink `json:"links,omitempty"`
}

// NotificationLink represents a link in a notification.
type NotificationLink struct {
	// Name is the link name
	Name string `json:"name"`

	// URL is the link URL
	URL string `json:"url"`
}

// ChannelType represents the type of notification channel.
type ChannelType string

const (
	ChannelTypeWebhook   ChannelType = "webhook"
	ChannelTypeSlack     ChannelType = "slack"
	ChannelTypeWotan     ChannelType = "wotan"
	ChannelTypeEmail     ChannelType = "email"
	ChannelTypePagerDuty ChannelType = "pagerduty"
	ChannelTypeOpsGenie  ChannelType = "opsgenie"
	ChannelTypeTeams     ChannelType = "teams"
	ChannelTypeDiscord   ChannelType = "discord"
)

// Channel represents a notification channel.
type Channel struct {
	// Name is the channel name
	Name string `json:"name"`

	// Type is the channel type
	Type ChannelType `json:"type"`

	// Enabled indicates if the channel is enabled
	Enabled bool `json:"enabled"`

	// Target is the channel target (URL, email, topic)
	Target string `json:"target"`

	// Filters define which notifications to send
	Filters *ChannelFilters `json:"filters,omitempty"`

	// Templates for custom message formatting
	Templates *ChannelTemplates `json:"templates,omitempty"`

	// Config contains channel-specific configuration
	Config *ChannelConfig `json:"config,omitempty"`
}

// ChannelFilters defines filtering rules for a channel.
type ChannelFilters struct {
	// Types to include (empty = all)
	Types []NotificationType `json:"types,omitempty"`

	// Severities to include (empty = all)
	Severities []Severity `json:"severities,omitempty"`

	// Services to include (empty = all)
	Services []string `json:"services,omitempty"`

	// Environments to include (empty = all)
	Environments []string `json:"environments,omitempty"`
}

// ChannelTemplates contains message templates for a channel.
type ChannelTemplates struct {
	// Title template
	Title string `json:"title,omitempty"`

	// Message template
	Message string `json:"message,omitempty"`

	// Body template (for complex channels like Slack)
	Body string `json:"body,omitempty"`
}

// ChannelConfig contains channel-specific configuration.
type ChannelConfig struct {
	// Webhook config
	WebhookMethod  string            `json:"webhook_method,omitempty"`
	WebhookHeaders map[string]string `json:"webhook_headers,omitempty"`

	// Slack config
	SlackChannel   string `json:"slack_channel,omitempty"`
	SlackUsername  string `json:"slack_username,omitempty"`
	SlackIconEmoji string `json:"slack_icon_emoji,omitempty"`

	// Wotan config
	WotanTopic string `json:"wotan_topic,omitempty"`

	// Email config
	EmailFrom    string   `json:"email_from,omitempty"`
	EmailTo      []string `json:"email_to,omitempty"`
	EmailSubject string   `json:"email_subject,omitempty"`

	// PagerDuty config
	PagerDutyRoutingKey string `json:"pagerduty_routing_key,omitempty"`

	// OpsGenie config
	OpsGenieApiKey string `json:"opsgenie_api_key,omitempty"`

	// Teams config
	TeamsWebhook string `json:"teams_webhook,omitempty"`

	// Discord config
	DiscordWebhook string `json:"discord_webhook,omitempty"`

	// Timeout for notifications
	Timeout time.Duration `json:"timeout,omitempty"`

	// Retries for failed notifications
	Retries int `json:"retries,omitempty"`
}

// NotificationManager manages notifications for the pipeline.
type NotificationManager struct {
	mu sync.RWMutex

	// channels stores configured channels
	channels map[string]*Channel

	// httpClient for HTTP-based notifications
	httpClient *http.Client

	// wotanPublisher for Wotan notifications
	wotanPublisher WotanPublisher

	// history stores recent notifications
	history []*NotificationRecord

	// maxHistory is max history entries
	maxHistory int

	// templates stores parsed templates
	templates map[string]*template.Template

	// defaultTemplates contains default message templates
	defaultTemplates map[NotificationType]string
}

// NotificationRecord contains a notification history record.
type NotificationRecord struct {
	// Notification that was sent
	Notification *Notification `json:"notification"`

	// Channel that received the notification
	Channel string `json:"channel"`

	// SentAt is when the notification was sent
	SentAt time.Time `json:"sent_at"`

	// Success indicates if sending succeeded
	Success bool `json:"success"`

	// Error contains error message if failed
	Error string `json:"error,omitempty"`

	// Duration is how long sending took
	Duration time.Duration `json:"duration"`
}

// NewNotificationManager creates a new notification manager.
func NewNotificationManager() *NotificationManager {
	return &NotificationManager{
		channels:   make(map[string]*Channel),
		httpClient: &http.Client{Timeout: 30 * time.Second},
		history:    make([]*NotificationRecord, 0),
		maxHistory: 1000,
		templates:  make(map[string]*template.Template),
		defaultTemplates: map[NotificationType]string{
			NotifyDeploymentStarted:   "Deployment started for {{.Service}} v{{.Version}} in {{.Environment}}",
			NotifyDeploymentCompleted: "Deployment completed for {{.Service}} v{{.Version}} in {{.Environment}} ({{.Duration}})",
			NotifyDeploymentFailed:    "Deployment FAILED for {{.Service}} v{{.Version}} in {{.Environment}}: {{.Error}}",
			NotifyDeploymentProgress:  "Deployment progress: {{.Service}} v{{.Version}} - {{.Progress}}%",
			NotifyRollbackStarted:     "Rollback started for {{.Service}} in {{.Environment}}",
			NotifyRollbackCompleted:   "Rollback completed for {{.Service}} in {{.Environment}} ({{.Duration}})",
			NotifyRollbackFailed:      "Rollback FAILED for {{.Service}} in {{.Environment}}: {{.Error}}",
			NotifyCanaryAnalysis:      "Canary analysis for {{.Service}}: {{.Message}}",
			NotifyHealthCheckFailed:   "Health check failed for {{.Service}} in {{.Environment}}",
			NotifyApprovalRequired:    "Approval required for {{.Service}} v{{.Version}} deployment",
			NotifyHookFailed:          "Hook failed for {{.Service}}: {{.Error}}",
			NotifyAlert:               "Alert for {{.Service}}: {{.Message}}",
		},
	}
}

// SetWotanPublisher sets the Wotan publisher.
func (m *NotificationManager) SetWotanPublisher(publisher WotanPublisher) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.wotanPublisher = publisher
}

// AddChannel adds a notification channel.
func (m *NotificationManager) AddChannel(channel *Channel) error {
	if channel == nil || channel.Name == "" {
		return errors.New("invalid channel")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.channels[channel.Name] = channel

	// Parse templates if provided
	if channel.Templates != nil {
		if channel.Templates.Title != "" {
			tmpl, err := template.New(channel.Name + "_title").Parse(channel.Templates.Title)
			if err != nil {
				return fmt.Errorf("%w: %v", ErrTemplateError, err)
			}
			m.templates[channel.Name+"_title"] = tmpl
		}
		if channel.Templates.Message != "" {
			tmpl, err := template.New(channel.Name + "_message").Parse(channel.Templates.Message)
			if err != nil {
				return fmt.Errorf("%w: %v", ErrTemplateError, err)
			}
			m.templates[channel.Name+"_message"] = tmpl
		}
		if channel.Templates.Body != "" {
			tmpl, err := template.New(channel.Name + "_body").Parse(channel.Templates.Body)
			if err != nil {
				return fmt.Errorf("%w: %v", ErrTemplateError, err)
			}
			m.templates[channel.Name+"_body"] = tmpl
		}
	}

	return nil
}

// RemoveChannel removes a notification channel.
func (m *NotificationManager) RemoveChannel(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.channels, name)
	delete(m.templates, name+"_title")
	delete(m.templates, name+"_message")
	delete(m.templates, name+"_body")
}

// GetChannel returns a channel by name.
func (m *NotificationManager) GetChannel(name string) (*Channel, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	channel, exists := m.channels[name]
	if !exists {
		return nil, ErrChannelNotConfigured
	}
	return channel, nil
}

// ListChannels returns all configured channels.
func (m *NotificationManager) ListChannels() []*Channel {
	m.mu.RLock()
	defer m.mu.RUnlock()

	channels := make([]*Channel, 0, len(m.channels))
	for _, ch := range m.channels {
		channels = append(channels, ch)
	}
	return channels
}

// Notify sends a notification to all matching channels.
func (m *NotificationManager) Notify(ctx context.Context, notification *Notification) error {
	if notification == nil {
		return errors.New("notification is nil")
	}

	// Set defaults
	if notification.ID == "" {
		notification.ID = generateNotificationID()
	}
	if notification.Timestamp.IsZero() {
		notification.Timestamp = time.Now()
	}
	if notification.Severity == "" {
		notification.Severity = m.defaultSeverity(notification.Type)
	}
	if notification.Title == "" {
		notification.Title = m.formatDefaultTitle(notification)
	}
	if notification.Message == "" {
		notification.Message = m.formatDefaultMessage(notification)
	}

	m.mu.RLock()
	channels := make([]*Channel, 0)
	for _, ch := range m.channels {
		if ch.Enabled && m.shouldSendToChannel(ch, notification) {
			channels = append(channels, ch)
		}
	}
	m.mu.RUnlock()

	var lastErr error
	for _, channel := range channels {
		if err := m.sendToChannel(ctx, channel, notification); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// NotifyChannel sends a notification to a specific channel.
func (m *NotificationManager) NotifyChannel(ctx context.Context, channelName string, notification *Notification) error {
	m.mu.RLock()
	channel, exists := m.channels[channelName]
	m.mu.RUnlock()

	if !exists {
		return ErrChannelNotConfigured
	}

	if !channel.Enabled {
		return nil
	}

	// Set defaults
	if notification.ID == "" {
		notification.ID = generateNotificationID()
	}
	if notification.Timestamp.IsZero() {
		notification.Timestamp = time.Now()
	}
	if notification.Title == "" {
		notification.Title = m.formatDefaultTitle(notification)
	}
	if notification.Message == "" {
		notification.Message = m.formatDefaultMessage(notification)
	}

	return m.sendToChannel(ctx, channel, notification)
}

// shouldSendToChannel checks if a notification should be sent to a channel.
func (m *NotificationManager) shouldSendToChannel(channel *Channel, notification *Notification) bool {
	if channel.Filters == nil {
		return true
	}

	filters := channel.Filters

	// Check type filter
	if len(filters.Types) > 0 {
		found := false
		for _, t := range filters.Types {
			if t == notification.Type {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check severity filter
	if len(filters.Severities) > 0 {
		found := false
		for _, s := range filters.Severities {
			if s == notification.Severity {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check service filter
	if len(filters.Services) > 0 && notification.Service != "" {
		found := false
		for _, s := range filters.Services {
			if s == notification.Service {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check environment filter
	if len(filters.Environments) > 0 && notification.Environment != "" {
		found := false
		for _, e := range filters.Environments {
			if e == notification.Environment {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// sendToChannel sends a notification to a specific channel.
func (m *NotificationManager) sendToChannel(ctx context.Context, channel *Channel, notification *Notification) error {
	startTime := time.Now()

	// Format message with templates
	formattedNotification := m.formatNotification(channel, notification)

	var err error
	switch channel.Type {
	case ChannelTypeWebhook:
		err = m.sendWebhook(ctx, channel, formattedNotification)
	case ChannelTypeSlack:
		err = m.sendSlack(ctx, channel, formattedNotification)
	case ChannelTypeWotan:
		err = m.sendWotan(ctx, channel, formattedNotification)
	case ChannelTypeTeams:
		err = m.sendTeams(ctx, channel, formattedNotification)
	case ChannelTypeDiscord:
		err = m.sendDiscord(ctx, channel, formattedNotification)
	case ChannelTypePagerDuty:
		err = m.sendPagerDuty(ctx, channel, formattedNotification)
	case ChannelTypeOpsGenie:
		err = m.sendOpsGenie(ctx, channel, formattedNotification)
	default:
		err = m.sendWebhook(ctx, channel, formattedNotification)
	}

	// Record in history
	record := &NotificationRecord{
		Notification: notification,
		Channel:      channel.Name,
		SentAt:       startTime,
		Duration:     time.Since(startTime),
		Success:      err == nil,
	}
	if err != nil {
		record.Error = err.Error()
	}

	m.mu.Lock()
	m.history = append(m.history, record)
	if len(m.history) > m.maxHistory {
		m.history = m.history[len(m.history)-m.maxHistory:]
	}
	m.mu.Unlock()

	return err
}

// formatNotification formats a notification using channel templates.
func (m *NotificationManager) formatNotification(channel *Channel, notification *Notification) *Notification {
	formatted := *notification

	m.mu.RLock()
	defer m.mu.RUnlock()

	// Format title
	if tmpl, ok := m.templates[channel.Name+"_title"]; ok {
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, notification); err == nil {
			formatted.Title = buf.String()
		}
	}

	// Format message
	if tmpl, ok := m.templates[channel.Name+"_message"]; ok {
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, notification); err == nil {
			formatted.Message = buf.String()
		}
	}

	return &formatted
}

// sendWebhook sends a notification via webhook.
func (m *NotificationManager) sendWebhook(ctx context.Context, channel *Channel, notification *Notification) error {
	method := "POST"
	if channel.Config != nil && channel.Config.WebhookMethod != "" {
		method = channel.Config.WebhookMethod
	}

	body, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, channel.Target, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if channel.Config != nil && channel.Config.WebhookHeaders != nil {
		for key, value := range channel.Config.WebhookHeaders {
			req.Header.Set(key, value)
		}
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNotificationFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%w: status %d", ErrNotificationFailed, resp.StatusCode)
	}

	return nil
}

// sendSlack sends a notification to Slack.
func (m *NotificationManager) sendSlack(ctx context.Context, channel *Channel, notification *Notification) error {
	payload := map[string]interface{}{
		"text": notification.Message,
	}

	if channel.Config != nil {
		if channel.Config.SlackChannel != "" {
			payload["channel"] = channel.Config.SlackChannel
		}
		if channel.Config.SlackUsername != "" {
			payload["username"] = channel.Config.SlackUsername
		}
		if channel.Config.SlackIconEmoji != "" {
			payload["icon_emoji"] = channel.Config.SlackIconEmoji
		}
	}

	// Build rich attachment
	attachment := map[string]interface{}{
		"fallback": notification.Message,
		"title":    notification.Title,
		"text":     notification.Message,
		"ts":       notification.Timestamp.Unix(),
	}

	// Set color based on severity
	switch notification.Severity {
	case SeveritySuccess:
		attachment["color"] = "good"
	case SeverityWarning:
		attachment["color"] = "warning"
	case SeverityCritical:
		attachment["color"] = "danger"
	default:
		attachment["color"] = "#439FE0"
	}

	// Add fields
	fields := []map[string]interface{}{}
	if notification.Service != "" {
		fields = append(fields, map[string]interface{}{
			"title": "Service",
			"value": notification.Service,
			"short": true,
		})
	}
	if notification.Version != "" {
		fields = append(fields, map[string]interface{}{
			"title": "Version",
			"value": notification.Version,
			"short": true,
		})
	}
	if notification.Environment != "" {
		fields = append(fields, map[string]interface{}{
			"title": "Environment",
			"value": notification.Environment,
			"short": true,
		})
	}
	if notification.Progress > 0 {
		fields = append(fields, map[string]interface{}{
			"title": "Progress",
			"value": fmt.Sprintf("%d%%", notification.Progress),
			"short": true,
		})
	}
	if notification.Duration > 0 {
		fields = append(fields, map[string]interface{}{
			"title": "Duration",
			"value": notification.Duration.String(),
			"short": true,
		})
	}
	if notification.Error != "" {
		fields = append(fields, map[string]interface{}{
			"title": "Error",
			"value": notification.Error,
			"short": false,
		})
	}

	if len(fields) > 0 {
		attachment["fields"] = fields
	}

	payload["attachments"] = []interface{}{attachment}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", channel.Target, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNotificationFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: Slack returned status %d", ErrNotificationFailed, resp.StatusCode)
	}

	return nil
}

// sendWotan sends a notification to Wotan.
func (m *NotificationManager) sendWotan(ctx context.Context, channel *Channel, notification *Notification) error {
	if m.wotanPublisher == nil {
		return fmt.Errorf("%w: Wotan publisher not configured", ErrChannelNotConfigured)
	}

	topic := "notifications"
	if channel.Config != nil && channel.Config.WotanTopic != "" {
		topic = channel.Config.WotanTopic
	}

	body, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	return m.wotanPublisher.Publish(ctx, topic, body)
}

// sendTeams sends a notification to Microsoft Teams.
func (m *NotificationManager) sendTeams(ctx context.Context, channel *Channel, notification *Notification) error {
	target := channel.Target
	if channel.Config != nil && channel.Config.TeamsWebhook != "" {
		target = channel.Config.TeamsWebhook
	}

	// Build Teams adaptive card
	themeColor := "0076D7"
	switch notification.Severity {
	case SeveritySuccess:
		themeColor = "28A745"
	case SeverityWarning:
		themeColor = "FFC107"
	case SeverityCritical:
		themeColor = "DC3545"
	}

	payload := map[string]interface{}{
		"@type":      "MessageCard",
		"@context":   "http://schema.org/extensions",
		"themeColor": themeColor,
		"summary":    notification.Title,
		"sections": []interface{}{
			map[string]interface{}{
				"activityTitle": notification.Title,
				"facts": []map[string]string{
					{"name": "Service", "value": notification.Service},
					{"name": "Version", "value": notification.Version},
					{"name": "Environment", "value": notification.Environment},
				},
				"text":     notification.Message,
				"markdown": true,
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", target, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNotificationFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: Teams returned status %d", ErrNotificationFailed, resp.StatusCode)
	}

	return nil
}

// sendDiscord sends a notification to Discord.
func (m *NotificationManager) sendDiscord(ctx context.Context, channel *Channel, notification *Notification) error {
	target := channel.Target
	if channel.Config != nil && channel.Config.DiscordWebhook != "" {
		target = channel.Config.DiscordWebhook
	}

	// Build Discord embed
	color := 3447003 // Blue
	switch notification.Severity {
	case SeveritySuccess:
		color = 3066993 // Green
	case SeverityWarning:
		color = 16776960 // Yellow
	case SeverityCritical:
		color = 15158332 // Red
	}

	embed := map[string]interface{}{
		"title":       notification.Title,
		"description": notification.Message,
		"color":       color,
		"timestamp":   notification.Timestamp.Format(time.RFC3339),
		"fields":      []map[string]interface{}{},
	}

	fields, _ := embed["fields"].([]map[string]interface{})
	if notification.Service != "" {
		fields = append(fields, map[string]interface{}{
			"name":   "Service",
			"value":  notification.Service,
			"inline": true,
		})
	}
	if notification.Version != "" {
		fields = append(fields, map[string]interface{}{
			"name":   "Version",
			"value":  notification.Version,
			"inline": true,
		})
	}
	if notification.Environment != "" {
		fields = append(fields, map[string]interface{}{
			"name":   "Environment",
			"value":  notification.Environment,
			"inline": true,
		})
	}
	embed["fields"] = fields

	payload := map[string]interface{}{
		"embeds": []interface{}{embed},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", target, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNotificationFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("%w: Discord returned status %d", ErrNotificationFailed, resp.StatusCode)
	}

	return nil
}

// sendPagerDuty sends a notification to PagerDuty.
func (m *NotificationManager) sendPagerDuty(ctx context.Context, channel *Channel, notification *Notification) error {
	routingKey := ""
	if channel.Config != nil && channel.Config.PagerDutyRoutingKey != "" {
		routingKey = channel.Config.PagerDutyRoutingKey
	}
	if routingKey == "" {
		return fmt.Errorf("%w: PagerDuty routing key not configured", ErrChannelNotConfigured)
	}

	// Determine event action
	eventAction := "trigger"
	if notification.Type == NotifyDeploymentCompleted && notification.Severity == SeveritySuccess {
		eventAction = "resolve"
	}

	// Build PagerDuty payload
	payload := map[string]interface{}{
		"routing_key":  routingKey,
		"event_action": eventAction,
		"payload": map[string]interface{}{
			"summary":   notification.Title,
			"source":    notification.Service,
			"severity":  m.pagerDutySeverity(notification.Severity),
			"timestamp": notification.Timestamp.Format(time.RFC3339),
			"component": notification.Service,
			"group":     notification.Environment,
			"class":     string(notification.Type),
			"custom_details": map[string]interface{}{
				"service":       notification.Service,
				"version":       notification.Version,
				"environment":   notification.Environment,
				"deployment_id": notification.DeploymentID,
				"message":       notification.Message,
			},
		},
		"dedup_key": notification.DeploymentID,
	}

	if len(notification.Links) > 0 {
		links := make([]map[string]string, 0, len(notification.Links))
		for _, link := range notification.Links {
			links = append(links, map[string]string{
				"href": link.URL,
				"text": link.Name,
			})
		}
		payload["links"] = links
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://events.pagerduty.com/v2/enqueue", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNotificationFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%w: PagerDuty returned status %d", ErrNotificationFailed, resp.StatusCode)
	}

	return nil
}

// sendOpsGenie sends a notification to OpsGenie.
func (m *NotificationManager) sendOpsGenie(ctx context.Context, channel *Channel, notification *Notification) error {
	apiKey := ""
	if channel.Config != nil && channel.Config.OpsGenieApiKey != "" {
		apiKey = channel.Config.OpsGenieApiKey
	}
	if apiKey == "" {
		return fmt.Errorf("%w: OpsGenie API key not configured", ErrChannelNotConfigured)
	}

	// Build OpsGenie payload
	payload := map[string]interface{}{
		"message":     notification.Title,
		"alias":       notification.DeploymentID,
		"description": notification.Message,
		"source":      notification.Service,
		"priority":    m.opsGeniePriority(notification.Severity),
		"tags":        []string{notification.Service, notification.Environment, string(notification.Type)},
		"details": map[string]string{
			"service":       notification.Service,
			"version":       notification.Version,
			"environment":   notification.Environment,
			"deployment_id": notification.DeploymentID,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.opsgenie.com/v2/alerts", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "GenieKey "+apiKey)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNotificationFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%w: OpsGenie returned status %d", ErrNotificationFailed, resp.StatusCode)
	}

	return nil
}

// pagerDutySeverity converts severity to PagerDuty severity.
func (m *NotificationManager) pagerDutySeverity(severity Severity) string {
	switch severity {
	case SeverityCritical:
		return "critical"
	case SeverityWarning:
		return "warning"
	default:
		return "info"
	}
}

// opsGeniePriority converts severity to OpsGenie priority.
func (m *NotificationManager) opsGeniePriority(severity Severity) string {
	switch severity {
	case SeverityCritical:
		return "P1"
	case SeverityWarning:
		return "P3"
	default:
		return "P5"
	}
}

// defaultSeverity returns the default severity for a notification type.
func (m *NotificationManager) defaultSeverity(notifyType NotificationType) Severity {
	switch notifyType {
	case NotifyDeploymentCompleted, NotifyRollbackCompleted:
		return SeveritySuccess
	case NotifyDeploymentFailed, NotifyRollbackFailed, NotifyHealthCheckFailed, NotifyHookFailed:
		return SeverityCritical
	case NotifyApprovalRequired, NotifyCanaryAnalysis:
		return SeverityWarning
	default:
		return SeverityInfo
	}
}

// formatDefaultTitle returns a default title for a notification.
func (m *NotificationManager) formatDefaultTitle(notification *Notification) string {
	switch notification.Type {
	case NotifyDeploymentStarted:
		return fmt.Sprintf("Deployment Started: %s", notification.Service)
	case NotifyDeploymentCompleted:
		return fmt.Sprintf("Deployment Completed: %s", notification.Service)
	case NotifyDeploymentFailed:
		return fmt.Sprintf("Deployment Failed: %s", notification.Service)
	case NotifyRollbackStarted:
		return fmt.Sprintf("Rollback Started: %s", notification.Service)
	case NotifyRollbackCompleted:
		return fmt.Sprintf("Rollback Completed: %s", notification.Service)
	case NotifyRollbackFailed:
		return fmt.Sprintf("Rollback Failed: %s", notification.Service)
	case NotifyCanaryAnalysis:
		return fmt.Sprintf("Canary Analysis: %s", notification.Service)
	case NotifyHealthCheckFailed:
		return fmt.Sprintf("Health Check Failed: %s", notification.Service)
	case NotifyApprovalRequired:
		return fmt.Sprintf("Approval Required: %s", notification.Service)
	default:
		return fmt.Sprintf("%s: %s", cases.Title(language.English).String(string(notification.Type)), notification.Service)
	}
}

// formatDefaultMessage returns a default message for a notification.
func (m *NotificationManager) formatDefaultMessage(notification *Notification) string {
	m.mu.RLock()
	tmplStr, exists := m.defaultTemplates[notification.Type]
	m.mu.RUnlock()

	if !exists {
		return notification.Title
	}

	tmpl, err := template.New("message").Parse(tmplStr)
	if err != nil {
		return notification.Title
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, notification); err != nil {
		return notification.Title
	}

	return buf.String()
}

// GetHistory returns notification history.
func (m *NotificationManager) GetHistory(limit int) []*NotificationRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 || limit > len(m.history) {
		limit = len(m.history)
	}

	// Return newest first
	result := make([]*NotificationRecord, limit)
	for i := 0; i < limit; i++ {
		result[i] = m.history[len(m.history)-1-i]
	}

	return result
}

// GetStats returns notification statistics.
func (m *NotificationManager) GetStats() *NotificationStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := &NotificationStats{
		Total:     len(m.history),
		ByChannel: make(map[string]int),
		ByType:    make(map[NotificationType]int),
	}

	for _, record := range m.history {
		stats.ByChannel[record.Channel]++
		stats.ByType[record.Notification.Type]++
		if record.Success {
			stats.Successful++
		} else {
			stats.Failed++
		}
	}

	return stats
}

// NotificationStats contains notification statistics.
type NotificationStats struct {
	Total      int                      `json:"total"`
	Successful int                      `json:"successful"`
	Failed     int                      `json:"failed"`
	ByChannel  map[string]int           `json:"by_channel"`
	ByType     map[NotificationType]int `json:"by_type"`
}

// generateNotificationID generates a unique notification ID.
func generateNotificationID() string {
	return fmt.Sprintf("notif-%d", time.Now().UnixNano())
}

// NotificationBuilder helps build notifications.
type NotificationBuilder struct {
	notification *Notification
}

// NewNotificationBuilder creates a new notification builder.
func NewNotificationBuilder(notifyType NotificationType) *NotificationBuilder {
	return &NotificationBuilder{
		notification: &Notification{
			Type:      notifyType,
			Timestamp: time.Now(),
			Metadata:  make(map[string]interface{}),
		},
	}
}

// WithService sets the service name.
func (b *NotificationBuilder) WithService(service string) *NotificationBuilder {
	b.notification.Service = service
	return b
}

// WithVersion sets the version.
func (b *NotificationBuilder) WithVersion(version string) *NotificationBuilder {
	b.notification.Version = version
	return b
}

// WithEnvironment sets the environment.
func (b *NotificationBuilder) WithEnvironment(env string) *NotificationBuilder {
	b.notification.Environment = env
	return b
}

// WithDeployment sets the deployment ID.
func (b *NotificationBuilder) WithDeployment(id string) *NotificationBuilder {
	b.notification.DeploymentID = id
	return b
}

// WithPipeline sets the pipeline ID.
func (b *NotificationBuilder) WithPipeline(id string) *NotificationBuilder {
	b.notification.PipelineID = id
	return b
}

// WithSeverity sets the severity.
func (b *NotificationBuilder) WithSeverity(severity Severity) *NotificationBuilder {
	b.notification.Severity = severity
	return b
}

// WithTitle sets the title.
func (b *NotificationBuilder) WithTitle(title string) *NotificationBuilder {
	b.notification.Title = title
	return b
}

// WithMessage sets the message.
func (b *NotificationBuilder) WithMessage(message string) *NotificationBuilder {
	b.notification.Message = message
	return b
}

// WithProgress sets the progress.
func (b *NotificationBuilder) WithProgress(progress int) *NotificationBuilder {
	b.notification.Progress = progress
	return b
}

// WithDuration sets the duration.
func (b *NotificationBuilder) WithDuration(duration time.Duration) *NotificationBuilder {
	b.notification.Duration = duration
	return b
}

// WithError sets the error.
func (b *NotificationBuilder) WithError(err string) *NotificationBuilder {
	b.notification.Error = err
	return b
}

// WithMetadata adds metadata.
func (b *NotificationBuilder) WithMetadata(key string, value interface{}) *NotificationBuilder {
	b.notification.Metadata[key] = value
	return b
}

// WithLink adds a link.
func (b *NotificationBuilder) WithLink(name, url string) *NotificationBuilder {
	b.notification.Links = append(b.notification.Links, NotificationLink{Name: name, URL: url})
	return b
}

// Build returns the built notification.
func (b *NotificationBuilder) Build() *Notification {
	return b.notification
}
