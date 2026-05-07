// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
)

// GrafanaAdapter integrates with Grafana for dashboards and alerting.
// It generates dashboard JSON models and manages data source configuration.
// Metrics are read from Prometheus; this adapter handles dashboard provisioning
// and annotation events.
type GrafanaAdapter struct {
	mu          sync.RWMutex
	dashboards  map[string]*DashboardModel
	annotations []Annotation
	closed      atomic.Bool
}

// DashboardModel represents a Grafana dashboard definition.
type DashboardModel struct {
	UID         string           `json:"uid"`
	Title       string           `json:"title"`
	Tags        []string         `json:"tags"`
	Panels      []PanelModel     `json:"panels"`
	Templating  []TemplateVar    `json:"templating"`
	Time        DashboardTime    `json:"time"`
	Annotations []AnnotationConf `json:"annotations"`
}

// PanelModel represents a Grafana dashboard panel.
type PanelModel struct {
	ID        int            `json:"id"`
	Title     string         `json:"title"`
	Type      string         `json:"type"` // graph, stat, table, heatmap, logs
	GridPos   GridPos        `json:"gridPos"`
	Targets   []PanelTarget  `json:"targets"`
	FieldConf map[string]any `json:"fieldConfig,omitempty"`
}

// PanelTarget represents a Prometheus query target.
type PanelTarget struct {
	Expr         string `json:"expr"`
	LegendFormat string `json:"legendFormat,omitempty"`
	RefID        string `json:"refId"`
}

// GridPos represents panel position on the dashboard grid.
type GridPos struct {
	H int `json:"h"`
	W int `json:"w"`
	X int `json:"x"`
	Y int `json:"y"`
}

// TemplateVar is a dashboard template variable.
type TemplateVar struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Query   string `json:"query"`
	Current string `json:"current"`
}

// DashboardTime defines the dashboard time range.
type DashboardTime struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// AnnotationConf configures dashboard annotations.
type AnnotationConf struct {
	Name       string `json:"name"`
	Datasource string `json:"datasource"`
	Enable     bool   `json:"enable"`
}

// Annotation is an event marker on a dashboard.
type Annotation struct {
	Time    int64    `json:"time"`
	Text    string   `json:"text"`
	Tags    []string `json:"tags"`
	PanelID int      `json:"panelId,omitempty"`
}

// NewGrafanaAdapter creates a Grafana dashboard adapter.
func NewGrafanaAdapter() *GrafanaAdapter {
	return &GrafanaAdapter{
		dashboards: make(map[string]*DashboardModel),
	}
}

func (a *GrafanaAdapter) Backend() Backend { return BackendGrafana }

func (a *GrafanaAdapter) Supports() []SignalType {
	return []SignalType{SignalMetric, SignalAlert}
}

func (a *GrafanaAdapter) EmitMetric(_ context.Context, m Metric) error {
	if a.closed.Load() {
		return ErrAdapterClosed
	}
	// Grafana reads metrics from Prometheus data source; this is a no-op for
	// direct metric ingestion. Annotations are created for notable events.
	return nil
}

func (a *GrafanaAdapter) EmitLog(_ context.Context, _ LogEntry) error {
	return nil // Grafana reads logs from Loki data source
}

func (a *GrafanaAdapter) EmitTrace(_ context.Context, _ Span) error {
	return nil // Grafana reads traces from Jaeger/Tempo
}

func (a *GrafanaAdapter) EmitAlert(_ context.Context, alert Alert) error {
	if a.closed.Load() {
		return ErrAdapterClosed
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	a.annotations = append(a.annotations, Annotation{
		Time: alert.Timestamp.UnixMilli(),
		Text: fmt.Sprintf("[%s] %s: %s", alert.Severity, alert.Name, alert.Message),
		Tags: []string{string(alert.Severity), alert.Service},
	})
	return nil
}

func (a *GrafanaAdapter) Close() error {
	a.closed.Store(true)
	return nil
}

// GenerateServiceDashboard creates a dashboard model for a service.
func (a *GrafanaAdapter) GenerateServiceDashboard(service string) *DashboardModel {
	dash := &DashboardModel{
		UID:   fmt.Sprintf("unheaded-%s", service),
		Title: fmt.Sprintf("Unheaded - %s", service),
		Tags:  []string{"unheaded", service},
		Time:  DashboardTime{From: "now-1h", To: "now"},
		Templating: []TemplateVar{
			{Name: "interval", Type: "interval", Query: "1m,5m,15m,1h", Current: "1m"},
		},
		Panels: []PanelModel{
			{
				ID:      1,
				Title:   "Request Rate",
				Type:    "graph",
				GridPos: GridPos{H: 8, W: 12, X: 0, Y: 0},
				Targets: []PanelTarget{{
					Expr:         fmt.Sprintf(`rate(unheaded_http_requests_total{service="%s"}[$interval])`, service),
					LegendFormat: "{{method}} {{path}}",
					RefID:        "A",
				}},
			},
			{
				ID:      2,
				Title:   "Request Latency (P99)",
				Type:    "graph",
				GridPos: GridPos{H: 8, W: 12, X: 12, Y: 0},
				Targets: []PanelTarget{{
					Expr:         fmt.Sprintf(`histogram_quantile(0.99, rate(unheaded_http_request_duration_seconds_bucket{service="%s"}[$interval]))`, service),
					LegendFormat: "P99",
					RefID:        "A",
				}},
			},
			{
				ID:      3,
				Title:   "Wotan Messages",
				Type:    "stat",
				GridPos: GridPos{H: 4, W: 6, X: 0, Y: 8},
				Targets: []PanelTarget{{
					Expr:  fmt.Sprintf(`sum(rate(unheaded_wotan_messages_published_total{service="%s"}[$interval]))`, service),
					RefID: "A",
				}},
			},
			{
				ID:      4,
				Title:   "Error Rate",
				Type:    "stat",
				GridPos: GridPos{H: 4, W: 6, X: 6, Y: 8},
				Targets: []PanelTarget{{
					Expr:  fmt.Sprintf(`sum(rate(unheaded_http_requests_total{service="%s",status=~"5.."}[$interval]))`, service),
					RefID: "A",
				}},
			},
		},
	}

	a.mu.Lock()
	a.dashboards[dash.UID] = dash
	a.mu.Unlock()

	return dash
}

// DashboardJSON returns the JSON representation of a dashboard.
func (a *GrafanaAdapter) DashboardJSON(uid string) (string, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	dash, ok := a.dashboards[uid]
	if !ok {
		return "", fmt.Errorf("dashboard %q not found", uid)
	}
	data, err := json.MarshalIndent(dash, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Annotations returns all recorded annotations.
func (a *GrafanaAdapter) Annotations() []Annotation {
	a.mu.RLock()
	defer a.mu.RUnlock()
	result := make([]Annotation, len(a.annotations))
	copy(result, a.annotations)
	return result
}

// DashboardCount returns the number of registered dashboards.
func (a *GrafanaAdapter) DashboardCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.dashboards)
}
