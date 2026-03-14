// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package notify provides notification channel implementations.
package notify

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"strings"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"unheaded/pkg/alerting"
)

// ChannelConfig holds common configuration for notification channels.
type ChannelConfig struct {
	Name           string            `json:"name"`
	SendResolved   bool              `json:"send_resolved"`
	RepeatInterval time.Duration     `json:"repeat_interval"`
	Labels         map[string]string `json:"labels"`
}

// NotificationStatus represents the result of a notification attempt.
type NotificationStatus struct {
	Channel   string    `json:"channel"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	AlertIDs  []string  `json:"alert_ids"`
}

// Router routes alerts to appropriate notification channels.
type Router struct {
	channels   map[string]alerting.NotificationChannel
	routes     []Route
	defaultCh  string
}

// Route defines a routing rule for alerts.
type Route struct {
	Matchers   []alerting.LabelMatcher `json:"matchers"`
	Channel    string                  `json:"channel"`
	Continue   bool                    `json:"continue"`
	GroupBy    []string                `json:"group_by"`
	GroupWait  time.Duration           `json:"group_wait"`
}

// NewRouter creates a new notification router.
func NewRouter(defaultChannel string) *Router {
	return &Router{
		channels:  make(map[string]alerting.NotificationChannel),
		routes:    make([]Route, 0),
		defaultCh: defaultChannel,
	}
}

// RegisterChannel registers a notification channel.
func (r *Router) RegisterChannel(channel alerting.NotificationChannel) {
	r.channels[channel.Name()] = channel
}

// AddRoute adds a routing rule.
func (r *Router) AddRoute(route Route) {
	r.routes = append(r.routes, route)
}

// Route routes alerts to appropriate channels.
func (r *Router) Route(ctx context.Context, alerts []*alerting.Alert) []NotificationStatus {
	statuses := make([]NotificationStatus, 0)

	// Group alerts by destination channel
	channelAlerts := make(map[string][]*alerting.Alert)

	for _, alert := range alerts {
		channels := r.findChannels(alert)
		if len(channels) == 0 {
			channels = []string{r.defaultCh}
		}

		for _, ch := range channels {
			channelAlerts[ch] = append(channelAlerts[ch], alert)
		}
	}

	// Send to each channel
	for chName, chAlerts := range channelAlerts {
		channel, exists := r.channels[chName]
		if !exists {
			statuses = append(statuses, NotificationStatus{
				Channel:   chName,
				Success:   false,
				Error:     "channel not found",
				Timestamp: time.Now(),
				AlertIDs:  getAlertIDs(chAlerts),
			})
			continue
		}

		err := channel.Send(ctx, chAlerts)
		status := NotificationStatus{
			Channel:   chName,
			Success:   err == nil,
			Timestamp: time.Now(),
			AlertIDs:  getAlertIDs(chAlerts),
		}
		if err != nil {
			status.Error = err.Error()
		}
		statuses = append(statuses, status)
	}

	return statuses
}

// findChannels finds all matching channels for an alert.
func (r *Router) findChannels(alert *alerting.Alert) []string {
	channels := make([]string, 0)

	for _, route := range r.routes {
		if r.matchesRoute(alert, route) {
			channels = append(channels, route.Channel)
			if !route.Continue {
				break
			}
		}
	}

	return channels
}

// matchesRoute checks if an alert matches a route.
func (r *Router) matchesRoute(alert *alerting.Alert, route Route) bool {
	for _, m := range route.Matchers {
		value := r.getLabelValue(alert, m.Name)
		if !r.matchValue(value, m.Value, m.Type) {
			return false
		}
	}
	return true
}

// getLabelValue gets a label value from an alert.
func (r *Router) getLabelValue(alert *alerting.Alert, name string) string {
	switch name {
	case "alertname":
		return alert.Name
	case "severity":
		return string(alert.Severity)
	default:
		return alert.Labels[name]
	}
}

// matchValue matches a value against a pattern.
func (r *Router) matchValue(value, pattern string, matchType alerting.MatchType) bool {
	switch matchType {
	case alerting.MatchTypeEqual:
		return value == pattern
	case alerting.MatchTypeNotEqual:
		return value != pattern
	default:
		return false
	}
}

// getAlertIDs extracts alert IDs from a slice of alerts.
func getAlertIDs(alerts []*alerting.Alert) []string {
	ids := make([]string, len(alerts))
	for i, a := range alerts {
		ids[i] = a.ID
	}
	return ids
}

// TemplateData holds data for rendering notification templates.
type TemplateData struct {
	Alerts      []*alerting.Alert
	GroupLabels map[string]string
	CommonLabels map[string]string
	ExternalURL string
	Status      string
}

// TemplateRenderer renders notification templates.
type TemplateRenderer struct {
	templates map[string]*template.Template
}

// NewTemplateRenderer creates a new template renderer.
func NewTemplateRenderer() *TemplateRenderer {
	return &TemplateRenderer{
		templates: make(map[string]*template.Template),
	}
}

// AddTemplate adds a named template.
func (tr *TemplateRenderer) AddTemplate(name, tmpl string) error {
	t, err := template.New(name).Funcs(templateFuncs()).Parse(tmpl)
	if err != nil {
		return err
	}
	tr.templates[name] = t
	return nil
}

// Render renders a template with the given data.
func (tr *TemplateRenderer) Render(name string, data *TemplateData) (string, error) {
	t, exists := tr.templates[name]
	if !exists {
		return "", fmt.Errorf("template not found: %s", name)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// templateFuncs returns custom template functions.
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"toUpper":     strings.ToUpper,
		"toLower":     strings.ToLower,
		"title":       cases.Title(language.English).String,
		"join":        strings.Join,
		"formatTime":  formatTime,
		"severityIcon": severityIcon,
		"stateIcon":   stateIcon,
	}
}

func formatTime(t time.Time) string {
	return t.Format("2006-01-02 15:04:05 MST")
}

func severityIcon(severity alerting.AlertSeverity) string {
	switch severity {
	case alerting.SeverityCritical:
		return "[CRITICAL]"
	case alerting.SeverityWarning:
		return "[WARNING]"
	case alerting.SeverityInfo:
		return "[INFO]"
	default:
		return "[UNKNOWN]"
	}
}

func stateIcon(state alerting.AlertState) string {
	switch state {
	case alerting.AlertStateFiring:
		return "[FIRING]"
	case alerting.AlertStateResolved:
		return "[RESOLVED]"
	case alerting.AlertStatePending:
		return "[PENDING]"
	default:
		return "[UNKNOWN]"
	}
}

// DefaultTemplates contains default notification templates.
var DefaultTemplates = map[string]string{
	"title": `{{ .Status | toUpper }}: {{ len .Alerts }} alert(s) {{ if .GroupLabels }}for {{ range $k, $v := .GroupLabels }}{{ $k }}={{ $v }} {{ end }}{{ end }}`,
	"body": `{{ range .Alerts }}
{{ severityIcon .Severity }} {{ .Name }}
Labels: {{ range $k, $v := .Labels }}{{ $k }}={{ $v }} {{ end }}
{{ if .Description }}Description: {{ .Description }}{{ end }}
Value: {{ .Value }} (threshold: {{ .Threshold }})
Started: {{ formatTime .StartsAt }}
{{ if eq .State "resolved" }}Resolved: {{ formatTime .ResolvedAt }}{{ end }}
---
{{ end }}`,
}

// Notifier is a high-level notification service.
type Notifier struct {
	router   *Router
	renderer *TemplateRenderer
	history  []NotificationStatus
}

// NewNotifier creates a new notifier service.
func NewNotifier(defaultChannel string) *Notifier {
	renderer := NewTemplateRenderer()
	for name, tmpl := range DefaultTemplates {
		_ = renderer.AddTemplate(name, tmpl)
	}

	return &Notifier{
		router:   NewRouter(defaultChannel),
		renderer: renderer,
		history:  make([]NotificationStatus, 0),
	}
}

// RegisterChannel registers a notification channel.
func (n *Notifier) RegisterChannel(channel alerting.NotificationChannel) {
	n.router.RegisterChannel(channel)
}

// AddRoute adds a routing rule.
func (n *Notifier) AddRoute(route Route) {
	n.router.AddRoute(route)
}

// Notify sends notifications for the given alerts.
func (n *Notifier) Notify(ctx context.Context, alerts []*alerting.Alert) []NotificationStatus {
	statuses := n.router.Route(ctx, alerts)
	n.history = append(n.history, statuses...)
	return statuses
}

// GetHistory returns the notification history.
func (n *Notifier) GetHistory() []NotificationStatus {
	return n.history
}
