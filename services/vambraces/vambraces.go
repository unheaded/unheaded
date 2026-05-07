// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package vambraces provides observability, metrics, and alerting capabilities.
// Vambraces the Observability Stack is the Kingdom's sight - monitoring all systems.
// Implements metrics collection, distributed tracing, and SLO management.
package vambraces

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"unheaded/pkg/logger"
	wotanClient "unheaded/pkg/wotan-client"
)

// Metric represents a collected metric.
type Metric struct {
	Name      string            `json:"name"`
	Type      MetricType        `json:"type"`
	Value     float64           `json:"value"`
	Labels    map[string]string `json:"labels,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
}

// MetricType defines the kind of metric.
type MetricType string

const (
	MetricCounter   MetricType = "counter"
	MetricGauge     MetricType = "gauge"
	MetricHistogram MetricType = "histogram"
	MetricSummary   MetricType = "summary"
)

// Span represents a distributed trace span.
type Span struct {
	TraceID   string            `json:"trace_id"`
	SpanID    string            `json:"span_id"`
	ParentID  string            `json:"parent_id,omitempty"`
	Name      string            `json:"name"`
	Service   string            `json:"service"`
	StartTime time.Time         `json:"start_time"`
	EndTime   *time.Time        `json:"end_time,omitempty"`
	Duration  time.Duration     `json:"duration,omitempty"`
	Status    SpanStatus        `json:"status"`
	Tags      map[string]string `json:"tags,omitempty"`
	Logs      []SpanLog         `json:"logs,omitempty"`
}

// SpanStatus indicates span outcome.
type SpanStatus string

const (
	SpanOK    SpanStatus = "ok"
	SpanError SpanStatus = "error"
)

// SpanLog represents a log within a span.
type SpanLog struct {
	Timestamp time.Time              `json:"timestamp"`
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

// SLO represents a Service Level Objective.
type SLO struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Service     string    `json:"service"`
	Indicator   string    `json:"indicator"` // latency, availability, error_rate
	Target      float64   `json:"target"`    // 99.9 for 99.9%
	Window      string    `json:"window"`    // 7d, 30d
	Current     float64   `json:"current"`
	Budget      float64   `json:"budget"` // Remaining error budget
	LastUpdated time.Time `json:"last_updated"`
}

// Alert represents an alerting rule.
type Alert struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Query       string            `json:"query"`
	Condition   string            `json:"condition"`
	Threshold   float64           `json:"threshold"`
	Duration    string            `json:"duration"`
	Severity    string            `json:"severity"` // critical, warning, info
	State       AlertState        `json:"state"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	LastFired   *time.Time        `json:"last_fired,omitempty"`
}

// AlertState represents alert status.
type AlertState string

const (
	AlertInactive AlertState = "inactive"
	AlertPending  AlertState = "pending"
	AlertFiring   AlertState = "firing"
)

// Dashboard represents a monitoring dashboard.
type Dashboard struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Panels []Panel `json:"panels"`
}

// Panel represents a dashboard panel.
type Panel struct {
	ID      string            `json:"id"`
	Title   string            `json:"title"`
	Type    string            `json:"type"` // graph, stat, table
	Query   string            `json:"query"`
	Options map[string]string `json:"options,omitempty"`
}

// Service is the main Vambraces observability service.
type Service struct {
	log    *logger.Logger
	wotan  *wotanClient.Client
	config *Config

	mu         sync.RWMutex
	metrics    map[string][]*Metric // metric name -> values
	traces     map[string][]*Span   // trace ID -> spans
	slos       map[string]*SLO
	alerts     map[string]*Alert
	dashboards map[string]*Dashboard

	idCounter int64
}

// Config holds Vambraces service configuration.
type Config struct {
	MetricRetention    time.Duration `json:"metric_retention"`
	TraceRetention     time.Duration `json:"trace_retention"`
	ScrapeInterval     time.Duration `json:"scrape_interval"`
	EvaluationInterval time.Duration `json:"evaluation_interval"`
	WotanTopic         string        `json:"wotan_topic"`
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		MetricRetention:    7 * 24 * time.Hour,
		TraceRetention:     24 * time.Hour,
		ScrapeInterval:     15 * time.Second,
		EvaluationInterval: 1 * time.Minute,
		WotanTopic:         "vambraces.observability",
	}
}

