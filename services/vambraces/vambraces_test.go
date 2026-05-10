// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package vambraces

import (
	"context"
	"fmt"
	"io"
	"math"
	"sort"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/logger"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// floatEqual compares two float64 values with a tolerance of 1e-9.
func floatEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

// newTestService creates a Service wired to a silent logger and no wotan
// client, suitable for isolated unit tests.
func newTestService() *Service {
	log := logger.New(io.Discard)
	return NewService(log, nil, nil)
}

// newTestServiceWithConfig creates a Service with custom configuration.
func newTestServiceWithConfig(cfg *Config) *Service {
	log := logger.New(io.Discard)
	return NewService(log, nil, cfg)
}

// makeMetric is a shorthand for constructing a Metric.
func makeMetric(name string, mtype MetricType, value float64, ts time.Time, labels map[string]string) *Metric {
	return &Metric{
		Name:      name,
		Type:      mtype,
		Value:     value,
		Labels:    labels,
		Timestamp: ts,
	}
}

// ---------------------------------------------------------------------------
// DefaultConfig
// ---------------------------------------------------------------------------

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.MetricRetention != 7*24*time.Hour {
		t.Errorf("MetricRetention: want 7d, got %v", cfg.MetricRetention)
	}
	if cfg.TraceRetention != 24*time.Hour {
		t.Errorf("TraceRetention: want 24h, got %v", cfg.TraceRetention)
	}
	if cfg.ScrapeInterval != 15*time.Second {
		t.Errorf("ScrapeInterval: want 15s, got %v", cfg.ScrapeInterval)
	}
	if cfg.EvaluationInterval != 1*time.Minute {
		t.Errorf("EvaluationInterval: want 1m, got %v", cfg.EvaluationInterval)
	}
	if cfg.WotanTopic != "vambraces.observability" {
		t.Errorf("WotanTopic: want vambraces.observability, got %s", cfg.WotanTopic)
	}
}

// ---------------------------------------------------------------------------
// NewService
// ---------------------------------------------------------------------------

func TestNewService(t *testing.T) {
	t.Run("nil config uses defaults", func(t *testing.T) {
		svc := newTestService()
		if svc == nil {
			t.Fatal("expected non-nil service")
		}
		if svc.config == nil {
			t.Fatal("expected config to be set")
		}
		if svc.config.MetricRetention != 7*24*time.Hour {
			t.Error("expected default MetricRetention")
		}
		if svc.metrics == nil {
			t.Error("expected metrics map to be initialized")
		}
		if svc.traces == nil {
			t.Error("expected traces map to be initialized")
		}
		if svc.slos == nil {
			t.Error("expected slos map to be initialized")
		}
		if svc.alerts == nil {
			t.Error("expected alerts map to be initialized")
		}
		if svc.dashboards == nil {
			t.Error("expected dashboards map to be initialized")
		}
	})

	t.Run("custom config is honored", func(t *testing.T) {
		cfg := &Config{
			MetricRetention:    1 * time.Hour,
			TraceRetention:     30 * time.Minute,
			ScrapeInterval:     5 * time.Second,
			EvaluationInterval: 10 * time.Second,
			WotanTopic:         "custom.topic",
		}
		svc := newTestServiceWithConfig(cfg)
		if svc.config.MetricRetention != 1*time.Hour {
			t.Errorf("expected 1h retention, got %v", svc.config.MetricRetention)
		}
		if svc.config.WotanTopic != "custom.topic" {
			t.Errorf("expected custom.topic, got %s", svc.config.WotanTopic)
		}
	})
}

// ---------------------------------------------------------------------------
// Start / Stop
// ---------------------------------------------------------------------------

func TestStartStop(t *testing.T) {
	svc := newTestServiceWithConfig(&Config{
		MetricRetention:    1 * time.Hour,
		TraceRetention:     1 * time.Hour,
		ScrapeInterval:     1 * time.Second,
		EvaluationInterval: 50 * time.Millisecond,
		WotanTopic:         "test",
	})

	ctx, cancel := context.WithCancel(context.Background())

	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	// Let at least one evaluation tick fire to exercise evaluateAlerts + updateSLOs.
	time.Sleep(120 * time.Millisecond)

	// Cancel the context so the evaluation loop exits.
	cancel()

	if err := svc.Stop(); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
}

func TestStartContextCancellation(t *testing.T) {
	svc := newTestServiceWithConfig(&Config{
		MetricRetention:    1 * time.Hour,
		TraceRetention:     1 * time.Hour,
		ScrapeInterval:     1 * time.Second,
		EvaluationInterval: 24 * time.Hour, // very long, we cancel immediately
		WotanTopic:         "test",
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before start

	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	// Give the goroutine time to see the cancelled context and exit.
	time.Sleep(20 * time.Millisecond)

	if err := svc.Stop(); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// RecordMetric / GetMetrics — table-driven
// ---------------------------------------------------------------------------

func TestRecordMetric(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name      string
		metrics   []*Metric
		queryName string
		from      time.Time
		to        time.Time
		wantCount int
		desc      string
	}{
		{
			name: "single metric in range",
			metrics: []*Metric{
				makeMetric("cpu_usage", MetricGauge, 42.5, now, nil),
			},
			queryName: "cpu_usage",
			from:      now.Add(-1 * time.Second),
			to:        now.Add(1 * time.Second),
			wantCount: 1,
			desc:      "should find exactly one metric",
		},
		{
			name: "metric out of range (before from)",
			metrics: []*Metric{
				makeMetric("cpu_usage", MetricGauge, 42.5, now.Add(-10*time.Second), nil),
			},
			queryName: "cpu_usage",
			from:      now.Add(-5 * time.Second),
			to:        now.Add(1 * time.Second),
			wantCount: 0,
			desc:      "metric timestamp is before the from boundary",
		},
		{
			name: "metric out of range (after to)",
			metrics: []*Metric{
				makeMetric("cpu_usage", MetricGauge, 42.5, now.Add(10*time.Second), nil),
			},
			queryName: "cpu_usage",
			from:      now.Add(-1 * time.Second),
			to:        now.Add(1 * time.Second),
			wantCount: 0,
			desc:      "metric timestamp is after the to boundary",
		},
		{
			name: "multiple metrics, mixed in/out of range",
			metrics: []*Metric{
				makeMetric("mem_usage", MetricGauge, 1, now.Add(-10*time.Second), nil),
				makeMetric("mem_usage", MetricGauge, 2, now, nil),
				makeMetric("mem_usage", MetricGauge, 3, now.Add(1*time.Millisecond), nil),
				makeMetric("mem_usage", MetricGauge, 4, now.Add(10*time.Second), nil),
			},
			queryName: "mem_usage",
			from:      now.Add(-1 * time.Second),
			to:        now.Add(5 * time.Second),
			wantCount: 2,
			desc:      "only metrics strictly between from and to",
		},
		{
			name: "query nonexistent metric name",
			metrics: []*Metric{
				makeMetric("disk_io", MetricCounter, 100, now, nil),
			},
			queryName: "nonexistent",
			from:      now.Add(-1 * time.Second),
			to:        now.Add(1 * time.Second),
			wantCount: 0,
			desc:      "returns empty for unknown metric name",
		},
		{
			name:      "empty metrics store",
			metrics:   nil,
			queryName: "anything",
			from:      now.Add(-1 * time.Second),
			to:        now.Add(1 * time.Second),
			wantCount: 0,
			desc:      "returns nil for completely empty store",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService()
			for _, m := range tc.metrics {
				svc.RecordMetric(m)
			}
			got := svc.GetMetrics(tc.queryName, tc.from, tc.to)
			if len(got) != tc.wantCount {
				t.Errorf("%s: want %d metrics, got %d", tc.desc, tc.wantCount, len(got))
			}
		})
	}
}

func TestRecordMetric_ZeroTimestamp(t *testing.T) {
	svc := newTestService()

	m := &Metric{
		Name:  "auto_ts",
		Type:  MetricCounter,
		Value: 1,
		// Timestamp deliberately left zero.
	}

	before := time.Now()
	svc.RecordMetric(m)
	after := time.Now()

	if m.Timestamp.IsZero() {
		t.Fatal("expected timestamp to be populated")
	}
	if m.Timestamp.Before(before) || m.Timestamp.After(after) {
		t.Errorf("expected timestamp between %v and %v, got %v", before, after, m.Timestamp)
	}
}

func TestRecordMetric_PreserveExistingTimestamp(t *testing.T) {
	svc := newTestService()
	fixed := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)

	m := &Metric{
		Name:      "fixed_ts",
		Type:      MetricGauge,
		Value:     99,
		Timestamp: fixed,
	}
	svc.RecordMetric(m)

	if !m.Timestamp.Equal(fixed) {
		t.Errorf("expected timestamp to stay %v, got %v", fixed, m.Timestamp)
	}
}

