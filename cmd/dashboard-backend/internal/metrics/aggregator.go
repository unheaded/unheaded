// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package metrics provides time-series metrics aggregation and storage.
// This is a lightweight internal metrics aggregator for the dashboard backend.
// For the main metrics collection, see the scraper package.
package metrics

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"sync"
	"time"

	"unheaded/pkg/logger"
)

var (
	// ErrNilConfig indicates nil configuration
	ErrNilConfig = errors.New("config cannot be nil")
	// ErrInvalidRetention indicates invalid retention period
	ErrInvalidRetention = errors.New("retention period must be > 0")
	// ErrNilMetric indicates nil metric
	ErrNilMetric = errors.New("metric cannot be nil")
	// ErrEmptyMetricName indicates empty metric name
	ErrEmptyMetricName = errors.New("metric name cannot be empty")
	// ErrFutureTimestamp indicates future timestamp
	ErrFutureTimestamp = errors.New("metric timestamp cannot be in the future")
	// ErrMaxSeriesReached indicates series limit reached
	ErrMaxSeriesReached = errors.New("max series limit reached")
)

// AggFunc represents an aggregation function
type AggFunc string

const (
	// AggSum sums values
	AggSum AggFunc = "sum"
	// AggAvg averages values
	AggAvg AggFunc = "avg"
	// AggMin finds minimum value
	AggMin AggFunc = "min"
	// AggMax finds maximum value
	AggMax AggFunc = "max"
	// AggCount counts values
	AggCount AggFunc = "count"
)

// Config holds aggregator configuration
type Config struct {
	RetentionPeriod time.Duration // How long to keep metrics
	MaxSeries       int           // Maximum number of unique series
	FlushInterval   time.Duration // How often to flush old data
}

// DefaultConfig returns default aggregator configuration
func DefaultConfig() *Config {
	return &Config{
		RetentionPeriod: 1 * time.Hour,
		MaxSeries:       10000,
		FlushInterval:   1 * time.Minute,
	}
}

// Validate validates configuration
func (c *Config) Validate() error {
	if c == nil {
		return ErrNilConfig
	}
	if c.RetentionPeriod <= 0 {
		return ErrInvalidRetention
	}
	if c.MaxSeries <= 0 {
		c.MaxSeries = 10000
	}
	if c.FlushInterval == 0 {
		c.FlushInterval = 1 * time.Minute
	}
	return nil
}

// Metric represents a single metric data point
type Metric struct {
	Name      string            // Metric name
	Value     float64           // Metric value
	Timestamp time.Time         // When the metric was recorded
	Labels    map[string]string // Label key-value pairs
}

// Query represents a metrics query
type Query struct {
	Name      string            // Metric name to query
	Start     time.Time         // Start of time range
	End       time.Time         // End of time range
	Labels    map[string]string // Label filters
	Aggregate AggFunc           // Aggregation function (optional)
}

// seriesKey generates a unique key for a metric series
func seriesKey(name string, labels map[string]string) string {
	key := name
	if len(labels) > 0 {
		// Sort labels for consistent keys
		keys := make([]string, 0, len(labels))
		for k := range labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			key += fmt.Sprintf(",%s=%s", k, labels[k])
		}
	}
	return key
}

// matchesLabels checks if metric labels match query filters
func matchesLabels(metricLabels, queryLabels map[string]string) bool {
	for k, v := range queryLabels {
		if metricLabels[k] != v {
			return false
		}
	}
	return true
}

// timeSeries holds metrics for a single series
type timeSeries struct {
	name   string
	labels map[string]string
	points []*Metric
	mu     sync.RWMutex
}

// add adds a metric point to the series
func (ts *timeSeries) add(m *Metric) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.points = append(ts.points, m)
}

// query returns points within time range
func (ts *timeSeries) query(start, end time.Time) []*Metric {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	result := make([]*Metric, 0)
	for _, point := range ts.points {
		if (point.Timestamp.Equal(start) || point.Timestamp.After(start)) &&
			(point.Timestamp.Equal(end) || point.Timestamp.Before(end)) {
			result = append(result, point)
		}
	}
	return result
}

// cleanup removes points older than cutoff
func (ts *timeSeries) cleanup(cutoff time.Time) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	valid := make([]*Metric, 0, len(ts.points))
	for _, point := range ts.points {
		if point.Timestamp.After(cutoff) {
			valid = append(valid, point)
		}
	}
	ts.points = valid
}

// Aggregator aggregates and stores metrics
type Aggregator struct {
	config *Config
	log    *logger.Logger

	series   map[string]*timeSeries
	seriesMu sync.RWMutex

	shutdown     chan struct{}
	shutdownOnce sync.Once
	wg           sync.WaitGroup
}

