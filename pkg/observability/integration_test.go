// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package observability

import (
	"context"
	"sync"
	"testing"
	"time"
)

// --- Task 4: Integration tests for Prometheus + ELK adapters ---

func TestPipeline_PrometheusELK_FanOut(t *testing.T) {
	prom := NewPrometheusAdapter()
	elk := NewELKAdapter("integration-test", 100)

	p := NewPipeline(prom, elk)
	ctx := context.Background()

	// Metric should go to Prometheus only (ELK doesn't support metrics)
	err := p.EmitMetric(ctx, Metric{
		Name:   "http_requests_total",
		Value:  10,
		Labels: map[string]string{"service": "timeguru"},
		Type:   MetricCounter,
	})
	if err != nil {
		t.Fatalf("EmitMetric: %v", err)
	}
	if prom.MetricCount() != 1 {
		t.Errorf("prometheus should have 1 metric, got %d", prom.MetricCount())
	}
	if elk.DocumentCount() != 0 {
		t.Error("ELK should not receive metrics")
	}

	// Log should go to ELK only (Prometheus doesn't support logs)
	err = p.EmitLog(ctx, LogEntry{
		Level:   "info",
		Message: "request processed",
		Service: "timeguru",
		TraceID: "trace-abc",
	})
	if err != nil {
		t.Fatalf("EmitLog: %v", err)
	}
	if elk.DocumentCount() != 1 {
		t.Errorf("ELK should have 1 document, got %d", elk.DocumentCount())
	}
	if prom.MetricCount() != 1 {
		t.Error("prometheus metric count should not change from log emit")
	}
}

func TestPipeline_MultipleLogAdapters_FanOut(t *testing.T) {
	elk := NewELKAdapter("test", 100)
	zerolog := NewZerologAdapter(100)
	loki := NewLokiAdapter(100)
	fluentd := NewFluentdAdapter("test", 100)

	p := NewPipeline(elk, zerolog, loki, fluentd)
	ctx := context.Background()

	entry := LogEntry{
		Level:   "warn",
		Message: "high latency detected",
		Service: "gateway",
		TraceID: "trace-xyz",
		Fields:  map[string]string{"latency_ms": "500"},
	}

	err := p.EmitLog(ctx, entry)
	if err != nil {
		t.Fatalf("EmitLog: %v", err)
	}

	// All log adapters should receive the entry
	if elk.DocumentCount() != 1 {
		t.Errorf("ELK: expected 1, got %d", elk.DocumentCount())
	}
	if zerolog.EntryCount() != 1 {
		t.Errorf("zerolog: expected 1, got %d", zerolog.EntryCount())
	}
	if loki.EntryCount() != 1 {
		t.Errorf("loki: expected 1, got %d", loki.EntryCount())
	}
	if fluentd.EventCount() != 1 {
		t.Errorf("fluentd: expected 1, got %d", fluentd.EventCount())
	}
}

func TestPipeline_EmitLog_ELK_FieldPreservation(t *testing.T) {
	elk := NewELKAdapter("test", 100)
	p := NewPipeline(elk)
	ctx := context.Background()

	now := time.Date(2026, 3, 5, 12, 0, 0, 0, time.UTC)
	p.EmitLog(ctx, LogEntry{
		Level:     "error",
		Message:   "connection refused",
		Service:   "wotan",
		TraceID:   "trace-err-001",
		Fields:    map[string]string{"target": "10.10.10.20", "port": "19000"},
		Timestamp: now,
	})

	docs := elk.Documents()
	if len(docs) != 1 {
		t.Fatalf("expected 1 doc, got %d", len(docs))
	}

	doc := docs[0]
	if doc.Service != "wotan" {
		t.Errorf("expected service wotan, got %s", doc.Service)
	}
	if doc.Level != "error" {
		t.Errorf("expected level error, got %s", doc.Level)
	}
	if doc.TraceID != "trace-err-001" {
		t.Errorf("expected trace_id trace-err-001, got %s", doc.TraceID)
	}
	if doc.Fields["target"] != "10.10.10.20" {
		t.Errorf("expected field target, got %v", doc.Fields)
	}
	if doc.Index != "test-2026.03.05" {
		t.Errorf("expected date-based index, got %s", doc.Index)
	}
}