func TestRecordMetric_AllTypes(t *testing.T) {
	types := []MetricType{MetricCounter, MetricGauge, MetricHistogram, MetricSummary}

	for _, mt := range types {
		t.Run(string(mt), func(t *testing.T) {
			svc := newTestService()
			m := makeMetric("test_"+string(mt), mt, 1.0, time.Now(), map[string]string{"env": "test"})
			svc.RecordMetric(m)

			got := svc.GetMetrics(m.Name, time.Now().Add(-time.Second), time.Now().Add(time.Second))
			if len(got) != 1 {
				t.Fatalf("expected 1 metric, got %d", len(got))
			}
			if got[0].Type != mt {
				t.Errorf("expected type %s, got %s", mt, got[0].Type)
			}
		})
	}
}

func TestRecordMetric_WithLabels(t *testing.T) {
	svc := newTestService()
	labels := map[string]string{"region": "us-east-1", "env": "production"}
	m := makeMetric("requests_total", MetricCounter, 42, time.Now(), labels)
	svc.RecordMetric(m)

	got := svc.GetMetrics("requests_total", time.Now().Add(-time.Second), time.Now().Add(time.Second))
	if len(got) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(got))
	}
	if got[0].Labels["region"] != "us-east-1" {
		t.Errorf("expected label region=us-east-1, got %s", got[0].Labels["region"])
	}
	if got[0].Labels["env"] != "production" {
		t.Errorf("expected label env=production, got %s", got[0].Labels["env"])
	}
}

func TestGetMetrics_BoundaryExclusive(t *testing.T) {
	svc := newTestService()
	exact := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	// Metric exactly at the boundary.
	svc.RecordMetric(makeMetric("boundary", MetricGauge, 1, exact, nil))

	// Query where from == metric timestamp; After(from) is false for equal timestamps.
	got := svc.GetMetrics("boundary", exact, exact.Add(time.Second))
	if len(got) != 0 {
		t.Errorf("expected 0 metrics (from boundary exclusive), got %d", len(got))
	}

	// Query where to == metric timestamp; Before(to) is false for equal timestamps.
	got = svc.GetMetrics("boundary", exact.Add(-time.Second), exact)
	if len(got) != 0 {
		t.Errorf("expected 0 metrics (to boundary exclusive), got %d", len(got))
	}
}

// ---------------------------------------------------------------------------
// Spans / Traces
// ---------------------------------------------------------------------------

func TestStartSpan(t *testing.T) {
	svc := newTestService()

	span := svc.StartSpan("trace-1", "span-1", "", "http.request", "gateway")
	if span == nil {
		t.Fatal("expected non-nil span")
	}
	if span.TraceID != "trace-1" {
		t.Errorf("TraceID: want trace-1, got %s", span.TraceID)
	}
	if span.SpanID != "span-1" {
		t.Errorf("SpanID: want span-1, got %s", span.SpanID)
	}
	if span.ParentID != "" {
		t.Errorf("ParentID: want empty, got %s", span.ParentID)
	}
	if span.Name != "http.request" {
		t.Errorf("Name: want http.request, got %s", span.Name)
	}
	if span.Service != "gateway" {
		t.Errorf("Service: want gateway, got %s", span.Service)
	}
	if span.Status != SpanOK {
		t.Errorf("Status: want ok, got %s", span.Status)
	}
	if span.StartTime.IsZero() {
		t.Error("StartTime should not be zero")
	}
	if span.EndTime != nil {
		t.Error("EndTime should be nil before ending")
	}
	if span.Tags == nil {
		t.Error("Tags map should be initialized")
	}
	if span.Logs == nil {
		t.Error("Logs slice should be initialized")
	}
}

func TestStartSpanWithParent(t *testing.T) {
	svc := newTestService()

	parent := svc.StartSpan("trace-1", "span-1", "", "parent.op", "svc-a")
	child := svc.StartSpan("trace-1", "span-2", parent.SpanID, "child.op", "svc-b")

	if child.ParentID != parent.SpanID {
		t.Errorf("child ParentID: want %s, got %s", parent.SpanID, child.ParentID)
	}
}

func TestEndSpan(t *testing.T) {
	svc := newTestService()

	span := svc.StartSpan("trace-1", "span-1", "", "test.op", "svc")
	time.Sleep(5 * time.Millisecond)
	svc.EndSpan(span, SpanOK)

	if span.EndTime == nil {
		t.Fatal("EndTime should be set after ending")
	}
	if span.Duration <= 0 {
		t.Errorf("Duration should be positive, got %v", span.Duration)
	}
	if span.Status != SpanOK {
		t.Errorf("Status: want ok, got %s", span.Status)
	}
}

func TestEndSpan_ErrorStatus(t *testing.T) {
	svc := newTestService()

	span := svc.StartSpan("trace-err", "span-err", "", "fail.op", "svc")
	svc.EndSpan(span, SpanError)

	if span.Status != SpanError {
		t.Errorf("Status: want error, got %s", span.Status)
	}
}

