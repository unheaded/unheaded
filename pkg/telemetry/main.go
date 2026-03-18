// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package telemetry provides application telemetry publishing
// (metrics, logs, traces) to monitoring backends.
//
// The default implementation is NoOpPublisher (safe no-op for dev/testing).
// Services can swap in a real publisher at startup:
//
//	telemetry.DefaultPublisher = NewPrometheusPublisher(...)
package telemetry

import (
	"context"
	"sync"
)

// MetricType represents the type of metric.
type MetricType string

const (
	MetricTypeCounter   MetricType = "counter"
	MetricTypeGauge     MetricType = "gauge"
	MetricTypeHistogram MetricType = "histogram"
)

// Metric represents a single metric observation.
type Metric struct {
	Name  string            `json:"name"`
	Type  MetricType        `json:"type"`
	Value float64           `json:"value"`
	Tags  map[string]string `json:"tags,omitempty"`
}

// Publisher publishes application telemetry to monitoring backends.
//
// Backends are interchangeable: Prometheus, Grafana, Datadog, ELK, etc.
// The default implementation is NoOpPublisher (safe no-op for dev/testing).
type Publisher interface {
	// Publish sends a metric to the backend.
	Publish(ctx context.Context, metric *Metric) error

	// Close closes the publisher and flushes pending metrics.
	Close() error
}

// NoOpPublisher is a no-op telemetry publisher for development/testing.
type NoOpPublisher struct {
	mu sync.Mutex
}

// Publish is a no-op that returns nil.
func (n *NoOpPublisher) Publish(_ context.Context, _ *Metric) error {
	return nil
}

// Close is a no-op that returns nil.
func (n *NoOpPublisher) Close() error {
	return nil
}

// DefaultPublisher is the global telemetry publisher (initially no-op).
// Services may override this at startup with their preferred backend.
var DefaultPublisher Publisher = &NoOpPublisher{}

// Publish sends a metric to the default publisher.
func Publish(ctx context.Context, metric *Metric) error {
	return DefaultPublisher.Publish(ctx, metric)
}

// Close closes the default publisher.
func Close() error {
	return DefaultPublisher.Close()
}