// NewService creates a new Vambraces observability service.
func NewService(log *logger.Logger, wotan *wotanClient.Client, cfg *Config) *Service {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	return &Service{
		log:        log,
		wotan:      wotan,
		config:     cfg,
		metrics:    make(map[string][]*Metric),
		traces:     make(map[string][]*Span),
		slos:       make(map[string]*SLO),
		alerts:     make(map[string]*Alert),
		dashboards: make(map[string]*Dashboard),
	}
}

// Start initializes the Vambraces service.
func (s *Service) Start(ctx context.Context) error {
	s.log.Info().Msg("Vambraces strapped - observability active")
	go s.evaluationLoop(ctx)
	return nil
}

// Stop gracefully shuts down the service.
func (s *Service) Stop() error {
	s.log.Info().Msg("Vambraces removed - observability suspended")
	return nil
}

// RecordMetric records a metric value.
func (s *Service) RecordMetric(metric *Metric) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if metric.Timestamp.IsZero() {
		metric.Timestamp = time.Now()
	}

	s.metrics[metric.Name] = append(s.metrics[metric.Name], metric)
}

// GetMetrics returns metrics for a name.
func (s *Service) GetMetrics(name string, from, to time.Time) []*Metric {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*Metric
	for _, m := range s.metrics[name] {
		if m.Timestamp.After(from) && m.Timestamp.Before(to) {
			results = append(results, m)
		}
	}
	return results
}

// StartSpan starts a new trace span.
func (s *Service) StartSpan(traceID, spanID, parentID, name, service string) *Span {
	span := &Span{
		TraceID:   traceID,
		SpanID:    spanID,
		ParentID:  parentID,
		Name:      name,
		Service:   service,
		StartTime: time.Now(),
		Status:    SpanOK,
		Tags:      make(map[string]string),
		Logs:      make([]SpanLog, 0),
	}

	s.mu.Lock()
	s.traces[traceID] = append(s.traces[traceID], span)
	s.mu.Unlock()

	return span
}

// EndSpan completes a span.
func (s *Service) EndSpan(span *Span, status SpanStatus) {
	now := time.Now()
	span.EndTime = &now
	span.Duration = now.Sub(span.StartTime)
	span.Status = status
}

// GetTrace returns all spans for a trace.
func (s *Service) GetTrace(traceID string) []*Span {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.traces[traceID]
}

// CreateSLO creates a new SLO.
func (s *Service) CreateSLO(slo *SLO) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if slo.ID == "" {
		slo.ID = fmt.Sprintf("slo-%s-%d-%d", slo.Service, time.Now().UnixNano(), atomic.AddInt64(&s.idCounter, 1))
	}
	slo.LastUpdated = time.Now()
	slo.Current = slo.Target // Initialize at target
	slo.Budget = 100 - slo.Target

	s.slos[slo.ID] = slo
	s.log.Info().Str("id", slo.ID).Str("service", slo.Service).Float64("target", slo.Target).Msg("SLO created")
	return nil
}

// GetSLO returns an SLO by ID.
func (s *Service) GetSLO(sloID string) (*SLO, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	slo, ok := s.slos[sloID]
	return slo, ok
}

// ListSLOs returns all SLOs.
func (s *Service) ListSLOs() []*SLO {
	s.mu.RLock()
	defer s.mu.RUnlock()

	slos := make([]*SLO, 0, len(s.slos))
	for _, slo := range s.slos {
		slos = append(slos, slo)
	}
	return slos
}