func TestGetTrace(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(svc *Service)
		traceID   string
		wantCount int
	}{
		{
			name:      "nonexistent trace returns nil",
			setup:     func(svc *Service) {},
			traceID:   "does-not-exist",
			wantCount: 0,
		},
		{
			name: "single span trace",
			setup: func(svc *Service) {
				svc.StartSpan("t1", "s1", "", "op", "svc")
			},
			traceID:   "t1",
			wantCount: 1,
		},
		{
			name: "multi-span trace",
			setup: func(svc *Service) {
				svc.StartSpan("t2", "s1", "", "op1", "svc-a")
				svc.StartSpan("t2", "s2", "s1", "op2", "svc-b")
				svc.StartSpan("t2", "s3", "s1", "op3", "svc-c")
			},
			traceID:   "t2",
			wantCount: 3,
		},
		{
			name: "separate traces are isolated",
			setup: func(svc *Service) {
				svc.StartSpan("t-a", "s1", "", "op1", "svc")
				svc.StartSpan("t-b", "s2", "", "op2", "svc")
			},
			traceID:   "t-a",
			wantCount: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService()
			tc.setup(svc)
			got := svc.GetTrace(tc.traceID)
			if len(got) != tc.wantCount {
				t.Errorf("want %d spans, got %d", tc.wantCount, len(got))
			}
		})
	}
}

func TestTraceCorrelation(t *testing.T) {
	svc := newTestService()

	// Simulate a distributed request across three services.
	root := svc.StartSpan("trace-corr", "span-gw", "", "gateway.handle", "gateway")
	root.Tags["http.method"] = "GET"
	root.Tags["http.path"] = "/api/v1/status"

	child1 := svc.StartSpan("trace-corr", "span-auth", root.SpanID, "auth.validate", "auth-service")
	child1.Logs = append(child1.Logs, SpanLog{
		Timestamp: time.Now(),
		Message:   "token validated",
		Fields:    map[string]interface{}{"user_id": "u-123"},
	})

	child2 := svc.StartSpan("trace-corr", "span-db", root.SpanID, "db.query", "data-service")
	child2.Tags["db.statement"] = "SELECT 1"

	svc.EndSpan(child1, SpanOK)
	svc.EndSpan(child2, SpanOK)
	svc.EndSpan(root, SpanOK)

	// Verify all spans share the same trace ID.
	spans := svc.GetTrace("trace-corr")
	if len(spans) != 3 {
		t.Fatalf("expected 3 spans, got %d", len(spans))
	}
	for _, s := range spans {
		if s.TraceID != "trace-corr" {
			t.Errorf("expected trace-corr, got %s", s.TraceID)
		}
	}

	// Verify parent-child relationships by scanning spans.
	spanMap := make(map[string]*Span)
	for _, s := range spans {
		spanMap[s.SpanID] = s
	}
	if spanMap["span-auth"].ParentID != "span-gw" {
		t.Errorf("auth span parent: want span-gw, got %s", spanMap["span-auth"].ParentID)
	}
	if spanMap["span-db"].ParentID != "span-gw" {
		t.Errorf("db span parent: want span-gw, got %s", spanMap["span-db"].ParentID)
	}
	if spanMap["span-gw"].ParentID != "" {
		t.Errorf("root span parent: want empty, got %s", spanMap["span-gw"].ParentID)
	}

	// Verify tags survived.
	if spanMap["span-gw"].Tags["http.method"] != "GET" {
		t.Error("root span missing http.method tag")
	}
	if spanMap["span-db"].Tags["db.statement"] != "SELECT 1" {
		t.Error("db span missing db.statement tag")
	}

	// Verify logs on child1.
	if len(spanMap["span-auth"].Logs) != 1 {
		t.Fatalf("expected 1 log on auth span, got %d", len(spanMap["span-auth"].Logs))
	}
	if spanMap["span-auth"].Logs[0].Message != "token validated" {
		t.Errorf("log message: want 'token validated', got '%s'", spanMap["span-auth"].Logs[0].Message)
	}

	// Verify all spans have durations after ending.
	for _, s := range spans {
		if s.Duration <= 0 {
			t.Errorf("span %s has non-positive duration %v", s.SpanID, s.Duration)
		}
		if s.EndTime == nil {
			t.Errorf("span %s has nil EndTime", s.SpanID)
		}
	}
}

// ---------------------------------------------------------------------------
// SLO Management
// ---------------------------------------------------------------------------

func TestCreateSLO(t *testing.T) {
	tests := []struct {
		name         string
		slo          *SLO
		wantIDPrefix string
		wantBudget   float64
		wantCurrent  float64
	}{
		{
			name: "with explicit ID",
			slo: &SLO{
				ID:        "slo-explicit",
				Name:      "API Availability",
				Service:   "gateway",
				Indicator: "availability",
				Target:    99.9,
				Window:    "30d",
			},
			wantIDPrefix: "slo-explicit",
			wantBudget:   0.1, // 100 - 99.9
			wantCurrent:  99.9,
		},
		{
			name: "auto-generated ID",
			slo: &SLO{
				Name:      "Latency P99",
				Service:   "timeguru",
				Indicator: "latency",
				Target:    99.0,
				Window:    "7d",
			},
			wantIDPrefix: "slo-timeguru-",
			wantBudget:   1.0,
			wantCurrent:  99.0,
		},
		{
			name: "error rate SLO",
			slo: &SLO{
				Name:      "Error Rate",
				Service:   "captain",
				Indicator: "error_rate",
				Target:    99.95,
				Window:    "7d",
			},
			wantIDPrefix: "slo-captain-",
			wantBudget:   0.05,
			wantCurrent:  99.95,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService()
			err := svc.CreateSLO(tc.slo)
			if err != nil {
				t.Fatalf("CreateSLO returned error: %v", err)
			}

			if tc.slo.ID == "" {
				t.Fatal("expected ID to be set")
			}
			if len(tc.wantIDPrefix) > 0 && tc.slo.ID[:len(tc.wantIDPrefix)] != tc.wantIDPrefix {
				// For auto-generated IDs, just check the prefix.
				if tc.name != "with explicit ID" {
					// Auto-generated IDs start with "slo-<service>-"
					if len(tc.slo.ID) < len(tc.wantIDPrefix) {
						t.Errorf("ID prefix: want %s, got %s", tc.wantIDPrefix, tc.slo.ID)
					}
				}
			}

			if !floatEqual(tc.slo.Budget, tc.wantBudget) {
				t.Errorf("Budget: want %f, got %f", tc.wantBudget, tc.slo.Budget)
			}
			if !floatEqual(tc.slo.Current, tc.wantCurrent) {
				t.Errorf("Current: want %f, got %f", tc.wantCurrent, tc.slo.Current)
			}
			if tc.slo.LastUpdated.IsZero() {
				t.Error("LastUpdated should be set")
			}
		})
	}
}

func TestGetSLO(t *testing.T) {
	t.Run("existing SLO", func(t *testing.T) {
		svc := newTestService()
		slo := &SLO{ID: "slo-1", Name: "Test", Service: "svc", Indicator: "availability", Target: 99.9}
		_ = svc.CreateSLO(slo)

		got, ok := svc.GetSLO("slo-1")
		if !ok {
			t.Fatal("expected SLO to be found")
		}
		if got.Name != "Test" {
			t.Errorf("Name: want Test, got %s", got.Name)
		}
	})

	t.Run("nonexistent SLO", func(t *testing.T) {
		svc := newTestService()
		_, ok := svc.GetSLO("nope")
		if ok {
			t.Error("expected SLO not to be found")
		}
	})
}

