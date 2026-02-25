// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package observability provides pluggable adapter interfaces for metrics,
// logging, tracing, and alerting backends.
//
// Unheaded emits OpenTelemetry-compatible signals (metrics, logs, traces).
// Users plug in their preferred backends via the Adapter interface. Multiple
// adapters can be active simultaneously (fan-out pattern).
//
// Supported backends (Alpha ships Prometheus + zerolog; rest deferred):
//   - Prometheus (metrics)
//   - Grafana (dashboards)
//   - ELK / Elasticsearch + Logstash + Kibana (logging + search)
//   - Fluentd / Fluent Bit (log forwarding)
//   - Jaeger (distributed tracing)
//   - Nagios (monitoring + alerting)
//   - Loki + Promtail (log aggregation)
//   - Custom Wotan-native backends (future)
//
// Usage:
//
//	adapter := observability.NewPrometheusAdapter(cfg)
//	pipeline := observability.NewPipeline(adapter)
//	pipeline.EmitMetric(ctx, metric)
//	pipeline.EmitLog(ctx, logEntry)
//	pipeline.EmitTrace(ctx, span)
package observability

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Backend identifies an observability backend.
type Backend string

const (
	BackendPrometheus Backend = "prometheus"
	BackendGrafana    Backend = "grafana"
	BackendELK        Backend = "elk"
	BackendFluentd    Backend = "fluentd"
	BackendJaeger     Backend = "jaeger"
	BackendNagios     Backend = "nagios"
	BackendLoki       Backend = "loki"
	BackendWotan      Backend = "wotan"
)

// AllBackends returns all supported backend identifiers.
func AllBackends() []Backend {
	return []Backend{
		BackendPrometheus,
		BackendGrafana,
		BackendELK,
		BackendFluentd,
		BackendJaeger,
		BackendNagios,
		BackendLoki,
		BackendWotan,
	}
}

// SignalType classifies observability signals.
type SignalType string

const (
	SignalMetric SignalType = "metric"
	SignalLog    SignalType = "log"
	SignalTrace  SignalType = "trace"
	SignalAlert  SignalType = "alert"
)

// Common errors.
var (
	ErrAdapterClosed  = errors.New("observability: adapter closed")
	ErrUnknownBackend = errors.New("observability: unknown backend")
	ErrBufferFull     = errors.New("observability: buffer full")
)

// Metric represents a single metric data point.
type Metric struct {
	Name      string
	Value     float64
	Labels    map[string]string
	Timestamp time.Time
	Type      MetricType
}

// MetricType classifies metrics.
type MetricType string

const (
	MetricCounter   MetricType = "counter"
	MetricGauge     MetricType = "gauge"
	MetricHistogram MetricType = "histogram"
	MetricSummary   MetricType = "summary"
)

// LogEntry represents a structured log event.
type LogEntry struct {
	Level     string
	Message   string
	Service   string
	TraceID   string
	Fields    map[string]string
	Timestamp time.Time
}

// Span represents a distributed trace span.
type Span struct {
	TraceID    string
	SpanID     string
	ParentID   string
	Operation  string
	Service    string
	StartTime  time.Time
	Duration   time.Duration
	Status     SpanStatus
	Attributes map[string]string
}

// SpanStatus indicates the outcome of a span.
type SpanStatus string

const (
	SpanOK    SpanStatus = "ok"
	SpanError SpanStatus = "error"
)

// Alert represents an alerting event.
type Alert struct {
	Name      string
	Severity  AlertSeverity
	Message   string
	Service   string
	Labels    map[string]string
	Timestamp time.Time
}

// AlertSeverity classifies alert urgency.
type AlertSeverity string

const (
	SeverityInfo     AlertSeverity = "info"
	SeverityWarning  AlertSeverity = "warning"
	SeverityError    AlertSeverity = "error"
	SeverityCritical AlertSeverity = "critical"
)

// Adapter is the interface that all observability backends implement.
// Each adapter handles one or more signal types.
type Adapter interface {
	// Backend returns the backend identifier.
	Backend() Backend

	// Supports returns which signal types this adapter handles.
	Supports() []SignalType

	// EmitMetric sends a metric to the backend.
	EmitMetric(ctx context.Context, m Metric) error

	// EmitLog sends a log entry to the backend.
	EmitLog(ctx context.Context, e LogEntry) error

	// EmitTrace sends a trace span to the backend.
	EmitTrace(ctx context.Context, s Span) error

	// EmitAlert sends an alert to the backend.
	EmitAlert(ctx context.Context, a Alert) error

	// Close shuts down the adapter and flushes pending data.
	Close() error
}

// Pipeline fans out observability signals to multiple adapters.
// It is the primary integration point for services.
type Pipeline struct {
	mu       sync.RWMutex
	adapters []Adapter
	closed   bool
}

// NewPipeline creates a pipeline with the given adapters.
func NewPipeline(adapters ...Adapter) *Pipeline {
	return &Pipeline{
		adapters: adapters,
	}
}

// AddAdapter adds an adapter to the pipeline at runtime.
func (p *Pipeline) AddAdapter(a Adapter) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.adapters = append(p.adapters, a)
}

// EmitMetric fans out a metric to all adapters that support it.
func (p *Pipeline) EmitMetric(ctx context.Context, m Metric) error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed {
		return ErrAdapterClosed
	}
	if m.Timestamp.IsZero() {
		m.Timestamp = time.Now()
	}
	var lastErr error
	for _, a := range p.adapters {
		if supports(a, SignalMetric) {
			if err := a.EmitMetric(ctx, m); err != nil {
				lastErr = err
			}
		}
	}
	return lastErr
}

// EmitLog fans out a log entry to all adapters that support it.
func (p *Pipeline) EmitLog(ctx context.Context, e LogEntry) error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed {
		return ErrAdapterClosed
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}
	var lastErr error
	for _, a := range p.adapters {
		if supports(a, SignalLog) {
			if err := a.EmitLog(ctx, e); err != nil {
				lastErr = err
			}
		}
	}
	return lastErr
}

// EmitTrace fans out a trace span to all adapters that support it.
func (p *Pipeline) EmitTrace(ctx context.Context, s Span) error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed {
		return ErrAdapterClosed
	}
	var lastErr error
	for _, a := range p.adapters {
		if supports(a, SignalTrace) {
			if err := a.EmitTrace(ctx, s); err != nil {
				lastErr = err
			}
		}
	}
	return lastErr
}

// EmitAlert fans out an alert to all adapters that support it.
func (p *Pipeline) EmitAlert(ctx context.Context, a Alert) error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed {
		return ErrAdapterClosed
	}
	if a.Timestamp.IsZero() {
		a.Timestamp = time.Now()
	}
	var lastErr error
	for _, ad := range p.adapters {
		if supports(ad, SignalAlert) {
			if err := ad.EmitAlert(ctx, a); err != nil {
				lastErr = err
			}
		}
	}
	return lastErr
}

// Close shuts down all adapters.
func (p *Pipeline) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	var lastErr error
	for _, a := range p.adapters {
		if err := a.Close(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// Adapters returns the current adapter list.
func (p *Pipeline) Adapters() []Adapter {
	p.mu.RLock()
	defer p.mu.RUnlock()
	result := make([]Adapter, len(p.adapters))
	copy(result, p.adapters)
	return result
}

func supports(a Adapter, signal SignalType) bool {
	for _, s := range a.Supports() {
		if s == signal {
			return true
		}
	}
	return false
}
