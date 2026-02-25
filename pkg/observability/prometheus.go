// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package observability

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
)

// PrometheusAdapter exports metrics in Prometheus exposition format.
// This is the default metrics adapter for Alpha.
type PrometheusAdapter struct {
	mu      sync.RWMutex
	metrics map[string]*prometheusMetric
	closed  atomic.Bool
}

type prometheusMetric struct {
	name   string
	mtype  MetricType
	labels map[string]string
	value  float64
}

// NewPrometheusAdapter creates a Prometheus metrics adapter.
func NewPrometheusAdapter() *PrometheusAdapter {
	return &PrometheusAdapter{
		metrics: make(map[string]*prometheusMetric),
	}
}

func (a *PrometheusAdapter) Backend() Backend { return BackendPrometheus }

func (a *PrometheusAdapter) Supports() []SignalType {
	return []SignalType{SignalMetric}
}

func (a *PrometheusAdapter) EmitMetric(_ context.Context, m Metric) error {
	if a.closed.Load() {
		return ErrAdapterClosed
	}
	key := metricKey(m.Name, m.Labels)
	a.mu.Lock()
	defer a.mu.Unlock()

	existing, ok := a.metrics[key]
	if !ok {
		a.metrics[key] = &prometheusMetric{
			name:   m.Name,
			mtype:  m.Type,
			labels: m.Labels,
			value:  m.Value,
		}
		return nil
	}

	switch m.Type {
	case MetricCounter:
		existing.value += m.Value
	case MetricGauge:
		existing.value = m.Value
	default:
		existing.value = m.Value
	}
	return nil
}

func (a *PrometheusAdapter) EmitLog(_ context.Context, _ LogEntry) error {
	return nil // Prometheus doesn't handle logs
}

func (a *PrometheusAdapter) EmitTrace(_ context.Context, _ Span) error {
	return nil // Prometheus doesn't handle traces
}

func (a *PrometheusAdapter) EmitAlert(_ context.Context, _ Alert) error {
	return nil // Use AlertManager for alerts
}

func (a *PrometheusAdapter) Close() error {
	a.closed.Store(true)
	return nil
}

// Exposition returns the current metrics in Prometheus exposition format.
func (a *PrometheusAdapter) Exposition() string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var s string
	for _, m := range a.metrics {
		s += fmt.Sprintf("# TYPE %s %s\n", m.name, m.mtype)
		s += fmt.Sprintf("%s", m.name)
		if len(m.labels) > 0 {
			s += "{"
			first := true
			for k, v := range m.labels {
				if !first {
					s += ","
				}
				s += fmt.Sprintf(`%s="%s"`, k, v)
				first = false
			}
			s += "}"
		}
		s += fmt.Sprintf(" %g\n", m.value)
	}
	return s
}

// MetricCount returns the number of tracked metrics.
func (a *PrometheusAdapter) MetricCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.metrics)
}

func metricKey(name string, labels map[string]string) string {
	key := name
	for k, v := range labels {
		key += ":" + k + "=" + v
	}
	return key
}