func TestListSLOs(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		svc := newTestService()
		got := svc.ListSLOs()
		if len(got) != 0 {
			t.Errorf("expected 0 SLOs, got %d", len(got))
		}
	})

	t.Run("multiple SLOs", func(t *testing.T) {
		svc := newTestService()
		for i := 0; i < 5; i++ {
			_ = svc.CreateSLO(&SLO{
				Name:      "SLO",
				Service:   "svc",
				Indicator: "availability",
				Target:    99.9,
			})
		}
		got := svc.ListSLOs()
		if len(got) != 5 {
			t.Errorf("expected 5 SLOs, got %d", len(got))
		}
	})
}

func TestSLOBudgetCalculation(t *testing.T) {
	tests := []struct {
		target     float64
		wantBudget float64
	}{
		{99.9, 0.1},
		{99.99, 0.01},
		{99.0, 1.0},
		{95.0, 5.0},
		{100.0, 0.0},
		{0.0, 100.0},
	}

	for _, tc := range tests {
		t.Run("", func(t *testing.T) {
			svc := newTestService()
			slo := &SLO{
				Name:      "Budget Test",
				Service:   "svc",
				Indicator: "availability",
				Target:    tc.target,
			}
			_ = svc.CreateSLO(slo)

			// Use a small epsilon for floating point comparison.
			diff := slo.Budget - tc.wantBudget
			if diff < -0.0001 || diff > 0.0001 {
				t.Errorf("target %.2f: want budget %.4f, got %.4f", tc.target, tc.wantBudget, slo.Budget)
			}
		})
	}
}

func TestSLOCurrentInitializedToTarget(t *testing.T) {
	svc := newTestService()
	slo := &SLO{
		Name:      "Init Test",
		Service:   "svc",
		Indicator: "latency",
		Target:    99.5,
	}
	_ = svc.CreateSLO(slo)

	if slo.Current != slo.Target {
		t.Errorf("Current should equal Target on creation: want %f, got %f", slo.Target, slo.Current)
	}
}

// ---------------------------------------------------------------------------
// Alert Management
// ---------------------------------------------------------------------------

func TestCreateAlert(t *testing.T) {
	tests := []struct {
		name         string
		alert        *Alert
		wantState    AlertState
		wantIDPrefix string
	}{
		{
			name: "with explicit ID",
			alert: &Alert{
				ID:        "alert-explicit",
				Name:      "High CPU",
				Query:     "cpu_usage > 90",
				Condition: ">",
				Threshold: 90,
				Duration:  "5m",
				Severity:  "critical",
			},
			wantState:    AlertInactive,
			wantIDPrefix: "alert-explicit",
		},
		{
			name: "auto-generated ID",
			alert: &Alert{
				Name:      "High Memory",
				Query:     "mem_usage > 80",
				Condition: ">",
				Threshold: 80,
				Duration:  "10m",
				Severity:  "warning",
			},
			wantState:    AlertInactive,
			wantIDPrefix: "alert-",
		},
		{
			name: "info severity",
			alert: &Alert{
				Name:      "Deployment",
				Query:     "deployments_total",
				Condition: ">",
				Threshold: 0,
				Duration:  "0s",
				Severity:  "info",
				Labels:    map[string]string{"team": "platform"},
			},
			wantState:    AlertInactive,
			wantIDPrefix: "alert-",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService()
			err := svc.CreateAlert(tc.alert)
			if err != nil {
				t.Fatalf("CreateAlert returned error: %v", err)
			}
			if tc.alert.State != tc.wantState {
				t.Errorf("State: want %s, got %s", tc.wantState, tc.alert.State)
			}
			if tc.alert.ID == "" {
				t.Error("expected ID to be set")
			}
		})
	}
}

func TestCreateAlert_SetsInactiveState(t *testing.T) {
	svc := newTestService()
	alert := &Alert{
		ID:       "a1",
		Name:     "Test",
		State:    AlertFiring, // Explicitly set to firing
		Severity: "critical",
	}
	_ = svc.CreateAlert(alert)

	// CreateAlert should force state to AlertInactive regardless of input.
	if alert.State != AlertInactive {
		t.Errorf("expected state to be forced to inactive, got %s", alert.State)
	}
}

func TestGetAlert(t *testing.T) {
	t.Run("existing alert", func(t *testing.T) {
		svc := newTestService()
		_ = svc.CreateAlert(&Alert{ID: "a1", Name: "Test Alert", Severity: "critical"})

		got, ok := svc.GetAlert("a1")
		if !ok {
			t.Fatal("expected alert to be found")
		}
		if got.Name != "Test Alert" {
			t.Errorf("Name: want Test Alert, got %s", got.Name)
		}
	})

	t.Run("nonexistent alert", func(t *testing.T) {
		svc := newTestService()
		_, ok := svc.GetAlert("nope")
		if ok {
			t.Error("expected alert not to be found")
		}
	})
}

func TestListAlerts(t *testing.T) {
	svc := newTestService()

	// Create alerts and manually set different states (since CreateAlert forces inactive).
	_ = svc.CreateAlert(&Alert{ID: "a1", Name: "Alert 1", Severity: "critical"})
	_ = svc.CreateAlert(&Alert{ID: "a2", Name: "Alert 2", Severity: "warning"})
	_ = svc.CreateAlert(&Alert{ID: "a3", Name: "Alert 3", Severity: "info"})

	// Manually set states for filter testing.
	svc.mu.Lock()
	svc.alerts["a2"].State = AlertFiring
	svc.alerts["a3"].State = AlertPending
	svc.mu.Unlock()

	tests := []struct {
		name      string
		state     AlertState
		wantCount int
	}{
		{"all alerts (empty filter)", "", 3},
		{"inactive only", AlertInactive, 1},
		{"firing only", AlertFiring, 1},
		{"pending only", AlertPending, 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := svc.ListAlerts(tc.state)
			if len(got) != tc.wantCount {
				t.Errorf("want %d alerts, got %d", tc.wantCount, len(got))
			}
		})
	}
}

func TestListAlerts_Empty(t *testing.T) {
	svc := newTestService()
	got := svc.ListAlerts("")
	if len(got) != 0 {
		t.Errorf("expected empty list, got %d", len(got))
	}
}

func TestAlertWithLabelsAndAnnotations(t *testing.T) {
	svc := newTestService()
	alert := &Alert{
		ID:       "a-labels",
		Name:     "Labeled Alert",
		Severity: "warning",
		Labels: map[string]string{
			"team":        "platform",
			"environment": "production",
		},
		Annotations: map[string]string{
			"summary":     "High CPU usage detected",
			"description": "CPU has been above 90% for 5 minutes",
			"runbook_url": "https://runbooks.example.com/high-cpu",
		},
	}
	_ = svc.CreateAlert(alert)

	got, ok := svc.GetAlert("a-labels")
	if !ok {
		t.Fatal("expected alert to be found")
	}
	if got.Labels["team"] != "platform" {
		t.Error("missing team label")
	}
	if got.Annotations["runbook_url"] != "https://runbooks.example.com/high-cpu" {
		t.Error("missing runbook_url annotation")
	}
}

// ---------------------------------------------------------------------------
// Alert Rule Evaluation
// ---------------------------------------------------------------------------

