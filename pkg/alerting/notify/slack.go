// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package notify provides Slack notification channel implementation.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"unheaded/pkg/alerting"
)

// SlackConfig holds Slack channel configuration.
type SlackConfig struct {
	ChannelConfig
	WebhookURL  string `json:"webhook_url"`
	Channel     string `json:"channel"`
	Username    string `json:"username"`
	IconEmoji   string `json:"icon_emoji"`
	IconURL     string `json:"icon_url"`
	LinkNames   bool   `json:"link_names"`
	Timeout     time.Duration `json:"timeout"`
}

// SlackMessage represents a Slack message payload.
type SlackMessage struct {
	Channel     string            `json:"channel,omitempty"`
	Username    string            `json:"username,omitempty"`
	IconEmoji   string            `json:"icon_emoji,omitempty"`
	IconURL     string            `json:"icon_url,omitempty"`
	LinkNames   bool              `json:"link_names,omitempty"`
	Text        string            `json:"text,omitempty"`
	Attachments []SlackAttachment `json:"attachments,omitempty"`
}

// SlackAttachment represents a Slack message attachment.
type SlackAttachment struct {
	Color      string       `json:"color,omitempty"`
	Title      string       `json:"title,omitempty"`
	TitleLink  string       `json:"title_link,omitempty"`
	Text       string       `json:"text,omitempty"`
	Fallback   string       `json:"fallback,omitempty"`
	Fields     []SlackField `json:"fields,omitempty"`
	Footer     string       `json:"footer,omitempty"`
	FooterIcon string       `json:"footer_icon,omitempty"`
	Timestamp  int64        `json:"ts,omitempty"`
	MrkdwnIn   []string     `json:"mrkdwn_in,omitempty"`
}

// SlackField represents a field in a Slack attachment.
type SlackField struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

// SlackChannel implements the NotificationChannel interface for Slack.
type SlackChannel struct {
	config SlackConfig
	client *http.Client
}

// NewSlackChannel creates a new Slack notification channel.
func NewSlackChannel(config SlackConfig) *SlackChannel {
	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}
	if config.Username == "" {
		config.Username = "Unheaded Alerting"
	}
	if config.IconEmoji == "" {
		config.IconEmoji = ":warning:"
	}

	return &SlackChannel{
		config: config,
		client: &http.Client{Timeout: config.Timeout},
	}
}

// Name returns the channel name.
func (s *SlackChannel) Name() string {
	return s.config.Name
}