func TestPipeline_EmitTrace_Jaeger(t *testing.T) {
	jaeger := NewJaegerAdapter(100)
	p := NewPipeline(jaeger)
	ctx := context.Background()

	now := time.Now()
	err := p.EmitTrace(ctx, Span{
		TraceID:   "trace-001",
		SpanID:    "span-001",
		ParentID:  "",
		Operation: "HTTP GET /api/v1/timeline",
		Service:   "timeguru",
		StartTime: now,
		Duration:  15 * time.Millisecond,
		Status:    SpanOK,
		Attributes: map[string]string{
			"http.method":      "GET",
			"http.status_code": "200",
			"http.path":        "/api/v1/timeline",
		},
	})
	if err != nil {
		t.Fatalf("EmitTrace: %v", err)
	}

	if jaeger.SpanCount() != 1 {
		t.Fatalf("expected 1 span, got %d", jaeger.SpanCount())
	}

	// Emit a child span
	err = p.EmitTrace(ctx, Span{
		TraceID:   "trace-001",
		SpanID:    "span-002",
		ParentID:  "span-001",
		Operation: "DB query",
		Service:   "timeguru",
		StartTime: now.Add(1 * time.Millisecond),
		Duration:  5 * time.Millisecond,
		Status:    SpanOK,
	})
	if err != nil {
		t.Fatalf("EmitTrace child: %v", err)
	}

	if jaeger.SpanCount() != 2 {
		t.Errorf("expected 2 spans, got %d", jaeger.SpanCount())
	}
}

func TestPipeline_ConcurrentMetrics_Prometheus(t *testing.T) {
	prom := NewPrometheusAdapter()
	p := NewPipeline(prom)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			p.EmitMetric(ctx, Metric{
				Name:   "concurrent_counter",
				Value:  1,
				Labels: map[string]string{"service": "test"},
				Type:   MetricCounter,
			})
		}(i)
	}
	wg.Wait()

	// All 200 increments should be counted (same label set = same metric key)
	expo := prom.Exposition()
	if expo == "" {
		t.Error("exposition should not be empty")
	}
	if prom.MetricCount() != 1 {
		t.Errorf("expected 1 metric key, got %d", prom.MetricCount())
	}
}

func TestPipeline_ConcurrentLogs_ELK(t *testing.T) {
	elk := NewELKAdapter("concurrent-test", 0)
	p := NewPipeline(elk)
	ctx := context.Background()

	var wg sync.WaitGroup
	n := 100
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			p.EmitLog(ctx, LogEntry{
				Level:   "info",
				Message: "concurrent log",
				Service: "test",
			})
		}(i)
	}
	wg.Wait()

	if elk.DocumentCount() != n {
		t.Errorf("expected %d documents, got %d", n, elk.DocumentCount())
	}
}

func TestPipeline_ConcurrentTraces(t *testing.T) {
	jaeger := NewJaegerAdapter(0)
	p := NewPipeline(jaeger)
	ctx := context.Background()

	var wg sync.WaitGroup
	n := 100
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			p.EmitTrace(ctx, Span{
				TraceID:   "trace-concurrent",
				SpanID:    "span",
				Operation: "op",
				Service:   "test",
				Duration:  time.Millisecond,
			})
		}(i)
	}
	wg.Wait()

	if jaeger.SpanCount() != n {
		t.Errorf("expected %d spans, got %d", n, jaeger.SpanCount())
	}
}