func TestEvaluateAlerts_DoesNotPanic(t *testing.T) {
	svc := newTestService()

	// Create several alerts to evaluate.
	for i := 0; i < 10; i++ {
		_ = svc.CreateAlert(&Alert{
			Name:      "Alert",
			Query:     "metric > 0",
			Condition: ">",
			Threshold: float64(i * 10),
			Severity:  "warning",
		})
	}

	// evaluateAlerts is a private method; we call it directly to confirm no panics.
	svc.evaluateAlerts()
}

func TestEvaluateAlerts_WithEmptyAlerts(t *testing.T) {
	svc := newTestService()
	// Should not panic on empty alerts map.
	svc.evaluateAlerts()
}

func TestUpdateSLOs_UpdatesLastUpdated(t *testing.T) {
	svc := newTestService()
	slo := &SLO{
		ID:        "slo-update",
		Name:      "Update Test",
		Service:   "svc",
		Indicator: "availability",
		Target:    99.9,
	}
	_ = svc.CreateSLO(slo)

	originalTime := slo.LastUpdated

	// Small sleep to ensure time moves forward.
	time.Sleep(2 * time.Millisecond)

	svc.updateSLOs()

	got, _ := svc.GetSLO("slo-update")
	if !got.LastUpdated.After(originalTime) {
		t.Errorf("expected LastUpdated to advance after updateSLOs")
	}
}

func TestUpdateSLOs_EmptySLOs(t *testing.T) {
	svc := newTestService()
	// Should not panic on empty SLOs map.
	svc.updateSLOs()
}

// ---------------------------------------------------------------------------
// Dashboard Management
// ---------------------------------------------------------------------------

func TestCreateDashboard(t *testing.T) {
	tests := []struct {
		name       string
		dashboard  *Dashboard
		wantPrefix string
	}{
		{
			name: "with explicit ID",
			dashboard: &Dashboard{
				ID:   "dash-explicit",
				Name: "System Overview",
				Panels: []Panel{
					{ID: "p1", Title: "CPU", Type: "graph", Query: "cpu_usage"},
				},
			},
			wantPrefix: "dash-explicit",
		},
		{
			name: "auto-generated ID",
			dashboard: &Dashboard{
				Name: "Network Dashboard",
				Panels: []Panel{
					{ID: "p1", Title: "Bandwidth", Type: "graph", Query: "net_bytes"},
					{ID: "p2", Title: "Packets", Type: "stat", Query: "net_packets"},
				},
			},
			wantPrefix: "dash-",
		},
		{
			name: "empty panels",
			dashboard: &Dashboard{
				Name:   "Empty Dashboard",
				Panels: []Panel{},
			},
			wantPrefix: "dash-",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService()
			err := svc.CreateDashboard(tc.dashboard)
			if err != nil {
				t.Fatalf("CreateDashboard returned error: %v", err)
			}
			if tc.dashboard.ID == "" {
				t.Error("expected ID to be set")
			}
		})
	}
}

func TestGetDashboard(t *testing.T) {
	t.Run("existing dashboard", func(t *testing.T) {
		svc := newTestService()
		_ = svc.CreateDashboard(&Dashboard{
			ID:   "d1",
			Name: "My Dashboard",
			Panels: []Panel{
				{ID: "p1", Title: "CPU", Type: "graph", Query: "cpu_usage"},
			},
		})

		got, ok := svc.GetDashboard("d1")
		if !ok {
			t.Fatal("expected dashboard to be found")
		}
		if got.Name != "My Dashboard" {
			t.Errorf("Name: want My Dashboard, got %s", got.Name)
		}
		if len(got.Panels) != 1 {
			t.Errorf("Panels: want 1, got %d", len(got.Panels))
		}
	})

	t.Run("nonexistent dashboard", func(t *testing.T) {
		svc := newTestService()
		_, ok := svc.GetDashboard("nope")
		if ok {
			t.Error("expected dashboard not to be found")
		}
	})
}

func TestDashboardPanelOptions(t *testing.T) {
	svc := newTestService()
	_ = svc.CreateDashboard(&Dashboard{
		ID:   "d-opts",
		Name: "Options Test",
		Panels: []Panel{
			{
				ID:    "p1",
				Title: "Latency",
				Type:  "graph",
				Query: "http_duration",
				Options: map[string]string{
					"unit":  "ms",
					"min":   "0",
					"color": "blue",
				},
			},
		},
	})

	got, _ := svc.GetDashboard("d-opts")
	if got.Panels[0].Options["unit"] != "ms" {
		t.Error("expected panel option unit=ms")
	}
}

// ---------------------------------------------------------------------------
// Stats
// ---------------------------------------------------------------------------

func TestStats(t *testing.T) {
	svc := newTestService()

	// Empty service stats.
	stats := svc.Stats()
	assertStatVal(t, stats, "metric_series", 0)
	assertStatVal(t, stats, "total_metrics", 0)
	assertStatVal(t, stats, "active_traces", 0)
	assertStatVal(t, stats, "total_slos", 0)
	assertStatVal(t, stats, "total_alerts", 0)
	assertStatVal(t, stats, "firing_alerts", 0)
	assertStatVal(t, stats, "dashboards", 0)

	// Populate with data.
	now := time.Now()
	svc.RecordMetric(makeMetric("cpu", MetricGauge, 42, now, nil))
	svc.RecordMetric(makeMetric("cpu", MetricGauge, 45, now, nil))
	svc.RecordMetric(makeMetric("mem", MetricGauge, 70, now, nil))
	svc.StartSpan("t1", "s1", "", "op", "svc")
	svc.StartSpan("t2", "s2", "", "op", "svc")
	_ = svc.CreateSLO(&SLO{Name: "SLO1", Service: "svc", Indicator: "availability", Target: 99.9})
	_ = svc.CreateAlert(&Alert{ID: "a1", Name: "Alert1", Severity: "critical"})
	_ = svc.CreateAlert(&Alert{ID: "a2", Name: "Alert2", Severity: "warning"})
	_ = svc.CreateDashboard(&Dashboard{ID: "d1", Name: "Dash1"})

	// Set one alert to firing.
	svc.mu.Lock()
	svc.alerts["a1"].State = AlertFiring
	svc.mu.Unlock()

	stats = svc.Stats()
	assertStatVal(t, stats, "metric_series", 2) // cpu, mem
	assertStatVal(t, stats, "total_metrics", 3) // 2 cpu + 1 mem
	assertStatVal(t, stats, "active_traces", 2) // t1, t2
	assertStatVal(t, stats, "total_slos", 1)
	assertStatVal(t, stats, "total_alerts", 2)
	assertStatVal(t, stats, "firing_alerts", 1)
	assertStatVal(t, stats, "dashboards", 1)
}

func assertStatVal(t *testing.T, stats map[string]interface{}, key string, want int) {
	t.Helper()
	got, ok := stats[key]
	if !ok {
		t.Errorf("missing stat key %q", key)
		return
	}
	if got != want {
		t.Errorf("stat %q: want %d, got %v", key, want, got)
	}
}

// ---------------------------------------------------------------------------
// Evaluation Loop Integration
// ---------------------------------------------------------------------------

