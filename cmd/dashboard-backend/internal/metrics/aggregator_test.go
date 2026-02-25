// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package metrics

import (
	"context"
	"testing"
	"time"
)

// TestAggregator_NewAggregator tests aggregator creation
func TestAggregator_NewAggregator(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				RetentionPeriod: 1 * time.Hour,
				MaxSeries:       1000,
				FlushInterval:   10 * time.Second,
			},
			wantErr: false,
		},
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name: "zero retention",
			config: &Config{
				RetentionPeriod: 0,
				MaxSeries:       1000,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agg, err := NewAggregator(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewAggregator() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && agg == nil {
				t.Error("NewAggregator() returned nil aggregator")
			}
		})
	}
}

// TestAggregator_RecordMetric tests metric recording
func TestAggregator_RecordMetric(t *testing.T) {
	config := &Config{
		RetentionPeriod: 1 * time.Hour,
		MaxSeries:       1000,
		FlushInterval:   10 * time.Second,
	}

	agg, err := NewAggregator(config)
	if err != nil {
		t.Fatalf("NewAggregator() failed: %v", err)
	}
	defer agg.Shutdown(context.Background())

	tests := []struct {
		name    string
		metric  *Metric
		wantErr bool
	}{
		{
			name: "valid metric",
			metric: &Metric{
				Name:      "http_requests_total",
				Value:     100.0,
				Timestamp: time.Now(),
				Labels: map[string]string{
					"service": "dashboard",
					"method":  "GET",
				},
			},
			wantErr: false,
		},
		{
			name:    "nil metric",
			metric:  nil,
			wantErr: true,
		},
		{
			name: "empty name",
			metric: &Metric{
				Name:      "",
				Value:     100.0,
				Timestamp: time.Now(),
			},
			wantErr: true,
		},
		{
			name: "future timestamp",
			metric: &Metric{
				Name:      "test",
				Value:     100.0,
				Timestamp: time.Now().Add(1 * time.Hour),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := agg.RecordMetric(tt.metric)
			if (err != nil) != tt.wantErr {
				t.Errorf("RecordMetric() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestAggregator_QueryMetrics tests metric querying
func TestAggregator_QueryMetrics(t *testing.T) {
	config := &Config{
		RetentionPeriod: 1 * time.Hour,
		MaxSeries:       1000,
		FlushInterval:   10 * time.Second,
	}

	agg, err := NewAggregator(config)
	if err != nil {
		t.Fatalf("NewAggregator() failed: %v", err)
	}
	defer agg.Shutdown(context.Background())

	// Record test metrics
	now := time.Now()
	for i := 0; i < 5; i++ {
		err := agg.RecordMetric(&Metric{
			Name:      "test_metric",
			Value:     float64(i),
			Timestamp: now.Add(time.Duration(i) * time.Second),
			Labels: map[string]string{
				"service": "test",
			},
		})
		if err != nil {
			t.Fatalf("RecordMetric() failed: %v", err)
		}
	}

	// Query metrics
	query := &Query{
		Name:  "test_metric",
		Start: now.Add(-1 * time.Minute),
		End:   now.Add(1 * time.Minute),
		Labels: map[string]string{
			"service": "test",
		},
	}

	results, err := agg.QueryMetrics(query)
	if err != nil {
		t.Fatalf("QueryMetrics() failed: %v", err)
	}

	if len(results) != 5 {
		t.Errorf("QueryMetrics() returned %d results, want 5", len(results))
	}
}

// TestAggregator_QueryMetrics_TimeRange tests time range filtering
func TestAggregator_QueryMetrics_TimeRange(t *testing.T) {
	config := &Config{
		RetentionPeriod: 1 * time.Hour,
		MaxSeries:       1000,
		FlushInterval:   10 * time.Second,
	}

	agg, err := NewAggregator(config)
	if err != nil {
		t.Fatalf("NewAggregator() failed: %v", err)
	}
	defer agg.Shutdown(context.Background())

	// Record metrics across different times
	now := time.Now()
	timestamps := []time.Time{
		now.Add(-2 * time.Minute),
		now.Add(-1 * time.Minute),
		now,
		now.Add(1 * time.Minute),
	}

	for i, ts := range timestamps {
		err := agg.RecordMetric(&Metric{
			Name:      "time_test",
			Value:     float64(i),
			Timestamp: ts,
		})
		if err != nil && i < 3 { // Last one is future, should error
			t.Fatalf("RecordMetric() failed: %v", err)
		}
	}

	// Query only last minute
	query := &Query{
		Name:  "time_test",
		Start: now.Add(-90 * time.Second),
		End:   now.Add(10 * time.Second),
	}

	results, err := agg.QueryMetrics(query)
	if err != nil {
		t.Fatalf("QueryMetrics() failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("QueryMetrics() returned %d results, want 2", len(results))
	}
}

// TestAggregator_Aggregation tests metric aggregation functions
func TestAggregator_Aggregation(t *testing.T) {
	config := &Config{
		RetentionPeriod: 1 * time.Hour,
		MaxSeries:       1000,
		FlushInterval:   10 * time.Second,
	}

	agg, err := NewAggregator(config)
	if err != nil {
		t.Fatalf("NewAggregator() failed: %v", err)
	}
	defer agg.Shutdown(context.Background())

	// Record metrics
	now := time.Now()
	values := []float64{10, 20, 30, 40, 50}
	for _, val := range values {
		err := agg.RecordMetric(&Metric{
			Name:      "agg_test",
			Value:     val,
			Timestamp: now,
		})
		if err != nil {
			t.Fatalf("RecordMetric() failed: %v", err)
		}
	}

	tests := []struct {
		name     string
		function AggFunc
		want     float64
	}{
		{"sum", AggSum, 150.0},
		{"avg", AggAvg, 30.0},
		{"min", AggMin, 10.0},
		{"max", AggMax, 50.0},
		{"count", AggCount, 5.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := &Query{
				Name:      "agg_test",
				Start:     now.Add(-1 * time.Minute),
				End:       now.Add(1 * time.Minute),
				Aggregate: tt.function,
			}

			results, err := agg.QueryMetrics(query)
			if err != nil {
				t.Fatalf("QueryMetrics() failed: %v", err)
			}

			if len(results) != 1 {
				t.Fatalf("got %d results, want 1", len(results))
			}

			if results[0].Value != tt.want {
				t.Errorf("got value %f, want %f", results[0].Value, tt.want)
			}
		})
	}
}

// TestAggregator_Retention tests metric retention
func TestAggregator_Retention(t *testing.T) {
	config := &Config{
		RetentionPeriod: 100 * time.Millisecond,
		MaxSeries:       1000,
		FlushInterval:   50 * time.Millisecond,
	}

	agg, err := NewAggregator(config)
	if err != nil {
		t.Fatalf("NewAggregator() failed: %v", err)
	}
	defer agg.Shutdown(context.Background())

	// Record old metric
	err = agg.RecordMetric(&Metric{
		Name:      "retention_test",
		Value:     100.0,
		Timestamp: time.Now().Add(-200 * time.Millisecond),
	})
	if err != nil {
		t.Fatalf("RecordMetric() failed: %v", err)
	}

	// Wait for cleanup
	time.Sleep(200 * time.Millisecond)

	// Query should return no results
	query := &Query{
		Name:  "retention_test",
		Start: time.Now().Add(-1 * time.Minute),
		End:   time.Now(),
	}

	results, err := agg.QueryMetrics(query)
	if err != nil {
		t.Fatalf("QueryMetrics() failed: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("QueryMetrics() returned %d results after retention, want 0", len(results))
	}
}

// TestAggregator_MaxSeries tests series limit enforcement
func TestAggregator_MaxSeries(t *testing.T) {
	config := &Config{
		RetentionPeriod: 1 * time.Hour,
		MaxSeries:       3,
		FlushInterval:   10 * time.Second,
	}

	agg, err := NewAggregator(config)
	if err != nil {
		t.Fatalf("NewAggregator() failed: %v", err)
	}
	defer agg.Shutdown(context.Background())

	// Record metrics up to limit
	now := time.Now()
	for i := 0; i < 4; i++ {
		err := agg.RecordMetric(&Metric{
			Name:      "series_test",
			Value:     100.0,
			Timestamp: now,
			Labels: map[string]string{
				"series": string(rune('A' + i)),
			},
		})
		if i < 3 && err != nil {
			t.Errorf("RecordMetric(%d) unexpected error: %v", i, err)
		}
		if i == 3 && err == nil {
			t.Error("RecordMetric(3) should fail with max series reached")
		}
	}
}

// TestAggregator_ConcurrentAccess tests concurrent metric recording
func TestAggregator_ConcurrentAccess(t *testing.T) {
	config := &Config{
		RetentionPeriod: 1 * time.Hour,
		MaxSeries:       1000,
		FlushInterval:   10 * time.Second,
	}

	agg, err := NewAggregator(config)
	if err != nil {
		t.Fatalf("NewAggregator() failed: %v", err)
	}
	defer agg.Shutdown(context.Background())

	// Concurrent writes
	done := make(chan bool)
	workers := 10
	recordsPerWorker := 100

	for w := 0; w < workers; w++ {
		go func(workerID int) {
			for i := 0; i < recordsPerWorker; i++ {
				_ = agg.RecordMetric(&Metric{
					Name:      "concurrent_test",
					Value:     float64(i),
					Timestamp: time.Now(),
					Labels: map[string]string{
						"worker": string(rune('0' + workerID)),
					},
				})
			}
			done <- true
		}(w)
	}

	// Wait for all workers
	for w := 0; w < workers; w++ {
		<-done
	}

	// No race conditions should occur (test with -race flag)
}