func TestPipeline_ClosePreventsFurtherEmit(t *testing.T) {
	prom := NewPrometheusAdapter()
	elk := NewELKAdapter("test", 100)
	jaeger := NewJaegerAdapter(100)

	p := NewPipeline(prom, elk, jaeger)

	// Close the pipeline
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	ctx := context.Background()

	// All signal types should fail after close
	if err := p.EmitMetric(ctx, Metric{Name: "test", Value: 1}); err != ErrAdapterClosed {
		t.Errorf("EmitMetric after close: expected ErrAdapterClosed, got %v", err)
	}
	if err := p.EmitLog(ctx, LogEntry{Message: "test"}); err != ErrAdapterClosed {
		t.Errorf("EmitLog after close: expected ErrAdapterClosed, got %v", err)
	}
	if err := p.EmitTrace(ctx, Span{SpanID: "test"}); err != ErrAdapterClosed {
		t.Errorf("EmitTrace after close: expected ErrAdapterClosed, got %v", err)
	}
	if err := p.EmitAlert(ctx, Alert{Name: "test"}); err != ErrAdapterClosed {
		t.Errorf("EmitAlert after close: expected ErrAdapterClosed, got %v", err)
	}

	// Individual adapters should also be closed
	if err := prom.EmitMetric(ctx, Metric{Name: "test", Value: 1}); err != ErrAdapterClosed {
		t.Errorf("prometheus should be closed, got %v", err)
	}
	if err := elk.EmitLog(ctx, LogEntry{Message: "test"}); err != ErrAdapterClosed {
		t.Errorf("ELK should be closed, got %v", err)
	}
	if err := jaeger.EmitTrace(ctx, Span{SpanID: "test"}); err != ErrAdapterClosed {
		t.Errorf("jaeger should be closed, got %v", err)
	}
}

func TestPipeline_Close_Idempotent(t *testing.T) {
	p := NewPipeline(NewPrometheusAdapter())

	// Closing twice should not panic
	if err := p.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestPipeline_AddAdapter_AfterEmit(t *testing.T) {
	prom := NewPrometheusAdapter()
	p := NewPipeline(prom)
	ctx := context.Background()

	// Emit before adding ELK
	p.EmitLog(ctx, LogEntry{Message: "before elk"})

	elk := NewELKAdapter("test", 100)
	p.AddAdapter(elk)

	// ELK should receive logs going forward
	p.EmitLog(ctx, LogEntry{Message: "after elk", Service: "test", Level: "info"})
	if elk.DocumentCount() != 1 {
		t.Errorf("ELK should have 1 doc after being added, got %d", elk.DocumentCount())
	}
}

func TestPipeline_ConcurrentAddAndEmit(t *testing.T) {
	p := NewPipeline()
	ctx := context.Background()

	var wg sync.WaitGroup

	// Concurrent adds
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.AddAdapter(NewPrometheusAdapter())
		}()
	}

	// Concurrent emits
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.EmitMetric(ctx, Metric{Name: "test", Value: 1, Type: MetricGauge})
		}()
	}

	wg.Wait()

	adapters := p.Adapters()
	if len(adapters) != 10 {
		t.Errorf("expected 10 adapters, got %d", len(adapters))
	}
}

func TestPipeline_EmitLog_TimestampAutoFill(t *testing.T) {
	elk := NewELKAdapter("test", 100)
	p := NewPipeline(elk)
	ctx := context.Background()

	before := time.Now()
	p.EmitLog(ctx, LogEntry{Message: "auto-timestamp", Service: "test", Level: "info"})
	after := time.Now()

	docs := elk.Documents()
	if len(docs) != 1 {
		t.Fatalf("expected 1 doc, got %d", len(docs))
	}

	// Timestamp string should be within the before/after window
	ts, err := time.Parse(time.RFC3339Nano, docs[0].Timestamp)
	if err != nil {
		t.Fatalf("failed to parse timestamp: %v", err)
	}
	if ts.Before(before) || ts.After(after) {
		t.Errorf("auto-filled timestamp %v not in [%v, %v]", ts, before, after)
	}
}