func TestEvaluationLoopUpdatesState(t *testing.T) {
	svc := newTestServiceWithConfig(&Config{
		MetricRetention:    1 * time.Hour,
		TraceRetention:     1 * time.Hour,
		ScrapeInterval:     1 * time.Second,
		EvaluationInterval: 25 * time.Millisecond,
		WotanTopic:         "test",
	})

	_ = svc.CreateSLO(&SLO{
		ID:        "slo-loop",
		Name:      "Loop Test",
		Service:   "svc",
		Indicator: "availability",
		Target:    99.9,
	})

	origSLO, _ := svc.GetSLO("slo-loop")
	origTime := origSLO.LastUpdated

	ctx, cancel := context.WithCancel(context.Background())
	_ = svc.Start(ctx)

	// Wait for at least 2 evaluation ticks.
	time.Sleep(80 * time.Millisecond)
	cancel()
	_ = svc.Stop()

	updatedSLO, _ := svc.GetSLO("slo-loop")
	if !updatedSLO.LastUpdated.After(origTime) {
		t.Error("expected SLO LastUpdated to advance after evaluation loop ticks")
	}
}

// ---------------------------------------------------------------------------
// Concurrent Access (Race Safety)
// ---------------------------------------------------------------------------

func TestConcurrentMetricRecording(t *testing.T) {
	svc := newTestService()

	const goroutines = 50
	const metricsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < metricsPerGoroutine; i++ {
				m := &Metric{
					Name:      "concurrent_metric",
					Type:      MetricCounter,
					Value:     float64(gid*metricsPerGoroutine + i),
					Timestamp: time.Now(),
				}
				svc.RecordMetric(m)
			}
		}(g)
	}
	wg.Wait()

	all := svc.GetMetrics("concurrent_metric", time.Now().Add(-time.Minute), time.Now().Add(time.Minute))
	want := goroutines * metricsPerGoroutine
	if len(all) != want {
		t.Errorf("expected %d metrics, got %d", want, len(all))
	}
}

func TestConcurrentSpanCreation(t *testing.T) {
	svc := newTestService()

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(gid int) {
			defer wg.Done()
			traceID := "concurrent-trace"
			span := svc.StartSpan(traceID, "", "", "op", "svc")
			svc.EndSpan(span, SpanOK)
		}(g)
	}
	wg.Wait()

	spans := svc.GetTrace("concurrent-trace")
	if len(spans) != goroutines {
		t.Errorf("expected %d spans, got %d", goroutines, len(spans))
	}
}

func TestConcurrentSLOAccess(t *testing.T) {
	svc := newTestService()

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines * 2) // writers + readers

	// Writers
	for g := 0; g < goroutines; g++ {
		go func(gid int) {
			defer wg.Done()
			_ = svc.CreateSLO(&SLO{
				Name:      "Concurrent SLO",
				Service:   "svc",
				Indicator: "availability",
				Target:    99.9,
			})
		}(g)
	}

	// Readers
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			_ = svc.ListSLOs()
		}()
	}

	wg.Wait()

	slos := svc.ListSLOs()
	if len(slos) != goroutines {
		t.Errorf("expected %d SLOs, got %d", goroutines, len(slos))
	}
}

func TestConcurrentAlertAccess(t *testing.T) {
	svc := newTestService()

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// Writers
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			_ = svc.CreateAlert(&Alert{
				Name:     "Concurrent Alert",
				Severity: "warning",
			})
		}()
	}

	// Readers
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			_ = svc.ListAlerts("")
		}()
	}

	wg.Wait()

	alerts := svc.ListAlerts("")
	if len(alerts) != goroutines {
		t.Errorf("expected %d alerts, got %d", goroutines, len(alerts))
	}
}

func TestConcurrentDashboardAccess(t *testing.T) {
	svc := newTestService()

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// Writers
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			_ = svc.CreateDashboard(&Dashboard{Name: "Concurrent Dash"})
		}()
	}

	// Readers
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			_ = svc.Stats()
		}()
	}

	wg.Wait()
}

func TestConcurrentMixedOperations(t *testing.T) {
	svc := newTestService()

	ctx, cancel := context.WithCancel(context.Background())
	_ = svc.Start(ctx)
	defer func() {
		cancel()
		_ = svc.Stop()
	}()

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines * 6)

	// Metric writers
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			svc.RecordMetric(makeMetric("mixed", MetricCounter, 1, time.Now(), nil))
		}()
	}

	// Metric readers
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			svc.GetMetrics("mixed", time.Now().Add(-time.Minute), time.Now().Add(time.Minute))
		}()
	}

	// Span writers
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			span := svc.StartSpan("mixed-trace", "s", "", "op", "svc")
			svc.EndSpan(span, SpanOK)
		}()
	}

	// Trace readers
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			svc.GetTrace("mixed-trace")
		}()
	}

	// Stats readers
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			svc.Stats()
		}()
	}

	// SLO creators
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			_ = svc.CreateSLO(&SLO{Name: "Mixed", Service: "svc", Indicator: "availability", Target: 99.9})
		}()
	}

	wg.Wait()
}

// ---------------------------------------------------------------------------
// Edge Cases & Error Paths
// ---------------------------------------------------------------------------

func TestMetricTypes_Constants(t *testing.T) {
	if MetricCounter != "counter" {
		t.Errorf("MetricCounter: want counter, got %s", MetricCounter)
	}
	if MetricGauge != "gauge" {
		t.Errorf("MetricGauge: want gauge, got %s", MetricGauge)
	}
	if MetricHistogram != "histogram" {
		t.Errorf("MetricHistogram: want histogram, got %s", MetricHistogram)
	}
	if MetricSummary != "summary" {
		t.Errorf("MetricSummary: want summary, got %s", MetricSummary)
	}
}

func TestSpanStatus_Constants(t *testing.T) {
	if SpanOK != "ok" {
		t.Errorf("SpanOK: want ok, got %s", SpanOK)
	}
	if SpanError != "error" {
		t.Errorf("SpanError: want error, got %s", SpanError)
	}
}

func TestAlertState_Constants(t *testing.T) {
	if AlertInactive != "inactive" {
		t.Errorf("AlertInactive: want inactive, got %s", AlertInactive)
	}
	if AlertPending != "pending" {
		t.Errorf("AlertPending: want pending, got %s", AlertPending)
	}
	if AlertFiring != "firing" {
		t.Errorf("AlertFiring: want firing, got %s", AlertFiring)
	}
}

func TestRecordMetric_NilLabels(t *testing.T) {
	svc := newTestService()
	m := &Metric{
		Name:      "nil_labels",
		Type:      MetricGauge,
		Value:     1.0,
		Labels:    nil,
		Timestamp: time.Now(),
	}
	svc.RecordMetric(m)

	got := svc.GetMetrics("nil_labels", time.Now().Add(-time.Second), time.Now().Add(time.Second))
	if len(got) != 1 {
		t.Errorf("expected 1 metric, got %d", len(got))
	}
}