// Send sends alert notifications to Slack.
func (s *SlackChannel) Send(ctx context.Context, alerts []*alerting.Alert) error {
	if len(alerts) == 0 {
		return nil
	}

	msg := s.buildMessage(alerts)
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("%w: failed to marshal message: %v", alerting.ErrNotificationError, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.config.WebhookURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("%w: failed to create request: %v", alerting.ErrNotificationError, err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: request failed: %v", alerting.ErrNotificationError, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("%w: Slack returned status %d: %s", alerting.ErrNotificationError, resp.StatusCode, string(body))
	}

	return nil
}

// buildMessage builds the Slack message from alerts.
func (s *SlackChannel) buildMessage(alerts []*alerting.Alert) SlackMessage {
	msg := SlackMessage{
		Channel:   s.config.Channel,
		Username:  s.config.Username,
		IconEmoji: s.config.IconEmoji,
		IconURL:   s.config.IconURL,
		LinkNames: s.config.LinkNames,
	}

	// Count firing and resolved
	firingCount := 0
	resolvedCount := 0
	for _, alert := range alerts {
		if alert.State == alerting.AlertStateFiring {
			firingCount++
		} else if alert.State == alerting.AlertStateResolved {
			resolvedCount++
		}
	}

	// Build summary text
	if firingCount > 0 && resolvedCount > 0 {
		msg.Text = fmt.Sprintf("*%d firing, %d resolved*", firingCount, resolvedCount)
	} else if firingCount > 0 {
		msg.Text = fmt.Sprintf("*%d alert(s) firing*", firingCount)
	} else {
		msg.Text = fmt.Sprintf("*%d alert(s) resolved*", resolvedCount)
	}

	// Build attachments for each alert
	attachments := make([]SlackAttachment, 0, len(alerts))
	for _, alert := range alerts {
		attachments = append(attachments, s.buildAttachment(alert))
	}
	msg.Attachments = attachments

	return msg
}

// buildAttachment builds a Slack attachment for an alert.
func (s *SlackChannel) buildAttachment(alert *alerting.Alert) SlackAttachment {
	color := s.getColor(alert)
	status := string(alert.State)

	attachment := SlackAttachment{
		Color:     color,
		Title:     fmt.Sprintf("[%s] %s", status, alert.Name),
		TitleLink: alert.GeneratorURL,
		Fallback:  fmt.Sprintf("[%s] %s: %s", status, alert.Name, alert.Description),
		MrkdwnIn:  []string{"text", "fields"},
		Timestamp: alert.StartsAt.Unix(),
	}

	if alert.Description != "" {
		attachment.Text = alert.Description
	}

	fields := []SlackField{
		{Title: "Severity", Value: string(alert.Severity), Short: true},
		{Title: "Status", Value: status, Short: true},
		{Title: "Value", Value: fmt.Sprintf("%.2f (threshold: %.2f)", alert.Value, alert.Threshold), Short: true},
	}

	// Add labels as fields
	labelStr := ""
	for k, v := range alert.Labels {
		if labelStr != "" {
			labelStr += ", "
		}
		labelStr += fmt.Sprintf("`%s=%s`", k, v)
	}
	if labelStr != "" {
		fields = append(fields, SlackField{Title: "Labels", Value: labelStr, Short: false})
	}

	// Add timing info
	if alert.State == alerting.AlertStateResolved && !alert.ResolvedAt.IsZero() {
		duration := alert.ResolvedAt.Sub(alert.StartsAt)
		fields = append(fields, SlackField{
			Title: "Duration",
			Value: duration.String(),
			Short: true,
		})
	}

	attachment.Fields = fields
	attachment.Footer = "Unheaded Kingdom Alerting"

	return attachment
}

// getColor returns the appropriate color for an alert.
func (s *SlackChannel) getColor(alert *alerting.Alert) string {
	if alert.State == alerting.AlertStateResolved {
		return "good" // Green
	}

	switch alert.Severity {
	case alerting.SeverityCritical:
		return "danger" // Red
	case alerting.SeverityWarning:
		return "warning" // Yellow/Orange
	default:
		return "#439FE0" // Blue
	}
}

// Test tests the Slack connection.
func (s *SlackChannel) Test(ctx context.Context) error {
	testAlert := &alerting.Alert{
		ID:          "test-alert",
		Name:        "Test Alert",
		Description: "This is a test notification from the Unheaded Kingdom alerting system.",
		State:       alerting.AlertStateFiring,
		Severity:    alerting.SeverityInfo,
		Labels:      map[string]string{"alertname": "TestAlert", "test": "true"},
		Annotations: map[string]string{"summary": "Test notification"},
		StartsAt:    time.Now(),
		Fingerprint: "test-fingerprint",
	}

	return s.Send(ctx, []*alerting.Alert{testAlert})
}

// SlackChannelBuilder provides a fluent interface for building Slack channels.
type SlackChannelBuilder struct {
	config SlackConfig
}

// NewSlackChannelBuilder creates a new Slack channel builder.
func NewSlackChannelBuilder(name, webhookURL string) *SlackChannelBuilder {
	return &SlackChannelBuilder{
		config: SlackConfig{
			ChannelConfig: ChannelConfig{
				Name:         name,
				SendResolved: true,
			},
			WebhookURL: webhookURL,
		},
	}
}

// WithChannel sets the Slack channel.
func (b *SlackChannelBuilder) WithChannel(channel string) *SlackChannelBuilder {
	b.config.Channel = channel
	return b
}

// WithUsername sets the bot username.
func (b *SlackChannelBuilder) WithUsername(username string) *SlackChannelBuilder {
	b.config.Username = username
	return b
}

// WithIconEmoji sets the bot icon emoji.
func (b *SlackChannelBuilder) WithIconEmoji(emoji string) *SlackChannelBuilder {
	b.config.IconEmoji = emoji
	return b
}

// WithIconURL sets the bot icon URL.
func (b *SlackChannelBuilder) WithIconURL(url string) *SlackChannelBuilder {
	b.config.IconURL = url
	return b
}

// WithTimeout sets the request timeout.
func (b *SlackChannelBuilder) WithTimeout(timeout time.Duration) *SlackChannelBuilder {
	b.config.Timeout = timeout
	return b
}

// Build creates the Slack channel.
func (b *SlackChannelBuilder) Build() *SlackChannel {
	return NewSlackChannel(b.config)
}