// NewAggregator creates a new metrics aggregator
func NewAggregator(config *Config) (*Aggregator, error) {
	if config == nil {
		return nil, ErrNilConfig
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	a := &Aggregator{
		config:   config,
		log:      logger.New(io.Discard),
		series:   make(map[string]*timeSeries),
		shutdown: make(chan struct{}),
	}

	// Start cleanup goroutine
	a.wg.Add(1)
	go a.cleanupLoop()

	return a, nil
}

// cleanupLoop periodically removes old metrics
func (a *Aggregator) cleanupLoop() {
	defer a.wg.Done()

	ticker := time.NewTicker(a.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-a.shutdown:
			return
		case <-ticker.C:
			a.cleanup()
		}
	}
}

// cleanup removes metrics older than retention period
func (a *Aggregator) cleanup() {
	cutoff := time.Now().Add(-a.config.RetentionPeriod)

	a.seriesMu.RLock()
	defer a.seriesMu.RUnlock()

	for _, ts := range a.series {
		ts.cleanup(cutoff)
	}

	a.log.Debug().
		Time("cutoff", cutoff).
		Dur("retention", a.config.RetentionPeriod).
		Msg("metrics cleanup complete")
}

// RecordMetric records a new metric data point
func (a *Aggregator) RecordMetric(m *Metric) error {
	// Validation
	if m == nil {
		return ErrNilMetric
	}
	if m.Name == "" {
		return ErrEmptyMetricName
	}
	if m.Timestamp.After(time.Now().Add(1 * time.Minute)) {
		return ErrFutureTimestamp
	}
	if m.Timestamp.IsZero() {
		m.Timestamp = time.Now()
	}

	// Generate series key
	key := seriesKey(m.Name, m.Labels)

	// Get or create series
	a.seriesMu.Lock()
	ts, exists := a.series[key]
	if !exists {
		// Check series limit
		if len(a.series) >= a.config.MaxSeries {
			a.seriesMu.Unlock()
			return ErrMaxSeriesReached
		}

		// Create new series
		ts = &timeSeries{
			name:   m.Name,
			labels: m.Labels,
			points: make([]*Metric, 0, 1024),
		}
		a.series[key] = ts
		a.log.Debug().
			Str("series", key).
			Msg("created new metric series")
	}
	a.seriesMu.Unlock()

	// Add point to series
	ts.add(m)

	return nil
}

// QueryMetrics queries metrics matching the given criteria
func (a *Aggregator) QueryMetrics(q *Query) ([]*Metric, error) {
	if q == nil {
		return nil, errors.New("query cannot be nil")
	}
	if q.Name == "" {
		return nil, errors.New("query name cannot be empty")
	}
	if q.End.Before(q.Start) {
		return nil, errors.New("end time must be after start time")
	}

	results := make([]*Metric, 0)

	a.seriesMu.RLock()
	defer a.seriesMu.RUnlock()

	// Find matching series
	for _, ts := range a.series {
		if ts.name != q.Name {
			continue
		}
		if !matchesLabels(ts.labels, q.Labels) {
			continue
		}

		// Query points from series
		points := ts.query(q.Start, q.End)
		results = append(results, points...)
	}

	// Apply aggregation if specified
	if q.Aggregate != "" && len(results) > 0 {
		aggregated := a.aggregate(results, q.Aggregate)
		return []*Metric{aggregated}, nil
	}

	return results, nil
}

// aggregate applies aggregation function to metrics
func (a *Aggregator) aggregate(metrics []*Metric, fn AggFunc) *Metric {
	if len(metrics) == 0 {
		return nil
	}

	var value float64
	switch fn {
	case AggSum:
		for _, m := range metrics {
			value += m.Value
		}
	case AggAvg:
		sum := 0.0
		for _, m := range metrics {
			sum += m.Value
		}
		value = sum / float64(len(metrics))
	case AggMin:
		value = metrics[0].Value
		for _, m := range metrics {
			if m.Value < value {
				value = m.Value
			}
		}
	case AggMax:
		value = metrics[0].Value
		for _, m := range metrics {
			if m.Value > value {
				value = m.Value
			}
		}
	case AggCount:
		value = float64(len(metrics))
	default:
		value = 0
	}

	return &Metric{
		Name:      metrics[0].Name,
		Value:     value,
		Timestamp: time.Now(),
		Labels:    metrics[0].Labels,
	}
}

// SeriesCount returns the number of active series
func (a *Aggregator) SeriesCount() int {
	a.seriesMu.RLock()
	defer a.seriesMu.RUnlock()
	return len(a.series)
}

// Shutdown gracefully shuts down the aggregator
func (a *Aggregator) Shutdown(ctx context.Context) error {
	var shutdownErr error

	a.shutdownOnce.Do(func() {
		a.log.Info().Msg("shutting down metrics aggregator")
		close(a.shutdown)

		// Wait for cleanup loop
		done := make(chan struct{})
		go func() {
			a.wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			a.log.Info().Msg("metrics aggregator shutdown complete")
		case <-ctx.Done():
			shutdownErr = ctx.Err()
			a.log.Error().Err(shutdownErr).Msg("metrics aggregator shutdown timeout")
		}
	})

	return shutdownErr
}