func TestRecordMetric_ZeroValue(t *testing.T) {
	svc := newTestService()
	m := makeMetric("zero_val", MetricGauge, 0.0, time.Now(), nil)
	svc.RecordMetric(m)

	got := svc.GetMetrics("zero_val", time.Now().Add(-time.Second), time.Now().Add(time.Second))
	if len(got) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(got))
	}
	if got[0].Value != 0.0 {
		t.Errorf("expected value 0.0, got %f", got[0].Value)
	}
}

func TestRecordMetric_NegativeValue(t *testing.T) {
	svc := newTestService()
	m := makeMetric("negative", MetricGauge, -42.5, time.Now(), nil)
	svc.RecordMetric(m)

	got := svc.GetMetrics("negative", time.Now().Add(-time.Second), time.Now().Add(time.Second))
	if len(got) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(got))
	}
	if got[0].Value != -42.5 {
		t.Errorf("expected value -42.5, got %f", got[0].Value)
	}
}

func TestRecordMetric_LargeValue(t *testing.T) {
	svc := newTestService()
	m := makeMetric("large", MetricCounter, 1e18, time.Now(), nil)
	svc.RecordMetric(m)

	got := svc.GetMetrics("large", time.Now().Add(-time.Second), time.Now().Add(time.Second))
	if len(got) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(got))
	}
	if got[0].Value != 1e18 {
		t.Errorf("expected value 1e18, got %f", got[0].Value)
	}
}

func TestRecordMetric_EmptyName(t *testing.T) {
	svc := newTestService()
	m := makeMetric("", MetricGauge, 1.0, time.Now(), nil)
	svc.RecordMetric(m)

	got := svc.GetMetrics("", time.Now().Add(-time.Second), time.Now().Add(time.Second))
	if len(got) != 1 {
		t.Errorf("expected metric with empty name to be stored, got %d", len(got))
	}
}

func TestGetMetrics_IdenticalFromTo(t *testing.T) {
	svc := newTestService()
	ts := time.Now()
	svc.RecordMetric(makeMetric("test", MetricGauge, 1, ts, nil))

	// from == to, no metric can be strictly between.
	got := svc.GetMetrics("test", ts, ts)
	if len(got) != 0 {
		t.Errorf("expected 0 metrics when from == to, got %d", len(got))
	}
}

func TestGetMetrics_ReversedTimeRange(t *testing.T) {
	svc := newTestService()
	ts := time.Now()
	svc.RecordMetric(makeMetric("test", MetricGauge, 1, ts, nil))

	// Reversed: from > to.
	got := svc.GetMetrics("test", ts.Add(time.Second), ts.Add(-time.Second))
	if len(got) != 0 {
		t.Errorf("expected 0 metrics when from > to, got %d", len(got))
	}
}

func TestSpan_DurationPositiveAfterEnd(t *testing.T) {
	svc := newTestService()
	span := svc.StartSpan("t", "s", "", "op", "svc")

	// Sleep briefly to ensure measurable duration.
	time.Sleep(1 * time.Millisecond)
	svc.EndSpan(span, SpanOK)

	if span.Duration <= 0 {
		t.Errorf("expected positive duration, got %v", span.Duration)
	}
}

func TestSpan_TagsMutable(t *testing.T) {
	svc := newTestService()
	span := svc.StartSpan("t", "s", "", "op", "svc")

	span.Tags["key1"] = "val1"
	span.Tags["key2"] = "val2"

	if span.Tags["key1"] != "val1" {
		t.Errorf("expected tag key1=val1, got %s", span.Tags["key1"])
	}
	if len(span.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(span.Tags))
	}
}

func TestSpan_LogsAppendable(t *testing.T) {
	svc := newTestService()
	span := svc.StartSpan("t", "s", "", "op", "svc")

	span.Logs = append(span.Logs, SpanLog{
		Timestamp: time.Now(),
		Message:   "log entry 1",
		Fields:    map[string]interface{}{"key": "value"},
	})
	span.Logs = append(span.Logs, SpanLog{
		Timestamp: time.Now(),
		Message:   "log entry 2",
	})

	if len(span.Logs) != 2 {
		t.Errorf("expected 2 logs, got %d", len(span.Logs))
	}
}

func TestSLO_ExplicitIDPreserved(t *testing.T) {
	svc := newTestService()
	slo := &SLO{
		ID:        "my-custom-id",
		Name:      "Custom",
		Service:   "svc",
		Indicator: "availability",
		Target:    99.9,
	}
	_ = svc.CreateSLO(slo)

	if slo.ID != "my-custom-id" {
		t.Errorf("expected ID to remain my-custom-id, got %s", slo.ID)
	}
}

func TestAlert_ExplicitIDPreserved(t *testing.T) {
	svc := newTestService()
	alert := &Alert{
		ID:       "my-alert-id",
		Name:     "Custom",
		Severity: "info",
	}
	_ = svc.CreateAlert(alert)

	if alert.ID != "my-alert-id" {
		t.Errorf("expected ID to remain my-alert-id, got %s", alert.ID)
	}
}

func TestDashboard_ExplicitIDPreserved(t *testing.T) {
	svc := newTestService()
	d := &Dashboard{
		ID:   "my-dash-id",
		Name: "Custom",
	}
	_ = svc.CreateDashboard(d)

	if d.ID != "my-dash-id" {
		t.Errorf("expected ID to remain my-dash-id, got %s", d.ID)
	}
}

func TestAlert_LastFiredNilOnCreate(t *testing.T) {
	svc := newTestService()
	alert := &Alert{ID: "a1", Name: "Test", Severity: "info"}
	_ = svc.CreateAlert(alert)

	got, _ := svc.GetAlert("a1")
	if got.LastFired != nil {
		t.Error("expected LastFired to be nil on newly created alert")
	}
}

func TestMultipleMetricSeries(t *testing.T) {
	svc := newTestService()
	now := time.Now()

	series := []string{"cpu_usage", "mem_usage", "disk_io", "net_bytes", "http_requests"}
	for _, name := range series {
		for i := 0; i < 10; i++ {
			svc.RecordMetric(makeMetric(name, MetricGauge, float64(i), now.Add(time.Duration(i)*time.Millisecond), nil))
		}
	}

	stats := svc.Stats()
	if stats["metric_series"] != len(series) {
		t.Errorf("expected %d series, got %v", len(series), stats["metric_series"])
	}
	if stats["total_metrics"] != len(series)*10 {
		t.Errorf("expected %d total metrics, got %v", len(series)*10, stats["total_metrics"])
	}
}

func TestMultipleTracesIsolation(t *testing.T) {
	svc := newTestService()

	for i := 0; i < 5; i++ {
		traceID := fmt.Sprintf("trace-%d", i)
		for j := 0; j < 3; j++ {
			svc.StartSpan(traceID, "", "", "op", "svc")
		}
	}

	stats := svc.Stats()
	if stats["active_traces"] != 5 {
		t.Errorf("expected 5 active traces, got %v", stats["active_traces"])
	}
}

// ---------------------------------------------------------------------------
// Metric Collection Tests - Aggregation Scenarios
// ---------------------------------------------------------------------------