// CreateAlert creates an alerting rule.
func (s *Service) CreateAlert(alert *Alert) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if alert.ID == "" {
		alert.ID = fmt.Sprintf("alert-%d-%d", time.Now().UnixNano(), atomic.AddInt64(&s.idCounter, 1))
	}
	alert.State = AlertInactive

	s.alerts[alert.ID] = alert
	s.log.Info().Str("id", alert.ID).Str("name", alert.Name).Msg("Alert created")
	return nil
}

// GetAlert returns an alert by ID.
func (s *Service) GetAlert(alertID string) (*Alert, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	alert, ok := s.alerts[alertID]
	return alert, ok
}

// ListAlerts returns all alerts.
func (s *Service) ListAlerts(state AlertState) []*Alert {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*Alert
	for _, a := range s.alerts {
		if state == "" || a.State == state {
			results = append(results, a)
		}
	}
	return results
}

// CreateDashboard creates a new dashboard.
func (s *Service) CreateDashboard(dashboard *Dashboard) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if dashboard.ID == "" {
		dashboard.ID = fmt.Sprintf("dash-%d-%d", time.Now().UnixNano(), atomic.AddInt64(&s.idCounter, 1))
	}

	s.dashboards[dashboard.ID] = dashboard
	s.log.Info().Str("id", dashboard.ID).Str("name", dashboard.Name).Msg("Dashboard created")
	return nil
}

// GetDashboard returns a dashboard by ID.
func (s *Service) GetDashboard(dashboardID string) (*Dashboard, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	dashboard, ok := s.dashboards[dashboardID]
	return dashboard, ok
}

func (s *Service) evaluationLoop(ctx context.Context) {
	ticker := time.NewTicker(s.config.EvaluationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.evaluateAlerts()
			s.updateSLOs()
		}
	}
}

func (s *Service) evaluateAlerts() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, alert := range s.alerts {
		// Simplified evaluation - real implementation would query metrics
		_ = alert
	}
}

func (s *Service) updateSLOs() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, slo := range s.slos {
		slo.LastUpdated = time.Now()
		// Would calculate actual SLI values
	}
}

// Stats returns service statistics.
func (s *Service) Stats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	totalMetrics := 0
	for _, ms := range s.metrics {
		totalMetrics += len(ms)
	}

	firingAlerts := 0
	for _, a := range s.alerts {
		if a.State == AlertFiring {
			firingAlerts++
		}
	}

	return map[string]interface{}{
		"metric_series": len(s.metrics),
		"total_metrics": totalMetrics,
		"active_traces": len(s.traces),
		"total_slos":    len(s.slos),
		"total_alerts":  len(s.alerts),
		"firing_alerts": firingAlerts,
		"dashboards":    len(s.dashboards),
	}
}

// RegisterRoutes registers /health, /ready, and /metrics endpoints on the provided mux.
// Required by CLAUDE.md: every component must expose these endpoints.
func (s *Service) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "healthy",
			"service": "vambraces",
		})
	})

	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ready":   true,
			"service": "vambraces",
		})
	})

	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		stats := s.Stats()
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprintf(w, "# HELP vambraces_metric_series Total metric series\n")
		fmt.Fprintf(w, "# TYPE vambraces_metric_series gauge\n")
		fmt.Fprintf(w, "vambraces_metric_series %v\n", stats["metric_series"])
		fmt.Fprintf(w, "# HELP vambraces_total_metrics Total metrics collected\n")
		fmt.Fprintf(w, "# TYPE vambraces_total_metrics gauge\n")
		fmt.Fprintf(w, "vambraces_total_metrics %v\n", stats["total_metrics"])
		fmt.Fprintf(w, "# HELP vambraces_active_traces Active traces\n")
		fmt.Fprintf(w, "# TYPE vambraces_active_traces gauge\n")
		fmt.Fprintf(w, "vambraces_active_traces %v\n", stats["active_traces"])
		fmt.Fprintf(w, "# HELP vambraces_firing_alerts Firing alerts\n")
		fmt.Fprintf(w, "# TYPE vambraces_firing_alerts gauge\n")
		fmt.Fprintf(w, "vambraces_firing_alerts %v\n", stats["firing_alerts"])
	})
}