func TestMetricCollection_TimeSeriesOrdering(t *testing.T) {
	svc := newTestService()
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	// Record metrics in non-chronological order.
	svc.RecordMetric(makeMetric("ordered", MetricGauge, 3, base.Add(3*time.Second), nil))
	svc.RecordMetric(makeMetric("ordered", MetricGauge, 1, base.Add(1*time.Second), nil))
	svc.RecordMetric(makeMetric("ordered", MetricGauge, 2, base.Add(2*time.Second), nil))

	got := svc.GetMetrics("ordered", base, base.Add(10*time.Second))
	if len(got) != 3 {
		t.Fatalf("expected 3 metrics, got %d", len(got))
	}

	// Verify we can sort by timestamp.
	sort.Slice(got, func(i, j int) bool {
		return got[i].Timestamp.Before(got[j].Timestamp)
	})

	for i, m := range got {
		if m.Value != float64(i+1) {
			t.Errorf("index %d: want value %d, got %f", i, i+1, m.Value)
		}
	}
}

func TestMetricCollection_HighVolume(t *testing.T) {
	svc := newTestService()
	now := time.Now()

	const count = 10000
	for i := 0; i < count; i++ {
		svc.RecordMetric(makeMetric("high_volume", MetricCounter, float64(i),
			now.Add(time.Duration(i)*time.Microsecond), nil))
	}

	got := svc.GetMetrics("high_volume", now.Add(-time.Second), now.Add(time.Second))
	if len(got) != count {
		t.Errorf("expected %d metrics, got %d", count, len(got))
	}
}

// ---------------------------------------------------------------------------
// SLO Calculation Edge Cases
// ---------------------------------------------------------------------------

func TestSLO_OverriddenByCreate(t *testing.T) {
	svc := newTestService()
	slo := &SLO{
		ID:        "override-test",
		Name:      "Override",
		Service:   "svc",
		Indicator: "availability",
		Target:    99.9,
		Current:   50.0,  // Should be overridden to Target.
		Budget:    999.0, // Should be overridden to 100 - Target.
	}
	_ = svc.CreateSLO(slo)

	if !floatEqual(slo.Current, 99.9) {
		t.Errorf("Current should be set to Target (99.9), got %f", slo.Current)
	}
	if !floatEqual(slo.Budget, 0.1) {
		t.Errorf("Budget should be 0.1 (100 - 99.9), got %f", slo.Budget)
	}
}

func TestSLO_SameIDOverwrites(t *testing.T) {
	svc := newTestService()

	slo1 := &SLO{ID: "same-id", Name: "First", Service: "svc", Indicator: "availability", Target: 99.0}
	slo2 := &SLO{ID: "same-id", Name: "Second", Service: "svc", Indicator: "latency", Target: 99.9}

	_ = svc.CreateSLO(slo1)
	_ = svc.CreateSLO(slo2)

	got, ok := svc.GetSLO("same-id")
	if !ok {
		t.Fatal("expected SLO to be found")
	}
	// The second write should overwrite.
	if got.Name != "Second" {
		t.Errorf("expected Second, got %s", got.Name)
	}
	if got.Target != 99.9 {
		t.Errorf("expected target 99.9, got %f", got.Target)
	}

	// Should still be only 1 SLO.
	if len(svc.ListSLOs()) != 1 {
		t.Errorf("expected 1 SLO after overwrite, got %d", len(svc.ListSLOs()))
	}
}

// ---------------------------------------------------------------------------
// Dashboard Overwrite
// ---------------------------------------------------------------------------

func TestDashboard_SameIDOverwrites(t *testing.T) {
	svc := newTestService()

	_ = svc.CreateDashboard(&Dashboard{ID: "d-same", Name: "First"})
	_ = svc.CreateDashboard(&Dashboard{ID: "d-same", Name: "Second"})

	got, ok := svc.GetDashboard("d-same")
	if !ok {
		t.Fatal("expected dashboard to be found")
	}
	if got.Name != "Second" {
		t.Errorf("expected Second, got %s", got.Name)
	}
}

// ---------------------------------------------------------------------------
// Alert Overwrite
// ---------------------------------------------------------------------------

func TestAlert_SameIDOverwrites(t *testing.T) {
	svc := newTestService()

	_ = svc.CreateAlert(&Alert{ID: "a-same", Name: "First", Severity: "info"})
	_ = svc.CreateAlert(&Alert{ID: "a-same", Name: "Second", Severity: "critical"})

	got, ok := svc.GetAlert("a-same")
	if !ok {
		t.Fatal("expected alert to be found")
	}
	if got.Name != "Second" {
		t.Errorf("expected Second, got %s", got.Name)
	}
	if got.Severity != "critical" {
		t.Errorf("expected critical, got %s", got.Severity)
	}
}

// ---------------------------------------------------------------------------
// Stats after mixed operations
// ---------------------------------------------------------------------------

func TestStats_FiringAlertsCount(t *testing.T) {
	svc := newTestService()

	// Create 5 alerts, set 3 to firing.
	for i := 0; i < 5; i++ {
		_ = svc.CreateAlert(&Alert{Name: "Alert", Severity: "warning"})
	}

	alerts := svc.ListAlerts("")
	svc.mu.Lock()
	for i, a := range alerts {
		if i < 3 {
			a.State = AlertFiring
		}
	}
	svc.mu.Unlock()

	stats := svc.Stats()
	if stats["firing_alerts"] != 3 {
		t.Errorf("expected 3 firing alerts, got %v", stats["firing_alerts"])
	}
}

// ---------------------------------------------------------------------------
// Benchmark (Optional, demonstrates performance characteristics)
// ---------------------------------------------------------------------------

func BenchmarkRecordMetric(b *testing.B) {
	svc := newTestService()
	m := &Metric{
		Name:      "bench_metric",
		Type:      MetricCounter,
		Value:     1.0,
		Timestamp: time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		svc.RecordMetric(m)
	}
}

func BenchmarkGetMetrics(b *testing.B) {
	svc := newTestService()
	now := time.Now()
	for i := 0; i < 1000; i++ {
		svc.RecordMetric(makeMetric("bench", MetricGauge, float64(i),
			now.Add(time.Duration(i)*time.Millisecond), nil))
	}

	from := now.Add(-time.Second)
	to := now.Add(2 * time.Second)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		svc.GetMetrics("bench", from, to)
	}
}

func BenchmarkStartSpan(b *testing.B) {
	svc := newTestService()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		svc.StartSpan("bench-trace", "span", "", "op", "svc")
	}
}

func BenchmarkStats(b *testing.B) {
	svc := newTestService()
	now := time.Now()

	// Populate with representative data.
	for i := 0; i < 100; i++ {
		svc.RecordMetric(makeMetric("metric_"+string(rune('a'+i%26)), MetricGauge, float64(i), now, nil))
	}
	for i := 0; i < 50; i++ {
		svc.StartSpan("trace", "s", "", "op", "svc")
	}
	for i := 0; i < 10; i++ {
		_ = svc.CreateSLO(&SLO{Name: "SLO", Service: "svc", Indicator: "availability", Target: 99.9})
		_ = svc.CreateAlert(&Alert{Name: "Alert", Severity: "warning"})
		_ = svc.CreateDashboard(&Dashboard{Name: "Dash"})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		svc.Stats()
	}
}
